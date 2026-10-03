package State

import "time"

const MarketBarrowFreshness = 2 * time.Minute
const MarketBarrowLeaseRetention = 10 * time.Minute

type MarketBarrowLeaseRecord struct {
	HomeCastleID CastleID  `json:"homeCastleId"`
	Barrows      int       `json:"barrows"`
	ReleasesAt   time.Time `json:"releasesAt"`
}

type MarketBarrowLease struct {
	AwaitingConfirmation int
	Barrows              int
	ReleasesAt           time.Time
}

func MarketBarrowMovementReleaseAt(movement MovementState) *time.Time {
	if movement.MarketBarrows <= 0 {
		return nil
	}
	if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() {
		releasesAt := movement.ReturnsAt.UTC()
		return &releasesAt
	}
	if movement.Direction == 0 && movement.ArrivesAt != nil && !movement.ArrivesAt.IsZero() {
		releasesAt := movement.ArrivesAt.UTC()
		if movement.TravelSeconds > 0 {
			releasesAt = releasesAt.Add(time.Duration(movement.TravelSeconds) * time.Second)
		}
		return &releasesAt
	}
	return movement.ProjectedCompletionAt()
}

func MarketBarrowMovementActiveAt(movement MovementState, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	releasesAt := MarketBarrowMovementReleaseAt(movement)
	return releasesAt != nil && !releasesAt.IsZero() && releasesAt.After(now)
}

func marketBarrowHome(movement MovementState) CastleID {
	if movement.Direction == 1 {
		return movement.TargetCastleID
	}
	return movement.SourceCastleID
}

func marketBarrowLeaseHeld(gameState *GameState, record MarketBarrowLeaseRecord, now time.Time) bool {
	if record.Barrows <= 0 || record.ReleasesAt.IsZero() {
		return false
	}
	return record.ReleasesAt.After(now) || (now.Before(record.ReleasesAt.Add(MarketBarrowLeaseRetention)) &&
		!gameState.Market.Castles[record.HomeCastleID].ObservedAt.After(record.ReleasesAt))
}

// Records outlive movement cleanup, without changing movement clocks or GAM rules.
func RecordMarketBarrowLeases(gameState *GameState, now time.Time) bool {
	changed := false
	gameState.RangeMovements(func(id MovementID, movement MovementState) bool {
		if movement.MarketBarrows <= 0 || !MovementOwnedByCurrentPlayer(gameState, movement) {
			return true
		}
		r := MarketBarrowMovementReleaseAt(movement)
		if r == nil {
			return true
		}
		record := MarketBarrowLeaseRecord{HomeCastleID: marketBarrowHome(movement), Barrows: movement.MarketBarrows, ReleasesAt: *r}
		old, exists := gameState.Market.BarrowLeases[id]
		if old.ReleasesAt.After(record.ReleasesAt) {
			record.ReleasesAt = old.ReleasesAt
		}
		if marketBarrowLeaseHeld(gameState, record, now) && (!exists || old != record) {
			if gameState.Market.BarrowLeases == nil {
				gameState.Market.BarrowLeases = map[MovementID]MarketBarrowLeaseRecord{}
			}
			gameState.Market.BarrowLeases[id] = record
			changed = true
		}
		return true
	})
	for id, record := range gameState.Market.BarrowLeases {
		if !marketBarrowLeaseHeld(gameState, record, now) {
			delete(gameState.Market.BarrowLeases, id)
			changed = true
		}
	}
	return changed
}

func MarketBarrowLeaseAt(gameState *GameState, castleID CastleID, now time.Time) MarketBarrowLease {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lease := MarketBarrowLease{}
	add := func(record MarketBarrowLeaseRecord) {
		if record.HomeCastleID != castleID || !marketBarrowLeaseHeld(gameState, record, now) {
			return
		}
		lease.Barrows += record.Barrows
		if !record.ReleasesAt.After(now) {
			lease.AwaitingConfirmation += record.Barrows
		} else if lease.ReleasesAt.IsZero() || record.ReleasesAt.Before(lease.ReleasesAt) {
			lease.ReleasesAt = record.ReleasesAt
		}
	}
	gameState.RangeMovements(func(id MovementID, movement MovementState) bool {
		if movement.MarketBarrows <= 0 || !MovementOwnedByCurrentPlayer(gameState, movement) {
			return true
		}
		r := MarketBarrowMovementReleaseAt(movement)
		if r != nil {
			record := MarketBarrowLeaseRecord{HomeCastleID: marketBarrowHome(movement), Barrows: movement.MarketBarrows, ReleasesAt: *r}
			if old := gameState.Market.BarrowLeases[id]; old.ReleasesAt.After(record.ReleasesAt) {
				record.ReleasesAt = old.ReleasesAt
			}
			add(record)
		}
		return true
	})
	for id, record := range gameState.Market.BarrowLeases {
		if _, exists := gameState.LookupMovement(id); !exists {
			add(record)
		}
	}
	return lease
}

type MarketBarrowSourceStatus struct {
	Ready     bool
	RefreshAt time.Time
}

func marketObservationCurrent(observedAt, now, sessionChangedAt time.Time) bool {
	return !observedAt.IsZero() && !observedAt.After(now) &&
		(sessionChangedAt.IsZero() || !observedAt.Before(sessionChangedAt))
}

func MarketBarrowSourceStatusAt(gameState *GameState, castle CastleID, now time.Time) MarketBarrowSourceStatus {
	row, exists := gameState.Market.Castles[castle]
	current := marketObservationCurrent(row.ObservedAt, now, gameState.Session.ChangedAt)
	if exists && current && now.Sub(row.ObservedAt) < MarketBarrowFreshness {
		return MarketBarrowSourceStatus{Ready: true}
	}
	status := MarketBarrowSourceStatus{RefreshAt: now}
	if !exists && marketObservationCurrent(gameState.Market.ObservedAt, now, gameState.Session.ChangedAt) && now.Sub(gameState.Market.ObservedAt) < MarketBarrowFreshness {
		status.RefreshAt = gameState.Market.ObservedAt.Add(MarketBarrowFreshness)
	} else if exists && current {
		lease := MarketBarrowLeaseAt(gameState, castle, now)
		if lease.AwaitingConfirmation == 0 && lease.Barrows >= row.TotalBarrows && row.TotalBarrows > 0 {
			status.RefreshAt = lease.ReleasesAt
		}
	}
	return status
}

func AvailableMarketBarrowsAt(gameState *GameState, market MarketCastleState, now time.Time) int {
	available := max(0, market.AvailableBarrows)
	lease := MarketBarrowLeaseAt(gameState, market.CastleID, now)
	if market.TotalBarrows > 0 {
		available = min(available, max(0, market.TotalBarrows-lease.Barrows))
	} else if lease.Barrows > 0 {
		available = 0
	}
	return available
}
