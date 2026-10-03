package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type confirmationReplaySender struct {
	pipeline        *Ingest.Pipeline
	opcode, payload string
}

func (*confirmationReplaySender) Ready() bool       { return true }
func (*confirmationReplaySender) Namespace() string { return "EmpireEx_21" }
func (sender *confirmationReplaySender) Send(ctx context.Context, raw []byte) error {
	command, err := Protocol.Decode(string(raw), Protocol.DirectionOutbound, time.Now())
	if err != nil {
		return err
	}
	if command.Opcode == "agb" && string(command.Payload) != `{"GEID":2}` {
		return fmt.Errorf("unexpected purchase payload: %s", command.Payload)
	}
	_, err = sender.pipeline.HandleRawAt(ctx, fmt.Sprintf("%%xt%%%s%%1%%440%%%s%%", sender.opcode, sender.payload), Protocol.DirectionInbound, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	return err
}

func TestConfirmationRequiredEngineCapturedAGB440(t *testing.T) {
	for _, tc := range []struct {
		name, opcode, payload, lane string
		noCodes, wantLock           bool
	}{
		{"captured", "agb", `{"GEID":2,"CC2T":2500}`, "autoBooster", false, false},
		{"no success codes", "agb", `{"CC2T":2500}`, "autoBooster", true, false},
		{"missing quote", "agb", `{"GEID":2}`, "autoBooster", false, true},
		{"fractional quote", "agb", `{"CC2T":2500.5}`, "autoBooster", false, true},
		{"string quote", "agb", `{"CC2T":"2500"}`, "autoBooster", false, true},
		{"negative quote", "agb", `{"CC2T":-1}`, "autoBooster", false, true},
		{"other lane", "agb", `{"CC2T":2500}`, "autoBuyer", false, true},
		{"EUP unchanged", "eup", `{"CC2T":2500}`, "autoBooster", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := State.NewGameState()
			store := State.NewStore(&state)
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &confirmationReplaySender{pipeline: pipeline, opcode: tc.opcode, payload: tc.payload}
			registry := NewRegistry()
			step := Step{Name: "Purchase", Opcode: tc.opcode, AwaitOpcode: tc.opcode, TimeoutMillis: 1000, SuccessCodes: []int{0}, CaptureResponse: true, Command: Protocol.Command{Opcode: tc.opcode, Payload: json.RawMessage(`{"GEID":2}`)}, DefinitiveResponseFailureAction: "test.reject"}
			if tc.noCodes {
				step.SuccessCodes = nil
			}
			if err := registry.Register(Definition{Name: "test.quote", Effect: EffectWrite, Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
				return Plan{Steps: []Step{step}}, nil
			}}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, sender, pipeline)
			compensated := 0
			if err := engine.RegisterAction("test.reject", func(context.Context, json.RawMessage) error { compensated++; return nil }); err != nil {
				t.Fatal(err)
			}
			request := Request{ID: "sanitized-quote", Name: "test.quote", Actor: "automation:autoBooster", AutomationLane: tc.lane}
			receipt := engine.Submit(t.Context(), request)
			lock := engine.AutomationLaneLock(tc.lane)
			if (lock.OperationID != "") != tc.wantLock {
				t.Fatalf("lock=%+v receipt=%+v", lock, receipt)
			}
			if receipt.Status != StatusFailed {
				t.Fatalf("status=%s error=%s", receipt.Status, receipt.Error)
			}
			if tc.wantLock {
				if receipt.Failure == nil || receipt.Failure.SafetyLock == nil {
					t.Fatal("ordinary rejection did not lock")
				}
				return
			}
			if receipt.Failure == nil || receipt.Failure.Toast || receipt.Failure.Explanation != "Daily boost needs confirmation in the game" || receipt.Failure.ExplanationDescriptor == nil || receipt.Failure.ExplanationDescriptor.Key != "server.intent.ruby_confirmation_required" {
				t.Fatalf("failure=%+v", receipt.Failure)
			}
			if len(receipt.Evidence) != 1 || receipt.Evidence[0].Kind != "confirmation_required" {
				t.Fatalf("evidence=%+v", receipt.Evidence)
			}
			var evidence map[string]json.RawMessage
			if err := json.Unmarshal(receipt.Evidence[0].Data, &evidence); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"opcode", "code", "quotedC2", "settingAmount", "settingObservedAt", "rejectedAt"} {
				if _, found := evidence[key]; !found {
					t.Fatalf("missing %s", key)
				}
			}
			if string(evidence["quotedC2"]) != "2500" {
				t.Fatal(evidence)
			}
			if !tc.noCodes && compensated != 1 {
				t.Fatalf("compensation count=%d", compensated)
			}
			// Exercise the returned error itself, after the definitive failure action.
			ctx := context.WithValue(t.Context(), laneSafetyContextKey{}, request)
			_, err := engine.executeStep(ctx, store.ReadOnlyView().Revision, step)
			var confirmation *ConfirmationRequiredError
			var response *ResponseCodeError
			if !errors.As(err, &confirmation) || !errors.As(err, &response) || confirmation.QuotedC2 != 2500 || response.ReceivedAt.IsZero() {
				t.Fatalf("returned error=%T %v", err, err)
			}
		})
	}
}
