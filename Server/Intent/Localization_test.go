package Intent

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestUnknownAndCoinFailureCannotRetainGenericDescriptor(t *testing.T) {
	engine := &Engine{}
	for _, err := range []error{errors.New("A unique validation reason"), &CoinUnavailableError{Required: 200, Observed: 50, Source: "test"}} {
		receipt := engine.withFailure(Receipt{Status: StatusFailed}, err)
		if receipt.Failure.ExplanationDescriptor != nil {
			t.Fatalf("specific reason masked: %+v", receipt.Failure)
		}
	}
}
func TestOfficialFailureCarriesUsableOfficialKey(t *testing.T) {
	language, _ := GameData.DecodeLanguage([]byte(`{"errorCode_123":"No units available."}`), GameData.LanguageMetadata{})
	receipt := (&Engine{}).withFailure(Receipt{Status: StatusFailed}, &ResponseCodeError{Meaning: GameData.ResolveResponseCode(language, "test", 123)})
	descriptor := receipt.Failure.ExplanationDescriptor
	if descriptor == nil || descriptor.Key != "game.errorCode_123" || descriptor.OfficialKey != "errorCode_123" {
		t.Fatal(descriptor)
	}
}
func TestLocalizedReceiptDurabilityPreservesLegacyEvidence(t *testing.T) {
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	receipt := Receipt{ID: "localized", Intent: "test", Status: StatusSucceeded, SubmittedAt: time.Now().UTC(), Plan: &Plan{Intent: "test", Effect: EffectRead, Summary: "Read castle {literal}", SummaryDescriptor: Localization.New("read", "Read castle {name}", Localization.Params{"name": "{literal}"})}}
	if _, created, err := store.Reserve(t.Context(), "stable-fingerprint", receipt); err != nil || !created {
		t.Fatalf("%t %v", created, err)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		ID   string `json:"id"`
		Plan struct {
			Summary string `json:"summary"`
		} `json:"plan"`
	}
	if err = json.Unmarshal(raw, &legacy); err != nil || legacy.Plan.Summary != receipt.Plan.Summary {
		t.Fatal("legacy fields changed")
	}
	var restored Receipt
	if err = json.Unmarshal(raw, &restored); err != nil || restored.Plan.SummaryDescriptor.Params["name"] != "{literal}" {
		t.Fatal("descriptor did not survive serialization")
	}
}

func TestReceiptHumanizationDiscardsInconsistentFailureDescriptors(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":489,"name":"elitecrossbowman"}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"elitecrossbowman_name":"Veteran Crossbowman"}`), GameData.LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{labelsReady: true, labels: GameData.NewIdentifierLabels(State.GameState{}, store, language)}
	raw := "unit 489"
	descriptor := Localization.New("test", "unit {id}", Localization.Params{"id": "489"})
	result := engine.humanizeReceiptIdentifiers(Receipt{Error: raw, Failure: &FailurePresentation{Message: raw, Explanation: raw, Recovery: raw, MessageDescriptor: descriptor, ExplanationDescriptor: descriptor, RecoveryDescriptor: descriptor}})
	if result.Failure.Message == raw || result.Failure.Explanation == raw || result.Failure.Recovery == raw {
		t.Fatal("fixture did not exercise identifier humanization")
	}
	if result.Failure.MessageDescriptor != nil || result.Failure.ExplanationDescriptor != nil || result.Failure.RecoveryDescriptor != nil {
		t.Fatal("descriptor could reintroduce humanized identifier")
	}
}
