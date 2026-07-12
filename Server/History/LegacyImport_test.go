package History

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadAcceptsEarlierHistoryEnvelopeSchema(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"timestampUnix":1783785600,"playerId":7,"might":100}`)
	line, err := json.Marshal(entry{
		SchemaVersion: 1,
		CapturedAt:    time.Unix(1783785600, 0).UTC(),
		Payload:       payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dataDir, "History", "PlayerSamples.jsonl"),
		append(line, '\n'), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	samples, err := store.PlayerSamples(time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].PlayerID != 7 || samples[0].Might != 100 {
		t.Fatalf("earlier history envelope was not read: %+v", samples)
	}
}

func TestOpenImportsChangedLegacyPlayerTrackerWithoutDuplicates(t *testing.T) {
	dataDir := t.TempDir()
	sourcePath := filepath.Join(dataDir, "PlayerTracker.json")
	first := `{"version":4,"players":{"7":[{"timestampUnix":1783785600,"playerId":7,"might":100}]}}`
	if err := os.WriteFile(sourcePath, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	assertPlayerSampleCount(t, store, 1)

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	assertPlayerSampleCount(t, reopened, 1)

	second := `{"version":4,"players":{"7":[
		{"timestampUnix":1783785600,"playerId":7,"might":100},
		{"timestampUnix":1783785660,"playerId":7,"might":110}
	]}}`
	if err := os.WriteFile(sourcePath, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	assertPlayerSampleCount(t, updated, 2)
	contents, err := os.ReadFile(sourcePath)
	if err != nil || string(contents) != second {
		t.Fatal("legacy player tracker was modified")
	}
}

func assertPlayerSampleCount(t *testing.T, store *Store, expected int) {
	t.Helper()
	samples, err := store.PlayerSamples(time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != expected {
		t.Fatalf("player sample count = %d, want %d", len(samples), expected)
	}
}
