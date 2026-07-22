package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/State"
)

const (
	autoKhanEventID              = 72
	autoKhanCampTypeID           = 2
	autoKhanCampVictoryCount     = 845
	autoKhanTargetRadius         = 6
	defaultKhanDefenseRefreshSec = 30
	defaultKhanMapRefreshSec     = 30
	defaultKhanCheckIntervalSec  = 30
)

type AutoKhanPolicy struct{}

type autoKhanSettings struct {
	Version                   int              `json:"version"`
	SourceCastleID            State.CastleID   `json:"sourceCastleId"`
	AttackPresetID            string           `json:"attackPresetId"`
	DefensePresetID           string           `json:"defensePresetId"`
	MinimumRemainingSec       int64            `json:"minimumRemainingSec"`
	CheckIntervalSec          int              `json:"checkIntervalSec"`
	DefenseRefreshIntervalSec int              `json:"defenseRefreshIntervalSec"`
	MapRefreshIntervalSec     int              `json:"mapRefreshIntervalSec"`
	DailyAttackLimit          int64            `json:"dailyAttackLimit"`
	SkipCooldowns             bool             `json:"skipCooldowns"`
	TimeSkipReserve           map[string]int64 `json:"timeSkipReserve"`
	OpenGateProtection        bool             `json:"openGateProtection"`
	OffensiveUnitThreshold    int64            `json:"offensiveUnitThreshold"`
	HorseTravelBoostID        int              `json:"horseTravelBoostId"`
	NomadPointThreshold       int64            `json:"nomadPointThreshold"`
	ReplenishDefenseTools     bool             `json:"replenishDefenseTools"`
}

func NewAutoKhanPolicy() *AutoKhanPolicy { return &AutoKhanPolicy{} }

func (*AutoKhanPolicy) ID() string { return "autoKhan" }

func (*AutoKhanPolicy) EnabledKey() string { return "auto_khan" }

func (*AutoKhanPolicy) WakeDomains() []string {
	return []string{
		"achievements", "attacks", "commanders", "currencies", "defense", "events", "event-scores", "inventory", "khan", "map",
		"movement-snapshot", "movements", "resources", "stationing", "tower-cooldowns", "units",
	}
}

func (*AutoKhanPolicy) WakeSections() []string {
	return []string{"automation.autoKhan", AttackPresets.ConfigurationSection, KhanDomain.DefensePresetsSection, commanderFeatureSection}
}

