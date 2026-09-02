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
		stepCount   int
	}{
		{name: "recruitment", lineID: recruitmentProductionLineID, requestID: 0, requestType: allianceHelpRecruitmentType, stepCount: 4},
		{name: "hospital", lineID: hospitalProductionLineID, requestID: 2033307472, requestType: allianceHelpHospitalType, stepCount: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := State.NewGameState()
			observedAt := time.Now().UTC()
			state.Player.ID = 501
			state.Session.Generation = 7
			state.Session.ConnectionGeneration = 3
			state.AllianceHelpRequests = State.AllianceHelpRequestState{
				ObservedAt: observedAt, OwnObservedGeneration: 7,
			}
			state.Castles[77] = State.CastleState{
				ID: 77, X: 12, Y: 34, KingdomID: 1, Focused: true,
				Production: map[int]State.ProductionQueue{
					test.lineID: {
						LineID: test.lineID, ObservedAt: observedAt,
						Active: &State.QueueItem{ProductionID: 2033307472},
					},
				},
			}
			plan, err := planAllianceHelpRequest(context.Background(), Intent.PlanningContext{State: state}, json.RawMessage(`{"productionId":2033307472}`))
			if err != nil {
				t.Fatal(err)
			}
			resolverIndex := 1
			if test.lineID == recruitmentProductionLineID {
				resolverIndex = 2
			}
			if len(plan.Steps) != test.stepCount || plan.Steps[0].Command.Opcode != "jaa" ||
				plan.Steps[resolverIndex].Resolver != "alliance.help.build" {
				t.Fatalf("unexpected plan: %#v", plan)
			}
			if len(plan.Steps[0].StaleCodes) != 1 || plan.Steps[0].StaleCodes[0] != 175 {
				t.Fatalf("AHR castle focus does not classify inaccessible-kingdom code 175 as stale: %#v", plan.Steps[0])
			}
			if test.lineID == hospitalProductionLineID && plan.Steps[2].Action != "alliance.help.mark_requested" {
				t.Fatalf("hospital request lost its post-success marker: %#v", plan)
			}
			if test.lineID == recruitmentProductionLineID &&
				(plan.Steps[1].Action != "alliance.help.prepare_recruitment_bup" ||
					plan.Steps[3].Action != "alliance.help.reconcile_recruitment_bup") {
				t.Fatalf("recruitment request lost its post-success reconciliation: %#v", plan)
			}
			protocolContext := State.ProtocolContextState{
				SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77,
				FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4,
			}
			if test.lineID == recruitmentProductionLineID {
				protocolContext.RecruitmentBUPCastleID = 77
				protocolContext.RecruitmentBUPFocusEpoch = 4
				protocolContext.RecruitmentAHRPending = true
			}
			resolved, err := (&Application{}).resolveAllianceHelpRequestStep(
				t.Context(), Intent.PlanningContext{
					State: state, ProtocolContext: protocolContext,
				}, plan.Steps[resolverIndex].ResolverArguments,
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
			if len(resolved.StaleCodes) != 1 || resolved.StaleCodes[0] != 175 {
				t.Fatalf("AHR command does not classify inaccessible-kingdom code 175 as stale: %#v", resolved)
			}
			if test.lineID == recruitmentProductionLineID &&
				(resolved.ResponseIdentity.PlayerID != 501 || resolved.ResponseIdentity.CastleID != 77 ||
					resolved.ResponseBarrier != Intent.ResponseBarrierCommitted) {
				t.Fatalf("recruitment AHR response identity/barrier = %#v / %q", resolved.ResponseIdentity, resolved.ResponseBarrier)
			}
			var recorded allianceHelpRequest
			if err := json.Unmarshal(plan.Steps[resolverIndex].ResolverArguments, &recorded); err != nil {
				t.Fatalf("decode recorded request: %v", err)
			}
			if recorded.CastleID != 77 || recorded.LineID != test.lineID || recorded.ProductionID != 2033307472 {
				t.Fatalf("recorded request = %#v", recorded)
			}
		})
	}
}

