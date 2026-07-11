package Automation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandTraceRedactsWireValues(t *testing.T) {
	tracker := NewCommandTraceTracker(8)
	command := Command{
		ID:      1,
		Owner:   OwnerToolkit,
		Builder: "alliance_chat",
		Intent:  "send_message",
		Lane:    LaneCommand,
		Payload: []byte(`%xt%EmpireEx_21%acm%1%{"M":"sensitive-message-7f39","RID":987654321}%`),
	}
	tracker.RecordQueued(command)

	traces := tracker.Recent(CommandTraceFilter{Limit: 1})
	if len(traces) != 1 {
		t.Fatalf("trace count=%d, want 1", len(traces))
	}
	trace := traces[0]
	if trace.Opcode != "acm" || trace.RequestShape != "object{M:string,RID:integer}" {
		t.Fatalf("unexpected redacted request description: %+v", trace)
	}
	if strings.Join(trace.RequestFields, ",") != "M,RID" {
		t.Fatalf("request fields=%v, want [M RID]", trace.RequestFields)
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sensitive-message-7f39") || strings.Contains(string(encoded), `"RID":987654321`) {
		t.Fatalf("trace retained request values: %s", encoded)
	}
}

func TestCommandTraceCorrelatesResponseAndState(t *testing.T) {
	tracker := NewCommandTraceTracker(8)
	command := Command{
		ID:       2,
		WorkID:   "work-attack",
		Owner:    OwnerToolkit,
		Builder:  "cra",
		Intent:   "attack_target",
		Priority: PriorityManual,
		Lane:     LaneAttackLaunch,
		Payload:  []byte(`%xt%EmpireEx_21%cra%1%{"KID":0,"TX":100,"TY":200}%`),
	}
	tracker.RecordQueued(command)
	tracker.RecordDispatch(command)
	tracker.RecordSent(command)
	State.Observe(StateMovement, StateOpcode("cra"))
	tracker.RecordResponse("cra", []byte(`{"MID":44332211,"OK":true}`), 0, true)

	traces := tracker.Recent(CommandTraceFilter{Opcode: "cra", Limit: 1})
	if len(traces) != 1 {
		t.Fatalf("trace count=%d, want 1", len(traces))
	}
	trace := traces[0]
	if trace.Status != CommandTraceResponded || trace.ResponseCode == nil || *trace.ResponseCode != 0 {
		t.Fatalf("response was not correlated: %+v", trace)
	}
	if trace.ResponseShape != "object{MID:integer,OK:boolean}" {
		t.Fatalf("response shape=%q", trace.ResponseShape)
	}
	if !containsString(trace.StateChanges, StateMovement) || !containsString(trace.StateChanges, StateOpcode("cra")) {
		t.Fatalf("state changes=%v, want movement and opcode", trace.StateChanges)
	}
	if tracker.Overview().Pending != 0 {
		t.Fatal("responded command remained pending")
	}
}

func TestCommandTraceDeduplicatesMirroredNativeSend(t *testing.T) {
	tracker := NewCommandTraceTracker(8)
	payload := []byte(`%xt%EmpireEx_21%jca%1%{"CID":42,"KID":2}%`)
	command := Command{ID: 3, Owner: OwnerToolkit, Builder: "jca", Intent: "focus_castle", Lane: LaneCommand, Payload: payload}
	tracker.RecordQueued(command)
	tracker.RecordDispatch(command)
	tracker.RecordNativeSent(payload)
	tracker.RecordSent(command)

	traces := tracker.Recent(CommandTraceFilter{Limit: 10})
	if len(traces) != 1 {
		t.Fatalf("mirrored frame produced %d traces, want 1", len(traces))
	}
	if traces[0].Source != CommandTraceSourceCitadel || traces[0].WireObservedAt == nil || traces[0].Status != CommandTraceSent {
		t.Fatalf("mirrored send was not attached to Citadel trace: %+v", traces[0])
	}

	tracker.RecordNativeSent([]byte(`%xt%lli%1%credential-material%`))
	if tracker.Overview().Total != 1 {
		t.Fatal("session opcode was retained in command telemetry")
	}
}

