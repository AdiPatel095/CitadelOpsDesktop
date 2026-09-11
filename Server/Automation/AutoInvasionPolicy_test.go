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
	"CitadelDesktop/Server/State"
)

func TestAutoInvasionPolicyWaitsForEnoughCapacityAdjustedInventory(t *testing.T) {
	now := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"eventAutoScalingDifficulties":[
			{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0},
			{"difficultyID":108,"eventID":103,"difficultyTypeID":1,"isLocked":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 1}},
	}
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = foreignLordsEventID
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, DifficultyID: 8, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Invasion.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	snapshot := Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoInvasion": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"trial",
				"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
				"scoreTarget":5000000,"minimumRemainingSec":1800,
				"checkIntervalSec":30,"mapRefreshIntervalSec":300
			}`),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
		}},
	}

	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "Waiting for attack inventory") {
		t.Fatalf("invasion shortage decision: %#v err=%v", decision, err)
	}

	castle := snapshot.State.Castles[1]
	castle.Units.Stationed[216] = 2_000
	snapshot.State.Castles[1] = castle
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("invasion ready decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnStale {
		t.Fatal("Auto Invasion does not immediately rotate after a stale launch plan")
	}

	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(10 * time.Minute)
	snapshot.State.Player.ID = 1
	snapshot.State.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 1,
		CommanderID: &commanderID, ArrivesAt: &arrivesAt,
	}
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "No commander") {
		t.Fatalf("busy invasion commander decision: %#v err=%v", decision, err)
	}
	delete(snapshot.State.Movements, 50)

	snapshot.State.EventScores.ActiveEventID = bloodcrowEventID
	snapshot.State.EventScores.ByEvent[bloodcrowEventID] = State.ScalableEventScore{
		EventID: bloodcrowEventID, DifficultyID: 108, RemainingSec: 7_200, ObservedAt: now,
	}
	snapshot.State.Invasion.FortifyCurrencies = []string{"GTO", "STO", "ST"}
	snapshot.State.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: bloodcrowMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	snapshot.Configuration.Sections["automation.autoInvasion"] = json.RawMessage(`{
		"version":1,"sourceCastleId":1,"presetId":"trial",
		"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
		"scoreTarget":5000000,"minimumRemainingSec":1800,
		"checkIntervalSec":30,"mapRefreshIntervalSec":300,"fortifyCurrency":"KM"
	}`)
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("Bloodcrow invasion decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		FortifyCurrency string `json:"fortifyCurrency"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.FortifyCurrency != "ST" {
		t.Fatalf("Bloodcrow fortification arguments = %+v err=%v", arguments, err)
	}
}

func TestInvasionCapacityShortageCountsOnlyTargetAvailableWaves(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216},{"wodID":217}],"buildings":[],
		"effects":[{"effectID":700,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":99}],
		"effectCaps":[{"capID":99}],"legendskills":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	source := State.CastleState{
		ID: 1, KingdomID: 0,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 4, 217: 2}},
	}
	gameState.Castles[1] = source
	gameState.Commanders[7] = State.CommanderState{
		ID: 7, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 1001},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 700, Values: []float64{2}}},
	}
	gameState.Player.LegendSkills.ObservedAt = now
	unitID, supportUnitID := int64(216), int64(217)
	wave := AttackPresets.Wave{
		Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 1}}},
	}
	preset := AttackPresets.Preset{
		Waves: []AttackPresets.Wave{wave, wave, wave, wave, wave},
		CourtyardSupport: AttackPresets.CourtyardSupport{
			Troops: []AttackPresets.Slot{{ItemID: &supportUnitID, Quantity: 10}},
		},
	}
	target := State.MapObservation{KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100, ObjectID: 70, Level: 70}
	snapshot := Snapshot{State: gameState, GameData: gameData, Now: now}

	_, required, available, shortage, err := invasionCapacityShortage(snapshot, source, target, preset, 7)
	if err != nil || shortage {
		t.Fatalf("four target-available waves reported shortage: required=%d available=%d shortage=%t err=%v", required, available, shortage, err)
	}

	source.Units.Stationed[216] = 3
	_, required, available, shortage, err = invasionCapacityShortage(snapshot, source, target, preset, 7)
	if err != nil || !shortage || required != 4 || available != 3 {
		t.Fatalf("target wave shortage: required=%d available=%d shortage=%t err=%v", required, available, shortage, err)
	}

	source.Units.Stationed[216] = 4
	source.Units.Stationed[217] = 1
	itemID, required, available, shortage, err := invasionCapacityShortage(snapshot, source, target, preset, 7)
	if err != nil || !shortage || itemID != 217 || required != 2 || available != 1 {
		t.Fatalf("target support shortage: item=%d required=%d available=%d shortage=%t err=%v", itemID, required, available, shortage, err)
	}
}

