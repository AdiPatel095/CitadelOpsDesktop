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
	"CitadelDesktop/Server/GameData"
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
	// invasionTargetFreshness is how recently the game must have confirmed a
	// candidate before the policy attacks it. Older than this and the policy
	// first refreshes the candidate's neighborhood (one gaa window) so the pick
	// is made from a live set — invasion castles are defeated by other players
	// constantly and a minutes-old sweep routinely offers dead ones.
	invasionTargetFreshness = 60 * time.Second
	// invasionNeighborhoodHalfSize keeps the pre-pick refresh to a single
	// 49x49 window (2401 tiles) centered on the candidate.
	invasionNeighborhoodHalfSize = 24
	eventMedalsCurrency          = "MEDALS"
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
	return []string{"attacks", "map-invasion", "movements", "commanders", "units", "events", "event-scores", "invasion", "achievements", "player-protection"}
}

func (*AutoInvasionPolicy) WakeSections() []string {
	return []string{"automation.autoInvasion", AttackPresets.ConfigurationSection, commanderFeatureSection}
}

func (*AutoInvasionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (decision Decision, err error) {

	settings := autoInvasionSettings{
		MinimumRemainingSec: 1800,
		CheckIntervalSec:    30, MapRefreshIntervalSec: defaultInvasionRefresh, HorseTravelBoostID: -1,
	}
	configured := decodeSection(snapshot.Configuration, "automation.autoInvasion", &settings)
	settings.PresetID = strings.TrimSpace(settings.PresetID)
	settings.FortifyCurrency = strings.ToUpper(strings.TrimSpace(settings.FortifyCurrency))
	if !configured {
		return invasionWaiting(snapshot.Now, "Auto Invasion is not configured"), nil
	}
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
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{foreignLordsEventID, bloodcrowEventID}, "Foreign Lords or Bloodcrow event",
		); locked {
			return decision, nil
		}
		return invasionWaiting(snapshot.Now, "No scalable invasion event is active"), nil
	}
	targetTypeID, supported := invasionTargetType(score.EventID)
	if !supported {
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{foreignLordsEventID, bloodcrowEventID}, "Foreign Lords or Bloodcrow event",
		); locked {
			return decision, nil
		}
		return invasionWaiting(snapshot.Now, "Auto Invasion supports Foreign Lords and Bloodcrow"), nil
	}
	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists {
		return invasionWaiting(snapshot.Now, fmt.Sprintf("Source castle %d is unavailable", settings.SourceCastleID)), nil
	}
	if source.KingdomID != 0 {
		return invasionWaiting(snapshot.Now, "Foreign Lords and Bloodcrow attacks require a Great Empire castle"), nil
	}
	activeTargets := activeInvasionTargets(snapshot.State, targetTypeID, snapshot.Now)
	activeCount := activeInvasionAttackCount(snapshot.State, source.ID, targetTypeID, snapshot.Now)
	metrics := map[string]float64{
		"score": float64(score.PlayerScore), "scoreTarget": float64(settings.ScoreTarget),
		"activeAttacks": float64(activeCount),
		"movingTargets": float64(len(activeTargets)),
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
			Metrics:     metrics,
		}, nil
	}
	if remaining := invasionEventRemaining(score, snapshot.Now); remaining >= 0 && remaining <= max(0, settings.MinimumRemainingSec) {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Event has %d seconds remaining; no new attacks will launch", remaining),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
		}, nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	preset, exists := AttackPresets.Find(document, settings.PresetID)
	if !exists {
		return invasionWaiting(snapshot.Now, "The selected CitadelOps attack preset no longer exists"), nil
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

	pool := invasionCandidatePool(snapshot.State, source, targetTypeID, fixedInvasionRadius, lastScan)
	metrics["knownTargets"] = float64(len(pool))
	if len(pool) == 0 {
		nextScan := lastScan.Add(refreshInterval)
		if nextScan.Before(snapshot.Now) {
			nextScan = snapshot.Now.Add(2 * time.Second)
		}
		return Decision{
			Status: "idle", Detail: "No eligible invasion castle is available in the latest map scan",
			NextCheckAt: nextScan, Metrics: metrics,
		}, nil
	}
	candidates, blocked := availableInvasionCandidates(
		snapshot.State, pool, activeTargets, score.EventID, targetTypeID, snapshot.Now,
	)
	metrics["availableTargets"] = float64(len(candidates))
	metrics["busyTargets"] = float64(blocked.busy)
	metrics["reservedTargets"] = float64(blocked.reserved)
	metrics["settlingTargets"] = float64(blocked.settling)
	metrics["unavailableTargets"] = float64(blocked.unavailable)
	metrics["unconfirmedTargets"] = float64(blocked.unconfirmed)
	if len(candidates) == 0 {
		nextCheck := snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30))
		if nextScan := lastScan.Add(refreshInterval); nextScan.Before(nextCheck) {
			nextCheck = nextScan
		}
		if target, refreshAt, found := nextUnconfirmedInvasionTarget(pool, snapshot.Now); found {
			if !refreshAt.After(snapshot.Now) {
				bounds := invasionNeighborhoodBounds(target)
				arguments, _ := json.Marshal(map[string]any{
					"sourceCastleId": source.ID, "radius": fixedInvasionRadius, "scanStartedAt": snapshot.Now,
					"bounds": bounds,
				})
				return Decision{
					Status:      "ready",
					Detail:      fmt.Sprintf("Refresh invasion target %d:%d to confirm attack availability", target.X, target.Y),
					NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
					Request:             &Intent.Request{Name: "invasion.map.scan", Arguments: arguments},
					ReevaluateOnSuccess: true,
				}, nil
			}
			if refreshAt.Before(nextCheck) {
				nextCheck = refreshAt
			}
		}
		return Decision{
			Status: "idle",
			Detail: fmt.Sprintf(
				"No attackable invasion castle is available: %d moving, %d reserved, %d settling, %d hidden or protected, %d awaiting confirmation",
				blocked.busy, blocked.reserved, blocked.settling, blocked.unavailable, blocked.unconfirmed,
			),
			NextCheckAt: nextCheck, Metrics: metrics,
		}, nil
	}
	target := candidates[0]
	if target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) > invasionTargetFreshness {
		// Fresh set before the pick: refresh the candidate's neighborhood so a
		// castle that vanished since the last sweep is dropped (the window is
		// authoritative) and the next evaluation chooses from live targets.
		bounds := invasionNeighborhoodBounds(target)
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "radius": fixedInvasionRadius, "scanStartedAt": snapshot.Now,
			"bounds": bounds,
		})
		return Decision{
			Status: "ready",
			Detail: fmt.Sprintf(
				"Refresh invasion targets around %d:%d before attacking (last confirmed %s ago)",
				target.X, target.Y, snapshot.Now.Sub(target.ObservedAt).Round(time.Second),
			),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "invasion.map.scan", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if itemID, required, available, found, err := invasionCapacityShortage(
		snapshot, source, target, preset, commanderID,
	); err != nil {
		if decision, refresh := generalSkillsRefreshDecision(err, snapshot.Now, metrics); refresh {
			return decision, nil
		}
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
		"sourceCastleId": source.ID, "eventId": score.EventID,
		"eventEndsAt": State.ScalableEventEndsAt(score), "scoreTarget": settings.ScoreTarget,
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
				candidate = strings.ToUpper(strings.TrimSpace(candidate))
				if candidate != "" && candidate != "GTO" && candidate != "STO" && candidate != "C2" {
					return candidate, true
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

func invasionPresetShortage(
	preset AttackPresets.Preset,
	source State.CastleState,
	gameData *GameData.Store,
) (State.UnitID, int64, int64, bool, error) {
	_, shortage, err := AttackPresets.CheckInventory(preset, source.Units.Stationed, gameData, 1)
	if err != nil {
		return 0, 0, 0, false, err
	}
	if shortage == nil {
		return 0, 0, 0, false, nil
	}
	return shortage.ItemID, shortage.Required, shortage.Available, true, nil
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
	itemID, required, available, shortage, err := invasionPresetShortage(
		AttackPresets.LimitToCapacity(preset, capacity),
		source,
		snapshot.GameData,
	)
	return itemID, required, available, shortage, err
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

func activeInvasionTargets(gameState State.GameState, targetTypeID int, now time.Time) map[string]struct{} {
	result := map[string]struct{}{}
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		for _, target := range State.MovementMapEndpoints(movement) {
			if !State.MovementOccupiesMapTargetAt(movement, State.MapTargetKey{
				KingdomID: target.KingdomID, TypeID: targetTypeID, X: target.X, Y: target.Y,
			}, now) {
				continue
			}
			result[State.InvasionTargetKey(target.KingdomID, target.X, target.Y)] = struct{}{}
		}
		return true
	})
	return result
}

func activeInvasionAttackCount(
	gameState State.GameState,
	sourceCastleID State.CastleID,
	targetTypeID int,
	now time.Time,
) int {
	active := 0
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if !State.MovementOwnedByCurrentPlayer(gameState, movement) ||
			movement.Direction == 0 && movement.SourceCastleID != sourceCastleID ||
			movement.Direction == 1 && movement.TargetCastleID != sourceCastleID {
			return true
		}
		target, found := State.MovementMapTarget(movement)
		if !found || target.TypeID > 0 && target.TypeID != targetTypeID ||
			!State.MovementOccupiesMapTargetAt(movement, State.MapTargetKey{
				KingdomID: target.KingdomID, TypeID: targetTypeID, X: target.X, Y: target.Y,
			}, now) {
			return true
		}
		active++
		return true
	})
	return active
}

