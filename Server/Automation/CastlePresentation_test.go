package Automation

import (
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"strings"
	"testing"
	"time"
)

func TestCastleDecisionVariantsKeepUserNamesAndExactIDs(t *testing.T) {
	variants := []string{"bird_refresh_target", "bird_refresh_expired", "bird_refresh_inventory", "bird_restart_settings", "bird_restart_preset", "bird_discover", "bird_dispatch", "station_evacuate", "fortress_discover", "fortress_refresh_purchased", "fortress_refresh_destination", "fortress_allocate", "fortress_refresh_arrived", "fortress_settle_transfer"}
	values := Localization.Params{"troops": int64(1234), "target": "0017"}
	for _, variant := range variants {
		named := castleDecisionDescriptor(variant, State.CastleState{ID: 17, Name: "Castle {admin}"}, values)
		unnamed := castleDecisionDescriptor(variant, State.CastleState{ID: 17}, values)
		if named == nil || named.Params["castle"] != "Castle {admin}" || strings.Contains(named.Fallback, "{admin}") {
			t.Fatalf("%s name became template: %+v", variant, named)
		}
		if unnamed == nil || unnamed.Params["castleID"] != "17" || !strings.Contains(unnamed.Fallback, "castle {castleID}") {
			t.Fatalf("%s generated noun lost: %+v", variant, unnamed)
		}
		if named.Params["troops"] != int64(1234) || named.Params["target"] != "0017" {
			t.Fatal("typed quantity/exact ID changed")
		}
		if _, found := values["castle"]; found {
			t.Fatal("caller params mutated")
		}
	}
	if castleDecisionDescriptor("unknown", State.CastleState{}, nil) != nil {
		t.Fatal("unknown template inferred")
	}
}

func TestAutoStationCountdownDescriptorPreservesTiming(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	for _, protection := range []bool{false, true} {
		gameState := State.NewGameState()
		gameState.Player.ID = 7
		mode := -1
		protectionSeconds := int64(0)
		key := "server.automation.station_evacuation_window"
		if protection {
			mode = 1
			protectionSeconds = 3600
			key = "server.automation.station_gate_window"
		}
		gameState.Player.ProtectionMode = State.PlayerProtectionModeState{ModeState: mode, RemainingSec: protectionSeconds, ObservedAt: now}
		gameState.Player.AllianceObservedAt = now
		gameState.Alliance = State.AllianceState{ID: 9, ObservedAt: now,
			Members:  []State.AllianceMember{{PlayerID: 1, ReturnProtectionSec: 4 * 86400}},
			Holdings: []State.AllianceHolding{{CastleID: 20, PlayerID: 1, KingdomID: 0, SlotType: 1}},
		}
		gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 4}
		arrives := now.Add(90500 * time.Millisecond)
		gameState.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7, SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100, ArrivesAt: &arrives}
		decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
		if err != nil || decision.DetailDescriptor == nil {
			t.Fatalf("protection=%v decision=%+v err=%v", protection, decision, err)
		}
		descriptor := decision.DetailDescriptor
		if descriptor.Key != key || descriptor.Params["attacks"] != 1 || descriptor.Params["seconds"] != int64(31) {
			t.Fatalf("unexpected countdown: %+v", descriptor)
		}
		if decision.Request != nil || !decision.NextCheckAt.Equal(now.Add(30500*time.Millisecond)) || !strings.Contains(decision.Detail, "31s") {
			t.Fatalf("raw timing changed: %+v", decision)
		}
	}
}

func TestAutoBirdUnknownReasonKeepsWholeRawFallback(t *testing.T) {
	castle := State.CastleState{ID: 17, Name: "Castle {admin}"}
	now := time.Now().UTC()
	for _, decision := range []Decision{
		autoBirdDiscoverDecision(castle, autoBirdConfiguration{}, now, "External reason"),
		autoBirdPrepareDecision(castle, autoBirdConfiguration{}, now, "External reason"),
	} {
		if decision.DetailDescriptor != nil || decision.Detail != "External reason for Castle {admin}" {
			t.Fatalf("unknown reason was partially translated: %+v", decision)
		}
	}
}
