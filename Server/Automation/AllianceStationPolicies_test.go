package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestProtectedHoldingsUseMemberProtectionAndKingdomDistance(t *testing.T) {
	alliance := State.AllianceState{
		Members: []State.AllianceMember{
			{PlayerID: 1, ReturnProtectionSec: 4 * 86_400},
			{PlayerID: 2, ReturnProtectionSec: 86_400},
		},
		Holdings: []State.AllianceHolding{
			{CastleID: 10, PlayerID: 1, KingdomID: 0, X: 20, Y: 20, SlotType: 1},
			{CastleID: 11, PlayerID: 1, KingdomID: 0, X: 12, Y: 12, SlotType: 4},
			{CastleID: 12, PlayerID: 2, KingdomID: 0, X: 11, Y: 11, SlotType: 1},
			{CastleID: 13, PlayerID: 1, KingdomID: 2, X: 10, Y: 10, SlotType: 12},
		},
	}
	holdings := protectedHoldings(alliance, 3)
	if len(holdings) != 3 {
		t.Fatalf("expected three protected holdings, got %#v", holdings)
	}
	nearest, ok := nearestHolding(holdings, State.CastleState{ID: 99, KingdomID: 0, X: 10, Y: 10})
	if !ok || nearest.CastleID != 11 {
		t.Fatalf("unexpected nearest holding: %#v", nearest)
	}
}

func TestAutoBirdStartsEachCastleWithAINAndFreshJAAPreparation(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Alliance.ID = 9
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Castles[10] = State.CastleState{ID: 10, KingdomID: 0, X: 10, Y: 10}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.discover" {
		t.Fatalf("castle preparation = %#v, err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess {
		t.Fatal("castle preparation should continue the current bird cycle after success")
	}
	if want := now.Add(30 * time.Second); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("castle preparation next check = %s, want %s", decision.NextCheckAt, want)
	}
}

func TestAutoBirdStillRunsCastlePreparationWhenCachedAllianceRosterIsEmpty(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Alliance.ID = 9
	gameState.Alliance.ObservedAt = now
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Castles[10] = State.CastleState{ID: 10, KingdomID: 0, X: 10, Y: 10}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "auto_bird.discover" {
		t.Fatalf("empty cached roster did not run the castle's fresh AIN cycle: %#v", decision)
	}
}

func TestAutoBirdWakesForEachCastleReturnAndUnitRefresh(t *testing.T) {
	want := map[string]bool{
		"alliance": true, "movement-snapshot": true, "movements": true,
		"player-protection": true, "stationing": true, "units": true,
	}
	domains := NewAutoBirdPolicy().WakeDomains()
	if len(domains) != len(want) {
		t.Fatalf("auto bird wake domains = %v", domains)
	}
	for _, domain := range domains {
		if !want[domain] {
			t.Fatalf("unexpected auto bird wake domain %q in %v", domain, domains)
		}
	}
}

func TestStationPoliciesRefreshProtectionFromGameAfterToggleOrStaleObservation(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 30, 0, 0, time.UTC)
	for _, policy := range []Policy{NewAutoBirdPolicy(), NewAutoStationPolicy()} {
		for _, test := range []struct {
			name                 string
			observedAt           time.Time
			configurationChanged bool
		}{
			{name: "toggle", observedAt: now, configurationChanged: true},
			{name: "stale", observedAt: now.Add(-playerProtectionRefreshInterval)},
		} {
			t.Run(policy.ID()+"/"+test.name, func(t *testing.T) {
				gameState := State.NewGameState()
				gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
					ModeState: -1, ObservedAt: test.observedAt,
				}
				gameState.Castles[10] = State.CastleState{
					ID: 10, KingdomID: 0, X: 123, Y: 456, Focused: true,
				}
				decision, err := policy.Evaluate(t.Context(), Snapshot{
					State: gameState, Now: now, PolicyConfigurationChanged: test.configurationChanged,
				})
				if err != nil || decision.Request == nil || decision.Request.Name != "map.query" ||
					decision.Status != "refreshing" || !decision.ReevaluateOnSuccess {
					t.Fatalf("protection refresh decision = %#v err=%v", decision, err)
				}
				var request struct {
					KingdomID State.KingdomID `json:"kingdomId"`
					X1        int             `json:"x1"`
					Y1        int             `json:"y1"`
					X2        int             `json:"x2"`
					Y2        int             `json:"y2"`
				}
				if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
					t.Fatal(err)
				}
				if request.KingdomID != 0 || request.X1 != 123 || request.Y1 != 456 ||
					request.X2 != 123 || request.Y2 != 456 {
					t.Fatalf("protection refresh map request = %+v", request)
				}
			})
		}
	}
}

