package Automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriGallantryBoosterGateCoversEveryFeatureLane(t *testing.T) {
	now := time.Date(2026, 8, 4, 20, 30, 0, 0, time.UTC)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"requireActiveGallantryBooster":true,"build":{"enabled":true}}`),
	}}
	type lane struct {
		name        string
		wakeDomains []string
		evaluate    func(context.Context, Snapshot) (Decision, error)
	}
	lanes := []lane{
		{name: "transfers", wakeDomains: NewBeriPolicy().WakeDomains(), evaluate: NewBeriPolicy().Evaluate},
		{name: "attacks", wakeDomains: NewBeriAttackPolicy().WakeDomains(), evaluate: NewBeriAttackPolicy().Evaluate},
		{name: "tools", wakeDomains: NewBeriToolPolicy().WakeDomains(), evaluate: NewBeriToolPolicy().Evaluate},
		{name: "builder", wakeDomains: NewBeriBuildPolicy().WakeDomains(), evaluate: NewBeriBuildPolicy().Evaluate},
	}

	for _, test := range lanes {
		t.Run(test.name, func(t *testing.T) {
			if !containsString(test.wakeDomains, "boosters") {
				t.Fatalf("wake domains do not include boosters: %v", test.wakeDomains)
			}
			gameState := State.NewGameState()
			gameState.Market.BoostersObservedAt = now
			snapshot := Snapshot{State: gameState, Configuration: configuration, Now: now}

			decision, err := test.evaluate(t.Context(), snapshot)
			if err != nil || decision.Request != nil || decision.Status != "gated" ||
				!strings.Contains(decision.Detail, "boi ID 24") {
				t.Fatalf("inactive booster decision = %#v err=%v", decision, err)
			}

			gameState.Market.Boosters[GameData.GallantryPointsBoosterID] = State.MarketBoosterState{
				ID: GameData.GallantryPointsBoosterID, BonusPercent: 400, ExpiresAt: now.Add(3 * time.Hour),
			}
			snapshot.State = gameState
			decision, err = test.evaluate(t.Context(), snapshot)
			if err != nil || decision.Status == "gated" || strings.Contains(decision.Detail, "boi ID 24") {
				t.Fatalf("active booster decision = %#v err=%v", decision, err)
			}

			booster := gameState.Market.Boosters[GameData.GallantryPointsBoosterID]
			booster.ExpiresAt = now
			gameState.Market.Boosters[GameData.GallantryPointsBoosterID] = booster
			snapshot.State = gameState
			decision, err = test.evaluate(t.Context(), snapshot)
			if err != nil || decision.Status != "gated" {
				t.Fatalf("expired booster decision = %#v err=%v", decision, err)
			}
		})
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
