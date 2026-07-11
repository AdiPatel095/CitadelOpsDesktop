package GameParser

import (
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Scheduler"
)

const (
	gamSnapshotPageSize    = 25
	gamSnapshotQuietPeriod = 150 * time.Millisecond
	gamRequestTimeout      = 5 * time.Second
)

var gamSnapshotAssembly struct {
	sync.Mutex
	pending              []Models.GAMMovement
	pendingObservedUnix  int64
	assembling           bool
	generation           uint64
	timer                *time.Timer
	deferred             []Models.GAMMovement
	deferredObservedUnix int64
	deferredValid        bool
}

var gamRequestState struct {
	sync.Mutex
	inFlightUntil time.Time
}

// RequestGAMSnapshot queues at most one GAM request at a time. A timed-out request may be retried.
func RequestGAMSnapshot() bool {
	if !ResponseRegistry.IsGameWebSocketReady() {
		return false
	}
	now := time.Now()
	gamRequestState.Lock()
	if now.Before(gamRequestState.inFlightUntil) {
		gamRequestState.Unlock()
		return false
	}
	gamRequestState.inFlightUntil = now.Add(gamRequestTimeout)
	gamRequestState.Unlock()
	GameCommands.QueueBackgroundRefresh(GameCommands.GAMPayload())
	return true
}

func completeGAMRequest() {
	gamRequestState.Lock()
	gamRequestState.inFlightUntil = time.Time{}
	gamRequestState.Unlock()
}

func queueGAMSnapshotPage(page []Models.GAMMovement, pageSize int, observedUnix int64) {
	gamSnapshotAssembly.Lock()
	if gamSnapshotAssembly.timer != nil {
		gamSnapshotAssembly.timer.Stop()
		gamSnapshotAssembly.timer = nil
	}
	gamSnapshotAssembly.pending = append(gamSnapshotAssembly.pending, page...)
	gamSnapshotAssembly.pendingObservedUnix = observedUnix
	gamSnapshotAssembly.assembling = true
	gamSnapshotAssembly.generation++
	generation := gamSnapshotAssembly.generation
	gamSnapshotAssembly.timer = time.AfterFunc(gamSnapshotQuietPeriod, func() {
		commitQueuedGAMSnapshot(generation)
	})
	gamSnapshotAssembly.Unlock()
}

func commitQueuedGAMSnapshot(generation uint64) {
	gamSnapshotAssembly.Lock()
	if !gamSnapshotAssembly.assembling || generation != gamSnapshotAssembly.generation {
		gamSnapshotAssembly.Unlock()
		return
	}
	movements, snapshotUnix := takePendingGAMSnapshotLocked()
	gamSnapshotAssembly.Unlock()
	applyAuthoritativeGAMSnapshot(movements, snapshotUnix)
}

func takePendingGAMSnapshotLocked() ([]Models.GAMMovement, int64) {
	movements := append([]Models.GAMMovement(nil), gamSnapshotAssembly.pending...)
	snapshotUnix := gamSnapshotAssembly.pendingObservedUnix
	gamSnapshotAssembly.pending = nil
	gamSnapshotAssembly.pendingObservedUnix = 0
	gamSnapshotAssembly.assembling = false
	gamSnapshotAssembly.timer = nil
	return movements, snapshotUnix
}

func recordDeltaInPendingGAMSnapshot(movement Models.GAMMovement) {
	gamSnapshotAssembly.Lock()
	if gamSnapshotAssembly.assembling {
		kept := make([]Models.GAMMovement, 0, len(gamSnapshotAssembly.pending)+1)
		for _, pending := range gamSnapshotAssembly.pending {
			if movement.MID != 0 && pending.MID == movement.MID {
				continue
			}
			if movement.CommanderID >= 0 && pending.CommanderID == movement.CommanderID {
				continue
			}
			kept = append(kept, pending)
		}
		gamSnapshotAssembly.pending = append(kept, movement)
	}
	gamSnapshotAssembly.Unlock()
}

func removePendingGAMMovement(mid int) {
	gamSnapshotAssembly.Lock()
	if gamSnapshotAssembly.assembling {
		kept := make([]Models.GAMMovement, 0, len(gamSnapshotAssembly.pending))
		for _, movement := range gamSnapshotAssembly.pending {
			if movement.MID != mid {
				kept = append(kept, movement)
			}
		}
		gamSnapshotAssembly.pending = kept
	}
	gamSnapshotAssembly.Unlock()
}

