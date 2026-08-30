//go:build !windows

package State

import "os"

func replaceStateFile(source string, destination string) error {
	return os.Rename(source, destination)
}

func moveStateDirectory(source string, destination string) error {
	return os.Rename(source, destination)
}
