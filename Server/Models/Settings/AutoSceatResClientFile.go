package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const autoSceatResClientFileName = "AutoSceatRes.json"

var autoSceatResClientMu sync.Mutex

type autoSceatResClientFile struct {
	Version int `json:"version"`
	AutoSceatResConfig
}

func AutoSceatResSettingsPath() string {
	return filepath.Join(Paths.DataDir(), autoSceatResClientFileName)
}

// ReadAutoSceatResConfig loads persisted Auto Sceat Resources settings.
func ReadAutoSceatResConfig() AutoSceatResConfig {
	autoSceatResClientMu.Lock()
	defer autoSceatResClientMu.Unlock()

	data, err := os.ReadFile(AutoSceatResSettingsPath())
	if err != nil || len(data) == 0 {
		if legacyDir := Paths.LegacyDotCitadelOpsDir(); legacyDir != "" {
			data, _ = os.ReadFile(filepath.Join(legacyDir, autoSceatResClientFileName))
		}
	}
	if len(data) == 0 {
		return DefaultAutoSceatResConfig()
	}

	var file autoSceatResClientFile
	if err := json.Unmarshal(data, &file); err != nil {
		return DefaultAutoSceatResConfig()
	}
	return file.AutoSceatResConfig.Normalize()
}

// WriteAutoSceatResConfig atomically writes AutoSceatRes.json.
func WriteAutoSceatResConfig(cfg AutoSceatResConfig) error {
	file := autoSceatResClientFile{Version: 1, AutoSceatResConfig: cfg.Normalize()}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	autoSceatResClientMu.Lock()
	defer autoSceatResClientMu.Unlock()
	if err := os.MkdirAll(Paths.DataDir(), 0755); err != nil {
		return err
	}
	path := AutoSceatResSettingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
