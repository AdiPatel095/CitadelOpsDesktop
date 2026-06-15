package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

const (
	riftMaidenCommsFileName      = "RiftMaidenComms.json"
	defaultRiftMaidenProbeUnitID = 216
)

var riftMaidenCommsMu sync.Mutex

type riftMaidenCommsFile struct {
	Version   int `json:"version"`
	UnitWodID int `json:"unitWodID"`
}

// RiftMaidenCommsSettings is the persisted maiden-comms probe unit selection.
type RiftMaidenCommsSettings struct {
	UnitWodID int `json:"unitWodID"`
}

func riftMaidenCommsPath() string {
	return filepath.Join(Paths.DataDir(), riftMaidenCommsFileName)
}

func sanitizeRiftMaidenCommsSettings(s RiftMaidenCommsSettings) RiftMaidenCommsSettings {
	if s.UnitWodID <= 0 {
		s.UnitWodID = defaultRiftMaidenProbeUnitID
	}
	return s
}

// ReadRiftMaidenCommsSettings loads RiftMaidenComms.json (defaults if missing).
func ReadRiftMaidenCommsSettings() RiftMaidenCommsSettings {
	riftMaidenCommsMu.Lock()
	defer riftMaidenCommsMu.Unlock()

	data, err := os.ReadFile(riftMaidenCommsPath())
	if err != nil || len(data) == 0 {
		return sanitizeRiftMaidenCommsSettings(RiftMaidenCommsSettings{})
	}
	var f riftMaidenCommsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return sanitizeRiftMaidenCommsSettings(RiftMaidenCommsSettings{})
	}
	return sanitizeRiftMaidenCommsSettings(RiftMaidenCommsSettings{UnitWodID: f.UnitWodID})
}

// WriteRiftMaidenCommsSettings atomically writes RiftMaidenComms.json.
func WriteRiftMaidenCommsSettings(s RiftMaidenCommsSettings) error {
	s = sanitizeRiftMaidenCommsSettings(s)
	f := riftMaidenCommsFile{
		Version:   1,
		UnitWodID: s.UnitWodID,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	riftMaidenCommsMu.Lock()
	defer riftMaidenCommsMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := riftMaidenCommsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
