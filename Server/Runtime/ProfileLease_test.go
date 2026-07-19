package Runtime

import (
	"errors"
	"testing"
)

func TestProfileLeaseFencesDataDirectoryAndKeepsStableIdentity(t *testing.T) {
	dataDir := t.TempDir()
	first, err := AcquireProfileLease(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProfileID == "" {
		t.Fatal("profile identity is empty")
	}
	if _, err := AcquireProfileLease(dataDir); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("second profile lease error = %v", err)
	}
	profileID := first.ProfileID
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireProfileLease(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if second.ProfileID != profileID {
		t.Fatalf("profile identity changed from %q to %q", profileID, second.ProfileID)
	}
}
