package settings

import (
	"CitadelDesktop/Server/Logging"
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

// AutoBirdDelayConfig random delay range (hours), minimum surplus to send, and minimum alliance RPT (days) for bird targets.
type AutoBirdDelayConfig struct {
	MinDelay   int `json:"minDelay"`
	MaxDelay   int `json:"maxDelay"`
	MinSend    int `json:"minSend"`
	MinRPTDays int `json:"minRPTDays"`
}

// RecruitTroopsConfig represents the target troops to recruit per castle
// CastleID -> (unitID -> targetAmount)
type RecruitTroopsConfig struct {
	Targets map[int]map[int]int `json:"targets"`
}

// AutoTCILevelTarget is the allowed in-game tier range (1–4) for one construction item group.
// MaxLevel is the level ceiling (legacy JSON key "amount"); MinLevel is the floor (default 1).
type AutoTCILevelTarget struct {
	MinLevel int `json:"minLevel"`
	MaxLevel int `json:"maxLevel"`
}

// Normalize clamps both levels to 1–4 and ensures MinLevel <= MaxLevel.
func (t AutoTCILevelTarget) Normalize() AutoTCILevelTarget {
	min, max := t.MinLevel, t.MaxLevel
	if min < 1 {
		min = 1
	}
	if min > 4 {
		min = 4
	}
	if max < 1 {
		max = 1
	}
	if max > 4 {
		max = 4
	}
	if min > max {
		min = max
	}
	return AutoTCILevelTarget{MinLevel: min, MaxLevel: max}
}

// AutoTCILevelTargetFromMax builds a target with floor 1 (legacy ceiling-only payloads).
func AutoTCILevelTargetFromMax(maxLevel int) AutoTCILevelTarget {
	return AutoTCILevelTarget{MinLevel: 1, MaxLevel: maxLevel}.Normalize()
}

// AutoTCIConfig is per-castle selected temporary / construction items (by constructionItemID).
type AutoTCIConfig struct {
	Targets map[int]map[int]AutoTCILevelTarget `json:"targets"`
}

// AutoBeriWorldConfig holds the simple global options for the Beri (Berimond) world troop-transfer loop.
type AutoBeriWorldConfig struct {
	// MinTroopsToTransfer is the minimum surplus before troops are transferred to the Beri world (0 disables transfers).
	MinTroopsToTransfer int `json:"minTroopsToTransfer"`
	// BeriCastleCID is the Beri castle id for fuc; set in UI or filled from GCL when still zero.
	BeriCastleCID int `json:"beriCastleCID"`
	// BeriMapX and BeriMapY are the Beri world map tile from GCL (indices 1–2).
	BeriMapX int `json:"beriMapX"`
	BeriMapY int `json:"beriMapY"`
	// TransferTroopWID is the unit type sent in kut "A":[[wid,amount]].
	TransferTroopWID int `json:"transferTroopWID"`
	// KutSourceCastleSCID is the kut SCID (main castle instance id from GCL KID 0, or manual).
	KutSourceCastleSCID int `json:"kutSourceCastleSCID"`
	// KutCastleCID is the kut wire CID field (often -1).
	KutCastleCID int `json:"kutCastleCID"`
	// TroopSpaceCheckIntervalSec is how often the module runs a full check (fuc, then kut/msk when eligible).
	TroopSpaceCheckIntervalSec int `json:"troopSpaceCheckIntervalSec"`
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
	AutoTCIEnabled       bool `json:"autoTCIEnabled"`
	AutoBeriWorldEnabled bool `json:"autoBeriWorldEnabled"`

	AutoBirdDelay  AutoBirdDelayConfig `json:"autoBirdDelay"`
	BirdIgnoreList SaveInCastleTroops  `json:"birdIgnoreList"`

	// Recruit Troops Configuration
	RecruitTroopsList RecruitTroopsConfig `json:"recruitTroopsList"`

	// Auto TCI (construction items) per castle
	AutoTCIList AutoTCIConfig `json:"autoTCIList"`

	// Auto Beri World (Berimond troop transfer) global options
	AutoBeriWorld AutoBeriWorldConfig `json:"autoBeriWorld"`

	// UpgradeEreDelayMs is the pause between consecutive **ere** commands when bulk-upgrading equipment/gems.
	UpgradeEreDelayMs int `json:"upgradeEreDelayMs"`

	// UpgradeCoinThreshold blocks upgrades when GlobalResources.Coins is at or below this reserve.
	UpgradeCoinThreshold float64 `json:"upgradeCoinThreshold"`
}

// UpgradeCoinReserveThreshold returns the configured minimum coin reserve (never negative).
func (s *SettingsState) UpgradeCoinReserveThreshold() float64 {
	if s == nil || s.UpgradeCoinThreshold < 0 {
		return 0
	}
	return s.UpgradeCoinThreshold
}

// CoinsUnderUpgradeReserve reports whether the balance is at or below the configured reserve.
func (s *SettingsState) CoinsUnderUpgradeReserve(coins float64) bool {
	return coins <= s.UpgradeCoinReserveThreshold()
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
			AutoTCIEnabled:       false,
			AutoBeriWorldEnabled: false,
			AutoBirdDelay:        AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0, MinRPTDays: 3},
			BirdIgnoreList:       SaveInCastleTroops{Troops: make(map[int]map[int]int)},
			RecruitTroopsList: RecruitTroopsConfig{
				Targets: make(map[int]map[int]int),
			},
			AutoTCIList: AutoTCIConfig{
				Targets: make(map[int]map[int]AutoTCILevelTarget),
			},
			AutoBeriWorld:        DefaultAutoBeriWorldConfig(),
			UpgradeEreDelayMs:    50,
			UpgradeCoinThreshold: 0,
		}
		LoadSchedulerSettingsInto(instanceSettingsState)
		instanceSettingsState.AutoBeriWorld = ReadAutoBeriWorldConfig()
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
	s.AutoTCIEnabled = false
	s.AutoBeriWorldEnabled = false
	s.AutoBirdDelay = AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0, MinRPTDays: 3}
	s.BirdIgnoreList.Troops = make(map[int]map[int]int)
	s.RecruitTroopsList.Targets = make(map[int]map[int]int)
	s.AutoTCIList.Targets = make(map[int]map[int]AutoTCILevelTarget)
	s.AutoBeriWorld = DefaultAutoBeriWorldConfig()
	s.UpgradeEreDelayMs = 50
	s.UpgradeCoinThreshold = 0
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

