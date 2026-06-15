package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const schedulerSettingsFileName = "SchedulerSettings.json"

var schedulerSettingsMu sync.Mutex

type schedulerSettingsFile struct {
	Version              int               `json:"version"`
	MinAttackDelay       float64           `json:"minAttackDelay"`
	MaxAttackDelay       float64           `json:"maxAttackDelay"`
	UpgradeEreDelayMs    int               `json:"upgradeEreDelayMs"`
	UpgradeCoinThreshold float64           `json:"upgradeCoinThreshold"`
	TabPriorities        map[string]string `json:"tabPriorities"`
}

func schedulerSettingsPath() string {
	return filepath.Join(Paths.DataDir(), schedulerSettingsFileName)
}

func defaultSchedulerSettingsFile() schedulerSettingsFile {
	return schedulerSettingsFile{
		Version:              1,
		MinAttackDelay:       4.0,
		MaxAttackDelay:       6.0,
		UpgradeEreDelayMs:    50,
		UpgradeCoinThreshold: 0,
		TabPriorities:        map[string]string{},
	}
}

func normalizeSchedulerSettings(s *SettingsState) {
	if s.MinAttackDelay < 4.0 {
		s.MinAttackDelay = 4.0
	}
	if s.MaxAttackDelay < s.MinAttackDelay {
		s.MaxAttackDelay = s.MinAttackDelay
	}
	if s.UpgradeEreDelayMs <= 0 {
		s.UpgradeEreDelayMs = 50
	}
	if s.UpgradeEreDelayMs < 10 {
		s.UpgradeEreDelayMs = 10
	}
	if s.UpgradeEreDelayMs > 5000 {
		s.UpgradeEreDelayMs = 5000
	}
	if s.UpgradeCoinThreshold < 0 {
		s.UpgradeCoinThreshold = 0
	}
	if s.TabPriorities == nil {
		s.TabPriorities = make(map[string]TabPriority)
	}
}

// LoadSchedulerSettingsInto merges persisted SettingsView values from SchedulerSettings.json.
func LoadSchedulerSettingsInto(s *SettingsState) {
	if s == nil {
		return
	}
	schedulerSettingsMu.Lock()
	defer schedulerSettingsMu.Unlock()

	data, err := os.ReadFile(schedulerSettingsPath())
	if err != nil || len(data) == 0 {
		if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
			if leg, err := os.ReadFile(filepath.Join(d, schedulerSettingsFileName)); err == nil && len(leg) > 0 {
				data = leg
			}
		}
	}
	if len(data) == 0 {
		return
	}

	var f schedulerSettingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return
	}

	if f.MinAttackDelay > 0 {
		s.MinAttackDelay = f.MinAttackDelay
	}
	if f.MaxAttackDelay > 0 {
		s.MaxAttackDelay = f.MaxAttackDelay
	}
	if f.MinAttackDelay > 0 || f.MaxAttackDelay > 0 {
		MarkSchedulerAttackDelaysPersisted()
	}
	if f.UpgradeEreDelayMs > 0 {
		s.UpgradeEreDelayMs = f.UpgradeEreDelayMs
	}
	s.UpgradeCoinThreshold = f.UpgradeCoinThreshold
	if f.TabPriorities != nil {
		if s.TabPriorities == nil {
			s.TabPriorities = make(map[string]TabPriority, len(f.TabPriorities))
		}
		for tabID, p := range f.TabPriorities {
			s.TabPriorities[tabID] = TabPriority(p)
		}
	}
	normalizeSchedulerSettings(s)
}

// PersistSchedulerSettings writes SettingsView scheduler fields to SchedulerSettings.json.
func PersistSchedulerSettings(s *SettingsState) error {
	if s == nil {
		return nil
	}
	normalizeSchedulerSettings(s)

	priorities := make(map[string]string, len(s.TabPriorities))
	for tabID, p := range s.TabPriorities {
		priorities[tabID] = string(p)
	}

	f := schedulerSettingsFile{
		Version:              1,
		MinAttackDelay:       s.MinAttackDelay,
		MaxAttackDelay:       s.MaxAttackDelay,
		UpgradeEreDelayMs:    s.UpgradeEreDelayMs,
		UpgradeCoinThreshold: s.UpgradeCoinThreshold,
		TabPriorities:        priorities,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	schedulerSettingsMu.Lock()
	defer schedulerSettingsMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := schedulerSettingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	MarkSchedulerAttackDelaysPersisted()
	return nil
}
