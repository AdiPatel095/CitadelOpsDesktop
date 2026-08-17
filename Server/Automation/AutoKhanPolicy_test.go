package Automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

func TestAutoKhanPolicyProtectsMainThresholdButAllowsOutpostSource(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	mainSnapshot := autoKhanPolicySnapshot(t, now, 1)
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), mainSnapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.open_gate" {
		t.Fatalf("main threshold decision: %#v err=%v", decision, err)
	}
	if decision.Metrics["offensiveWallUnits"] != 1_000 {
		t.Fatalf("main threshold metrics = %#v", decision.Metrics)
	}

	outpostSnapshot := autoKhanPolicySnapshot(t, now, 2)
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), outpostSnapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("outpost attack decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnStale {
		t.Fatal("Khan attack did not opt into immediate cooldown-state re-evaluation")
	}
	var request struct {
		SourceCastleID     State.CastleID `json:"sourceCastleId"`
		MainCastleID       State.CastleID `json:"mainCastleId"`
		OpenGateProtection bool           `json:"openGateProtection"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != 2 || request.MainCastleID != 1 || request.OpenGateProtection {
		t.Fatalf("outpost request = %#v", request)
	}
}

func TestAutoKhanPolicyYieldsToPlayerAttackAndAutoStationButNotKhanTaunt(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	playerImpact := now.Add(time.Minute)
	snapshot.State.Movements[10] = State.MovementState{
		ID: 10, Direction: 0, TypeID: 0, OwnerPlayerID: 999, TargetPlayerID: 158,
		SourceTypeID: 1, SourceCastleID: 999, TargetTypeID: 1, TargetCastleID: 1, ArrivesAt: &playerImpact,
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "yielding" || !strings.Contains(decision.Detail, "Auto Station") {
		t.Fatalf("player threat decision: %#v err=%v", decision, err)
	}

	delete(snapshot.State.Movements, 10)
	tauntImpact := now.Add(30 * time.Second)
	snapshot.State.Movements[11] = State.MovementState{
		ID: 11, Direction: 1, TypeID: 2, SourceTypeID: autoKhanCampTypeID, SourceCastleID: -1,
		TargetCastleID: 1, SourceX: 210, SourceY: 942, TargetX: 212, TargetY: 941, ReturnsAt: &tauntImpact,
	}
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("Khan taunt was mistaken for a player attack: %#v err=%v", decision, err)
	}

	stationImpact := now.Add(45 * time.Second)
	snapshot.State.Movements[12] = State.MovementState{
		ID: 12, Direction: 0, SourceCastleID: 1, TargetCastleID: 900, ArrivesAt: &stationImpact,
	}
	snapshot.State.Stationing["autoStation:1"] = State.StationingOperation{
		ID: "autoStation:1", Purpose: "autoStation", SourceCastleID: 1, TargetCastleID: 900, MovementID: 12,
	}
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "yielding" || !strings.Contains(decision.Detail, "moving troops") {
		t.Fatalf("Auto Station movement decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyKeepsExpiredProtectionLockedUntilDefenseRecovers(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 1)
	expiredAt := now.Add(-time.Minute)
	snapshot.State.Khan.Protection = State.KhanProtectionState{
		Active: true, CastleID: 1, OffensiveWallUnits: 1_000, OffensiveUnitThreshold: 1_000,
		TriggeredAt: now.Add(-6*time.Hour - time.Minute), GateOpenUntil: expiredAt,
		Reason: "Add defense units to continue",
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "blocked" {
		t.Fatalf("expired unsafe protection decision: %#v err=%v", decision, err)
	}

	main := snapshot.State.Castles[1]
	main.Defense.Inventory[489] = 1_001
	main.Defense.ObservedAt = now
	main.Defense.InventoryObservedAt = now
	snapshot.State.Castles[1] = main
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.protection.clear" {
		t.Fatalf("recovered protection decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyBlocksAnUnsafeCurrentRun(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.Khan = State.KhanState{
		RunID: "unsafe", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		Launches: []State.KhanLaunchState{}, Taunts: map[State.MovementID]State.KhanTauntState{},
		SafetyError: "a later launch would overtake the previous hit",
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "blocked" || !strings.Contains(decision.Detail, "unsafe arrival order") {
		t.Fatalf("unsafe chain decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyStopsAtNomadPointThreshold(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.EventScores.ByEvent[autoKhanEventID] = State.ScalableEventScore{
		EventID: autoKhanEventID, PlayerScore: 50_000, RemainingSec: 10, ObservedAt: now,
	}
	snapshot.Configuration.Sections["automation.autoKhan"] = json.RawMessage(`{
		"version":1,"sourceCastleId":2,"attackPresetId":"camp","defensePresetId":"defense",
		"minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,
		"mapRefreshIntervalSec":30,"skipCooldowns":true,"timeSkipReserve":{},
		"openGateProtection":false,"offensiveUnitThreshold":1000,"nomadPointThreshold":50000
	}`)
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.point_limit.protect" {
		t.Fatalf("point-limit decision: %#v err=%v", decision, err)
	}

	main := snapshot.State.Castles[1]
	gateOpenUntil := now.Add(6 * time.Hour)
	main.Defense.OpenGateUntil = &gateOpenUntil
	snapshot.State.Castles[1] = main
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "protected" {
		t.Fatalf("protected point-limit decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanDefenseToolShopRouteUsesCapturedLunaTable(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Storm.LunaShopTableID = 14
	route, active := autoKhanDefenseToolShopRoute(gameState, GameData.DefenseToolShopPackage{
		PackageID: 244, PriceScope: GameData.DefenseToolPriceCastleResource, PriceID: GameData.StormAquamarineID,
	}, time.Now().UTC())
	if !active || route.EventID != 14 {
		t.Fatalf("Luna route = %#v active=%t", route, active)
	}
}

func TestAutoKhanPolicyJumpsDirectlyToMissingKhanCamp(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 37, 59, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.Map[0] = map[string]State.MapObservation{}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.map.jump" {
		t.Fatalf("missing Khan camp decision: %#v err=%v", decision, err)
	}
	if string(decision.Request.Arguments) != "{}" {
		t.Fatalf("Khan map jump arguments = %s", decision.Request.Arguments)
	}
}

func TestAutoKhanPolicyRejectsOldTypeTwoTowerSignature(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 37, 59, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.Map[0]["210:942"] = State.MapObservation{
		KingdomID: 0, TypeID: 2, X: 210, Y: 942, TowerVictoryCount: 845, ObservedAt: now,
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.map.jump" {
		t.Fatalf("old type-2 tower decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanAttackPolicyClearsUnreportedCooldownImmediatelyBeforeCRA(t *testing.T) {
	now := time.Date(2026, 7, 25, 14, 34, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	target := snapshot.State.Map[0]["210:942"]
	target.EventCampCooldownRemaining = 194
	snapshot.State.Map[0]["210:942"] = target
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil ||
		decision.Request.Name != "nomad.cooldown.minute_skip" || decision.Status != "cooldown" ||
		!strings.Contains(decision.Detail, "immediately before the next CRA") {
		t.Fatalf("pre-CRA fallback cooldown decision: %#v err=%v", decision, err)
	}
	decision, err = NewAutoKhanCooldownPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("unreported cooldown-lane decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanAttackPolicyFallbackDoesNotRequireAReport(t *testing.T) {
	now := time.Date(2026, 7, 25, 14, 34, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	target := snapshot.State.Map[0]["210:942"]
	target.EventCampCooldownRemaining = 194
	snapshot.State.Map[0]["210:942"] = target
	snapshot.State.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1, Taunts: map[State.MovementID]State.KhanTauntState{},
	}

	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil ||
		decision.Request.Name != "nomad.cooldown.minute_skip" || decision.Status != "cooldown" {
		t.Fatalf("unreported pre-CRA cooldown decision: %#v err=%v", decision, err)
	}
	var request struct {
		KhanReportIDs []int64 `json:"khanReportIds"`
		KhanGuard     struct {
			MainCastleID State.CastleID `json:"mainCastleId"`
		} `json:"khanGuard"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.KhanReportIDs) != 0 || request.KhanGuard.MainCastleID != 1 {
		t.Fatalf("unreported pre-CRA cooldown request = %#v", request)
	}
}

