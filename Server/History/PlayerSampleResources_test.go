package History

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestNewPlayerSampleCapturesEveryObservedResourceAndCurrency(t *testing.T) {
	snapshot := State.NewGameState()
	snapshot.Player.ID = 42
	snapshot.Player.Resources = map[State.ResourceID]float64{
		1:  5_000,
		2:  44,
		12: 0,
		37: 88,
	}
	snapshot.Player.Currencies = map[State.CurrencyID]float64{
		9:  7,
		22: 0,
		30: 99,
	}

	sample := NewPlayerSampleAt(snapshot, nil, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	want := map[string]float64{
		"resource:1":  5_000,
		"resource:2":  44,
		"resource:12": 0,
		"resource:37": 88,
		"currency:9":  7,
		"currency:22": 0,
		"currency:30": 99,
	}
	if sample.Coins != 5_000 || sample.Rubies != 44 {
		t.Fatalf("wallet aliases coins=%v rubies=%v", sample.Coins, sample.Rubies)
	}
	if len(sample.Currencies) != len(want) {
		t.Fatalf("wallet entries = %#v", sample.Currencies)
	}
	for key, amount := range want {
		if observed, found := sample.Currencies[key]; !found || observed != amount {
			t.Fatalf("wallet %s = %v found=%t; want %v", key, observed, found, amount)
		}
	}
}