func invasionReservationDueForReconciliation(
	gameState State.GameState,
	now time.Time,
) (State.InvasionTargetReservation, State.MovementID, bool) {
	keys := make([]string, 0, len(gameState.Invasion.TargetReservations))
	for key := range gameState.Invasion.TargetReservations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Positive evidence wins globally. A due absence/refresh candidate that is
	// first by coordinate must never starve a later reservation whose exact
	// movement is already available for accounting.
	for _, key := range keys {
		reservation := gameState.Invasion.TargetReservations[key]
		if reservation.OperationID == "" || reservation.ReservedAt.IsZero() {
			continue
		}
		if movement, matched := State.InvasionReservationMovement(gameState, reservation); matched {
			return reservation, movement.ID, true
		}
	}
	for _, key := range keys {
		reservation := gameState.Invasion.TargetReservations[key]
		if reservation.OperationID == "" || reservation.ReservedAt.IsZero() {
			continue
		}
		occurrence, occurrenceKnown := gameState.LookupEventOccurrence(reservation.EventID)
		occurrenceAdvanced := !reservation.OccurrenceEndsAt.IsZero() && occurrenceKnown &&
			!State.SameEventOccurrence(reservation.OccurrenceEndsAt, occurrence.EndsAt)
		if !reservation.RecoveryExhaustedAt.IsZero() && !occurrenceAdvanced {
			continue
		}
		if occurrenceAdvanced {
			return reservation, 0, true
		}
		dueAt := reservation.ReservedAt.Add(State.InvasionTargetReservationReconcileGrace)
		if reservation.ReconcileAfter.After(dueAt) {
			dueAt = reservation.ReconcileAfter
		}
		if now.Before(dueAt) {
			continue
		}
		reservationOccurrenceKnown := !reservation.OccurrenceEndsAt.IsZero() && occurrenceKnown &&
			State.SameEventOccurrence(reservation.OccurrenceEndsAt, occurrence.EndsAt)
		if reservationOccurrenceKnown && State.AnyActiveMovementAtMapTarget(gameState, State.MapTargetKey{
			KingdomID: reservation.KingdomID, TypeID: reservation.TargetTypeID,
			X: reservation.X, Y: reservation.Y,
		}, now) {
			continue
		}
		return reservation, 0, true
	}
	return State.InvasionTargetReservation{}, 0, false
}