func TestInvasionPresetShortageCombinesOfficialTroopFamilyInventory(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"effects":[],"effectCaps":[],
		"units":[
			{"wodID":3,"upgradeWodID":4},
			{"wodID":4,"downgradeWodID":3}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	anchor := int64(3)
	preset := AttackPresets.Preset{
		UseTroopFamilies: true,
		Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &anchor, Quantity: 100}}},
		}},
	}
	source := State.CastleState{
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{3: 60, 4: 40}},
	}

	_, required, available, shortage, err := invasionPresetShortage(preset, source, gameData)
	if err != nil || shortage {
		t.Fatalf("combined family inventory reported shortage: required=%d available=%d shortage=%t err=%v", required, available, shortage, err)
	}

	preset.UseTroopFamilies = false
	itemID, required, available, shortage, err := invasionPresetShortage(preset, source, gameData)
	if err != nil || !shortage || itemID != 3 || required != 100 || available != 60 {
		t.Fatalf("exact troop inventory shortage: item=%d required=%d available=%d shortage=%t err=%v", itemID, required, available, shortage, err)
	}
}

func TestInvasionFortifyCurrencyUsesVariantSpecificEventMedals(t *testing.T) {
	for _, test := range []struct {
		name     string
		setting  string
		eventID  int64
		offered  []string
		expected string
		valid    bool
	}{
		{name: "legacy Khan selection in Foreign Lords", setting: "KM", eventID: foreignLordsEventID, expected: "KM", valid: true},
		{name: "legacy Khan selection in Bloodcrow", setting: "KM", eventID: bloodcrowEventID, expected: "ST", valid: true},
		{name: "event medals in Foreign Lords", setting: eventMedalsCurrency, eventID: foreignLordsEventID, expected: "KM", valid: true},
		{name: "event medals in Bloodcrow", setting: eventMedalsCurrency, eventID: bloodcrowEventID, expected: "ST", valid: true},
		{name: "live currencies override Foreign Lords fallback", setting: eventMedalsCurrency, eventID: foreignLordsEventID, offered: []string{"GTO", "STO", "ST"}, expected: "ST", valid: true},
		{name: "live currencies override Bloodcrow fallback", setting: eventMedalsCurrency, eventID: bloodcrowEventID, offered: []string{"GTO", "STO", "KM"}, expected: "KM", valid: true},
		{name: "server supplied event currency overrides legacy codes", setting: eventMedalsCurrency, eventID: bloodcrowEventID, offered: []string{"GTO", "STO", "KT"}, expected: "KT", valid: true},
		{name: "live currencies without event medals fail closed", setting: eventMedalsCurrency, eventID: foreignLordsEventID, offered: []string{"GTO", "STO"}, valid: false},
		{name: "gold shared by both variants", setting: "GTO", eventID: bloodcrowEventID, expected: "GTO", valid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			currency, valid := invasionFortifyCurrencyForEvent(test.setting, test.eventID, test.offered)
			if valid != test.valid || currency != test.expected {
				t.Fatalf("fortification currency = %q valid=%t, want %q valid=%t", currency, valid, test.expected, test.valid)
			}
		})
	}
}

