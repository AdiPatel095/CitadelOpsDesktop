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

const (
	RecruitTroopsModeGlobal    = "global"
	RecruitTroopsModePerCastle = "perCastle"

	DefaultRecruitTroopsCheckIntervalSec = 300
	MinRecruitTroopsCheckIntervalSec     = 30
	MaxRecruitTroopsCheckIntervalSec     = 86400

	AutoToolModeGlobal    = "global"
	AutoToolModePerCastle = "perCastle"

	DefaultAutoToolCheckIntervalSec = 300
	MinAutoToolCheckIntervalSec     = 30
	MaxAutoToolCheckIntervalSec     = 86400
)

// RecruitTroopsConfig represents the Auto Recruit unit selections.
// In global mode, GlobalTargets is the shared unit map for every enabled castle.
// In perCastle mode, Targets holds castle-specific unit maps.
type RecruitTroopsConfig struct {
	Mode             string              `json:"mode"`
	CheckIntervalSec int                 `json:"checkIntervalSec"`
	GlobalTargets    map[int]int         `json:"globalTargets,omitempty"`
	EnabledCastles   map[int]bool        `json:"enabledCastles,omitempty"`
	Targets          map[int]map[int]int `json:"targets"`
}

func clampRecruitTroopsCheckIntervalSec(value int) int {
	if value <= 0 {
		return DefaultRecruitTroopsCheckIntervalSec
	}
	if value < MinRecruitTroopsCheckIntervalSec {
		return MinRecruitTroopsCheckIntervalSec
	}
	if value > MaxRecruitTroopsCheckIntervalSec {
		return MaxRecruitTroopsCheckIntervalSec
	}
	return value
}

func sanitizeRecruitTroopsFlatMap(input map[int]int) map[int]int {
	output := make(map[int]int)
	for unitID, amount := range input {
		if unitID <= 0 || amount < 0 {
			continue
		}
		output[unitID] = amount
	}
	return output
}

func sanitizeRecruitTroopsNestedMap(input map[int]map[int]int) map[int]map[int]int {
	output := make(map[int]map[int]int)
	for castleID, targets := range input {
		if castleID <= 0 {
			continue
		}
		output[castleID] = sanitizeRecruitTroopsFlatMap(targets)
	}
	return output
}

func copyRecruitTroopsTargets(input map[int]int) map[int]int {
	output := make(map[int]int, len(input))
	for unitID, amount := range input {
		output[unitID] = amount
	}
	return output
}

// DefaultRecruitTroopsConfig returns the default Auto Recruit settings.
func DefaultRecruitTroopsConfig() RecruitTroopsConfig {
	return RecruitTroopsConfig{
		Mode:             RecruitTroopsModeGlobal,
		CheckIntervalSec: DefaultRecruitTroopsCheckIntervalSec,
		GlobalTargets:    make(map[int]int),
		EnabledCastles:   make(map[int]bool),
		Targets:          make(map[int]map[int]int),
	}
}

// Normalize applies defaults, clamps intervals, and preserves legacy target-only configs.
func (c RecruitTroopsConfig) Normalize() RecruitTroopsConfig {
	cfg := DefaultRecruitTroopsConfig()

	cfg.Targets = sanitizeRecruitTroopsNestedMap(c.Targets)
	cfg.GlobalTargets = sanitizeRecruitTroopsFlatMap(c.GlobalTargets)
	cfg.CheckIntervalSec = clampRecruitTroopsCheckIntervalSec(c.CheckIntervalSec)

	mode := c.Mode
	if mode == "" && len(cfg.Targets) > 0 && len(cfg.GlobalTargets) == 0 {
		mode = RecruitTroopsModePerCastle
	}
	if mode != RecruitTroopsModePerCastle {
		mode = RecruitTroopsModeGlobal
	}
	cfg.Mode = mode

	if c.EnabledCastles != nil {
		for castleID, enabled := range c.EnabledCastles {
			if castleID > 0 {
				cfg.EnabledCastles[castleID] = enabled
			}
		}
	} else {
		for castleID, targets := range cfg.Targets {
			if len(targets) > 0 {
				cfg.EnabledCastles[castleID] = true
			}
		}
	}

	return cfg
}

