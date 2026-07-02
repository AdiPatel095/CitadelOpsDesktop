package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const autoHospitalClientFileName = "AutoHospital.json"

var autoHospitalClientMu sync.Mutex

type autoHospitalClientFile struct {
	Version          int `json:"version"`
	CheckIntervalSec int `json:"checkIntervalSec"`
}

func AutoHospitalSettingsPath() string {
	return filepath.Join(Paths.DataDir(), autoHospitalClientFileName)
}

// ReadAutoHospitalConfig loads persisted Auto Hospital settings from AutoHospital.json.
func ReadAutoHospitalConfig() AutoHospitalConfig {
	autoHospitalClientMu.Lock()
	defer autoHospitalClientMu.Unlock()

	data, err := os.ReadFile(AutoHospitalSettingsPath())
	if err != nil || len(data) == 0 {
		if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
			if leg, err := os.ReadFile(filepath.Join(d, autoHospitalClientFileName)); err == nil && len(leg) > 0 {
				data = leg
			}
		}
	}
	if len(data) == 0 {
		return DefaultAutoHospitalConfig()
	}

	var f autoHospitalClientFile
	if err := json.Unmarshal(data, &f); err != nil {
		return DefaultAutoHospitalConfig()
	}

	return AutoHospitalConfig{
		CheckIntervalSec: f.CheckIntervalSec,
	}.Normalize()
}

// WriteAutoHospitalConfig atomically writes AutoHospital.json.
func WriteAutoHospitalConfig(cfg AutoHospitalConfig) error {
	cfg = cfg.Normalize()
	f := autoHospitalClientFile{
		Version:          1,
		CheckIntervalSec: cfg.CheckIntervalSec,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	autoHospitalClientMu.Lock()
	defer autoHospitalClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := AutoHospitalSettingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
