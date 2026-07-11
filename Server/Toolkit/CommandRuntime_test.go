package Toolkit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
)

func TestCommandRuntimeDirectAPIUsesSharedAdmissionHarness(t *testing.T) {
	broker := Automation.NewCommandBroker()
	runtime, err := NewCommandRuntime(Automation.NewCommandHarness(broker))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := runtime.Catalog("jca", "")
	if err != nil || len(definitions) != 1 || definitions[0].Name != "jca" {
		t.Fatalf("catalog definitions=%+v err=%v", definitions, err)
	}

	result, err := runtime.Dispatch(context.Background(), CommandInvocation{
		Name:      "jca",
		Arguments: json.RawMessage(`{"castleId":424242,"kingdomId":2}`),
		Intent:    "focus_saved_castle",
		Options:   Automation.CommandOptions{Owner: Automation.OwnerManual},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.Effect != EffectGameQuery || result.Opcode != "jca" ||
		result.Command != "jca" || result.Intent != "focus_saved_castle" ||
		result.Surface != Automation.CommandSurfaceRuntime || result.CommandReceipt.Effect != string(EffectGameQuery) || len(result.Frames) != 1 {
		t.Fatalf("direct dispatch result=%+v", result)
	}
	queued := broker.Snapshot()[Automation.LaneCommand]
	if len(queued) != 1 || queued[0].SubmissionID != result.SubmissionID ||
		queued[0].Builder != "jca" || queued[0].Intent != "focus_saved_castle" ||
		queued[0].Surface != Automation.CommandSurfaceRuntime || queued[0].Effect != string(EffectGameQuery) {
		t.Fatalf("queued runtime command=%+v", queued)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"CID":424242`) {
		t.Fatalf("dispatch receipt exposed structured argument values: %s", encoded)
	}
	var portable map[string]interface{}
	if err := json.Unmarshal(encoded, &portable); err != nil {
		t.Fatal(err)
	}
	if portable["effect"] != string(EffectGameQuery) || portable["surface"] != Automation.CommandSurfaceRuntime {
		t.Fatalf("portable result lost effect/surface metadata: %s", encoded)
	}
}

func TestCommandRuntimePreservesMultiFrameCommandAsOneSubmission(t *testing.T) {
	broker := Automation.NewCommandBroker()
	runtime, err := NewCommandRuntime(Automation.NewCommandHarness(broker))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Dispatch(context.Background(), CommandInvocation{
		Name:      "upgrade_menu_refresh",
		Arguments: json.RawMessage(`{}`),
		Intent:    "refresh_loadout_context",
		Options:   Automation.CommandOptions{Owner: Automation.OwnerToolkit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.PayloadCount != 4 || len(result.Frames) != 4 {
		t.Fatalf("multi-frame result=%+v", result)
	}
	queued := broker.Snapshot()[Automation.LaneCommand]
	if len(queued) != 4 {
		t.Fatalf("queued frames=%d, want 4", len(queued))
	}
	for index, command := range queued {
		if command.SubmissionID != result.SubmissionID || command.FrameIndex != index ||
			command.Builder != "upgrade_menu_refresh" || command.Intent != "refresh_loadout_context" ||
			command.Opcode == "" || command.RequestShape == "" {
			t.Fatalf("queued frame %d lost runtime identity: %+v", index, command)
		}
	}
}

func TestDefaultHarnessCanExposeExistingCommandRuntime(t *testing.T) {
	runtime, err := NewCommandRuntime(Automation.NewCommandHarness(Automation.NewCommandBroker()))
	if err != nil {
		t.Fatal(err)
	}
	harness, err := NewDefaultHarnessWithRuntime(runtime)
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.preview",
		Arguments: json.RawMessage(`{"name":"jca","arguments":{"castleId":9,"kingdomId":2}}`),
	})
	if !result.OK || !strings.Contains(string(result.Content), "%jca%") {
		t.Fatalf("runtime-backed tool preview=%+v content=%s", result, result.Content)
	}
}

func TestCommandRuntimeDirectContextPlanning(t *testing.T) {
	gameState := Models.GetGameState()
	originalCastle := gameState.Castle.MainCastle
	originalFocus := gameState.CastleFocus
	defer func() {
		gameState.Castle.MainCastle = originalCastle
		gameState.CastleFocus = originalFocus
	}()
	gameState.Castle.MainCastle = Models.PlayerCastleInfo{
		Name:         "Runtime Castle",
		Aid:          5150,
		MapKingdomID: 0,
		MapX:         1200,
		MapY:         1300,
	}
	gameState.CastleFocus.CastleAID = 5150

	runtime, err := NewCommandRuntime(Automation.NewCommandHarness(Automation.NewCommandBroker()))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := runtime.ContextCatalog("focus_castle", "")
	if err != nil || len(definitions) != 1 {
		t.Fatalf("context catalog=%+v err=%v", definitions, err)
	}
	plan, err := runtime.PlanContext(context.Background(), ContextCommandInvocation{
		Name:      "focus_castle",
		Arguments: json.RawMessage(`{"castle":{"key":"mainCastle"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" || plan.Command != "focus_castle" || len(plan.Steps) != 1 ||
		plan.Steps[0].Castle == nil || plan.Steps[0].Castle.CastleID != 5150 {
		t.Fatalf("direct context plan=%+v", plan)
	}
}