// EffectiveTargets returns the per-castle unit maps after applying mode and enable switches.
func (c RecruitTroopsConfig) EffectiveTargets() map[int]map[int]int {
	cfg := c.Normalize()
	output := make(map[int]map[int]int)

	if cfg.Mode == RecruitTroopsModeGlobal {
		if len(cfg.GlobalTargets) == 0 {
			return output
		}
		for castleID, enabled := range cfg.EnabledCastles {
			if enabled {
				output[castleID] = copyRecruitTroopsTargets(cfg.GlobalTargets)
			}
		}
		return output
	}

	for castleID, targets := range cfg.Targets {
		if len(targets) == 0 || !cfg.EnabledCastles[castleID] {
			continue
		}
		output[castleID] = copyRecruitTroopsTargets(targets)
	}
	return output
}

// AutoToolConfig represents the Auto Tool selections.
// In global mode, GlobalTargets is the shared tool map for every enabled castle.
// In perCastle mode, Targets holds castle-specific tool maps.
type AutoToolConfig struct {
	Mode             string              `json:"mode"`
	CheckIntervalSec int                 `json:"checkIntervalSec"`
	GlobalTargets    map[int]int         `json:"globalTargets,omitempty"`
	EnabledCastles   map[int]bool        `json:"enabledCastles,omitempty"`
	Targets          map[int]map[int]int `json:"targets"`
}

func clampAutoToolCheckIntervalSec(value int) int {
	if value <= 0 {
		return DefaultAutoToolCheckIntervalSec
	}
	if value < MinAutoToolCheckIntervalSec {
		return MinAutoToolCheckIntervalSec
	}
	if value > MaxAutoToolCheckIntervalSec {
		return MaxAutoToolCheckIntervalSec
	}
	return value
}

// DefaultAutoToolConfig returns the default Auto Tool settings.
func DefaultAutoToolConfig() AutoToolConfig {
	return AutoToolConfig{
		Mode:             AutoToolModeGlobal,
		CheckIntervalSec: DefaultAutoToolCheckIntervalSec,
		GlobalTargets:    make(map[int]int),
		EnabledCastles:   make(map[int]bool),
		Targets:          make(map[int]map[int]int),
	}
}

// Normalize applies defaults, clamps intervals, and preserves legacy target-only configs.
func (c AutoToolConfig) Normalize() AutoToolConfig {
	cfg := DefaultAutoToolConfig()

	cfg.Targets = sanitizeRecruitTroopsNestedMap(c.Targets)
	cfg.GlobalTargets = sanitizeRecruitTroopsFlatMap(c.GlobalTargets)
	cfg.CheckIntervalSec = clampAutoToolCheckIntervalSec(c.CheckIntervalSec)

	mode := c.Mode
	if mode == "" && len(cfg.Targets) > 0 && len(cfg.GlobalTargets) == 0 {
		mode = AutoToolModePerCastle
	}
	if mode != AutoToolModePerCastle {
		mode = AutoToolModeGlobal
	}
	cfg.Mode = mode

	if c.EnabledCastles != nil {
		for castleID, enabled := range c.EnabledCastles {
			if castleID > 0 {
				cfg.EnabledCastles[castleID] = enabled
			}
		}
	} else {
		for castleID, targets := range cfg.Targets {
			if len(targets) > 0 {
				cfg.EnabledCastles[castleID] = true
			}
		}
	}

	return cfg
}

