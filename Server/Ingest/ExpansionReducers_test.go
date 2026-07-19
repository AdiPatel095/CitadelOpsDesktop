package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestExpansionResponseReconcilesNestedCastleAndResourcesWithoutRootKingdom(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],
		"resources":[{"resourceID":"3","JSONKey":"W","name":"wood"},{"resourceID":"4","JSONKey":"S","name":"stone"}],
		"buildings":[{"wodID":"200","name":"Ground","group":"Ground"},{"wodID":"201","name":"Expansion","group":"Ground"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	castle := newCastleState(5358)
	castle.KingdomID = 4
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	code := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ebe", ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{
			"NO":[201,41,220,220,1,0,4,100,-1,-1,0,0,0,0,-1,-1],
			"grc":{"AID":5358,"W":1677.935,"S":1681,"KID":4},
			"gca":{
				"A":[12,682,537,5358,17334928,1,2,2,2,0,"Castle Adolphus",0,0,-1,-1,-1,4,0,[],0],
				"BG":[
					[200,1,200,200,0,0,4,100,-1,-1,0,0,0,0,-1,-1],
					[201,41,220,220,1,0,4,100,-1,-1,0,0,0,0,-1,-1]
				],
				"BD":[],"T":[],"G":[],"D":[],"scl":{"OIDL":[-1,-2],"SSC":2}
			}
		}`),
	}
	domains, changed, err := combineReducers(reduceExpansionMutation, reduceResponseResources)(context.Background(), frame, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || len(domains) == 0 {
		t.Fatalf("expansion reducer changed=%v domains=%v", changed, domains)
	}
	castle = gameState.Castles[5358]
	if castle.KingdomID != 4 || len(castle.Layout.Ground) != 2 {
		t.Fatalf("expanded castle = %#v", castle)
	}
	ground, found := castle.Layout.Ground[41]
	if !found || ground.GridX != 220 || ground.GridY != 220 || ground.Rotation != 1 {
		t.Fatalf("expanded ground = %#v", ground)
	}
	if castle.Resources[3].Amount != 1677.935 || castle.Resources[4].Amount != 1681 {
		t.Fatalf("expanded resources = %#v", castle.Resources)
	}
	fallbackFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ebe", ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"NO":[201,42,230,220,1,0,4,100,-1,-1,0,0,0,0,-1,-1]}`),
	}
	_, changed, err = reduceExpansionMutation(context.Background(), fallbackFrame, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("compact expansion response changed=%v err=%v", changed, err)
	}
	if ground, found := gameState.Castles[5358].Layout.Ground[42]; !found || ground.GridX != 230 {
		t.Fatalf("compact expansion ground = %#v", ground)
	}
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	if !registry.HasInbound("ebe") {
		t.Fatal("ebe reducer is not registered")
	}
}
