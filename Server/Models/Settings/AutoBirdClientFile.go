package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// Auto Bird UI state (ignore list + delay + named presets) lives next to DecorationPresets.json
// under paths.DataDir() — same durability as decoration presets.
const autoBirdClientFileName = "AutoBird.json"

var (
	autoBirdClientMu sync.Mutex
	// defaultAutoBirdJSON matches Client autoBirdClientState.ts defaults.
	defaultAutoBirdJSON = []byte(`{"version":1,"ignoreSettings":{"settings":{},"minDelay":6,"maxDelay":12,"minSend":0,"minRPTDays":3},"presets":{"version":1,"lastSelectedPresetId":null,"presets":[]}}`)
)

func autoBirdClientPath() string {
	return filepath.Join(Paths.DataDir(), autoBirdClientFileName)
}

func legacyExeDirPresetsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "autobird_presets.json"
	}
	return filepath.Join(filepath.Dir(exe), "autobird_presets.json")
}

func mergeLegacyPresetsIntoDefault(legacyPresetsJSON []byte) []byte {
	var presets interface{}
	if json.Unmarshal(legacyPresetsJSON, &presets) != nil {
		return nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal(defaultAutoBirdJSON, &root); err != nil {
		return nil
	}
	root["presets"] = presets
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil
	}
	return append(out, '\n')
}

// ReadAutoBirdClientFile returns JSON for the unified Auto Bird file (defaults or migrated legacy).
func ReadAutoBirdClientFile() []byte {
	autoBirdClientMu.Lock()
	defer autoBirdClientMu.Unlock()
	path := autoBirdClientPath()
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		return data
	}
	if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
		if leg, err := os.ReadFile(filepath.Join(d, autoBirdClientFileName)); err == nil && len(leg) > 0 {
			return leg
		}
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if leg, err := os.ReadFile(filepath.Join(filepath.Dir(exe), autoBirdClientFileName)); err == nil && len(leg) > 0 {
			return leg
		}
	}
	if leg, err := os.ReadFile(legacyExeDirPresetsPath()); err == nil && len(leg) > 0 {
		if merged := mergeLegacyPresetsIntoDefault(leg); merged != nil {
			return merged
		}
	}
	out := make([]byte, len(defaultAutoBirdJSON))
	copy(out, defaultAutoBirdJSON)
	return out
}

// WriteAutoBirdClientFile writes AutoBird.json (atomic replace).
func WriteAutoBirdClientFile(data []byte) error {
	if len(data) == 0 {
		data = append([]byte(nil), defaultAutoBirdJSON...)
	}
	var check interface{}
	if err := json.Unmarshal(data, &check); err != nil {
		return err
	}
	autoBirdClientMu.Lock()
	defer autoBirdClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := autoBirdClientPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DefaultAutoBirdClientJSON returns a copy of the built-in default document.
func DefaultAutoBirdClientJSON() []byte {
	out := make([]byte, len(defaultAutoBirdJSON))
	copy(out, defaultAutoBirdJSON)
	return out
}
