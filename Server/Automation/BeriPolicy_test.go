package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func TestBeriPolicyRefreshesThenTransfersExactCapacity(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoBeriWorld": json.RawMessage(`{"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,"minTroopsToTransfer":20,"troopSpaceCheckIntervalSec":30}`),
	}}
	policy := NewBeriPolicy()

	decision, err := policy.Evaluate(t.Context(), Snapshot{State: gameState, Configuration: configuration, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}

	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
	}
	decision, err = policy.Evaluate(t.Context(), Snapshot{State: gameState, Configuration: configuration, Now: now.Add(time.Second)})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("transfer decision: %#v err=%v", decision, err)
	}
	var arguments map[string]any
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil || arguments["amount"] != float64(25) {
		t.Fatalf("transfer did not freeze exact capacity: %s", decision.Request.Arguments)
	}
}
