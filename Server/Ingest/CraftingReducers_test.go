package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCraftingSnapshotNormalizesQueuesAndEntitlements(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	code := 0
	payload := json.RawMessage(`{
		"CAI":[{
			"CBI":[{
				"KID":0,"AID":100,"OID":4107,"CQID":3,"S":4,"WID":3072,
				"QS":{"CRID":[347],"BV":[50],"RUT":[604754,604758,604760]},
				"PS":{"CRID":[347,348],"BV":[50,25],"RUT":[604751],"RCT":[17999,3600]}
			}],
			"CE":[[616,[347,348],"RH"],[377,[11,12],"BG"],[373,[3,50],"BG"]]
		}]
	}`)
	_, changed, err := reduceCraftingSnapshot(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "crin", ResponseCode: &code,
		Payload: payload, ReceivedAt: time.Unix(1000, 0).UTC(),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("crafting snapshot did not report a change")
	}
	crafting := gameState.Castles[100].Crafting
	building := crafting.Buildings[4107]
	if building.QueueTypeID != 3 || building.DefinitionID != 3072 {
		t.Fatalf("unexpected building: %#v", building)
	}
	if len(building.Active) != 2 || building.Active[0].RemainingSec == nil || *building.Active[0].RemainingSec != 17999 {
		t.Fatalf("unexpected active queue: %#v", building.Active)
	}
	if len(building.Queued) != 1 || building.Queued[0].RecipeID != 347 {
		t.Fatalf("unexpected queued recipes: %#v", building.Queued)
	}
	if len(crafting.EnabledRecipeIDs) != 2 || crafting.EnabledRecipeIDs[0] != 347 {
		t.Fatalf("unexpected recipe entitlements: %#v", crafting.EnabledRecipeIDs)
	}
	if crafting.OutputBoostByQueueType[3] != 50 {
		t.Fatalf("unexpected output boosts: %#v", crafting.OutputBoostByQueueType)
	}
}

func TestCraftingBuildingUpdateMergesOneQueue(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Crafting.Buildings[1] = State.CraftingBuilding{CastleID: 100, InstanceID: 1, DefinitionID: 200}
	gameState.Castles[100] = castle
	code := 0
	payload := json.RawMessage(`{
		"KID":0,"AID":100,"OID":2,"CQID":1,"S":4,"WID":2998,
		"QS":{"CRID":[],"BV":[],"RUT":[]},
		"PS":{"CRID":[11],"BV":[1],"RUT":[],"RCT":[80]}
	}`)
	_, changed, err := reduceCraftingBuilding(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "crst", ResponseCode: &code,
		Payload: payload, ReceivedAt: time.Now().UTC(),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || len(gameState.Castles[100].Crafting.Buildings) != 2 {
		t.Fatalf("crafting building was not merged: %#v", gameState.Castles[100].Crafting.Buildings)
	}
}
