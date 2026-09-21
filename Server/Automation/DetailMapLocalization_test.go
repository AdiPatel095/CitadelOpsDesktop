package Automation

import (
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func TestDetailMapPresentationIsBoundIsolatedAndInvalidated(t *testing.T) {
	store := State.NewStore(coordinatorReadyState())
	coordinator := NewCoordinator(store, openCoordinatorTestConfiguration(t, "lane"), nil, nil)
	message := Localization.New("supply", "Allocating {amount} units", Localization.Params{"amount": 10})
	decision := Decision{Status: "ready", Detail: "Ready", Details: map[string]string{"supply": "Allocating 10 units"}, DetailsDescriptors: map[string]*Localization.Message{"supply": message, "orphan": message}}
	coordinator.recordDecision("lane", true, decision)
	current := store.ReadOnlyView().Automations["lane"]
	if len(current.DetailsDescriptors) != 1 || current.DetailsDescriptors["supply"].FallbackText != current.Details["supply"] {
		t.Fatal("detail map not bound or orphan retained")
	}
	message.Params["amount"] = 999
	if current.DetailsDescriptors["supply"].Params["amount"] != 10 {
		t.Fatal("source map aliases state")
	}
	snapshot := store.Snapshot()
	snapshot.Automations["lane"].DetailsDescriptors["supply"].Params["amount"] = 999
	if store.ReadOnlyView().Automations["lane"].DetailsDescriptors["supply"].Params["amount"] != 10 {
		t.Fatal("snapshot aliases descriptor params")
	}
	coordinator.updateAutomation("lane", func(next State.AutomationState) State.AutomationState {
		next.Details["supply"] = "Waiting for confirmation"
		return next
	})
	current = store.ReadOnlyView().Automations["lane"]
	if current.Details["supply"] != "Waiting for confirmation" || len(current.DetailsDescriptors) != 0 {
		t.Fatal("stale descriptor concealed changed map detail")
	}
}

func TestDetailMapDescriptorsDoNotChangeDecisionFingerprints(t *testing.T) {
	original := Decision{Status: "waiting", Detail: "Wait", Details: map[string]string{"supply": "Waiting"}}
	decorated := original
	decorated.DetailsDescriptors = map[string]*Localization.Message{"supply": Localization.New("waiting", "Waiting", nil)}
	before, ok := passiveDecisionFingerprint(original)
	after, otherOK := passiveDecisionFingerprint(decorated)
	if !ok || !otherOK || before != after || decisionRequestFingerprint(original) != decisionRequestFingerprint(decorated) {
		t.Fatal("detail map metadata changed scheduling/dispatch identity")
	}
}

func TestAutoFortressVisibleDetailMapCarriesProducerDescriptor(t *testing.T) {
	settings := defaultAutoFortressSettings()
	settings.Kingdoms["2"] = autoFortressKingdom{Enabled: true}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewAutoFortressPolicy().Evaluate(t.Context(), Snapshot{State: State.NewGameState(), GameData: autoFortressTestGameData(t), Now: time.Now().UTC(), Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoFortressSection: raw}}})
	if err != nil {
		t.Fatal(err)
	}
	detail := decision.Details["supplyKingdom2"]
	descriptor := decision.DetailsDescriptors["supplyKingdom2"]
	if detail == "" || descriptor == nil || descriptor.Fallback != detail {
		t.Fatalf("visible map lost producer identity: %#v", decision)
	}
	if decision.Request != nil {
		t.Fatal("presentation metadata enabled a supply action without a castle")
	}
}