func TestAutoKhanCooldownPolicySkipsCooldownAfterConfirmedReportAndReping(t *testing.T) {
	now := time.Date(2026, 7, 25, 14, 34, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	target := snapshot.State.Map[0]["210:942"]
	target.EventCampCooldownRemaining = 194
	snapshot.State.Map[0]["210:942"] = target
	battleAt := now.Add(-time.Second)
	snapshot.State.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1, VictoriesConfirmed: 1, Taunts: map[State.MovementID]State.KhanTauntState{},
		CooldownReports: map[int64]State.KhanCooldownReportState{
			101: {
				ReportID: 101, KingdomID: 0, X: 210, Y: 942, LandedAt: battleAt,
				CooldownRemaining: 194, CooldownObservedAt: now,
			},
		},
	}
	snapshot.State.NomadCamps.Cooldowns["0:210:942"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 210, Y: 942, LastSuccessfulBattleAt: battleAt,
		CooldownRemaining: 194, CooldownObservedAt: now,
	}

	attackDecision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || attackDecision.Request != nil || attackDecision.Status != "cooldown" ||
		!strings.Contains(attackDecision.Detail, "only the CRA launch cursor is held") {
		t.Fatalf("report-linked CRA cursor decision: %#v err=%v", attackDecision, err)
	}
	decision, err := NewAutoKhanCooldownPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.cooldown.minute_skip" {
		t.Fatalf("post-landing cooldown decision: %#v err=%v", decision, err)
	}
	if decision.Metrics["pendingCooldownReports"] != 1 || decision.Metrics["cooldownReportsInMSD"] != 1 {
		t.Fatalf("post-landing cooldown metrics = %#v", decision.Metrics)
	}
	var request struct {
		TargetTypeID  int     `json:"targetTypeId"`
		KhanReportIDs []int64 `json:"khanReportIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TargetTypeID != autoKhanCampTypeID ||
		len(request.KhanReportIDs) != 1 || request.KhanReportIDs[0] != 101 {
		t.Fatalf("report-linked Khan cooldown request = %#v", request)
	}
}

func TestAutoKhanCooldownPolicyRequiresReportAndDoesNotBlockAttackOnActiveTaunt(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 34, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	target := snapshot.State.Map[0]["210:942"]
	target.EventCampCooldownRemaining = 86_100
	snapshot.State.Map[0]["210:942"] = target
	lastSkipAt := now.Add(-2 * time.Minute)
	arrivedAt := now.Add(-time.Minute)
	snapshot.State.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 2, VictoriesConfirmed: 2, CooldownsSkipped: 1, LastCooldownSkippedAt: lastSkipAt,
		Launches: []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: arrivedAt}},
		Taunts: map[State.MovementID]State.KhanTauntState{
			90: {MovementID: 90, ObservedAt: now.Add(-time.Minute), ImpactAt: now.Add(time.Minute)},
		},
		CooldownReports: map[int64]State.KhanCooldownReportState{
			202: {
				ReportID: 202, KingdomID: 0, X: 210, Y: 942, LandedAt: arrivedAt,
				CooldownRemaining: 86_100, CooldownObservedAt: now,
			},
		},
	}
	snapshot.State.NomadCamps.Cooldowns["0:210:942"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 210, Y: 942,
		LastSuccessfulBattleAt: lastSkipAt.Add(-time.Second),
		CooldownRemaining:      86_100,
		CooldownObservedAt:     now,
	}

	decision, err := NewAutoKhanCooldownPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.cooldown.minute_skip" {
		t.Fatalf("report-linked cooldown decision: %#v err=%v", decision, err)
	}

	target.EventCampCooldownRemaining = 0
	target.ObservedAt = now.Add(time.Second)
	snapshot.State.Map[0]["210:942"] = target
	cooldown := snapshot.State.NomadCamps.Cooldowns["0:210:942"]
	cooldown.CooldownRemaining = 0
	cooldown.CooldownObservedAt = now.Add(time.Second)
	snapshot.State.NomadCamps.Cooldowns["0:210:942"] = cooldown
	report := snapshot.State.Khan.CooldownReports[202]
	report.CooldownRemaining = 0
	report.CooldownObservedAt = now.Add(time.Second)
	snapshot.State.Khan.CooldownReports[202] = report
	decision, err = NewAutoKhanCooldownPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.cooldown.reports.resolve" {
		t.Fatalf("cleared report resolution decision: %#v err=%v", decision, err)
	}
	report.ResolvedAt = now.Add(time.Second)
	snapshot.State.Khan.CooldownReports[202] = report
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("active taunt blocked resumed Khan attack: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyCapacityLimitsPresetBeforeInventoryCheck(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 20, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	source := snapshot.State.Castles[2]
	source.Units.Stationed[215] = 200
	snapshot.State.Castles[2] = source
	snapshot.Configuration.Sections["attacks.presets"] = json.RawMessage(`{
		"version":1,"presets":[{"id":"camp","name":"Khan Camp","waves":[{
			"L":{"troops":[],"tools":[]},
			"M":{"troops":[{"itemId":215,"quantity":65000}],"tools":[]},
			"R":{"troops":[],"tools":[]}
		}]}]
	}`)

	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("capacity-limited Khan decision: %#v err=%v", decision, err)
	}
	if decision.Metrics["presetCopies"] != 1 {
		t.Fatalf("capacity-limited preset copies = %v", decision.Metrics["presetCopies"])
	}
	var request struct {
		Preset AttackPresets.Preset `json:"preset"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if quantity := request.Preset.Waves[0].Middle.Troops[0].Quantity; quantity != 65_000 {
		t.Fatalf("CRA request preset quantity = %d, want original 65000", quantity)
	}
}

