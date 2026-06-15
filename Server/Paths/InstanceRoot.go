package Paths

import (
	"os"
	"path/filepath"
	"strings"
)

const appConfigDirName = "CitadelDesktop"

func isEphemeralGoRunExecutable(exe string) bool {
	if exe == "" {
		return false
	}
	p := strings.ToLower(filepath.ToSlash(exe))
	return strings.Contains(p, "/go-build") && strings.Contains(p, "/exe/")
}

// devInstanceRootFromCwd walks up from the working directory to find a go.mod (go run from repo).
func devInstanceRootFromCwd() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return "", false
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func stableDevInstanceRoot() (string, error) {
	if dev, ok := devInstanceRootFromCwd(); ok {
		return dev, nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, appConfigDirName), nil
}

// InstanceRoot is the per-instance directory: the same parent used for ChromeProfile
// (binary directory, or CITADEL_DATA_DIR, or OS config fallback). Each copy of the
// app in its own folder, or each CITADEL_DATA_DIR value, is an isolated instance for
// multi-account / parallel runs.
//
// Resolution matches Server/ChromeUserData (ChromeProfile lives at InstanceRoot()/ChromeProfile).
// go run uses a fresh go-build temp exe each session; in that case we anchor to the repo (go.mod) or OS config.
func InstanceRoot() (string, error) {
	if d := strings.TrimSpace(os.Getenv("CITADEL_DATA_DIR")); d != "" {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
		return d, nil
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if isEphemeralGoRunExecutable(exe) {
			root, err := stableDevInstanceRoot()
			if err != nil {
				return "", err
			}
			_ = os.MkdirAll(root, 0755)
			return root, nil
		}
		return filepath.Dir(exe), nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, appConfigDirName), nil
}

// DataDir is the directory for this software’s durable JSON (decoration presets, Auto Bird, Auto TCI,
// game state snapshots, optional CidTrivialProduct.json overrides, etc.; shipped PID+AMT come from
// Server/Data/packages/items.json via GameParser). It is not the same as Server/Data, which
// should hold only copies of official in-game catalog JSON from public endpoints. InstanceRoot()/Data. Created on demand.
func DataDir() string {
	root, err := InstanceRoot()
	if err != nil {
		d := filepath.Join(".", "Data")
		_ = os.MkdirAll(d, 0755)
		return d
	}
	dir := filepath.Join(root, "Data")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// LegacyDotCitadelOpsDir is the old default (~/.citadelops) used before Data/ next to the binary.
func LegacyDotCitadelOpsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".citadelops")
}
