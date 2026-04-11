package decoration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"CitadelDesktop/Server/paths"
)

// PresetLayer is BG or BD from JAA gca.
type PresetLayer string

const (
	LayerBG PresetLayer = "BG"
	LayerBD PresetLayer = "BD"
)

// PresetPlacement is one decoration slot saved from BuildingData (no PWR/PO/DOID; EBU uses defaults).
type PresetPlacement struct {
	WID   int         `json:"wid"`
	X     int         `json:"x"`
	Y     int         `json:"y"`
	R     int         `json:"r"`
	Layer PresetLayer `json:"layer"`
}

// NamedPreset is a named layout for one castle.
type NamedPreset struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Items []PresetPlacement `json:"items"`
}

// PresetsFile is the on-disk JSON root (castle instance id string -> presets).
type PresetsFile struct {
	Version int                      `json:"version"`
	Castles map[string][]NamedPreset `json:"castles"`
}

const presetsFileVersion = 1

var (
	presetsMu     sync.Mutex
	presetsPath   string
	presetsCached *PresetsFile
)

func presetsFilePath() string {
	if presetsPath != "" {
		return presetsPath
	}
	return filepath.Join(paths.DataDir(), "DecorationPresets.json")
}

// SetDecorationPresetsPathForTest overrides the presets file path (call with empty to reset).
func SetDecorationPresetsPathForTest(p string) {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	presetsPath = p
	presetsCached = nil
}

func tryMigrateDecorationPresetsFromLegacy() *PresetsFile {
	var legacyPaths []string
	if d := paths.LegacyDotCitadelOpsDir(); d != "" {
		legacyPaths = append(legacyPaths, filepath.Join(d, "DecorationPresets.json"))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		legacyPaths = append(legacyPaths, filepath.Join(filepath.Dir(exe), "DecorationPresets.json"))
	}
	for _, p := range legacyPaths {
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			continue
		}
		var f PresetsFile
		if json.Unmarshal(b, &f) != nil {
			continue
		}
		if f.Castles == nil {
			f.Castles = make(map[string][]NamedPreset)
		}
		f.Version = presetsFileVersion
		return &f
	}
	return nil
}

func loadPresetsUnlocked() *PresetsFile {
	if presetsCached != nil {
		return presetsCached
	}
	path := presetsFilePath()
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		if migrated := tryMigrateDecorationPresetsFromLegacy(); migrated != nil {
			_ = savePresetsUnlocked(migrated)
			return presetsCached
		}
		presetsCached = &PresetsFile{Version: presetsFileVersion, Castles: make(map[string][]NamedPreset)}
		return presetsCached
	}
	var f PresetsFile
	if err := json.Unmarshal(b, &f); err != nil {
		presetsCached = &PresetsFile{Version: presetsFileVersion, Castles: make(map[string][]NamedPreset)}
		return presetsCached
	}
	if f.Castles == nil {
		f.Castles = make(map[string][]NamedPreset)
	}
	f.Version = presetsFileVersion
	presetsCached = &f
	return presetsCached
}

func savePresetsUnlocked(f *PresetsFile) error {
	f.Version = presetsFileVersion
	path := presetsFilePath()
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0600); err != nil {
		return err
	}
	presetsCached = f
	return nil
}

// LoadDecorationPresets returns a copy of the in-memory / disk presets file.
func LoadDecorationPresets() PresetsFile {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	out := PresetsFile{Version: f.Version, Castles: make(map[string][]NamedPreset, len(f.Castles))}
	for k, v := range f.Castles {
		cp := make([]NamedPreset, len(v))
		copy(cp, v)
		out.Castles[k] = cp
	}
	return out
}

// ListPresetsForCastle returns presets for the given castle instance id.
func ListPresetsForCastle(castleID int) []NamedPreset {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	key := fmt.Sprintf("%d", castleID)
	return append([]NamedPreset(nil), f.Castles[key]...)
}

// SavePresetForCastle appends a new preset built from items (caller filters decorations).
func SavePresetForCastle(castleID int, name string, items []PresetPlacement) (NamedPreset, error) {
	if name == "" {
		return NamedPreset{}, fmt.Errorf("preset name required")
	}
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	key := fmt.Sprintf("%d", castleID)
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	p := NamedPreset{ID: id, Name: name, Items: append([]PresetPlacement(nil), items...)}
	f.Castles[key] = append(f.Castles[key], p)
	if err := savePresetsUnlocked(f); err != nil {
		return NamedPreset{}, err
	}
	return p, nil
}

// DeletePreset removes a preset by id for the castle.
func DeletePreset(castleID int, presetID string) error {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	key := fmt.Sprintf("%d", castleID)
	list := f.Castles[key]
	var out []NamedPreset
	for _, p := range list {
		if p.ID != presetID {
			out = append(out, p)
		}
	}
	f.Castles[key] = out
	return savePresetsUnlocked(f)
}

// LookupPreset returns a deep copy of the named preset for the castle, if it exists.
func LookupPreset(castleID int, presetID string) (NamedPreset, bool) {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	key := fmt.Sprintf("%d", castleID)
	for _, p := range f.Castles[key] {
		if p.ID != presetID {
			continue
		}
		out := p
		out.Items = append([]PresetPlacement(nil), p.Items...)
		return out, true
	}
	return NamedPreset{}, false
}