func TestAutoBirdSuppressesStationingDuringPurchasedProtectionMode(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Alliance.ID = 9
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, RemainingSec: 3600, ObservedAt: now,
	}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "protected" {
		t.Fatalf("Protection Mode bird decision = %#v err=%v", decision, err)
	}
}

func TestStationableUnitsExcludesToolsFromBirdAndStationMovements(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":489},{"wodID":735,"toolCategory":"Premium","slotTypes":"1,2,9"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	castle := State.CastleState{Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
		489: 222,
		735: 472_910,
	}}}
	units := stationableUnits(Snapshot{GameData: gameData}, castle, nil)
	if len(units) != 1 || units[0].UnitID != 489 || units[0].Amount != 222 {
		t.Fatalf("stationable units = %#v, want only troop 489", units)
	}
}

func TestAutoBirdPreparationCarriesConfiguredReservesAndLimits(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoBird": json.RawMessage(`{
			"ignoreSettings":{
				"settings":{"10":[{"id":489,"amount":25}]},
				"minSend":50
			}
		}`),
	}}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.discover" {
		t.Fatalf("Auto Bird decision = %#v err=%v", decision, err)
	}
	var request struct {
		MinimumRPTDays    int           `json:"minimumRPTDays"`
		MinimumDelayHours int           `json:"minimumDelayHours"`
		MaximumDelayHours int           `json:"maximumDelayHours"`
		MinimumSend       int64         `json:"minimumSend"`
		Reserves          []stationUnit `json:"reserves"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.MinimumRPTDays != 3 || request.MinimumDelayHours != 6 ||
		request.MaximumDelayHours != 12 || request.MinimumSend != 50 {
		t.Fatalf("castle-cycle settings = %+v", request)
	}
	if len(request.Reserves) != 1 || request.Reserves[0] != (stationUnit{UnitID: 489, Amount: 25}) {
		t.Fatalf("castle-cycle reserves = %#v", request.Reserves)
	}
}

func TestAutoBirdDispatchesOnlyAfterCastlePreparationIsFresh(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 100},
		DelayHours: 8, AllianceObservedAt: now, UnitsObservedAt: now, UpdatedAt: now,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.dispatch" {
		t.Fatalf("prepared Auto Bird decision = %#v err=%v", decision, err)
	}
}

func TestAutoBirdRunsJAAAfterAINSelectsTarget(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseTargetReady,
		SourceCastleID: 10, TargetCastleID: 20, DelayHours: 8, WaitSeconds: 8 * 3600,
		AllianceObservedAt: now, UpdatedAt: now,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.prepare" {
		t.Fatalf("target-ready Auto Bird decision = %#v err=%v", decision, err)
	}
}