func TestAutoInvasionPolicyPausesThroughoutPurchasedProtectionMode(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	for _, modeState := range []int{0, 1} {
		gameState := State.NewGameState()
		gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
			ModeState: modeState, RemainingSec: 3_600, ObservedAt: now,
		}
		decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, Now: now,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
				"automation.autoInvasion": json.RawMessage(`{
					"version":1,"sourceCastleId":1,"presetId":"trial",
					"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
					"scoreTarget":5000000
				}`),
			}},
		})
		if err != nil || decision.Request != nil || decision.Status != "protected" ||
			decision.Detail != "Protection Mode is preparing or active; Auto Invasion attacks are paused" ||
			!decision.NextCheckAt.Equal(now.Add(playerProtectionRefreshInterval)) {
			t.Fatalf("mode state %d protection decision: %#v err=%v", modeState, decision, err)
		}
	}

	wakesForProtection := false
	for _, domain := range NewAutoInvasionPolicy().WakeDomains() {
		if domain == "player-protection" {
			wakesForProtection = true
			break
		}
	}
	if !wakesForProtection {
		t.Fatal("Auto Invasion does not wake when purchased Protection Mode changes")
	}
}

func TestAutoInvasionRefreshesProtectionAfterToggleOrStaleObservation(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name                 string
		observedAt           time.Time
		configurationChanged bool
	}{
		{name: "toggle", observedAt: now, configurationChanged: true},
		{name: "stale", observedAt: now.Add(-playerProtectionRefreshInterval)},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
				ModeState: -1, ObservedAt: test.observedAt,
			}
			gameState.Castles[1] = State.CastleState{
				ID: 1, KingdomID: 0, X: 123, Y: 456, Focused: true,
			}
			decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Now: now, PolicyConfigurationChanged: test.configurationChanged,
				Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
					"automation.autoInvasion": json.RawMessage(`{
						"version":1,"sourceCastleId":1,"presetId":"trial",
						"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
						"scoreTarget":5000000
					}`),
				}},
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "map.query" ||
				decision.Status != "refreshing" || !decision.ReevaluateOnSuccess {
				t.Fatalf("protection refresh decision = %#v err=%v", decision, err)
			}
		})
	}
}

