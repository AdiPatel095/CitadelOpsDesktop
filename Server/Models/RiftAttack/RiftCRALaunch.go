package riftattack

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// CRALaunchBodyJSON is the persisted **cra** JSON body (same fields as GameCommands.CRALaunchBody).
type CRALaunchBodyJSON map[string]interface{}

const (
	riftCRALaunchFileName   = "rift_cra_launch.json"
	maxSavedLaunches        = 15
	maxLaunchDisplayNameLen = 80
)

// SavedLaunch is one captured outbound **cra** frame targeting the world Rift.
type SavedLaunch struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName,omitempty"`
	SavedAtUnix int64             `json:"savedAtUnix"`
	WirePayload string            `json:"wirePayload"`
	Body        CRALaunchBodyJSON `json:"body"`
	// OneWayTTSeconds is feather-boosted one-way travel (**M.TT**) from the last successful inbound **cra** ack.
	OneWayTTSeconds   int   `json:"oneWayTTSeconds,omitempty"`
	LastSuccessAtUnix int64 `json:"lastSuccessAtUnix,omitempty"`
}

// File is persisted JSON for re-sending captured Rift attack templates.
type File struct {
	PlayerID   int           `json:"playerId"`
	Launches   []SavedLaunch `json:"launches,omitempty"`
	LastLaunch *SavedLaunch  `json:"lastLaunch,omitempty"` // legacy single-entry field
}

var fileMu sync.Mutex

func filePath() string {
	return filepath.Join(Paths.DataDir(), riftCRALaunchFileName)
}

// FilePath is the persisted rift CRA templates JSON path (DataDir/rift_cra_launch.json).
func FilePath() string {
	return filePath()
}

func tryMigrateRiftCRALaunchFromLegacy() *File {
	var legacyPaths []string
	if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
		legacyPaths = append(legacyPaths, filepath.Join(d, riftCRALaunchFileName))
	}
	if root, err := Paths.InstanceRoot(); err == nil && root != "" {
		legacyPaths = append(legacyPaths, filepath.Join(root, riftCRALaunchFileName))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		legacyPaths = append(legacyPaths, filepath.Join(filepath.Dir(exe), riftCRALaunchFileName))
	}
	for _, p := range legacyPaths {
		if p == filePath() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			continue
		}
		var f File
		if json.Unmarshal(b, &f) != nil {
			continue
		}
		log.Printf("[riftattack] migrated launches from %s", p)
		normalized := normalizeFile(&f)
		archiveLegacyRiftCRALaunchFile(p)
		return normalized
	}
	return nil
}

func archiveLegacyRiftCRALaunchFile(path string) {
	if path == "" || path == filePath() {
		return
	}
	legacyHome := Paths.LegacyDotCitadelOpsDir()
	if legacyHome != "" && strings.HasPrefix(filepath.Clean(path), filepath.Clean(legacyHome)) {
		return
	}
	archived := path + ".migrated"
	if err := os.Rename(path, archived); err == nil {
		log.Printf("[riftattack] archived legacy launches file → %s", archived)
	}
}

func normalizeFile(f *File) *File {
	if f == nil {
		return &File{}
	}
	if len(f.Launches) == 0 && f.LastLaunch != nil {
		legacy := *f.LastLaunch
		if legacy.ID == "" {
			legacy.ID = launchID(legacy.SavedAtUnix, legacy.Body)
		}
		f.Launches = []SavedLaunch{legacy}
		f.LastLaunch = nil
	}
	return f
}

// attackSetupFingerprint returns a stable canonical key for the troop/wave layout in a **cra** body.
// Compared fields: **A** (waves/flanks), **AST** (attack support tools), **RW** (support troops).
// Coords, commander, AV, and movement boosters are ignored so the same layout from another castle
// or commander is not captured twice.
func attackSetupFingerprint(body CRALaunchBodyJSON) string {
	if body == nil {
		return ""
	}
	subset := make(map[string]interface{}, 3)
	for _, key := range []string{"A", "AST", "RW"} {
		if v, ok := body[key]; ok {
			subset[key] = v
		}
	}
	if len(subset) == 0 {
		return ""
	}
	b, err := json.Marshal(subset)
	if err != nil {
		return ""
	}
	return string(b)
}

func hasMatchingAttackSetup(launches []SavedLaunch, body CRALaunchBodyJSON) bool {
	fp := attackSetupFingerprint(body)
	if fp == "" {
		return false
	}
	for _, existing := range launches {
		if attackSetupFingerprint(existing.Body) == fp {
			return true
		}
	}
	return false
}

func launchID(savedAtUnix int64, body CRALaunchBodyJSON) string {
	lid := 0
	if body != nil {
		if v, ok := body["LID"].(float64); ok {
			lid = int(v)
		}
	}
	return fmt.Sprintf("%d-%d", savedAtUnix, lid)
}

