package State

import (
	"testing"
	"time"
)

func TestMarketBarrowLeaseUsesReturnLegAndCapsStaleAvailability(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	returnsAt := now.Add(10 * time.Minute)
	gameState := NewGameState()
	gameState.Session.ChangedAt = time.Time{}
	gameState.Player.ID = 1
	gameState.Castles[10] = CastleState{ID: 10}
	gameState.Movements[50] = MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: 20, TargetCastleID: 10,
		MarketBarrows: 75, ReturnsAt: &returnsAt,
	}
	market := MarketCastleState{CastleID: 10, TotalBarrows: 100, AvailableBarrows: 100}

	lease := MarketBarrowLeaseAt(&gameState, 10, now)
	if lease.Barrows != 75 || !lease.ReleasesAt.Equal(returnsAt) {
		t.Fatalf("market lease = %+v", lease)
	}
	if available := AvailableMarketBarrowsAt(&gameState, market, now); available != 25 {
		t.Fatalf("lease-adjusted available barrows = %d, want 25", available)
	}
	market.ObservedAt = returnsAt.Add(time.Nanosecond)
	gameState.Market.Castles[market.CastleID] = market
	if available := AvailableMarketBarrowsAt(&gameState, market, returnsAt.Add(time.Nanosecond)); available != 100 {
		t.Fatalf("returned barrows remained leased: %d", available)
	}
}

func TestMarketBarrowOutboundLeaseProjectsTheReturnTrip(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	arrivesAt := now.Add(5 * time.Minute)
	movement := MovementState{
		Direction: 0, MarketBarrows: 10, TravelSeconds: 300, ArrivesAt: &arrivesAt,
	}
	want := arrivesAt.Add(5 * time.Minute)
	releasesAt := MarketBarrowMovementReleaseAt(movement)
	if releasesAt == nil || !releasesAt.Equal(want) {
		t.Fatalf("outbound lease release = %v, want %s", releasesAt, want)
	}
	if !MarketBarrowMovementActiveAt(movement, want.Add(-time.Millisecond)) || MarketBarrowMovementActiveAt(movement, want) {
		t.Fatal("outbound market lease did not expire at its projected return")
	}
}

func TestMarketBarrowLeaseKeepsHomeFleetReservedAcrossReturnTransition(t *testing.T) {
	now := time.Date(2026, 9, 12, 17, 14, 46, 0, time.UTC)
	arrivesAt := now.Add(81 * time.Second)
	returnsAt := arrivesAt.Add(81 * time.Second)
	state := NewGameState()
	state.Session.ChangedAt = time.Time{}
	state.Player.ID = 1
	market := MarketCastleState{CastleID: 10, TotalBarrows: 125, AvailableBarrows: 125}
	for i, carts := range []int{90, 26, 7, 2} {
		id := MovementID(i + 1)
		state.Movements[id] = MovementState{
			ID: id, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 10, TargetCastleID: 20,
			MarketBarrows: carts, TravelSeconds: 81, ArrivesAt: &arrivesAt,
		}
	}
	check := func(at time.Time) {
		t.Helper()
		if lease := MarketBarrowLeaseAt(&state, 10, at); lease.Barrows != 125 || !lease.ReleasesAt.Equal(returnsAt) {
			t.Fatalf("home fleet reservation = %+v", lease)
		}
		if lease := MarketBarrowLeaseAt(&state, 20, at); lease.Barrows != 0 {
			t.Fatalf("recipient incorrectly owns returning carts: %+v", lease)
		}
		if available := AvailableMarketBarrowsAt(&state, market, at); available != 0 {
			t.Fatalf("home carts reused before returning: %d", available)
		}
	}
	check(now)
	for id, movement := range state.Movements {
		movement.Direction = 1
		movement.SourceCastleID, movement.TargetCastleID = 20, 10
		movement.ArrivesAt, movement.ReturnsAt = nil, &returnsAt
		state.Movements[id] = movement
		check(arrivesAt)
	}
	// Another player's returning carts must not reserve this account's fleet.
	state.Movements[99] = MovementState{ID: 99, Direction: 1, OwnerPlayerID: 2,
		SourceCastleID: 20, TargetCastleID: 10, MarketBarrows: 50, ReturnsAt: &returnsAt}
	check(returnsAt.Add(-time.Nanosecond))
	market.ObservedAt = returnsAt.Add(time.Nanosecond)
	state.Market.Castles[market.CastleID] = market
	if available := AvailableMarketBarrowsAt(&state, market, returnsAt.Add(time.Nanosecond)); available != 125 {
		t.Fatalf("completed return did not release fleet: %d", available)
	}
}

