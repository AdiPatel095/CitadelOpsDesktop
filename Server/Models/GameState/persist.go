package gamestate

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	mapstate "CitadelDesktop/Server/Models/MapState"
)

const snapshotVersion = 1

const snapshotFileName = "game_state_snapshot.json"

var persistMu sync.Mutex

func snapshotFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return snapshotFileName
	}
	return filepath.Join(filepath.Dir(exe), snapshotFileName)
}

// PersistSnapshot writes the full in-memory GameState plus map kingdom tiles to JSON beside the executable.
func PersistSnapshot() {
	gs := GetGameState()
	kingdoms := mapstate.GetMapState().ExportKingdoms()

	persistMu.Lock()
	defer persistMu.Unlock()

	payload := struct {
		Version     int                                 `json:"version"`
		SavedAtUnix int64                               `json:"savedAtUnix"`
		GameState   GameState                           `json:"gameState"`
		MapKingdoms map[int]map[string]mapstate.MapNode `json:"mapKingdoms"`
	}{
		Version:     snapshotVersion,
		SavedAtUnix: time.Now().Unix(),
		GameState:   *gs,
		MapKingdoms: kingdoms,
	}

	data, err := json.MarshalIndent(&payload, "", "  ")
	if err != nil {
		log.Printf("[gamestate] persist marshal: %v", err)
		return
	}
	path := snapshotFilePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("[gamestate] persist write: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[gamestate] persist rename: %v", err)
		_ = os.Remove(tmp)
	}
}

// ReadSnapshotForBroadcast returns the persisted JSON as a generic map for websocket clients.
func ReadSnapshotForBroadcast() (map[string]interface{}, error) {
	persistMu.Lock()
	defer persistMu.Unlock()
	data, err := os.ReadFile(snapshotFilePath())
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// StartPeriodicSnapshotSaver writes snapshots on a fixed interval for continuous durability.
func StartPeriodicSnapshotSaver(interval time.Duration) {
	if interval <= 0 {
		interval = 90 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			PersistSnapshot()
		}
	}()
}
