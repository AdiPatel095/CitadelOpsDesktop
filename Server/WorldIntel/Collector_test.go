package WorldIntel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestBuildObservationBatchIncludesOnlyPublicWorldData(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 7, 0, 0, time.UTC)
	state := State.NewGameState()
	state.UpdatedAt = now
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.Generation = 1
	state.Session.BaselineGeneration = 1
	state.Account = State.AccountBindingState{UID: 999, WorldID: "wss://EP-LIVE-US1-GAME.EXAMPLE/ws", PlayerID: 7}
	state.Player = State.PlayerState{
		ID: 7, Name: "Owner", AllianceID: 9, Level: 70, LegendLevel: 950, Might: 12345, Glory: 321,
		Resources: map[State.ResourceID]float64{1: 999999}, Currencies: map[State.CurrencyID]float64{2: 888888},
	}
	state.Alliance = State.AllianceState{
		ID: 9, Name: "Public Alliance", ObservedAt: now,
		Members:  []State.AllianceMember{{PlayerID: 7, Name: "Owner", Level: 70, LegendLevel: 950, Might: 12345, ReturnProtectionSec: 3600}},
		Holdings: []State.AllianceHolding{{CastleID: 77, PlayerID: 7, KingdomID: 0, X: 12, Y: 34, SlotType: 1}},
	}
	state.Alliances[9] = state.Alliance
	state.EventScores.RankingByEvent[72] = State.EventRankingState{
		EventID: 72, ObservedAt: now,
		Entries: []State.EventRankingEntry{{AllianceID: 9, Alliance: "Public Alliance", Rank: 3, Score: 456}},
	}

	batch, available, err := BuildObservationBatch(state, now)
	if err != nil {
		t.Fatal(err)
	}
	if !available || batch.WorldID != "ep-live-us1-game.example" || len(batch.Players) != 1 || len(batch.Holdings) != 1 {
		t.Fatalf("unexpected batch: %#v", batch)
	}
	if err := ValidateFinalizedBatch(batch); err != nil {
		t.Fatalf("finalized batch is invalid: %v", err)
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"999999", "888888", "returnProtection", "rptSeconds", "troops", "inventory", "UID"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("batch exposed private field %q: %s", forbidden, encoded)
		}
	}
}

func TestBuildObservationBatchRequiresCurrentLiveSession(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Account.WorldID = "wss://world.example/socket"
	state.Player = State.PlayerState{ID: 7, Name: "Persisted Player", Might: 1234}
	state.Session.LoggedIn = false
	if _, available, err := BuildObservationBatch(state, now); err != nil || available {
		t.Fatalf("offline batch available = %t, err = %v", available, err)
	}
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.Generation = 2
	state.Session.BaselineGeneration = 1
	if _, available, err := BuildObservationBatch(state, now); err != nil || available {
		t.Fatalf("stale-generation batch available = %t, err = %v", available, err)
	}
}

func TestBuildObservationBatchIgnoresUnrelatedStateTimestampWithinBucket(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 7, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Account.WorldID = "wss://world.example/socket"
	state.Player = State.PlayerState{ID: 7, Name: "Player", Might: 1234}
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.Generation = 1
	state.Session.BaselineGeneration = 1
	state.UpdatedAt = now
	first, available, err := BuildObservationBatch(state, now)
	if err != nil || !available {
		t.Fatalf("first batch available = %t, err = %v", available, err)
	}
	state.UpdatedAt = now.Add(5 * time.Minute)
	second, available, err := BuildObservationBatch(state, now.Add(5*time.Minute))
	if err != nil || !available {
		t.Fatalf("second batch available = %t, err = %v", available, err)
	}
	if first.BatchID != second.BatchID {
		t.Fatalf("unrelated timestamp changed batch identity: %s != %s", first.BatchID, second.BatchID)
	}
}

func TestFinalizeBatchRejectsPayloadIdentityChanges(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	batch, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		Players: []PlayerObservation{{WorldID: "world.example", PlayerID: 1, Name: "Player", Source: "account", ObservedAt: now}},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch.Players[0].Name = "Changed"
	if err := ValidateFinalizedBatch(batch); err == nil {
		t.Fatal("expected changed payload to fail batch identity validation")
	}
}

func TestNormalizeWorldIDRemovesStandardTransportDetails(t *testing.T) {
	tests := map[string]string{
		"wss://EP-LIVE-US1-GAME.EXAMPLE:443/ws": "ep-live-us1-game.example",
		"https://World.Example:443/game/":       "world.example",
		"ws://World.Example:80/socket":          "world.example",
		"https://World.Example:8443/game":       "world.example:8443",
		"WORLD.EXAMPLE:443/socket":              "world.example",
		"WORLD.EXAMPLE/":                        "world.example",
	}
	for input, expected := range tests {
		if actual := NormalizeWorldID(input); actual != expected {
			t.Errorf("NormalizeWorldID(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestFinalizeBatchRejectsFarFutureRowTimestamps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	_, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		Players: []PlayerObservation{{
			WorldID: "world.example", PlayerID: 1, Name: "Player", Source: "account",
			ObservedAt: now.Add(24 * time.Hour),
		}},
	})
	if err == nil {
		t.Fatal("expected a far-future row timestamp to fail validation")
	}
}
