package Configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsVersionedSections(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir, map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"minAttackDelay":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := store.Subscribe(1)
	defer cancel()
	snapshot, err := store.Update("scheduler", json.RawMessage(`{ "minAttackDelay": 5, "enabled": true }`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("revision = %d, want 1", snapshot.Revision)
	}
	select {
	case event := <-events:
		if event.Section != "scheduler" || event.Revision != 1 {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for configuration event")
	}
	reloaded, err := Open(dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := reloaded.Section("scheduler")
	if !ok || string(value) != `{"minAttackDelay":5,"enabled":true}` {
		t.Fatalf("reloaded scheduler = %s", value)
	}
	info, err := os.Stat(filepath.Join(dataDir, "Config", "Settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("configuration permissions = %o, want owner-only", info.Mode().Perm())
	}
}

func TestStoreRejectsInvalidSections(t *testing.T) {
	store, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		section string
		value   json.RawMessage
	}{
		{"", json.RawMessage(`{}`)},
		{"bad section", json.RawMessage(`{}`)},
		{"valid", json.RawMessage(`{`)},
	} {
		if _, err := store.Update(test.section, test.value); err == nil {
			t.Fatalf("Update(%q, %s) unexpectedly succeeded", test.section, test.value)
		}
	}
}

func TestStoreNoOpKeepsRevision(t *testing.T) {
	store, err := Open(t.TempDir(), map[string]json.RawMessage{"feature": json.RawMessage(`{"enabled":false}`)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Update("feature", json.RawMessage(`{ "enabled": false }`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 {
		t.Fatalf("no-op revision = %d, want 0", snapshot.Revision)
	}
}
