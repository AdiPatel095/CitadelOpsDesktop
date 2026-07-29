package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	foreignLordsEventID    = 71
	foreignLordsMapTypeID  = 21
	bloodcrowEventID       = 103
	bloodcrowMapTypeID     = 34
	fixedInvasionRadius    = 50
	defaultInvasionRefresh = 300
	eventMedalsCurrency    = "MEDALS"
)

type AutoInvasionPolicy struct{}

type autoInvasionSettings struct {
	Version                  int            `json:"version"`
	SourceCastleID           State.CastleID `json:"sourceCastleId"`
	PresetID                 string         `json:"presetId"`
	ForeignLordsDifficultyID int64          `json:"foreignLordsDifficultyId"`
	BloodcrowDifficultyID    int64          `json:"bloodcrowDifficultyId"`
	ScoreTarget              int64          `json:"scoreTarget"`
	MinimumRemainingSec      int64          `json:"minimumRemainingSec"`
	CheckIntervalSec         int            `json:"checkIntervalSec"`
	MapRefreshIntervalSec    int            `json:"mapRefreshIntervalSec"`
	DailyAttackLimit         int64          `json:"dailyAttackLimit"`
	HorseTravelBoostID       int            `json:"horseTravelBoostId"`
	FortifyCurrency          string         `json:"fortifyCurrency,omitempty"`
}

func NewAutoInvasionPolicy() *AutoInvasionPolicy { return &AutoInvasionPolicy{} }

func (*AutoInvasionPolicy) ID() string { return "autoInvasion" }

func (*AutoInvasionPolicy) EnabledKey() string { return "auto_invasion" }

func (*AutoInvasionPolicy) WakeDomains() []string {
	return []string{"attacks", "map", "movements", "commanders", "units", "events", "event-scores", "invasion", "achievements", "player-protection"}
}

func (*AutoInvasionPolicy) WakeSections() []string {
	return []string{"automation.autoInvasion", AttackPresets.ConfigurationSection, commanderFeatureSection}
}

