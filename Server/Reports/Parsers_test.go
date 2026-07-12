package Reports

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestParseSpyCaptureBuildsCanonicalReport(t *testing.T) {
	capture := State.SpyReportCapture{
		MessageID: 101, CapturedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{
			"MID":101,"SA":95,"SR":10,"SC":20,"GC":30,"CID":77,
			"OI":{"OID":2,"N":"Target","AN":"Them"},
			"SO":{"OID":1,"N":"Source","AN":"Us"},
			"AI":{"N":"Target Castle","K":0,"X":10,"Y":20},
			"S":[[[216,100]],[],[[227,50]]],
			"B":{"L":70,"W":100,"GID":5,"D":10,"SPR":20,"ST":6,"AE":[],"EQ":[],"SIDS":[]}
		}`),
	}
	report, err := ParseSpyCapture(capture)
	if err != nil {
		t.Fatalf("parse spy capture: %v", err)
	}
	if report.Status != "success" || report.TotalTroops != 150 || report.Castle.Name != "Target Castle" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestParseBattleCaptureBuildsCombatantsAndMetrics(t *testing.T) {
	capture := State.BattleReportCapture{
		MessageID: 101, ReportID: 202, BattleKey: "battle#key",
		CapturedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Summary: json.RawMessage(`{
			"MID":101,"LID":202,"MT":6,"AHP":1,"DHP":0,
			"PI":[{"OID":1,"N":"Attacker","AN":"Us"},{"OID":2,"N":"Defender","AN":"Them"}],
			"PBI":[[1,0,1000,-100],[2,1,900,-900]],
			"AI":{"N":"Defender Castle","DP":2,"K":0,"X":10,"Y":20}
		}`),
		Details: json.RawMessage(`{"LID":202,"Y":[[1,[216,1000,-100]],[2,[227,900,-900]]]}`),
	}
	report, err := ParseBattleCapture(capture, 1)
	if err != nil {
		t.Fatalf("parse battle capture: %v", err)
	}
	if report.Result != "Victory" || report.Role != "attacker" || report.Attacker.Name != "Attacker" || report.Defender.Name != "Defender" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Metrics.AttackerSent != 1000 || report.Metrics.DefenderLost != 900 || len(report.TopUnits) != 2 {
		t.Fatalf("unexpected metrics: %#v units=%#v", report.Metrics, report.TopUnits)
	}
}
