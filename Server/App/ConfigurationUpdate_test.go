package App

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Configuration"
)

func TestConfigurationIntentAcceptsLegacyExpectedFeastAndValidatesProposedWrite(t *testing.T) {
	legacy := json.RawMessage(`{"version":1,"checkIntervalSec":1800,"feast":{"enabled":true,"minimumRemainingHours":0}}`)
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.autoBuyer": legacy})
	if err != nil {
		t.Fatal(err)
	}
	disabled := json.RawMessage(`{"version":1,"checkIntervalSec":1800,"feast":{"enabled":false,"minimumRemainingHours":0}}`)
	arguments, err := json.Marshal(configurationUpdate{
		Section: "automation.autoBuyer", Value: disabled, ExpectedValue: &legacy,
	})
	if err != nil {
		t.Fatal(err)
	}
	update, err := decodeConfigurationUpdate(arguments)
	if err != nil {
		t.Fatalf("legacy expected value rejected before intent write: %v", err)
	}
	if _, err := store.UpdateConditional(update.Section, update.Value, update.ExpectedRevision, update.ExpectedValue); err != nil {
		t.Fatalf("legacy feast disable rejected at intent write: %v", err)
	}

	invalid := json.RawMessage(`{"version":1,"feast":{"enabled":true,"minimumRemainingHours":1.5}}`)
	arguments, err = json.Marshal(configurationUpdate{Section: "automation.autoBuyer", Value: invalid})
	if err != nil {
		t.Fatal(err)
	}
	update, err = decodeConfigurationUpdate(arguments)
	if err != nil {
		t.Fatalf("generic intent decoding applied write policy too early: %v", err)
	}
	if _, err := store.UpdateConditional(update.Section, update.Value, nil, nil); err == nil {
		t.Fatal("intent mutation boundary accepted a fractional enabled feast floor")
	}
}
