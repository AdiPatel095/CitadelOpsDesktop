package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// Auto Beri World options live next to AutoTCI.json / AutoBird.json under paths.DataDir().
const autoBeriWorldClientFileName = "AutoBeriWorld.json"

const (
	defaultAutoBeriKutCastleCID       = -1
	defaultAutoBeriTroopSpaceCheckSec = 30
	minAutoBeriTroopSpaceCheckSec     = 5
	maxAutoBeriTroopSpaceCheckSec     = 3600
)

var autoBeriWorldClientMu sync.Mutex

type autoBeriWorldClientFile struct {
	Version                    int `json:"version"`
	MinTroopsToTransfer        int `json:"minTroopsToTransfer"`
	BeriCastleCID              int `json:"beriCastleCID"`
	BeriMapX                   int `json:"beriMapX"`
	BeriMapY                   int `json:"beriMapY"`
	TransferTroopWID           int `json:"transferTroopWID"`
	KutSourceCastleSCID        int `json:"kutSourceCastleSCID"`
	KutCastleCID               int `json:"kutCastleCID"`
	TroopSpaceCheckIntervalSec int `json:"troopSpaceCheckIntervalSec"`
}

func autoBeriWorldClientPath() string {
	return filepath.Join(Paths.DataDir(), autoBeriWorldClientFileName)
}

// DefaultAutoBeriWorldConfig returns the built-in defaults for the Beri world loop.
func DefaultAutoBeriWorldConfig() AutoBeriWorldConfig {
	return AutoBeriWorldConfig{
		TroopSpaceCheckIntervalSec: defaultAutoBeriTroopSpaceCheckSec,
		MinTroopsToTransfer:        0,
	}
}

// sanitizeAutoBeriWorldConfig clamps options to sane ranges.
func sanitizeAutoBeriWorldConfig(cfg AutoBeriWorldConfig) AutoBeriWorldConfig {
	if cfg.MinTroopsToTransfer < 0 {
		cfg.MinTroopsToTransfer = 0
	}
	if cfg.BeriCastleCID < 0 {
		cfg.BeriCastleCID = 0
	}
	if cfg.BeriMapX < 0 {
		cfg.BeriMapX = 0
	}
	if cfg.BeriMapY < 0 {
		cfg.BeriMapY = 0
	}
	if cfg.TransferTroopWID < 0 {
		cfg.TransferTroopWID = 0
	}
	if cfg.KutSourceCastleSCID < 0 {
		cfg.KutSourceCastleSCID = 0
	}
	if cfg.KutCastleCID == 0 {
		cfg.KutCastleCID = defaultAutoBeriKutCastleCID
	}
	if cfg.TroopSpaceCheckIntervalSec < minAutoBeriTroopSpaceCheckSec {
		cfg.TroopSpaceCheckIntervalSec = defaultAutoBeriTroopSpaceCheckSec
	}
	if cfg.TroopSpaceCheckIntervalSec > maxAutoBeriTroopSpaceCheckSec {
		cfg.TroopSpaceCheckIntervalSec = maxAutoBeriTroopSpaceCheckSec
	}
	return cfg
}

// ReadAutoBeriWorldConfig loads persisted options from AutoBeriWorld.json (defaults if missing).
func ReadAutoBeriWorldConfig() AutoBeriWorldConfig {
	autoBeriWorldClientMu.Lock()
	defer autoBeriWorldClientMu.Unlock()

	path := autoBeriWorldClientPath()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
			if leg, err := os.ReadFile(filepath.Join(d, autoBeriWorldClientFileName)); err == nil && len(leg) > 0 {
				data = leg
			}
		}
	}
	if len(data) == 0 {
		return DefaultAutoBeriWorldConfig()
	}
	var f autoBeriWorldClientFile
	if err := json.Unmarshal(data, &f); err != nil {
		return DefaultAutoBeriWorldConfig()
	}
	return sanitizeAutoBeriWorldConfig(AutoBeriWorldConfig{
		MinTroopsToTransfer:        f.MinTroopsToTransfer,
		BeriCastleCID:              f.BeriCastleCID,
		BeriMapX:                   f.BeriMapX,
		BeriMapY:                   f.BeriMapY,
		TransferTroopWID:           f.TransferTroopWID,
		KutSourceCastleSCID:        f.KutSourceCastleSCID,
		KutCastleCID:               f.KutCastleCID,
		TroopSpaceCheckIntervalSec: f.TroopSpaceCheckIntervalSec,
	})
}

// WriteAutoBeriWorldConfig atomically writes AutoBeriWorld.json.
func WriteAutoBeriWorldConfig(cfg AutoBeriWorldConfig) error {
	cfg = sanitizeAutoBeriWorldConfig(cfg)
	f := autoBeriWorldClientFile{
		Version:                    1,
		MinTroopsToTransfer:        cfg.MinTroopsToTransfer,
		BeriCastleCID:              cfg.BeriCastleCID,
		BeriMapX:                   cfg.BeriMapX,
		BeriMapY:                   cfg.BeriMapY,
		TransferTroopWID:           cfg.TransferTroopWID,
		KutSourceCastleSCID:        cfg.KutSourceCastleSCID,
		KutCastleCID:               cfg.KutCastleCID,
		TroopSpaceCheckIntervalSec: cfg.TroopSpaceCheckIntervalSec,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	autoBeriWorldClientMu.Lock()
	defer autoBeriWorldClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := autoBeriWorldClientPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
