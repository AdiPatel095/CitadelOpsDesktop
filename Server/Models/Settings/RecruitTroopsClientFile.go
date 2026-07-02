package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// Auto Recruit settings live next to AutoBird.json / AutoTCI.json under paths.DataDir().
const recruitTroopsClientFileName = "RecruitTroops.json"

var recruitTroopsClientMu sync.Mutex

type recruitTroopsClientFile struct {
	Version          int                 `json:"version"`
	Mode             string              `json:"mode"`
	CheckIntervalSec int                 `json:"checkIntervalSec"`
	GlobalTargets    map[int]int         `json:"globalTargets,omitempty"`
	EnabledCastles   map[int]bool        `json:"enabledCastles,omitempty"`
	Targets          map[int]map[int]int `json:"targets"`
}

func RecruitTroopsSettingsPath() string {
	return filepath.Join(Paths.DataDir(), recruitTroopsClientFileName)
}

// ReadRecruitTroopsConfig loads persisted Auto Recruit settings from RecruitTroops.json.
func ReadRecruitTroopsConfig() RecruitTroopsConfig {
	recruitTroopsClientMu.Lock()
	defer recruitTroopsClientMu.Unlock()

	data, err := os.ReadFile(RecruitTroopsSettingsPath())
	if err != nil || len(data) == 0 {
		if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
			if leg, err := os.ReadFile(filepath.Join(d, recruitTroopsClientFileName)); err == nil && len(leg) > 0 {
				data = leg
			}
		}
	}
	if len(data) == 0 {
		return DefaultRecruitTroopsConfig()
	}

	var f recruitTroopsClientFile
	if err := json.Unmarshal(data, &f); err != nil {
		return DefaultRecruitTroopsConfig()
	}

	return RecruitTroopsConfig{
		Mode:             f.Mode,
		CheckIntervalSec: f.CheckIntervalSec,
		GlobalTargets:    f.GlobalTargets,
		EnabledCastles:   f.EnabledCastles,
		Targets:          f.Targets,
	}.Normalize()
}

// WriteRecruitTroopsConfig atomically writes RecruitTroops.json.
func WriteRecruitTroopsConfig(cfg RecruitTroopsConfig) error {
	cfg = cfg.Normalize()
	f := recruitTroopsClientFile{
		Version:          1,
		Mode:             cfg.Mode,
		CheckIntervalSec: cfg.CheckIntervalSec,
		GlobalTargets:    cfg.GlobalTargets,
		EnabledCastles:   cfg.EnabledCastles,
		Targets:          cfg.Targets,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	recruitTroopsClientMu.Lock()
	defer recruitTroopsClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := RecruitTroopsSettingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
