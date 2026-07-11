package Automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandHarnessPreservesSubmissionIdentityAndShapes(t *testing.T) {
	broker := NewCommandBroker()
	harness := NewCommandHarness(broker)
	receipt := harness.Dispatch(context.Background(), CommandSubmission{
		ContractVersion: CommandContractVersion,
		Command:         "attack_target",
		Intent:          "attack_saved_preset",
		Frames: []CommandFrame{
			{Opcode: "jca", Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":421,"KID":2}%`)},
			{Opcode: "cra", Payload: []byte(`%xt%EmpireEx_21%cra%1%{"LID":7,"TX":100,"TY":200}%`)},
		},
		Options: CommandOptions{Owner: OwnerToolkit, WorkID: "work-421", Priority: PriorityManual},
	})
	if !receipt.Accepted || receipt.SubmissionID == 0 || receipt.Surface != CommandSurfaceRuntime || len(receipt.Frames) != 2 {
		t.Fatalf("dispatch receipt=%+v", receipt)
	}
	if receipt.Frames[0].Shape.Signature != "object{CID:integer,KID:integer}" ||
		receipt.Frames[1].Shape.Signature != "object{LID:integer,TX:integer,TY:integer}" {
		t.Fatalf("wire shapes were not preserved: %+v", receipt.Frames)
	}

	queues := broker.Snapshot()
	focus := queues[LaneCommand]
	attack := queues[LaneAttackLaunch]
	if len(focus) != 1 || len(attack) != 1 {
		t.Fatalf("lane queues command=%d attack=%d", len(focus), len(attack))
	}
	for _, command := range []Command{focus[0], attack[0]} {
		if command.SubmissionID != receipt.SubmissionID || command.Builder != "attack_target" ||
			command.Intent != "attack_saved_preset" || command.WorkID != "work-421" ||
			command.Surface != CommandSurfaceRuntime || command.Opcode == "" || command.RequestShape == "" {
			t.Fatalf("queued command lost harness metadata: %+v", command)
		}
	}

	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"CID":421`) || strings.Contains(string(encoded), `"TX":100`) {
		t.Fatalf("receipt exposed wire values: %s", encoded)
	}
}

func TestCommandHarnessRejectsInvalidOrSessionFramesBeforeQueueing(t *testing.T) {
	broker := NewCommandBroker()
	harness := NewCommandHarness(broker)
	mismatch := harness.Dispatch(context.Background(), CommandSubmission{
		Command: "focus",
		Frames: []CommandFrame{{
			Opcode:  "cra",
			Payload: []byte(`%xt%EmpireEx_21%jca%1%{"CID":1,"KID":0}%`),
		}},
	})
	if mismatch.Accepted || mismatch.Code != "opcode_mismatch" {
		t.Fatalf("opcode mismatch receipt=%+v", mismatch)
	}
	session := harness.Dispatch(context.Background(), CommandSubmission{
		Frames: []CommandFrame{{Payload: []byte(`%xt%lli%1%credential-material%`)}},
	})
	if session.Accepted || session.Code != "session_opcode_forbidden" {
		t.Fatalf("session receipt=%+v", session)
	}
	if len(broker.Snapshot()[LaneCommand]) != 0 || len(broker.Snapshot()[LaneAttackLaunch]) != 0 {
		t.Fatal("rejected submission changed broker queues")
	}
}

func TestCommandHarnessBatchAdmissionIsAtomic(t *testing.T) {
	broker := NewCommandBroker()
	for index := 0; index < maxQueuedCommandsPerLane; index++ {
		if _, ok := broker.submit(Command{
			Owner:   OwnerBackground,
			Lane:    LaneCommand,
			Payload: []byte("occupied"),
		}); !ok {
			t.Fatalf("could not fill command lane at index %d", index)
		}
	}
	harness := NewCommandHarness(broker)
	receipt := harness.Dispatch(context.Background(), CommandSubmission{
		Command: "mixed_batch",
		Frames: []CommandFrame{
			{Payload: []byte(`%xt%EmpireEx_21%cra%1%{}%`)},
			{Payload: []byte(`%xt%EmpireEx_21%gam%1%{}%`)},
		},
	})
	if receipt.Accepted || receipt.Code != "queue_rejected" {
		t.Fatalf("full-lane batch receipt=%+v", receipt)
	}
	if got := len(broker.Snapshot()[LaneAttackLaunch]); got != 0 {
		t.Fatalf("atomic rejection left %d attack frames queued", got)
	}
}

func TestCommandHarnessKeepsMultiFrameCoalescingDistinct(t *testing.T) {
	broker := NewCommandBroker()
	harness := NewCommandHarness(broker)
	submission := CommandSubmission{
		Command: "refresh_equipment",
		Frames: []CommandFrame{
			{Payload: []byte(`%xt%EmpireEx_21%gnr%1%{}%`)},
			{Payload: []byte(`%xt%EmpireEx_21%gei%1%{}%`)},
		},
		Options: CommandOptions{Owner: OwnerBackground, CoalesceKey: "refresh:equipment"},
	}
	first := harness.Dispatch(context.Background(), submission)
	second := harness.Dispatch(context.Background(), submission)
	if !first.Accepted || !second.Accepted {
		t.Fatalf("coalesced receipts first=%+v second=%+v", first, second)
	}
	if got := len(broker.Snapshot()[LaneCommand]); got != 2 {
		t.Fatalf("multi-frame refresh queue length=%d, want 2", got)
	}
	if first.Frames[0].CommandID != second.Frames[0].CommandID ||
		first.Frames[1].CommandID != second.Frames[1].CommandID ||
		first.Frames[0].CommandID == first.Frames[1].CommandID {
		t.Fatalf("frame coalescing identities first=%+v second=%+v", first.Frames, second.Frames)
	}
}

func TestCommandHarnessTraceIdentitySeparatesBrokersAndCoalescesSharedBroker(t *testing.T) {
	previous := CommandTraces
	CommandTraces = NewCommandTraceTracker(16)
	defer func() { CommandTraces = previous }()

	firstBroker := NewCommandBroker()
	secondBroker := NewCommandBroker()
	firstHarness := NewCommandHarness(firstBroker)
	secondHarness := NewCommandHarness(secondBroker)
	submission := CommandSubmission{
		Command: "gam",
		Frames:  []CommandFrame{{Payload: []byte(`%xt%EmpireEx_21%gam%1%{}%`)}},
	}
	first := firstHarness.Dispatch(context.Background(), submission)
	second := secondHarness.Dispatch(context.Background(), submission)
	if !first.Accepted || !second.Accepted || first.Frames[0].CommandID != second.Frames[0].CommandID || first.BrokerID == second.BrokerID {
		t.Fatalf("independent harness receipts first=%+v second=%+v", first, second)
	}
	if traces := CommandTraces.Recent(CommandTraceFilter{Opcode: "gam", Limit: 10}); len(traces) != 2 {
		t.Fatalf("same command IDs from different brokers collapsed into %d traces", len(traces))
	}

	sharedBroker := NewCommandBroker()
	sharedOne := NewCommandHarness(sharedBroker)
	sharedTwo := NewCommandHarness(sharedBroker)
	sharedSubmission := submission
	sharedSubmission.Options.CoalesceKey = "refresh:gam"
	sharedOneReceipt := sharedOne.Dispatch(context.Background(), sharedSubmission)
	sharedTwoReceipt := sharedTwo.Dispatch(context.Background(), sharedSubmission)
	if !sharedOneReceipt.Accepted || !sharedTwoReceipt.Accepted ||
		sharedOneReceipt.Frames[0].CommandID != sharedTwoReceipt.Frames[0].CommandID {
		t.Fatalf("shared broker did not coalesce receipts first=%+v second=%+v", sharedOneReceipt, sharedTwoReceipt)
	}
	traces := CommandTraces.Recent(CommandTraceFilter{Opcode: "gam", Limit: 10})
	if len(traces) != 3 || traces[0].Coalesced != 1 || traces[0].HarnessID != sharedTwoReceipt.HarnessID {
		t.Fatalf("shared-broker trace identity=%+v", traces)
	}
}