func TestAutoInvasionRefreshesGeneralSkillsInsteadOfWaitingForever(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"generalSkills":[],
		"eventAutoScalingDifficulties":[{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 2_000}},
	}
	// The commander has a general assigned whose skills were never observed —
	// the live hosted deadlock ("general 125 skills have not been observed").
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true, GeneralID: 125}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = foreignLordsEventID
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, DifficultyID: 8, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Invasion.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	snapshot := Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoInvasion": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"trial",
				"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
				"scoreTarget":5000000,"minimumRemainingSec":1800,
				"checkIntervalSec":30,"mapRefreshIntervalSec":300
			}`),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
		}},
	}

	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "general.skills.refresh" || !decision.ReevaluateOnSuccess {
		t.Fatalf("unobserved general must schedule a skills refresh, got %#v", decision)
	}

	// Once the roster is observed the same evaluation resolves capacity and
	// proceeds to the attack instead of re-requesting the refresh.
	snapshot.State.Generals[125] = State.GeneralState{ID: 125, ObservedAt: now}
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("observed general should let the attack plan through, got %#v", decision)
	}
}

func TestAutoInvasionRefreshesNeighborhoodBeforePickingStaleTarget(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"eventAutoScalingDifficulties":[{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 2_000}},
	}
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = foreignLordsEventID
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, DifficultyID: 8, RemainingSec: 7_200, ObservedAt: now,
	}
	// The full sweep ran 4 minutes ago and the best candidate is from it — a
	// castle other players may have defeated since. The policy must refresh the
	// neighborhood before committing an attack to it.
	lastScan := now.Add(-4 * time.Minute)
	gameState.Invasion.LastScannedAt[1] = lastScan
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: lastScan,
		},
	}
	snapshot := Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoInvasion": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"trial",
				"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
				"scoreTarget":5000000,"minimumRemainingSec":1800,
				"checkIntervalSec":30,"mapRefreshIntervalSec":300
			}`),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
		}},
	}

	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "invasion.map.scan" || !decision.ReevaluateOnSuccess {
		t.Fatalf("stale candidate must trigger a neighborhood refresh first, got %#v", decision)
	}
	var arguments struct {
		Bounds *State.StormMapBounds `json:"bounds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.Bounds == nil {
		t.Fatalf("neighborhood refresh must carry bounds: %s err=%v", decision.Request.Arguments, err)
	}
	if arguments.Bounds.X1 != 101-invasionNeighborhoodHalfSize || arguments.Bounds.X2 != 101+invasionNeighborhoodHalfSize ||
		arguments.Bounds.Y1 != 100-invasionNeighborhoodHalfSize || arguments.Bounds.Y2 != 100+invasionNeighborhoodHalfSize {
		t.Fatalf("neighborhood bounds should be centered on the candidate, got %+v", *arguments.Bounds)
	}

	// Once the game confirmed the castle seconds ago, the same evaluation
	// commits the attack.
	target := gameState.Map[0]["101:100"]
	target.ObservedAt = now.Add(-10 * time.Second)
	gameState.Map[0]["101:100"] = target
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("fresh candidate should be attacked directly, got %#v", decision)
	}
}

func TestAutoInvasionRotatesPastBusySettlingAndUnavailableTargets(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		prepare func(*State.GameState)
	}{
		{
			name: "outbound movement",
			prepare: func(state *State.GameState) {
				arrivesAt := now.Add(time.Minute)
				state.Movements[1] = State.MovementState{
					ID: 1, Direction: 0, KingdomID: 0, TargetX: 101, TargetY: 100,
					ArrivesAt: &arrivesAt, TravelSeconds: 60,
				}
			},
		},
		{
			name: "return movement",
			prepare: func(state *State.GameState) {
				returnsAt := now.Add(time.Minute)
				state.Movements[2] = State.MovementState{
					ID: 2, Direction: 1, TypeID: 2, KingdomID: 0,
					SourceTypeID: foreignLordsMapTypeID, SourceX: 101, SourceY: 100,
					TargetCastleID: 1, TargetX: 100, TargetY: 100, ReturnsAt: &returnsAt,
				}
			},
		},
		{
			name: "outbound movement from target",
			prepare: func(state *State.GameState) {
				arrivesAt := now.Add(time.Minute)
				state.Movements[3] = State.MovementState{
					ID: 3, Direction: 0, KingdomID: 0,
					SourceTypeID: foreignLordsMapTypeID, SourceX: 101, SourceY: 100,
					TargetTypeID: 2, TargetX: 100, TargetY: 100, ArrivesAt: &arrivesAt,
				}
			},
		},
		{
			name: "hidden target",
			prepare: func(state *State.GameState) {
				state.Invasion.MarkTargetUnavailable(0, 101, 100, now)
			},
		},
		{
			name: "protected target",
			prepare: func(state *State.GameState) {
				target := state.Map[0]["101:100"]
				target.InvasionProtected = true
				state.Map[0]["101:100"] = target
			},
		},
		{
			name: "pending settlement",
			prepare: func(state *State.GameState) {
				state.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
					MovementID: 3, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: 0,
					TargetTypeID: foreignLordsMapTypeID, TargetX: 101, TargetY: 100,
					LaunchedAt: now, ArrivesAt: now.Add(time.Minute),
				}}
			},
		},
		{
			name: "unresolved dispatch reservation",
			prepare: func(state *State.GameState) {
				state.Commanders[1] = State.CommanderState{ID: 1, Available: true}
				reservedAt := now
				state.Invasion.ReserveTarget(State.InvasionTargetReservation{
					KingdomID: 0, EventID: foreignLordsEventID,
					OccurrenceEndsAt: State.ScalableEventEndsAt(state.EventScores.ByEvent[foreignLordsEventID]),
					TargetTypeID:     foreignLordsMapTypeID, X: 101, Y: 100,
					SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
					OperationID: "indeterminate-cra", ReservedAt: reservedAt,
					ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
				})
			},
		},
		{
			name: "unconfirmed target",
			prepare: func(state *State.GameState) {
				target := state.Map[0]["101:100"]
				target.InvasionAvailabilityKnown = false
				state.Map[0]["101:100"] = target
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := autoInvasionRotationSnapshot(t, now)
			test.prepare(&snapshot.State)
			decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
			if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
				t.Fatalf("rotation decision: %#v err=%v", decision, err)
			}
			var request struct {
				TargetX int `json:"targetX"`
				TargetY int `json:"targetY"`
			}
			if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil ||
				request.TargetX != 102 || request.TargetY != 100 {
				t.Fatalf("rotated target = %+v, arguments=%s err=%v", request, decision.Request.Arguments, err)
			}
		})
	}
}

func TestAutoInvasionIdlesWhenEveryKnownTargetIsBlocked(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	snapshot := autoInvasionRotationSnapshot(t, now)
	arrivesAt := now.Add(time.Minute)
	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, KingdomID: 0, TargetTypeID: foreignLordsMapTypeID,
		TargetX: 101, TargetY: 100, ArrivesAt: &arrivesAt, TravelSeconds: 60,
	}
	snapshot.State.Invasion.MarkTargetUnavailable(0, 102, 100, now)

	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "idle" ||
		decision.Metrics["busyTargets"] != 1 || decision.Metrics["unavailableTargets"] != 1 ||
		!strings.Contains(decision.Detail, "No attackable invasion castle") {
		t.Fatalf("all-blocked decision: %#v err=%v", decision, err)
	}
}

func TestAutoInvasionRefreshesLegacyUnconfirmedTargetsWithoutWaitingForFullSweep(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	snapshot := autoInvasionRotationSnapshot(t, now)
	snapshot.State.Invasion.LastScannedAt[1] = now.Add(-2 * time.Minute)
	for key, target := range snapshot.State.Map[0] {
		target.InvasionAvailabilityKnown = false
		target.ObservedAt = now.Add(-2 * time.Minute)
		snapshot.State.Map[0][key] = target
	}
	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.map.scan" || !decision.ReevaluateOnSuccess {
		t.Fatalf("legacy availability migration decision: %#v err=%v", decision, err)
	}
	if !strings.Contains(decision.Detail, "confirm attack availability") {
		t.Fatalf("legacy availability refresh detail = %q", decision.Detail)
	}
}

func TestInvasionCandidatePoolDoesNotAssumeExactlyTenTargets(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	source := State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	state.Castles[1] = source
	state.Map[0] = map[string]State.MapObservation{}
	for index := 0; index < 12; index++ {
		x := 101 + index
		state.Map[0][fmt.Sprintf("%d:100", x)] = State.MapObservation{
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: x, Y: 100,
			Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		}
	}
	if candidates := invasionCandidatePool(state, source, foreignLordsMapTypeID, fixedInvasionRadius, now); len(candidates) != 12 {
		t.Fatalf("candidate pool len=%d, want all 12 protocol-returned targets", len(candidates))
	}
}

func TestAutoInvasionDoesNotReuseTargetReservedByPriorEvent(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	snapshot := autoInvasionRotationSnapshot(t, now)
	reservedAt := now
	snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: bloodcrowEventID, OccurrenceEndsAt: now.Add(-7 * 24 * time.Hour),
		TargetTypeID: bloodcrowMapTypeID, X: 101, Y: 100,
		SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
		OperationID: "prior-event", ReservedAt: reservedAt,
		ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
	})
	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("event-rollover decision: %#v err=%v", decision, err)
	}
	var request struct {
		TargetX int `json:"targetX"`
		TargetY int `json:"targetY"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || request.TargetX != 102 || request.TargetY != 100 {
		t.Fatalf("prior event reservation was reused before reconciliation: request=%+v err=%v", request, err)
	}
}