// UpdateAutoTCIList updates in-memory Auto TCI targets from the UI and persists to AutoTCI.json.
func (s *SettingsState) UpdateAutoTCIList(data map[int]map[int]AutoTCILevelTarget) {
	s.AutoTCIList.Targets = data
	if err := WriteAutoTCITargetsOnly(data); err != nil {
		Logging.AutoTCILogf("settings", "disk write failed: %v", err)
		return
	}
	Logging.AutoTCILogf("settings", "saved %d castles to AutoTCI.json", len(s.AutoTCIList.Targets))
}

// ClearAutoTCIList clears Auto TCI targets from memory.
func (s *SettingsState) ClearAutoTCIList() {
	s.AutoTCIList.Targets = nil
	Logging.AutoTCILog("settings", "Cleared from memory")
}

// UpdateAutoBeriWorldConfig updates in-memory Auto Beri World options from the UI and persists to AutoBeriWorld.json.
func (s *SettingsState) UpdateAutoBeriWorldConfig(cfg AutoBeriWorldConfig) {
	cfg = sanitizeAutoBeriWorldConfig(cfg)
	s.AutoBeriWorld = cfg
	if err := WriteAutoBeriWorldConfig(cfg); err != nil {
		Logging.AutoBeriWorldLogf("settings", "disk write failed: %v", err)
		return
	}
	Logging.AutoBeriWorldLogf("settings", "saved (check=%ds, minTroops=%d)", cfg.TroopSpaceCheckIntervalSec, cfg.MinTroopsToTransfer)
}
