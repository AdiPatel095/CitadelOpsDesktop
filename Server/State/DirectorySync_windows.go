//go:build windows

package State

import "os"

func syncDirectory(directory string) error {
	// On Windows, os.File.Sync delegates to FlushFileBuffers. A directory
	// opened by os.Open has read access, while FlushFileBuffers requires a
	// handle with GENERIC_WRITE access, so syncing it reports access denied.
	// State files and quarantine directories are moved into place with
	// MOVEFILE_WRITE_THROUGH before this check. Opening and closing here still
	// verifies directory accessibility without issuing an unsupported flush.
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	return handle.Close()
}
