package Automation

import (
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"strings"
	"testing"
)

func TestNomadEventVariantsPreserveIdentityAndUserContent(t *testing.T) {
	for _, eventID := range []int64{nomadEventID, samuraiEventID} {
		message := nomadEventDescriptor(eventID, "advisor_launch", Localization.Params{"attacks": 3, "x": "001", "y": "100"})
		if message == nil || message.Params["attacks"] != 3 || message.Params["x"] != "001" || !strings.Contains(message.Fallback, nomadEventName(eventID)) {
			t.Fatalf("lost identity/quantity: %+v", message)
		}
		if _, exists := message.Params["event"]; exists {
			t.Fatal("event noun leaked into primitive")
		}
		named := nomadDiscoverDescriptor(eventID, State.CastleState{ID: 17, Name: "Castle {admin}"})
		if named == nil || named.Params["castle"] != "Castle {admin}" || strings.Contains(named.Fallback, "{admin}") {
			t.Fatalf("user text became template: %+v", named)
		}
		unnamed := nomadDiscoverDescriptor(eventID, State.CastleState{ID: 17})
		if unnamed == nil || unnamed.Params["castleID"] != "17" || !strings.Contains(unnamed.Fallback, "castle {castleID}") {
			t.Fatalf("ID fallback lost: %+v", unnamed)
		}
	}
	if nomadEventDescriptor(999, "advisor_launch", nil) != nil || nomadEventDescriptor(nomadEventID, "unknown", nil) != nil {
		t.Fatal("unknown event/template inferred")
	}
}
