package App

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestKingdomTroopShipmentUsesCapturedKutShape(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[10] = 20
	donor.Units.Stationed[20] = 30
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"units":[{"unitId":20,"amount":7},{"unitId":10,"amount":5}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "kpi" ||
		plan.Steps[1].Action != "kingdom.transport.verify_available" || plan.Steps[2].Opcode != "kut" ||
		plan.Steps[3].Action != "troops.kingdom.consume_source" {
		t.Fatalf("unexpected troop transfer steps: %#v", plan.Steps)
	}
	if got := string(plan.Steps[2].Command.Payload); got != `{"SCID":10,"SKID":0,"TKID":4,"CID":-1,"A":[[10,5],[20,7]]}` {
		t.Fatalf("KUT payload = %s", got)
	}
}

func TestKingdomTroopShipmentRejectsToolsAndSkipUsesTroopTransportType(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[30] = 10
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,"units":[{"unitId":30,"amount":1}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "tool") {
		t.Fatalf("tool transfer error = %v", err)
	}

	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{KingdomID: 4, RemainingSec: 3600}}
	gameState.Player.Currencies[1005] = 2
	plan, err := planKingdomTroopSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"targetKingdomId":4,"timeSkipId":"MS5"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(plan.Steps[0].Command.Payload); got != `{"KID":"4","MST":"MS5","TT":"1"}` {
		t.Fatalf("troop time-skip payload = %s", got)
	}
	if plan.Summary != "Apply a 1-hour time skip to kingdom 4 troop transport" {
		t.Fatalf("troop time-skip summary = %q", plan.Summary)
	}
	if !slices.Contains(plan.Claims, "currency:1005") {
		t.Fatalf("troop time-skip plan is missing its currency claim: %#v", plan.Claims)
	}
}

func TestKingdomTroopShipmentRejectsStormBelowMeadFloor(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Resources[12] = State.ResourceBalance{Amount: GameData.StormTroopSupportMead - 1}
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,"units":[{"unitId":10,"amount":1}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "at least 50000 Mead") {
		t.Fatalf("low Storm Mead error = %v", err)
	}
}

func TestKingdomTroopShipmentCapCountsAwayTargetTroops(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Units.Stationed[10] = 2
	target.Units.Traveling[10] = 6
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.Movements[99] = State.MovementState{
		ID: 99, SourceCastleID: target.ID, KingdomID: target.KingdomID,
		Units: map[State.UnitID]int64{10: 6},
	}
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"maximumTargetTroops":10,"units":[{"unitId":10,"amount":3}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "above its 10-troop import cap") ||
		!strings.Contains(err.Error(), "8 committed, 3 incoming") {
		t.Fatalf("away-target troop cap error = %v", err)
	}
}

func TestKingdomTroopShipmentAddsExecutionTimeCapGuard(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Units.Stationed[10] = 2
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"maximumTargetTroops":10,"units":[{"unitId":10,"amount":3}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[2].Action != "troops.kingdom.guard_target_cap" ||
		plan.Steps[3].Opcode != "kut" {
		t.Fatalf("capped troop transfer steps = %#v", plan.Steps)
	}
}

func kingdomTroopIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":10},{"wodID":20},{"wodID":30,"slotTypes":"1,2"}],
		"resources":[{"resourceID":12,"JSONKey":"MEAD"}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"1005","MinutesSkipValue":"60"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func kingdomTroopIntentCastle(id State.CastleID, kingdom State.KingdomID, name string) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, Name: name,
		Resources:           map[State.ResourceID]State.ResourceBalance{12: {Amount: GameData.StormTroopSupportMead}},
		FoodStateObservedAt: time.Now().UTC(),
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{}, Traveling: map[State.UnitID]int64{},
			Hospital: map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{}, Total: map[State.UnitID]int64{},
		},
	}
}