func TestAutoBirdWaitingCastleDoesNotBlockAnotherCastleCycle(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Castles[11] = State.CastleState{
		ID: 11, KingdomID: 0, X: 11, Y: 11,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	retryAt := now.Add(5 * time.Minute)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseWaiting,
		SourceCastleID: 10, AllianceObservedAt: now.Add(-time.Minute), UnitsObservedAt: now,
		NextAttemptAt: &retryAt, StatusDetail: "No protected alliance bird target is available for this castle",
		UpdatedAt: now,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.discover" {
		t.Fatalf("independent castle decision = %#v err=%v", decision, err)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != 11 {
		t.Fatalf("prepared castle = %d, want independent castle 11", request.SourceCastleID)
	}
}

func TestAutoBirdTargetlessCastleHonorsItsOwnRetryDespiteOldUnitState(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Alliance.Holdings = nil
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	retryAt := now.Add(5 * time.Minute)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseWaiting,
		SourceCastleID: 10, AllianceObservedAt: now,
		NextAttemptAt: &retryAt, UpdatedAt: now,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request != nil ||
		!decision.NextCheckAt.After(now) || decision.NextCheckAt.After(retryAt) {
		t.Fatalf("target-less castle retry decision = %#v err=%v", decision, err)
	}
}

func TestAutoBirdPublishesPersistedReturnWhenMovementSnapshotOmitsBird(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	expectedReturn := now.Add(8 * time.Hour)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseAway,
		SourceCastleID: 10, TargetCastleID: 20, MovementID: 50,
		ExpectedReturnAt: &expectedReturn, NextAttemptAt: &expectedReturn, UpdatedAt: now,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request != nil {
		t.Fatalf("persisted return decision = %#v err=%v", decision, err)
	}
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != expectedReturn.UnixMilli() {
		t.Fatalf("persisted bird return = %d, want %d", got, expectedReturn.UnixMilli())
	}
	if got := int64(decision.Metrics[autoBirdCastleReturnMetricKey+"10"]); got != expectedReturn.UnixMilli() {
		t.Fatalf("persisted castle return = %d, want %d", got, expectedReturn.UnixMilli())
	}
}

func TestAutoBirdRecentTrackedLaunchBlocksDuplicateUntilMovementAppears(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20,
		UpdatedAt: now,
	}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, GameData: gameData, Now: now})
	if err != nil || decision.Request != nil {
		t.Fatalf("tracked bird decision = %#v err=%v", decision, err)
	}
	if _, exists := decision.Metrics[autoBirdNextMetricKey]; exists {
		t.Fatalf("recent launch guard published a predicted return: %#v", decision.Metrics)
	}
}

func TestAutoBirdRecordsTheCastleOutboundExpectedReturn(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	arrivesAt := now.Add(10 * time.Minute)
	expectedReturn := arrivesAt.Add(6*time.Hour + 10*time.Minute)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, SourceCastleID: 10, TargetCastleID: 20,
		TravelSeconds: 600, WaitSeconds: 6 * 3600, ArrivesAt: &arrivesAt,
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20, MovementID: 50,
	}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, GameData: gameData, Now: now})
	if err != nil || decision.Request != nil {
		t.Fatalf("active bird decision = %#v err=%v", decision, err)
	}
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != expectedReturn.UnixMilli() {
		t.Fatalf("expected bird return = %d, want %d", got, expectedReturn.UnixMilli())
	}
	if got := State.CastleID(decision.Metrics[autoBirdNextCastleMetricKey]); got != 10 {
		t.Fatalf("expected bird castle = %d, want 10", got)
	}
	if decision.NextCheckAt.IsZero() || !decision.NextCheckAt.Before(expectedReturn) {
		t.Fatalf("outbound return did not retain the earlier safety wake: %s", decision.NextCheckAt)
	}
}

func TestAutoBirdRecoversExpectedReturnFromUntrackedAllianceStationMovement(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	arrivesAt := now.Add(-10 * time.Minute)
	expectedReturn := arrivesAt.Add(6*time.Hour + 10*time.Minute)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, SourceCastleID: 10, TargetCastleID: 20,
		TravelSeconds: 600, WaitSeconds: 6 * 3600, ArrivesAt: &arrivesAt,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, GameData: gameData, Now: now})
	if err != nil || decision.Request != nil {
		t.Fatalf("untracked active bird decision = %#v err=%v", decision, err)
	}
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != expectedReturn.UnixMilli() {
		t.Fatalf("recovered expected bird return = %d, want %d", got, expectedReturn.UnixMilli())
	}
	if got := State.CastleID(decision.Metrics[autoBirdNextCastleMetricKey]); got != 10 {
		t.Fatalf("recovered expected bird castle = %d, want 10", got)
	}
}

