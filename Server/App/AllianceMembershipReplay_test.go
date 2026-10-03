package App

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func helpMembershipReplayState(now time.Time) State.GameState {
	state := State.NewGameState()
	state.Player.ID = 7
	state.Player.AllianceID = 9
	state.Player.AllianceMembershipObservedAt = now
	state.Player.AllianceMembershipGeneration = 7
	state.Session.Generation = 7
	state.Session.ConnectionGeneration = 3
	state.Session.BaselineGeneration = 7
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.AllianceHelpRequests = State.AllianceHelpRequestState{
		ObservedAt: now, OwnObservedGeneration: 7,
		OthersObservedAt: now, OthersObservedGeneration: 7,
		PendingOtherListIDs: []int64{101},
	}
	state.Castles[77] = State.CastleState{ID: 77, KingdomID: 1, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, ObservedAt: now, Active: &State.QueueItem{ProductionID: 201, Amount: 5}},
			2: {LineID: 2, ObservedAt: now, Active: &State.QueueItem{ProductionID: 205, Amount: 5}},
		},
	}
	return state
}

func helpMembershipReplayPipeline(t *testing.T, state State.GameState) (*State.Store, *Ingest.Pipeline) {
	t.Helper()
	store := State.NewStore(&state)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	return store, Ingest.NewPipeline(store, nil, registry)
}

func helpMembershipReplayInput(store *State.Store) Intent.PlanningContext {
	view := store.PlanningView()
	return Intent.PlanningContext{State: view.State, ProtocolContext: view.ProtocolContext}
}

func assertAllHelpLocallyBlocked(t *testing.T, input Intent.PlanningContext) {
	t.Helper()
	for _, tc := range []struct {
		name    string
		planner Intent.Planner
		args    json.RawMessage
	}{
		{"answer all bootstrap", planAllianceHelpAnswerAll, json.RawMessage(`{"allowUnobserved":true}`)},
		{"hospital", planAllianceHelpRequest, json.RawMessage(`{"productionId":205}`)},
		{"standalone recruitment", planAllianceHelpRequest, json.RawMessage(`{"productionId":201}`)},
	} {
		plan, err := tc.planner(t.Context(), input, tc.args)
		if err != nil || len(plan.Steps) != 0 || len(plan.Claims) != 0 || plan.SummaryDescriptor == nil ||
			plan.Summary != "Alliance help is paused: you aren't in an alliance" {
			t.Fatalf("%s planned a send instead of local block: %+v err=%v", tc.name, plan, err)
		}
	}
	app := &Application{}
	for _, tc := range []struct {
		name     string
		resolver Intent.StepResolver
		args     json.RawMessage
	}{
		{"answer all", resolveAllianceHelpAnswerAllStep, json.RawMessage(`{"sessionGeneration":7,"allowUnobserved":true}`)},
		{"hospital", app.resolveAllianceHelpRequestStep, json.RawMessage(`{"productionId":205,"castleId":77,"lineId":2}`)},
		{"standalone recruitment", app.resolveAllianceHelpRequestStep, json.RawMessage(`{"productionId":201,"castleId":77,"lineId":0}`)},
		{"post BUP", app.resolveRecruitmentBUPAllianceHelpStep, json.RawMessage(`{"castleId":77}`)},
	} {
		step, err := tc.resolver(t.Context(), input, tc.args)
		if !errors.Is(err, Intent.ErrPlanStale) || step.Command.Opcode != "" || step.Opcode != "" {
			t.Fatalf("%s resolver did not locally block: %+v err=%v", tc.name, step, err)
		}
	}
}

