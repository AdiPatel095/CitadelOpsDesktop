package State

import (
	"reflect"
	"testing"
	"time"
)

// The stable bridge intentionally retains the beta data model without
// registering beta automation policies or spend intents.
func TestStableBridgePreservesBetaStateThroughFullRewrite(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	initial := NewGameState()
	initial.Player.ID = 99
	initial.AttackAnalytics.RecentTowerAdvisorTimeSkips = []TowerAdvisorTimeSkipUsage{{MovementID: 51, TimeSkips: 20, UsedAt: now}}
	initial.EventScores.Inventory = EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]EventAvailability{},
		GlobalEffectsObservedAt: now, GlobalEffects: map[int64]GlobalEffectAvailability{7: {GlobalEffectID: 7, Strength: 60, EndsAt: now.Add(time.Hour)}},
		GlobalEffectBoosterOffers:    map[int64]GlobalEffectBoosterOffer{7: {GlobalEffectID: 7, RubyCost: 300, BonusValue: 10}},
		GlobalEffectBoostsObservedAt: now, GlobalEffectBoosts: map[int64]GlobalEffectBoostState{7: {GlobalEffectID: 7, Boosted: true, OccurrenceEndsAt: now.Add(time.Hour), ObservedAt: now}},
	}
	initial.Market.FeastPurchasePending = true
	initial.Market.FeastPurchaseInactiveGeneration = 4
	initial.Market.FeastPurchaseInactiveObservedAt = now
	initial.Market.FeastPurchaseInactiveResponseToken = "inert-fixture"
	initial.Stationing["pending"] = StationingOperation{ID: "pending", PresetID: "retained-preset", Units: map[UnitID]int64{216: 200}}
	initial.Map[0] = map[string]MapObservation{"101:102": {KingdomID: 0, TypeID: MapTypeKingdomFortress, X: 101, Y: 102, Level: 80, FortressDefeaterPlayerID: 99, ObservedAt: now}}
	initial.Movements[51] = MovementState{ID: 51, AdvisorType: 1, AdvisorAttackNumber: 2, AdvisorAttackCount: 3, AdvisorLaunchState: 4}
	directory := t.TempDir()
	if err := SaveSnapshot(directory, initial); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 3; cycle++ {
		loaded, err := LoadSnapshot(directory)
		if err != nil {
			t.Fatal(err)
		}
		store := NewStore(loaded)
		event, err := store.Apply(func(state *GameState) ([]string, bool, error) {
			state.Player.Level++
			return []string{"player"}, true, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveComponentSnapshot(directory, event, AllComponents); err != nil {
			t.Fatal(err)
		}
		after, err := LoadSnapshot(directory)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(initial.AttackAnalytics.RecentTowerAdvisorTimeSkips, after.AttackAnalytics.RecentTowerAdvisorTimeSkips) {
			t.Fatalf("cycle %d lost advisor usage: %+v", cycle, after.AttackAnalytics)
		}
		if !reflect.DeepEqual(initial.EventScores.Inventory, after.EventScores.Inventory) {
			t.Fatalf("cycle %d lost booster inventory: %+v", cycle, after.EventScores.Inventory)
		}
		if !after.Market.FeastPurchasePending || after.Market.FeastPurchaseInactiveGeneration != 4 || !after.Market.FeastPurchaseInactiveObservedAt.Equal(now) || after.Market.FeastPurchaseInactiveResponseToken != "inert-fixture" {
			t.Fatalf("cycle %d lost feast uncertainty", cycle)
		}
		if !reflect.DeepEqual(initial.Stationing, after.Stationing) {
			t.Fatalf("cycle %d lost stationing: %+v", cycle, after.Stationing)
		}
		movement, ok := after.LookupMovement(51)
		if !ok || movement.AdvisorType != 1 || movement.AdvisorAttackNumber != 2 || movement.AdvisorAttackCount != 3 || movement.AdvisorLaunchState != 4 {
			t.Fatal("advisor movement data lost")
		}
		observation, ok := after.LookupMapObservation(0, "101:102")
		if !ok || observation.FortressDefeaterPlayerID != 99 {
			t.Fatal("fortress history lost")
		}
	}
}
