package GameCommands

import (
	"context"
	"strings"
	"testing"

	"CitadelDesktop/Server/Automation"
)

func TestQueueOutgoingPayloadRoutesOnlyCRAToAttackLane(t *testing.T) {
	previous := Automation.Commands
	Automation.Commands = Automation.NewCommandBroker()
	defer func() { Automation.Commands = previous }()

	receipt := DispatchPayload(context.Background(), "cra", "scheduled_attack", `%xt%EmpireEx_21%cra%1%{}%`, Automation.CommandOptions{Owner: Automation.OwnerAttack})
	if !receipt.Accepted || receipt.SubmissionID == 0 || len(receipt.Frames) != 1 || receipt.Frames[0].Shape.Signature != "object{}" {
		t.Fatalf("cra command receipt=%+v", receipt)
	}
	if !QueueOutgoingPayloadWithOptions(`%xt%EmpireEx_21%gam%1%{}%`, Automation.CommandOptions{Owner: Automation.OwnerBackground}) {
		t.Fatal("ordinary command was rejected")
	}

	queues := Automation.Commands.Snapshot()
	if got := len(queues[Automation.LaneAttackLaunch]); got != 1 {
		t.Fatalf("attack lane length=%d; want 1", got)
	}
	if got := len(queues[Automation.LaneCommand]); got != 1 {
		t.Fatalf("ordinary command lane length=%d; want 1", got)
	}
	if command := queues[Automation.LaneAttackLaunch][0]; command.Builder != "cra" || command.Intent != "scheduled_attack" {
		t.Fatalf("attack command metadata=%+v, want opcode-derived builder and intent", command)
	}
	if command := queues[Automation.LaneCommand][0]; command.Builder != "gam" || command.Intent != "gam" {
		t.Fatalf("ordinary command metadata=%+v, want opcode-derived builder and intent", command)
	}
	if queues[Automation.LaneAttackLaunch][0].Surface != Automation.CommandSurfaceInternalApp ||
		queues[Automation.LaneCommand][0].Surface != Automation.CommandSurfaceInternalApp {
		t.Fatalf("internal queue commands did not retain their surface: %+v", queues)
	}
}

func TestFeatureCommandOptionsPreserveHighLevelIntent(t *testing.T) {
	options := featureCommandOptions(Automation.OwnerAutoTCI, nil)
	if options.Owner != Automation.OwnerAutoTCI || options.Intent != Automation.OwnerAutoTCI {
		t.Fatalf("feature command options=%+v, want owner-derived intent", options)
	}
}

func TestRefreshCoalesceKeyDoesNotRetainWireValues(t *testing.T) {
	previous := Automation.Commands
	Automation.Commands = Automation.NewCommandBroker()
	defer func() { Automation.Commands = previous }()
	payload := `%xt%EmpireEx_21%jca%1%{"CID":987654321,"KID":2}%`
	if !QueueBackgroundRefresh(payload) || !QueueBackgroundRefresh(payload) {
		t.Fatal("background refresh was rejected")
	}
	queued := Automation.Commands.Snapshot()[Automation.LaneCommand]
	if len(queued) != 1 {
		t.Fatalf("identical refreshes queued %d commands, want 1", len(queued))
	}
	if !strings.HasPrefix(queued[0].CoalesceKey, "refresh:") || strings.Contains(queued[0].CoalesceKey, "987654321") || strings.Contains(queued[0].CoalesceKey, "%") {
		t.Fatalf("unsafe refresh coalesce key %q", queued[0].CoalesceKey)
	}
}