func TestAutoInvasionReconcilesMatchedMovementWithinGrace(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	snapshot := autoInvasionRotationSnapshot(t, now)
	reservedAt := now.Add(-5 * time.Second)
	snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: foreignLordsEventID,
		OccurrenceEndsAt: State.ScalableEventEndsAt(snapshot.State.EventScores.ByEvent[foreignLordsEventID]),
		TargetTypeID:     foreignLordsMapTypeID,
		X:                101, Y: 100, SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
		OperationID: "uncertain-cra", ReservedAt: reservedAt,
		ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
	})
	decision, err := NewInvasionRecoveryPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "game.refresh_movements" {
		t.Fatalf("young reservation recovery decision: %#v err=%v", decision, err)
	}

	arrivesAt := now.Add(time.Minute)
	commanderID := State.CommanderID(0)
	snapshot.State.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: foreignLordsMapTypeID, TargetX: 101, TargetY: 100,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now, ArrivesAt: &arrivesAt, TravelSeconds: 60,
	}
	decision, err = NewInvasionRecoveryPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("movement-confirmed reservation was not reconciled into accounting: %#v err=%v", decision, err)
	}
	var request struct {
		MatchedMovementID State.MovementID `json:"matchedMovementId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || request.MatchedMovementID != 9 {
		t.Fatalf("movement-confirmed reconciliation boundary = %+v err=%v", request, err)
	}
}

func TestAutoInvasionRotatesPastReservationOccupiedByAnotherMovement(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	snapshot := autoInvasionRotationSnapshot(t, now)
	snapshot.State.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: foreignLordsEventID,
		OccurrenceEndsAt: State.ScalableEventEndsAt(snapshot.State.EventScores.ByEvent[foreignLordsEventID]),
		TargetTypeID:     foreignLordsMapTypeID,
		X:                101, Y: 100, SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
		OperationID: "uncertain-cra", ReservedAt: reservedAt,
		ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
	})
	arrivesAt := now.Add(time.Minute)
	otherCommanderID := State.CommanderID(7)
	snapshot.State.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 2, CommanderID: &otherCommanderID,
		KingdomID: 0, TargetTypeID: foreignLordsMapTypeID, TargetX: 101, TargetY: 100,
		ObservedAt: now, ArrivesAt: &arrivesAt, TravelSeconds: 60,
	}
	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("other movement target rotation: %#v err=%v", decision, err)
	}
	var request struct {
		TargetX int `json:"targetX"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || request.TargetX != 102 {
		t.Fatalf("other movement did not rotate to target 2: request=%+v err=%v", request, err)
	}
}