func deferGAMSnapshotUntilPlayerKnown(movements []Models.GAMMovement, observedUnix int64) {
	gamSnapshotAssembly.Lock()
	gamSnapshotAssembly.deferred = append([]Models.GAMMovement(nil), movements...)
	gamSnapshotAssembly.deferredObservedUnix = observedUnix
	gamSnapshotAssembly.deferredValid = true
	gamSnapshotAssembly.Unlock()
}

func applyDeferredGAMSnapshot() {
	if Models.GetGameState().PlayerID <= 0 {
		return
	}
	gamSnapshotAssembly.Lock()
	if !gamSnapshotAssembly.deferredValid {
		gamSnapshotAssembly.Unlock()
		return
	}
	movements := append([]Models.GAMMovement(nil), gamSnapshotAssembly.deferred...)
	observedUnix := gamSnapshotAssembly.deferredObservedUnix
	gamSnapshotAssembly.deferred = nil
	gamSnapshotAssembly.deferredObservedUnix = 0
	gamSnapshotAssembly.deferredValid = false
	gamSnapshotAssembly.Unlock()
	applyAuthoritativeGAMSnapshot(movements, observedUnix)
}

func clearDeferredGAMSnapshot() {
	gamSnapshotAssembly.Lock()
	gamSnapshotAssembly.deferred = nil
	gamSnapshotAssembly.deferredObservedUnix = 0
	gamSnapshotAssembly.deferredValid = false
	gamSnapshotAssembly.Unlock()
}

func applyAuthoritativeGAMSnapshot(allMovements []Models.GAMMovement, observedUnix int64) {
	completeGAMRequest()
	gs := Models.GetGameState()
	if gs.PlayerID <= 0 {
		deferGAMSnapshotUntilPlayerKnown(allMovements, observedUnix)
		if gs.PlayerID > 0 {
			applyDeferredGAMSnapshot()
		}
		return
	}
	clearDeferredGAMSnapshot()
	owned := make([]Models.GAMMovement, 0, len(allMovements))
	incoming := make([]Models.GAMMovement, 0)
	for _, movement := range allMovements {
		if movement.OID != gs.PlayerID {
			if movement.D == 0 && movement.MovementType == 0 &&
				movement.TargetPlayerID == gs.PlayerID && movement.TargetCastleID > 0 {
				incoming = append(incoming, movement)
			}
			continue
		}
		movement = resolveMovementCommander(gs, movement)
		owned = append(owned, movement)
		setMovementTargetCooldown(movement)
	}
	gs.Movement.ReplaceSnapshotWithIncoming(owned, incoming, observedUnix)
	publishMovementState(true)
}

func applyGAMMovementDeltas(movements []Models.GAMMovement) {
	gs := Models.GetGameState()
	if gs.PlayerID <= 0 {
		return
	}
	changed := false
	for _, movement := range movements {
		if movement.OID != gs.PlayerID {
			continue
		}
		movement = resolveMovementCommander(gs, movement)
		gs.Movement.ApplyDelta(movement)
		recordDeltaInPendingGAMSnapshot(movement)
		setMovementTargetCooldown(movement)
		changed = true
	}
	if changed {
		publishMovementState(false)
	}
}

func resolveMovementCommander(gs *Models.GameState, movement Models.GAMMovement) Models.GAMMovement {
	if movement.CommanderID >= 0 {
		return movement
	}
	if movement.MID != 0 {
		if commanderID, ok := gs.Movement.CommanderForMID(movement.MID); ok {
			movement.CommanderID = commanderID
			return movement
		}
	}
	if sourceIsRift(movement) {
		if commanderID, ok := savedLaunchCommanderForRiftReturn(movement); ok {
			movement.CommanderID = commanderID
		}
	}
	return movement
}

func setMovementTargetCooldown(movement Models.GAMMovement) {
	if movement.TargetX == 0 || movement.TargetY == 0 || movement.TT <= 0 {
		return
	}
	targetID := fmt.Sprintf("%d,%d", movement.TargetX, movement.TargetY)
	totalCooldown := time.Duration(movement.TT*2+10) * time.Second
	Scheduler.GetScheduler().CooldownTracker.SetCooldown(targetID, totalCooldown)
}

func publishMovementState(authoritativeSnapshot bool) {
	MaybeNotifyRiftCRALaunchBusyChanged()
	if NotifyMovementChanged != nil {
		NotifyMovementChanged()
	}
	if authoritativeSnapshot {
		markGAMParsed()
	}
	if OnGAMParsed != nil {
		go OnGAMParsed()
	}
}
