// Package ChromeUserData provides the on-disk user data directory for the
// Chromium instance CitadelDesktop launches (cookies, logins, local storage, etc.).
package ChromeUserData

import (
	"os"
	"path/filepath"

	"CitadelDesktop/Server/paths"
)

const chromeProfileDirName = "ChromeProfile"

// AppUserDataDir returns the absolute path to this application’s Chrome user
// data root (a full --user-data-dir value). The directory is created if needed.
//
// Same instance root as paths.InstanceRoot(): CITADEL_DATA_DIR, else directory of the
// executable, else OS config dir / CitadelDesktop. ChromeProfile is a subdirectory
// of that root, alongside paths.DataDir() (…/Data).
func AppUserDataDir() (string, error) {
	parent, err := paths.InstanceRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(parent, chromeProfileDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
