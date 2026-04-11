package sentbird

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// LoggedBird is one outbound bird batch we sent; used to reconcile against GAM + TU.
type LoggedBird struct {
	SourceCastleID int         `json:"sourceCastleId"`
	SourceKID      int         `json:"sourceKid"`
	TargetX        int         `json:"targetX"`
	TargetY        int         `json:"targetY"`
	Troops         map[int]int `json:"troops"`
	SentAtUnix     int64       `json:"sentAtUnix"`
}

// File is persisted JSON for AutoBird reconciliation across runs.
type File struct {
	PlayerID int          `json:"playerId"`
	Birds    []LoggedBird `json:"birds"`
}

var fileMu sync.Mutex

func filePath() string {
	return filepath.Join(Paths.DataDir(), "autobird_sent.json")
}

func legacySentBirdBesideExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "autobird_sent.json")
}

// Copies autobird_sent.json from beside the executable into Data/ once (older builds).
func tryMigrateSentBirdFromLegacyUnlocked() {
	newPath := filePath()
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	old := legacySentBirdBesideExe()
	if old == "" || newPath == old {
		return
	}
	b, err := os.ReadFile(old)
	if err != nil || len(b) == 0 {
		return
	}
	_ = os.MkdirAll(filepath.Dir(newPath), 0755)
	_ = os.WriteFile(newPath, b, 0644)
}

// Load returns stored birds (empty if missing).
func Load() *File {
	fileMu.Lock()
	defer fileMu.Unlock()
	tryMigrateSentBirdFromLegacyUnlocked()
	data, err := os.ReadFile(filePath())
	if err != nil {
		return &File{Birds: []LoggedBird{}}
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		log.Printf("[sentbird] parse: %v", err)
		return &File{Birds: []LoggedBird{}}
	}
	if f.Birds == nil {
		f.Birds = []LoggedBird{}
	}
	return &f
}

func saveInternal(f *File) {
	if f == nil {
		return
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filePath(), data, 0644)
}

// Save replaces the whole list.
func Save(f *File) {
	fileMu.Lock()
	defer fileMu.Unlock()
	saveInternal(f)
}

// Append adds one bird and saves.
func Append(playerID int, b LoggedBird) {
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	f.PlayerID = playerID
	f.Birds = append(f.Birds, b)
	saveInternal(f)
}

func loadUnlocked() *File {
	tryMigrateSentBirdFromLegacyUnlocked()
	data, err := os.ReadFile(filePath())
	if err != nil {
		return &File{Birds: []LoggedBird{}}
	}
	var f File
	if json.Unmarshal(data, &f) != nil {
		return &File{Birds: []LoggedBird{}}
	}
	if f.Birds == nil {
		f.Birds = []LoggedBird{}
	}
	return &f
}

// ReplaceBirds overwrites the bird list (after reconciliation).
func ReplaceBirds(playerID int, birds []LoggedBird) {
	fileMu.Lock()
	defer fileMu.Unlock()
	saveInternal(&File{PlayerID: playerID, Birds: birds})
}

// Clear removes all logged sent birds; keeps playerId from the existing file for the next append cycle.
func Clear() {
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	saveInternal(&File{PlayerID: f.PlayerID, Birds: []LoggedBird{}})
}
