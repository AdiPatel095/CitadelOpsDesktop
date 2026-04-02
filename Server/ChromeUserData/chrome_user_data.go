// Package ChromeUserData provides the on-disk user data directory for the
// Chromium instance CitadelDesktop launches (cookies, logins, local storage, etc.).
package ChromeUserData

import (
	"os"
	"path/filepath"
	"strings"
)

const appConfigDirName = "CitadelDesktop"

// AppUserDataDir returns the absolute path to this application’s Chrome user
// data root (a full --user-data-dir value). The directory is created if needed.
// If CITADEL_DATA_DIR is set, the profile is <dir>/ChromeProfile (same env as other app data).
// Otherwise it uses the OS user config dir (e.g. ~/Library/Application Support/CitadelDesktop on macOS).
func AppUserDataDir() (string, error) {
	var parent string
	if d := strings.TrimSpace(os.Getenv("CITADEL_DATA_DIR")); d != "" {
		parent = d
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", err
		}
	} else {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		parent = filepath.Join(cfg, appConfigDirName)
	}
	dir := filepath.Join(parent, "ChromeProfile")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
