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
	autoKhanCampTypeID           = 35
	defaultKhanDefenseRefreshSec = 30
	defaultKhanMapRefreshSec     = 30
	defaultKhanCheckIntervalSec  = 30
)

type AutoKhanPolicy struct{}
type AutoKhanCooldownPolicy struct{}
type AutoKhanRagePolicy struct{}
type AutoKhanDefensePolicy struct{}

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
	AttackLaunchesEnabled     bool             `json:"attackLaunchesEnabled"`
	TriggerRage               bool             `json:"triggerRage"`
	SkipCooldowns             bool             `json:"skipCooldowns"`
	TimeSkipReserve           map[string]int64 `json:"timeSkipReserve"`
	OpenGateProtection        bool             `json:"openGateProtection"`
	OffensiveUnitThreshold    int64            `json:"offensiveUnitThreshold"`
	HorseTravelBoostID        int              `json:"horseTravelBoostId"`
	NomadPointThreshold       int64            `json:"nomadPointThreshold"`
	ReplenishDefenseTools     bool             `json:"replenishDefenseTools"`
}

func NewAutoKhanPolicy() *AutoKhanPolicy                 { return &AutoKhanPolicy{} }
func NewAutoKhanCooldownPolicy() *AutoKhanCooldownPolicy { return &AutoKhanCooldownPolicy{} }
func NewAutoKhanRagePolicy() *AutoKhanRagePolicy         { return &AutoKhanRagePolicy{} }
func NewAutoKhanDefensePolicy() *AutoKhanDefensePolicy   { return &AutoKhanDefensePolicy{} }

func (*AutoKhanPolicy) ID() string         { return "autoKhan" }
func (*AutoKhanCooldownPolicy) ID() string { return "autoKhan:cooldown" }
func (*AutoKhanRagePolicy) ID() string     { return "autoKhan:rage" }
func (*AutoKhanDefensePolicy) ID() string  { return "autoKhan:defense" }

func (*AutoKhanPolicy) EnabledKey() string         { return "auto_khan" }
func (*AutoKhanCooldownPolicy) EnabledKey() string { return "auto_khan" }
func (*AutoKhanRagePolicy) EnabledKey() string     { return "auto_khan" }
func (*AutoKhanDefensePolicy) EnabledKey() string  { return "auto_khan" }

func (*AutoKhanCooldownPolicy) ScheduleKey() string { return "autoKhan" }
func (*AutoKhanRagePolicy) ScheduleKey() string     { return "autoKhan" }
func (*AutoKhanDefensePolicy) ScheduleKey() string  { return "autoKhan" }

// Every lane is one feature on the wire. Without this the coordinator would
// attribute "autoKhan:rage" and the other lanes to an unknown actor, and their
// operations would fall back to background priority behind the attack chain.
func (*AutoKhanCooldownPolicy) ActorID() string { return "autoKhan" }
func (*AutoKhanRagePolicy) ActorID() string     { return "autoKhan" }
func (*AutoKhanDefensePolicy) ActorID() string  { return "autoKhan" }

func (*AutoKhanPolicy) WakeDomains() []string {
	return []string{
		"achievements", "attacks", "commanders", "currencies", "defense", "events", "event-scores", "inventory", "khan", "map-event-camp",
		"movement-snapshot", "movements", "nomad-camps", "resources", "stationing", "units",
	}
}

func (*AutoKhanCooldownPolicy) WakeDomains() []string {
	return []string{
		"currencies", "defense", "events", "event-scores", "khan", "map-event-camp", "movements", "nomad-camps", "stationing",
	}
}

func (*AutoKhanRagePolicy) WakeDomains() []string {
	return []string{
		"defense", "events", "event-scores", "inventory", "khan", "map-event-camp", "movement-snapshot", "movements", "stationing",
	}
}

func (*AutoKhanDefensePolicy) WakeDomains() []string {
	return []string{
		"defense", "events", "event-scores", "inventory", "khan", "map-event-camp", "movements", "stationing",
	}
}

// The rage bar fills mid-chain and the taunt has to go out on the frame that
// reports it full, so khan updates skip the shared state-change coalescing.
func (*AutoKhanRagePolicy) UrgentWakeDomains() []string {
	return []string{"khan"}
}

func (*AutoKhanPolicy) WakeSections() []string {
	return []string{"automation.autoKhan", AttackPresets.ConfigurationSection, KhanDomain.DefensePresetsSection, commanderFeatureSection}
}

func (*AutoKhanCooldownPolicy) WakeSections() []string {
	return []string{"automation.autoKhan", KhanDomain.DefensePresetsSection}
}

