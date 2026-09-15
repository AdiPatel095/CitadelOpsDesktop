package GameData

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestSelectAutoBuyerFeastSourceUsesMostStoredPositiveNetCastle(t *testing.T) {
	store := autoBuyerFeastSourceTestStore(t)
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Castles[30] = autoBuyerFeastSourceCastle(30, 0, 1, 150000, 200, 300, now) // negative net
	gameState.Castles[20] = autoBuyerFeastSourceCastle(20, 2, 12, 90000, 200, 10, now)
	gameState.Castles[10] = autoBuyerFeastSourceCastle(10, 0, 1, 90000, 100, 10, now)
	gameState.Castles[40] = autoBuyerFeastSourceCastle(40, 0, 1, 80000, 0, 0, now) // zero net

	selected, status, err := store.SelectAutoBuyerFeastSource(gameState, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Castle.ID != 10 || selected.Castle.KingdomID != 0 || selected.FoodAmount != 90000 ||
		selected.NetFoodPerHour != 90 || status.EligibleCount != 2 {
		t.Fatalf("automatic feast source = %+v, status = %+v", selected, status)
	}
}

func TestSelectAutoBuyerFeastSourceWaitsForEveryUsableCastleAuthority(t *testing.T) {
	store := autoBuyerFeastSourceTestStore(t)
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Castles[10] = autoBuyerFeastSourceCastle(10, 0, 1, 90000, 100, 10, now)
	stale := autoBuyerFeastSourceCastle(20, 1, 12, 200000, 100, 10, now)
	stale.FoodEconomyObservedAt = now.Add(-time.Hour)
	gameState.Castles[20] = stale

	selected, status, err := store.SelectAutoBuyerFeastSource(gameState, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Castle.ID != 0 || len(status.StaleCandidateIDs) != 1 || status.StaleCandidateIDs[0] != 20 {
		t.Fatalf("stale automatic source result = %+v, status = %+v", selected, status)
	}
}

func autoBuyerFeastSourceTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},{"resourceID":13,"JSONKey":"BEEF"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func autoBuyerFeastSourceCastle(
	id State.CastleID,
	kingdom State.KingdomID,
	slotType int,
	foodAmount, production, consumption float64,
	observedAt time.Time,
) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, SlotType: slotType,
		ContextSnapshotObservedAt: observedAt,
		FoodBalanceObservedAt:     observedAt,
		FoodEconomyObservedAt:     observedAt,
		Resources: map[State.ResourceID]State.ResourceBalance{
			5: {Amount: foodAmount, ProductionPerHour: &production, ConsumptionPerHour: &consumption},
		},
	}
}