// Replay the sanitized 2026-09-24 AHA/270 -> hospital AHR/114 capture.
// IDs are synthetic; sender payload shapes and the one-second offset are kept.
// Use a current clock origin so production-queue freshness stays realistic.
func TestCapturedAHA270ThenAHR114ReplayBlocksHospitalAndResumes(t *testing.T) {
	base := time.Now().UTC()
	store, pipeline := helpMembershipReplayPipeline(t, helpMembershipReplayState(base))
	input := helpMembershipReplayInput(store)
	ahaPlan, err := planAllianceHelpAnswerAll(t.Context(), input, json.RawMessage(`{"allowUnobserved":true}`))
	if err != nil || len(ahaPlan.Steps) != 2 {
		t.Fatalf("AHA plan: %+v %v", ahaPlan, err)
	}
	hospitalPlan, err := planAllianceHelpRequest(t.Context(), input, json.RawMessage(`{"productionId":205}`))
	if err != nil || len(hospitalPlan.Steps) != 3 {
		t.Fatalf("hospital plan: %+v %v", hospitalPlan, err)
	}
	aha, err := resolveAllianceHelpAnswerAllStep(t.Context(), input, ahaPlan.Steps[0].ResolverArguments)
	if err != nil || string(aha.Command.Payload) != `{"KID":15}` {
		t.Fatalf("sender-shaped AHA: %+v %v", aha, err)
	}
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%aha%1%270%{"KID":15}%`, Protocol.DirectionInbound, base); err != nil {
		t.Fatal(err)
	}
	input = helpMembershipReplayInput(store)
	hospital, err := (&Application{}).resolveAllianceHelpRequestStep(t.Context(), input, hospitalPlan.Steps[1].ResolverArguments)
	if !errors.Is(err, Intent.ErrPlanStale) || hospital.Command.Opcode != "" {
		t.Fatalf("due hospital AHR escaped AHA/270: %+v %v", hospital, err)
	}
	assertAllHelpLocallyBlocked(t, input)
	// Also replay the captured in-flight rejection. No new AHR was dispatched.
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%ahr%1%114%{"ID":205,"T":2}%`, Protocol.DirectionInbound, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	assertAllHelpLocallyBlocked(t, helpMembershipReplayInput(store))
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%gbd%1%0%{"gpi":{"PID":7},"gal":{"AID":9}}%`, Protocol.DirectionInbound, base.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	input = helpMembershipReplayInput(store)
	if !State.AllianceMembershipCurrent(&input.State) {
		t.Fatal("fresh GBD did not restore membership")
	}
	if _, err := resolveAllianceHelpAnswerAllStep(t.Context(), input, ahaPlan.Steps[0].ResolverArguments); err != nil {
		t.Fatalf("AHA did not resume: %v", err)
	}
	if step, err := (&Application{}).resolveAllianceHelpRequestStep(t.Context(), input, hospitalPlan.Steps[1].ResolverArguments); err != nil || step.Command.Opcode != "ahr" {
		t.Fatalf("hospital did not resume: %+v %v", step, err)
	}
	if step, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(t.Context(), input, json.RawMessage(`{"castleId":77}`)); err != nil || step.Command.Opcode != "ahr" {
		t.Fatalf("post-BUP did not resume: %+v %v", step, err)
	}
	for _, automation := range input.State.Automations {
		if !automation.SafetyLock.ObservedAt.IsZero() {
			t.Fatal("local membership block created a lane safety lock")
		}
	}
}

func TestHelpBlocksUnknownAndPreviousSessionMembership(t *testing.T) {
	for _, tc := range []struct {
		name       string
		generation uint64
		observed   bool
	}{
		{"unknown bootstrap", 7, false}, {"previous session", 6, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := helpMembershipReplayState(time.Now().UTC())
			state.Player.AllianceMembershipGeneration = tc.generation
			if !tc.observed {
				state.Player.AllianceMembershipObservedAt = time.Time{}
			}
			store, _ := helpMembershipReplayPipeline(t, state)
			assertAllHelpLocallyBlocked(t, helpMembershipReplayInput(store))
		})
	}
}

type membershipBUPSender struct {
	pipeline *Ingest.Pipeline
	opcodes  []string
}

func (*membershipBUPSender) Ready() bool       { return true }
func (*membershipBUPSender) Namespace() string { return "EmpireEx_21" }
func (sender *membershipBUPSender) Send(ctx context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, frame.Opcode)
	code := 0
	metadata := Outbound.MetadataFromContext(ctx)
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{Direction: Protocol.DirectionInbound,
		Opcode: frame.Opcode, ResponseCode: &code, Payload: json.RawMessage(`{}`), ReceivedAt: time.Now().UTC(),
		ResponseToken: metadata.ResponseToken, CausationOperationID: metadata.OperationID,
	})
	return err
}

func TestPostBUPMembershipBlockPreservesBUPWithoutLaneLock(t *testing.T) {
	state := helpMembershipReplayState(time.Now().UTC())
	state.Player.AllianceMembershipObservedAt = time.Time{}
	store, pipeline := helpMembershipReplayPipeline(t, state)
	sender := &membershipBUPSender{pipeline: pipeline}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "membership.bup", Effect: Intent.EffectWrite,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			bup := commandStep("Recruit", "bup", json.RawMessage(`{"LID":0,"WID":1,"AMT":5}`), "bup", nil)
			return Intent.Plan{Steps: appendRecruitmentBUPAllianceHelpSteps([]Intent.Step{bup}, json.RawMessage(`{"castleId":77}`))}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, store, nil, sender, pipeline)
	if err := engine.RegisterStepResolver("production.enqueue.alliance_help.build", (&Application{}).resolveRecruitmentBUPAllianceHelpStep); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	receipt := engine.Submit(ctx, Intent.Request{Name: "membership.bup", AutomationLane: "autoRecruit"})
	if len(sender.opcodes) != 1 || sender.opcodes[0] != "bup" || receipt.Status != Intent.StatusPartiallySucceeded {
		t.Fatalf("BUP was affected or AHR sent: opcodes=%v receipt=%+v", sender.opcodes, receipt)
	}
	if !store.ReadOnlyView().Automations["autoRecruit"].SafetyLock.ObservedAt.IsZero() {
		t.Fatal("post-BUP local block created a safety lock")
	}
}
