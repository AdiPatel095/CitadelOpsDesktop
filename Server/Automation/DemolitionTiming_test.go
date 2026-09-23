package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"testing"
	"time"
)

func TestDemolitionQueueRefreshIsBoundedAndNeverSkipsCompletedWork(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"buildings":[{"wodID":249,"name":"FactionMarket","buildDuration":9000}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := data.BuildingCatalog()
	now := time.Now()
	boost := 100.0
	castle := State.CastleState{ID: 1, Layout: State.CastleLayout{Objects: map[State.BuildingInstanceID]State.Building{15: {InstanceID: 15, DefinitionID: 249, ConstructionState: State.BuildingStateDisassembleInProgress, ConstructionBoostPercent: boost, ProgressSec: 4500}}, ObservedAt: now}, BuildingQueue: State.BuildingConstructionQueue{ObservedAt: now, Slots: []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: 15}}}}
	settings := defaultAutoStormSettings()
	settings.Build.AllowTimeSkips = true
	for _, known := range []bool{true, false} {
		b := castle.Layout.Objects[15]
		if !known {
			b.ConstructionBoostPercent = 0
		}
		castle.Layout.Objects[15] = b
		for _, elapsed := range []time.Duration{0, time.Second, 4999 * time.Millisecond, 5 * time.Second} {
			decision, blocked := autoStormQueueDecision(Snapshot{Now: now.Add(elapsed), GameData: data}, settings, castle, catalog, map[string]float64{}, autoEventBuildProfile{FeatureLabel: "Berimond"})
			if !blocked {
				t.Fatal("completed/unknown demolition queue was treated as available")
			}
			if elapsed < 5*time.Second && decision != nil {
				t.Fatalf("hot-loop mutation/refresh at %s: %+v", elapsed, decision)
			}
			if elapsed >= 5*time.Second && (decision == nil || decision.Request == nil || decision.Request.Name != "building.refresh") {
				t.Fatalf("want bounded read-only refresh: %+v", decision)
			}
		}
	}
	// A new authoritative snapshot restarts the refresh interval.
	castle.Layout.ObservedAt = now.Add(5 * time.Second)
	castle.BuildingQueue.ObservedAt = castle.Layout.ObservedAt
	decision, _ := autoStormQueueDecision(Snapshot{Now: now.Add(5 * time.Second)}, settings, castle, catalog, map[string]float64{}, autoEventBuildProfile{})
	if decision != nil {
		t.Fatal("successful refresh immediately triggered another request")
	}
	b := castle.Layout.Objects[15]
	b.ConstructionBoostPercent = boost
	b.ProgressSec = 4490
	castle.Layout.Objects[15] = b
	decision, _ = autoStormQueueDecision(Snapshot{Now: now.Add(5 * time.Second)}, settings, castle, catalog, map[string]float64{}, autoEventBuildProfile{})
	if decision == nil || decision.Request.Name != "building.finish_free" {
		t.Fatalf("positive free window not preserved: %+v", decision)
	}
}