func TestAutoInvasionDeferredUnknownReservationRotatesWithoutReconcileLoop(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*State.MapObservation)
	}{
		{
			name: "unknown availability",
			mutate: func(target *State.MapObservation) {
				target.InvasionAvailabilityKnown = false
			},
		},
		{
			name: "short GAA row",
			mutate: func(target *State.MapObservation) {
				target.ObjectID = 0
				target.Level = 0
				target.InvasionAvailabilityKnown = false
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := autoInvasionRotationSnapshot(t, now)
			reservedAt := now.Add(-2 * State.InvasionTargetReservationReconcileGrace)
			snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: foreignLordsEventID,
				OccurrenceEndsAt: State.ScalableEventEndsAt(snapshot.State.EventScores.ByEvent[foreignLordsEventID]),
				TargetTypeID:     foreignLordsMapTypeID,
				X:                101, Y: 100, SourceCastleID: 1,
				OperationID: "deferred-unknown-cra", ReservedAt: reservedAt,
				ReconcileAfter: now.Add(State.InvasionTargetReservationReconcileGrace),
			})
			target := snapshot.State.Map[0]["101:100"]
			test.mutate(&target)
			target.ObservedAt = now
			snapshot.State.Map[0]["101:100"] = target

			decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
			if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
				t.Fatalf("deferred unknown reservation decision = %#v, err=%v", decision, err)
			}
			var request struct {
				TargetX int `json:"targetX"`
				TargetY int `json:"targetY"`
			}
			if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil ||
				request.TargetX != 102 || request.TargetY != 100 {
				t.Fatalf("deferred reservation did not rotate to the next candidate: request=%+v err=%v", request, err)
			}
			if decision.Metrics["unconfirmedTargets"] != 1 || decision.Metrics["reservedTargets"] != 0 {
				t.Fatalf("deferred unknown target metrics = %#v", decision.Metrics)
			}
		})
	}
}

func TestInvasionRecoveryReconcilesReservationAfterLaunchStopGates(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*State.ScalableEventScore)
	}{
		{name: "score target reached", mutate: func(score *State.ScalableEventScore) { score.PlayerScore = 5_000_000 }},
		{name: "event cutoff reached", mutate: func(score *State.ScalableEventScore) { score.RemainingSec = 1_800 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := autoInvasionRotationSnapshot(t, now)
			score := snapshot.State.EventScores.ByEvent[foreignLordsEventID]
			test.mutate(&score)
			snapshot.State.EventScores.ByEvent[foreignLordsEventID] = score
			reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
			snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: foreignLordsEventID,
				OccurrenceEndsAt: State.ScalableEventEndsAt(score), TargetTypeID: foreignLordsMapTypeID,
				X: 101, Y: 100, SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
				OperationID: "uncertain-cra", ReservedAt: reservedAt,
				ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
			})
			decision, err := NewInvasionRecoveryPolicy().Evaluate(t.Context(), snapshot)
			if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
				t.Fatalf("stop gate bypassed reservation reconciliation: %#v err=%v", decision, err)
			}
		})
	}
}

