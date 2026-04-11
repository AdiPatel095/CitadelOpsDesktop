package Paths

import (
	"os"
	"path/filepath"
	"strings"
)

const appConfigDirName = "CitadelDesktop"

// InstanceRoot is the per-instance directory: the same parent used for ChromeProfile
// (binary directory, or CITADEL_DATA_DIR, or OS config fallback). Each copy of the
// app in its own folder, or each CITADEL_DATA_DIR value, is an isolated instance for
// multi-account / parallel runs.
//
// Resolution matches Server/ChromeUserData (ChromeProfile lives at InstanceRoot()/ChromeProfile).
func InstanceRoot() (string, error) {
	if d := strings.TrimSpace(os.Getenv("CITADEL_DATA_DIR")); d != "" {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
		return d, nil
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return filepath.Dir(exe), nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, appConfigDirName), nil
}

// DataDir is the directory for durable JSON (decoration presets, Auto Bird, sent-bird log,
// game snapshots): InstanceRoot()/Data. Created on demand.
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
