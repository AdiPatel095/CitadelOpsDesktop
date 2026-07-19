package App

import (
	"encoding/json"
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
	if len(plan.Steps) != 3 || plan.Steps[0].Opcode != "kpi" || plan.Steps[1].Opcode != "kut" || plan.Steps[2].Action != "troops.kingdom.consume_source" {
		t.Fatalf("unexpected troop transfer steps: %#v", plan.Steps)
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"SCID":10,"SKID":0,"TKID":4,"CID":-1,"A":[[10,5],[20,7]]}` {
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
}

func kingdomTroopIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":10},{"wodID":20},{"wodID":30,"slotTypes":"1,2"}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func kingdomTroopIntentCastle(id State.CastleID, kingdom State.KingdomID, name string) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, Name: name,
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{}, Traveling: map[State.UnitID]int64{},
			Hospital: map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{}, Total: map[State.UnitID]int64{},
		},
	}
}
