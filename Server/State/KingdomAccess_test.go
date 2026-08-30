package State

import (
	"testing"
	"time"
)

func TestCastleFocusKnownUnavailableRequiresCurrentExplicitAbsence(t *testing.T) {
	now := time.Date(2026, 8, 26, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		castle     CastleState
		observedAt time.Time
		unlock     *KingdomTransportUnlock
		want       bool
	}{
		{name: "Great Empire is always retained", castle: CastleState{ID: 1, KingdomID: 0}, observedAt: now, unlock: &KingdomTransportUnlock{}, want: false},
		{name: "current row says instance absent", castle: CastleState{ID: 10, KingdomID: 10}, observedAt: now, unlock: &KingdomTransportUnlock{KingdomID: 10, Unlocked: true}, want: true},
		{name: "current row says instance exists", castle: CastleState{ID: 10, KingdomID: 10}, observedAt: now, unlock: &KingdomTransportUnlock{KingdomID: 10, Created: true}, want: false},
		{name: "unlocked Storm castle uses live seasonal representation", castle: CastleState{ID: 40, KingdomID: 4, SlotType: 12}, observedAt: now, unlock: &KingdomTransportUnlock{KingdomID: 4, Unlocked: true, Created: false}, want: false},
		{name: "locked Storm castle is absent", castle: CastleState{ID: 40, KingdomID: 4, SlotType: 12}, observedAt: now, unlock: &KingdomTransportUnlock{KingdomID: 4, Unlocked: false, Created: false}, want: true},
		{name: "missing row remains unknown", castle: CastleState{ID: 10, KingdomID: 10}, observedAt: now, want: false},
		{name: "pre-session row remains unknown", castle: CastleState{ID: 10, KingdomID: 10}, observedAt: now.Add(-time.Second), unlock: &KingdomTransportUnlock{KingdomID: 10}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewGameState()
			state.Session.ChangedAt = now
			state.KingdomTransport.ObservedAt = test.observedAt
			if test.unlock != nil {
				state.KingdomTransport.Unlocks[test.castle.KingdomID] = *test.unlock
			}
			if got := CastleFocusKnownUnavailable(state, test.castle); got != test.want {
				t.Fatalf("CastleFocusKnownUnavailable() = %t, want %t", got, test.want)
			}
		})
	}
}
