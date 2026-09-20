package App

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestSupportResolversBatchEveryTroopExactlyOnce(t *testing.T) {
	for _, count := range []int{1, 10, 11, 20, 21, 26} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			now := time.Now().UTC()
			state, _ := autoBirdIntentTestState(t, now)
			amounts := map[State.UnitID]int64{}
			units := []stationUnitRequest{}
			catalog := []map[string]any{}
			for i := count; i >= 1; i-- {
				id := State.UnitID(1000 + i)
				amounts[id] = int64(i) * 100000
				units = append(units, stationUnitRequest{UnitID: id, Amount: amounts[id]})
				catalog = append(catalog, map[string]any{"wodID": id})
			}
			raw, _ := json.Marshal(map[string]any{"versionInfo": []any{}, "buildings": []any{}, "units": catalog})
			data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "test"})
			if err != nil {
				t.Fatal(err)
			}
			state.Castles[10] = State.CastleState{ID: 10, Focused: true, UnitsObservedAt: now, Units: State.CastleUnits{Stationed: amounts}}
			state.Stationing["autoBird:10"] = State.StationingOperation{ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady, SourceCastleID: 10, TargetCastleID: 20, DelayHours: 6, Units: amounts, UnitsObservedAt: now}
			app := &Application{State: State.NewStore(state)}
			input := Intent.PlanningContext{State: state, GameData: data}
			manualArgs, _ := json.Marshal(stationRequest{SourceCastleID: 10, TargetCastleID: 20, DelayHours: 6, Units: units})
			manual, err := resolveTroopsStationStep(t.Context(), input, manualArgs)
			if err != nil {
				t.Fatal(err)
			}
			birdArgs, _ := json.Marshal(autoBirdCycleRequest{SourceCastleID: 10, TrackingID: "autoBird:10"})
			bird, err := app.resolveAutoBirdDispatchStep(t.Context(), input, birdArgs)
			if err != nil {
				t.Fatal(err)
			}
			for _, variant := range []struct {
				name string
				step Intent.Step
			}{{"manual", manual}, {"autoBird", bird}} {
				resolved := variant.step
				steps := resolved.Batch
				if len(steps) == 0 {
					steps = []Intent.Step{resolved}
				}
				got := map[State.UnitID]int64{}
				batches := 0
				lastID := int64(0)
				for _, step := range steps {
					if step.Action != "" {
						continue
					}
					batches++
					var payload struct {
						SID int
						TX  int
						TY  int
						LID int
						WT  int
						HBW int
						BPC int
						PTT int
						SD  int
						A   [][2]int64
					}
					if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					if len(payload.A) == 0 || len(payload.A) > 10 {
						t.Fatalf("oversized/empty batch: %v", payload.A)
					}
					if variant.name == "autoBird" {
						if step.PreDispatchAction != "auto_bird.batch.guard" ||
							step.FinalDispatchAction != "auto_bird.batch.guard" ||
							string(step.PreDispatchArguments) != string(step.FinalDispatchArguments) {
							t.Fatal("Auto Bird batch lost its pre/final safety guard")
						}
						if err := app.guardAutoBirdBatch(t.Context(), step.PreDispatchArguments); err != nil {
							t.Fatalf("valid %d-type Auto Bird batch rejected by its actual pre-dispatch guard: %v", len(payload.A), err)
						}
						var guard autoBirdBatchGuardRequest
						if err := json.Unmarshal(step.PreDispatchArguments, &guard); err != nil {
							t.Fatal(err)
						}
						if string(guard.Payload) != string(step.Command.Payload) || guard.Cycle.SourceCastleID != 10 || guard.Cycle.TrackingID != "autoBird:10" {
							t.Fatal("Auto Bird guard does not carry the exact command and cycle")
						}
						for _, invalidation := range []string{"focus", "inventory", "pause", "control-revision"} {
							changed := app.State.Snapshot()
							castle := changed.Castles[10]
							switch invalidation {
							case "focus":
								castle.Focused = false
							case "inventory":
								castle.Units.Stationed[State.UnitID(payload.A[0][0])] = payload.A[0][1] - 1
							case "pause", "control-revision":
								control := State.StationingOperation{ID: State.AutoBirdControlID(10), Purpose: "autoBirdControl", SourceCastleID: 10}
								if invalidation == "pause" {
									control.Paused = true
								} else {
									control.UpdatedAt = now
								}
								changed.Stationing[control.ID] = control
							}
							changed.Castles[10] = castle
							rejected := &Application{State: State.NewStore(changed)}
							if err := rejected.guardAutoBirdBatch(t.Context(), step.PreDispatchArguments); err == nil {
								t.Fatalf("Auto Bird batch guard accepted changed %s", invalidation)
							}
						}
					}
					if payload.SID != 10 || payload.TX != 20 || payload.TY != 20 || payload.LID != -14 || payload.WT != 6 || payload.HBW != -1 || payload.BPC != 1 || payload.PTT != 1 || payload.SD != 0 {
						t.Fatalf("route/options changed: %+v", payload)
					}
					if step.ResponseBarrier != Intent.ResponseBarrierCommitted {
						t.Fatal("batch does not await committed response")
					}
					for _, pair := range payload.A {
						if pair[0] <= lastID {
							t.Fatal("duplicate or unsorted unit")
						}
						lastID = pair[0]
						got[State.UnitID(pair[0])] = pair[1]
					}
				}
				if batches != (count+9)/10 || !reflect.DeepEqual(got, amounts) {
					t.Fatalf("troops lost or duplicated: %v", got)
				}
			}
		})
	}
}

