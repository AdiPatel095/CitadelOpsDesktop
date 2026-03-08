package Models

import (
	"log"
	"sync"
)

// SaveInCastleTroops represents the troops to keep in castle per castle ID
// This is loaded from BirdIgnoreList.json and persists across restarts
type SaveInCastleTroops struct {
	Troops map[int]map[int]int // CastleID -> (unitID -> count)
}

// BirdIgnoreListFile represents the JSON file structure
type BirdIgnoreListFile struct {
	Description string             `json:"description"`
	Castles     map[string][][]int `json:"castles"` // CastleID (string) -> [[troopID, count], ...]
}

// AutoBirdDelayConfig holds the min and max delay hours and min send amount
type AutoBirdDelayConfig struct {
	MinDelay int
	MaxDelay int
	MinSend  int
}

// GetSaveAmount returns the amount of troops to save for a specific castle and unit
func (s *SaveInCastleTroops) GetSaveAmount(castleID, unitID int) (int, bool) {
	if s.Troops == nil {
		return 0, false
	}

	// Check specific castle config
	if castleConfig, ok := s.Troops[castleID]; ok {
		if amount, ok := castleConfig[unitID]; ok {
			return amount, true
		}
	}

	return 0, false
}

// TabPriority defines the scheduled group for a specific tab ID
type TabPriority string

const (
	Priority1 TabPriority = "P1"
	Priority2 TabPriority = "P2"
	Priority3 TabPriority = "P3"
	Ignored   TabPriority = "Ignored"
)

// SettingsState holds the global settings configurations for the user.
// This includes preferences for the Attack Scheduler like random timers and tab priorities.
type SettingsState struct {
	MinAttackDelay float64                `json:"minAttackDelay"`
	MaxAttackDelay float64                `json:"maxAttackDelay"`
	TabPriorities  map[string]TabPriority `json:"tabPriorities"` // Map of TabID string to Priority Group

	// Global Connection/Feature Flags
	BotEnabled       bool `json:"botEnabled"`
	AutoBirdEnabled  bool `json:"autoBirdEnabled"`
	BeriWorldEnabled bool `json:"beriWorldEnabled"`

	// Auto Bird Configuration
	BirdIgnoreList SaveInCastleTroops  `json:"birdIgnoreList"`
	AutoBirdDelay  AutoBirdDelayConfig `json:"autoBirdDelay"`
}

var (
	instanceSettingsState *SettingsState
	onceSettingsState     sync.Once
)

// GetSettingsState returns the singleton instance of SettingsState.
func GetSettingsState() *SettingsState {
	onceSettingsState.Do(func() {
		instanceSettingsState = &SettingsState{
			MinAttackDelay:  4.0,
			MaxAttackDelay:  6.0,
			TabPriorities:   make(map[string]TabPriority),
			BotEnabled:      false,
			AutoBirdEnabled: false,
			BirdIgnoreList: SaveInCastleTroops{
				Troops: make(map[int]map[int]int),
			},
			AutoBirdDelay: AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0},
		}
	})
	return instanceSettingsState
}

// Reset clears the settings state back to default values.
func (s *SettingsState) Reset() {
	s.MinAttackDelay = 4.0
	s.MaxAttackDelay = 6.0
	s.TabPriorities = make(map[string]TabPriority)
	s.BotEnabled = false
	s.AutoBirdEnabled = false
	s.BirdIgnoreList.Troops = make(map[int]map[int]int)
	s.AutoBirdDelay = AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0}
}

// UpdateBirdIgnoreList updates the in-memory bird ignore list from the given map
func (s *SettingsState) UpdateBirdIgnoreList(data map[int]map[int]int) {
	s.BirdIgnoreList.Troops = data
	log.Printf("[BirdIgnoreList] Updated in-memory config for %d castles", len(s.BirdIgnoreList.Troops))
}

// ClearBirdIgnoreList clears the BirdIgnoreList from memory to save space
func (s *SettingsState) ClearBirdIgnoreList() {
	s.BirdIgnoreList.Troops = nil
	log.Println("[BirdIgnoreList] Cleared from memory")
}
