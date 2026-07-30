package Runtime

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofrs/flock"
)

var ErrProfileInUse = errors.New("CitadelOps data directory is already owned by another process")

type ProfileLease struct {
	ProfileID string
	Path      string

	lock      *flock.Flock
	closeOnce sync.Once
	closeErr  error
}

func AcquireProfileLease(dataDir string) (*ProfileLease, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("profile data directory is required")
	}
	directory := filepath.Join(dataDir, "Runtime")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}
	path := filepath.Join(directory, "Profile.lock")
	fileLock := flock.New(path)
	locked, err := fileLock.TryLock()
	if err != nil {
		_ = fileLock.Close()
		return nil, fmt.Errorf("acquire profile lease: %w", err)
	}
	if !locked {
		_ = fileLock.Close()
		return nil, fmt.Errorf("%w: %s", ErrProfileInUse, dataDir)
	}
	profileID, err := loadOrCreateProfileID(directory)
	if err != nil {
		_ = fileLock.Unlock()
		_ = fileLock.Close()
		return nil, err
	}
	return &ProfileLease{ProfileID: profileID, Path: path, lock: fileLock}, nil
}

func (lease *ProfileLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.closeOnce.Do(func() {
		if lease.lock == nil {
			return
		}
		lease.closeErr = errors.Join(lease.lock.Unlock(), lease.lock.Close())
	})
	return lease.closeErr
}

func loadOrCreateProfileID(directory string) (string, error) {
	path := filepath.Join(directory, "ProfileID")
	contents, err := os.ReadFile(path)
	if err == nil {
		profileID := strings.TrimSpace(string(contents))
		if profileID == "" {
			return "", fmt.Errorf("profile identity is empty: %s", path)
		}
		return profileID, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read profile identity: %w", err)
	}
	profileID, err := newProfileID()
	if err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create profile identity: %w", err)
	}
	if _, err := file.WriteString(profileID + "\n"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write profile identity: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("sync profile identity: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close profile identity: %w", err)
	}
	return profileID, nil
}

func newProfileID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate profile identity: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	), nil
}
