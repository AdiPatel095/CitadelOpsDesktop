package Configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion   = 1
	maxSectionBytes = 1 << 20
)

var ErrExternalAuthority = errors.New("configuration is owned by an external authority")

type Snapshot struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Revision      uint64                     `json:"revision"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
	Sections      map[string]json.RawMessage `json:"sections"`
}

type Event struct {
	Sequence  uint64          `json:"sequence"`
	Gap       bool            `json:"gap,omitempty"`
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

	externalAuthority bool
	locallyWritable   map[string]struct{}
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

func (store *Store) Revision() uint64 {
	if store == nil {
		return 0
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.snapshot.Revision
}

func (store *Store) Section(name string) (json.RawMessage, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	value, ok := store.snapshot.Sections[name]
	return cloneRaw(value), ok
}

// SetExternalAuthority makes portable sections read-only at the store
// boundary. This closes races where a request passed a higher-level API or
// intent check immediately before hosted reconciliation changed authority.
func (store *Store) SetExternalAuthority(enabled bool, locallyWritable ...string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	store.externalAuthority = enabled
	store.locallyWritable = nil
	if enabled {
		store.locallyWritable = make(map[string]struct{}, len(locallyWritable))
		for _, section := range locallyWritable {
			section = strings.TrimSpace(section)
			if section != "" {
				store.locallyWritable[section] = struct{}{}
			}
		}
	}
	store.mu.Unlock()
}

func (store *Store) Update(section string, value json.RawMessage) (Snapshot, error) {
	return store.UpdateExpected(section, value, nil)
}

func (store *Store) UpdateExpected(section string, value json.RawMessage, expectedRevision *uint64) (Snapshot, error) {
	return store.UpdateConditional(section, value, expectedRevision, nil)
}

func (store *Store) UpdateMany(sections map[string]json.RawMessage) (Snapshot, []string, error) {
	canonical := make(map[string]json.RawMessage, len(sections))
	names := make([]string, 0, len(sections))
	for section, value := range sections {
		normalized, err := canonicalSection(section, value)
		if err != nil {
			return Snapshot{}, nil, err
		}
		canonical[section] = normalized
		names = append(names, section)
	}
	sort.Strings(names)

	store.mu.Lock()
	if err := store.requireWritableLocked(names...); err != nil {
		store.mu.Unlock()
		return Snapshot{}, nil, err
	}
	changed := make([]string, 0, len(names))
	next := cloneSnapshot(store.snapshot)
	for _, section := range names {
		value := canonical[section]
		if current, exists := store.snapshot.Sections[section]; exists && bytes.Equal(current, value) {
			continue
		}
		next.Sections[section] = value
		changed = append(changed, section)
	}
	if len(changed) == 0 {
		snapshot := cloneSnapshot(store.snapshot)
		store.mu.Unlock()
		return snapshot, nil, nil
	}

	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	if err := writeSnapshot(store.path, next); err != nil {
		store.mu.Unlock()
		return Snapshot{}, nil, err
	}
	store.snapshot = next
	event := Event{
		Sequence: next.Revision, Revision: next.Revision, UpdatedAt: next.UpdatedAt,
		Snapshot: cloneSnapshot(next),
	}
	if len(changed) == 1 {
		event.Section = changed[0]
		event.Value = cloneRaw(canonical[changed[0]])
	} else {
		event.Section = "*"
		event.Gap = true
	}
	store.publishLocked(event)
	snapshot := cloneSnapshot(next)
	store.mu.Unlock()
	return snapshot, changed, nil
}

// ReplaceAllAuthoritative atomically makes sections the complete desired
// document while preserving the named installation-local sections under the
// same store lock. It is the sole hosted control-plane write path.
func (store *Store) ReplaceAllAuthoritative(
	sections map[string]json.RawMessage,
	preserve ...string,
) (Snapshot, []string, error) {
	canonical := make(map[string]json.RawMessage, len(sections))
	for section, value := range sections {
		normalized, err := canonicalSection(section, value)
		if err != nil {
			return Snapshot{}, nil, err
		}
		canonical[section] = normalized
	}

	store.mu.Lock()
	for _, section := range preserve {
		if current, exists := store.snapshot.Sections[section]; exists {
			canonical[section] = cloneRaw(current)
		}
	}
	changedSet := make(map[string]struct{}, len(store.snapshot.Sections)+len(canonical))
	for section, current := range store.snapshot.Sections {
		if desired, exists := canonical[section]; !exists || !bytes.Equal(current, desired) {
			changedSet[section] = struct{}{}
		}
	}
	for section, desired := range canonical {
		if current, exists := store.snapshot.Sections[section]; !exists || !bytes.Equal(current, desired) {
			changedSet[section] = struct{}{}
		}
	}
	if len(changedSet) == 0 {
		snapshot := cloneSnapshot(store.snapshot)
		store.mu.Unlock()
		return snapshot, nil, nil
	}

	changed := make([]string, 0, len(changedSet))
	for section := range changedSet {
		changed = append(changed, section)
	}
	sort.Strings(changed)
	next := cloneSnapshot(store.snapshot)
	next.Sections = make(map[string]json.RawMessage, len(canonical))
	for section, value := range canonical {
		next.Sections[section] = cloneRaw(value)
	}
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	if err := writeSnapshot(store.path, next); err != nil {
		store.mu.Unlock()
		return Snapshot{}, nil, err
	}
	store.snapshot = next
	store.publishLocked(Event{
		Sequence: next.Revision, Gap: true, Revision: next.Revision,
		Section: "*", UpdatedAt: next.UpdatedAt, Snapshot: cloneSnapshot(next),
	})
	snapshot := cloneSnapshot(next)
	store.mu.Unlock()
	return snapshot, changed, nil
}

func (store *Store) UpdateConditional(
	section string,
	value json.RawMessage,
	expectedRevision *uint64,
	expectedValue *json.RawMessage,
) (Snapshot, error) {
	canonical, err := canonicalSection(section, value)
	if err != nil {
		return Snapshot{}, err
	}
	var expectedCanonical json.RawMessage
	if expectedValue != nil {
		expectedCanonical, err = canonicalSection(section, *expectedValue)
		if err != nil {
			return Snapshot{}, fmt.Errorf("expected configuration %s: %w", section, err)
		}
	}
	store.mu.Lock()
	if err := store.requireWritableLocked(section); err != nil {
		store.mu.Unlock()
		return Snapshot{}, err
	}
	if expectedRevision != nil && store.snapshot.Revision != *expectedRevision {
		actual := store.snapshot.Revision
		store.mu.Unlock()
		return Snapshot{}, fmt.Errorf("configuration revision changed: expected %d, current %d", *expectedRevision, actual)
	}
	current, exists := store.snapshot.Sections[section]
	if expectedValue != nil && (!exists || !bytes.Equal(current, expectedCanonical)) {
		store.mu.Unlock()
		return Snapshot{}, fmt.Errorf("configuration section %q changed", section)
	}
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
		Sequence: next.Revision, Revision: next.Revision, Section: section, Value: cloneRaw(canonical),
		UpdatedAt: next.UpdatedAt, Snapshot: cloneSnapshot(next),
	}
	store.publishLocked(event)
	snapshot := cloneSnapshot(next)
	store.mu.Unlock()
	return snapshot, nil
}

func (store *Store) requireWritableLocked(sections ...string) error {
	if !store.externalAuthority {
		return nil
	}
	for _, section := range sections {
		if _, allowed := store.locallyWritable[section]; !allowed {
			return fmt.Errorf("%w: section %q", ErrExternalAuthority, section)
		}
	}
	return nil
}

func (store *Store) publishLocked(event Event) {
	for _, subscriber := range store.subscribers {
		delivery := event
		select {
		case subscriber <- delivery:
			continue
		default:
		}
		select {
		case <-subscriber:
			delivery.Gap = true
		default:
		}
		subscriber <- delivery
	}
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
