package State

import (
	"os"
	"testing"
)

func TestSnapshotRoundTripResetsSession(t *testing.T) {
	directory := t.TempDir()
	state := NewGameState()
	state.Revision = 42
	state.Player.ID = 99
	state.Session.Status = "connected"
	state.Session.LoggedIn = true
	state.Session.ServerURL = "wss://ep-live-us1-game.example.test/socket"
	state.Castles[7] = CastleState{ID: 7, Name: "Test", Resources: map[ResourceID]ResourceBalance{}}
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 42 || loaded.Player.ID != 99 || loaded.Castles[7].Name != "Test" {
		t.Fatalf("unexpected loaded state: %#v", loaded)
	}
	if loaded.Session.Status != "stopped" || loaded.Session.LoggedIn {
		t.Fatalf("session was not reset: %#v", loaded.Session)
	}
	if loaded.Session.ServerURL != state.Session.ServerURL {
		t.Fatalf("last server URL was not retained: %#v", loaded.Session)
	}
	info, err := os.Stat(snapshotPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot permissions = %o", info.Mode().Perm())
	}
}

func TestSnapshotLoadMovesInspectedAllianceOutOfOwnSlot(t *testing.T) {
	directory := t.TempDir()
	state := NewGameState()
	state.Player.AllianceID = 9
	state.Alliance = AllianceState{ID: 10, Name: "Inspected", Members: []AllianceMember{}, Holdings: []AllianceHolding{}}
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Alliance.ID != 9 || loaded.Alliance.Name != "" {
		t.Fatalf("own alliance = %+v", loaded.Alliance)
	}
	if loaded.Alliances[10].Name != "Inspected" {
		t.Fatalf("alliance directory = %+v", loaded.Alliances)
	}
}
