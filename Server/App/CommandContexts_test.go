package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestConstructionShopContextMatchesCapturedWireSequence(t *testing.T) {
	castle := State.CastleState{ID: 10, KingdomID: 2}
	steps := constructionShopContextSteps(castle)
	if len(steps) != 2 || steps[0].Opcode != "aec" || steps[1].Opcode != "gbc" {
		t.Fatalf("construction context = %#v", steps)
	}
	for _, step := range steps {
		if step.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("context step %q is not rebuilt on resume", step.Name)
		}
	}
	steps[0].Command.Namespace = "EmpireEx_21"
	wire, err := Protocol.Encode(steps[0].Command)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != "%xt%EmpireEx_21%aec%1%{}%" {
		t.Fatalf("aec wire = %s", wire)
	}
	if payload := string(steps[1].Command.Payload); payload != `{"CID":10,"KID":2}` {
		t.Fatalf("gbc payload = %s", payload)
	}
}

func TestStationRouteContextIsFullyRestartable(t *testing.T) {
	source := State.CastleState{X: 10, Y: 20}
	target := State.AllianceHolding{X: 30, Y: 40}
	steps := stationRouteContextSteps(source, target)
	if len(steps) != 1 || steps[0].Opcode != "sdi" || steps[0].AwaitOpcode != "sdi" {
		t.Fatalf("station context = %#v", steps)
	}
	if steps[0].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("station context is not restartable: %#v", steps)
	}
}

func TestAttackCastleContextUsesJAAForSelectionAndJCAForRefocus(t *testing.T) {
	focused := attackCastleContextStep(State.CastleState{ID: 10, KingdomID: 2, X: 12, Y: 34, Focused: true})
	if focused.Opcode != "jca" || focused.AwaitOpcode != "jaa" || string(focused.Command.Payload) != `{"CID":10,"KID":2}` {
		t.Fatalf("focused attack context = %#v", focused)
	}

	selected := attackCastleContextStep(State.CastleState{ID: 10, KingdomID: 2, X: 12, Y: 34})
	if selected.Opcode != "jaa" || selected.AwaitOpcode != "jaa" || string(selected.Command.Payload) != `{"PX":12,"PY":34,"KID":2}` {
		t.Fatalf("selected attack context = %#v", selected)
	}
}

func TestCRASetupContextRefreshesWorldMapAttackDialogThenPresets(t *testing.T) {
	steps, err := craSetupContextSteps([]byte(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 || steps[0].Opcode != "gbl" || steps[1].Opcode != "adi" || steps[2].Opcode != "gas" {
		t.Fatalf("CRA setup context = %#v", steps)
	}
	for _, step := range steps {
		if step.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("context step %q is not rebuilt on resume", step.Name)
		}
	}
	if string(steps[0].Command.Payload) != `{}` {
		t.Fatalf("gbl payload = %s", steps[0].Command.Payload)
	}
	if string(steps[1].Command.Payload) != `{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}` {
		t.Fatalf("adi payload = %s", steps[1].Command.Payload)
	}
	if string(steps[2].Command.Payload) != `{}` {
		t.Fatalf("gas payload = %s", steps[2].Command.Payload)
	}
}

func TestCRACommandDependenciesOwnSetupAndAuthoritativeGuard(t *testing.T) {
	payload := json.RawMessage(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}`)
	application := &Application{}
	dependencies, err := application.resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{}, Intent.Step{Command: Protocol.Command{Opcode: "cra", Payload: payload}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.Key != "2:12:34:56:78" || len(dependencies.Steps) != 4 ||
		dependencies.Steps[0].Opcode != "gbl" || dependencies.Steps[1].Opcode != "adi" ||
		dependencies.Steps[2].Opcode != "gas" || dependencies.Steps[3].Action != "attack.cra.send.guard" {
		t.Fatalf("CRA command dependencies = %#v", dependencies)
	}
}

func TestCRASendGuardRejectsPendingOrPositiveCooldown(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 56, Y: 78},
	}
	state.Map[0] = map[string]State.MapObservation{
		"56:78": {KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 56, Y: 78, ObservedAt: now},
	}
	state.TowerCooldowns["0:56:78"] = State.TowerCooldownState{
		KingdomID: 0, X: 56, Y: 78, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, DialogObservedAt: now,
	})
	if err := application.guardCRASend(context.Background(), arguments); err == nil {
		t.Fatal("CRA send guard accepted a target awaiting its post-victory cooldown refresh")
	}
	_, _ = application.State.Apply(func(current *State.GameState) ([]string, bool, error) {
		cooldown := current.TowerCooldowns["0:56:78"]
		cooldown.PendingCooldownRefresh = false
		current.TowerCooldowns["0:56:78"] = cooldown
		current.AttackDialog.Target.TowerCooldownRemaining = 300
		return []string{"attack_dialog", "tower-cooldowns"}, true, nil
	})
	if err := application.guardCRASend(context.Background(), arguments); err == nil {
		t.Fatal("CRA send guard accepted a positive authoritative cooldown")
	}
	_, _ = application.State.Apply(func(current *State.GameState) ([]string, bool, error) {
		current.AttackDialog.Target.TowerCooldownRemaining = 0
		return []string{"attack_dialog"}, true, nil
	})
	if err := application.guardCRASend(context.Background(), arguments); err != nil {
		t.Fatalf("CRA send guard rejected a freshly confirmed ready target: %v", err)
	}
}
