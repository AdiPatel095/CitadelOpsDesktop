//go:build windows

package State

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectoryWindowsAvoidsUnsupportedDirectoryFlush(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("sync accessible directory: %v", err)
	}
}

func TestSyncDirectoryWindowsStillReportsOpenFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := syncDirectory(missing)
	if err == nil {
		t.Fatal("sync missing directory unexpectedly succeeded")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("sync missing directory error = %v, want not-exist", err)
	}
}