func (*AutoKhanRagePolicy) WakeSections() []string {
	return []string{"automation.autoKhan", KhanDomain.DefensePresetsSection}
}

func (*AutoKhanDefensePolicy) WakeSections() []string {
	return []string{"automation.autoKhan", KhanDomain.DefensePresetsSection}
}

func (*AutoKhanPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {

	settings := autoKhanSettings{
		MinimumRemainingSec: 300, CheckIntervalSec: defaultKhanCheckIntervalSec,
		DefenseRefreshIntervalSec: defaultKhanDefenseRefreshSec, MapRefreshIntervalSec: defaultKhanMapRefreshSec,
		AttackLaunchesEnabled: true, TriggerRage: true, SkipCooldowns: true,
		TimeSkipReserve: map[string]int64{}, OffensiveUnitThreshold: 1000,
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

	score, active := snapshot.State.LookupScalableEventScore(autoKhanEventID)
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{autoKhanEventID}, "Nomad Khan event",
		); locked {
			return decision, nil
		}
		return autoKhanWaiting(snapshot.Now, "Waiting for the Nomad event and Khan camp", settings.CheckIntervalSec, nil), nil
	}
	remaining := autoKhanRemaining(score, snapshot.Now)
	metrics := autoKhanMetrics(snapshot.State, main)
	metrics["eventRemainingSec"] = float64(max(int64(0), remaining))
	metrics["nomadPoints"] = float64(score.PlayerScore)
	metrics["nomadPointThreshold"] = float64(settings.NomadPointThreshold)
	if settings.AttackLaunchesEnabled {
		metrics["attackLaunchesEnabled"] = 1
	} else {
		metrics["attackLaunchesEnabled"] = 0
	}
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
	tauntDefenseBoundary := autoKhanDefenseBoundary(snapshot.State.Khan)
	if !tauntDefenseBoundary.IsZero() && !main.Defense.ObservedAt.After(tauntDefenseBoundary) {
		return Decision{
			Status: "defending", Detail: "Waiting for the Auto Khan defense lane to reapply the main-castle defense preset",
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
		}, nil
	}
	defenseStale := main.Defense.ObservedAt.IsZero() || main.Defense.InventoryObservedAt.IsZero() ||
		snapshot.Now.Sub(main.Defense.ObservedAt) >= refreshInterval
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

	target, found := autoKhanTarget(snapshot.State)
	if !found {
		return autoKhanMapJump(snapshot.Now, "Jump directly to the active Khan camp", metrics), nil
	}
	metrics["targetX"] = float64(target.X)
	metrics["targetY"] = float64(target.Y)
	metrics["targetEventCampId"] = float64(target.EventCampID)
	metrics["targetLevel"] = float64(target.Level)
	activeRun := snapshot.State.Khan.RunID != "" && snapshot.State.Khan.SourceCastleID == source.ID &&
		snapshot.State.Khan.MainCastleID == main.ID && snapshot.State.Khan.TargetX == target.X && snapshot.State.Khan.TargetY == target.Y
	if activeRun && snapshot.State.Khan.SafetyError != "" {
		return Decision{
			Status: "blocked", Detail: "Khan chain stopped on unsafe arrival order: " + snapshot.State.Khan.SafetyError,
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
		}, nil
	}
	metrics["playerRage"] = float64(snapshot.State.Khan.PlayerRage)
	metrics["playerRageCap"] = float64(snapshot.State.Khan.PlayerRageCap)
	metrics["playerTotalRage"] = float64(snapshot.State.Khan.PlayerTotalRage)
	metrics["tauntsTriggered"] = float64(snapshot.State.Khan.TauntsTriggered)
	mapInterval := policyInterval(settings.MapRefreshIntervalSec, defaultKhanMapRefreshSec)
	if target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) >= mapInterval {
		return autoKhanTileRefresh(snapshot.Now, target, "Refresh the located Khan camp", metrics), nil
	}
	key := towerTargetKey(target.KingdomID, target.X, target.Y)
	pendingCooldownReports := pendingKhanCooldownReports(snapshot.State)
	metrics["pendingCooldownReports"] = float64(len(pendingCooldownReports))
	if cooldown, exists := snapshot.State.NomadCamps.Cooldowns[key]; exists && cooldown.PendingCooldownRefresh {
		if len(pendingCooldownReports) == 0 {
			return autoKhanTileRefresh(
				snapshot.Now, target,
				"Refresh the Khan cooldown before the CRA launch cursor selects a fallback skip",
				metrics,
			), nil
		}
		detail := "Report-linked cooldown lane is re-pinging the Khan camp; only the CRA launch cursor is held"
		if !settings.AttackLaunchesEnabled {
			detail = "Report-linked cooldown lane is re-pinging the Khan camp while automatic Khan attacks remain locked"
		}
		return Decision{
			Status: "cooldown", Detail: detail,
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
		}, nil
	}
	remainingCooldown := nomadCampCooldownRemaining(snapshot.State, target, snapshot.Now)
	metrics["cooldownRemaining"] = float64(remainingCooldown)
	if settings.AttackLaunchesEnabled {
		if _, blocked := dailyAttackLimitAllowance(
			snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec), metrics,
		); blocked != nil {
			return *blocked, nil
		}
	}
	if remainingCooldown > 0 {
		if len(pendingCooldownReports) == 0 {
			if responseGatedDungeonCooldownCount(snapshot.State, settings.TimeSkipReserve, int64(remainingCooldown)) < 1 {
				return Decision{
					Status: "waiting", Detail: fmt.Sprintf(
						"Khan CRA launch cursor paused: no cooldown skip is available for %d remaining seconds",
						remainingCooldown,
					),
					NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)),
					Metrics:     metrics,
				}, nil
			}
			arguments, _ := json.Marshal(map[string]any{
				"kingdomId": target.KingdomID, "targetTypeId": autoKhanCampTypeID,
				"targetX": target.X, "targetY": target.Y, "eventCampId": target.EventCampID,
				"minimumRemaining": settings.TimeSkipReserve,
				"khanGuard": map[string]any{
					"mainCastleId": main.ID, "defensePreset": defensePreset,
					"openGateProtection":     settings.OpenGateProtection,
					"offensiveUnitThreshold": settings.OffensiveUnitThreshold,
					"nomadPointThreshold":    settings.NomadPointThreshold,
				},
			})
			detail := fmt.Sprintf(
				"Clear the unreported %d-second Khan cooldown immediately before the next CRA",
				remainingCooldown,
			)
			if !settings.AttackLaunchesEnabled {
				detail = fmt.Sprintf(
					"Clear the unreported %d-second Khan cooldown to maintain the manual chain",
					remainingCooldown,
				)
			}
			return Decision{
				Status: "cooldown", Detail: detail,
				NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
				Request:             &Intent.Request{Name: "nomad.cooldown.minute_skip", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, nil
		}
		detail := fmt.Sprintf(
			"Report-linked cooldown lane is clearing %d seconds; only the CRA launch cursor is held",
			remainingCooldown,
		)
		if !settings.AttackLaunchesEnabled {
			detail = fmt.Sprintf(
				"Report-linked cooldown lane is clearing %d seconds while automatic Khan attacks remain locked",
				remainingCooldown,
			)
		}
		return Decision{
			Status: "cooldown", Detail: detail,
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: metrics,
		}, nil
	}
	if !settings.AttackLaunchesEnabled {
		metrics["attackLaunchLocked"] = 1
		return Decision{
			Status:      "idle",
			Detail:      "Automatic Khan attacks are locked; cooldown, rage, and defense lanes remain active for a manual chain",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, defaultKhanCheckIntervalSec)),
			Metrics:     metrics,
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
		inFlight := max(0, snapshot.State.Khan.AttacksLaunched-snapshot.State.Khan.VictoriesConfirmed)
		outstandingSkips = int64(inFlight + len(pendingKhanCooldownReports(snapshot.State)))
	}
	availableSkips := responseGatedDungeonCooldownCount(snapshot.State, settings.TimeSkipReserve, 3*60*60)
	usableSkips := max(int64(0), availableSkips-outstandingSkips)
	metrics["availableCooldownSkips"] = float64(availableSkips)
	metrics["committedCooldownSkips"] = float64(outstandingSkips)
	metrics["usableCooldownSkips"] = float64(usableSkips)
	if usableSkips < 1 {
		return autoKhanWaitingUntilTaunt(snapshot, "All available Khan cooldown skips are committed to launched hits", settings.CheckIntervalSec, metrics), nil
	}
	metrics["availableCommanders"] = float64(len(available))
	ordered := autoKhanOrderCommanders(snapshot, source, target, available)
	commanderID := ordered[0]
	limitedPreset, err := autoKhanCapacityLimitedPreset(snapshot, source, target, attackPreset, commanderID)
	if err != nil {
		return autoKhanWaiting(
			snapshot.Now,
			fmt.Sprintf("Cannot calculate %s inventory requirements: %v", attackPreset.Name, err),
			settings.CheckIntervalSec,
			metrics,
		), nil
	}
	presetCopies := availablePresetCopies(limitedPreset, source, len(available))
	metrics["presetCopies"] = float64(presetCopies)
	if presetCopies < 1 {
		return autoKhanPresetWaiting(snapshot.Now, limitedPreset, source, settings.CheckIntervalSec, metrics), nil
	}
	eventEndsAt := autoKhanEventEndsAt(score)
	runID := autoKhanRunID(snapshot.State.Khan, source.ID, main.ID, target, eventEndsAt)
	arguments, _ := json.Marshal(map[string]any{
		"runId": runID, "eventEndsAt": eventEndsAt, "sourceCastleId": source.ID, "mainCastleId": main.ID,
		"kingdomId": target.KingdomID, "targetX": target.X, "targetY": target.Y,
		"preset": attackPreset, "commanderId": commanderID,
		"defensePreset": defensePreset, "openGateProtection": settings.OpenGateProtection,
		"offensiveUnitThreshold": settings.OffensiveUnitThreshold,
		"horseTravelBoostId":     settings.HorseTravelBoostID,
		"dailyAttackLimit":       settings.DailyAttackLimit,
		"nomadPointThreshold":    settings.NomadPointThreshold,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch the next Khan chain attack with commander %d from %s", commanderID, castleName(source)),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request:             &Intent.Request{Name: "khan.attack", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func autoKhanPresetWaiting(
	now time.Time,
	preset AttackPresets.Preset,
	source State.CastleState,
	checkIntervalSec int,
	metrics map[string]float64,
) Decision {
	metrics["attackLaunchPaused"] = 1
	if itemID, required, available, found := invasionPresetShortage(preset, source); found {
		metrics["attackPresetShortageItemId"] = float64(itemID)
		metrics["attackPresetShortageRequired"] = float64(required)
		metrics["attackPresetShortageAvailable"] = float64(available)
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf(
				"Khan CRA launch cursor paused: preset needs %d of item %d; source castle has %d",
				required, itemID, available,
			),
			NextCheckAt: now.Add(policyInterval(checkIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
		}
	}
	return Decision{
		Status: "waiting", Detail: "Khan CRA launch cursor paused: the selected preset has no launchable troops",
		NextCheckAt: now.Add(policyInterval(checkIntervalSec, defaultKhanCheckIntervalSec)), Metrics: metrics,
	}
}

type autoKhanLaneContext struct {
	Settings      autoKhanSettings
	Source        State.CastleState
	Main          State.CastleState
	Score         State.ScalableEventScore
	DefensePreset KhanDomain.DefensePreset
	Guard         map[string]any
	Metrics       map[string]float64
}

func (*AutoKhanCooldownPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	lane, blocked, err := autoKhanAsyncLaneContext(snapshot)
	if err != nil || blocked != nil {
		if blocked != nil {
			return *blocked, nil
		}
		return Decision{}, err
	}
	if !lane.Settings.SkipCooldowns {
		return autoKhanWaiting(snapshot.Now, "Khan cooldown time skips are disabled", lane.Settings.CheckIntervalSec, lane.Metrics), nil
	}
	reports := pendingKhanCooldownReports(snapshot.State)
	lane.Metrics["pendingCooldownReports"] = float64(len(reports))
	if len(reports) == 0 {
		return Decision{
			Status: "idle", Detail: "Waiting for a confirmed Khan battle report",
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
		}, nil
	}
	selected := reports[0]
	if selected.CooldownObservedAt.IsZero() || selected.CooldownObservedAt.Before(selected.LandedAt) {
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": selected.KingdomID,
			"x1":        selected.X, "y1": selected.Y, "x2": selected.X, "y2": selected.Y,
		})
		return Decision{
			Status: "refreshing", Detail: fmt.Sprintf(
				"Re-ping Khan camp %d:%d for report %d before choosing its MSD", selected.X, selected.Y, selected.ReportID,
			),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
			Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	observation, found := snapshot.State.LookupMapObservation(selected.KingdomID, fmt.Sprintf("%d:%d", selected.X, selected.Y))
	if !found || observation.TypeID != autoKhanCampTypeID ||
		observation.ObservedAt.Before(selected.CooldownObservedAt) {
		selected.CooldownObservedAt = time.Time{}
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": selected.KingdomID,
			"x1":        selected.X, "y1": selected.Y, "x2": selected.X, "y2": selected.Y,
		})
		return Decision{
			Status: "refreshing", Detail: fmt.Sprintf(
				"Re-ping Khan camp %d:%d for report %d", selected.X, selected.Y, selected.ReportID,
			),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
			Request: &Intent.Request{Name: "map.query", Arguments: arguments}, ReevaluateOnSuccess: true,
		}, nil
	}
	reportIDs := khanCooldownReportGroup(reports, selected, observation.ObservedAt)
	remaining := nomadCampCooldownRemaining(snapshot.State, observation, snapshot.Now)
	lane.Metrics["cooldownRemaining"] = float64(remaining)
	lane.Metrics["cooldownReportsInMSD"] = float64(len(reportIDs))
	if remaining <= 0 {
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": selected.KingdomID, "targetX": selected.X, "targetY": selected.Y,
			"reportIds": reportIDs, "cooldownObservedAt": observation.ObservedAt, "khanGuard": lane.Guard,
		})
		return Decision{
			Status: "resolving", Detail: fmt.Sprintf(
				"Resolve %d Khan report(s) already clear after the target re-ping", len(reportIDs),
			),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
			Request:             &Intent.Request{Name: "khan.cooldown.reports.resolve", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	if responseGatedDungeonCooldownCount(snapshot.State, lane.Settings.TimeSkipReserve, int64(remaining)) < 1 {
		return autoKhanWaiting(
			snapshot.Now,
			"No Khan cooldown time skip is available above the configured reserves",
			lane.Settings.CheckIntervalSec,
			lane.Metrics,
		), nil
	}
	request := map[string]any{
		"kingdomId": selected.KingdomID, "targetTypeId": autoKhanCampTypeID,
		"targetX": selected.X, "targetY": selected.Y, "eventCampId": observation.EventCampID,
		"minimumRemaining": lane.Settings.TimeSkipReserve, "khanReportIds": reportIDs, "khanGuard": lane.Guard,
	}
	arguments, _ := json.Marshal(request)
	return Decision{
		Status: "cooldown", Detail: fmt.Sprintf(
			"Apply one report-linked MSD to %d live cooldown seconds for %d report(s)",
			remaining, len(reportIDs),
		),
		NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
		Request:             &Intent.Request{Name: "nomad.cooldown.minute_skip", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func (*AutoKhanRagePolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	lane, blocked, err := autoKhanAsyncLaneContext(snapshot)
	if err != nil || blocked != nil {
		if blocked != nil {
			return *blocked, nil
		}
		return Decision{}, err
	}
	if !lane.Settings.TriggerRage {
		lane.Metrics["rageTriggerEnabled"] = 0
		return Decision{
			Status: "idle", Detail: "Automatic Khan rage retaliation is disabled; attacks and cooldown handling remain active",
			NextCheckAt: snapshot.Now.Add(policyInterval(lane.Settings.CheckIntervalSec, defaultKhanCheckIntervalSec)),
			Metrics:     lane.Metrics,
		}, nil
	}
	lane.Metrics["rageTriggerEnabled"] = 1
	target, found := autoKhanTarget(snapshot.State)
	if !found {
		return autoKhanWaiting(snapshot.Now, "Waiting for the attack lane to locate the type-35 Khan camp", 1, lane.Metrics), nil
	}
	lane.Metrics["playerRage"] = float64(snapshot.State.Khan.PlayerRage)
	lane.Metrics["playerRageCap"] = float64(snapshot.State.Khan.PlayerRageCap)
	lane.Metrics["playerTotalRage"] = float64(snapshot.State.Khan.PlayerTotalRage)
	lane.Metrics["activeTaunts"] = float64(len(snapshot.State.Khan.Taunts))
	if autoKhanTauntDue(snapshot.State.Khan) {
		arguments, _ := json.Marshal(map[string]any{
			"eventId": autoKhanEventID, "mainCastleId": lane.Main.ID,
			"targetX": target.X, "targetY": target.Y,
			"rageCampId":      snapshot.State.Khan.RageCampID,
			"playerTotalRage": snapshot.State.Khan.PlayerTotalRage,
			"rageObservedAt":  snapshot.State.Khan.RageObservedAt,
			"khanGuard":       lane.Guard,
		})
		return Decision{
			Status: "taunting", Detail: fmt.Sprintf(
				"Dispatch Khan retaliation at full rage (%d / %d)",
				snapshot.State.Khan.PlayerRage, snapshot.State.Khan.PlayerRageCap,
			),
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
			Request:             &Intent.Request{Name: "khan.taunt", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	if len(snapshot.State.Khan.Taunts) > 0 {
		next := snapshot.Now.Add(2 * time.Second)
		for _, taunt := range snapshot.State.Khan.Taunts {
			candidate := taunt.ImpactAt.Add(time.Second)
			if candidate.After(snapshot.Now) && candidate.Before(next) {
				next = candidate
			}
		}
		return Decision{
			Status: "resolving", Detail: fmt.Sprintf(
				"Tracking %d active Khan retaliation(s) through resolution", len(snapshot.State.Khan.Taunts),
			),
			NextCheckAt: next, Metrics: lane.Metrics,
		}, nil
	}
	return Decision{
		Status: "idle", Detail: fmt.Sprintf(
			"Watching Khan rage (%d / %d)", snapshot.State.Khan.PlayerRage, snapshot.State.Khan.PlayerRageCap,
		),
		NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
	}, nil
}

// The wall has to be restocked after every landed retaliation, but that work
// must never stand between a full rage bar and its taunt. It therefore owns a
// lane of its own and trails the taunt boundary the same way the cooldown lane
// trails a confirmed report.
func (*AutoKhanDefensePolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	lane, blocked, err := autoKhanAsyncLaneContext(snapshot)
	if err != nil || blocked != nil {
		if blocked != nil {
			return *blocked, nil
		}
		return Decision{}, err
	}
	boundary := autoKhanDefenseBoundary(snapshot.State.Khan)
	if boundary.IsZero() || lane.Main.Defense.ObservedAt.After(boundary) {
		return Decision{
			Status: "idle", Detail: "Main castle defense is current for the last Khan retaliation",
			NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
		}, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"castleId": lane.Main.ID, "presetId": lane.DefensePreset.ID, "presetName": lane.DefensePreset.Name,
		"wall": lane.DefensePreset.Wall, "moat": lane.DefensePreset.Moat, "keep": lane.DefensePreset.Keep,
		"khanGuard": lane.Guard,
	})
	return Decision{
		Status: "defending", Detail: fmt.Sprintf(
			"Reapply defense preset %s after the dispatched Khan taunt", lane.DefensePreset.Name,
		),
		NextCheckAt: snapshot.Now.Add(time.Second), Metrics: lane.Metrics,
		Request:             &Intent.Request{Name: "defense.preset.apply", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

// autoKhanDefenseBoundary is the moment the main-castle defense must be newer
// than: the most recent taunt dispatch or resolution, whichever came last.
func autoKhanDefenseBoundary(khan State.KhanState) time.Time {
	boundary := khan.LastTauntTriggeredAt
	if khan.LastTauntResolvedAt.After(boundary) {
		boundary = khan.LastTauntResolvedAt
	}
	return boundary
}

func autoKhanAsyncLaneContext(
	snapshot Snapshot,
) (autoKhanLaneContext, *Decision, error) {
	settings := autoKhanSettings{
		MinimumRemainingSec: 300, CheckIntervalSec: defaultKhanCheckIntervalSec,
		DefenseRefreshIntervalSec: defaultKhanDefenseRefreshSec, MapRefreshIntervalSec: defaultKhanMapRefreshSec,
		AttackLaunchesEnabled: true, TriggerRage: true, SkipCooldowns: true,
		TimeSkipReserve: map[string]int64{}, OffensiveUnitThreshold: 1000,
		HorseTravelBoostID: -1,
	}
	wait := func(detail string, metrics map[string]float64) (autoKhanLaneContext, *Decision, error) {
		decision := autoKhanWaiting(snapshot.Now, detail, settings.CheckIntervalSec, metrics)
		return autoKhanLaneContext{}, &decision, nil
	}
	if !decodeSection(snapshot.Configuration, "automation.autoKhan", &settings) {
		return wait("Auto Khan is not configured", nil)
	}
	settings.AttackPresetID = strings.TrimSpace(settings.AttackPresetID)
	settings.DefensePresetID = strings.TrimSpace(settings.DefensePresetID)
	if settings.SourceCastleID <= 0 || settings.AttackPresetID == "" || settings.DefensePresetID == "" {
		return wait("Choose a Great Empire attack castle plus attack and defense presets", nil)
	}
	if invalidTimeSkipReserve(settings.TimeSkipReserve) {
		return wait("Khan time-skip reserves cannot be negative", nil)
	}
	if settings.OpenGateProtection && settings.OffensiveUnitThreshold <= 0 {
		return wait("Open-gate protection requires a positive offensive-unit threshold", nil)
	}
	if snapshot.GameData == nil {
		return wait("Official game data is unavailable", nil)
	}
	source, found := snapshot.State.Castles[settings.SourceCastleID]
	if !found || source.KingdomID != 0 {
		return wait("Auto Khan attack source must be an available Great Empire castle", nil)
	}
	main, found := autoKhanMainCastle(snapshot.State)
	if !found {
		return wait("The Great Empire main castle is unavailable", nil)
	}
	if settings.OpenGateProtection && source.ID != main.ID {
		settings.OpenGateProtection = false
	}
	score, active := snapshot.State.LookupScalableEventScore(autoKhanEventID)
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		if decision, locked := limitedEventGate(
			snapshot.State, snapshot.Now, []int64{autoKhanEventID}, "Nomad Khan event",
		); locked {
			return autoKhanLaneContext{}, &decision, nil
		}
		return wait("Waiting for the Nomad event and Khan camp", nil)
	}
	defensePreset, err := KhanDomain.DecodeDefensePreset(
		snapshot.Configuration.Sections[KhanDomain.DefensePresetsSection], settings.DefensePresetID,
	)
	if err != nil {
		return wait(err.Error(), nil)
	}
	metrics := autoKhanMetrics(snapshot.State, main)
	metrics["eventRemainingSec"] = float64(max(int64(0), autoKhanRemaining(score, snapshot.Now)))
	metrics["nomadPoints"] = float64(score.PlayerScore)
	metrics["nomadPointThreshold"] = float64(settings.NomadPointThreshold)
	guard := map[string]any{
		"mainCastleId": main.ID, "defensePreset": defensePreset,
		"openGateProtection":     settings.OpenGateProtection,
		"offensiveUnitThreshold": settings.OffensiveUnitThreshold,
		"nomadPointThreshold":    settings.NomadPointThreshold,
	}
	if settings.NomadPointThreshold > 0 && score.PlayerScore >= settings.NomadPointThreshold {
		return wait("Nomad point threshold reached; the protection lane is stopping Auto Khan", metrics)
	}
	if _, threatCount, _, _ := incomingThreats(snapshot.State, snapshot.Now); threatCount > 0 {
		decision := Decision{
			Status: "yielding", Detail: "Incoming player attack detected; Auto Khan yielded to Auto Station",
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		}
		return autoKhanLaneContext{}, &decision, nil
	}
	if autoKhanStationingActive(snapshot.State, snapshot.Now) {
		decision := Decision{
			Status: "yielding", Detail: "Auto Station is moving troops; Auto Khan lanes are paused",
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		}
		return autoKhanLaneContext{}, &decision, nil
	}
	if snapshot.State.Khan.Protection.Active {
		return wait("Auto Khan protection is active: "+snapshot.State.Khan.Protection.Reason, metrics)
	}
	if main.Defense.OpenGateUntil != nil && main.Defense.OpenGateUntil.After(snapshot.Now) {
		decision := Decision{
			Status: "protected", Detail: "Main castle gates are open; Auto Khan lanes are paused",
			NextCheckAt: minTime(main.Defense.OpenGateUntil.Add(time.Second), snapshot.Now.Add(30*time.Second)), Metrics: metrics,
		}
		return autoKhanLaneContext{}, &decision, nil
	}
	if settings.OpenGateProtection {
		risk, riskErr := KhanDomain.OffensiveWallUnits(main, snapshot.GameData, defensePreset)
		if riskErr != nil {
			return wait(riskErr.Error(), metrics)
		}
		metrics["offensiveWallUnits"] = float64(risk.OffensiveUnits)
		metrics["offensiveUnitThreshold"] = float64(settings.OffensiveUnitThreshold)
		if risk.OffensiveUnits >= settings.OffensiveUnitThreshold {
			return wait(fmt.Sprintf(
				"Offensive wall units reached %d / %d; waiting for shared gate protection",
				risk.OffensiveUnits, settings.OffensiveUnitThreshold,
			), metrics)
		}
	}
	return autoKhanLaneContext{
		Settings: settings, Source: source, Main: main, Score: score,
		DefensePreset: defensePreset, Guard: guard, Metrics: metrics,
	}, nil, nil
}

func pendingKhanCooldownReports(gameState State.GameState) []State.KhanCooldownReportState {
	reports := make([]State.KhanCooldownReportState, 0, len(gameState.Khan.CooldownReports))
	for _, report := range gameState.Khan.CooldownReports {
		if !report.ResolvedAt.IsZero() {
			continue
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(left, right int) bool {
		if !reports[left].LandedAt.Equal(reports[right].LandedAt) {
			return reports[left].LandedAt.Before(reports[right].LandedAt)
		}
		return reports[left].ReportID < reports[right].ReportID
	})
	return reports
}

func khanCooldownReportGroup(
	reports []State.KhanCooldownReportState,
	selected State.KhanCooldownReportState,
	observedAt time.Time,
) []int64 {
	result := make([]int64, 0, len(reports))
	for _, report := range reports {
		if report.KingdomID != selected.KingdomID || report.X != selected.X || report.Y != selected.Y ||
			report.CooldownObservedAt.IsZero() || report.CooldownObservedAt.After(observedAt) ||
			report.LandedAt.After(observedAt) {
			continue
		}
		result = append(result, report.ReportID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func autoKhanMainCastle(gameState State.GameState) (State.CastleState, bool) {
	for _, castle := range gameState.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

func autoKhanTarget(gameState State.GameState) (State.MapObservation, bool) {
	if gameState.Khan.RunID != "" {
		if target, found := gameState.LookupMapObservation(0, fmt.Sprintf("%d:%d", gameState.Khan.TargetX, gameState.Khan.TargetY)); found && autoKhanTargetCandidate(target) {
			return target, true
		}
	}
	candidates := make([]State.MapObservation, 0)
	gameState.RangeMapObservationsByKind(0, State.MapProjectionEventCamp, func(_ string, target State.MapObservation) bool {
		if autoKhanTargetCandidate(target) {
			candidates = append(candidates, target)
		}
		return true
	})
	sort.Slice(candidates, func(left, right int) bool {
		if !candidates[left].ObservedAt.Equal(candidates[right].ObservedAt) {
			return candidates[left].ObservedAt.After(candidates[right].ObservedAt)
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

func autoKhanTargetCandidate(target State.MapObservation) bool {
	return target.KingdomID == 0 && target.TypeID == autoKhanCampTypeID
}

func autoKhanCooldownSkipDue(gameState State.GameState, target State.MapObservation) bool {
	key := towerTargetKey(target.KingdomID, target.X, target.Y)
	cooldown, exists := gameState.NomadCamps.Cooldowns[key]
	if !exists {
		return false
	}
	if !cooldown.LastSuccessfulBattleAt.IsZero() &&
		(gameState.Khan.LastCooldownSkippedAt.IsZero() ||
			cooldown.LastSuccessfulBattleAt.After(gameState.Khan.LastCooldownSkippedAt)) {
		return true
	}
	if cooldown.CooldownRemaining <= 0 || cooldown.CooldownObservedAt.IsZero() {
		return false
	}
	for _, launch := range gameState.Khan.Launches {
		if launch.ArrivesAt.IsZero() || launch.ArrivesAt.After(cooldown.CooldownObservedAt) ||
			!gameState.Khan.LastCooldownSkippedAt.IsZero() &&
				!launch.ArrivesAt.After(gameState.Khan.LastCooldownSkippedAt) {
			continue
		}
		return true
	}
	return false
}

func autoKhanTauntDue(khan State.KhanState) bool {
	if khan.PlayerRageCap <= 0 || khan.PlayerRage < khan.PlayerRageCap || khan.RageObservedAt.IsZero() {
		return false
	}
	if khan.LastTauntTriggeredAt.IsZero() {
		return true
	}
	return khan.PlayerTotalRage > khan.LastTauntTriggeredRage
}

func autoKhanCapacityLimitedPreset(
	snapshot Snapshot,
	source State.CastleState,
	target State.MapObservation,
	preset AttackPresets.Preset,
	commanderID State.CommanderID,
) (AttackPresets.Preset, error) {
	dialog := snapshot.State.AttackDialog
	useAttackDialog := dialog.SourceCastleID == source.ID && dialog.KingdomID == target.KingdomID &&
		dialog.Target.TypeID == target.TypeID && dialog.Target.X == target.X && dialog.Target.Y == target.Y
	capacity, err := (AttackCapacity.Resolver{}).Resolve(snapshot.State, snapshot.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: useAttackDialog,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("khan-camp:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				Level: target.Level,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return AttackPresets.Preset{}, err
	}
	return AttackPresets.LimitToCapacity(preset, capacity), nil
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
					Level: target.Level,
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

func autoKhanMapJump(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "khan.map.jump", Arguments: json.RawMessage(`{}`)}, ReevaluateOnSuccess: true,
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

	offers, _, _ := snapshot.State.ConstructionOffersFor(main.ID, main.KingdomID)
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
				remaining := max(int64(0), item.Stock-offers[State.PackageID(item.PackageID)])
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
		"tauntsTriggered":    float64(gameState.Khan.TauntsTriggered),
		"tauntsObserved":     float64(gameState.Khan.TauntsObserved), "tauntsResolved": float64(gameState.Khan.TauntsResolved),
		"activeTaunts": float64(len(gameState.Khan.Taunts)),
	}
	return metrics
}

func autoKhanOutgoingMovementIDs(gameState State.GameState, now time.Time) []State.MovementID {
	seen := map[State.MovementID]struct{}{}
	result := make([]State.MovementID, 0, len(gameState.Khan.Launches))
	for _, launch := range gameState.Khan.Launches {
		movement, found := gameState.LookupMovement(launch.MovementID)
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
