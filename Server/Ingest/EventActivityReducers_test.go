package Ingest

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestEventBattleActivitySeparatesCampAndKhanDefenseTotals(t *testing.T) {
	now := time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, RemainingSec: 7_200, ObservedAt: now}
	State.RecordEventAttackLaunch(&gameState, 72, State.EventAttackRecord{
		MovementID: 1, Kind: State.EventActivityCamp, KingdomID: 0, TargetTypeID: 27,
		TargetX: 100, TargetY: 101, LaunchedAt: now, ArrivesAt: now.Add(time.Minute),
	})
	camp := State.BattleReportCapture{
		MessageID: 100, ReportID: 200, CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{"MID":100,"LID":200,"PBI":[[10,0,100,-20],[-601,1,50,-50]],"AI":{"AT":27,"K":0,"X":100,"Y":101}}`),
		Details: json.RawMessage(`{"W":[[[10,[[[1,100,-20]],[[700,10,-10]]] ],[-601,[[],[]]]]],"S":[[10,[403,1,-1]]]}`),
	}
	changed, err := reconcileEventBattleActivity(&gameState, &camp)
	if err != nil || !changed {
		t.Fatalf("camp reconcile: changed=%t err=%v", changed, err)
	}
	if camp.AutomationFeature != State.AttackFeatureAutoNomad || camp.EventID != 72 || camp.EventActivity != State.EventActivityCamp || camp.MovementID != 1 {
		t.Fatalf("camp attribution = %#v", camp)
	}
	activity := gameState.EventScores.ActivityByEvent[72]
	if activity.Camp.Launches != 1 || activity.Camp.Battles != 1 || activity.Camp.Victories != 1 ||
		activity.Camp.TroopLosses != 20 || activity.Camp.ToolsUsed != 11 || len(activity.PendingAttacks) != 0 {
		t.Fatalf("camp activity = %#v", activity.Camp)
	}

	State.RecordEventAttackLaunch(&gameState, 72, State.EventAttackRecord{
		MovementID: 2, Kind: State.EventActivityKhanDefense, KingdomID: 0, TargetTypeID: 1,
		TargetX: 110, TargetY: 111, LaunchedAt: now.Add(2 * time.Minute), ArrivesAt: now.Add(3 * time.Minute),
	})
	defense := State.BattleReportCapture{
		MessageID: 101, ReportID: 201, CapturedAt: now.Add(3 * time.Minute),
		Summary: json.RawMessage(`{"MID":101,"LID":201,"PBI":[[-601,0,50,-50],[10,1,100,-5]],"AI":{"AT":1,"K":0,"X":110,"Y":111}}`),
		Details: json.RawMessage(`{"W":[[[-601,[[],[]]],[10,[[[1,100,-5]],[[701,3,-3]]]]]]}`),
	}
	changed, err = reconcileEventBattleActivity(&gameState, &defense)
	if err != nil || !changed {
		t.Fatalf("defense reconcile: changed=%t err=%v", changed, err)
	}
	if defense.AutomationFeature != State.AttackFeatureAutoKhan || defense.EventID != 72 ||
		defense.EventActivity != State.EventActivityKhanDefense || defense.MovementID != 2 {
		t.Fatalf("Khan defense attribution = %#v", defense)
	}
	activity = gameState.EventScores.ActivityByEvent[72]
	if activity.KhanDefense.Launches != 1 || activity.KhanDefense.Battles != 1 || activity.KhanDefense.Victories != 1 ||
		activity.KhanDefense.TroopLosses != 5 || activity.KhanDefense.ToolsUsed != 3 {
		t.Fatalf("Khan defense activity = %#v", activity.KhanDefense)
	}
}

func TestEventBattleActivityTracksBloodcrowInvasion(t *testing.T) {
	now := time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{EventID: 103, RemainingSec: 7_200, ObservedAt: now}
	State.RecordEventAttackLaunch(&gameState, 103, State.EventAttackRecord{
		MovementID: 3, Kind: State.EventActivityInvasion, KingdomID: 0, TargetTypeID: 34,
		TargetX: 120, TargetY: 121, LaunchedAt: now, ArrivesAt: now.Add(time.Minute),
	})
	report := State.BattleReportCapture{
		MessageID: 102, ReportID: 202, CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{"MID":102,"LID":202,"PBI":[[10,0,18560,-765,[["W",184800],["S",184800],["F",171037],["C1",11445],["HONEY",164120],["MEAD",164120]]],[-1002,1,19281,-19281]],"AI":{"AT":34,"K":0,"X":120,"Y":121}}`),
		Details: json.RawMessage(`{"W":[[[10,[[[1,100,-12]],[[702,7,-7]]] ],[-603,[[],[]]]]]}`),
	}
	changed, err := reconcileEventBattleActivity(&gameState, &report)
	if err != nil || !changed {
		t.Fatalf("Bloodcrow reconcile: changed=%t err=%v", changed, err)
	}
	if report.AutomationFeature != State.AttackFeatureAutoInvasion || report.EventID != 103 ||
		report.EventActivity != State.EventActivityInvasion || report.MovementID != 3 {
		t.Fatalf("Bloodcrow attribution = %#v", report)
	}
	totals := gameState.EventScores.ActivityByEvent[103].Invasion
	if totals.Launches != 1 || totals.Battles != 1 || totals.Victories != 1 ||
		totals.TroopLosses != 765 || totals.ToolsUsed != 7 || totals.Loot != 880_322 {
		t.Fatalf("Bloodcrow activity = %#v", totals)
	}
}

