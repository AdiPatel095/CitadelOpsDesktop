package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanAllianceHelpRequestUsesCapturedAHRPayload(t *testing.T) {
	for _, test := range []struct {
		name        string
		lineID      int
		requestID   int64
		requestType int
	}{
		{name: "recruitment", lineID: recruitmentProductionLineID, requestID: 0, requestType: allianceHelpRecruitmentType},
		{name: "hospital", lineID: hospitalProductionLineID, requestID: 2033307472, requestType: allianceHelpHospitalType},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := State.NewGameState()
			state.Castles[77] = State.CastleState{
				ID: 77, X: 12, Y: 34, KingdomID: 1, Focused: true,
				Production: map[int]State.ProductionQueue{
					test.lineID: {LineID: test.lineID, Active: &State.QueueItem{ProductionID: 2033307472}},
				},
			}
			plan, err := planAllianceHelpRequest(context.Background(), Intent.PlanningContext{State: state}, json.RawMessage(`{"productionId":2033307472}`))
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Steps) != 3 || plan.Steps[0].Command.Opcode != "jaa" ||
				plan.Steps[1].Resolver != "alliance.help.build" || plan.Steps[2].Action != "alliance.help.mark_requested" {
				t.Fatalf("unexpected plan: %#v", plan)
			}
			resolved, err := (&Application{}).resolveAllianceHelpRequestStep(
				t.Context(), Intent.PlanningContext{State: state}, plan.Steps[1].ResolverArguments,
			)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				RequestID int64 `json:"ID"`
				Type      int   `json:"T"`
			}
			if err := json.Unmarshal(resolved.Command.Payload, &payload); err != nil {
				t.Fatalf("decode AHR payload: %v", err)
			}
			if payload.RequestID != test.requestID || payload.Type != test.requestType {
				t.Fatalf("AHR payload = %#v", payload)
			}
			if !allianceHelpContainsString(resolved.AwaitOpcodes, "ahh") || !allianceHelpContainsString(resolved.AwaitOpcodes, "ahr") {
				t.Fatalf("AHR response opcodes = %#v", resolved.AwaitOpcodes)
			}
			var recorded allianceHelpRequest
			if err := json.Unmarshal(plan.Steps[2].ActionArguments, &recorded); err != nil {
				t.Fatalf("decode recorded request: %v", err)
			}
			if recorded.CastleID != 77 || recorded.LineID != test.lineID || recorded.ProductionID != 2033307472 {
				t.Fatalf("recorded request = %#v", recorded)
			}
		})
	}
}

func TestPlanHospitalAllianceHelpStopsAtObservedThreeRequestLimit(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Queued: []State.QueueItem{
					{ProductionID: 201, AllianceHelpRequested: true},
					{ProductionID: 202, AllianceHelpRequested: true},
					{ProductionID: 203, AllianceHelpRequested: true},
					{ProductionID: 204},
				},
			},
		},
	}
	plan, err := planAllianceHelpRequest(
		t.Context(), Intent.PlanningContext{State: state}, json.RawMessage(`{"productionId":204}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "3 outstanding requests") {
		t.Fatalf("alliance-help capacity plan = %#v", plan)
	}
}

func TestAllianceHelpGuardReplansAtAuthoritativeHospitalLimit(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Queued: []State.QueueItem{{ProductionID: 204}},
			},
		},
	}
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		HospitalProductionIDs: []int64{201, 202, 203},
		ObservedAt:            time.Now().UTC(),
	}
	_, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"productionId":204,"castleId":77,"lineId":2}`),
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("hospital help limit should make the plan stale: %v", err)
	}
}

func TestAllianceHelpGuardSkipsAuthoritativeRecruitmentRequestForCastle(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID,
				Active: &State.QueueItem{ProductionID: 205},
			},
		},
	}
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77},
		ObservedAt:           time.Now().UTC(),
	}
	plan, err := planAllianceHelpRequest(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"productionId":205}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "no longer eligible") {
		t.Fatalf("recruitment help duplicate guard plan = %#v", plan)
	}
}