func (*AutoKhanPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := autoKhanSettings{
		MinimumRemainingSec: 300, CheckIntervalSec: defaultKhanCheckIntervalSec,
		DefenseRefreshIntervalSec: defaultKhanDefenseRefreshSec, MapRefreshIntervalSec: defaultKhanMapRefreshSec,
		SkipCooldowns: true, TimeSkipReserve: map[string]int64{}, OffensiveUnitThreshold: 1000,
		HorseTravelBoostID: -1,
	}
	if !decodeSection(snapshot.Configuration, "automation.autoKhan", &settings) {
		return autoKhanWaiting(snapshot.Now, "Auto Khan is not configured", settings.CheckIntervalSec, nil), nil
	}
	settings.AttackPresetID = strings.TrimSpace(settings.AttackPresetID)
	settings.DefensePresetID = strings.TrimSpace(settings.DefensePresetID)
	if settings.SourceCastleID <= 0 || settings.AttackPresetID == "" || settings.DefensePresetID == "" {
		return autoKhanWaiting(snapshot.Now, "Choose a Great Empire attack castle plus attack and defense presets", settings.CheckIntervalSec, nil), nil
	}
	if !settings.SkipCooldowns {
		return autoKhanWaiting(snapshot.Now, "Enable cooldown time skips to run a safe Khan chain", settings.CheckIntervalSec, nil), nil
	}
	if invalidTimeSkipReserve(settings.TimeSkipReserve) {
		return autoKhanWaiting(snapshot.Now, "Khan time-skip reserves cannot be negative", settings.CheckIntervalSec, nil), nil
	}
	if settings.OpenGateProtection && settings.OffensiveUnitThreshold <= 0 {
		return autoKhanWaiting(snapshot.Now, "Open-gate protection requires a positive offensive-unit threshold", settings.CheckIntervalSec, nil), nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return autoKhanWaiting(snapshot.Now, "Choose a supported horse travel boost", settings.CheckIntervalSec, nil), nil
	}
	if snapshot.GameData == nil {
		return autoKhanWaiting(snapshot.Now, "Official game data is unavailable", settings.CheckIntervalSec, nil), nil
	}
	source, exists := snapshot.State.Castles[settings.SourceCastleID]
	if !exists || source.KingdomID != 0 {
		return autoKhanWaiting(snapshot.Now, "Auto Khan attack source must be an available Great Empire castle", settings.CheckIntervalSec, nil), nil
	}
	main, found := autoKhanMainCastle(snapshot.State)
	if !found {
		return autoKhanWaiting(snapshot.Now, "The Great Empire main castle is unavailable", settings.CheckIntervalSec, nil), nil
	}
	if settings.OpenGateProtection && source.ID != main.ID {
		settings.OpenGateProtection = false
	}

	score, active := snapshot.State.EventScores.ByEvent[autoKhanEventID]
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		return autoKhanWaiting(snapshot.Now, "Waiting for the Nomad event and Khan camp", settings.CheckIntervalSec, nil), nil
	}
	remaining := autoKhanRemaining(score, snapshot.Now)
	metrics := autoKhanMetrics(snapshot.State, main)
	metrics["eventRemainingSec"] = float64(max(int64(0), remaining))
	metrics["nomadPoints"] = float64(score.PlayerScore)
	metrics["nomadPointThreshold"] = float64(settings.NomadPointThreshold)
	if settings.NomadPointThreshold > 0 && score.PlayerScore >= settings.NomadPointThreshold {
		outgoing := autoKhanOutgoingMovementIDs(snapshot.State, snapshot.Now)
		metrics["outgoingKhanMovements"] = float64(len(outgoing))
		gateOpen := main.Defense.OpenGateUntil != nil && main.Defense.OpenGateUntil.After(snapshot.Now)
		if gateOpen && len(outgoing) == 0 {
			metrics["gateOpenUntilUnixMs"] = float64(main.Defense.OpenGateUntil.UnixMilli())
			return Decision{
				Status: "protected", Detail: fmt.Sprintf(
					"Nomad point limit reached: %d / %d; gates are open and Khan launches are stopped",
					score.PlayerScore, settings.NomadPointThreshold,
				),
				NextCheckAt: snapshot.Now.Add(30 * time.Second), Metrics: metrics,
			}, nil
		}
		arguments, _ := json.Marshal(map[string]any{
			"castleId": main.ID, "pointThreshold": settings.NomadPointThreshold,
		})
		return Decision{
			Status: "protecting", Detail: fmt.Sprintf(
				"Nomad point limit reached: %d / %d; recall %d Khan movement(s) and open gates",
				score.PlayerScore, settings.NomadPointThreshold, len(outgoing),
			),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "khan.point_limit.protect", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if remaining >= 0 && remaining <= max(int64(0), settings.MinimumRemainingSec) {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Nomad event has %d seconds remaining; no new Khan attacks will launch", remaining),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
		}, nil
	}

	attackDocument, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return Decision{}, err
	}
	attackPreset, exists := AttackPresets.Find(attackDocument, settings.AttackPresetID)
	if !exists {
		return autoKhanWaiting(snapshot.Now, "The selected Khan attack preset no longer exists", settings.CheckIntervalSec, nil), nil
	}
	defensePreset, err := KhanDomain.DecodeDefensePreset(
		snapshot.Configuration.Sections[KhanDomain.DefensePresetsSection], settings.DefensePresetID,
	)
	if err != nil {
		return autoKhanWaiting(snapshot.Now, err.Error(), settings.CheckIntervalSec, nil), nil
	}

	if _, threatCount, earliestImpact, _ := incomingThreats(snapshot.State, snapshot.Now); threatCount > 0 {
		metrics["playerThreatCount"] = float64(threatCount)
		next := snapshot.Now.Add(2 * time.Second)
		if !earliestImpact.IsZero() && earliestImpact.Before(next) {
			next = earliestImpact
		}
		return Decision{
			Status: "yielding", Detail: "Incoming player attack detected; Auto Khan yielded to Auto Station",
			NextCheckAt: next, Metrics: metrics,
		}, nil
	}
	if autoKhanStationingActive(snapshot.State, snapshot.Now) {
		return Decision{
			Status: "yielding", Detail: "Auto Station is moving troops; Khan attacks and defense changes are paused",
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		}, nil
	}

	if protection := snapshot.State.Khan.Protection; protection.Active {
		metrics["protectionOffensiveUnits"] = float64(protection.OffensiveWallUnits)
		metrics["protectionThreshold"] = float64(protection.OffensiveUnitThreshold)
		if protection.GateOpenUntil.After(snapshot.Now) {
			metrics["gateOpenUntilUnixMs"] = float64(protection.GateOpenUntil.UnixMilli())
			return Decision{
				Status: "protected", Detail: protection.Reason,
				NextCheckAt: minTime(protection.GateOpenUntil.Add(time.Second), snapshot.Now.Add(30*time.Second)), Metrics: metrics,
			}, nil
		}
		if !main.Defense.ObservedAt.After(protection.GateOpenUntil) {
			return autoKhanDefenseRefresh(snapshot.Now, main, "Protection expired; refresh defense before checking recovery", metrics), nil
		}
		risk, riskErr := KhanDomain.OffensiveWallUnits(main, snapshot.GameData, defensePreset)
		if riskErr != nil {
			return Decision{}, riskErr
		}
		metrics["offensiveWallUnits"] = float64(risk.OffensiveUnits)
		if risk.OffensiveUnits >= protection.OffensiveUnitThreshold {
			return Decision{
				Status: "blocked", Detail: fmt.Sprintf(
					"Add defense units to continue; %d offensive units still reach the wall (threshold %d)",
					risk.OffensiveUnits, protection.OffensiveUnitThreshold,
				),
				NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
			}, nil
		}
		arguments, _ := json.Marshal(map[string]any{
			"runId": snapshot.State.Khan.RunID, "castleId": main.ID, "defensePreset": defensePreset,
			"offensiveUnitThreshold": protection.OffensiveUnitThreshold,
		})
		return Decision{
			Status: "ready", Detail: "Defense recovered; clear the expired Auto Khan protection lock",
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "khan.protection.clear", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	if main.Defense.OpenGateUntil != nil && main.Defense.OpenGateUntil.After(snapshot.Now) {
		metrics["gateOpenUntilUnixMs"] = float64(main.Defense.OpenGateUntil.UnixMilli())
		return Decision{
			Status: "protected", Detail: "Main castle gates are open; Auto Khan will not queue attacks or taunts",
			NextCheckAt: minTime(main.Defense.OpenGateUntil.Add(time.Second), snapshot.Now.Add(30*time.Second)), Metrics: metrics,
		}, nil
	}

	refreshInterval := policyInterval(settings.DefenseRefreshIntervalSec, defaultKhanDefenseRefreshSec)
	defenseStale := main.Defense.ObservedAt.IsZero() || main.Defense.InventoryObservedAt.IsZero() ||
		snapshot.Now.Sub(main.Defense.ObservedAt) >= refreshInterval ||
		!snapshot.State.Khan.LastTauntResolvedAt.IsZero() && !main.Defense.ObservedAt.After(snapshot.State.Khan.LastTauntResolvedAt)
	if defenseStale {
		return autoKhanDefenseRefresh(snapshot.Now, main, "Refresh main castle defense before continuing the Khan chain", metrics), nil
	}
	if settings.ReplenishDefenseTools {
		purchase, missing, purchaseErr := autoKhanDefenseToolPurchase(snapshot, main, defensePreset)
		if purchaseErr != nil {
			return Decision{}, purchaseErr
		}
		if missing > 0 {
			metrics["missingDefenseTools"] = float64(missing)
			if purchase == nil {
				return Decision{
					Status: "waiting", Detail: fmt.Sprintf(
						"Defense preset is short %d tool(s); no active affordable non-ruby package is available",
						missing,
					),
					NextCheckAt: snapshot.Now.Add(30 * time.Second), Metrics: metrics,
				}, nil
			}
			nextPurchaseAt := snapshot.State.Khan.LastDefenseToolPurchaseAt.Add(30 * time.Second)
			if nextPurchaseAt.After(snapshot.Now) {
				return Decision{
					Status: "replenishing", Detail: fmt.Sprintf(
						"Defense preset is short %d tool(s); next non-ruby shop check is at %s",
						missing, nextPurchaseAt.Local().Format("15:04:05"),
					),
					NextCheckAt: nextPurchaseAt, Metrics: metrics,
				}, nil
			}
			metrics["defenseToolId"] = float64(purchase.Package.ToolID)
			metrics["defenseToolPackageId"] = float64(purchase.Package.PackageID)
			metrics["defenseToolPurchaseCount"] = float64(purchase.Amount)
			arguments, _ := json.Marshal(map[string]any{
				"castleId": main.ID, "packageId": purchase.Package.PackageID,
				"toolId": purchase.Package.ToolID, "amount": purchase.Amount,
				"shopTableId": purchase.Route.EventID, "defensePreset": defensePreset,
			})
			return Decision{
				Status: "replenishing", Detail: fmt.Sprintf(
					"Buy %d × %s package for %d %s to replenish tool %d",
					purchase.Amount, purchase.Package.Name, purchase.Amount*purchase.Package.Price,
					purchase.Package.PriceName, purchase.Package.ToolID,
				),
				NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
				Request: &Intent.Request{Name: "khan.defense_tools.replenish", Arguments: arguments}, ReevaluateOnSuccess: true,
			}, nil
		}
	}
	if settings.OpenGateProtection {
		risk, riskErr := KhanDomain.OffensiveWallUnits(main, snapshot.GameData, defensePreset)
		if riskErr != nil {
			return Decision{}, riskErr
		}
		metrics["offensiveWallUnits"] = float64(risk.OffensiveUnits)
		metrics["offensiveUnitThreshold"] = float64(settings.OffensiveUnitThreshold)
		if risk.OffensiveUnits >= settings.OffensiveUnitThreshold {
			arguments, _ := json.Marshal(map[string]any{
				"runId": snapshot.State.Khan.RunID, "castleId": main.ID, "defensePreset": defensePreset,
				"offensiveUnitThreshold": settings.OffensiveUnitThreshold,
			})
			return Decision{
				Status: "protecting", Detail: fmt.Sprintf(
					"Open gates once and stop the Khan chain: %d offensive wall units reached threshold %d",
					risk.OffensiveUnits, settings.OffensiveUnitThreshold,
				),
				NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
				Request: &Intent.Request{Name: "khan.open_gate", Arguments: arguments}, ReevaluateOnSuccess: true,
			}, nil
		}
	}
	if !KhanDomain.Matches(main, defensePreset) {
		arguments, _ := json.Marshal(map[string]any{
			"castleId": main.ID, "presetId": defensePreset.ID, "presetName": defensePreset.Name,
			"wall": defensePreset.Wall, "moat": defensePreset.Moat, "keep": defensePreset.Keep,
		})
		return Decision{
			Status: "defending", Detail: fmt.Sprintf("Apply defense preset %s to the main castle", defensePreset.Name),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "defense.preset.apply", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}

	target, found := autoKhanTarget(snapshot.State, main)
	if !found {
		return autoKhanMapRefresh(snapshot.Now, main, "Discover the Khan camp beside the main castle", metrics), nil
	}
	metrics["targetX"] = float64(target.X)
	metrics["targetY"] = float64(target.Y)
	activeRun := snapshot.State.Khan.RunID != "" && snapshot.State.Khan.SourceCastleID == source.ID &&
		snapshot.State.Khan.MainCastleID == main.ID && snapshot.State.Khan.TargetX == target.X && snapshot.State.Khan.TargetY == target.Y
	if activeRun && snapshot.State.Khan.SafetyError != "" {
		return Decision{
			Status: "blocked", Detail: "Khan chain stopped on unsafe arrival order: " + snapshot.State.Khan.SafetyError,
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
		}, nil
	}
	mapInterval := policyInterval(settings.MapRefreshIntervalSec, defaultKhanMapRefreshSec)
	if target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) >= mapInterval {
		return autoKhanMapRefresh(snapshot.Now, main, "Refresh the Khan camp before continuing", metrics), nil
	}
	key := towerTargetKey(target.KingdomID, target.X, target.Y)
	if cooldown, exists := snapshot.State.TowerCooldowns[key]; exists && cooldown.PendingCooldownRefresh {
		return autoKhanTileRefresh(snapshot.Now, target, "Refresh the Khan camp immediately after the confirmed hit", metrics), nil
	}
	remainingCooldown := towerCooldownRemaining(target, snapshot.Now)
	metrics["cooldownRemaining"] = float64(remainingCooldown)
	if _, blocked := dailyAttackLimitAllowance(
		snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec), metrics,
	); blocked != nil {
		return *blocked, nil
	}
	if remainingCooldown > 0 {
		if oneCommandDungeonSkipCount(snapshot.State, settings.TimeSkipReserve, 3*60*60) < 1 {
			return autoKhanWaiting(snapshot.Now, "No five-hour or one-day time skip is available above reserves", settings.CheckIntervalSec, metrics), nil
		}
		return Decision{
			Status: "cooldown", Detail: fmt.Sprintf("Skip the Khan camp cooldown before the next chain hit (%d seconds)", remainingCooldown),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
			Request: &Intent.Request{
				Name: "nomad.cooldown.minute_skip", Arguments: nomadMinuteSkipArguments(target, settings.TimeSkipReserve),
			},
			ReevaluateOnSuccess: true,
		}, nil
	}

	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoKhan")
	available := availableNomadCommanders(snapshot.State, commanderIDs, restricted)
	if restricted && len(commanderIDs) == 0 {
		return autoKhanWaiting(snapshot.Now, "Assign at least one commander to Auto Khan", settings.CheckIntervalSec, metrics), nil
	}
	if len(available) == 0 {
		detail := "No commander is currently available"
		if restricted {
			detail = "No assigned Auto Khan commander is currently available"
		}
		return autoKhanWaitingUntilTaunt(snapshot, detail, settings.CheckIntervalSec, metrics), nil
	}
	outstandingSkips := int64(0)
	if activeRun {
		outstandingSkips = max(int64(0), int64(snapshot.State.Khan.AttacksLaunched-snapshot.State.Khan.CooldownsSkipped))
	}
	availableSkips := oneCommandDungeonSkipCount(snapshot.State, settings.TimeSkipReserve, 3*60*60)
	usableSkips := max(int64(0), availableSkips-outstandingSkips)
	metrics["availableCooldownSkips"] = float64(availableSkips)
	metrics["committedCooldownSkips"] = float64(outstandingSkips)
	metrics["usableCooldownSkips"] = float64(usableSkips)
	if usableSkips < 1 {
		return autoKhanWaitingUntilTaunt(snapshot, "All available Khan cooldown skips are committed to launched hits", settings.CheckIntervalSec, metrics), nil
	}
	presetCopies := availablePresetCopies(attackPreset, source, len(available))
	metrics["availableCommanders"] = float64(len(available))
	metrics["presetCopies"] = float64(presetCopies)
	if presetCopies < 1 {
		return nomadPresetWaiting(snapshot.Now, attackPreset, source, metrics), nil
	}
	ordered := autoKhanOrderCommanders(snapshot, source, target, available)
	commanderID := ordered[0]
	eventEndsAt := autoKhanEventEndsAt(score)
	runID := autoKhanRunID(snapshot.State.Khan, source.ID, main.ID, target, eventEndsAt)
	arguments, _ := json.Marshal(map[string]any{
		"runId": runID, "eventEndsAt": eventEndsAt, "sourceCastleId": source.ID, "mainCastleId": main.ID,
		"kingdomId": target.KingdomID, "targetX": target.X, "targetY": target.Y,
		"victoryCount": target.TowerVictoryCount, "preset": attackPreset, "commanderId": commanderID,
		"defensePreset": defensePreset, "openGateProtection": settings.OpenGateProtection,
		"offensiveUnitThreshold": settings.OffensiveUnitThreshold,
		"horseTravelBoostId":     settings.HorseTravelBoostID,
		"dailyAttackLimit":       settings.DailyAttackLimit,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch the next Khan chain attack with commander %d from %s", commanderID, castleName(source)),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "khan.attack", Arguments: arguments}, ReevaluateOnSuccess: true,
	}, nil
}

func autoKhanMainCastle(gameState State.GameState) (State.CastleState, bool) {
	for _, castle := range gameState.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

func autoKhanTarget(gameState State.GameState, main State.CastleState) (State.MapObservation, bool) {
	if gameState.Khan.RunID != "" && gameState.Khan.MainCastleID == main.ID {
		if target, found := gameState.Map[0][fmt.Sprintf("%d:%d", gameState.Khan.TargetX, gameState.Khan.TargetY)]; found && autoKhanTargetCandidate(main, target) {
			return target, true
		}
	}
	candidates := make([]State.MapObservation, 0)
	for _, target := range gameState.Map[0] {
		if autoKhanTargetCandidate(main, target) {
			candidates = append(candidates, target)
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftDistance := autoKhanDistance(main, candidates[left])
		rightDistance := autoKhanDistance(main, candidates[right])
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if candidates[left].Y != candidates[right].Y {
			return candidates[left].Y < candidates[right].Y
		}
		return candidates[left].X < candidates[right].X
	})
	if len(candidates) == 0 {
		return State.MapObservation{}, false
	}
	return candidates[0], true
}

func autoKhanTargetCandidate(main State.CastleState, target State.MapObservation) bool {
	return target.KingdomID == 0 && target.TypeID == autoKhanCampTypeID &&
		target.TowerVictoryCount == autoKhanCampVictoryCount && (target.X != main.X || target.Y != main.Y) &&
		absoluteInt(target.X-main.X) <= autoKhanTargetRadius && absoluteInt(target.Y-main.Y) <= autoKhanTargetRadius
}

func autoKhanDistance(main State.CastleState, target State.MapObservation) int {
	return absoluteInt(target.X-main.X) + absoluteInt(target.Y-main.Y)
}

func autoKhanOrderCommanders(
	snapshot Snapshot,
	source State.CastleState,
	target State.MapObservation,
	commanderIDs []State.CommanderID,
) []State.CommanderID {
	type rankedCommander struct {
		id    State.CommanderID
		speed float64
	}
	ranked := make([]rankedCommander, 0, len(commanderIDs))
	for _, commanderID := range commanderIDs {
		result, err := (AttackCapacity.Resolver{}).ResolveTravelSpeed(snapshot.State, snapshot.GameData, AttackCapacity.Request{
			SourceCastleID: source.ID, CommanderID: commanderID,
			Target: AttackCapacity.TargetContext{
				Map: &AttackCapacity.MapTarget{
					KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
					ObjectID: target.ObjectID, Level: target.Level, VictoryCount: target.TowerVictoryCount,
				},
				Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
			},
		})
		speed := float64(0)
		if err == nil {
			speed = result.AppliedPercent
		}
		ranked = append(ranked, rankedCommander{id: commanderID, speed: speed})
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].speed != ranked[right].speed {
			return ranked[left].speed > ranked[right].speed
		}
		return ranked[left].id < ranked[right].id
	})
	result := make([]State.CommanderID, len(ranked))
	for index := range ranked {
		result[index] = ranked[index].id
	}
	return result
}