func TestAutoKhanMissingPresetUnitsPausesOnlyTheCRALaunchCursor(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 10, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	source := snapshot.State.Castles[2]
	source.Units.Stationed[215] = 50
	snapshot.State.Castles[2] = source

	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "CRA launch cursor paused") ||
		decision.Metrics["attackLaunchPaused"] != 1 {
		t.Fatalf("missing-unit attack cursor decision: %#v err=%v", decision, err)
	}

	snapshot.State.Khan.RageCampID = 1147
	snapshot.State.Khan.PlayerRage = 1740
	snapshot.State.Khan.PlayerRageCap = 1740
	snapshot.State.Khan.PlayerTotalRage = 1740
	snapshot.State.Khan.RageObservedAt = now
	rageDecision, err := NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || rageDecision.Request == nil || rageDecision.Request.Name != "khan.taunt" {
		t.Fatalf("preset shortage blocked the rage lane: %#v err=%v", rageDecision, err)
	}
	cooldownDecision, err := NewAutoKhanCooldownPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || cooldownDecision.Status != "idle" {
		t.Fatalf("preset shortage blocked the cooldown lane: %#v err=%v", cooldownDecision, err)
	}
}

func TestAutoKhanRageToggleStopsTauntsWithoutStoppingAttacks(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	setAutoKhanPolicySetting(t, &snapshot, "triggerRage", false)
	snapshot.State.Khan.RageCampID = 1147
	snapshot.State.Khan.PlayerRage = 1740
	snapshot.State.Khan.PlayerRageCap = 1740
	snapshot.State.Khan.PlayerTotalRage = 1740
	snapshot.State.Khan.RageObservedAt = now

	rageDecision, err := NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || rageDecision.Request != nil || rageDecision.Status != "idle" ||
		!strings.Contains(rageDecision.Detail, "disabled") || rageDecision.Metrics["rageTriggerEnabled"] != 0 {
		t.Fatalf("disabled rage decision: %#v err=%v", rageDecision, err)
	}
	attackDecision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || attackDecision.Request == nil || attackDecision.Request.Name != "khan.attack" {
		t.Fatalf("disabled rage stopped Khan attacks: %#v err=%v", attackDecision, err)
	}
}

