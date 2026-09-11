package State

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestMarketFeastFreshAtRequiresCurrentCoherentObservation(t *testing.T) {
	now := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	active := MarketFeastState{
		ID: 0, RemainingSec: 7200, ObservedAt: observedAt,
		ExpiresAt: observedAt.Add(7200 * time.Second),
	}
	inactive := MarketFeastState{ObservedAt: observedAt}
	maxAge := 5 * time.Minute

	testCases := []struct {
		name             string
		feast            MarketFeastState
		evaluatedAt      time.Time
		sessionChangedAt time.Time
		maxAge           time.Duration
		want             bool
	}{
		{name: "active", feast: active, evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: maxAge, want: true},
		{name: "explicit inactive", feast: inactive, evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: maxAge, want: true},
		{name: "session boundary inclusive", feast: active, evaluatedAt: now, sessionChangedAt: observedAt, maxAge: maxAge, want: true},
		{name: "zero session boundary", feast: active, evaluatedAt: now, maxAge: maxAge, want: true},
		{name: "zero evaluation time", feast: active, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: maxAge},
		{name: "zero max age", feast: active, evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute)},
		{name: "negative max age", feast: active, evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: -time.Second},
		{name: "unobserved default", feast: MarketFeastState{}, evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: maxAge},
		{
			name: "future observation",
			feast: MarketFeastState{
				ID: 1, RemainingSec: 60, ObservedAt: now.Add(time.Second),
				ExpiresAt: now.Add(61 * time.Second),
			},
			evaluatedAt: now, sessionChangedAt: now.Add(-2 * time.Minute), maxAge: maxAge,
		},
		{
			name: "age boundary is stale",
			feast: MarketFeastState{
				ID: 1, RemainingSec: 60, ObservedAt: now.Add(-maxAge),
				ExpiresAt: now.Add(-maxAge).Add(time.Minute),
			},
			evaluatedAt: now, sessionChangedAt: now.Add(-10 * time.Minute), maxAge: maxAge,
		},
		{name: "predates session", feast: active, evaluatedAt: now, sessionChangedAt: observedAt.Add(time.Second), maxAge: maxAge},
		{name: "inactive nonzero id", feast: MarketFeastState{ID: 1, ObservedAt: observedAt}, evaluatedAt: now, maxAge: maxAge},
		{name: "inactive nonzero duration", feast: MarketFeastState{RemainingSec: 1, ObservedAt: observedAt}, evaluatedAt: now, maxAge: maxAge},
		{
			name:        "active negative id",
			feast:       MarketFeastState{ID: -1, RemainingSec: 60, ObservedAt: observedAt, ExpiresAt: observedAt.Add(time.Minute)},
			evaluatedAt: now, maxAge: maxAge,
		},
		{
			name:        "active zero duration",
			feast:       MarketFeastState{ID: 1, ObservedAt: observedAt, ExpiresAt: observedAt.Add(time.Minute)},
			evaluatedAt: now, maxAge: maxAge,
		},
		{
			name:        "active elapsed at observation",
			feast:       MarketFeastState{ID: 1, RemainingSec: 60, ObservedAt: observedAt, ExpiresAt: observedAt},
			evaluatedAt: now, maxAge: maxAge,
		},
		{
			name:        "active expiry inconsistent with duration",
			feast:       MarketFeastState{ID: 1, RemainingSec: 60, ObservedAt: observedAt, ExpiresAt: observedAt.Add(61 * time.Second)},
			evaluatedAt: now, maxAge: maxAge,
		},
		{
			name:        "active duration overflow",
			feast:       MarketFeastState{ID: 1, RemainingSec: math.MaxInt, ObservedAt: observedAt, ExpiresAt: observedAt.Add(time.Hour)},
			evaluatedAt: now, maxAge: maxAge,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.feast.FreshAt(testCase.evaluatedAt, testCase.sessionChangedAt, testCase.maxAge); got != testCase.want {
				t.Fatalf("FreshAt() = %t, want %t for %+v", got, testCase.want, testCase.feast)
			}
		})
	}
}

func TestFeastLastPurchaseAtPersistsButIsNotClientProjected(t *testing.T) {
	directory := t.TempDir()
	purchasedAt := time.Date(2026, time.September, 8, 18, 30, 0, 0, time.UTC)
	store := NewStore(NewGameState())
	bootstrap, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level = 1
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, bootstrap, Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}
	event, err := store.ApplyComponents(Components(ComponentMarket), func(state *GameState) ([]string, bool, error) {
		state.Market.FeastLastPurchaseAt = purchasedAt
		return []string{"market"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, event, Components(event.Components...)); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Market.FeastLastPurchaseAt.Equal(purchasedAt) {
		t.Fatalf("persisted feast purchase time = %s, want %s", loaded.Market.FeastLastPurchaseAt, purchasedAt)
	}
	client, err := json.Marshal(NewClientStateSnapshot(loaded))
	if err != nil {
		t.Fatal(err)
	}
	var projected GameState
	if err := json.Unmarshal(client, &projected); err != nil {
		t.Fatal(err)
	}
	if !projected.Market.FeastLastPurchaseAt.IsZero() {
		t.Fatalf("backend feast purchase timestamp leaked into client state: %s", projected.Market.FeastLastPurchaseAt)
	}
	var envelope struct {
		Market map[string]json.RawMessage `json:"market"`
	}
	if err := json.Unmarshal(client, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, present := envelope.Market["feastLastPurchaseAt"]; present {
		t.Fatalf("backend feast purchase field leaked into client JSON: %s", envelope.Market["feastLastPurchaseAt"])
	}
}
