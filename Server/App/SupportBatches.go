package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// The official support dialog has ten slots, one troop type per slot.
const supportUnitTypeLimit = Protocol.MaximumSupportUnitTypes

// Freeze the freshly resolved manifest once. Each disjoint batch becomes an
// ordinary acknowledged step, so a resume cannot repartition or replay troops.
func supportDispatchStep(name string, source State.CastleState, target State.AllianceHolding, wait int, amounts map[State.UnitID]int64, after Intent.Step) Intent.Step {
	ids := make([]int64, 0, len(amounts))
	for id, amount := range amounts {
		if amount > 0 {
			ids = append(ids, int64(id))
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	steps := []Intent.Step{}
	for start := 0; start < len(ids); start += supportUnitTypeLimit {
		end := min(start+supportUnitTypeLimit, len(ids))
		army := make([][2]int64, 0, end-start)
		for _, id := range ids[start:end] {
			army = append(army, [2]int64{id, amounts[State.UnitID(id)]})
		}
		payload, _ := json.Marshal(struct {
			SID State.CastleID `json:"SID"`
			TX  int            `json:"TX"`
			TY  int            `json:"TY"`
			LID int            `json:"LID"`
			WT  int            `json:"WT"`
			HBW int            `json:"HBW"`
			BPC int            `json:"BPC"`
			PTT int            `json:"PTT"`
			SD  int            `json:"SD"`
			A   [][2]int64     `json:"A"`
		}{source.ID, target.X, target.Y, stationLeaderID, wait, -1, 1, 1, 0, army})
		step := commandStep(fmt.Sprintf("%s (types %d–%d)", name, start+1, end), "cds", payload, "cds", Localization.New("server.app.p_types_p_p.39a761e5", "{p0} (types {p1}\u2013{p2})", Localization.Params{"p0": fmt.Sprintf("%s", name), "p1": start + 1, "p2": end}))
		step.ResponseBarrier = Intent.ResponseBarrierCommitted
		step.ResponseProjectionFailureIndeterminate = true
		step.CaptureResponse = true
		if len(ids) > supportUnitTypeLimit {
			step.PreDispatchAction = "support.batch.guard"
			step.PreDispatchArguments = payload
		}
		steps = append(steps, step)
		if after.Action != "" {
			steps = append(steps, after)
		}
	}
	// Preserve the single-command resolver contract when only one batch is needed.
	if len(ids) <= supportUnitTypeLimit && len(steps) > 0 {
		return steps[0]
	}
	return Intent.Step{Batch: steps}
}

// A paused operation keeps its frozen batches. Fail instead of sending a stale
// amount or sending from a different focus after resuming.
func (application *Application) guardSupportBatch(_ context.Context, arguments json.RawMessage) error {
	var payload struct {
		SID State.CastleID
		A   [][2]int64
	}
	if err := json.Unmarshal(arguments, &payload); err != nil {
		return err
	}
	if len(payload.A) == 0 || len(payload.A) > supportUnitTypeLimit {
		return Localization.WithError(fmt.Errorf("support requires 1 to %d troop types per command", supportUnitTypeLimit), Localization.New("server.app.support_requires_to_p.63ead59c", "support requires 1 to {p0} troop types per command", Localization.Params{"p0": supportUnitTypeLimit}))
	}
	state := application.State.Snapshot()
	source, ok := state.Castles[payload.SID]
	if !ok || !source.Focused {
		return Localization.WithError(fmt.Errorf("support source %d is no longer focused", payload.SID), Localization.New("server.app.support_source_p_is.b2153b5b", "support source {p0} is no longer focused", Localization.Params{"p0": fmt.Sprintf("%d", payload.SID)}))
	}
	if state.Player.ProtectionMode.PreparingOrActive(time.Now().UTC()) {
		return Localization.WithError(fmt.Errorf("Protection Mode became active before support batch"), Localization.New("server.app.protection_mode_became_active.734f9b3d", "Protection Mode became active before support batch", nil))
	}
	seen := map[int64]bool{}
	for _, unit := range payload.A {
		if unit[0] <= 0 || unit[1] <= 0 || seen[unit[0]] || source.Units.Stationed[State.UnitID(unit[0])] < unit[1] {
			return Localization.WithError(fmt.Errorf("support batch troop %d is invalid or no longer available", unit[0]), Localization.New("server.app.support_batch_troop_p.85621240", "support batch troop {p0} is invalid or no longer available", Localization.Params{"p0": unit[0]}))
		}
		seen[unit[0]] = true
	}
	return nil
}
