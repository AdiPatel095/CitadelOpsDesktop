package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestMovementFrameImmediatelyReconcilesReservedInvasionLaunch(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, X: 10, Y: 11}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103])
	reservedAt := now.Add(-2 * time.Second)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: 34, X: 120, Y: 121,
		SourceCastleID: 100, SourceX: 10, SourceY: 11, SourceKnown: true,
		CommanderID: 7, CommanderKnown: true,
		OperationID: "reserved-cra", ReservedAt: reservedAt,
	})
	gameState.SetBattleReportCapture(501, State.BattleReportCapture{
		MessageID: 501, ReportID: 601, OccurredAt: now.Add(59 * time.Second), CapturedAt: now,
		Summary: json.RawMessage(`{"MID":501,"LID":601,"PBI":[[10,0,18560,-765],[-1002,1,19281,-19281]],"AI":{"AT":34,"K":0,"X":120,"Y":121}}`),
		Details: json.RawMessage(`{"LID":601,"W":[[[10,[[[1,100,-12]],[[702,7,-7]]] ],[-603,[[],[]]]]]}`),
	})

	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	fencedRevision := uint64(0)
	pipeline.SetDurabilityFence(func(_ context.Context, event State.Event) error {
		fencedRevision = event.Revision
		return nil
	})
	code := 0
	committed, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":3,"PT":1,"TT":60,"D":0,"T":0,"KID":0,"OID":10,"TID":-1002,"SA":[2,10,11,100,10],"TA":[34,120,121,-1,-1002]},"UM":{"L":{"ID":7}}}
		]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	result := store.ReadOnlyView()
	activity, found := result.LookupEventActivity(103)
	report, reportFound := result.LookupBattleReportCapture(501)
	if _, reserved := result.Invasion.TargetReservation(0, 120, 121); reserved || !found || !reportFound ||
		activity.Invasion.Launches != 1 || activity.Invasion.Battles != 1 || len(activity.PendingAttacks) != 0 ||
		len(result.AttackAnalytics.PendingAttacks) != 0 || report.MovementID != 3 ||
		report.AutomationFeature != State.AttackFeatureAutoInvasion || report.EventID != 103 {
		t.Fatalf("immediate invasion reconciliation: activity=%#v analytics=%#v report=%#v reserved=%t",
			activity, result.AttackAnalytics, report, reserved)
	}
	if !containsDomain(committed.Domains, "reports") || !containsDomain(committed.Domains, "invasion") {
		t.Fatalf("reconciliation domains = %v", committed.Domains)
	}
	if fencedRevision != committed.Revision {
		t.Fatalf("durability fence revision = %d, committed = %d", fencedRevision, committed.Revision)
	}
}

func TestRecoveredOutboundUsesMovementStartForEventAndAnalyticsLaunch(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	observedAt := reservedAt.Add(3 * time.Second)
	startedAt := reservedAt.Add(time.Second)
	arrivesAt := reservedAt.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, X: 10, Y: 11}
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: reservedAt,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103])
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: 34, X: 120, Y: 121,
		SourceCastleID: 100, SourceX: 10, SourceY: 11, SourceKnown: true,
		CommanderID: 7, CommanderKnown: true, OperationID: "outbound-start-cra", ReservedAt: reservedAt,
	})
	commanderID := State.CommanderID(7)
	gameState.Movements[3] = State.MovementState{
		ID: 3, Direction: 0, KingdomID: 0,
		SourceCastleID: 100, SourceX: 10, SourceY: 11,
		TargetTypeID: 34, TargetX: 120, TargetY: 121,
		CommanderID: &commanderID, StartedAt: startedAt, ObservedAt: observedAt, ArrivesAt: &arrivesAt,
	}

	result, err := ReconcileInvasionReservationMovement(
		&gameState,
		gameState.Invasion.TargetReservations[State.InvasionTargetKey(0, 120, 121)],
		gameState.Movements[3],
	)
	if err != nil || !result.EventChanged || !result.AnalyticsChanged {
		t.Fatalf("outbound reconciliation = %#v err=%v", result, err)
	}
	activity, found := gameState.LookupEventActivity(103)
	if !found || len(activity.PendingAttacks) != 1 || !activity.PendingAttacks[0].LaunchedAt.Equal(startedAt) {
		t.Fatalf("event pending launch = %#v", activity.PendingAttacks)
	}
	if len(gameState.AttackAnalytics.PendingAttacks) != 1 ||
		!gameState.AttackAnalytics.PendingAttacks[0].LaunchedAt.Equal(startedAt) {
		t.Fatalf("analytics pending launch = %#v", gameState.AttackAnalytics.PendingAttacks)
	}
}

func TestRawShortReturnFrameReconcilesReservedInvasionLaunch(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	observedAt := reservedAt.Add(25 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, X: 10, Y: 11}
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: reservedAt,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103])
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: 34, X: 120, Y: 121,
		SourceCastleID: 100, SourceX: 10, SourceY: 11, SourceKnown: true,
		CommanderID: 7, CommanderKnown: true, OperationID: "short-return-cra", ReservedAt: reservedAt,
	})
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	_, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":3,"PT":5,"TT":60,"D":1,"T":0,"KID":0,"OID":10,"TID":10,"SA":[34,120,121,-1,-1002],"TA":[2,10,11,100,10]},"UM":{"L":{"ID":7}}}
		]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	result := store.ReadOnlyView()
	activity, found := result.LookupEventActivity(103)
	if _, reserved := result.Invasion.TargetReservation(0, 120, 121); reserved || !found ||
		activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		activity.PendingAttacks[0].MovementID != 3 || !activity.PendingAttacks[0].LaunchedAt.Equal(reservedAt) ||
		!activity.PendingAttacks[0].ArrivesAt.Equal(reservedAt.Add(20*time.Second)) ||
		len(result.AttackAnalytics.PendingAttacks) != 1 ||
		!result.AttackAnalytics.PendingAttacks[0].LaunchedAt.Equal(reservedAt) {
		t.Fatalf("raw short-return recovery: activity=%#v invasion=%#v", activity, result.Invasion)
	}
}

func containsDomain(domains []string, expected string) bool {
	for _, domain := range domains {
		if domain == expected {
			return true
		}
	}
	return false
}
