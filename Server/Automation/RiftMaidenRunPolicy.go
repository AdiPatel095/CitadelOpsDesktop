package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const riftMaidenProbeUnitsPerAttack = 33

type RiftMaidenRunPolicy struct{}

func NewRiftMaidenRunPolicy() *RiftMaidenRunPolicy { return &RiftMaidenRunPolicy{} }

func (*RiftMaidenRunPolicy) ID() string          { return "riftMaidenRun" }
func (*RiftMaidenRunPolicy) ActorID() string     { return "riftMaiden" }
func (*RiftMaidenRunPolicy) EnabledKey() string  { return "rift_maiden_run" }
func (*RiftMaidenRunPolicy) ScheduleKey() string { return "riftMaiden" }

func (*RiftMaidenRunPolicy) Active(state State.GameState) bool {
	return state.Rift.MaidenRun != nil && state.Rift.MaidenRun.Status == "running"
}

func (*RiftMaidenRunPolicy) WakeDomains() []string {
	return []string{"rift", "commanders", "movements", "units", "equipment", "map-rift"}
}

func (*RiftMaidenRunPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	run := snapshot.State.Rift.MaidenRun
	if run == nil || run.Status != "running" {
		return Decision{Status: "idle", Detail: "No Rift Maiden probe run is active"}, nil
	}
	remaining := max(0, run.RequestedAttacks-run.AttacksLaunched)
	metrics := map[string]float64{
		"requestedAttacks": float64(run.RequestedAttacks),
		"attacksLaunched":  float64(run.AttacksLaunched),
		"remainingAttacks": float64(remaining),
	}
	if remaining == 0 {
		return Decision{Status: "complete", Detail: fmt.Sprintf("Completed all %d Rift Maiden probes", run.RequestedAttacks), Metrics: metrics}, nil
	}
	source, exists := snapshot.State.Castles[run.SourceCastleID]
	if !exists || source.X != run.SourceX || source.Y != run.SourceY || source.KingdomID != run.KingdomID {
		return riftMaidenRunWaiting(snapshot.Now, "The run's main-castle source is no longer authoritative", metrics), nil
	}
	target, exists := snapshot.State.LookupMapObservation(run.KingdomID, fmt.Sprintf("%d:%d", run.TargetX, run.TargetY))
	if !exists || target.TypeID != 43 {
		return riftMaidenRunWaiting(snapshot.Now, "The run's Rift target is not in the current map state", metrics), nil
	}
	available := 0
	for _, commanderID := range run.CommanderIDs {
		if commander, found := snapshot.State.Commanders[commanderID]; found && commander.Available {
			available++
		}
	}
	metrics["availableCommanders"] = float64(available)
	stockCapacity := int(source.Units.Stationed[run.UnitID] / riftMaidenProbeUnitsPerAttack)
	metrics["stockCapacity"] = float64(stockCapacity)
	batchSize := min(remaining, available, stockCapacity)
	if batchSize == 0 {
		detail := "Waiting for an assigned Maiden commander to return"
		if stockCapacity == 0 {
			detail = fmt.Sprintf("Waiting for at least %d probe units in the main castle", riftMaidenProbeUnitsPerAttack)
		}
		return riftMaidenRunWaiting(snapshot.Now, detail, metrics), nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"runId":              run.ID,
		"unitWodID":          run.UnitID,
		"horseTravelBoostId": run.HorseTravelBoostID,
		"commanderSelection": map[string]any{
			"candidates": run.CommanderIDs,
			"count":      batchSize,
			"strategy":   "first_available",
		},
	})
	return Decision{
		Status: "ready",
		Detail: fmt.Sprintf(
			"Launch the next %d Rift Maiden probe(s); %d of %d confirmed",
			batchSize, run.AttacksLaunched, run.RequestedAttacks,
		),
		Metrics:             metrics,
		Request:             &Intent.Request{Name: "rift.maiden_wave.launch", Arguments: arguments},
		ReevaluateOnSuccess: true,
		ReevaluateOnStale:   true,
	}, nil
}

func riftMaidenRunWaiting(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second), Metrics: metrics}
}