func TestMarketBarrowRecordRetentionAndConfirmation(t *testing.T) {
	if MarketBarrowLeaseRetention < MarketBarrowFreshness {
		t.Fatal("retention shorter than freshness")
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	r := now.Add(time.Minute)
	for _, tc := range []struct {
		name         string
		at, observed time.Time
		want         int
	}{
		{"at return", r, now, 100}, {"past return", r.Add(time.Second), now, 100},
		{"observation at return", r, r, 100}, {"confirmed", r.Add(time.Nanosecond), r.Add(time.Nanosecond), 0},
		{"last retained instant", r.Add(MarketBarrowLeaseRetention - time.Nanosecond), now, 100},
		{"retention boundary", r.Add(MarketBarrowLeaseRetention), now, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewGameState()
			gs.Session.ChangedAt = time.Time{}
			gs.Player.ID = 1
			gs.Castles[10] = CastleState{ID: 10}
			gs.Market.Castles[10] = MarketCastleState{CastleID: 10, TotalBarrows: 100, AvailableBarrows: 100, ObservedAt: now}
			gs.Movements[50] = MovementState{ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: 20, TargetCastleID: 10, MarketBarrows: 100, ReturnsAt: &r}
			if !RecordMarketBarrowLeases(&gs, now) {
				t.Fatal("not recorded")
			}
			if got := MarketBarrowLeaseAt(&gs, 10, now).Barrows; got != 100 {
				t.Fatalf("double count: %d", got)
			}
			delete(gs.Movements, 50)
			row := gs.Market.Castles[10]
			row.ObservedAt = tc.observed
			gs.Market.Castles[10] = row
			lease := MarketBarrowLeaseAt(&gs, 10, tc.at)
			if lease.Barrows != tc.want || lease.AwaitingConfirmation != tc.want {
				t.Fatalf("lease=%+v want=%d", lease, tc.want)
			}
			if tc.name == "retention boundary" && MarketBarrowSourceStatusAt(&gs, 10, tc.at).Ready {
				t.Fatal("expired projection authorized stale source")
			}
			RecordMarketBarrowLeases(&gs, tc.at)
			if len(gs.Market.BarrowLeases) == 0 && tc.want > 0 {
				t.Fatal("record pruned early")
			}
		})
	}
}

func TestMarketBarrowSourceFreshnessAndDeferral(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name              string
		observed, changed time.Time
		missing           bool
		wantReady         bool
		wantRefresh       time.Time
	}{
		{"fresh", now.Add(-MarketBarrowFreshness + time.Nanosecond), time.Time{}, false, true, time.Time{}},
		{"boundary", now.Add(-MarketBarrowFreshness), time.Time{}, false, false, now},
		{"future", now.Add(time.Nanosecond), time.Time{}, false, false, now},
		{"session changed", now.Add(-time.Second), now, false, false, now},
		{"unknown", time.Time{}, time.Time{}, false, false, now},
		{"omitted", now, time.Time{}, true, false, now.Add(MarketBarrowFreshness)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewGameState()
			gs.Session.ChangedAt = time.Time{}
			gs.Session.ChangedAt = tc.changed
			gs.Market.ObservedAt = tc.observed
			if !tc.missing {
				gs.Market.Castles[10] = MarketCastleState{CastleID: 10, ObservedAt: tc.observed}
			}
			got := MarketBarrowSourceStatusAt(&gs, 10, now)
			if got.Ready != tc.wantReady || !got.RefreshAt.Equal(tc.wantRefresh) {
				t.Fatalf("status=%+v", got)
			}
		})
	}
	gs := NewGameState()
	gs.Session.ChangedAt = time.Time{}
	gs.Player.ID = 1
	gs.Castles[10] = CastleState{ID: 10}
	r := now.Add(time.Minute)
	gs.Market.Castles[10] = MarketCastleState{CastleID: 10, TotalBarrows: 100, AvailableBarrows: 100, ObservedAt: now.Add(-3 * time.Minute)}
	gs.Market.BarrowLeases = map[MovementID]MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 100, ReleasesAt: r}}
	if got := MarketBarrowSourceStatusAt(&gs, 10, now); got.Ready || !got.RefreshAt.Equal(r) {
		t.Fatalf("full fleet: %+v", got)
	}
	gs.Market.Castles[20] = MarketCastleState{CastleID: 20, ObservedAt: now.Add(-3 * time.Minute)}
	if got := MarketBarrowSourceStatusAt(&gs, 20, now); !got.RefreshAt.Equal(now) {
		t.Fatalf("other fleet deferred source: %+v", got)
	}
}

func TestMarketBarrowRecordUpsertKeepsLatestReturn(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	r := now.Add(time.Minute)
	gs := NewGameState()
	gs.Session.ChangedAt = time.Time{}
	gs.Player.ID = 1
	gs.Castles[10] = CastleState{ID: 10}
	gs.Movements[50] = MovementState{ID: 50, OwnerPlayerID: 1, SourceCastleID: 10, TargetCastleID: 20, MarketBarrows: 75, ReturnsAt: &r}
	if !RecordMarketBarrowLeases(&gs, now) {
		t.Fatal("insert")
	}
	earlier := now.Add(30 * time.Second)
	m := gs.Movements[50]
	m.ReturnsAt = &earlier
	m.MarketBarrows = 80
	gs.Movements[50] = m
	if !RecordMarketBarrowLeases(&gs, now) {
		t.Fatal("update")
	}
	if got := gs.Market.BarrowLeases[50]; got.Barrows != 80 || !got.ReleasesAt.Equal(r) {
		t.Fatalf("record=%+v", got)
	}
	if RecordMarketBarrowLeases(&gs, now) {
		t.Fatal("unchanged upsert")
	}
}

func TestMarketBarrowRecordCloneIsolation(t *testing.T) {
	gs := NewGameState()
	r := time.Now().Add(time.Minute)
	gs.Market.BarrowLeases = map[MovementID]MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 125, ReleasesAt: r}}
	store := NewStore(&gs)
	snapshot := store.Snapshot()
	delete(snapshot.Market.BarrowLeases, 50)
	if len(store.ReadOnlyView().Market.BarrowLeases) != 1 {
		t.Fatal("snapshot aliases record map")
	}
	_, err := store.ApplyComponents(Components(ComponentMarket), func(state *GameState) ([]string, bool, error) {
		delete(state.Market.BarrowLeases, 50)
		return nil, false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.ReadOnlyView().Market.BarrowLeases) != 1 {
		t.Fatal("uncommitted selective mutation leaked")
	}
}