func TestAutoKhanAttackLockMaintainsManualChainWithoutLaunching(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 5, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	setAutoKhanPolicySetting(t, &snapshot, "attackLaunchesEnabled", false)

	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "idle" ||
		!strings.Contains(decision.Detail, "manual chain") || decision.Metrics["attackLaunchLocked"] != 1 {
		t.Fatalf("locked attack decision: %#v err=%v", decision, err)
	}

	target := snapshot.State.Map[0]["210:942"]
	target.EventCampCooldownRemaining = 194
	snapshot.State.Map[0]["210:942"] = target
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.cooldown.minute_skip" ||
		!strings.Contains(decision.Detail, "maintain the manual chain") {
		t.Fatalf("locked attack cooldown maintenance: %#v err=%v", decision, err)
	}

	snapshot.State.Khan.RageCampID = 1147
	snapshot.State.Khan.PlayerRage = 1740
	snapshot.State.Khan.PlayerRageCap = 1740
	snapshot.State.Khan.PlayerTotalRage = 1740
	snapshot.State.Khan.RageObservedAt = now
	rageDecision, err := NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || rageDecision.Request == nil || rageDecision.Request.Name != "khan.taunt" {
		t.Fatalf("attack lock stopped the independent rage lane: %#v err=%v", rageDecision, err)
	}
}

