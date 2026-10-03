package State

import (
	"testing"
	"time"
)

func TestRubyConfirmationBlocksAndOccurrenceHold(t *testing.T) {
	session := SessionState{Generation: 3, LoggedIn: true, SocketReady: true}
	for _, tc := range []struct {
		amount int64
		want   bool
	}{{250, true}, {2500, true}, {2501, false}, {-1, false}, {0, true}, {1000001, true}} {
		setting := RubyConfirmationState{Known: true, Amount: tc.amount, Generation: 3}
		if blocked, _ := RubyConfirmationBlocks(setting, session, 2500); blocked != tc.want {
			t.Fatalf("%d blocked=%t", tc.amount, blocked)
		}
	}
	rejectedAt := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	record := GlobalEffectPurchaseRecord{Outcome: GlobalEffectPurchaseConfirmationRequired, ResultObservedAt: rejectedAt}
	setting := RubyConfirmationState{Known: true, Amount: -1, Generation: 3, ObservedAt: rejectedAt}
	if !RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("equal observation lifted hold")
	}
	setting.ObservedAt = rejectedAt.Add(time.Second)
	if RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("fresh disabled confirmation did not lift hold")
	}
	setting.Amount = 2501
	if RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("fresh high threshold did not lift hold")
	}
	setting.Generation--
	if !RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("wrong session lifted hold")
	}
	setting.Generation++
	setting.Known = false
	if !RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("unknown setting lifted hold")
	}
}