func TestAllianceHelpResolverRejectsNewRecruitQueueWhileCastleRequestIsOutstanding(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID,
				Active: &State.QueueItem{ProductionID: 205},
			},
		},
	}
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77},
		ObservedAt:           time.Now().UTC(),
	}
	_, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"productionId":205,"castleId":77}`),
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("recruitment help duplicate should make the plan stale: %v", err)
	}
}

func allianceHelpContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestPlanAllianceHelpRequestRejectsToolQueue(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			1: {LineID: 1, Active: &State.QueueItem{ProductionID: 101, AllianceHelpAvailable: true}},
		},
	}
	if _, err := planAllianceHelpRequest(context.Background(), Intent.PlanningContext{State: state}, json.RawMessage(`{"productionId":101}`)); err == nil {
		t.Fatal("expected tool queue alliance help request to be rejected")
	}
}

func TestMarkAllianceHelpRequestedPreventsRepeatRequests(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {
				LineID: 0,
				Active: &State.QueueItem{ProductionID: 101, AllianceHelpAvailable: true},
				Queued: []State.QueueItem{{ProductionID: 101}, {ProductionID: 102}},
			},
		},
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.markAllianceHelpRequested(context.Background(), json.RawMessage(`{"productionId":101,"castleId":77,"lineId":0}`)); err != nil {
		t.Fatal(err)
	}
	queue := application.State.Snapshot().Castles[77].Production[0]
	item := queue.Active
	if item == nil || !item.AllianceHelpRequested {
		t.Fatalf("alliance help request was not recorded: %#v", item)
	}
	for index, queued := range queue.Queued {
		if !queued.AllianceHelpRequested {
			t.Fatalf("queued recruitment item %d was not recorded: %#v", index, queued)
		}
	}
	if allianceHelpEligible(application.State.Snapshot(), 101) {
		t.Fatal("recorded job is still eligible for a duplicate alliance help request")
	}
}

func TestMarkHospitalAllianceHelpOnlyMarksMatchingJob(t *testing.T) {
	state := State.NewGameState()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			2: {LineID: 2, Queued: []State.QueueItem{{ProductionID: 201}, {ProductionID: 202}}},
		},
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.markAllianceHelpRequested(context.Background(), json.RawMessage(`{"productionId":201,"castleId":77,"lineId":2}`)); err != nil {
		t.Fatal(err)
	}
	queued := application.State.Snapshot().Castles[77].Production[2].Queued
	if !queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("unexpected hospital alliance-help state: %#v", queued)
	}
}

func TestPlanAllianceHelpAnswerAllUsesCapturedHelpAllPayload(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		PendingOtherListIDs: []int64{101, 102}, OthersObservedAt: time.Now().UTC(), OthersObservedGeneration: 7,
	}
	plan, err := planAllianceHelpAnswerAll(t.Context(), Intent.PlanningContext{State: state}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Resolver != "alliance.help.answer_all.build" ||
		plan.Steps[1].Action != "alliance.help.mark_answered" || !allianceHelpContainsString(plan.Claims, "alliance-help") {
		t.Fatalf("unexpected help-all plan: %#v", plan)
	}
	resolved, err := resolveAllianceHelpAnswerAllStep(
		t.Context(), Intent.PlanningContext{State: state}, plan.Steps[0].ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Limit int `json:"KID"`
	}
	if err := json.Unmarshal(resolved.Command.Payload, &payload); err != nil {
		t.Fatalf("decode AHA payload: %v", err)
	}
	if resolved.Command.Opcode != "aha" || resolved.AwaitOpcode != "aha" || payload.Limit != allianceHelpAllLimit {
		t.Fatalf("unexpected AHA step: %#v payload=%#v", resolved, payload)
	}
}

func TestAllianceHelpAnswerAllResolverRejectsCompletedBatch(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		PendingOtherListIDs: []int64{202}, OthersObservedAt: time.Now().UTC(), OthersObservedGeneration: 7,
	}
	_, err := resolveAllianceHelpAnswerAllStep(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"listIds":[101],"sessionGeneration":7}`),
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("completed help-all batch should make the plan stale: %v", err)
	}
}

func TestAllianceHelpAnswerAllAllowsOneUnobservedSessionBootstrap(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	plan, err := planAllianceHelpAnswerAll(
		t.Context(), Intent.PlanningContext{State: state}, json.RawMessage(`{"allowUnobserved":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || !strings.Contains(plan.Summary, "Check and help") {
		t.Fatalf("unexpected bootstrap plan: %#v", plan)
	}
	if _, err := resolveAllianceHelpAnswerAllStep(
		t.Context(), Intent.PlanningContext{State: state}, plan.Steps[0].ResolverArguments,
	); err != nil {
		t.Fatalf("resolve bootstrap help-all: %v", err)
	}

	application := &Application{State: State.NewStore(state)}
	if err := application.markAllianceHelpAnswered(t.Context(), plan.Steps[1].ActionArguments); err != nil {
		t.Fatal(err)
	}
	snapshot := application.State.Snapshot()
	if snapshot.AllianceHelpRequests.LastHelpAllGeneration != 7 ||
		snapshot.AllianceHelpRequests.LastHelpAllAt.IsZero() {
		t.Fatalf("bootstrap was not recorded: %#v", snapshot.AllianceHelpRequests)
	}
	second, err := planAllianceHelpAnswerAll(
		t.Context(), Intent.PlanningContext{State: snapshot}, json.RawMessage(`{"allowUnobserved":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Steps) != 0 {
		t.Fatalf("bootstrap repeated in one session: %#v", second)
	}
}

func TestMarkAllianceHelpAnsweredPreservesNewRequests(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		PendingOtherListIDs: []int64{101, 102, 103}, OthersObservedAt: time.Now().UTC(), OthersObservedGeneration: 7,
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.markAllianceHelpAnswered(
		t.Context(), json.RawMessage(`{"listIds":[101,102],"sessionGeneration":7}`),
	); err != nil {
		t.Fatal(err)
	}
	pending := application.State.Snapshot().AllianceHelpRequests.PendingOtherListIDs
	if len(pending) != 1 || pending[0] != 103 {
		t.Fatalf("pending help requests = %#v, want newly arrived request 103", pending)
	}
	if generation := application.State.Snapshot().AllianceHelpRequests.LastHelpAllGeneration; generation != 7 {
		t.Fatalf("last help-all generation = %d, want 7", generation)
	}
}
