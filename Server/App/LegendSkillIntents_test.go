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

func TestLegendSkillPurchaseBuildsCapturedSKPCommand(t *testing.T) {
	gameData := legendSkillIntentGameData(t)
	gameState := State.NewGameState()
	gameState.Player.LegendSkills = State.LegendSkillState{
		ActiveIDs: []int64{11}, SkillPoints: 3, ObservedAt: time.Now().UTC(),
	}
	input := Intent.PlanningContext{State: gameState, GameData: gameData}
	plan, err := planLegendSkillPurchase(t.Context(), input, json.RawMessage(`{"skillId":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Claims, "hall-of-legends") || len(plan.Steps) != 1 ||
		plan.Steps[0].Resolver != "legend.skill.purchase.build" || plan.Steps[0].AwaitOpcode != "skp" {
		t.Fatalf("unexpected Hall of Legends purchase plan: %+v", plan)
	}
	step, err := resolveLegendSkillPurchaseStep(t.Context(), input, plan.Steps[0].ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "skp" || step.AwaitOpcode != "skp" || string(step.Command.Payload) != `{"ID":12}` {
		t.Fatalf("unexpected SKP step: %+v", step)
	}
}

func TestLegendSkillPurchaseRefreshesStaleStateAndValidatesSequence(t *testing.T) {
	gameData := legendSkillIntentGameData(t)
	gameState := State.NewGameState()
	input := Intent.PlanningContext{State: gameState, GameData: gameData}
	plan, err := planLegendSkillPurchase(t.Context(), input, json.RawMessage(`{"skillId":11}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "skl" || plan.Steps[0].AwaitOpcode != "skl" ||
		plan.Steps[1].Resolver != "legend.skill.purchase.build" {
		t.Fatalf("stale Hall state did not add SKL refresh: %+v", plan.Steps)
	}

	gameState.Player.LegendSkills = State.LegendSkillState{
		ActiveIDs: []int64{11}, SkillPoints: 3, ObservedAt: time.Now().UTC(),
	}
	input.State = gameState
	_, err = planLegendSkillPurchase(t.Context(), input, json.RawMessage(`{"skillId":13}`))
	if err == nil || !strings.Contains(err.Error(), "requires active skill 12") {
		t.Fatalf("expected prerequisite rejection, got %v", err)
	}
	_, err = planLegendSkillPurchase(t.Context(), input, json.RawMessage(`{"skillId":101}`))
	if err == nil || !strings.Contains(err.Error(), "granted automatically") {
		t.Fatalf("expected automatic-skill rejection, got %v", err)
	}
}

func TestLegendSkillResetUsesCapturedSKRThenRefreshesSKL(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.LegendSkills = State.LegendSkillState{
		ActiveIDs: []int64{11}, SkillPoints: 3, ResetRemainingSec: 0, ResetCount: 1,
		ObservedAt: time.Now().UTC(),
	}
	input := Intent.PlanningContext{State: gameState}
	plan, err := planLegendSkillsReset(t.Context(), input, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Resolver != "legend.skills.reset.build" ||
		plan.Steps[0].AwaitOpcode != "skr" || plan.Steps[1].Opcode != "skl" || plan.Steps[1].AwaitOpcode != "skl" {
		t.Fatalf("unexpected Hall of Legends reset plan: %+v", plan.Steps)
	}
	step, err := resolveLegendSkillsResetStep(t.Context(), input, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "skr" || step.AwaitOpcode != "skr" || string(step.Command.Payload) != `{}` {
		t.Fatalf("unexpected SKR step: %+v", step)
	}

	gameState.Player.LegendSkills.ResetRemainingSec = 3600
	input.State = gameState
	_, err = planLegendSkillsReset(t.Context(), input, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "next free Hall of Legends reset") {
		t.Fatalf("expected cooldown rejection, got %v", err)
	}
}

func legendSkillIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"legendskills":[
			{"skillID":11,"followingSkillID":12,"level":1,"skillTreeID":0,"tier":1,"skillGroupID":2,"totalCostSkillPoints":1,"costSkillPoints":1},
			{"skillID":12,"requiredSkillID":11,"followingSkillID":13,"level":2,"skillTreeID":0,"tier":1,"skillGroupID":2,"totalCostSkillPoints":2,"costSkillPoints":1},
			{"skillID":13,"requiredSkillID":12,"level":3,"skillTreeID":0,"tier":1,"skillGroupID":2,"totalCostSkillPoints":3,"costSkillPoints":1},
			{"skillID":101,"level":1,"skillTreeID":0,"tier":3,"skillGroupID":10,"specialType":"special"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
