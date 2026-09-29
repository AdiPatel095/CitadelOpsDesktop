package Intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// CIT-13: mapping a pair documents it; it must not bypass the lane lock or
// run any step recovery hook.
func TestCIT13MappedPairsStillLockTheirLaneWithoutRecovery(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{
		"errorCode_95":"This target can't be attacked again yet. Wait for target's cooldown to end or refresh the world map.",
		"errorCode_175":"This place is located in a kingdom which you can no longer access.",
		"errorCode_66":"This message is old and has been deleted."
	}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		opcode  string
		code    int
		lane    string
		source  GameData.ResponseCodeSource
		meaning string
	}{
		{"abi", 95, "autoFortress", GameData.ResponseCodeOfficial, "can't be attacked again yet"},
		{"cra", 95, "autoStorm", GameData.ResponseCodeOfficial, "can't be attacked again yet"},
		{"seq", 214, "autoEquipmentCleanup", GameData.ResponseCodeOfficialClient, "not found in storage"},
		{"sbp", 175, "autoFortress", GameData.ResponseCodeOfficial, "can no longer access"},
		{"ahr", 269, "autoRecruit", GameData.ResponseCodeOfficialClient, "cannot receive alliance help"},
		{"bsd", 130, "reportLane", GameData.ResponseCodeOfficialClient, "no spy data"},
		{"bsd", 66, "reportLane", GameData.ResponseCodeOfficial, "has been deleted"},
	} {
		t.Run(fmt.Sprintf("%s-%d", test.opcode, test.code), func(t *testing.T) {
			if State.AutomationRejectionWhitelisted(test.opcode, test.code) || rejectionAllowsRecovery(test.opcode, test.code) {
				t.Fatalf("%s %d entered the safety whitelist", test.opcode, test.code)
			}
			store := State.NewStore(State.NewGameState())
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &responseSequenceSender{pipeline: pipeline, responseCodes: []int{test.code}}
			registry := NewRegistry()
			step := Step{
				Name: "Rejected command", Opcode: test.opcode, AwaitOpcode: test.opcode, TimeoutMillis: 1000,
				SuccessCodes:  []int{0},
				ResponseRetry: &ResponseRetryPolicy{Codes: []int{test.code}, GuardAction: "test.recovery", DelayMillis: 1},
				Command:       Protocol.Command{Opcode: test.opcode, Payload: json.RawMessage(`{}`)},
			}
			if err := registry.Register(Definition{Name: "test.cit13", Effect: EffectWrite,
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					return Plan{Steps: []Step{step}}, nil
				}}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, localizedGameDataProvider{store: gameData, language: language}, sender, pipeline)
			recovery := 0
			if err := engine.RegisterAction("test.recovery", func(context.Context, json.RawMessage) error { recovery++; return nil }); err != nil {
				t.Fatal(err)
			}
			receipt := engine.Submit(t.Context(), Request{ID: fmt.Sprintf("cit13-%s-%d", test.opcode, test.code), Name: "test.cit13", Actor: "automation:shared", AutomationLane: test.lane})
			if receipt.Status != StatusFailed || receipt.Failure == nil || receipt.Failure.SafetyLock == nil {
				t.Fatalf("mapped rejection did not lock the lane: %#v", receipt)
			}
			lock := store.ReadOnlyView().Automations[test.lane].SafetyLock
			if lock.Opcode != test.opcode || lock.Code != test.code || lock.OperationID != receipt.ID ||
				lock.MeaningSource != string(test.source) || !strings.Contains(strings.ToLower(lock.Meaning), strings.ToLower(test.meaning)) ||
				!lock.ExpiresAt().Equal(lock.ObservedAt.Add(State.AutomationSafetyLockDuration)) {
				t.Fatalf("lane lock = %#v", lock)
			}
			if sends, _ := sender.snapshot(); sends != 1 || recovery != 0 {
				t.Fatalf("sends=%d recovery=%d", sends, recovery)
			}
		})
	}
}
