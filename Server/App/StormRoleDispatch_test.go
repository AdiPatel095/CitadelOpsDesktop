package App

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Sophie's D1 reproduction: keep 37 of 100, then clear reserves after the
// pre-dispatch guard but immediately before the transport's final validation.
type stormReserveClearingSender struct {
	config             *Configuration.Store
	replacement        json.RawMessage
	attempted, crossed []json.RawMessage
}

func (*stormReserveClearingSender) Ready() bool       { return true }
func (*stormReserveClearingSender) Namespace() string { return "SyntheticRealm" }
func (s *stormReserveClearingSender) Send(ctx context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now())
	if err != nil {
		return err
	}
	s.attempted = append(s.attempted, frame.Payload)
	if _, err = s.config.Update("automation.autoBird", s.replacement); err != nil {
		return err
	}
	if err = Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	s.crossed = append(s.crossed, frame.Payload)
	return errAutoBirdFinalGuardSentinel
}

func TestStormRoleFinalDispatchRejectsRemovedReserve(t *testing.T) {
	for _, preset := range []string{"", "synthetic"} {
		for _, entries := range []string{`{}`, `{"storm":[]}`, `{"storm":[],"10":[{"id":489,"amount":37}]}`} {
			t.Run(preset+entries, func(t *testing.T) {
				now := time.Now().UTC()
				state, data := autoBirdIntentTestState(t, now)
				state.Castles[10] = State.CastleState{ID: 10, KingdomID: 4, SlotType: 12, Name: "Synthetic Storm", Focused: true, UnitsObservedAt: now, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}}}
				state.Alliance.Holdings[0].KingdomID = 4
				state.Stationing["autoBird:10"] = State.StationingOperation{ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady, SourceCastleID: 10, TargetCastleID: 20, PresetID: preset, DelayHours: 8, UnitsObservedAt: now, AllianceObservedAt: now, UpdatedAt: now}
				configFor := func(entries string) json.RawMessage {
					if preset == "" {
						return json.RawMessage(`{"ignoreSettings":{"settings":` + entries + `}}`)
					}
					return json.RawMessage(`{"activePresetId":"synthetic","presets":{"presets":[{"id":"synthetic","settings":` + entries + `}]}}`)
				}
				config, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.enabled": json.RawMessage(`{"auto_bird":true}`), "automation.autoBird": configFor(`{"storm":[{"id":489,"amount":37}]}`)})
				if err != nil {
					t.Fatal(err)
				}
				app := &Application{State: State.NewStore(&state), Configuration: config}
				sender := &stormReserveClearingSender{config: config, replacement: configFor(entries)}
				registry := Intent.NewRegistry()
				if err := registry.Register(Intent.Definition{Name: "test.storm.final", Effect: Intent.EffectLaunch, Planner: func(_ context.Context, _ Intent.PlanningContext, args json.RawMessage) (Intent.Plan, error) {
					return Intent.Plan{Steps: []Intent.Step{{Name: "Dispatch synthetic Storm", Resolver: "auto_bird.dispatch.build", ResolverArguments: args, AwaitOpcode: "cds"}}}, nil
				}}); err != nil {
					t.Fatal(err)
				}
				engine := Intent.NewEngine(registry, app.State, autoBirdStaticGameData{store: data}, sender, autoBirdNoResponseObserver{})
				if err := engine.RegisterStepResolver("auto_bird.dispatch.build", app.resolveAutoBirdDispatchStep); err != nil {
					t.Fatal(err)
				}
				if err := engine.RegisterAction("auto_bird.batch.guard", app.guardAutoBirdBatch); err != nil {
					t.Fatal(err)
				}
				args, err := json.Marshal(autoBirdCycleRequest{SourceCastleID: 10, TrackingID: "autoBird:10", PresetID: preset, ExpectedTargetCastle: 20, MinimumDelayHours: 6, MaximumDelayHours: 12, DispatchStartedAt: now.Add(-time.Second), Reserves: []stationUnitRequest{{UnitID: 489, Amount: 37}}})
				if err != nil {
					t.Fatal(err)
				}
				receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.storm.final", Actor: "automation:autoBird", AutomationLane: "autoBird", Arguments: args})
				if len(sender.attempted) == 0 {
					t.Fatalf("did not reach final transport guard: %s", receipt.DiagnosticError())
				}
				var wire struct {
					A [][2]int64 `json:"A"`
				}
				if err := json.Unmarshal(sender.attempted[0], &wire); err != nil {
					t.Fatal(err)
				}
				if len(wire.A) != 1 || wire.A[0] != [2]int64{489, 63} {
					t.Fatalf("repro manifest: %s", sender.attempted[0])
				}
				if len(sender.crossed) != 0 {
					t.Fatalf("removed reserves sent troops: %s", sender.crossed[0])
				}
				status := "Auto Bird skips Synthetic Storm: no troops to keep are set for the Storm castle."
				if receipt.Status != Intent.StatusFailed || !strings.Contains(receipt.DiagnosticError(), status) {
					t.Fatalf("missing guard status: %+v", receipt)
				}
				if engine.AutomationLaneLock("autoBird").OperationID != "" || receipt.Failure != nil && receipt.Failure.SafetyLock != nil {
					t.Fatal("reserve guard took a safety lock")
				}
				decision, err := Automation.NewAutoBirdPolicy().Evaluate(t.Context(), Automation.Snapshot{State: app.State.Snapshot(), Configuration: config.Snapshot(), Now: time.Now()})
				if err != nil || decision.Request != nil || decision.Detail != status {
					t.Fatalf("runtime status after dispatch rejection: %+v %v", decision, err)
				}
			})
		}
	}
}
