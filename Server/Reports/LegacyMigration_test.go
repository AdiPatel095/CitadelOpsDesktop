package Reports

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func TestMigrateLegacyHistoryParsesCanonicalReports(t *testing.T) {
	dataDir := t.TempDir()
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	spyLine := `{
		"version":1,"mid":101,"capturedAtUnixMillis":1783785600000,
		"bsd":{
			"MID":101,"SA":95,"SR":10,"SC":20,"GC":30,"CID":77,
			"OI":{"OID":2,"N":"Target","AN":"Them"},
			"SO":{"OID":1,"N":"Source","AN":"Us"},
			"AI":{"N":"Target Castle","K":0,"X":10,"Y":20},
			"S":[[[216,100]],[],[[227,50]]],
			"B":{"L":70,"W":100,"GID":5,"D":10,"SPR":20,"ST":6,"AE":[],"EQ":[],"SIDS":[]}
		}
	}`
	battleLine := `{
		"version":1,"mid":102,"lid":202,"battleKey":"battle#key","capturedAtUnixMillis":1783785600000,
		"bld":{
			"MID":102,"LID":202,"MT":6,"AHP":1,"DHP":0,
			"PI":[{"OID":1,"N":"Attacker","AN":"Us"},{"OID":2,"N":"Defender","AN":"Them"}],
			"PBI":[[1,0,1000,-100],[2,1,900,-900]],
			"AI":{"N":"Defender Castle","DP":2,"K":0,"X":10,"Y":20}
		},
		"blm":{"LID":202},
		"bls":{"LID":202,"Y":[[1,[216,1000,-100]],[2,[227,900,-900]]]}
	}`
	if err := os.WriteFile(filepath.Join(dataDir, "SpyReports.jsonl"), append(compactJSONLine(t, spyLine), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	battleContents := append([]byte(`{"version":1,"mid":999,"source":"sne"}`+"\n"), compactJSONLine(t, battleLine)...)
	battleContents = append(battleContents, '\n')
	if err := os.WriteFile(
		filepath.Join(dataDir, "BattleReports.jsonl"),
		battleContents, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyHistory(dataDir, history, State.PlayerID(1)); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyHistory(dataDir, history, State.PlayerID(1)); err != nil {
		t.Fatal(err)
	}

	spyRows, err := history.Read(History.CollectionSpyReports, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(spyRows) != 1 {
		t.Fatalf("spy report count = %d, want 1", len(spyRows))
	}
	var spy SpyReport
	if err := json.Unmarshal(spyRows[0], &spy); err != nil {
		t.Fatal(err)
	}
	if spy.Status != "success" || spy.TotalTroops != 150 || spy.Castle.Name != "Target Castle" {
		t.Fatalf("unexpected migrated spy report: %+v", spy)
	}

	battleRows, err := history.Read(History.CollectionBattleReports, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(battleRows) != 1 {
		t.Fatalf("battle report count = %d, want 1", len(battleRows))
	}
	var battle BattleReport
	if err := json.Unmarshal(battleRows[0], &battle); err != nil {
		t.Fatal(err)
	}
	if battle.Role != "attacker" || battle.Result != "Victory" || battle.Metrics.DefenderLost != 900 {
		t.Fatalf("unexpected migrated battle report: %+v", battle)
	}
}

func compactJSONLine(t *testing.T, value string) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := json.Compact(&output, []byte(value)); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