func (*AutoInvasionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (decision Decision, err error) {
	settings := autoInvasionSettings{
		MinimumRemainingSec: 1800,
		CheckIntervalSec:    30, MapRefreshIntervalSec: defaultInvasionRefresh, HorseTravelBoostID: -1,
	}
	if !decodeSection(snapshot.Configuration, "automation.autoInvasion", &settings) {
		return invasionWaiting(snapshot.Now, "Auto Invasion is not configured"), nil
	}
	settings.PresetID = strings.TrimSpace(settings.PresetID)
	settings.FortifyCurrency = strings.ToUpper(strings.TrimSpace(settings.FortifyCurrency))
	if settings.SourceCastleID <= 0 || settings.PresetID == "" || settings.ScoreTarget <= 0 ||
		settings.ForeignLordsDifficultyID <= 0 || settings.BloodcrowDifficultyID <= 0 {
		return invasionWaiting(snapshot.Now, "Choose a source castle, attack preset, both event difficulties, and score target"), nil
	}
	if !validAutoInvasionFortifyCurrency(settings.FortifyCurrency) {
		return invasionWaiting(snapshot.Now, "Choose a supported Auto Invasion fortification currency"), nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return invasionWaiting(snapshot.Now, "Choose a supported horse travel boost"), nil
	}
	if refresh, required := playerProtectionRefreshDecision(snapshot); required {
		return refresh, nil
	}
	defer func() {
		if err == nil {
			decision = capAtPlayerProtectionRefresh(snapshot, decision)
		}
	}()
	if snapshot.State.Player.ProtectionMode.PreparingOrActive(snapshot.Now) {
		return Decision{
			Status: "protected", Detail: "Protection Mode is preparing or active; Auto Invasion attacks are paused",
			NextCheckAt: snapshot.State.Player.ProtectionMode.Until().Add(time.Second),
		}, nil
	}

	score, found := snapshot.State.ActiveScalableEventScore()
	if !found {
		return invasionWaiting(snapshot.Now, "No scalable invasion event is active"), nil
	}
	targetTypeID, supported := invasionTargetType(score.EventID)
	if !supported {
		return invasionWaiting(snapshot.Now, "Auto Invasion supports Foreign Lords and Bloodcrow"), nil
	}
	fortifyCurrency, fortifyCurrencyValid := invasionFortifyCurrencyForEvent(
		settings.FortifyCurrency,
		score.EventID,
		snapshot.State.Invasion.FortifyCurrencies,
	)
	if !fortifyCurrencyValid {
		return invasionWaiting(snapshot.Now, "The selected fortification currency is not valid for the active invasion event"), nil
	}
	if fortifyCurrency != "" && fortifyCurrency != "C2" && len(snapshot.State.Invasion.FortifyCurrencies) > 0 &&
		!snapshot.State.Invasion.SupportsFortifyCurrency(fortifyCurrency) {
		return invasionWaiting(snapshot.Now, fmt.Sprintf(
			"The active invasion event does not offer %s for fortification; available currencies: %s",
			fortifyCurrency, strings.Join(snapshot.State.Invasion.FortifyCurrencies, ", "),
		)), nil
	}
	difficultyID := configuredInvasionDifficulty(settings, score.EventID)
	if snapshot.GameData == nil {
		return invasionWaiting(snapshot.Now, "Official event difficulty data is unavailable"), nil
	}
	difficulty, valid := snapshot.GameData.ScalableEvent(score.EventID, difficultyID)
	if !valid {
		return invasionWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not valid for event %d", difficultyID, score.EventID)), nil
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !snapshot.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return invasionWaiting(snapshot.Now, fmt.Sprintf("Difficulty %d is not unlocked by this player's achievements", difficultyID)), nil
	}
	if score.DifficultyID <= 0 {
		arguments, _ := json.Marshal(map[string]any{"eventId": score.EventID, "difficultyId": difficultyID})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Select difficulty %d for the active invasion event", difficultyID),
			NextCheckAt: snapshot.Now.Add(2 * time.Second),
			Request:     &Intent.Request{Name: "invasion.difficulty.select", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if score.PlayerScore >= settings.ScoreTarget {
		return Decision{
			Status: "complete", Detail: fmt.Sprintf("Score target reached: %d / %d", score.PlayerScore, settings.ScoreTarget),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
			Metrics:     map[string]float64{"score": float64(score.PlayerScore), "scoreTarget": float64(settings.ScoreTarget)},
		}, nil
	}
	if remaining := invasionEventRemaining(score, snapshot.Now); remaining >= 0 && remaining <= max(0, settings.MinimumRemainingSec) {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Event has %d seconds remaining; no new attacks will launch", remaining),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
		}, nil
	}

	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists {
		return invasionWaiting(snapshot.Now, fmt.Sprintf("Source castle %d is unavailable", settings.SourceCastleID)), nil
	}
	if source.KingdomID != 0 {
		return invasionWaiting(snapshot.Now, "Foreign Lords and Bloodcrow attacks require a Great Empire castle"), nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	preset, exists := AttackPresets.Find(document, settings.PresetID)
	if !exists {
		return invasionWaiting(snapshot.Now, "The selected CitadelOps attack preset no longer exists"), nil
	}
	activeTargets := activeInvasionTargets(snapshot.State, source.ID, targetTypeID, snapshot.Now)
	activeCount := len(activeTargets)
	metrics := map[string]float64{
		"score": float64(score.PlayerScore), "scoreTarget": float64(settings.ScoreTarget),
		"activeAttacks": float64(activeCount),
	}
	if _, blocked := dailyAttackLimitAllowance(
		snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics,
	); blocked != nil {
		return *blocked, nil
	}
	commanderIDs, commandersRestricted := commanderFeatureCandidates(
		snapshot.State,
		snapshot.Configuration,
		"autoInvasion",
	)
	if commandersRestricted && len(commanderIDs) == 0 {
		return Decision{
			Status: "waiting", Detail: "No commanders are assigned to Auto Invasion",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	}
	commanderID, commanderAvailable := nextAvailableFeatureCommander(snapshot.State, commanderIDs, commandersRestricted, snapshot.Now)
	if !commanderAvailable {
		detail := "No commander is currently available"
		if commandersRestricted {
			detail = "No assigned Auto Invasion commander is currently available"
		}
		return Decision{
			Status: "waiting", Detail: detail,
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	}

	refreshInterval := invasionMapRefreshInterval(settings.MapRefreshIntervalSec)
	lastScan := snapshot.State.Invasion.LastScannedAt[source.ID]
	if lastScan.IsZero() || snapshot.Now.Sub(lastScan) >= refreshInterval {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "radius": fixedInvasionRadius, "scanStartedAt": snapshot.Now,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Refresh invasion targets around %s", invasionCastleName(source)),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "invasion.map.scan", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}

	candidates := invasionCandidates(snapshot.State, source, targetTypeID, fixedInvasionRadius, lastScan, activeTargets)
	metrics["knownTargets"] = float64(len(candidates))
	if len(candidates) == 0 {
		nextScan := lastScan.Add(refreshInterval)
		if nextScan.Before(snapshot.Now) {
			nextScan = snapshot.Now.Add(2 * time.Second)
		}
		return Decision{
			Status: "idle", Detail: "No eligible invasion castle is available in the latest map scan",
			NextCheckAt: nextScan, Metrics: metrics,
		}, nil
	}
	target := candidates[0]
	if itemID, required, available, found, err := invasionCapacityShortage(
		snapshot, source, target, preset, commanderID,
	); err != nil {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("Cannot calculate %s inventory requirements: %v", preset.Name, err),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	} else if found {
		return Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Waiting for attack inventory: %s has %d of item %d; %s currently requires %d",
				invasionCastleName(source), available, itemID, preset.Name, required,
			),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	}
	attackArguments := map[string]any{
		"sourceCastleId": source.ID, "eventId": score.EventID, "scoreTarget": settings.ScoreTarget,
		"minimumRemainingSec": settings.MinimumRemainingSec, "targetTypeId": targetTypeID,
		"kingdomId": target.KingdomID, "targetX": target.X, "targetY": target.Y,
		"targetObjectId": target.ObjectID, "preset": preset,
		"fortifyCurrency":    fortifyCurrency,
		"horseTravelBoostId": settings.HorseTravelBoostID,
		"dailyAttackLimit":   settings.DailyAttackLimit,
	}
	if commandersRestricted {
		attackArguments["commanderIds"] = commanderIDs
	}
	arguments, _ := json.Marshal(attackArguments)
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Attack invasion castle %d:%d with %s", target.X, target.Y, preset.Name),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request:             &Intent.Request{Name: "invasion.attack", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func validAutoInvasionFortifyCurrency(currency string) bool {
	switch currency {
	case "", "GTO", "STO", "KM", "ST", eventMedalsCurrency, "C2":
		return true
	default:
		return false
	}
}

func invasionFortifyCurrencyForEvent(currency string, eventID int64, offered []string) (string, bool) {
	switch currency {
	case "":
		return "", true
	case "GTO", "STO", "C2":
		return currency, true
	case "KM", "ST", eventMedalsCurrency:
		if len(offered) > 0 {
			for _, candidate := range offered {
				switch strings.ToUpper(strings.TrimSpace(candidate)) {
				case "KM":
					return "KM", true
				case "ST":
					return "ST", true
				}
			}
			return "", false
		}
		switch eventID {
		case foreignLordsEventID:
			return "KM", true
		case bloodcrowEventID:
			return "ST", true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func configuredInvasionDifficulty(settings autoInvasionSettings, eventID int64) int64 {
	switch eventID {
	case foreignLordsEventID:
		return settings.ForeignLordsDifficultyID
	case bloodcrowEventID:
		return settings.BloodcrowDifficultyID
	default:
		return 0
	}
}

func invasionWaiting(now time.Time, detail string) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second)}
}

func invasionTargetType(eventID int64) (int, bool) {
	switch eventID {
	case foreignLordsEventID:
		return foreignLordsMapTypeID, true
	case bloodcrowEventID:
		return bloodcrowMapTypeID, true
	default:
		return 0, false
	}
}

func invasionEventRemaining(score State.ScalableEventScore, now time.Time) int64 {
	if score.RemainingSec <= 0 {
		return -1
	}
	elapsed := int64(0)
	if !score.ObservedAt.IsZero() && now.After(score.ObservedAt) {
		elapsed = int64(now.Sub(score.ObservedAt) / time.Second)
	}
	return max(0, score.RemainingSec-elapsed)
}

func invasionMapRefreshInterval(value int) time.Duration {
	if value < 60 {
		value = defaultInvasionRefresh
	}
	return time.Duration(min(3600, value)) * time.Second
}

func invasionPresetShortage(preset AttackPresets.Preset, source State.CastleState) (State.UnitID, int64, int64, bool) {
	requested := map[State.UnitID]int64{}
	for _, wave := range preset.Waves {
		for _, lane := range []AttackPresets.Lane{wave.Left, wave.Middle, wave.Right} {
			for _, slots := range [][]AttackPresets.Slot{lane.Troops, lane.Tools} {
				for _, slot := range slots {
					if slot.ItemID != nil && *slot.ItemID > 0 && slot.Quantity > 0 {
						requested[State.UnitID(*slot.ItemID)] += slot.Quantity
					}
				}
			}
		}
	}
	addPresetCourtyardRequirements(requested, preset, true)
	ids := make([]State.UnitID, 0, len(requested))
	for id := range requested {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		if required, available := requested[id], source.Units.Stationed[id]; required > available {
			return id, required, available, true
		}
	}
	return 0, 0, 0, false
}

func invasionCapacityShortage(
	snapshot Snapshot,
	source State.CastleState,
	target State.MapObservation,
	preset AttackPresets.Preset,
	commanderID State.CommanderID,
) (State.UnitID, int64, int64, bool, error) {
	level := target.Level
	if level <= 0 {
		level = int(target.ObjectID)
	}
	dialog := snapshot.State.AttackDialog
	useAttackDialog := dialog.SourceCastleID == source.ID && dialog.KingdomID == target.KingdomID &&
		dialog.Target.TypeID == target.TypeID && dialog.Target.X == target.X && dialog.Target.Y == target.Y
	capacity, err := (AttackCapacity.Resolver{}).Resolve(snapshot.State, snapshot.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: useAttackDialog,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("invasion:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: level,
			},
			Level: level, CastleTypeID: target.TypeID, PvP: true, LegendaryFight: true,
		},
	})
	if err != nil {
		return 0, 0, 0, false, err
	}
	itemID, required, available, shortage := invasionPresetShortage(
		AttackPresets.LimitToCapacity(preset, capacity),
		source,
	)
	return itemID, required, available, shortage, nil
}

func addPresetCourtyardRequirements(
	requested map[State.UnitID]int64,
	preset AttackPresets.Preset,
	includeTroops bool,
) {
	if includeTroops {
		for _, slot := range preset.CourtyardSupport.Troops {
			if slot.ItemID != nil && *slot.ItemID > 0 && slot.Quantity > 0 {
				requested[State.UnitID(*slot.ItemID)] += slot.Quantity
			}
		}
	}
	for _, slot := range preset.CourtyardSupport.Tools {
		if slot.ItemID != nil && *slot.ItemID > 0 {
			requested[State.UnitID(*slot.ItemID)]++
		}
	}
}

func activeInvasionTargets(gameState State.GameState, sourceCastleID State.CastleID, targetTypeID int, now time.Time) map[string]struct{} {
	result := map[string]struct{}{}
	for _, movement := range gameState.Movements {
		if movement.Direction != 0 || movement.SourceCastleID != sourceCastleID || movement.TargetTypeID != targetTypeID {
			continue
		}
		if movement.ArrivesAt != nil && !movement.ArrivesAt.IsZero() && !movement.ArrivesAt.After(now) &&
			movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() && !movement.ReturnsAt.After(now) {
			continue
		}
		result[fmt.Sprintf("%d:%d:%d", movement.KingdomID, movement.TargetX, movement.TargetY)] = struct{}{}
	}
	return result
}

func invasionCandidates(
	gameState State.GameState,
	source State.CastleState,
	targetTypeID int,
	radius int,
	lastScan time.Time,
	active map[string]struct{},
) []State.MapObservation {
	result := make([]State.MapObservation, 0)
	for _, target := range gameState.Map[source.KingdomID] {
		key := fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)
		if target.TypeID != targetTypeID || target.ObservedAt.Before(lastScan) || invasionDistanceSquared(source, target) > radius*radius {
			continue
		}
		if _, busy := active[key]; busy {
			continue
		}
		result = append(result, target)
	}
	sort.Slice(result, func(left, right int) bool {
		leftDistance := invasionDistanceSquared(source, result[left])
		rightDistance := invasionDistanceSquared(source, result[right])
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if result[left].Y != result[right].Y {
			return result[left].Y < result[right].Y
		}
		return result[left].X < result[right].X
	})
	return result
}

func invasionDistanceSquared(source State.CastleState, target State.MapObservation) int {
	x, y := target.X-source.X, target.Y-source.Y
	return x*x + y*y
}

func invasionCastleName(castle State.CastleState) string {
	if name := strings.TrimSpace(castle.Name); name != "" {
		return name
	}
	return fmt.Sprintf("castle %d", castle.ID)
}
