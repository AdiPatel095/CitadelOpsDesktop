package settings

import (
	"log"
	"sync"
)

// SaveInCastleTroops is per-castle “keep at home” amounts (AutoBird subtracts these from **I**).
type SaveInCastleTroops struct {
	Troops map[int]map[int]int // CastleID -> (unitID -> count)
}

// GetSaveAmount returns the amount to keep in castle for a unit (0 if unset).
func (s *SaveInCastleTroops) GetSaveAmount(castleID, unitID int) (int, bool) {
	if s.Troops == nil {
		return 0, false
	}
	if castleConfig, ok := s.Troops[castleID]; ok {
		if amount, ok := castleConfig[unitID]; ok {
			return amount, true
		}
	}
	return 0, false
}

// AutoBirdDelayConfig random delay range (hours) and minimum surplus to send.
type AutoBirdDelayConfig struct {
	MinDelay int `json:"minDelay"`
	MaxDelay int `json:"maxDelay"`
	MinSend  int `json:"minSend"`
}

// RecruitTroopsConfig represents the target troops to recruit per castle
// CastleID -> (unitID -> targetAmount)
type RecruitTroopsConfig struct {
	Targets map[int]map[int]int `json:"targets"`
}

// GetTargetAmount returns the target amount of troops to recruit for a specific castle and unit
func (c *RecruitTroopsConfig) GetTargetAmount(castleID, unitID int) (int, bool) {
	if c.Targets == nil {
		return 0, false
	}

	if castleConfig, ok := c.Targets[castleID]; ok {
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
	BotEnabled           bool `json:"botEnabled"`
	AutoBirdEnabled      bool `json:"autoBirdEnabled"`
	RecruitTroopsEnabled bool `json:"recruitTroopsEnabled"`

	AutoBirdDelay  AutoBirdDelayConfig `json:"autoBirdDelay"`
	BirdIgnoreList SaveInCastleTroops  `json:"birdIgnoreList"`

	// Recruit Troops Configuration
	RecruitTroopsList RecruitTroopsConfig `json:"recruitTroopsList"`
}

var (
	instanceSettingsState *SettingsState
	onceSettingsState     sync.Once
)

// GetSettingsState returns the singleton instance of SettingsState.
func GetSettingsState() *SettingsState {
	onceSettingsState.Do(func() {
		instanceSettingsState = &SettingsState{
			MinAttackDelay:       4.0,
			MaxAttackDelay:       6.0,
			TabPriorities:        make(map[string]TabPriority),
			BotEnabled:           false,
			AutoBirdEnabled:      false,
			RecruitTroopsEnabled: false,
			AutoBirdDelay:        AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0},
			BirdIgnoreList:       SaveInCastleTroops{Troops: make(map[int]map[int]int)},
			RecruitTroopsList: RecruitTroopsConfig{
				Targets: make(map[int]map[int]int),
			},
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
	s.RecruitTroopsEnabled = false
	s.AutoBirdDelay = AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0}
	s.BirdIgnoreList.Troops = make(map[int]map[int]int)
	s.RecruitTroopsList.Targets = make(map[int]map[int]int)
}

// UpdateBirdIgnoreList replaces the in-memory ignore map (from UI).
func (s *SettingsState) UpdateBirdIgnoreList(data map[int]map[int]int) {
	s.BirdIgnoreList.Troops = data
	log.Printf("[BirdIgnoreList] updated for %d castles", len(data))
}

// ClearBirdIgnoreList clears ignore config from memory.
func (s *SettingsState) ClearBirdIgnoreList() {
	s.BirdIgnoreList.Troops = nil
	log.Println("[BirdIgnoreList] cleared")
}

// UpdateRecruitTroopsList updates the in-memory recruit troops list from the given map
func (s *SettingsState) UpdateRecruitTroopsList(data map[int]map[int]int) {
	s.RecruitTroopsList.Targets = data
	log.Printf("[RecruitTroopsList] Updated in-memory config for %d castles", len(s.RecruitTroopsList.Targets))
}

// ClearRecruitTroopsList clears the RecruitTroopsList from memory
func (s *SettingsState) ClearRecruitTroopsList() {
	s.RecruitTroopsList.Targets = nil
	log.Println("[RecruitTroopsList] Cleared from memory")
}
