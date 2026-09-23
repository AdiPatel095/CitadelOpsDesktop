package App

import (
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConstructionCompletionRecheckedBeforeFCOAndMSB(t *testing.T) {
	const catalog = `{"versionInfo":[],"units":[],"buildings":[{"wodID":339,"name":"FactionDeco","group":"Building","width":4,"height":5,"buildDuration":3000}],"currencies":[{"currencyID":1003,"Name":"10MinSkip","JSONKey":"MS3"}],"currencyMinutesSkipValues":[{"currencyID":1003,"MinutesSkipValue":"10","MinuteSkipIndex":"2"}]}`
	manager := appTestGameDataManagerFromCatalog(t, catalog)
	data, _ := manager.Current()
	for _, free := range []bool{true, false} {
		state := buildingIntentState()
		castle := state.Castles[10]
		castle.Layout.ObservedAt = time.Now()
		castle.BuildingQueue.ObservedAt = castle.Layout.ObservedAt
		b := castle.Layout.Objects[42]
		b.DefinitionID = 339
		b.ConstructionState = State.BuildingStateBuildInProgress
		b.ConstructionBoostPercent = 100
		b.ProgressSec = 2990
		castle.Layout.Objects[42] = b
		castle.Buildings[42] = b
		castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{Status: State.BuildingQueueSlotOccupied, BuildingID: 42}
		state.Castles[10] = castle
		input := Intent.PlanningContext{State: state, GameData: data}
		args := json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`)
		if !free {
			args = json.RawMessage(`{"castleId":10,"buildingInstanceId":42,"minutes":10}`)
		}
		var step Intent.Step
		var err error
		if free {
			step, err = resolveBuildingFinishFreeStep(context.Background(), input, args)
		} else {
			step, err = resolveBuildingTimeSkipStep(context.Background(), input, args)
		}
		if err != nil {
			t.Fatal(err)
		}
		if step.FinalDispatchAction == "" {
			t.Fatal("normal construction lacks final dispatch guard")
		}
		// Progress reaches the complete duration before state2/queue are updated.
		b.ProgressSec = 3000
		castle.Layout.Objects[42] = b
		castle.Buildings[42] = b
		state.Castles[10] = castle
		app := &Application{State: State.NewStore(state), GameData: manager}
		if free {
			err = app.guardBuildingFinishFree(context.Background(), step.FinalDispatchArguments)
		} else {
			err = app.guardBuildingTimeSkip(context.Background(), step.FinalDispatchArguments)
		}
		if !errors.Is(err, Intent.ErrPlanStale) {
			t.Fatalf("free%t completed queued construction accepted: %v", free, err)
		}
		input.State = state
		if free {
			_, err = resolveBuildingFinishFreeStep(context.Background(), input, args)
		} else {
			_, err = resolveBuildingTimeSkipStep(context.Background(), input, args)
		}
		if !errors.Is(err, Intent.ErrPlanStale) {
			t.Fatalf("free%t resolver accepted full progress: %v", free, err)
		}
		b.ConstructionState = State.BuildingStateBuildCompleted
		castle.Layout.Objects[42], castle.Buildings[42] = b, b
		castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{Status: State.BuildingQueueSlotAvailable}
		state.Castles[10] = castle
		app.State = State.NewStore(state)
		if free {
			err = app.guardBuildingFinishFree(context.Background(), args)
		} else {
			err = app.guardBuildingTimeSkip(context.Background(), args)
		}
		if !errors.Is(err, Intent.ErrPlanStale) {
			t.Fatalf("free%t authoritative completed queue is not stale: %v", free, err)
		}

	}
}