// EffectiveTargets returns the per-castle tool maps after applying mode and enable switches.
func (c AutoToolConfig) EffectiveTargets() map[int]map[int]int {
	cfg := c.Normalize()
	output := make(map[int]map[int]int)

	if cfg.Mode == AutoToolModeGlobal {
		if len(cfg.GlobalTargets) == 0 {
			return output
		}
		for castleID, enabled := range cfg.EnabledCastles {
			if enabled {
				output[castleID] = copyRecruitTroopsTargets(cfg.GlobalTargets)
			}
		}
		return output
	}

	for castleID, targets := range cfg.Targets {
		if len(targets) == 0 || !cfg.EnabledCastles[castleID] {
			continue
		}
		output[castleID] = copyRecruitTroopsTargets(targets)
	}
	return output
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

// GetTargetAmount returns the legacy stored value for a selected unit.
func (c *RecruitTroopsConfig) GetTargetAmount(castleID, unitID int) (int, bool) {
	if c == nil {
		return 0, false
	}

	if castleConfig, ok := c.EffectiveTargets()[castleID]; ok {
		if amount, ok := castleConfig[unitID]; ok {
			return amount, true
		}
	}

	return 0, false
}

// GetTargetAmount returns the legacy stored value for a selected tool.
func (c *AutoToolConfig) GetTargetAmount(castleID, toolID int) (int, bool) {
	if c == nil {
		return 0, false
	}

	if castleConfig, ok := c.EffectiveTargets()[castleID]; ok {
		if amount, ok := castleConfig[toolID]; ok {
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

	// FeatureSchedules optionally limits when automation features are allowed to run.
	FeatureSchedules map[string]FeatureSchedule `json:"featureSchedules"`

	// Global Connection/Feature Flags
	BotEnabled           bool `json:"botEnabled"`
	AutoBirdEnabled      bool `json:"autoBirdEnabled"`
	RecruitTroopsEnabled bool `json:"recruitTroopsEnabled"`
	AutoToolEnabled      bool `json:"autoToolEnabled"`
	AutoTCIEnabled       bool `json:"autoTCIEnabled"`
	AutoBeriWorldEnabled bool `json:"autoBeriWorldEnabled"`

	AutoBirdDelay  AutoBirdDelayConfig `json:"autoBirdDelay"`
	BirdIgnoreList SaveInCastleTroops  `json:"birdIgnoreList"`

	// Recruit Troops Configuration
	RecruitTroopsList RecruitTroopsConfig `json:"recruitTroopsList"`

	// Auto Tool Configuration
	AutoToolList AutoToolConfig `json:"autoToolList"`

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
			FeatureSchedules:     make(map[string]FeatureSchedule),
			BotEnabled:           false,
			AutoBirdEnabled:      false,
			RecruitTroopsEnabled: false,
			AutoToolEnabled:      false,
			AutoTCIEnabled:       false,
			AutoBeriWorldEnabled: false,
			AutoBirdDelay:        AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0, MinRPTDays: 3},
			BirdIgnoreList:       SaveInCastleTroops{Troops: make(map[int]map[int]int)},
			RecruitTroopsList:    DefaultRecruitTroopsConfig(),
			AutoToolList:         DefaultAutoToolConfig(),
			AutoTCIList: AutoTCIConfig{
				Targets: make(map[int]map[int]AutoTCILevelTarget),
			},
			AutoBeriWorld:        DefaultAutoBeriWorldConfig(),
			UpgradeEreDelayMs:    50,
			UpgradeCoinThreshold: 0,
		}
		LoadSchedulerSettingsInto(instanceSettingsState)
		instanceSettingsState.RecruitTroopsList = ReadRecruitTroopsConfig()
		instanceSettingsState.AutoToolList = ReadAutoToolConfig()
		instanceSettingsState.AutoBeriWorld = ReadAutoBeriWorldConfig()
	})
	return instanceSettingsState
}

