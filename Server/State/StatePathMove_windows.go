//go:build windows

package State

import (
	"os"

	"golang.org/x/sys/windows"
)

func replaceStateFile(source string, destination string) error {
	return moveStatePathWindows(
		source,
		destination,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func moveStateDirectory(source string, destination string) error {
	return moveStatePathWindows(source, destination, windows.MOVEFILE_WRITE_THROUGH)
}

func moveStatePathWindows(source string, destination string, flags uint32) error {
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: source, New: destination, Err: err}
	}
	destinationPointer, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: source, New: destination, Err: err}
	}
	if err := windows.MoveFileEx(sourcePointer, destinationPointer, flags); err != nil {
		return &os.LinkError{Op: "rename", Old: source, New: destination, Err: err}
	}
	return nil
}
