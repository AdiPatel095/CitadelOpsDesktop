package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// EmpireItemsSnapshotMeta describes the Empire items snapshot (EmpireItemsMeta.json next to the EmpireItems/ folder).
type EmpireItemsSnapshotMeta struct {
	CastleItemXMLVersion string `json:"castleItemXMLVersion"`
	SectionCount         int    `json:"sectionCount"`
	FetchedAt            string `json:"fetchedAt"`
	SourceURL            string `json:"sourceUrl"`
	ItemsDirectory       string `json:"itemsDirectory"`
}

var (
	empireDataDirMu       sync.RWMutex
	empireDataDirOverride string

	empireItemsDirMu       sync.RWMutex
	empireItemsDirOverride string

	empireMetaOnce sync.Once
	empireMeta     EmpireItemsSnapshotMeta
	empireMetaErr  error
)

// SetEmpireDataDir sets the directory that contains EmpireItemsMeta.json (default: auto-detect Server/Data or Data).
func SetEmpireDataDir(dir string) {
	empireDataDirMu.Lock()
	defer empireDataDirMu.Unlock()
	empireDataDirOverride = dir
}

// SetEmpireItemsDir sets the directory of per-section *.json files (default: <EmpireDataDir>/EmpireItems).
func SetEmpireItemsDir(dir string) {
	empireItemsDirMu.Lock()
	defer empireItemsDirMu.Unlock()
	empireItemsDirOverride = dir
}

func empireDataDir() (string, error) {
	empireDataDirMu.RLock()
	o := empireDataDirOverride
	empireDataDirMu.RUnlock()
	if o != "" {
		return filepath.Clean(o), nil
	}
	if env := strings.TrimSpace(os.Getenv("CITADEL_DATA_DIR")); env != "" {
		return filepath.Clean(env), nil
	}
	candidates := []string{
		filepath.Join("Server", "Data"),
		"Data",
	}
	for _, c := range candidates {
		meta := filepath.Join(c, "EmpireItemsMeta.json")
		if _, err := os.Stat(meta); err == nil {
			return filepath.Abs(c)
		}
	}
	ex, err := os.Executable()
	if err == nil {
		cand := filepath.Join(filepath.Dir(ex), "Data")
		if _, err2 := os.Stat(filepath.Join(cand, "EmpireItemsMeta.json")); err2 == nil {
			return filepath.Abs(cand)
		}
	}
	return "", fmt.Errorf("empire data directory not found (expected EmpireItemsMeta.json); set CITADEL_DATA_DIR or Server/Data")
}

func empireItemsDir() (string, error) {
	empireItemsDirMu.RLock()
	o := empireItemsDirOverride
	empireItemsDirMu.RUnlock()
	if o != "" {
		return filepath.Clean(o), nil
	}
	if env := strings.TrimSpace(os.Getenv("CITADEL_EMPIRE_ITEMS_DIR")); env != "" {
		return filepath.Clean(env), nil
	}
	base, err := empireDataDir()
	if err != nil {
		return "", err
	}
	sub := "EmpireItems"
	if _, err := os.Stat(filepath.Join(base, sub)); err == nil {
		return filepath.Join(base, sub), nil
	}
	return "", fmt.Errorf("empire items section directory %q not found under %q", sub, base)
}

// LoadEmpireItemsSnapshotMeta reads and caches EmpireItemsMeta.json from the resolved data directory.
func LoadEmpireItemsSnapshotMeta() (EmpireItemsSnapshotMeta, error) {
	empireMetaOnce.Do(func() {
		base, err := empireDataDir()
		if err != nil {
			empireMetaErr = err
			return
		}
		b, err := os.ReadFile(filepath.Join(base, "EmpireItemsMeta.json"))
		if err != nil {
			empireMetaErr = err
			return
		}
		empireMetaErr = json.Unmarshal(b, &empireMeta)
	})
	return empireMeta, empireMetaErr
}

func validSectionKey(key string) bool {
	if key == "" || key == "." || key == ".." {
		return false
	}
	if strings.Contains(key, string(filepath.Separator)) || strings.Contains(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	return true
}

// ReadEmpireItemsSection returns raw JSON for one top-level GGE items key (filename: <key>.json under EmpireItems/).
func ReadEmpireItemsSection(sectionKey string) ([]byte, error) {
	if !validSectionKey(sectionKey) {
		return nil, fmt.Errorf("invalid empire items section key %q", sectionKey)
	}
	dir, err := empireItemsDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sectionKey+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read empire items section %q: %w", sectionKey, err)
	}
	return b, nil
}

// UnmarshalEmpireItemsSection reads one section file and unmarshals into v.
func UnmarshalEmpireItemsSection(sectionKey string, v any) error {
	b, err := ReadEmpireItemsSection(sectionKey)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// EmpireItemsSectionNames returns sorted section keys (base names of *.json in the EmpireItems directory).
func EmpireItemsSectionNames() ([]string, error) {
	dir, err := empireItemsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read empire items dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ".json"))
	}
	slices.Sort(names)
	return names, nil
}