func autoKhanMapRefresh(now time.Time, main State.CastleState, detail string, metrics map[string]float64) Decision {
	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": 0, "x1": main.X - autoKhanTargetRadius, "y1": main.Y - autoKhanTargetRadius,
		"x2": main.X + autoKhanTargetRadius, "y2": main.Y + autoKhanTargetRadius,
	})
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

func autoKhanTileRefresh(now time.Time, target State.MapObservation, detail string, metrics map[string]float64) Decision {
	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": target.KingdomID, "x1": target.X, "y1": target.Y, "x2": target.X, "y2": target.Y,
	})
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

type autoKhanDefenseToolPurchaseCandidate struct {
	Package GameData.DefenseToolShopPackage
	Route   State.EventShopRoute
	Amount  int64
	Deficit int64
}

func autoKhanDefenseToolPurchase(
	snapshot Snapshot,
	main State.CastleState,
	preset KhanDomain.DefensePreset,
) (*autoKhanDefenseToolPurchaseCandidate, int64, error) {
	deficits := KhanDomain.DefenseToolDeficits(main, preset)
	var missing int64
	for _, deficit := range deficits {
		missing += deficit
	}
	if missing == 0 {
		return nil, 0, nil
	}

	candidates := make([]autoKhanDefenseToolPurchaseCandidate, 0)
	for toolID, deficit := range deficits {
		packages, err := snapshot.GameData.DefenseToolShopPackages(int64(toolID))
		if err != nil {
			return nil, missing, err
		}
		for _, item := range packages {
			route, active := autoKhanDefenseToolShopRoute(snapshot.State, item, snapshot.Now)
			if !active {
				continue
			}
			balance, available := autoKhanDefenseToolBalance(snapshot.State, main, item)
			if !available || item.Price <= 0 || balance < item.Price {
				continue
			}
			amount := (deficit + item.ToolAmount - 1) / item.ToolAmount
			amount = min(amount, balance/item.Price)
			if item.MaxBuyPerClick > 0 {
				amount = min(amount, item.MaxBuyPerClick)
			}
			if item.Stock > 0 {
				remaining := max(int64(0), item.Stock-snapshot.State.Inventory.ConstructionOffers[State.PackageID(item.PackageID)])
				amount = min(amount, remaining)
			}
			if amount <= 0 {
				continue
			}
			candidates = append(candidates, autoKhanDefenseToolPurchaseCandidate{
				Package: item, Route: route, Amount: amount, Deficit: deficit,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, missing, nil
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftPriority := autoKhanDefenseToolPricePriority(candidates[left].Package)
		rightPriority := autoKhanDefenseToolPricePriority(candidates[right].Package)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftUnitPrice := float64(candidates[left].Package.Price) / float64(candidates[left].Package.ToolAmount)
		rightUnitPrice := float64(candidates[right].Package.Price) / float64(candidates[right].Package.ToolAmount)
		if leftUnitPrice != rightUnitPrice {
			return leftUnitPrice < rightUnitPrice
		}
		if candidates[left].Deficit != candidates[right].Deficit {
			return candidates[left].Deficit > candidates[right].Deficit
		}
		return candidates[left].Package.PackageID < candidates[right].Package.PackageID
	})
	return &candidates[0], missing, nil
}

func autoKhanDefenseToolShopRoute(
	gameState State.GameState,
	item GameData.DefenseToolShopPackage,
	now time.Time,
) (State.EventShopRoute, bool) {
	if route, active := gameState.ActiveShopForPackage(State.PackageID(item.PackageID), now); active {
		return route, true
	}
	if item.PriceScope == GameData.DefenseToolPriceCastleResource && item.PriceID == GameData.StormAquamarineID &&
		gameState.Storm.LunaShopTableID > 0 {
		return State.EventShopRoute{EventID: gameState.Storm.LunaShopTableID}, true
	}
	return State.EventShopRoute{}, false
}

func autoKhanDefenseToolBalance(
	gameState State.GameState,
	main State.CastleState,
	item GameData.DefenseToolShopPackage,
) (int64, bool) {
	switch item.PriceScope {
	case GameData.DefenseToolPricePlayerResource:
		return int64(math.Floor(gameState.Player.Resources[State.ResourceID(item.PriceID)])), true
	case GameData.DefenseToolPriceCurrency:
		return int64(math.Floor(gameState.Player.Currencies[State.CurrencyID(item.PriceID)])), true
	case GameData.DefenseToolPriceCastleResource:
		if item.PriceID != GameData.StormAquamarineID {
			return int64(math.Floor(main.Resources[State.ResourceID(item.PriceID)].Amount)), true
		}
		for _, castle := range gameState.Castles {
			if castle.KingdomID == GameData.StormKingdomID {
				return int64(math.Floor(castle.Resources[State.ResourceID(item.PriceID)].Amount)), true
			}
		}
	}
	return 0, false
}

func autoKhanDefenseToolPricePriority(item GameData.DefenseToolShopPackage) int {
	switch item.PriceScope {
	case GameData.DefenseToolPricePlayerResource:
		return 0
	case GameData.DefenseToolPriceCastleResource:
		if item.PriceID == GameData.StormAquamarineID {
			return 7
		}
		return 1
	case GameData.DefenseToolPriceCurrency:
		switch item.PriceID {
		case 1:
			return 2
		case 10:
			return 3
		case 36:
			return 4
		case 35:
			return 5
		default:
			return 6
		}
	}
	return 8
}

func autoKhanDefenseRefresh(now time.Time, main State.CastleState, detail string, metrics map[string]float64) Decision {
	arguments, _ := json.Marshal(map[string]any{"castleId": main.ID})
	return Decision{
		Status: "defending", Detail: detail, NextCheckAt: now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "defense.refresh", Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

func autoKhanWaiting(now time.Time, detail string, intervalSec int, metrics map[string]float64) Decision {
	return Decision{
		Status: "waiting", Detail: detail,
		NextCheckAt: now.Add(policyInterval(intervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
	}
}

func autoKhanWaitingUntilTaunt(snapshot Snapshot, detail string, intervalSec int, metrics map[string]float64) Decision {
	decision := autoKhanWaiting(snapshot.Now, detail, intervalSec, metrics)
	for _, taunt := range snapshot.State.Khan.Taunts {
		candidate := taunt.ImpactAt.Add(2 * time.Second)
		if candidate.After(snapshot.Now) && candidate.Before(decision.NextCheckAt) {
			decision.NextCheckAt = candidate
		}
	}
	return decision
}

func autoKhanMetrics(gameState State.GameState, main State.CastleState) map[string]float64 {
	metrics := map[string]float64{
		"mainCastleId": float64(main.ID), "attacksLaunched": float64(gameState.Khan.AttacksLaunched),
		"victoriesConfirmed": float64(gameState.Khan.VictoriesConfirmed),
		"cooldownsSkipped":   float64(gameState.Khan.CooldownsSkipped),
		"tauntsObserved":     float64(gameState.Khan.TauntsObserved), "tauntsResolved": float64(gameState.Khan.TauntsResolved),
		"activeTaunts": float64(len(gameState.Khan.Taunts)),
	}
	return metrics
}

func autoKhanOutgoingMovementIDs(gameState State.GameState, now time.Time) []State.MovementID {
	seen := map[State.MovementID]struct{}{}
	result := make([]State.MovementID, 0, len(gameState.Khan.Launches))
	for _, launch := range gameState.Khan.Launches {
		movement, found := gameState.Movements[launch.MovementID]
		if !found || movement.Direction != 0 || !towerMovementActiveAt(movement, now) {
			continue
		}
		if movement.OwnerPlayerID != 0 && movement.OwnerPlayerID != gameState.Player.ID {
			continue
		}
		if _, duplicate := seen[movement.ID]; duplicate {
			continue
		}
		seen[movement.ID] = struct{}{}
		result = append(result, movement.ID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func autoKhanStationingActive(gameState State.GameState, now time.Time) bool {
	for _, operation := range gameState.Stationing {
		if operation.Purpose != "autoStation" {
			continue
		}
		if operation.SafeAfter != nil && now.Before(operation.SafeAfter.Add(5*time.Second)) {
			return true
		}
		if movement, found := trackedStationMovement(gameState, operation); found && towerMovementActiveAt(movement, now) {
			return true
		}
	}
	return false
}

func autoKhanEventEndsAt(score State.ScalableEventScore) time.Time {
	if score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		return time.Time{}
	}
	return score.ObservedAt.Add(time.Duration(score.RemainingSec) * time.Second).UTC()
}

func autoKhanRemaining(score State.ScalableEventScore, now time.Time) int64 {
	if score.RemainingSec <= 0 {
		return -1
	}
	elapsed := int64(0)
	if !score.ObservedAt.IsZero() && now.After(score.ObservedAt) {
		elapsed = int64(now.Sub(score.ObservedAt) / time.Second)
	}
	return max(int64(0), score.RemainingSec-elapsed)
}

func autoKhanRunID(
	current State.KhanState,
	sourceCastleID State.CastleID,
	mainCastleID State.CastleID,
	target State.MapObservation,
	eventEndsAt time.Time,
) string {
	if current.RunID != "" && current.SourceCastleID == sourceCastleID && current.MainCastleID == mainCastleID &&
		current.TargetX == target.X && current.TargetY == target.Y &&
		(current.EventEndsAt.IsZero() || eventEndsAt.IsZero() || absoluteDuration(current.EventEndsAt.Sub(eventEndsAt)) < 2*time.Minute) {
		return current.RunID
	}
	eventKey := eventEndsAt.Unix() / 60
	return fmt.Sprintf("khan-%d-%d-%d-%d-%d", eventKey, sourceCastleID, mainCastleID, target.X, target.Y)
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func minTime(left, right time.Time) time.Time {
	if left.IsZero() || right.Before(left) {
		return right
	}
	return left
}
