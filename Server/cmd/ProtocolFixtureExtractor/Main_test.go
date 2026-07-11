package main

import (
	"strings"
	"testing"
	"time"
)

func TestExtractLiveCommandsSelectsLatestSuccessfulPair(t *testing.T) {
	logText := strings.Join([]string{
		`2026-07-11 12:00:00.000000 [SEND] [gam] %xt%EmpireEx_21%gam%1%{}%`,
		`2026-07-11 12:00:00.020000 [MATCH] [gam] %xt%gam%1%0%{"M":[]}%`,
		`2026-07-11 12:01:00.000000 [SEND] [gam] %xt%EmpireEx_21%gam%1%{}%`,
		`2026-07-11 12:01:00.025000 [MATCH] [gam] %xt%gam%1%0%{"M":[1]}%`,
		`2026-07-11 12:02:00.000000 [SEND] [gam] %xt%EmpireEx_21%gam%1%{}%`,
		`2026-07-11 12:02:00.010000 [MATCH] [gam] %xt%gam%1%63%{}%`,
	}, "\n")
	location := time.FixedZone("test", -4*60*60)
	contracts, err := extractLiveCommands(strings.NewReader(logText), extractionOptions{
		Generation: "test/live",
		Source:     "Logs/channels/app_send.log",
		Location:   location,
		MaxDelay:   time.Second,
		Opcodes:    map[string]bool{"gam": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 {
		t.Fatalf("contracts=%d, want 1", len(contracts))
	}
	contract := contracts[0]
	if contract.Name != "live_2026_07_11_gam" || contract.SourceRequestLine != 3 || contract.SourceResponseLine != 4 {
		t.Fatalf("unexpected contract identity: %+v", contract)
	}
	if contract.Response.Status != 0 || contract.Response.DelayMS != 25 || contract.Response.FrameBytes == 0 || len(contract.Response.FrameSHA256) != 64 {
		t.Fatalf("unexpected response evidence: %+v", contract.Response)
	}
	if contract.RequestFrame != `%xt%EmpireEx_21%gam%1%{}%` {
		t.Fatalf("requestFrame=%q", contract.RequestFrame)
	}
}

func TestExtractLiveCommandsRejectsPairsOutsideWindow(t *testing.T) {
	logText := strings.Join([]string{
		`2026-07-11 12:00:00.000000 [SEND] [gam] %xt%EmpireEx_21%gam%1%{}%`,
		`2026-07-11 12:00:06.000000 [MATCH] [gam] %xt%gam%1%0%{}%`,
	}, "\n")
	_, err := extractLiveCommands(strings.NewReader(logText), extractionOptions{
		Generation: "test/live",
		Source:     "Logs/channels/app_send.log",
		Location:   time.UTC,
		MaxDelay:   5 * time.Second,
		Opcodes:    map[string]bool{"gam": true},
	})
	if err == nil || !strings.Contains(err.Error(), "no successful") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseOpcodesRejectsSessionTraffic(t *testing.T) {
	_, err := parseOpcodes("gam,lli")
	if err == nil || !strings.Contains(err.Error(), "not in the reviewed safe allowlist") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseResponseEnvelope(t *testing.T) {
	for _, test := range []struct {
		frame  string
		opcode string
		status int
	}{
		{frame: `%xt%gam%1%0%{}%`, opcode: "gam", status: 0},
		{frame: `%xt%EmpireEx_21%gam%1%63%{}%`, opcode: "gam", status: 63},
	} {
		opcode, status, err := parseResponseEnvelope(test.frame)
		if err != nil {
			t.Fatal(err)
		}
		if opcode != test.opcode || status != test.status {
			t.Fatalf("parseResponseEnvelope(%q)=(%q,%d), want (%q,%d)", test.frame, opcode, status, test.opcode, test.status)
		}
	}
}