func loadUnlocked() *File {
	path := filePath()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		if migrated := tryMigrateRiftCRALaunchFromLegacy(); migrated != nil {
			saveInternal(migrated)
			return migrated
		}
		return &File{}
	}
	var f File
	if json.Unmarshal(data, &f) != nil {
		return &File{}
	}
	return normalizeFile(&f)
}

func saveInternal(f *File) {
	if f == nil {
		return
	}
	f = normalizeFile(f)
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		log.Printf("[riftattack] marshal: %v", err)
		return
	}
	tmp := filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("[riftattack] write: %v", err)
		return
	}
	if err := os.Rename(tmp, filePath()); err != nil {
		log.Printf("[riftattack] rename: %v", err)
		_ = os.Remove(tmp)
		return
	}
	log.Printf("[riftattack] saved %d launch(es) → %s", len(f.Launches), filePath())
}

// Load returns the persisted Rift CRA template file (empty if missing).
func Load() *File {
	fileMu.Lock()
	defer fileMu.Unlock()
	return loadUnlocked()
}

// Snapshot returns a shallow copy of the persisted file safe for read-only iteration off the lock.
func Snapshot() File {
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	if f == nil {
		return File{}
	}
	out := *normalizeFile(f)
	if len(out.Launches) > 0 {
		out.Launches = append([]SavedLaunch(nil), out.Launches...)
	}
	return out
}

// FindLaunch returns one saved launch by id.
func FindLaunch(id string) (SavedLaunch, bool) {
	if id == "" {
		return SavedLaunch{}, false
	}
	f := Load()
	for _, launch := range f.Launches {
		if launch.ID == id {
			return launch, true
		}
	}
	return SavedLaunch{}, false
}

// AppendLaunch stores a new Rift-targeting **cra** launch (newest first). Returns false when skipped
// because the wire payload or troop/wave layout (**A**/**AST**/**RW**) already exists in the list.
func AppendLaunch(playerID int, launch SavedLaunch) bool {
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	f.PlayerID = playerID

	if launch.ID == "" {
		launch.ID = launchID(launch.SavedAtUnix, launch.Body)
	}

	for _, existing := range f.Launches {
		if existing.WirePayload == launch.WirePayload {
			return false
		}
	}
	if hasMatchingAttackSetup(f.Launches, launch.Body) {
		return false
	}

	f.Launches = append([]SavedLaunch{launch}, f.Launches...)
	if len(f.Launches) > maxSavedLaunches {
		f.Launches = f.Launches[:maxSavedLaunches]
	}
	saveInternal(f)
	return true
}

// UpdateLaunchTravelTime records feather one-way **TT** from a successful inbound **cra** for the newest matching launch.
func UpdateLaunchTravelTime(commanderID, oneWayTT int, successAtUnix int64) bool {
	if commanderID < 0 || oneWayTT <= 0 {
		return false
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	updated := false
	for i := range f.Launches {
		launch := &f.Launches[i]
		lid := commanderIDFromBody(launch.Body)
		if lid != commanderID {
			continue
		}
		if !launchUsesTravelFeather(launch.Body) {
			continue
		}
		launch.OneWayTTSeconds = oneWayTT
		launch.LastSuccessAtUnix = successAtUnix
		updated = true
		break
	}
	if updated {
		saveInternal(f)
	}
	return updated
}

// CommanderIDFromLaunch reads wire **LID** from a saved launch body.
func CommanderIDFromLaunch(launch SavedLaunch) int {
	return commanderIDFromBody(launch.Body)
}

func commanderIDFromBody(body CRALaunchBodyJSON) int {
	if body == nil {
		return -1
	}
	if v, ok := body["LID"].(float64); ok {
		return int(v)
	}
	return -1
}

func launchUsesTravelFeather(body CRALaunchBodyJSON) bool {
	if body == nil {
		return false
	}
	hbw, hbwOK := body["HBW"].(float64)
	ptt, pttOK := body["PTT"].(float64)
	return hbwOK && pttOK && int(hbw) == -1 && int(ptt) == 1
}

// NormalizeLaunchDisplayName trims and caps user-facing launch names.
func NormalizeLaunchDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > maxLaunchDisplayNameLen {
		name = name[:maxLaunchDisplayNameLen]
	}
	return name
}

// RenameLaunch sets or clears the display name for one saved launch.
func RenameLaunch(id, displayName string) bool {
	if id == "" {
		return false
	}
	displayName = NormalizeLaunchDisplayName(displayName)

	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	for i := range f.Launches {
		if f.Launches[i].ID != id {
			continue
		}
		f.Launches[i].DisplayName = displayName
		saveInternal(f)
		return true
	}
	return false
}

// DeleteLaunch removes one saved launch by id.
func DeleteLaunch(id string) bool {
	if id == "" {
		return false
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	f := loadUnlocked()
	out := make([]SavedLaunch, 0, len(f.Launches))
	removed := false
	for _, launch := range f.Launches {
		if launch.ID == id {
			removed = true
			continue
		}
		out = append(out, launch)
	}
	if !removed {
		return false
	}
	f.Launches = out
	saveInternal(f)
	return true
}
