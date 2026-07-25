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
		if event.Section != "scheduler" || event.Revision != 1 || event.Sequence != 1 || event.Gap {
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

func TestStoreMarksSubscriberGapAndCarriesLatestSnapshot(t *testing.T) {
	store, err := Open(t.TempDir(), map[string]json.RawMessage{
		"feature": json.RawMessage(`{"value":0}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := store.Subscribe(1)
	defer cancel()
	if _, err := store.Update("feature", json.RawMessage(`{"value":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update("feature", json.RawMessage(`{"value":2}`)); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Sequence != 2 || event.Revision != 2 || !event.Gap {
		t.Fatalf("stream metadata = sequence %d revision %d gap %t", event.Sequence, event.Revision, event.Gap)
	}
	if got := string(event.Snapshot.Sections["feature"]); got != `{"value":2}` {
		t.Fatalf("latest snapshot value = %s", got)
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

func TestStoreUpdateManyPersistsOneRevisionAndPublishesFullSnapshot(t *testing.T) {
	store, err := Open(t.TempDir(), map[string]json.RawMessage{
		"automation.alpha": json.RawMessage(`{"enabled":false}`),
		"scheduler":        json.RawMessage(`{"delay":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := store.Subscribe(1)
	defer cancel()

	snapshot, changed, err := store.UpdateMany(map[string]json.RawMessage{
		"automation.alpha": json.RawMessage(`{"enabled":true}`),
		"scheduler":        json.RawMessage(`{"delay":6}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("revision = %d, want 1", snapshot.Revision)
	}
	if len(changed) != 2 || changed[0] != "automation.alpha" || changed[1] != "scheduler" {
		t.Fatalf("changed sections = %#v", changed)
	}
	select {
	case event := <-events:
		if event.Section != "*" || !event.Gap || event.Revision != 1 || event.Sequence != 1 {
			t.Fatalf("unexpected batch event: %+v", event)
		}
		if got := string(event.Snapshot.Sections["scheduler"]); got != `{"delay":6}` {
			t.Fatalf("event scheduler = %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch configuration event")
	}

	reloaded, err := Open(filepath.Dir(filepath.Dir(store.path)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := reloaded.Section("automation.alpha"); !ok || string(value) != `{"enabled":true}` {
		t.Fatalf("reloaded automation section = %s, found = %t", value, ok)
	}
}

func TestStoreUpdateManyRejectsWholeBatchBeforeWriting(t *testing.T) {
	store, err := Open(t.TempDir(), map[string]json.RawMessage{
		"feature": json.RawMessage(`{"value":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpdateMany(map[string]json.RawMessage{
		"feature":     json.RawMessage(`{"value":2}`),
		"bad section": json.RawMessage(`{}`),
	}); err == nil {
		t.Fatal("invalid batch unexpectedly succeeded")
	}
	snapshot := store.Snapshot()
	if snapshot.Revision != 0 || string(snapshot.Sections["feature"]) != `{"value":1}` {
		t.Fatalf("invalid batch changed snapshot: %+v", snapshot)
	}
}

func TestStoreConditionalUpdateTracksItsSectionInsteadOfGlobalRevision(t *testing.T) {
	store, err := Open(t.TempDir(), map[string]json.RawMessage{
		"automation.crafting": json.RawMessage(`{"cursor":0}`),
		"unrelated":           json.RawMessage(`{"enabled":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	expected, exists := store.Section("automation.crafting")
	if !exists {
		t.Fatal("crafting section was not initialized")
	}
	if _, err := store.Update("unrelated", json.RawMessage(`{"enabled":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateConditional(
		"automation.crafting",
		json.RawMessage(`{"cursor":1}`),
		nil,
		&expected,
	); err != nil {
		t.Fatalf("unrelated revision blocked section-scoped update: %v", err)
	}
	if _, err := store.UpdateConditional(
		"automation.crafting",
		json.RawMessage(`{"cursor":2}`),
		nil,
		&expected,
	); err == nil {
		t.Fatal("stale section value did not block a conflicting update")
	}
}