func TestInvasionReportHoldRequiresRecoverableLaunchBoundary(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	capture := State.BattleReportCapture{
		MessageID: 501, ReportID: 601, OccurredAt: now.Add(time.Minute), CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{"PBI":[[10,0,100,-10],[-2,1,100,-100]],"AI":{"AT":21,"K":0,"X":101,"Y":102}}`),
		Details: json.RawMessage(`{"LID":601}`),
	}
	reservation := State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: now.Add(time.Hour),
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
		SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
		CommanderID: 7, CommanderKnown: true,
		OperationID: "indeterminate-cra", ReservedAt: now,
	}
	if !InvasionReservationReportCandidate(gameState, reservation, capture) {
		t.Fatal("complete recovery-capable reservation did not hold a plausible report")
	}
	for _, malformed := range []json.RawMessage{
		json.RawMessage(`{"PBI":[[10,0,100,-10],[-2,1,100,-100]],"AI":{"AT":21,"K":"0","X":101,"Y":102}}`),
		json.RawMessage(`{"PBI":[[10,"0",100,-10],[-2,1,100,-100]],"AI":{"AT":21,"K":0,"X":101,"Y":102}}`),
		json.RawMessage(`{"PBI":[["10",0,100,-10],[-2,1,100,-100]],"AI":{"AT":21,"K":0,"X":101,"Y":102}}`),
		json.RawMessage(`{"PBI":[[10,0,100,-10],[-2,1,100,-100]],"AI":{"AT":0,"K":0,"X":101,"Y":102}}`),
	} {
		candidate := capture
		candidate.Summary = malformed
		if InvasionReservationReportCandidate(gameState, reservation, candidate) {
			t.Fatalf("malformed report identity was held for invasion recovery: %s", malformed)
		}
	}
	for _, mutate := range []func(*State.InvasionTargetReservation){
		func(candidate *State.InvasionTargetReservation) { candidate.OperationID = "" },
		func(candidate *State.InvasionTargetReservation) { candidate.SourceKnown = false },
		func(candidate *State.InvasionTargetReservation) { candidate.SourceCastleID = 0 },
		func(candidate *State.InvasionTargetReservation) { candidate.CommanderKnown = false },
	} {
		candidate := reservation
		mutate(&candidate)
		if InvasionReservationReportCandidate(gameState, candidate, capture) {
			t.Fatalf("unrecoverable reservation held report forever: %#v", candidate)
		}
	}
}

func TestLiveEventAndFeatureAttributionRejectReportsBeforeImpact(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, RemainingSec: 3_600, ObservedAt: now,
	}
	State.RecordEventAttackLaunch(&gameState, 71, State.EventAttackRecord{
		MovementID: 9, Kind: State.EventActivityInvasion, KingdomID: 0,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 102,
		LaunchedAt: now, ArrivesAt: arrivesAt,
	})
	gameState.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
		MovementID: 9, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: 0,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 102,
		LaunchedAt: now, ArrivesAt: arrivesAt,
	}}
	capture := State.BattleReportCapture{
		MessageID: 501, ReportID: 601, OccurredAt: now, CapturedAt: now,
		Summary: json.RawMessage(`{"MID":501,"LID":601,"PBI":[[10,0,100,-10],[-2,1,100,-100]],"AI":{"AT":21,"K":0,"X":101,"Y":102}}`),
		Details: json.RawMessage(`{"LID":601}`),
	}
	eventChanged, err := reconcileEventBattleActivity(&gameState, &capture)
	if err != nil {
		t.Fatal(err)
	}
	analyticsChanged, err := reconcileAttackFeatureBattleReport(&gameState, &capture)
	if err != nil {
		t.Fatal(err)
	}
	activity, _ := gameState.LookupEventActivity(71)
	if eventChanged || analyticsChanged || capture.MovementID != 0 || capture.AutomationFeature != "" ||
		len(activity.PendingAttacks) != 1 || len(gameState.AttackAnalytics.PendingAttacks) != 1 {
		t.Fatalf("pre-impact report was attributed: eventChanged=%t analyticsChanged=%t capture=%#v activity=%#v analytics=%#v",
			eventChanged, analyticsChanged, capture, activity, gameState.AttackAnalytics)
	}
}

func TestLiveEventAndFeatureAttributionRequireExactTargetType(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, RemainingSec: 3_600, ObservedAt: now,
	}
	State.RecordEventAttackLaunch(&gameState, 71, State.EventAttackRecord{
		MovementID: 9, Kind: State.EventActivityInvasion, KingdomID: 0,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 102,
		LaunchedAt: now, ArrivesAt: arrivesAt,
	})
	gameState.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
		MovementID: 9, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: 0,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 102,
		LaunchedAt: now, ArrivesAt: arrivesAt,
	}}
	capture := State.BattleReportCapture{
		MessageID: 501, ReportID: 601, OccurredAt: arrivesAt, CapturedAt: arrivesAt,
		Summary: json.RawMessage(`{"MID":501,"LID":601,"PBI":[[10,0,100,-10],[-2,1,100,-100]],"AI":{"AT":0,"K":0,"X":101,"Y":102}}`),
		Details: json.RawMessage(`{"LID":601}`),
	}
	eventChanged, err := reconcileEventBattleActivity(&gameState, &capture)
	if err != nil {
		t.Fatal(err)
	}
	analyticsChanged, err := reconcileAttackFeatureBattleReport(&gameState, &capture)
	if err != nil {
		t.Fatal(err)
	}
	activity, _ := gameState.LookupEventActivity(71)
	if eventChanged || analyticsChanged || capture.MovementID != 0 || capture.AutomationFeature != "" ||
		len(activity.PendingAttacks) != 1 || len(gameState.AttackAnalytics.PendingAttacks) != 1 {
		t.Fatalf("unknown-type report was attributed: eventChanged=%t analyticsChanged=%t capture=%#v activity=%#v analytics=%#v",
			eventChanged, analyticsChanged, capture, activity, gameState.AttackAnalytics)
	}
}

func TestRecoveredInvasionLaunchReconcilesReportCapturedWithinReservationGrace(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103])
	report := State.BattleReportCapture{
		MessageID: 102, ReportID: 202, CapturedAt: now.Add(10 * time.Second), OccurredAt: now.Add(10 * time.Second),
		Summary: json.RawMessage(`{"MID":102,"LID":202,"PBI":[[10,0,18560,-765],[-1002,1,19281,-19281]],"AI":{"AT":34,"K":0,"X":120,"Y":121}}`),
		Details: json.RawMessage(`{"LID":202,"W":[[[10,[[[1,100,-12]],[[702,7,-7]]] ],[-603,[[],[]]]]]}`),
	}
	gameState.SetBattleReportCapture(report.MessageID, report)
	if changed, err := reconcileEventBattleActivity(&gameState, &report); err != nil || changed {
		t.Fatalf("report unexpectedly attributed before launch recovery: changed=%t err=%v", changed, err)
	}

	record := State.EventAttackRecord{
		MovementID: 3, Kind: State.EventActivityInvasion, KingdomID: 0, TargetTypeID: 34,
		TargetX: 120, TargetY: 121, LaunchedAt: now.Add(5 * time.Second), ArrivesAt: now.Add(10 * time.Second),
	}
	if !State.RecordEventAttackLaunchForOccurrence(&gameState, 103, occurrenceEndsAt, record) {
		t.Fatal("recovered event launch was not recorded")
	}
	if !State.RecordAttackFeatureLaunch(&gameState, State.AttackFeatureLaunch{
		MovementID: 3, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: 0, TargetTypeID: 34,
		TargetX: 120, TargetY: 121, LaunchedAt: record.LaunchedAt, ArrivesAt: record.ArrivesAt,
	}) {
		t.Fatal("recovered feature launch was not recorded")
	}
	changed, err := ReconcileRetainedBattleCapturesForRecoveredLaunch(&gameState, 3)
	if err != nil || !changed {
		t.Fatalf("retained report reconciliation: changed=%t err=%v", changed, err)
	}

	activity := gameState.EventScores.ActivityByEvent[103]
	stored := gameState.Reports.BattleCaptures[102]
	if activity.Invasion.Launches != 1 || activity.Invasion.Battles != 1 || activity.Invasion.Victories != 1 ||
		len(activity.PendingAttacks) != 0 || len(gameState.AttackAnalytics.PendingAttacks) != 0 ||
		stored.MovementID != 3 || stored.AutomationFeature != State.AttackFeatureAutoInvasion ||
		stored.EventID != 103 || stored.EventActivity != State.EventActivityInvasion {
		t.Fatalf("recovered report attribution: activity=%#v analytics=%#v report=%#v", activity, gameState.AttackAnalytics, stored)
	}
}

func TestRecoveredLaunchChoosesClosestReportAndExactMovement(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	for _, record := range []State.EventAttackRecord{
		{MovementID: 2, Kind: State.EventActivityInvasion, KingdomID: 0, TargetTypeID: 34, TargetX: 120, TargetY: 121, LaunchedAt: now, ArrivesAt: now.Add(5 * time.Second)},
		{MovementID: 3, Kind: State.EventActivityInvasion, KingdomID: 0, TargetTypeID: 34, TargetX: 120, TargetY: 121, LaunchedAt: now.Add(10 * time.Second), ArrivesAt: now.Add(20 * time.Second)},
	} {
		if !State.RecordEventAttackLaunch(&gameState, 103, record) || !State.RecordAttackFeatureLaunch(&gameState, State.AttackFeatureLaunch{
			MovementID: record.MovementID, FeatureID: State.AttackFeatureAutoInvasion,
			KingdomID: record.KingdomID, TargetTypeID: record.TargetTypeID, TargetX: record.TargetX, TargetY: record.TargetY,
			LaunchedAt: record.LaunchedAt, ArrivesAt: record.ArrivesAt,
		}) {
			t.Fatalf("could not stage launch %d", record.MovementID)
		}
	}
	report := func(messageID int64, occurredAt time.Time) State.BattleReportCapture {
		return State.BattleReportCapture{
			MessageID: messageID, ReportID: messageID + 1, OccurredAt: occurredAt,
			// Deliberately much later than the battle: matching must use the
			// actual occurrence, not the time details were fetched.
			CapturedAt: now.Add(2 * time.Hour),
			Summary: json.RawMessage(fmt.Sprintf(
				`{"MID":%d,"LID":%d,"PBI":[[10,0,100,-1],[-1002,1,100,-100]],"AI":{"AT":34,"K":0,"X":120,"Y":121}}`,
				messageID, messageID+1,
			)),
			Details: json.RawMessage(fmt.Sprintf(`{"LID":%d,"W":[]}`, messageID+1)),
		}
	}
	gameState.Reports.BattleCaptures[100] = report(100, now.Add(5*time.Second))
	gameState.Reports.BattleCaptures[200] = report(200, now.Add(20*time.Second))

	changed, err := ReconcileRetainedBattleCapturesForRecoveredLaunch(&gameState, 3)
	if err != nil || !changed {
		t.Fatalf("exact recovered reconciliation: changed=%t err=%v", changed, err)
	}
	activity := gameState.EventScores.ActivityByEvent[103]
	if gameState.Reports.BattleCaptures[200].MovementID != 3 || gameState.Reports.BattleCaptures[100].MovementID != 0 ||
		len(activity.PendingAttacks) != 1 || activity.PendingAttacks[0].MovementID != 2 ||
		len(gameState.AttackAnalytics.PendingAttacks) != 1 || gameState.AttackAnalytics.PendingAttacks[0].MovementID != 2 {
		t.Fatalf("closest exact reconciliation: reports=%#v activity=%#v analytics=%#v",
			gameState.Reports.BattleCaptures, activity, gameState.AttackAnalytics)
	}
}
