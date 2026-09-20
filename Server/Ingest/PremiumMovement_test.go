package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func premiumMovementFixture(leader string, movementType, direction int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"M":{"MID":51,"PT":1163,"TT":896,"D":%d,"T":%d,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"PWD":267,"TWD":21600,"L":%s},"A":[[216,1500]]}`, direction, movementType, leader))
}

func TestParseMovementPreservesPremiumLeaderVariants(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name, leader        string
		id, dlid, commander *int64
	}{
		{"captured-premium", `{"DLID":-14,"GID":-1,"GEM":[],"AE":[]}`, nil, int64Pointer(-14), nil},
		{"negative-id", `{"ID":-1}`, int64Pointer(-1), nil, nil},
		{"station-id", `{"ID":-14}`, int64Pointer(-14), nil, nil},
		{"both-premium-identities", `{"ID":-1,"DLID":-14}`, int64Pointer(-1), int64Pointer(-14), nil},
		{"owned-commander", `{"ID":7}`, int64Pointer(7), nil, int64Pointer(7)},
		{"owned-with-dlid", `{"ID":7,"DLID":-14}`, int64Pointer(7), int64Pointer(-14), int64Pointer(7)},
		{"zero-commander", `{"ID":0}`, int64Pointer(0), nil, int64Pointer(0)},
	}
	for _, tt := range tests {
		for _, movementType := range []int{0, 1, 7} {
			for _, direction := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/type-%d/direction-%d", tt.name, movementType, direction), func(t *testing.T) {
					movement, ok := parseMovement(premiumMovementFixture(tt.leader, movementType, direction), now, nil)
					if !ok {
						t.Fatal("valid premium/owned leader rejected")
					}
					assertInt64Pointer(t, "ID", movement.LeaderID, tt.id)
					assertInt64Pointer(t, "DLID", movement.LeaderDLID, tt.dlid)
					var commander *int64
					if movement.CommanderID != nil {
						commander = int64Pointer(int64(*movement.CommanderID))
					}
					assertInt64Pointer(t, "commander", commander, tt.commander)
					if movement.WaitSeconds != 21600 || movement.Units[216] != 1500 ||
						movement.SourceCastleID != 100 || movement.TargetCastleID != 300 {
						t.Fatalf("movement tracking lost: %+v", movement)
					}
					if direction == 0 && movement.ArrivesAt == nil || direction == 1 && movement.ReturnsAt == nil {
						t.Fatal("movement completion time lost")
					}
					wire, err := json.Marshal(movement)
					if err != nil {
						t.Fatal(err)
					}
					var restored State.MovementState
					if err := json.Unmarshal(wire, &restored); err != nil {
						t.Fatal(err)
					}
					assertInt64Pointer(t, "round-trip ID", restored.LeaderID, tt.id)
					assertInt64Pointer(t, "round-trip DLID", restored.LeaderDLID, tt.dlid)
				})
			}
		}
	}
}

func TestPremiumMovementAdvancesGAMSnapshotWithoutOccupyingOwnedCommander(t *testing.T) {
	// Commander availability is reconciled against the live clock.
	now := time.Now().UTC()
	for _, authoritative := range []bool{true, false} {
		t.Run(fmt.Sprintf("authoritative-%t", authoritative), func(t *testing.T) {
			state := State.NewGameState()
			state.Player.ID = 1
			state.Session.ConnectionGeneration = 3
			state.Commanders[0] = State.CommanderState{ID: 0, Available: true}
			state.Commanders[7] = State.CommanderState{ID: 7, Available: true}
			state.MovementSnapshot = State.MovementSnapshot{Version: 2, ObservedAt: now.Add(-time.Minute)}
			premium := premiumMovementFixture(`{"DLID":-14,"GID":-1,"GEM":[],"AE":[]}`, 1, 0)
			owned := json.RawMessage(`{"M":{"MID":52,"PT":1,"TT":900,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"L":{"ID":7}}}`)
			payload := json.RawMessage(fmt.Sprintf(`{"M":[%s,%s]}`, premium, owned))
			code := 0
			_, changed, err := newMovementReducer(authoritative)(context.Background(), Protocol.Frame{
				Opcode: "gam", ResponseCode: &code, ReceivedAt: now, Payload: payload,
			}, &state, nil)
			if err != nil || !changed || state.MovementCount() != 2 {
				t.Fatalf("mixed movement snapshot: changed=%t err=%v count=%d", changed, err, state.MovementCount())
			}
			if authoritative {
				if state.MovementSnapshot.Version != 3 || !state.MovementSnapshot.ObservedAt.Equal(now) ||
					state.MovementSnapshot.ConnectionGeneration != 3 {
					t.Fatalf("GAM remained stale: %+v", state.MovementSnapshot)
				}
			} else if state.MovementSnapshot.Version != 2 {
				t.Fatal("incremental movement incorrectly established authoritative freshness")
			}
			if !state.Commanders[0].Available || state.Commanders[7].Available {
				t.Fatal("premium identity contaminated owned commander availability")
			}
			movement, ok := state.LookupMovement(51)
			if !ok || movement.CommanderID != nil || movement.LeaderDLID == nil || *movement.LeaderDLID != -14 {
				t.Fatalf("premium identity lost: %+v", movement)
			}
		})
	}
}

func TestMalformedPremiumLeaderStillBlocksGAMFreshness(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	for _, leader := range []string{
		`{}`, `{"GID":-1}`, `{"DLID":null}`, `{"DLID":"-14"}`, `{"DLID":1.5}`,
		`{"DLID":true}`, `{"DLID":[]}`, `{"DLID":9223372036854775808}`,
		`{"ID":null,"DLID":-14}`, `{"ID":"7","DLID":-14}`,
		`{"ID":7,"DLID":"-14"}`, `{"ID":7,"DLID":null}`,
	} {
		t.Run(leader, func(t *testing.T) {
			state := State.NewGameState()
			state.MovementSnapshot = State.MovementSnapshot{Version: 2, ObservedAt: now.Add(-time.Minute)}
			code := 0
			payload := json.RawMessage(fmt.Sprintf(`{"M":[%s]}`, premiumMovementFixture(leader, 1, 0)))
			_, _, err := newMovementReducer(true)(context.Background(), Protocol.Frame{
				Opcode: "gam", ResponseCode: &code, ReceivedAt: now, Payload: payload,
			}, &state, nil)
			if err != nil {
				t.Fatal(err)
			}
			if state.MovementSnapshot.Version != 2 || !state.MovementSnapshot.ObservedAt.Equal(now.Add(-time.Minute)) ||
				state.MovementCount() != 0 {
				t.Fatal("malformed leader became authoritative")
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }

func assertInt64Pointer(t *testing.T, label string, got, want *int64) {
	t.Helper()
	if (got == nil) != (want == nil) || got != nil && *got != *want {
		t.Fatalf("%s: got %v, want %v", label, got, want)
	}
}
