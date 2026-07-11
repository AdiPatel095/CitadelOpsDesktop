package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Automation"
)

func TestAutomationStateKeysForAINIncludeAllianceIdentity(t *testing.T) {
	keys := automationStateKeysForFrame("ain", `{"A":{"AID":42}}`)
	want := Automation.StateEntity("alliance", 42)
	for _, key := range keys {
		if key == want {
			return
		}
	}
	t.Fatalf("AIN state keys %v do not include %q", keys, want)
}
