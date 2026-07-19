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
	state.Session.SocketReady = true
	state.Session.Generation = 7
	state.Session.BaselineGeneration = 7
	state.Session.ConnectionGeneration = 3
	state.Session.ServerURL = "wss://ep-live-us1-game.example.test/socket"
	state.Castles[7] = CastleState{ID: 7, Name: "Test", Resources: map[ResourceID]ResourceBalance{}}
	islandReturnKey := StormIslandReturnKey(4, 101, 102)
	state.Storm.IslandReturns[islandReturnKey] = StormIslandReturnState{
		KingdomID: 4, SourceCastleID: 7, TargetX: 101, TargetY: 102, IslandObjectID: 777,
		ReportID: 202, Status: StormIslandReturnReady, LeaveBehind: 1, Survivors: map[UnitID]int64{10: 4, 12: 5},
	}
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
	loadedReturn := loaded.Storm.IslandReturns[islandReturnKey]
	if loadedReturn.Status != StormIslandReturnReady || loadedReturn.ReportID != 202 || loadedReturn.Survivors[12] != 5 {
		t.Fatalf("pending Storm island return was not restored: %#v", loadedReturn)
	}
	if loaded.Session.Status != "stopped" || loaded.Session.LoggedIn {
		t.Fatalf("session was not reset: %#v", loaded.Session)
	}
	if loaded.Session.Generation != state.Session.Generation {
		t.Fatalf("session generation = %d, want %d", loaded.Session.Generation, state.Session.Generation)
	}
	if loaded.Session.BaselineGeneration != 0 || loaded.Session.ConnectionGeneration != 0 {
		t.Fatalf("persisted live session epoch was retained: %#v", loaded.Session)
	}
	if loaded.Session.ServerURL != state.Session.ServerURL {
		t.Fatalf("last server URL was not retained: %#v", loaded.Session)
	}
	if loaded.Account.WorldID != state.Session.ServerURL || loaded.Account.PlayerID != state.Player.ID {
		t.Fatalf("snapshot account binding = %#v", loaded.Account)
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