func TestAutoBirdUsesEarliestGameReportedReturn(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Castles[11] = State.CastleState{
		ID: 11, KingdomID: 0, X: 11, Y: 11,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	later := now.Add(90 * time.Second)
	earlier := now.Add(45 * time.Second)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, SourceCastleID: 20, TargetCastleID: 10, ReturnsAt: &later,
	}
	gameState.Movements[51] = State.MovementState{
		ID: 51, Direction: 1, SourceCastleID: 20, TargetCastleID: 11, ReturnsAt: &earlier,
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20, MovementID: 50,
	}
	gameState.Stationing["autoBird:11"] = State.StationingOperation{
		ID: "autoBird:11", Purpose: "autoBird", SourceCastleID: 11, TargetCastleID: 20, MovementID: 51,
	}
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, GameData: gameData, Now: now})
	if err != nil || decision.Request != nil {
		t.Fatalf("active bird decision = %#v err=%v", decision, err)
	}
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != earlier.UnixMilli() {
		t.Fatalf("next bird metric = %d, want earliest game return %d", got, earlier.UnixMilli())
	}
	if got := State.CastleID(decision.Metrics[autoBirdNextCastleMetricKey]); got != 11 {
		t.Fatalf("next bird castle metric = %d, want earliest source castle 11", got)
	}
	if got := int64(decision.Metrics[autoBirdCastleReturnMetricKey+"10"]); got != later.UnixMilli() {
		t.Fatalf("castle 10 bird return metric = %d, want %d", got, later.UnixMilli())
	}
	if got := int64(decision.Metrics[autoBirdCastleReturnMetricKey+"11"]); got != earlier.UnixMilli() {
		t.Fatalf("castle 11 bird return metric = %d, want %d", got, earlier.UnixMilli())
	}
	if !decision.NextCheckAt.Equal(earlier) {
		t.Fatalf("next check = %s, want earliest game return %s", decision.NextCheckAt, earlier)
	}
}

func TestAutoBirdScheduleKeepsActualReturnWhenPolicyWakeIsLater(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	returnsAt := now.Add(20 * time.Minute)
	notBefore := now.Add(time.Hour)
	gameState := State.NewGameState()
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, SourceCastleID: 20, TargetCastleID: 10, ReturnsAt: &returnsAt,
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20, MovementID: 50,
	}

	decision := withAutoBirdSchedule(Snapshot{State: gameState, Now: now}, Decision{}, notBefore)
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != returnsAt.UnixMilli() {
		t.Fatalf("displayed next bird = %d, want actual return %d", got, returnsAt.UnixMilli())
	}
	if got := State.CastleID(decision.Metrics[autoBirdNextCastleMetricKey]); got != 10 {
		t.Fatalf("displayed next bird castle = %d, want 10", got)
	}
	if !decision.NextCheckAt.Equal(notBefore) {
		t.Fatalf("policy wake = %s, want safety gate %s", decision.NextCheckAt, notBefore)
	}
}

func TestAutoBirdResendsReturnedCastleWhileAnotherBirdIsActive(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdEligibleTestState(t, now)
	gameState.Castles[11] = State.CastleState{
		ID: 11, KingdomID: 0, X: 11, Y: 11,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	returnedLaunch := now.Add(-time.Hour)
	staleCooldown := now.Add(11 * time.Hour)
	activeReturn := now.Add(10 * time.Minute)
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20,
		MovementID: 50, SuccessCooldownUntil: &staleCooldown, UpdatedAt: returnedLaunch,
	}
	gameState.Stationing["autoBird:11"] = State.StationingOperation{
		ID: "autoBird:11", Purpose: "autoBird", SourceCastleID: 11, TargetCastleID: 20,
		MovementID: 51, UpdatedAt: returnedLaunch,
	}
	gameState.Movements[51] = State.MovementState{
		ID: 51, Direction: 1, SourceCastleID: 20, TargetCastleID: 11, ReturnsAt: &activeReturn,
	}

	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: gameState, GameData: gameData, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "auto_bird.discover" {
		t.Fatalf("returned castle bird decision = %#v err=%v", decision, err)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != 10 {
		t.Fatalf("bird source castle = %d, want returned castle 10", request.SourceCastleID)
	}
	if got := int64(decision.Metrics[autoBirdNextMetricKey]); got != activeReturn.UnixMilli() {
		t.Fatalf("next bird metric = %d, want other active return %d", got, activeReturn.UnixMilli())
	}
	if got := State.CastleID(decision.Metrics[autoBirdNextCastleMetricKey]); got != 11 {
		t.Fatalf("next bird castle metric = %d, want other active castle 11", got)
	}
}

func TestAutoStationDoesNotRecallAutoBirdAfterItsTrackedEvacuationEnded(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 30, 0, 0, time.UTC)
	arrivesAt := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Stationing["autoStation:10"] = State.StationingOperation{
		ID: "autoStation:10", Purpose: "autoStation", SourceCastleID: 10, TargetCastleID: 20,
		MovementID: 50, UpdatedAt: now.Add(-time.Hour),
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20,
		MovementID: 51, UpdatedAt: now,
	}
	gameState.Movements[51] = State.MovementState{
		ID: 51, Direction: 0, SourceCastleID: 10, TargetCastleID: 20,
		TravelSeconds: 60, WaitSeconds: 6 * 3600, ArrivesAt: &arrivesAt,
	}

	decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "armed" {
		t.Fatalf("Auto Station borrowed Auto Bird movement = %#v err=%v", decision, err)
	}
}

