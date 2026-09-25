package Intent

import (
	"context"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestSafetyRefreshPersistsLegacyExpiryAndWhitelistWithoutReset(t *testing.T) {
	now := time.Now().UTC()
	initial := State.NewGameState()
	for _, item := range []struct {
		lane, opcode string
		code         int
		observed     time.Time
	}{
		{"allowed", "ahr", 273, now}, {"expired", "cra", 256, now.Add(-31 * time.Minute)},
		{"recent", "unknown", 911, now.Add(-10 * time.Minute)}, {"msd", "msd", 311, now.Add(-5 * time.Minute)},
	} {
		lock := State.AutomationSafetyLock{Lane: item.lane, Opcode: item.opcode, Code: item.code, OperationID: item.lane, ObservedAt: item.observed}
		initial.Automations[item.lane] = State.AutomationState{ID: item.lane, Status: "gated", Detail: lock.Detail(), SafetyLock: lock}
	}
	dir := t.TempDir()
	store := State.NewStore(initial)
	engine := NewEngine(NewRegistry(), store, nil, nil, nil)
	engine.SetLaneSafetyPersistence(func(_ context.Context, event State.Event) error {
		return State.SaveComponentSnapshot(dir, event, State.Components(State.ComponentAutomations))
	})
	if err := engine.RefreshAutomationLaneLocks(); err != nil {
		t.Fatal(err)
	}
	loaded, err := State.LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, lane := range []string{"allowed", "expired", "recent", "msd"} {
		lock := loaded.Automations[lane].SafetyLock
		if !lock.Until.Equal(initial.Automations[lane].SafetyLock.ObservedAt.Add(30 * time.Minute)) {
			t.Fatalf("timer reset: %#v", lock)
		}
		wantActive := lane == "recent" || lane == "msd"
		if lock.Active(now) != wantActive {
			t.Fatalf("%s active=%v", lane, lock.Active(now))
		}
		if !wantActive {
			detail := loaded.Automations[lane]
			if detail.DetailDescriptor == nil || detail.DetailDescriptor.Key != "server.intent.safety_lock_released" || detail.DetailDescriptor.FallbackText != detail.Detail || detail.LastErrorDescriptor != nil {
				t.Fatal("release descriptor missing or stale")
			}
		}
		if !wantActive && (lock.ReviewedBy == "" || lock.ClearedAt.IsZero() || loaded.Automations[lane].Status == "gated") {
			t.Fatalf("release not audited: %#v", loaded.Automations[lane])
		}
	}
	restarted := NewEngine(NewRegistry(), State.NewStore(loaded), nil, nil, nil)
	saves := 0
	restarted.SetLaneSafetyPersistence(func(context.Context, State.Event) error { saves++; return nil })
	if err := restarted.RefreshAutomationLaneLocks(); err != nil || saves != 0 {
		t.Fatalf("refresh not idempotent: saves=%d err=%v", saves, err)
	}
}

func TestSafetyRefreshFailsIfMigrationCannotPersist(t *testing.T) {
	initial := State.NewGameState()
	initial.Automations["lane"] = State.AutomationState{SafetyLock: State.AutomationSafetyLock{OperationID: "legacy", Opcode: "ahr", Code: 273, ObservedAt: time.Now()}}
	engine := NewEngine(NewRegistry(), State.NewStore(initial), nil, nil, nil)
	engine.SetLaneSafetyPersistence(func(context.Context, State.Event) error { return errors.New("disk failed") })
	if err := engine.RefreshAutomationLaneLocks(); err == nil {
		t.Fatal("migration persistence failure ignored")
	}
}
