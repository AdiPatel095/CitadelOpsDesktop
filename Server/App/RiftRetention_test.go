package App

import (
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func TestRiftTombstonesExpireAfterADay(t *testing.T) {
	now := time.Now().UTC()
	s := State.NewGameState()
	s.Rift.Launches["launch"] = State.RiftLaunch{ID: "launch", Body: json.RawMessage(`{"A":[{}]}`)}
	s.Rift.DeletedLaunchIDs = map[string]int64{"old": now.Add(-25 * time.Hour).UnixMilli(), "recent": now.Add(-23 * time.Hour).UnixMilli(), "future": now.Add(time.Hour).UnixMilli()}
	app := &Application{DataDir: t.TempDir(), State: State.NewStore(s)}
	if err := app.deleteRiftTemplate(t.Context(), json.RawMessage(`{"launchId":"launch"}`)); err != nil {
		t.Fatal(err)
	}
	loaded, err := State.LoadSnapshot(app.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []State.GameState{app.State.ReadOnlyView(), loaded} {
		if _, ok := state.Rift.DeletedLaunchIDs["old"]; ok {
			t.Fatal("expired tombstone survived")
		}
		for _, id := range []string{"recent", "future", "launch"} {
			if _, ok := state.Rift.DeletedLaunchIDs[id]; !ok {
				t.Fatalf("%s tombstone missing", id)
			}
		}
		if _, ok := state.Rift.Launches["launch"]; ok {
			t.Fatal("deleted launch survived")
		}
	}
	if _, ok := s.Rift.DeletedLaunchIDs["old"]; !ok {
		t.Fatal("prune changed the prior generation")
	}
}
