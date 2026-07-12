package Configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMigratesLegacy138Configuration(t *testing.T) {
	dataDir := t.TempDir()
	legacyFiles := map[string]string{
		"SchedulerSettings.json": `{
			"version":2,"minAttackDelay":4.5,"maxAttackDelay":7,
			"featureSchedules":{"autoRecruit":{"enabled":true}}
		}`,
		"RecruitTroops.json": `{
			"version":1,"mode":"perCastle","checkIntervalSec":120,
			"globalTargets":{"216":0},
			"enabledCastles":{"20":true,"30":false},
			"targets":{"20":{"216":80,"215":40},"40":{"223":0}}
		}`,
		"AutoTool.json": `{
			"version":1,"mode":"global","checkIntervalSec":300,
			"globalTargets":{"614":0},"enabledCastles":{"20":true},"targets":{}
		}`,
		"AutoHospital.json": `{"version":1,"checkIntervalSec":90}`,
		"AutoSceatRes.json": `{"version":1,"checkIntervalSec":300,"castles":{}}`,
		"AutoTCI.json": `{
			"version":1,"targets":{"20":{"501":4,"502":{"minLevel":2,"maxLevel":3}}},
			"presets":{"version":1,"lastSelectedPresetId":null,"presets":[]}
		}`,
		"AutoBird.json":    `{"version":1,"ignoreSettings":{"settings":{},"minDelay":6,"maxDelay":12,"minSend":1,"minRPTDays":3}}`,
		"AutoStation.json": `{"version":1,"leadTimeSec":90,"recallWhenClear":true,"minRPTDays":3,"settings":{}}`,
		"AutoBeriWorld.json": `{
			"version":1,"minTroopsToTransfer":250,"beriCastleCID":81,
			"transferTroopWID":216,"kutSourceCastleSCID":20,"kutCastleCID":-1,
			"troopSpaceCheckIntervalSec":45
		}`,
		"RiftMaidenComms.json": `{"version":1,"unitWodID":215}`,
	}
	for name, contents := range legacyFiles {
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	store, err := Open(dataDir, map[string]json.RawMessage{
		"automation.enabled":       json.RawMessage(`{}`),
		"automation.autoBeriWorld": json.RawMessage(`{"beriCastleId":0}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduler := decodeTestSection[map[string]any](t, store, "scheduler")
	if _, exists := scheduler["version"]; exists {
		t.Fatal("scheduler migration retained the legacy envelope version")
	}
	if scheduler["minAttackDelay"] != 4.5 {
		t.Fatalf("scheduler minAttackDelay = %v", scheduler["minAttackDelay"])
	}

	recruit := decodeTestSection[struct {
		Mode        string                      `json:"mode"`
		GlobalItems []productionMigrationTarget `json:"globalItems"`
		Castles     map[string]productionCastle `json:"castles"`
	}](t, store, "automation.recruitTroops")
	if recruit.Mode != "perCastle" || len(recruit.GlobalItems) != 1 || recruit.GlobalItems[0].ID != 216 {
		t.Fatalf("unexpected recruit migration: %+v", recruit)
	}
	if !recruit.Castles["20"].Enabled || len(recruit.Castles["20"].Items) != 2 || recruit.Castles["20"].Items[0].ID != 215 {
		t.Fatalf("unexpected castle 20 recruit plan: %+v", recruit.Castles["20"])
	}
	if recruit.Castles["30"].Enabled || recruit.Castles["40"].Enabled {
		t.Fatalf("legacy enable switches were not preserved: %+v", recruit.Castles)
	}

	tools := decodeTestSection[struct {
		Mode        string                      `json:"mode"`
		GlobalItems []productionMigrationTarget `json:"globalItems"`
		Castles     map[string]productionCastle `json:"castles"`
	}](t, store, "automation.autoTool")
	if tools.Mode != "global" || len(tools.GlobalItems) != 1 || tools.GlobalItems[0].ID != 614 || !tools.Castles["20"].Enabled {
		t.Fatalf("unexpected tool migration: %+v", tools)
	}

	construction := decodeTestSection[struct {
		Targets map[string][]constructionMigrationTarget `json:"targets"`
	}](t, store, "automation.constructionItems")
	if len(construction.Targets["20"]) != 2 || construction.Targets["20"][0].ID != 501 || construction.Targets["20"][1].MinLevel != 2 {
		t.Fatalf("unexpected construction migration: %+v", construction.Targets)
	}

	beri := decodeTestSection[struct {
		Minimum  int64 `json:"minTroopsToTransfer"`
		CastleID int64 `json:"beriCastleId"`
		UnitID   int64 `json:"transferTroopId"`
		SourceID int64 `json:"sourceCastleId"`
		WireID   int64 `json:"wireCastleId"`
		Interval int   `json:"troopSpaceCheckIntervalSec"`
	}](t, store, "automation.autoBeriWorld")
	if beri.Minimum != 250 || beri.CastleID != 81 || beri.UnitID != 216 || beri.SourceID != 20 || beri.WireID != -1 || beri.Interval != 45 {
		t.Fatalf("unexpected Berimond migration: %+v", beri)
	}

	for name, contents := range legacyFiles {
		actual, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil || string(actual) != contents {
			t.Fatalf("legacy file %s was modified", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "Config", "Settings.json")); err != nil {
		t.Fatal(err)
	}
}

func TestOpenNeverOverwritesCanonicalSectionWithLegacyFile(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dataDir, "SchedulerSettings.json"),
		[]byte(`{"version":2,"minAttackDelay":4}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := decodeTestSection[map[string]float64](t, reloaded, "scheduler")
	if scheduler["minAttackDelay"] != 9 {
		t.Fatalf("canonical scheduler was overwritten: %v", scheduler)
	}
}

func decodeTestSection[T any](t *testing.T, store *Store, section string) T {
	t.Helper()
	raw, exists := store.Section(section)
	if !exists {
		t.Fatalf("configuration section %q is missing", section)
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode configuration section %q: %v", section, err)
	}
	return result
}
