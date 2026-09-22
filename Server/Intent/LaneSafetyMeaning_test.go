package Intent

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLaneSafetyMeaningPersistsAndRefreshPreservesTimerAndContext(t *testing.T) {
	language, err := GameData.DecodeLanguage([]byte(`{"errorCode_90":"Please wait 4 sec to attack","errorCode_109":"All market barrows are moving."}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		opcode string
		code   int
		want   string
	}{
		{"cra", 90, "Please wait 4 sec to attack"}, {"mbr", 109, "All market barrows are moving."}, {"eup", 440, "This ruby purchase requires confirmation in the game."}, {"future", 98765, ""},
	} {
		store := State.NewStore(State.NewGameState())
		engine := NewEngine(NewRegistry(), store, &responseCodeTestProvider{language: language}, nil, nil)
		dir := t.TempDir()
		engine.SetLaneSafetyPersistence(func(_ context.Context, event State.Event) error {
			return State.SaveComponentSnapshot(dir, event, State.Components(State.ComponentAutomations))
		})
		ctx := context.WithValue(context.Background(), laneSafetyContextKey{}, Request{ID: "op-private", Name: "building.upgrade", Actor: "automation:test", AutomationLane: "test"})
		var locked *LaneLockedError
		if err := engine.guardRejection(ctx, NewResponseCodeError(language, tc.opcode, tc.code)); !errors.As(err, &locked) {
			t.Fatalf("not locked: %v", err)
		}
		if locked.Lock.Meaning != tc.want || strings.Contains(locked.Error(), "op-private") {
			t.Fatalf("lock=%+v", locked.Lock)
		}
		original := locked.Lock
		if err := engine.RefreshAutomationLaneLocks(); err != nil {
			t.Fatal(err)
		}
		saved, err := State.LoadSnapshot(dir)
		if err != nil {
			t.Fatal(err)
		}
		after := saved.Automations["test"].SafetyLock
		if after.Meaning != tc.want || after.OperationID != "op-private" || !after.ExpiresAt().Equal(original.ExpiresAt()) {
			t.Fatalf("saved=%+v", after)
		}
	}
	// Refresh legacy diagnostics without extending their cooldown or replacing
	// historical purchase context with a later game setting.
	state := State.NewGameState()
	lock := State.AutomationSafetyLock{Opcode: "cra", Code: 90, OperationID: "legacy", ObservedAt: time.Now().Add(-time.Minute), Context: "Original contextual explanation"}
	state.Automations["test"] = State.AutomationState{ID: "test", Status: "gated", SafetyLock: lock}
	store := State.NewStore(state)
	engine := NewEngine(NewRegistry(), store, &responseCodeTestProvider{language: language}, nil, nil)
	engine.SetLaneSafetyPersistence(func(context.Context, State.Event) error { return nil })
	if err := engine.RefreshAutomationLaneLocks(); err != nil {
		t.Fatal(err)
	}
	after := store.ReadOnlyView().Automations["test"].SafetyLock
	if after.Meaning != "Please wait 4 sec to attack" || after.Context != lock.Context || !after.ExpiresAt().Equal(lock.ExpiresAt()) {
		t.Fatal(after)
	}
}

func TestEUPLockContextUsesUpgradeCostAndGameSetting(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"buildings":[{"wodID":1,"name":"Stable","upgradeWodID":2},{"wodID":2,"name":"Stable","costC2":3100}],"resources":[{"resourceID":2,"JSONKey":"C2"}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Session = State.SessionState{Generation: 1, LoggedIn: true, SocketReady: true}
	state.Player.RubyConfirmation = State.RubyConfirmationState{Known: true, Amount: 2500, Generation: 1}
	state.Castles[10] = State.CastleState{Layout: State.CastleLayout{Objects: map[State.BuildingInstanceID]State.Building{20: {InstanceID: 20, DefinitionID: 1}}}}
	engine := NewEngine(NewRegistry(), State.NewStore(state), localizedGameDataProvider{store: data}, nil, nil)
	request := Request{Name: "building.upgrade", Arguments: json.RawMessage(`{"castleId":10,"buildingInstanceId":20}`)}
	detail := engine.rubyRejectionContext(request, State.AutomationSafetyLock{Opcode: "eup", Code: 440})
	if !strings.Contains(detail, "3,100 rubies") || !strings.Contains(detail, "2,500 rubies") {
		t.Fatal(detail)
	}
	state.Player.RubyConfirmation.Known = false
	engine.state = State.NewStore(state)
	if got := engine.rubyRejectionContext(request, State.AutomationSafetyLock{Opcode: "eup", Code: 440}); got != "" {
		t.Fatal(got)
	}
}
