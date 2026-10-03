package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestMainBuilderStorehouse132PayloadAndKingdomDispatchRecheck(t *testing.T) {
	raw, err := os.ReadFile("../GameData/testdata/default_building_kingdoms_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	// Add only synthetic geometry; public catalog records remain verbatim.
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var records []json.RawMessage
	json.Unmarshal(fixture["buildings"], &records)
	records = append(records, json.RawMessage(`{"wodID":200,"name":"Ground","group":"Ground","width":30,"height":30}`))
	fixture["buildings"], _ = json.Marshal(records)
	raw, _ = json.Marshal(fixture)
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	state := buildingIntentState()
	state.Player.Level = 70
	castle := state.Castles[10]
	castle.KingdomID = 0
	castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	castle.Resources = map[State.ResourceID]State.ResourceBalance{3: {Amount: 10000}, 4: {Amount: 10000}}
	state.Castles[10] = castle
	input := Intent.PlanningContext{State: state, GameData: data}
	args := json.RawMessage(`{"kind":"construct","request":{"castleId":10,"definitionId":132,"x":0,"y":0}}`)
	plan, err := planBuildingConstruct(context.Background(), input, json.RawMessage(`{"castleId":10,"definitionId":132,"x":0,"y":0}`))
	if err != nil || len(plan.Steps) == 0 {
		t.Fatalf("classic planning: %v", err)
	}
	step, err := resolveBuildingPlacementStep(context.Background(), input, args)
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "ebu" || string(step.Command.Payload) != `{"WID":132,"X":0,"Y":0,"R":0,"PWR":0,"PO":-1,"DOID":-1}` {
		t.Fatalf("eligible EBU golden changed: %s", step.Command.Payload)
	}
	if step.FinalDispatchAction != "building.placement.kingdom.guard" {
		t.Fatal("EBU has no final kingdom guard")
	}
	if err := validateFinalBuildingPlacementKingdom(input, step.FinalDispatchArguments); err != nil {
		t.Fatal(err)
	}
	castle.KingdomID = 10
	input.State.Castles[10] = castle
	if err := validateFinalBuildingPlacementKingdom(input, step.FinalDispatchArguments); !errors.Is(err, Intent.ErrPlanStale) || Localization.FromError(err) == nil {
		t.Fatalf("changed kingdom accepted or unlocalized: %v", err)
	}
	if _, err := resolveBuildingPlacementStep(context.Background(), input, args); err == nil {
		t.Fatal("Storehouse132 resolved in Berimond")
	}
	if _, err := planBuildingConstruct(context.Background(), input, json.RawMessage(`{"castleId":10,"definitionId":132,"x":0,"y":0}`)); err == nil {
		t.Fatal("Storehouse132 planned in Berimond")
	}
	if len(input.State.Automations) != 0 {
		t.Fatal("local guard created an automation state or lock")
	}
	factionArgs := json.RawMessage(`{"kind":"construct","request":{"castleId":10,"definitionId":246,"eventId":3,"x":0,"y":0}}`)
	faction, err := resolveBuildingPlacementStep(context.Background(), input, factionArgs)
	if err != nil {
		t.Fatal(err)
	}
	if string(faction.Command.Payload) != `{"WID":246,"X":0,"Y":0,"R":0,"PWR":0,"PO":-1,"DOID":-1}` {
		t.Fatalf("FactionStorage EBU changed: %s", faction.Command.Payload)
	}
	if err := validateFinalBuildingPlacementKingdom(input, faction.FinalDispatchArguments); err != nil {
		t.Fatal(err)
	}

}
