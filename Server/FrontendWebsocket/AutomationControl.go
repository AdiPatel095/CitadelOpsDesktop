package FrontendWebsocket

import (
	"context"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/ResponseRegistry"
)

func beginManualWork(reason string, claims []Automation.Claim, maxHold time.Duration) (*Automation.Lease, func(), bool) {
	if maxHold <= 0 {
		maxHold = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxHold)
	lease, ok := Automation.Acquire(ctx, Automation.Request{
		Owner:        Automation.OwnerManual,
		Priority:     Automation.PriorityManual,
		Reason:       reason,
		Claims:       claims,
		MaxHold:      maxHold,
		PreemptLower: true,
	})
	if !ok {
		cancel()
		return nil, func() {}, false
	}
	finish := func() {
		Automation.WaitForWork(ctx, lease.WorkID())
		lease.Release()
		cancel()
	}
	return lease, finish, true
}

func refreshAutoSceatResCatalogForView() {
	if !ResponseRegistry.IsGameWebSocketReady() {
		sendAutoSceatResCatalog()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = Automation.RunWork(ctx, Automation.WorkItem{
		DedupeKey: "view:autoSceatRes:refresh",
		Request: Automation.Request{
			Owner:        Automation.OwnerManual,
			Priority:     Automation.PriorityManual,
			Reason:       "refresh Auto Sceat Resources view",
			Claims:       []Automation.Claim{Automation.ExclusiveClaim(Automation.ClaimCrafting), Automation.ExclusiveClaim(Automation.ClaimTransport)},
			MaxHold:      10 * time.Second,
			PreemptLower: true,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			refreshes := []struct {
				opcode  string
				payload string
				version uint64
			}{
				{opcode: "crin", payload: GameCommands.CRINPayload()},
				{opcode: "kpi", payload: GameCommands.KPIPayload()},
				{opcode: "boi", payload: GameCommands.BOIPayload()},
				{opcode: "cmi", payload: GameCommands.CMIPayload()},
			}
			for i := range refreshes {
				refreshes[i].version = Automation.StateSnapshot(Automation.StateOpcode(refreshes[i].opcode)).Version
				if !GameCommands.QueueFeatureRefresh(Automation.OwnerManual, refreshes[i].payload, lease) {
					return Automation.ErrWorkCancelled
				}
			}
			for _, refresh := range refreshes {
				if _, ok := Automation.AwaitStateAfter(workCtx, Automation.StateOpcode(refresh.opcode), refresh.version); !ok {
					return Automation.ErrWorkCancelled
				}
			}
			return nil
		},
	})
	sendAutoSceatResCatalog()
}

func refreshStateForManualWork(lease *Automation.Lease, opcode, payload string, timeout time.Duration) bool {
	if lease == nil || !lease.Active() {
		return false
	}
	stateKey := Automation.StateOpcode(opcode)
	previous := Automation.StateSnapshot(stateKey).Version
	if !GameCommands.QueueFeatureRefresh(Automation.OwnerManual, payload, lease) {
		return false
	}
	ctx, cancel := context.WithTimeout(lease.Context(), timeout)
	defer cancel()
	_, ok := Automation.AwaitStateAfter(ctx, stateKey, previous)
	return ok
}

func refreshEquipmentListForView() bool {
	lease, finish, ok := beginManualWork("refresh equipment view", []Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimEquipment),
	}, 15*time.Second)
	if !ok {
		return false
	}
	defer finish()
	return refreshStateForManualWork(lease, "gli", GameCommands.GLIPayload(), 10*time.Second)
}

func refreshUpgradeMenuForView() bool {
	lease, finish, ok := beginManualWork("refresh equipment upgrade view", []Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimEquipment),
	}, 15*time.Second)
	if !ok {
		return false
	}
	defer finish()
	refreshes := []struct {
		opcode  string
		payload string
		version uint64
	}{
		{opcode: "ggm", payload: GameCommands.GGMPayload()},
		{opcode: "gei", payload: GameCommands.GEIPayload()},
		{opcode: "gli", payload: GameCommands.GLIPayload()},
	}
	if !GameCommands.QueueFeatureRefresh(Automation.OwnerManual, GameCommands.GNRPayload(), lease) {
		return false
	}
	for i := range refreshes {
		refreshes[i].version = Automation.StateSnapshot(Automation.StateOpcode(refreshes[i].opcode)).Version
		if !GameCommands.QueueFeatureRefresh(Automation.OwnerManual, refreshes[i].payload, lease) {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(lease.Context(), 10*time.Second)
	defer cancel()
	for _, refresh := range refreshes {
		if _, ok := Automation.AwaitStateAfter(ctx, Automation.StateOpcode(refresh.opcode), refresh.version); !ok {
			return false
		}
	}
	return true
}