func TestSupportBatchesTrackAllMovementsAndLatestReturn(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Alliance.Holdings = []State.AllianceHolding{{CastleID: 20, X: 40, Y: 50}}
	state.Stationing["autoBird:10"] = State.StationingOperation{ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady, SourceCastleID: 10, TargetCastleID: 20, DelayHours: 6}
	latest := now.Add(8 * time.Hour)
	for i := 0; i < 3; i++ {
		end := now.Add(time.Duration(8-i) * time.Hour)
		id := State.MovementID(30 + i)
		state.Movements[id] = State.MovementState{ID: id, SourceCastleID: 10, TargetCastleID: 20, TargetX: 40, TargetY: 50, ObservedAt: now, StartedAt: now, ReturnsAt: &end, Units: map[State.UnitID]int64{State.UnitID(i + 1): 100}}
	}
	app := &Application{State: State.NewStore(state)}
	args, _ := json.Marshal(autoBirdCycleRequest{SourceCastleID: 10, TrackingID: "autoBird:10", DispatchStartedAt: now})
	if err := app.captureAutoBirdMovement(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	op := app.State.Snapshot().Stationing["autoBird:10"]
	if len(op.MovementIDs) != 3 || len(op.Units) != 3 || op.ExpectedReturnAt == nil || !op.ExpectedReturnAt.Equal(latest) {
		t.Fatalf("lost a batch: %+v", op)
	}
	// The legacy newest movement may return first; older batches must stay active.
	delete(state.Movements, 32)
	delete(state.Movements, 31)
	if !op.ActiveInState(state, now) {
		t.Fatal("earlier batch still active but tracking ended")
	}
	snap := app.State.Snapshot()
	snap.Stationing["autoBird:10"].MovementIDs[0] = 999
	if app.State.Snapshot().Stationing["autoBird:10"].MovementIDs[0] == 999 {
		t.Fatal("snapshot aliases movement IDs")
	}
}

func TestSupportBatchGuardRejectsChangedInventory(t *testing.T) {
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{ID: 10, Focused: true, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 99}}}
	app := &Application{State: State.NewStore(state)}
	if err := app.guardSupportBatch(t.Context(), json.RawMessage(`{"SID":10,"A":[[215,100]]}`)); err == nil {
		t.Fatal("stale batch accepted")
	}
}

func TestAutoStationTracksEveryAcceptedBatch(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Alliance.Holdings = []State.AllianceHolding{{CastleID: 20, X: 40, Y: 50}}
	units := []stationUnitRequest{{UnitID: 215, Amount: 100}, {UnitID: 216, Amount: 200}}
	latest := now.Add(8 * time.Hour)
	for i, id := range []State.MovementID{30, 31} {
		end := now.Add(time.Duration(8-i) * time.Hour)
		state.Movements[id] = State.MovementState{ID: id, SourceCastleID: 10, TargetCastleID: 20, TargetX: 40, TargetY: 50, StartedAt: now, ReturnsAt: &end, Units: map[State.UnitID]int64{units[i].UnitID: units[i].Amount}}
	}
	app := &Application{State: State.NewStore(state)}
	args, _ := json.Marshal(stationRequest{SourceCastleID: 10, TargetCastleID: 20, TrackingID: "autoStation:10", Purpose: "autoStation", DelayHours: 6, DispatchStartedAt: now, Units: units})
	if err := app.trackStationMovement(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	op := app.State.Snapshot().Stationing["autoStation:10"]
	if len(op.MovementIDs) != 2 || len(op.Units) != 2 || op.SuccessCooldownUntil == nil || !op.SuccessCooldownUntil.Equal(latest) {
		t.Fatalf("partial tracking: %+v", op)
	}
}
