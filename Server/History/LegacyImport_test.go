package History

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
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

func TestPlayerSamplesForPlayerIgnoresChangingLegacyUID(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []json.RawMessage{
		json.RawMessage(`{"timestampUnix":100,"uid":11,"playerId":7,"might":100}`),
		json.RawMessage(`{"timestampUnix":110,"uid":22,"playerId":8,"might":200}`),
		json.RawMessage(`{"timestampUnix":120,"uid":33,"playerId":7,"might":110}`),
	} {
		if err := store.Append(CollectionPlayerSamples, sample); err != nil {
			t.Fatal(err)
		}
	}

	samples, err := store.PlayerSamplesForPlayer(time.Time{}, 10, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0].Might != 100 || samples[1].Might != 110 {
		t.Fatalf("player 7 history = %+v", samples)
	}
	for _, sample := range samples {
		if sample.PlayerID != 7 {
			t.Fatalf("history included another player: %+v", sample)
		}
	}
}

func TestNormalizePlayerSampleTroopsExcludesTools(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},
		"buildings":[{"wodID":1}],
		"units":[{"wodID":10,"slotTypes":[]},{"wodID":20,"slotTypes":[1]}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		t.Fatal(err)
	}
	sample := normalizePlayerSampleTroops(PlayerSample{
		TroopsTotal:  60,
		TroopsByUnit: map[string]int64{"10": 10, "20": 20, "30": 30},
	}, &troopClassifier{catalog: catalog, cache: map[State.UnitID]bool{}, enabled: true})
	if sample.TroopsTotal != 10 || len(sample.TroopsByUnit) != 1 || sample.TroopsByUnit["10"] != 10 {
		t.Fatalf("troop-only sample = %+v", sample)
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