func TestAutoStationDoesNotRecallSameRouteBirdWithoutTrackedMovementID(t *testing.T) {
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(time.Minute)
	safeAfter := now.Add(-time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.MovementSnapshot.ObservedAt = now
	gameState.Stationing["autoStation:10"] = State.StationingOperation{
		ID: "autoStation:10", Purpose: "autoStation", SourceCastleID: 10, TargetCastleID: 20,
		SafeAfter: &safeAfter, UpdatedAt: now.Add(-time.Hour),
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20,
		MovementID: 51, UpdatedAt: now,
	}
	gameState.Movements[51] = State.MovementState{
		ID: 51, Direction: 0, SourceCastleID: 10, TargetCastleID: 20,
		TravelSeconds: 60, WaitSeconds: 6 * 3600, ArrivesAt: &arrivesAt,
	}

	decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "armed" {
		t.Fatalf("untracked Auto Station record borrowed Auto Bird movement = %#v err=%v", decision, err)
	}
}

func autoBirdEligibleTestState(t *testing.T, now time.Time) (State.GameState, *GameData.Store) {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	gameState.Alliance = State.AllianceState{
		ID: 9, ObservedAt: now,
		Members: []State.AllianceMember{{PlayerID: 1, ReturnProtectionSec: 4 * 86_400}},
		Holdings: []State.AllianceHolding{{
			CastleID: 20, PlayerID: 1, KingdomID: 0, X: 20, Y: 20, SlotType: 1,
		}},
	}
	return gameState, gameData
}

func TestAutoStationRefreshesStaleAllianceRosterBeforeEvacuating(t *testing.T) {
	now := time.Now().UTC()
	arrives := now.Add(30 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 7
	gameState.Alliance.ID = 9
	gameState.Castles[100] = State.CastleState{ID: 100, SlotType: 4}
	gameState.Movements[1] = State.MovementState{
		ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Status != "threat" || decision.Request == nil || decision.Request.Name != "alliance.refresh" {
		t.Fatalf("stale alliance refresh = %#v, err=%v", decision, err)
	}
}

func TestAutoStationUsesOptInOpenGateFallbackAfterStationFailure(t *testing.T) {
	now := time.Date(2026, 7, 29, 17, 0, 0, 0, time.UTC)
	arrives := now.Add(30 * time.Second)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.ID = 7
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{ModeState: -1, ObservedAt: now}
	gameState.Alliance = State.AllianceState{
		ID: 9, ObservedAt: now,
		Members: []State.AllianceMember{{PlayerID: 1, ReturnProtectionSec: 4 * 86_400}},
		Holdings: []State.AllianceHolding{{
			CastleID: 20, PlayerID: 1, KingdomID: 0, X: 20, Y: 20, SlotType: 1,
		}},
	}
	gameState.Castles[100] = State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 4, X: 10, Y: 10, Name: "Outpost",
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	gameState.Movements[1] = State.MovementState{
		ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoStation": json.RawMessage(`{"openGateFallback":true}`),
	}}

	decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.station" {
		t.Fatalf("Auto Station evacuation decision = %#v err=%v", decision, err)
	}
	if decision.FailureFallback == nil || decision.FailureFallback.Name != "defense.open_gate" {
		t.Fatalf("Auto Station failure fallback = %#v", decision.FailureFallback)
	}
	var fallback struct {
		CastleID              State.CastleID `json:"castleId"`
		RequireIncomingAttack bool           `json:"requireIncomingAttack"`
		RequireProtectionMode bool           `json:"requireProtectionMode"`
	}
	if err := json.Unmarshal(decision.FailureFallback.Arguments, &fallback); err != nil {
		t.Fatal(err)
	}
	if fallback.CastleID != 100 || !fallback.RequireIncomingAttack || fallback.RequireProtectionMode {
		t.Fatalf("Open Gate failure fallback = %+v", fallback)
	}

	decision, err = NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.FailureFallback != nil {
		t.Fatalf("non-opt-in Auto Station failure fallback = %#v err=%v", decision, err)
	}
}

func TestAutoStationUsesOnlyOpenGatesDuringPurchasedProtectionMode(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	for _, modeState := range []int{0, 1} {
		t.Run(string(rune('0'+modeState)), func(t *testing.T) {
			arrives := now.Add(30 * time.Second)
			gameState := State.NewGameState()
			gameState.Player.ID = 7
			gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
				ModeState: modeState, RemainingSec: 3600, ObservedAt: now,
			}
			gameState.Alliance.ID = 9
			gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 4, Name: "Outpost"}
			gameState.Movements[1] = State.MovementState{
				ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
				SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
			}
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				"automation.autoStation": json.RawMessage(`{"openGateFallback":true}`),
			}}
			decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, Now: now,
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "defense.open_gate" {
				t.Fatalf("Protection Mode Auto Station decision = %#v err=%v", decision, err)
			}
			var request struct {
				CastleID              State.CastleID `json:"castleId"`
				RequireIncomingAttack bool           `json:"requireIncomingAttack"`
				RequireProtectionMode bool           `json:"requireProtectionMode"`
			}
			if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
				t.Fatal(err)
			}
			if request.CastleID != 100 || !request.RequireIncomingAttack || !request.RequireProtectionMode {
				t.Fatalf("Open Gate request = %+v", request)
			}
		})
	}
}

