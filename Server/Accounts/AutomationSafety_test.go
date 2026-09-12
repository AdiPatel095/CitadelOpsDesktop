package Accounts

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestProfileAdoptionPreservesStrictestActiveLaneLocks(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name          string
		old, incoming State.AutomationSafetyLock
		want          string
	}{
		{"new unknown", State.AutomationSafetyLock{}, State.AutomationSafetyLock{OperationID: "new"}, "new"},
		{"existing unknown", State.AutomationSafetyLock{OperationID: "old"}, State.AutomationSafetyLock{OperationID: "new", Until: now.Add(time.Hour)}, "old"},
		{"unknown beats cooldown", State.AutomationSafetyLock{OperationID: "old", Until: now.Add(time.Hour)}, State.AutomationSafetyLock{OperationID: "new"}, "new"},
		{"longer cooldown", State.AutomationSafetyLock{OperationID: "old", Until: now.Add(time.Minute)}, State.AutomationSafetyLock{OperationID: "new", Until: now.Add(30 * time.Minute)}, "new"},
		{"review cannot clear corpus", State.AutomationSafetyLock{OperationID: "old"}, State.AutomationSafetyLock{OperationID: "new", ClearedAt: now}, "old"},
	} {
		t.Run(test.name, func(t *testing.T) {
			staging, player := t.TempDir(), t.TempDir()
			source, destination := State.NewGameState(), State.NewGameState()
			source.Automations["lane"] = State.AutomationState{ID: "lane", SafetyLock: test.incoming}
			destination.Automations["lane"] = State.AutomationState{ID: "lane", SafetyLock: test.old}
			destination.Player.Level = 70
			if err := State.SaveSnapshot(staging, source); err != nil {
				t.Fatal(err)
			}
			if err := State.SaveSnapshot(player, destination); err != nil {
				t.Fatal(err)
			}
			if err := mergeAutomationSafetyLocks(staging, player); err != nil {
				t.Fatal(err)
			}
			loaded, err := State.LoadSnapshot(player)
			if err != nil {
				t.Fatal(err)
			}
			if lock := loaded.Automations["lane"].SafetyLock; lock.OperationID != test.want || !lock.Active(now) {
				t.Fatalf("adopted lock=%#v", lock)
			}
			if loaded.Player.Level != 70 {
				t.Fatal("safety merge overwrote player corpus")
			}
		})
	}
}
