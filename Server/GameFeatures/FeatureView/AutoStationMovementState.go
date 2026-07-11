package featureview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const autoStationMovementFileName = "AutoStationMovements.json"

type autoStationMovement struct {
	MID                   int    `json:"mid"`
	SourceCastleID        int    `json:"sourceCastleID"`
	TargetX               int    `json:"targetX"`
	TargetY               int    `json:"targetY"`
	SentAtUnix            int64  `json:"sentAtUnix"`
	SafeAfterUnix         int64  `json:"safeAfterUnix"`
	SnapshotVersion       uint64 `json:"snapshotVersion,omitempty"`
	Returning             bool   `json:"returning,omitempty"`
	LastRecallAttemptUnix int64  `json:"lastRecallAttemptUnix,omitempty"`
}

type autoStationMovementFile struct {
	Version   int                   `json:"version"`
	PlayerID  int                   `json:"playerID"`
	Movements []autoStationMovement `json:"movements"`
}

var autoStationMovementFileMu sync.Mutex

func autoStationMovementPath() string {
	return filepath.Join(Paths.DataDir(), autoStationMovementFileName)
}

func loadAutoStationMovements(playerID int) []autoStationMovement {
	autoStationMovementFileMu.Lock()
	defer autoStationMovementFileMu.Unlock()
	data, err := os.ReadFile(autoStationMovementPath())
	if err != nil {
		return nil
	}
	var file autoStationMovementFile
	if json.Unmarshal(data, &file) != nil || file.PlayerID != playerID {
		return nil
	}
	movements := append([]autoStationMovement(nil), file.Movements...)
	// GAM snapshot versions are process-local. A fresh process must compare persisted movements
	// against its first authoritative snapshot instead of waiting to exceed an old process counter.
	for i := range movements {
		movements[i].SnapshotVersion = 0
	}
	return movements
}

func saveAutoStationMovements(playerID int, movements []autoStationMovement) error {
	file := autoStationMovementFile{
		Version:   1,
		PlayerID:  playerID,
		Movements: append([]autoStationMovement(nil), movements...),
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	autoStationMovementFileMu.Lock()
	defer autoStationMovementFileMu.Unlock()
	if err := os.MkdirAll(Paths.DataDir(), 0755); err != nil {
		return err
	}
	path := autoStationMovementPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
