package decoration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"CitadelDesktop/Server/Paths"
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

// PresetsFile is the on-disk JSON root (castle key string -> presets).
// Keys are usually the castle instance id; Storm castle uses the stable slot key "stormCastle".
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
	return filepath.Join(Paths.DataDir(), "DecorationPresets.json")
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
	if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
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

func mergePresetsUnique(dst, src []NamedPreset) []NamedPreset {
	if len(src) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst))
	for _, p := range dst {
		seen[p.ID] = struct{}{}
	}
	for _, p := range src {
		if _, ok := seen[p.ID]; ok {
			continue
		}
		dst = append(dst, p)
		seen[p.ID] = struct{}{}
	}
	return dst
}

// migrateLegacyNumericKey moves presets from a rotating castle's numeric instance key into storageKey.
func migrateLegacyNumericKeyUnlocked(f *PresetsFile, storageKey, legacyNumericKey string) bool {
	if storageKey == "" || legacyNumericKey == "" || storageKey == legacyNumericKey {
		return false
	}
	legacy := f.Castles[legacyNumericKey]
	if len(legacy) == 0 {
		return false
	}
	f.Castles[storageKey] = mergePresetsUnique(append([]NamedPreset(nil), f.Castles[storageKey]...), legacy)
	delete(f.Castles, legacyNumericKey)
	return true
}

func maybeMigrateRotatingCastleKeyUnlocked(f *PresetsFile, storageKey string, castleID int) bool {
	if castleID <= 0 || storageKey == fmt.Sprintf("%d", castleID) {
		return false
	}
	return migrateLegacyNumericKeyUnlocked(f, storageKey, fmt.Sprintf("%d", castleID))
}

// ListPresetsForKey returns presets for the given storage key.
// When storageKey differs from castleID (Storm castle), presets under the numeric id are migrated first.
func ListPresetsForKey(storageKey string, castleID int) []NamedPreset {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	if maybeMigrateRotatingCastleKeyUnlocked(f, storageKey, castleID) {
		_ = savePresetsUnlocked(f)
	}
	return append([]NamedPreset(nil), f.Castles[storageKey]...)
}

// SavePresetForKey appends a new preset built from items (caller filters decorations).
func SavePresetForKey(storageKey string, castleID int, name string, items []PresetPlacement) (NamedPreset, error) {
	if name == "" {
		return NamedPreset{}, fmt.Errorf("preset name required")
	}
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	if maybeMigrateRotatingCastleKeyUnlocked(f, storageKey, castleID) {
		_ = savePresetsUnlocked(f)
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	p := NamedPreset{ID: id, Name: name, Items: append([]PresetPlacement(nil), items...)}
	f.Castles[storageKey] = append(f.Castles[storageKey], p)
	if err := savePresetsUnlocked(f); err != nil {
		return NamedPreset{}, err
	}
	return p, nil
}

// DeletePresetForKey removes a preset by id for the storage key.
func DeletePresetForKey(storageKey string, castleID int, presetID string) error {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	if maybeMigrateRotatingCastleKeyUnlocked(f, storageKey, castleID) {
		_ = savePresetsUnlocked(f)
	}
	list := f.Castles[storageKey]
	var out []NamedPreset
	for _, p := range list {
		if p.ID != presetID {
			out = append(out, p)
		}
	}
	f.Castles[storageKey] = out
	return savePresetsUnlocked(f)
}

// LookupPresetForKey returns a deep copy of the named preset for the storage key, if it exists.
func LookupPresetForKey(storageKey string, castleID int, presetID string) (NamedPreset, bool) {
	presetsMu.Lock()
	defer presetsMu.Unlock()
	f := loadPresetsUnlocked()
	if maybeMigrateRotatingCastleKeyUnlocked(f, storageKey, castleID) {
		_ = savePresetsUnlocked(f)
	}
	for _, p := range f.Castles[storageKey] {
		if p.ID != presetID {
			continue
		}
		out := p
		out.Items = append([]PresetPlacement(nil), p.Items...)
		return out, true
	}
	return NamedPreset{}, false
}
