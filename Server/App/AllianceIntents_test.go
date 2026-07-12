package App

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanAllianceInspectVerifiesMatchingRoster(t *testing.T) {
	plan, err := planAllianceInspect(t.Context(), Intent.PlanningContext{}, json.RawMessage(`{"allianceId":9}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "ain" || plan.Steps[1].Action != "alliance.verify_inspection" {
		t.Fatalf("inspection plan = %#v", plan)
	}

	gameState := State.NewGameState()
	application := &Application{State: State.NewStore(gameState)}
	if err := application.verifyAllianceInspection(t.Context(), json.RawMessage(`{"allianceId":9}`)); err == nil {
		t.Fatal("missing alliance inspection passed verification")
	}
	_, err = application.State.Apply(func(state *State.GameState) ([]string, bool, error) {
		state.Alliances[9] = State.AllianceState{ID: 9, Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}}
		return []string{"alliances"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyAllianceInspection(t.Context(), json.RawMessage(`{"allianceId":9}`)); err != nil {
		t.Fatal(err)
	}
}
