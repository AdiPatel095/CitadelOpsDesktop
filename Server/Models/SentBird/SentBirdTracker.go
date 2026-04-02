package sentbird

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SentBird represents a bird that was sent by the AutoBird feature
// Since MID changes when birds reach their destination, we track by troop composition
type SentBird struct {
	CastleID          int       `json:"castleID"`          // Source castle
	TargetX           int       `json:"targetX"`           // Target X coordinate
	TargetY           int       `json:"targetY"`           // Target Y coordinate
	KingdomID         int       `json:"kingdomID"`         // Kingdom ID
	TroopComposition  [][]int   `json:"troopComposition"`  // Array of [troopID, count]
	SentTime          time.Time `json:"sentTime"`          // When the bird was sent
	ExpectedExpiry    time.Time `json:"expectedExpiry"`    // Calculated expiration time
	OneWayTimeSeconds int       `json:"oneWayTimeSeconds"` // Travel time for recalculation
	DelayHrs          int       `json:"delayHrs"`          // Wait delay hours
}

// SentBirdFile represents the structure of the persisted JSON file
type SentBirdFile struct {
	PlayerID int        `json:"playerID"` // Player's OID for validation
	Birds    []SentBird `json:"birds"`    // Active birds
}

const sentBirdsFilename = "sentBirds.json"

var (
	sentBirdsMu sync.Mutex
)

// getSentBirdsFilePath returns the absolute path to the sentBirds.json file
func getSentBirdsFilePath() string {
	ex, err := os.Executable()
	if err != nil {
		return sentBirdsFilename
	}
	return filepath.Join(filepath.Dir(ex), sentBirdsFilename)
}

// SaveSentBird appends a single bird to the persistent file
func AppendSentBird(playerID int, bird SentBird) {
	sentBirdsMu.Lock()
	defer sentBirdsMu.Unlock()

	// Load existing
	fileData := loadSentBirdsInternal()
	if fileData == nil {
		fileData = &SentBirdFile{
			PlayerID: playerID,
			Birds:    []SentBird{},
		}
	} else if fileData.PlayerID != playerID {
		// Player changed, reset file
		fileData = &SentBirdFile{
			PlayerID: playerID,
			Birds:    []SentBird{},
		}
	}

	// Append
	fileData.Birds = append(fileData.Birds, bird)

	saveSentBirdsInternal(fileData)
}

// SaveSentBirds overwrites the file with the given list
func SaveSentBirds(playerID int, birds []SentBird) {
	sentBirdsMu.Lock()
	defer sentBirdsMu.Unlock()

	fileData := &SentBirdFile{
		PlayerID: playerID,
		Birds:    birds,
	}
	saveSentBirdsInternal(fileData)
}

// LoadSentBirds reads the sent birds from file
func LoadSentBirds() *SentBirdFile {
	sentBirdsMu.Lock()
	defer sentBirdsMu.Unlock()
	return loadSentBirdsInternal()
}

// ClearSentBirds deletes the persistence file
func ClearSentBirds() {
	sentBirdsMu.Lock()
	defer sentBirdsMu.Unlock()

	path := getSentBirdsFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[SentBirdTracker] Error clearing sent birds file: %v", err)
	} else {
		log.Println("[SentBirdTracker] Cleared sent birds file")
	}
}

// Internal helper - caller must hold lock
func loadSentBirdsInternal() *SentBirdFile {
	path := getSentBirdsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[SentBirdTracker] Error reading file: %v", err)
		}
		return nil
	}

	var fileData SentBirdFile
	if err := json.Unmarshal(data, &fileData); err != nil {
		log.Printf("[SentBirdTracker] Error parsing file: %v", err)
		return nil
	}

	return &fileData
}

// Internal helper - caller must hold lock
func saveSentBirdsInternal(data *SentBirdFile) {
	path := getSentBirdsFilePath()
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("[SentBirdTracker] Error marshalling data: %v", err)
		return
	}

	if err := os.WriteFile(path, bytes, 0600); err != nil {
		log.Printf("[SentBirdTracker] Error writing file: %v", err)
	} else {
		// Only log on new file creation or significant updates to avoid spam?
		// log.Printf("[SentBirdTracker] Saved %d birds to %s", len(data.Birds), path)
	}
}
