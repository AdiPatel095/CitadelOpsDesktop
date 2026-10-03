package Telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestInboundResponseFailed(t *testing.T) {
	for _, test := range []struct {
		opcode string
		code   int
		failed bool
	}{
		{"CORE_POL", 10005, false},
		{"core_pol", 10005, false},
		{"CoRe_PoL", 10005, false},
		{"CORE_OTHER", 10005, false},
		{"CORE_POL", 0, true},
		{"CORE_POL", 10006, true},
		{"gam", 10005, true},
		{"gam", 0, false},
		{"gam", 63, true},
		{"gam", -1, true},
		{"CORE", 10005, true},
		{"other_core_pol", 10005, true},
	} {
		t.Run(fmt.Sprintf("%s/%d", test.opcode, test.code), func(t *testing.T) {
			if failed := InboundResponseFailed(test.opcode, test.code); failed != test.failed {
				t.Fatalf("InboundResponseFailed(%q, %d) = %v, want %v", test.opcode, test.code, failed, test.failed)
			}
		})
	}
}

func TestStoreClassifiesInboundResponsesByDomain(t *testing.T) {
	for _, test := range []struct {
		opcode string
		code   int
		label  string
	}{
		{"CORE_POL", 10005, "RECV"},
		{"CORE_POL", 0, "ERROR"},
		{"CORE_POL", 10006, "ERROR"},
		{"gam", 10005, "ERROR"},
		{"gam", 0, "RECV"},
		{"gam", 63, "ERROR"},
	} {
		t.Run(fmt.Sprintf("%s/%d", test.opcode, test.code), func(t *testing.T) {
			store := NewStore(10)
			defer store.Close()
			raw := fmt.Sprintf("%%xt%%%s%%1%%%d%%{}%%", test.opcode, test.code)
			frame, err := Protocol.Decode(raw, Protocol.DirectionInbound, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
			lines := store.Tail(ChannelWebSocketGame, 10)
			if len(lines) != 1 || !strings.Contains(lines[0], "["+test.label+"] ["+frame.Opcode+"] "+raw) {
				t.Fatalf("telemetry = %q, want one %s line preserving the wire frame", lines, test.label)
			}
		})
	}
}

func TestStoreCoreSuccessStillRecordsReducerErrors(t *testing.T) {
	store := NewStore(10)
	defer store.Close()
	frame, err := Protocol.Decode("%xt%CORE_POL%1%10005%{}%", Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Protocol.CommittedFrame{Frame: frame}, errors.New("synthetic reducer failure"))
	lines := store.Tail(ChannelWebSocketGame, 10)
	if len(lines) != 2 || !strings.Contains(lines[0], "[RECV]") || !strings.Contains(lines[1], "[ERROR] [core_pol] synthetic reducer failure") {
		t.Fatalf("telemetry = %q, want a successful response and a separate reducer error", lines)
	}
}

func TestStoreCapturedCorePolPush(t *testing.T) {
	raw, err := os.ReadFile("testdata/core_pol_success.xt")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := Protocol.Decode(strings.TrimSpace(string(raw)), Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != "core_pol" || frame.ResponseCode == nil || *frame.ResponseCode != 10005 || len(frame.Payload) == 0 {
		t.Fatal("captured fixture must decode as a CORE_POL/10005 JSON push")
	}
	var offers []struct {
		Details []struct {
			VisualComponents []json.RawMessage `json:"visualComponents"`
			RewardComponents []json.RawMessage `json:"rewardComponents"`
		} `json:"OD"`
	}
	if err := json.Unmarshal(frame.Payload, &offers); err != nil {
		t.Fatal(err)
	}
	if len(offers) == 0 || len(offers[0].Details) == 0 || len(offers[0].Details[0].VisualComponents) == 0 || len(offers[0].Details[0].RewardComponents) == 0 {
		t.Fatal("captured fixture must retain the sender's offer-list and component arrays")
	}
	store := NewStore(10)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
	lines := store.Tail(ChannelWebSocketGame, 10)
	if len(lines) != 1 || !strings.Contains(lines[0], "[RECV] [core_pol] "+frame.Raw) {
		t.Fatalf("telemetry = %q, want one successful push with its full wire shape", lines)
	}
	store.flushPersistence()
	paths := channelLogPathsNewest(store.channelsDir, ChannelWebSocketGame)
	if len(paths) != 1 {
		t.Fatalf("persistent logs = %d, want one", len(paths))
	}
	contents, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	// Opcode-error inventories count ERROR lines; there is no separate inbound
	// response-error counter in Store.
	if errorCount := strings.Count(string(contents), "[ERROR]"); errorCount != 0 {
		t.Fatalf("persisted push error count = %d, want zero", errorCount)
	}
	if !strings.Contains(string(contents), "[RECV] [core_pol] "+frame.Raw) {
		t.Fatal("persistent telemetry did not retain the successful push")
	}
}