func TestRecruitmentAllianceHelpResolverRequiresExactCommittedCastleContext(t *testing.T) {
	state := State.NewGameState()
	observedAt := time.Now().UTC()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		ObservedAt: observedAt, OwnObservedGeneration: 7,
	}
	state.Castles[77] = State.CastleState{
		ID: 77, KingdomID: 1, Focused: true,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID, ObservedAt: observedAt,
				Active: &State.QueueItem{ProductionID: 205, AllianceHelpAvailable: true},
			},
		},
	}
	arguments := json.RawMessage(`{"productionId":205,"castleId":77,"lineId":0}`)
	exact := State.ProtocolContextState{
		SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77,
		FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4,
	}
	marked := exact
	marked.RecruitmentBUPCastleID = 77
	marked.RecruitmentBUPFocusEpoch = 4
	marked.RecruitmentAHRPending = true
	for _, test := range []struct {
		name     string
		context  State.ProtocolContextState
		playerID State.PlayerID
	}{
		{name: "missing context", context: State.ProtocolContextState{}, playerID: 501},
		{name: "missing focus epoch", context: State.ProtocolContextState{SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextCastle}, playerID: 501},
		{name: "wrong castle", context: State.ProtocolContextState{SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 88, FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4}, playerID: 501},
		{name: "map subcontext", context: State.ProtocolContextState{SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextMap, FocusEpoch: 4}, playerID: 501},
		{name: "old session", context: State.ProtocolContextState{SessionGeneration: 6, ConnectionGeneration: 3, FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4}, playerID: 501},
		{name: "old connection", context: State.ProtocolContextState{SessionGeneration: 7, ConnectionGeneration: 2, FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4}, playerID: 501},
		{name: "missing AHR marker", context: exact, playerID: 501},
		{name: "missing player identity", context: marked, playerID: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := state
			candidate.Player.ID = test.playerID
			if _, err := (&Application{}).resolveAllianceHelpRequestStep(
				t.Context(), Intent.PlanningContext{State: candidate, ProtocolContext: test.context}, arguments,
			); !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatalf("resolver accepted unsafe recruitment context: %v", err)
			}
		})
	}
}

