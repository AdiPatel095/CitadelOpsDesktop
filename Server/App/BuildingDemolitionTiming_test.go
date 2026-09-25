package App

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const demolitionIntentCatalog = `{"versionInfo":[],"units":[],"buildings":[{"wodID":249,"name":"FactionMarket","group":"Building","width":6,"height":9,"buildDuration":9000}],"currencies":[{"currencyID":1003,"Name":"10MinSkip","JSONKey":"MS3"}],"currencyMinutesSkipValues":[{"currencyID":1003,"MinutesSkipValue":"10","MinuteSkipIndex":"2"}]}`

func TestDemolitionCompletionResolversRejectCompletedOrUnknownTiming(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(demolitionIntentCatalog), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		progress int64
		boost    float64
		free     bool
		allowed  bool
	}{{"skip positive", 3622, 100, false, true}, {"skip complete", 4500, 100, false, false}, {"skip boosted complete", 2250, 200, false, false}, {"skip missing boost", 1, 0, false, false}, {"free positive", 4490, 100, true, true}, {"free complete", 4500, 100, true, false}, {"free unknown", 1, 0, true, false}, {"free too early", 4000, 100, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			state := buildingIntentState()
			castle := state.Castles[10]
			castle.Layout.ObservedAt = time.Now()
			castle.BuildingQueue.ObservedAt = castle.Layout.ObservedAt
			b := castle.Layout.Objects[42]
			b.DefinitionID = 249
			b.ConstructionState = State.BuildingStateDisassembleInProgress
			b.ProgressSec = tc.progress
			b.ConstructionBoostPercent = tc.boost
			castle.Layout.Objects[42] = b
			castle.Buildings[42] = b
			castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{Status: State.BuildingQueueSlotOccupied, BuildingID: 42}
			state.Castles[10] = castle
			input := Intent.PlanningContext{State: state, GameData: data}
			args := json.RawMessage(`{"castleId":10,"buildingInstanceId":42,"minutes":10}`)
			var step Intent.Step
			var err error
			if tc.free {
				args = json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`)
				step, err = resolveBuildingFinishFreeStep(context.Background(), input, args)
			} else {
				step, err = resolveBuildingTimeSkipStep(context.Background(), input, args)
			}
			if !tc.allowed {
				if !errors.Is(err, Intent.ErrPlanStale) {
					t.Fatalf("expected stale rejection, got step%+v error%v", step, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := "building.skip_time.guard"
			if tc.free {
				expected = "building.finish_free.guard"
			}
			if step.FinalDispatchAction != expected || len(step.FinalDispatchArguments) == 0 {
				t.Fatal("demolition lacks final dispatch recheck", step)
			}
			// The same dispatch-time validator rejects a process completed while queued.
			b.ProgressSec = 4500
			castle.Layout.Objects[42] = b
			castle.Buildings[42] = b
			input.State.Castles[10] = castle
			if tc.free {
				_, _, _, err = validatedBuildingFinishFree(input, buildingInstanceIntentRequest{CastleID: 10, BuildingInstanceID: 42}, true)
			} else {
				_, _, _, _, err = validatedBuildingTimeSkip(input, buildingTimeSkipIntentRequest{CastleID: 10, BuildingInstanceID: 42, Minutes: 10}, true)
			}
			if !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatalf("queued completion passed final validator: %v", err)
			}
			application := &Application{State: State.NewStore(input.State), GameData: appTestGameDataManagerFromCatalog(t, demolitionIntentCatalog)}
			if tc.free {
				err = application.guardBuildingFinishFree(context.Background(), args)
			} else {
				err = application.guardBuildingTimeSkip(context.Background(), args)
			}
			if !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatalf("final dispatch callback accepted completed demolition: %v", err)
			}

		})
	}
}