func TestCommandTraceCorrelatesJCAThroughJAAAndDelayedError(t *testing.T) {
	tracker := NewCommandTraceTracker(8)
	command := Command{ID: 4, Owner: OwnerToolkit, Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":42,"KID":2}%`)}
	tracker.RecordQueued(command)
	tracker.RecordSent(command)
	tracker.RecordResponse("jaa", []byte(`{"AID":42}`), 0, true)

	trace := tracker.Recent(CommandTraceFilter{Opcode: "jca", Limit: 1})[0]
	if trace.Status != CommandTraceResponded || trace.ResponseOpcode != "jaa" || tracker.Overview().Pending != 0 {
		t.Fatalf("successful jca/jaa correlation=%+v overview=%+v", trace, tracker.Overview())
	}

	tracker.RecordResponse("jca", nil, 6, true)
	trace = tracker.Recent(CommandTraceFilter{Opcode: "jca", Limit: 1})[0]
	if trace.Status != CommandTraceResponseError || trace.ResponseOpcode != "jca" || trace.ResponseCode == nil || *trace.ResponseCode != 6 {
		t.Fatalf("delayed jca error was not attached to focus trace: %+v", trace)
	}
}

func TestCommandTraceAggregatesShapeVariants(t *testing.T) {
	tracker := NewCommandTraceTracker(8)
	tracker.RecordQueued(Command{ID: 1, Owner: OwnerManual, Builder: "jca", Intent: "focus", Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":1,"KID":0}%`)})
	tracker.RecordQueued(Command{ID: 2, Owner: OwnerToolkit, Builder: "jca", Intent: "focus_castle", Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":999,"KID":4}%`)})
	tracker.RecordQueued(Command{ID: 3, Owner: OwnerToolkit, Builder: "jca_saved", Intent: "focus_castle", Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":"saved","KID":4}%`)})

	variants := tracker.Variants("jca")
	if len(variants) != 2 {
		t.Fatalf("variant count=%d, want 2: %+v", len(variants), variants)
	}
	if variants[0].Count != 2 || variants[0].RequestShape != "object{CID:integer,KID:integer}" {
		t.Fatalf("primary variant=%+v", variants[0])
	}
	if strings.Join(variants[0].Owners, ",") != OwnerManual+","+OwnerToolkit {
		t.Fatalf("variant owners=%v", variants[0].Owners)
	}
}

func TestCommandBrokerSubmissionAndCancellationAreTraced(t *testing.T) {
	previousCommands := Commands
	previousTraces := CommandTraces
	Commands = NewCommandBroker()
	CommandTraces = NewCommandTraceTracker(8)
	defer func() {
		Commands = previousCommands
		CommandTraces = previousTraces
	}()

	commandID, accepted := SubmitCommand(
		[]byte(`%xt%EmpireEx_21%gam%1%{}%`),
		CommandOptions{Owner: OwnerBackground, Builder: "gam_refresh", Intent: "refresh_movements"},
	)
	if !accepted || commandID == 0 {
		t.Fatal("command was not accepted")
	}
	queued := CommandTraces.Recent(CommandTraceFilter{Opcode: "gam", Limit: 1})
	if len(queued) != 1 || queued[0].Status != CommandTraceQueued || queued[0].Builder != "gam_refresh" ||
		queued[0].Intent != "refresh_movements" || queued[0].SubmissionID == 0 ||
		queued[0].Surface != CommandSurfaceInternalApp || queued[0].Opcode != "gam" || queued[0].RequestShape != "object{}" {
		t.Fatalf("queued command trace=%+v", queued)
	}

	if removed := Commands.CancelOwner(OwnerBackground); removed != 1 {
		t.Fatalf("cancelled commands=%d, want 1", removed)
	}
	cancelled := CommandTraces.Recent(CommandTraceFilter{Opcode: "gam", Limit: 1})[0]
	if cancelled.Status != CommandTraceCancelled || cancelled.CompletedAt == nil {
		t.Fatalf("cancelled command lifecycle=%+v", cancelled)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
