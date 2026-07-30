package Automation

import (
	"encoding/json"
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
			ObjectID: 70, Level: 70, ObservedAt: now,
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
			ObjectID: 70, Level: 70, ObservedAt: now,
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