func TestAutoKhanRagePolicyTriggersLTAThenDefenseWithoutWaitingForExistingTaunts(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 20, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.Khan.RageCampID = 1147
	snapshot.State.Khan.PlayerRage = 1740
	snapshot.State.Khan.PlayerRageCap = 1740
	snapshot.State.Khan.PlayerTotalRage = 52140
	snapshot.State.Khan.RageObservedAt = now

	decision, err := NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.taunt" ||
		decision.Status != "taunting" {
		t.Fatalf("full-rage Khan decision: %#v err=%v", decision, err)
	}

	snapshot.State.Khan.TauntsTriggered = 1
	snapshot.State.Khan.LastTauntTriggeredAt = now.Add(time.Second)
	snapshot.State.Khan.LastTauntTriggeredRage = 50400
	impactAt := now.Add(2 * time.Minute)
	snapshot.State.Khan.Taunts[90] = State.KhanTauntState{
		MovementID: 90, ObservedAt: now, ImpactAt: impactAt,
	}
	decision, err = NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.taunt" {
		t.Fatalf("parallel full-rage Khan decision: %#v err=%v", decision, err)
	}

	snapshot.State.Khan.LastTauntTriggeredRage = snapshot.State.Khan.PlayerTotalRage
	decision, err = NewAutoKhanDefensePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "defense.preset.apply" {
		t.Fatalf("the defense lane did not reapply the preset after the taunt: %#v err=%v", decision, err)
	}
	decision, err = NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil {
		t.Fatalf("the rage lane queued defense work of its own: %#v err=%v", decision, err)
	}
	// An outstanding wall restock must never hold back the next taunt.
	snapshot.State.Khan.PlayerTotalRage = 53880
	decision, err = NewAutoKhanRagePolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.taunt" {
		t.Fatalf("a pending defense reapply blocked the next taunt: %#v err=%v", decision, err)
	}
	snapshot.State.Khan.PlayerTotalRage = 52140
	main := snapshot.State.Castles[1]
	main.Defense.ObservedAt = now.Add(2 * time.Second)
	main.Defense.InventoryObservedAt = now.Add(2 * time.Second)
	snapshot.State.Castles[1] = main
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("active taunt blocked the independent attack lane: %#v err=%v", decision, err)
	}
}

func setAutoKhanPolicySetting(t *testing.T, snapshot *Snapshot, key string, value any) {
	t.Helper()
	settings := map[string]any{}
	if err := json.Unmarshal(snapshot.Configuration.Sections["automation.autoKhan"], &settings); err != nil {
		t.Fatal(err)
	}
	settings[key] = value
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Configuration.Sections["automation.autoKhan"] = raw
}