func invasionReservationReconciliationDecision(snapshot Snapshot, preferred State.CastleID) (Decision, bool) {
	reservation, movementID, found := invasionReservationDueForReconciliation(snapshot.State, snapshot.Now)
	if !found {
		return Decision{}, false
	}
	arguments := map[string]any{
		"kingdomId":        reservation.KingdomID,
		"eventId":          reservation.EventID,
		"occurrenceEndsAt": reservation.OccurrenceEndsAt,
		"targetTypeId":     reservation.TargetTypeID,
		"targetX":          reservation.X,
		"targetY":          reservation.Y,
		"operationId":      reservation.OperationID,
		"reservedAt":       reservation.ReservedAt,
		"reconcileAfter":   reservation.ReconcileAfter,
	}
	if movementID > 0 {
		arguments["matchedMovementId"] = movementID
		encoded, _ := json.Marshal(arguments)
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Record confirmed invasion launch at %d:%d", reservation.X, reservation.Y),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Metrics:             map[string]float64{"unresolvedLaunches": 1, "confirmedLaunches": 1},
			Request:             &Intent.Request{Name: "invasion.target.reconcile", Arguments: encoded},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, true
	}
	occurrence, occurrenceKnown := snapshot.State.LookupEventOccurrence(reservation.EventID)
	if !reservation.OccurrenceEndsAt.IsZero() && occurrenceKnown &&
		!State.SameEventOccurrence(reservation.OccurrenceEndsAt, occurrence.EndsAt) {
		encoded, _ := json.Marshal(arguments)
		return Decision{
			Status: "ready", Detail: fmt.Sprintf(
				"Release prior-occurrence invasion reservation at %d:%d", reservation.X, reservation.Y,
			),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Metrics:             map[string]float64{"unresolvedLaunches": 1, "priorOccurrenceReservations": 1},
			Request:             &Intent.Request{Name: "invasion.target.reconcile", Arguments: encoded},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, true
	}
	if preferred <= 0 {
		preferred = reservation.SourceCastleID
	}
	source, sourceFound := snapshot.State.Castles[preferred]
	if !sourceFound || source.KingdomID != reservation.KingdomID {
		ids := make([]int64, 0, len(snapshot.State.Castles))
		for id, castle := range snapshot.State.Castles {
			if castle.KingdomID == reservation.KingdomID {
				ids = append(ids, int64(id))
			}
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		if len(ids) > 0 {
			source, sourceFound = snapshot.State.Castles[State.CastleID(ids[0])]
		}
	}
	if !sourceFound {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf(
				"Cannot reconcile unresolved invasion launch at %d:%d without a castle in kingdom %d",
				reservation.X, reservation.Y, reservation.KingdomID,
			),
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, true
	}
	arguments["sourceCastleId"] = source.ID
	encoded, _ := json.Marshal(arguments)
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Reconcile unresolved invasion launch at %d:%d", reservation.X, reservation.Y),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"unresolvedLaunches": 1},
		Request:             &Intent.Request{Name: "invasion.target.reconcile", Arguments: encoded},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, true
}

func invasionCandidatePool(
	gameState State.GameState,
	source State.CastleState,
	targetTypeID int,
	radius int,
	lastScan time.Time,
) []State.MapObservation {
	result := make([]State.MapObservation, 0)
	gameState.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionInvasion, func(_ string, target State.MapObservation) bool {
		if target.TypeID != targetTypeID || target.ObservedAt.Before(lastScan) || invasionDistanceSquared(source, target) > radius*radius {
			return true
		}
		result = append(result, target)
		return true
	})
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

type invasionBlockedTargets struct {
	busy        int
	reserved    int
	settling    int
	unavailable int
	unconfirmed int
}

func availableInvasionCandidates(
	gameState State.GameState,
	pool []State.MapObservation,
	active map[string]struct{},
	eventID int64,
	targetTypeID int,
	now time.Time,
) ([]State.MapObservation, invasionBlockedTargets) {
	available := make([]State.MapObservation, 0, len(pool))
	blocked := invasionBlockedTargets{}
	for _, target := range pool {
		key := State.InvasionTargetKey(target.KingdomID, target.X, target.Y)
		_, busy := active[key]
		_, reserved := gameState.Invasion.TargetReservation(target.KingdomID, target.X, target.Y)
		switch {
		case target.InvasionProtected || gameState.Invasion.TargetUnavailable(target.KingdomID, target.X, target.Y):
			blocked.unavailable++
		case !target.InvasionAvailabilityKnown || target.Level <= 0:
			blocked.unconfirmed++
		case reserved:
			blocked.reserved++
		case State.AttackFeatureTargetPendingAt(
			gameState, State.AttackFeatureAutoInvasion, target.KingdomID, targetTypeID,
			target.X, target.Y, now,
		):
			blocked.settling++
		case busy:
			blocked.busy++
		default:
			available = append(available, target)
		}
	}
	return available, blocked
}

func nextUnconfirmedInvasionTarget(
	pool []State.MapObservation,
	now time.Time,
) (State.MapObservation, time.Time, bool) {
	for _, target := range pool {
		if target.InvasionAvailabilityKnown && target.Level > 0 {
			continue
		}
		if target.ObservedAt.IsZero() {
			return target, now, true
		}
		return target, target.ObservedAt.Add(invasionTargetFreshness), true
	}
	return State.MapObservation{}, time.Time{}, false
}

// invasionNeighborhoodBounds is the single-window box refreshed before a pick:
// 49x49 tiles centered on the candidate, clipped at the map origin.
func invasionNeighborhoodBounds(target State.MapObservation) State.StormMapBounds {
	return State.StormMapBounds{
		X1: max(0, target.X-invasionNeighborhoodHalfSize), Y1: max(0, target.Y-invasionNeighborhoodHalfSize),
		X2: target.X + invasionNeighborhoodHalfSize, Y2: target.Y + invasionNeighborhoodHalfSize,
	}
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
