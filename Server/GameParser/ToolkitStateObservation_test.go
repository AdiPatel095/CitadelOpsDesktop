package GameParser

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/Automation"
)

func TestMessageRouterPublishesToolkitStateVersion(t *testing.T) {
	const opcode = "toolkitprobe"
	const stateKey = "state:opcode:" + opcode
	before := Automation.StateSnapshot(stateKey).Version
	MessageRouter(strings.Split("%xt%EmpireEx_21%toolkitprobe%1%{}%", "%"))
	after := Automation.StateSnapshot(stateKey).Version
	if after != before+1 {
		t.Fatalf("state version=%d, want %d", after, before+1)
	}
}
