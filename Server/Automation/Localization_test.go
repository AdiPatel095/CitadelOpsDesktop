package Automation

import (
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"testing"
)

func TestPresentationDescriptorsDoNotChangeDecisionFingerprints(t *testing.T) {
	original := Decision{Status: "ready", Detail: "Buy two units", Request: &Intent.Request{Name: "buy", Arguments: json.RawMessage(`{"amount":2}`)}}
	localized := original
	localized.DetailDescriptor = Localization.New("buy", "Buy {count} units", Localization.Params{"count": 2})
	localized.FailureDetailDescriptor = Localization.New("failed", "Failed", nil)
	if decisionRequestFingerprint(original) != decisionRequestFingerprint(localized) {
		t.Fatal("presentation changed dispatch identity")
	}
	before, ok := passiveDecisionFingerprint(original)
	after, otherOK := passiveDecisionFingerprint(localized)
	if !ok || !otherOK || before != after {
		t.Fatal("presentation changed passive policy scheduling")
	}
}
