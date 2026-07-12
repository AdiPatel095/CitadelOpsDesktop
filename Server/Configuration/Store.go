package Configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion   = 1
	maxSectionBytes = 1 << 20
)

type Snapshot struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Revision      uint64                     `json:"revision"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
	Sections      map[string]json.RawMessage `json:"sections"`
}

type Event struct {
	Revision  uint64          `json:"revision"`
	Section   string          `json:"section"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Snapshot  Snapshot        `json:"snapshot"`
}

type Store struct {
	path string

	mu          sync.RWMutex
	snapshot    Snapshot
	subscribers map[uint64]chan Event
	nextID      uint64
}

func Open(dataDir string, defaults map[string]json.RawMessage) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("configuration data directory is required")
	}
	path := filepath.Join(dataDir, "Config", "Settings.json")
	now := time.Now().UTC()
	snapshot := Snapshot{
		SchemaVersion: SchemaVersion,
		UpdatedAt:     now,
		Sections:      map[string]json.RawMessage{},
	}
	contents, err := os.ReadFile(path)
	loaded := false
	switch {
	case err == nil:
		loaded = true
		if err := json.Unmarshal(contents, &snapshot); err != nil {
			return nil, fmt.Errorf("decode configuration: %w", err)
		}
		if snapshot.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("unsupported configuration schema %d", snapshot.SchemaVersion)
		}
		if snapshot.Sections == nil {
			snapshot.Sections = map[string]json.RawMessage{}
		}
		if snapshot.UpdatedAt.IsZero() {
			snapshot.UpdatedAt = now
		}
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	changed := !loaded
	migrated, err := migrateLegacySections(dataDir, snapshot.Sections)
	if err != nil {
		return nil, err
	}
	changed = changed || migrated
	for section, value := range defaults {
		if _, exists := snapshot.Sections[section]; exists {
			continue
		}
		canonical, err := canonicalSection(section, value)
		if err != nil {
			return nil, fmt.Errorf("default configuration %s: %w", section, err)
		}
		snapshot.Sections[section] = canonical
		changed = true
	}
	for section, value := range snapshot.Sections {
		canonical, err := canonicalSection(section, value)
		if err != nil {
			return nil, fmt.Errorf("configuration %s: %w", section, err)
		}
		if !bytes.Equal(value, canonical) {
			changed = true
		}
		snapshot.Sections[section] = canonical
	}
	if changed {
		if loaded {
			snapshot.Revision++
		}
		snapshot.UpdatedAt = now
		if err := writeSnapshot(path, snapshot); err != nil {
			return nil, err
		}
	}
	return &Store{
		path: path, snapshot: snapshot, subscribers: map[uint64]chan Event{},
	}, nil
}

func (store *Store) Snapshot() Snapshot {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return cloneSnapshot(store.snapshot)
}

func (store *Store) Section(name string) (json.RawMessage, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	value, ok := store.snapshot.Sections[name]
	return cloneRaw(value), ok
}

func (store *Store) Update(section string, value json.RawMessage) (Snapshot, error) {
	return store.UpdateExpected(section, value, nil)
}

func (store *Store) UpdateExpected(section string, value json.RawMessage, expectedRevision *uint64) (Snapshot, error) {
	canonical, err := canonicalSection(section, value)
	if err != nil {
		return Snapshot{}, err
	}
	store.mu.Lock()
	if expectedRevision != nil && store.snapshot.Revision != *expectedRevision {
		actual := store.snapshot.Revision
		store.mu.Unlock()
		return Snapshot{}, fmt.Errorf("configuration revision changed: expected %d, current %d", *expectedRevision, actual)
	}
	current, exists := store.snapshot.Sections[section]
	if exists && bytes.Equal(current, canonical) {
		snapshot := cloneSnapshot(store.snapshot)
		store.mu.Unlock()
		return snapshot, nil
	}
	next := cloneSnapshot(store.snapshot)
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	next.Sections[section] = canonical
	if err := writeSnapshot(store.path, next); err != nil {
		store.mu.Unlock()
		return Snapshot{}, err
	}
	store.snapshot = next
	event := Event{
		Revision: next.Revision, Section: section, Value: cloneRaw(canonical),
		UpdatedAt: next.UpdatedAt, Snapshot: cloneSnapshot(next),
	}
	for _, subscriber := range store.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
	snapshot := cloneSnapshot(next)
	store.mu.Unlock()
	return snapshot, nil
}

func Validate(section string, value json.RawMessage) error {
	_, err := canonicalSection(section, value)
	return err
}

func (store *Store) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	store.mu.Lock()
	store.nextID++
	id := store.nextID
	channel := make(chan Event, buffer)
	store.subscribers[id] = channel
	store.mu.Unlock()
	return channel, func() {
		store.mu.Lock()
		if subscriber, ok := store.subscribers[id]; ok {
			delete(store.subscribers, id)
			close(subscriber)
		}
		store.mu.Unlock()
	}
}

func canonicalSection(section string, value json.RawMessage) (json.RawMessage, error) {
	if !validSectionName(section) {
		return nil, fmt.Errorf("invalid configuration section %q", section)
	}
	if len(value) == 0 || len(value) > maxSectionBytes {
		return nil, fmt.Errorf("configuration section %q must contain 1 to %d bytes", section, maxSectionBytes)
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("configuration section %q is not valid JSON", section)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return nil, err
	}
	return json.RawMessage(compact.Bytes()), nil
}

func validSectionName(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func writeSnapshot(path string, snapshot Snapshot) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	contents, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, ".Settings-*")
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(path)
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("save configuration: %w", retryErr)
		}
	}
	return nil
}

func cloneSnapshot(source Snapshot) Snapshot {
	clone := source
	clone.Sections = make(map[string]json.RawMessage, len(source.Sections))
	for section, value := range source.Sections {
		clone.Sections[section] = cloneRaw(value)
	}
	return clone
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
