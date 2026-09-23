package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"testing"
	"time"
)

func TestCompletedTrainingGroundWaitsInsteadOfSendingFCO(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"buildings":[{"wodID":339,"name":"FactionDeco","buildDuration":3000}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := data.BuildingCatalog()
	now := time.Date(2026, 9, 23, 2, 50, 18, 434847000, time.UTC)
	castle := State.CastleState{ID: 1, Layout: State.CastleLayout{ObservedAt: now, Objects: map[State.BuildingInstanceID]State.Building{}}, BuildingQueue: State.BuildingConstructionQueue{ObservedAt: now, Slots: []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: 110}}}}
	settings := defaultAutoStormSettings()
	for _, progress := range []int64{1800, 2400, 2990, 3000} {
		b := State.Building{InstanceID: 110, DefinitionID: 339, ConstructionState: State.BuildingStateBuildInProgress, ConstructionBoostPercent: 100, ProgressSec: progress}
		castle.Layout.Objects[110] = b
		remaining, known := autoStormBuildingRemaining(castle, b, catalog, now)
		if !known || remaining != 3000-progress {
			t.Fatalf("progress%d remaining%d known%t", progress, remaining, known)
		}
		decision, blocked := autoStormQueueDecision(Snapshot{Now: now}, settings, castle, catalog, map[string]float64{}, autoEventBuildProfile{})
		if progress == 2990 {
			if decision == nil || decision.Request.Name != "building.finish_free" {
				t.Fatalf("positive free window changed: %+v", decision)
			}
			continue
		}
		if decision != nil || !blocked {
			t.Fatalf("unexpected request at progress%d: %+v blocked%t", progress, decision, blocked)
		}
	}
	// Replay the exact live delayed state2/queued row after full progress.
	for _, elapsed := range []time.Duration{144 * time.Millisecond, time.Second, 5 * time.Second} {
		decision, blocked := autoStormQueueDecision(Snapshot{Now: now.Add(elapsed)}, settings, castle, catalog, map[string]float64{}, autoEventBuildProfile{})
		if !blocked {
			t.Fatal("completed but queued construction escaped wait")
		}
		if elapsed < 5*time.Second && decision != nil {
			t.Fatalf("completion generated request: %+v", decision)
		}
		if elapsed >= 5*time.Second && (decision == nil || decision.Request.Name != "building.refresh") {
			t.Fatalf("expected bounded refresh: %+v", decision)
		}
	}
}