func TestAutoStationNeverFallsBackToStationingWithoutOpenGateOptIn(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	arrives := now.Add(30 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 7
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, RemainingSec: 3600, ObservedAt: now,
	}
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 4}
	gameState.Movements[1] = State.MovementState{
		ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "threat" {
		t.Fatalf("Open Gate opt-in decision = %#v err=%v", decision, err)
	}
}

func TestAutoStationCountsOnlyGameReportedGateDurationBeyondDefense(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	arrives := now.Add(30 * time.Second)
	for _, test := range []struct {
		name          string
		gateOpenUntil time.Time
		wantStatus    string
	}{
		{name: "past final attack", gateOpenUntil: arrives.Add(time.Second), wantStatus: "protected"},
		{name: "equal to final attack", gateOpenUntil: arrives, wantStatus: "threat"},
		{name: "before final attack", gateOpenUntil: arrives.Add(-time.Second), wantStatus: "threat"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Player.ID = 7
			gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
				ModeState: 1, RemainingSec: 3600, ObservedAt: now,
			}
			gameState.Castles[100] = State.CastleState{
				ID: 100, KingdomID: 0, SlotType: 4,
				Defense: State.CastleDefenseState{OpenGateUntil: &test.gateOpenUntil},
			}
			gameState.Movements[1] = State.MovementState{
				ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
				SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
			}
			decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
			if err != nil || decision.Request != nil || decision.Status != test.wantStatus {
				t.Fatalf("game-reported Open Gate decision = %#v err=%v", decision, err)
			}
		})
	}
}

func TestIncomingThreatsOnlyIncludeHostileAttacksOnOwnedCastles(t *testing.T) {
	now := time.Now().UTC()
	arrives := now.Add(90 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 7
	gameState.Castles[100] = State.CastleState{ID: 100, SlotType: 4}
	gameState.Movements[1] = State.MovementState{
		ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	gameState.Movements[2] = State.MovementState{
		ID: 2, TypeID: 0, Direction: 0, OwnerPlayerID: 7, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 100, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	gameState.Movements[3] = State.MovementState{
		ID: 3, TypeID: 2, Direction: 1, OwnerPlayerID: 7, TargetPlayerID: 7,
		SourceTypeID: 2, SourceCastleID: -1, TargetTypeID: 4, TargetCastleID: 100, ReturnsAt: &arrives,
	}
	gameState.Movements[4] = State.MovementState{
		ID: 4, TypeID: 0, Direction: 0, TargetPlayerID: 7,
		SourceTypeID: 2, SourceCastleID: -2, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	gameState.Movements[5] = State.MovementState{
		ID: 5, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetCastleID: 100, ArrivesAt: &arrives,
	}
	threats, count, earliest, latest := incomingThreats(gameState, now)
	if count != 1 || len(threats) != 1 || !earliest.Equal(arrives) || !latest.Equal(arrives) {
		t.Fatalf("unexpected threats: count=%d threats=%#v earliest=%v latest=%v", count, threats, earliest, latest)
	}
}
