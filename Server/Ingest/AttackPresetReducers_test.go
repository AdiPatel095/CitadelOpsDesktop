package Ingest

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestReduceAttackPresetsStoresSixLaneFormation(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	_, changed, err := reduceAttackPresets(t.Context(), Protocol.Frame{
		Opcode: "gas", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{
			"S":[
				{"S":7,"SN":"Tower flanks","A":"[[215,100],[215,1],[215,100],[513,10],[],[524,5]]"},
				{"S":2,"SN":"Empty","A":"[[],[],[],[],[],[]]"},
				{"S":8,"SN":"Invalid","A":"[[],[]]"}
			]
		}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("attack presets: changed=%t err=%v", changed, err)
	}
	if len(gameState.AttackPresets) != 2 || gameState.AttackPresets[0].Slot != 2 || gameState.AttackPresets[1].Slot != 7 {
		t.Fatalf("unexpected presets: %#v", gameState.AttackPresets)
	}
	preset := gameState.AttackPresets[1]
	if preset.Name != "Tower flanks" || preset.Units[0][0] != (State.AttackPresetStack{DefinitionID: 215, Amount: 100}) || preset.Units[2][0] != (State.AttackPresetStack{DefinitionID: 215, Amount: 100}) {
		t.Fatalf("unexpected unit lanes: %#v", preset)
	}
	if preset.Tools[0][0] != (State.AttackPresetStack{DefinitionID: 513, Amount: 10}) || len(preset.Tools[1]) != 0 || preset.Tools[2][0] != (State.AttackPresetStack{DefinitionID: 524, Amount: 5}) {
		t.Fatalf("unexpected tool lanes: %#v", preset)
	}
}