// Reset clears the settings state back to default values.
func (s *SettingsState) Reset() {
	s.MinAttackDelay = 4.0
	s.MaxAttackDelay = 6.0
	s.TabPriorities = make(map[string]TabPriority)
	s.FeatureSchedules = make(map[string]FeatureSchedule)
	s.BotEnabled = false
	s.AutoBirdEnabled = false
	s.RecruitTroopsEnabled = false
	s.AutoToolEnabled = false
	s.AutoTCIEnabled = false
	s.AutoBeriWorldEnabled = false
	s.AutoBirdDelay = AutoBirdDelayConfig{MinDelay: 6, MaxDelay: 12, MinSend: 0, MinRPTDays: 3}
	s.BirdIgnoreList.Troops = make(map[int]map[int]int)
	s.RecruitTroopsList = DefaultRecruitTroopsConfig()
	s.AutoToolList = DefaultAutoToolConfig()
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
	cfg := s.RecruitTroopsList.Normalize()
	cfg.Mode = RecruitTroopsModePerCastle
	cfg.Targets = sanitizeRecruitTroopsNestedMap(data)
	cfg.EnabledCastles = make(map[int]bool)
	for castleID, targets := range cfg.Targets {
		if len(targets) > 0 {
			cfg.EnabledCastles[castleID] = true
		}
	}
	s.RecruitTroopsList = cfg.Normalize()
	log.Printf("[RecruitTroopsList] Updated in-memory legacy config for %d castles", len(s.RecruitTroopsList.Targets))
	Logging.AutoRecruitLogf("settings", "updated legacy targets castles=%d", len(s.RecruitTroopsList.Targets))
}

// UpdateRecruitTroopsConfig updates the in-memory recruit troops config from the UI.
func (s *SettingsState) UpdateRecruitTroopsConfig(cfg RecruitTroopsConfig) {
	s.RecruitTroopsList = cfg.Normalize()
	log.Printf("[RecruitTroopsList] Updated mode=%s enabled=%d interval=%ds",
		s.RecruitTroopsList.Mode,
		len(s.RecruitTroopsList.EnabledCastles),
		s.RecruitTroopsList.CheckIntervalSec,
	)
	Logging.AutoRecruitLogf("settings", "updated mode=%s enabled=%d interval=%ds",
		s.RecruitTroopsList.Mode,
		len(s.RecruitTroopsList.EnabledCastles),
		s.RecruitTroopsList.CheckIntervalSec,
	)
}

// ClearRecruitTroopsList clears the RecruitTroopsList from memory
func (s *SettingsState) ClearRecruitTroopsList() {
	s.RecruitTroopsList = DefaultRecruitTroopsConfig()
	log.Println("[RecruitTroopsList] Cleared from memory")
}

// UpdateAutoToolList updates the in-memory Auto Tool list from the given map.
func (s *SettingsState) UpdateAutoToolList(data map[int]map[int]int) {
	cfg := s.AutoToolList.Normalize()
	cfg.Mode = AutoToolModePerCastle
	cfg.Targets = sanitizeRecruitTroopsNestedMap(data)
	cfg.EnabledCastles = make(map[int]bool)
	for castleID, targets := range cfg.Targets {
		if len(targets) > 0 {
			cfg.EnabledCastles[castleID] = true
		}
	}
	s.AutoToolList = cfg.Normalize()
	log.Printf("[AutoToolList] Updated in-memory legacy config for %d castles", len(s.AutoToolList.Targets))
	Logging.AutoToolLogf("settings", "updated legacy targets castles=%d", len(s.AutoToolList.Targets))
}

// UpdateAutoToolConfig updates the in-memory Auto Tool config from the UI.
func (s *SettingsState) UpdateAutoToolConfig(cfg AutoToolConfig) {
	s.AutoToolList = cfg.Normalize()
	log.Printf("[AutoToolList] Updated mode=%s enabled=%d interval=%ds",
		s.AutoToolList.Mode,
		len(s.AutoToolList.EnabledCastles),
		s.AutoToolList.CheckIntervalSec,
	)
	Logging.AutoToolLogf("settings", "updated mode=%s enabled=%d interval=%ds",
		s.AutoToolList.Mode,
		len(s.AutoToolList.EnabledCastles),
		s.AutoToolList.CheckIntervalSec,
	)
}

// ClearAutoToolList clears the AutoToolList from memory.
func (s *SettingsState) ClearAutoToolList() {
	s.AutoToolList = DefaultAutoToolConfig()
	log.Println("[AutoToolList] Cleared from memory")
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
