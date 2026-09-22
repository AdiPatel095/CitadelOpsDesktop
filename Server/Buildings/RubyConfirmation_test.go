package Buildings

import (
	"CitadelDesktop/Server/State"
	"encoding/json"
	"strings"
	"testing"
)

func TestRubyUpgradeConfirmationBoundaryAndSessionAuthority(t *testing.T) {
	state := State.NewGameState()
	state.Session = State.SessionState{Generation: 2, LoggedIn: true, SocketReady: true}
	costs := []CostStatus{{Premium: true, Required: 3100}}
	for _, tc := range []struct {
		amount     int64
		known      bool
		generation uint64
		blocked    bool
	}{
		{1_000_000, true, 2, false}, {1_000_001, true, 2, true}, {9223372036854775807, true, 2, true},
		{-1, true, 2, false}, {3101, true, 2, false}, {3100, true, 2, true}, {1, true, 2, true},
		{0, true, 2, true}, {-2, true, 2, true}, {-1, false, 2, true}, {-1, true, 1, true},
	} {
		state.Player.RubyConfirmation = State.RubyConfirmationState{Amount: tc.amount, Known: tc.known, Generation: tc.generation}
		block := RubyUpgradeBlocker(state, costs)
		if (block != nil) != tc.blocked {
			t.Fatalf("%+v blocker=%+v", tc, block)
		}
	}
	state.Player.RubyConfirmation = State.RubyConfirmationState{Amount: 1, Known: true, Generation: 2}
	if got := RubyUpgradeBlocker(state, costs).Message; got != "Your ruby confirmation amount is too low for this upgrade. Upgrade cost: 3,100 rubies; game confirmation amount: 1 ruby." {
		t.Fatal(got)
	}
	state.Session.SocketReady = false
	if block := RubyUpgradeBlocker(state, costs); block == nil || !strings.Contains(block.Message, "unavailable") {
		t.Fatalf("disconnected=%+v", block)
	}
	if RubyUpgradeBlocker(state, []CostStatus{{Required: 10}}) != nil {
		t.Fatal("nonpremium blocked")
	}
	raw, _ := json.Marshal(state)
	var restored State.GameState
	_ = json.Unmarshal(raw, &restored)
	if restored.Player.RubyConfirmation.Known {
		t.Fatal("persisted authority")
	}
}
