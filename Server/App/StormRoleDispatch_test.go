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

func TestStormRoleFinalDispatchCurrentPositiveReserve(t *testing.T) {
	for _, mainCastle := range []bool{false, true} {
		for _, preset := range []string{"", "synthetic"} {
			for _, entries := range []string{`{}`, `{"storm":[]}`, `{"storm":[],"10":[{"id":489,"amount":37}]}`, `{"storm":[{"id":489,"amount":0}]}`, `{"storm":[{"id":0,"amount":1}]}`, `{"storm":[{"id":489,"amount":-1}]}`, `{"storm":[{"id":489,"amount":1}]}`} {
				name := preset + entries
				if mainCastle {
					name += " main castle"
				}
				t.Run(name, func(t *testing.T) {
					now := time.Now().UTC()
					state, data := autoBirdIntentTestState(t, now)
					// CIT-133 requires a current free-commander observation before
					// the send can reach the Storm reserve guard under test.
					input := supportCommanderTestInput(t, Intent.PlanningContext{State: state, GameData: data})
					state, data = input.State, input.GameData
					state.Castles[10] = State.CastleState{ID: 10, KingdomID: 4, SlotType: 12, Name: "Synthetic Storm", Focused: true, UnitsObservedAt: now, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}}}
					state.Alliance.Holdings[0].KingdomID = 4
					if mainCastle {
						castle := state.Castles[10]
						castle.KingdomID = 0
						state.Castles[10] = castle
						state.Alliance.Holdings[0].KingdomID = 0
					}
					state.Stationing["autoBird:10"] = State.StationingOperation{ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady, SourceCastleID: 10, TargetCastleID: 20, PresetID: preset, DelayHours: 8, UnitsObservedAt: now, AllianceObservedAt: now, UpdatedAt: now}
					configFor := func(entries string) json.RawMessage {
						if mainCastle {
							entries = strings.ReplaceAll(entries, "storm", "10")
						}
						if preset == "" {
							return json.RawMessage(`{"ignoreSettings":{"settings":` + entries + `}}`)
						}
						return json.RawMessage(`{"activePresetId":"synthetic","presets":{"presets":[{"id":"synthetic","settings":` + entries + `}]}}`)
					}
					config, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.enabled": json.RawMessage(`{"auto_bird":true}`), "automation.autoBird": configFor(`{"storm":[{"id":489,"amount":37}]}`)})
					if err != nil {
						t.Fatal(err)
					}
					app := &Application{State: travelTicketTestStore(&state), Configuration: config}
					sender := &stormReserveClearingSender{config: config, replacement: configFor(entries)}
					registry := Intent.NewRegistry()
					if err := registry.Register(Intent.Definition{Name: "test.storm.final", Effect: Intent.EffectLaunch, Planner: func(_ context.Context, _ Intent.PlanningContext, args json.RawMessage) (Intent.Plan, error) {
						return Intent.Plan{Steps: []Intent.Step{{Name: "Dispatch synthetic Storm", Resolver: "auto_bird.dispatch.build", ResolverArguments: args, AwaitOpcode: "cds"}}}, nil
					}}); err != nil {
						t.Fatal(err)
					}
					engine := Intent.NewEngine(registry, app.State, autoBirdStaticGameData{store: data}, sender, autoBirdNoResponseObserver{})
					engine.SetFinalDispatchProvider(newPremiumCommanderDispatchGate())
					if err := engine.RegisterStepResolver("auto_bird.dispatch.build", app.resolveAutoBirdDispatchStep); err != nil {
						t.Fatal(err)
					}
					if err := engine.RegisterAction("auto_bird.batch.guard", app.guardAutoBirdBatch); err != nil {
						t.Fatal(err)
					}
					args, err := json.Marshal(autoBirdCycleRequest{ConnectionGeneration: app.State.Snapshot().Session.ConnectionGeneration, SourceCastleID: 10, TrackingID: "autoBird:10", PresetID: preset, ExpectedTargetCastle: 20, MinimumDelayHours: 6, MaximumDelayHours: 12, DispatchStartedAt: now.Add(-time.Second), Reserves: []stationUnitRequest{{UnitID: 489, Amount: 37}}})
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
					if engine.AutomationLaneLock("autoBird").OperationID != "" || receipt.Failure != nil && receipt.Failure.SafetyLock != nil {
						t.Fatal("reserve guard took a safety lock")
					}
					if mainCastle || entries == `{"storm":[{"id":489,"amount":1}]}` {
						if len(sender.crossed) != 1 {
							t.Fatalf("permitted reserve did not cross final guard: %s", receipt.DiagnosticError())
						}
						return
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

}

func TestStormRolePositiveAndMainZeroReserveSurplus(t *testing.T) {
	_, data := autoBirdIntentTestState(t, time.Now().UTC())
	for _, test := range []struct {
		name     string
		reserves []stationUnitRequest
		kingdom  State.KingdomID
		want     int64
	}{
		{"main absent", nil, 0, 100},
		{"main zero", []stationUnitRequest{{UnitID: 489, Amount: 0}}, 0, 100},
		{"Storm keep one", []stationUnitRequest{{UnitID: 489, Amount: 1}}, 4, 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			castle := State.CastleState{ID: 10, KingdomID: test.kingdom, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}}}
			manifest, total, err := autoBirdStationManifest(data, castle, test.reserves, false)
			if err != nil || total != test.want || manifest[489] != test.want {
				t.Fatalf("surplus: %#v total=%d error=%v", manifest, total, err)
			}
		})
	}
}