func TestRecruitmentBUPAllianceHelpRejectsDuplicateCurrentCastleRequest(t *testing.T) {
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs:  []State.CastleID{77},
		OwnObservedGeneration: 7,
		ObservedAt:            time.Now().UTC(),
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 77, Progress: 1, MaximumHelpers: 3, ObservedAt: time.Now().UTC()},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	state.Castles[77] = State.CastleState{
		ID: 77, KingdomID: 1, Focused: true,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID, ObservedAt: time.Now().UTC(),
				Active: &State.QueueItem{ProductionID: 205, AllianceHelpRequested: true},
			},
		},
	}
	protocol := State.ProtocolContextState{
		SessionGeneration: 7, ConnectionGeneration: 3,
		FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 9,
		RecruitmentBUPCastleID: 77, RecruitmentBUPFocusEpoch: 9,
		RecruitmentBUPSerial: 2,
	}
	input := Intent.PlanningContext{State: state, ProtocolContext: protocol}
	if _, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, json.RawMessage(`{"castleId":77}`),
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("current same-castle request did not suppress a duplicate AHR: %v", err)
	}
	input.State.AllianceHelpRequests.OwnRecruitmentRequests = []State.RecruitmentAllianceHelpRequest{}
	input.State.AllianceHelpRequests.RecruitmentCastleIDs = []State.CastleID{}
	step, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, json.RawMessage(`{"castleId":77}`),
	)
	if err != nil {
		t.Fatalf("uncovered BUP without a current request did not produce AHR: %v", err)
	}
	var payload struct {
		RequestID int64 `json:"ID"`
		Type      int   `json:"T"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequestID != 0 || payload.Type != allianceHelpRecruitmentType ||
		step.ResponseIdentity.PlayerID != 501 || step.ResponseIdentity.CastleID != 77 ||
		step.ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("causal recruitment AHR step = %#v payload=%#v", step, payload)
	}

	input.ProtocolContext.RecruitmentAHRCoveredSerial = 2
	if _, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, json.RawMessage(`{"castleId":77}`),
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("covered BUP batch produced duplicate AHR: %v", err)
	}
	input.ProtocolContext.FocusEpoch = 10
	input.ProtocolContext.RecruitmentAHRCoveredSerial = 0
	if _, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, json.RawMessage(`{"castleId":77}`),
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("old focus-epoch BUP produced AHR after refocus: %v", err)
	}
	input.ProtocolContext.RecruitmentBUPFocusEpoch = 10
	input.ProtocolContext.RecruitmentBUPSerial = 1
	if _, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, json.RawMessage(`{"castleId":77}`),
	); err != nil {
		t.Fatalf("fresh post-refocus BUP without a current request did not produce a new AHR: %v", err)
	}
}

func TestRecruitmentBUPAllianceHelpMarkersRequireExactFocus(t *testing.T) {
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		OwnObservedGeneration: 7, OwnRecruitmentObservedGeneration: 7,
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 77, Progress: 0, MaximumHelpers: 3, ObservedAt: time.Now().UTC()},
		},
	}
	state.Castles[77] = State.CastleState{ID: 77, KingdomID: 1, Focused: true}
	store := State.NewStore(state)
	application := &Application{State: store}
	arguments := json.RawMessage(`{"castleId":77}`)
	if err := application.markRecruitmentBUPAllianceHelpDue(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.markRecruitmentBUPAllianceHelpDue(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	protocol := store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 2 || protocol.RecruitmentAHRCoveredSerial != 2 ||
		!protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("two committed BUP markers = %#v", protocol)
	}
	store.ObserveProtocolFocus(State.FocusSubcontextMap, time.Now().UTC())
	if err := application.markRecruitmentBUPAllianceHelpDue(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("map context accepted recruitment BUP marker: %v", err)
	}
}

func TestRecruitmentBUPCoveredMarkerRequiresCommittedLifecycleEvidence(t *testing.T) {
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Castles[77] = State.CastleState{ID: 77, KingdomID: 1, Focused: true}
	store := State.NewStore(state)
	application := &Application{State: store}
	arguments := json.RawMessage(`{"castleId":77}`)
	if err := application.markRecruitmentBUPAllianceHelpDue(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.markRecruitmentBUPAllianceHelpCovered(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("payloadless AHR success without lifecycle evidence error=%v, want stale", err)
	}
	protocol := store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 1 || protocol.RecruitmentAHRCoveredSerial != 0 {
		t.Fatalf("missing lifecycle evidence advanced covered serial: %#v", protocol)
	}
}

func TestRecruitmentBUPMarkerStopsWhenReusedCoverageExpired(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		OwnObservedGeneration: 7, OwnRecruitmentObservedGeneration: 7,
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{
				ListID: 91, CastleID: 77, Progress: 1, MaximumHelpers: 3, ObservedAt: now,
			},
		},
	}
	state.Castles[77] = State.CastleState{ID: 77, KingdomID: 1, Focused: true}
	store := State.NewStore(state)
	protocol := store.ProtocolContext()
	if !store.ObserveRecruitmentBUP(77, 7, 3, protocol.FocusEpoch) {
		t.Fatal("initial recruitment BUP was not recorded")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 1 || protocol.RecruitmentAHRCoveredSerial != 1 ||
		!protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("initial recruitment BUP did not inherit current lifecycle coverage: %#v", protocol)
	}
	if _, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
		state.AllianceHelpRequests.OwnRecruitmentRequests[0] = State.RecruitmentAllianceHelpRequest{
			ListID: 91, CastleID: 77, Progress: 3, MaximumHelpers: 3,
			ObservedAt: now, CompletedAt: now.Add(-State.RecruitmentAllianceHelpCompletionGrace),
		}
		return []string{"alliance-help"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	application := &Application{State: store}
	err := application.markRecruitmentBUPAllianceHelpDue(
		t.Context(), json.RawMessage(`{"castleId":77,"requireCovered":true}`),
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("expired reused coverage marker error=%v, want stale", err)
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 2 || protocol.RecruitmentAHRCoveredSerial != 1 {
		t.Fatalf("expired coverage did not leave one uncovered BUP: %#v", protocol)
	}
}

func TestRecruitmentAllianceHelpRequiresQueueFromCommittedCastleSnapshot(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Session.ChangedAt = now.Add(-time.Hour)
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		ObservedAt: now, OwnObservedGeneration: 7,
	}
	state.Castles[77] = State.CastleState{
		ID: 77, KingdomID: 1, Focused: true, ContextSnapshotObservedAt: now,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID, ObservedAt: now.Add(-time.Minute),
				Active: &State.QueueItem{ProductionID: 205, AllianceHelpAvailable: true},
			},
		},
	}
	arguments := json.RawMessage(`{"productionId":205,"castleId":77,"lineId":0}`)
	if _, err := planAllianceHelpRequest(t.Context(), Intent.PlanningContext{State: state}, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("AHR planner accepted a queue omitted by the committed castle snapshot: %v", err)
	}
	protocolContext := State.ProtocolContextState{
		SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77,
		FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4,
	}
	if _, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state, ProtocolContext: protocolContext}, arguments,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("AHR resolver accepted a queue omitted by the committed castle snapshot: %v", err)
	}
}

func TestAllianceHelpRejectsCurrentRetainedUnfocusableCastle(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: false,
	}
	state.Castles[77] = State.CastleState{
		ID: 77, KingdomID: 10,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Active: &State.QueueItem{ProductionID: 204, AllianceHelpAvailable: true},
			},
		},
	}
	arguments := json.RawMessage(`{"productionId":204,"castleId":77,"lineId":2}`)
	if _, err := planAllianceHelpRequest(t.Context(), Intent.PlanningContext{State: state}, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("AHR planner accepted retained unfocusable castle: %v", err)
	}
	if _, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state}, arguments,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("AHR resolver accepted retained unfocusable castle: %v", err)
	}
}

func TestPlanHospitalAllianceHelpStopsAtObservedAccountLimit(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		HospitalProductionIDs: []int64{201},
		ObservedAt:            time.Now().UTC(),
		OwnObservedGeneration: 7,
	}
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Queued: []State.QueueItem{{ProductionID: 201, AllianceHelpRequested: true}},
			},
		},
	}
	state.Castles[88] = State.CastleState{
		ID: 88,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Queued: []State.QueueItem{{ProductionID: 204}},
			},
		},
	}
	plan, err := planAllianceHelpRequest(
		t.Context(), Intent.PlanningContext{State: state}, json.RawMessage(`{"productionId":204}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "an outstanding request") {
		t.Fatalf("alliance-help capacity plan = %#v", plan)
	}
}

