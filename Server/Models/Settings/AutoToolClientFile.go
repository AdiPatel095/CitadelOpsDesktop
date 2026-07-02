package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// Auto Tool settings live next to AutoBird.json / AutoTCI.json under paths.DataDir().
const autoToolClientFileName = "AutoTool.json"

var autoToolClientMu sync.Mutex

type autoToolClientFile struct {
	Version          int                 `json:"version"`
	Mode             string              `json:"mode"`
	CheckIntervalSec int                 `json:"checkIntervalSec"`
	GlobalTargets    map[int]int         `json:"globalTargets,omitempty"`
	EnabledCastles   map[int]bool        `json:"enabledCastles,omitempty"`
	Targets          map[int]map[int]int `json:"targets"`
}

func AutoToolSettingsPath() string {
	return filepath.Join(Paths.DataDir(), autoToolClientFileName)
}

// ReadAutoToolConfig loads persisted Auto Tool settings from AutoTool.json.
func ReadAutoToolConfig() AutoToolConfig {
	autoToolClientMu.Lock()
	defer autoToolClientMu.Unlock()

	data, err := os.ReadFile(AutoToolSettingsPath())
	if err != nil || len(data) == 0 {
		if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
			if leg, err := os.ReadFile(filepath.Join(d, autoToolClientFileName)); err == nil && len(leg) > 0 {
				data = leg
			}
		}
	}
	if len(data) == 0 {
		return DefaultAutoToolConfig()
	}

	var f autoToolClientFile
	if err := json.Unmarshal(data, &f); err != nil {
		return DefaultAutoToolConfig()
	}

	return AutoToolConfig{
		Mode:             f.Mode,
		CheckIntervalSec: f.CheckIntervalSec,
		GlobalTargets:    f.GlobalTargets,
		EnabledCastles:   f.EnabledCastles,
		Targets:          f.Targets,
	}.Normalize()
}

// WriteAutoToolConfig atomically writes AutoTool.json.
func WriteAutoToolConfig(cfg AutoToolConfig) error {
	cfg = cfg.Normalize()
	f := autoToolClientFile{
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

	autoToolClientMu.Lock()
	defer autoToolClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := AutoToolSettingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
