package GameParser

import (
	"testing"
	"time"

	"CitadelDesktop/Server/Models"
	movement "CitadelDesktop/Server/Models/Movement"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
)

func TestRiftLaunchWireItemUsesAuthoritativeCommanderStatus(t *testing.T) {
	now := time.Now().Unix()
	gs := &Models.GameState{Movement: movement.NewPlayerMovement()}
	gs.Movement.SetCommanderRoster([]movement.CommanderRosterEntry{{CommanderID: 7}})
	launch := riftattack.SavedLaunch{
		ID: "status-check",
		Body: riftattack.CRALaunchBodyJSON{
			"LID": float64(7),
			"SX":  float64(10),
			"SY":  float64(20),
			"TX":  float64(30),
			"TY":  float64(40),
		},
	}

	assertRiftLaunchStatus(t, launchWireItem(launch, gs, nil), "syncing", true)

	gs.Movement.ReplaceSnapshot(nil, now)
	assertRiftLaunchStatus(t, launchWireItem(launch, gs, nil), "free", false)

	gs.Movement.ReplaceSnapshot([]Models.GAMMovement{{
		MID:          100,
		TT:           60,
		CommanderID:  7,
		ReceivedUnix: now,
	}}, now)
	assertRiftLaunchStatus(t, launchWireItem(launch, gs, nil), "outbound", true)

	gs.Movement.ReplaceSnapshot(nil, now-movement.CommanderSnapshotFreshnessSeconds-1)
	assertRiftLaunchStatus(t, launchWireItem(launch, gs, nil), "unknown", true)

	gs.Movement.ReplaceSnapshot(nil, now)
	unknownCommander := launch
	unknownCommander.Body = riftattack.CRALaunchBodyJSON{"LID": float64(99)}
	assertRiftLaunchStatus(t, launchWireItem(unknownCommander, gs, nil), "unknown", true)
}

func assertRiftLaunchStatus(t *testing.T, item map[string]interface{}, wantStatus string, wantBusy bool) {
	t.Helper()
	if got := item["commanderStatus"]; got != wantStatus {
		t.Fatalf("commanderStatus = %v, want %q", got, wantStatus)
	}
	if got := item["commanderBusy"]; got != wantBusy {
		t.Fatalf("commanderBusy = %v, want %t", got, wantBusy)
	}
	if got := item["canResend"]; got != !wantBusy {
		t.Fatalf("canResend = %v, want %t", got, !wantBusy)
	}
}