func autoKhanPolicySnapshot(t *testing.T, now time.Time, sourceCastleID State.CastleID) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":215,"role":"melee","fightType":"0"},
			{"wodID":489,"role":"melee","fightType":"1"}
		],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":"1147","eventID":"72","difficultyID":"310",
			"areaType":"35","camplevel":"107","playerRageCap":"1740"
		}],
		"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defenseRaw := json.RawMessage(`{
		"version":1,
		"presets":[{
			"id":"defense","name":"Defense",
			"wall":{
				"left":{"toolSlots":[],"unitPercent":33,"unitTypePercent":100},
				"middle":{"toolSlots":[],"unitPercent":34,"unitTypePercent":100},
				"right":{"toolSlots":[],"unitPercent":33,"unitTypePercent":100}
			},
			"moat":{"leftToolSlots":[],"middleToolSlots":[],"rightToolSlots":[]}
		}]
	}`)
	defensePreset, err := KhanDomain.DecodeDefensePreset(defenseRaw, "defense")
	if err != nil {
		t.Fatal(err)
	}
	main := State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, SlotType: 1, X: 212, Y: 941,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 3_000}, Traveling: map[State.UnitID]int64{}},
		Defense: State.CastleDefenseState{
			Wall: State.DefenseWallState{
				Left: defensePreset.Wall.Left, Middle: defensePreset.Wall.Middle, Right: defensePreset.Wall.Right,
				UnitCount: 2_000,
			},
			Moat: State.DefenseMoatState{
				LeftToolSlots:   defensePreset.Moat.LeftToolSlots,
				MiddleToolSlots: defensePreset.Moat.MiddleToolSlots,
				RightToolSlots:  defensePreset.Moat.RightToolSlots,
			},
			MeleeUnitIDs: []State.UnitID{489, 215}, RangedUnitIDs: []State.UnitID{},
			Inventory:  map[State.UnitID]int64{489: 1_000, 215: 2_000},
			ObservedAt: now, InventoryObservedAt: now,
		},
	}
	outpost := State.CastleState{
		ID: 2, Name: "Attack Outpost", KingdomID: 0, SlotType: 2, X: 220, Y: 945,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 3_000}, Traveling: map[State.UnitID]int64{}},
	}
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Player.Currencies[1006] = 10
	gameState.Castles[1] = main
	gameState.Castles[2] = outpost
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	gameState.EventScores.ActiveEventID = autoKhanEventID
	gameState.EventScores.ByEvent[autoKhanEventID] = State.ScalableEventScore{
		EventID: autoKhanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"210:942": {
			KingdomID: 0, TypeID: autoKhanCampTypeID, X: 210, Y: 942,
			EventCampID: 1145, ObjectID: 1145, Level: 105, ObservedAt: now,
		},
	}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoKhan": json.RawMessage(fmt.Sprintf(`{
				"version":1,"sourceCastleId":%d,"attackPresetId":"camp","defensePresetId":"defense",
				"minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,
				"mapRefreshIntervalSec":30,"skipCooldowns":true,"timeSkipReserve":{},
				"openGateProtection":true,"offensiveUnitThreshold":1000
			}`, sourceCastleID)),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"camp","name":"Khan Camp","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":215,"quantity":100}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
			"defense.presets": defenseRaw,
		}},
	}
}

// Every Auto Khan lane must reach the wire as the same feature. A lane whose
// actor is unknown to the priority table falls back to background priority and
// loses claim and dispatch ties to every other automation.
func TestAutoKhanLanesShareTheFeatureActorAndPriority(t *testing.T) {
	lanes := []Policy{
		NewAutoKhanPolicy(), NewAutoKhanCooldownPolicy(),
		NewAutoKhanRagePolicy(), NewAutoKhanDefensePolicy(),
	}
	for _, policy := range lanes {
		actor := "automation:" + policyActorID(policy)
		priority, err := Outbound.ResolvePriority(actor, 0)
		if err != nil {
			t.Fatalf("%s actor %q: %v", policy.ID(), actor, err)
		}
		if priority != Outbound.PriorityAutoKhan {
			t.Errorf("%s resolved to priority %d as %q, want %d",
				policy.ID(), priority, actor, Outbound.PriorityAutoKhan)
		}
		if policy.EnabledKey() != "auto_khan" {
			t.Errorf("%s enabled key = %q", policy.ID(), policy.EnabledKey())
		}
		if policyScheduleKey(policy) != "autoKhan" {
			t.Errorf("%s schedule key = %q", policy.ID(), policyScheduleKey(policy))
		}
	}
}