func TestHospitalAllianceHelpWaitsForCurrentRequestList(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
	state.Session.ChangedAt = time.Now().UTC()
	state.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			hospitalProductionLineID: {
				LineID: hospitalProductionLineID,
				Queued: []State.QueueItem{{ProductionID: 204}},
			},
		},
	}
	arguments := json.RawMessage(`{"productionId":204,"castleId":77,"lineId":2}`)
	plan, err := planAllianceHelpRequest(t.Context(), Intent.PlanningContext{State: state}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "waiting for the current hospital request list") {
		t.Fatalf("unobserved hospital help plan = %#v", plan)
	}
	if _, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state}, arguments,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("unobserved hospital request list did not stale the resolver: %v", err)
	}
}

func TestRecruitmentAllianceHelpWaitsForCurrentRequestList(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Player.ID = 501
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			recruitmentProductionLineID: {
				LineID: recruitmentProductionLineID, ObservedAt: now,
				Active: &State.QueueItem{ProductionID: 205, AllianceHelpAvailable: true},
			},
		},
	}
	arguments := json.RawMessage(`{"productionId":205,"castleId":77,"lineId":0}`)
	plan, err := planAllianceHelpRequest(t.Context(), Intent.PlanningContext{State: state}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "waiting for the current recruitment request list") {
		t.Fatalf("unobserved recruitment help plan = %#v", plan)
	}
	protocolContext := State.ProtocolContextState{
		SessionGeneration: 7, ConnectionGeneration: 3, FocusedCastleID: 77,
		FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4,
	}
	if _, err := (&Application{}).resolveAllianceHelpRequestStep(
		t.Context(), Intent.PlanningContext{State: state, ProtocolContext: protocolContext}, arguments,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("unobserved recruitment request list did not stale the resolver: %v", err)
	}
}

func TestAllianceHelpGuardReplansAtAuthoritativeHospitalLimit(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
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
		HospitalProductionIDs: []int64{201},
		ObservedAt:            time.Now().UTC(),
		OwnObservedGeneration: 7,
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
	state.Session.Generation = 7
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
		RecruitmentCastleIDs:  []State.CastleID{77},
		ObservedAt:            time.Now().UTC(),
		OwnObservedGeneration: 7,
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 77, Progress: 1, MaximumHelpers: 3, ObservedAt: time.Now().UTC()},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	plan, err := planAllianceHelpRequest(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"productionId":205}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "already has a current request") {
		t.Fatalf("recruitment help duplicate guard plan = %#v", plan)
	}
}

func TestAllianceHelpResolverRejectsNewRecruitQueueWhileCastleRequestIsOutstanding(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 7
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
		RecruitmentCastleIDs:  []State.CastleID{77},
		ObservedAt:            time.Now().UTC(),
		OwnObservedGeneration: 7,
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 77, Progress: 1, MaximumHelpers: 3, ObservedAt: time.Now().UTC()},
		},
		OwnRecruitmentObservedGeneration: 7,
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

func TestMarkAllianceHelpRequestedDoesNotInferRecruitmentSuccess(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 9
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
	if item == nil || item.AllianceHelpRequested {
		t.Fatalf("recruitment help was inferred without the game's own ahh: %#v", item)
	}
	for index, queued := range queue.Queued {
		if queued.AllianceHelpRequested {
			t.Fatalf("queued recruitment item %d was inferred helped: %#v", index, queued)
		}
	}
	if !allianceHelpEligible(application.State.Snapshot(), 101) {
		t.Fatal("unconfirmed recruitment job was made ineligible")
	}
}

func TestMarkHospitalAllianceHelpOnlyMarksMatchingJob(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 9
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
	requestState := application.State.Snapshot().AllianceHelpRequests
	if requestState.OwnObservedGeneration != 9 || len(requestState.HospitalProductionIDs) != 1 ||
		requestState.HospitalProductionIDs[0] != 201 {
		t.Fatalf("confirmed hospital help was not recorded for this session: %#v", requestState)
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