func TestInvasionRecoveryIsIndependentOfOrdinaryAutoInvasionGates(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{
			name: "no active event",
			mutate: func(snapshot *Snapshot) {
				snapshot.State.EventScores.ActiveEventID = 0
			},
		},
		{
			name: "Protection Mode active",
			mutate: func(snapshot *Snapshot) {
				snapshot.State.Player.ProtectionMode = State.PlayerProtectionModeState{
					ModeState: 1, RemainingSec: 3_600, ObservedAt: now,
				}
			},
		},
		{
			name: "missing settings",
			mutate: func(snapshot *Snapshot) {
				delete(snapshot.Configuration.Sections, "automation.autoInvasion")
			},
		},
		{
			name: "invalid settings",
			mutate: func(snapshot *Snapshot) {
				snapshot.Configuration.Sections["automation.autoInvasion"] = json.RawMessage(`{
					"version":1,"sourceCastleId":999,"presetId":"","scoreTarget":0
				}`)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := autoInvasionRotationSnapshot(t, now)
			reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
			occurrenceEndsAt := State.ScalableEventEndsAt(snapshot.State.EventScores.ByEvent[foreignLordsEventID])
			snapshot.State.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: foreignLordsEventID, OccurrenceEndsAt: occurrenceEndsAt,
				TargetTypeID: foreignLordsMapTypeID, X: 101, Y: 100,
				SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
				OperationID: "recovery-before-gates", ReservedAt: reservedAt,
				ReconcileAfter: reservedAt.Add(State.InvasionTargetReservationReconcileGrace),
			})
			test.mutate(&snapshot)

			decision, err := NewInvasionRecoveryPolicy().Evaluate(t.Context(), snapshot)
			if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" ||
				!decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
				t.Fatalf("due reservation did not preempt %s: decision=%#v err=%v", test.name, decision, err)
			}
			var request struct {
				SourceCastleID   State.CastleID `json:"sourceCastleId"`
				EventID          int64          `json:"eventId"`
				OccurrenceEndsAt time.Time      `json:"occurrenceEndsAt"`
				OperationID      string         `json:"operationId"`
				ReconcileAfter   time.Time      `json:"reconcileAfter"`
			}
			if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil ||
				request.SourceCastleID != 1 || request.EventID != foreignLordsEventID ||
				!request.OccurrenceEndsAt.Equal(occurrenceEndsAt) || request.OperationID != "recovery-before-gates" ||
				!request.ReconcileAfter.Equal(reservedAt.Add(State.InvasionTargetReservationReconcileGrace)) {
				t.Fatalf("recovery request lost reservation boundary: request=%+v err=%v", request, err)
			}
		})
	}
}

func TestActiveInvasionAttackCountCountsEveryCommanderAtSameTarget(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0}
	arrivesAt := now.Add(time.Minute)
	for movementID := State.MovementID(1); movementID <= 2; movementID++ {
		state.Movements[movementID] = State.MovementState{
			ID: movementID, Direction: 0, SourceCastleID: 1, KingdomID: 0,
			TargetTypeID: foreignLordsMapTypeID, TargetX: 101, TargetY: 100,
			ArrivesAt: &arrivesAt, TravelSeconds: 60,
		}
	}
	if count := activeInvasionAttackCount(state, 1, foreignLordsMapTypeID, now); count != 2 {
		t.Fatalf("active attack count=%d, want both commanders at the duplicated target", count)
	}
}

func autoInvasionRotationSnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"eventAutoScalingDifficulties":[{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 5_000}},
	}
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	state.Player.LegendSkills.ObservedAt = now
	state.EventScores.ActiveEventID = foreignLordsEventID
	state.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, DifficultyID: 8, RemainingSec: 7_200, ObservedAt: now,
	}
	state.Invasion.LastScannedAt[1] = now
	state.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
		"102:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 102, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	return Snapshot{
		State: state, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoInvasion": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"trial",
				"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
				"scoreTarget":5000000,"minimumRemainingSec":1800,
				"checkIntervalSec":30,"mapRefreshIntervalSec":300
			}`),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
		}},
	}
}
