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
			"AI":{"N":"Target Castle","K":0,"X":10,"Y":20,"WL":8,"GL":7,"ML":6,"KL":5,"TL":4},
			"S":[[[216,100]],[],[[227,50]]],
			"B":{"L":70,"W":100,"GID":105,"D":10,"SPR":20,"ST":6,"AE":[],"EQ":[],"SIDS":[],"GASAIDS":[[101057,10113]]}
		}`),
	}
	report, err := ParseSpyCapture(capture)
	if err != nil {
		t.Fatalf("parse spy capture: %v", err)
	}
	if report.Status != "success" || report.TotalTroops != 150 || report.Castle.Name != "Target Castle" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Castle.WallLevel != 8 || report.Castle.GateLevel != 7 || report.Castle.MoatLevel != 6 ||
		report.Castle.KeepLevel != 5 || report.Castle.TowerLevel != 4 || report.Castellan == nil ||
		report.Castellan.GeneralID != 105 || report.Castellan.GeneralAbilitySkillIDs == nil {
		t.Fatalf("fortification or castellan wire fields were misclassified: %#v", report)
	}
}

func TestParseBattleCaptureBuildsCombatantsAndMetrics(t *testing.T) {
	capture := State.BattleReportCapture{
		MessageID: 101, ReportID: 202, BattleKey: "battle#key",
		AutomationFeature: State.AttackFeatureAutoTowers,
		CapturedAt:        time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Summary: json.RawMessage(`{
			"MID":101,"LID":202,"MT":6,"AHP":1,"DHP":0,
			"PI":[{"OID":1,"AID":11,"N":"Attacker","AN":"Us"},{"OID":2,"AID":22,"N":"Defender","AN":"Them"}],
			"PBI":[[1,0,1000,-100,[["W",253460],["C1",12738],["C2",15]]],[2,1,900,-900]],
				"AI":{"N":"Defender Castle","DP":2,"AT":24,"K":0,"X":10,"Y":20}
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
	if report.Attacker.AllianceID != 11 || report.Defender.AllianceID != 22 {
		t.Fatalf("alliance IDs were not retained: attacker=%d defender=%d", report.Attacker.AllianceID, report.Defender.AllianceID)
	}
	if report.Metrics.AttackerSent != 1000 || report.Metrics.DefenderLost != 900 || len(report.TopUnits) != 2 {
		t.Fatalf("unexpected metrics: %#v units=%#v", report.Metrics, report.TopUnits)
	}
	if report.AutomationFeature != "autoTowers" || report.Loot["W"] != 253460 || report.Loot["C1"] != 12738 || report.Loot["C2"] != 15 {
		t.Fatalf("unexpected attack analytics: feature=%q loot=%#v", report.AutomationFeature, report.Loot)
	}
	if report.BattleTypeID != 6 || report.TargetTypeID != 24 || report.TargetType != "Type 24" ||
		report.KingdomID != 0 || report.TargetX != 10 || report.TargetY != 20 {
		t.Fatalf("unexpected target identity: %#v", report)
	}
}

func TestParseBattleCaptureReadsBerimondGallantry(t *testing.T) {
	capture := State.BattleReportCapture{
		MessageID: 301, ReportID: 302, BattleKey: "beri#tower",
		AutomationFeature: State.AttackFeatureAutoBeriWorld,
		CapturedAt:        time.Date(2026, 7, 29, 11, 36, 0, 0, time.UTC),
		Summary: json.RawMessage(`{
			"MID":301,"LID":302,"MT":6,"AHP":1,"DHP":0,
			"PI":[{"OID":1,"N":"Attacker"},{"OID":-410,"DUM":true}],
			"PBI":[
				[1,0,1076,-45,[["W",868],["S",815],["F",1262],["C1",46808],["C2",11]],0,0,0,1743,0,0,87,0],
				[-410,1,890,-890,[["W",-868],["S",-815],["F",-1262],["C1",-46808],["C2",-11]],0,0,0,-1,0,0,-100,0]
			],
			"AI":{"DP":-410,"AT":17,"K":10,"X":1438,"Y":82}
		}`),
		Details: json.RawMessage(`{"LID":302,"Y":[]}`),
	}
	report, err := ParseBattleCapture(capture, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.AutomationFeature != string(State.AttackFeatureAutoBeriWorld) ||
		report.GallantryPoints != 1743 || report.Loot["C1"] != 46808 {
		t.Fatalf("unexpected Berimond analytics: feature=%q gallantry=%d loot=%#v",
			report.AutomationFeature, report.GallantryPoints, report.Loot)
	}
}
