package Configuration

import (
	"CitadelDesktop/Server/Localization"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion   = 1
	maxSectionBytes = 1 << 20
)

var ErrExternalAuthority = Localization.WithError(errors.New("configuration is owned by an external authority"), Localization.New("server.configuration.configuration_is_owned_by.368c6b6f", "configuration is owned by an external authority", nil))
var ErrInvalidUpdate = Localization.WithError(errors.New("configuration update is invalid"), Localization.New("server.configuration.configuration_update_is_invalid.f6edf5c1", "configuration update is invalid", nil))

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
		return nil, Localization.WithError(fmt.Errorf("configuration data directory is required"), Localization.New("server.configuration.configuration_data_directory_is.49ba09b5", "configuration data directory is required", nil))
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
			return nil, Localization.WithError(fmt.Errorf("decode configuration: %w", err), Localization.ErrorContext(Localization.New("server.configuration.decode_configuration.7759a241", "decode configuration", nil), err))
		}
		if snapshot.SchemaVersion != SchemaVersion {
			return nil, Localization.WithError(fmt.Errorf("unsupported configuration schema %d", snapshot.SchemaVersion), Localization.New("server.configuration.unsupported_configuration_schema_p.fbf30d52", "unsupported configuration schema {p0}", Localization.Params{"p0": snapshot.SchemaVersion}))
		}
		if snapshot.Sections == nil {
			snapshot.Sections = map[string]json.RawMessage{}
		}
		if snapshot.UpdatedAt.IsZero() {
			snapshot.UpdatedAt = now
		}
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, Localization.WithError(fmt.Errorf("read configuration: %w", err), Localization.ErrorContext(Localization.New("server.configuration.read_configuration.4e4d571b", "read configuration", nil), err))
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
			return nil, Localization.WithError(fmt.Errorf("default configuration %s: %w", section, err), Localization.ErrorContext(Localization.New("server.configuration.default_configuration_p.54949bb9", "default configuration {p0}", Localization.Params{"p0": fmt.Sprintf("%s", section)}), err))
		}
		snapshot.Sections[section] = canonical
		changed = true
	}
	for section, value := range snapshot.Sections {
		canonical, err := canonicalSection(section, value)
		if err != nil {
			return nil, Localization.WithError(fmt.Errorf("configuration %s: %w", section, err), Localization.ErrorContext(Localization.New("server.configuration.configuration_p.65a7bf22", "configuration {p0}", Localization.Params{"p0": fmt.Sprintf("%s", section)}), err))
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
		if err := ValidateUpdate(section, value, store.snapshot.Sections[section]); err != nil {
			store.mu.Unlock()
			return Snapshot{}, nil, err
		}
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
			return Snapshot{}, Localization.WithError(fmt.Errorf("expected configuration %s: %w", section, err), Localization.ErrorContext(Localization.New("server.configuration.expected_configuration_p.2e3d4afd", "expected configuration {p0}", Localization.Params{"p0": fmt.Sprintf("%s", section)}), err))
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
		return Snapshot{}, Localization.WithError(fmt.Errorf("configuration revision changed: expected %d, current %d", *expectedRevision, actual), Localization.New("server.configuration.configuration_revision_changed_expected.60547a34", "configuration revision changed: expected {p0}, current {p1}", Localization.Params{"p0": *expectedRevision, "p1": actual}))
	}
	current, exists := store.snapshot.Sections[section]
	if expectedValue != nil && (!exists || !bytes.Equal(current, expectedCanonical)) {
		store.mu.Unlock()
		return Snapshot{}, Localization.WithError(fmt.Errorf("configuration section %q changed", section), Localization.New("server.configuration.configuration_section_p_changed.2747f4e3", "configuration section {p0} changed", Localization.Params{"p0": fmt.Sprintf("%q", section)}))
	}
	if err := ValidateUpdate(section, canonical, current); err != nil {
		store.mu.Unlock()
		return Snapshot{}, err
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
			return Localization.WithError(fmt.Errorf("%w: section %q", ErrExternalAuthority, section), Localization.New("server.configuration.configuration_is_owned_by.dcd1b417", "configuration is owned by an external authority: section {p1}", Localization.Params{"p1": fmt.Sprintf("%q", section)}))
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
	if _, err := canonicalSection(section, value); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate applies user-write policy relative to the current saved
// value. Existing invalid feast settings remain loadable and can be preserved
// during unrelated edits or disabled, while newly enabled or changed feast
// goals must carry a safe whole-hour target.
func ValidateUpdate(section string, proposed, current json.RawMessage) error {
	if section != "automation.autoBuyer" {
		return nil
	}
	type feastSettings struct {
		Enabled               bool            `json:"enabled"`
		MinimumRemainingHours json.RawMessage `json:"minimumRemainingHours"`
	}
	var proposedDocument struct {
		Feast json.RawMessage `json:"feast"`
	}
	if err := json.Unmarshal(proposed, &proposedDocument); err != nil {
		return fmt.Errorf("%w: decode Auto Buyer settings: %v", ErrInvalidUpdate, err)
	}
	var proposedFeast feastSettings
	if len(proposedDocument.Feast) == 0 || json.Unmarshal(proposedDocument.Feast, &proposedFeast) != nil || !proposedFeast.Enabled {
		return nil
	}
	var hours int
	if err := json.Unmarshal(proposedFeast.MinimumRemainingHours, &hours); err == nil && hours >= 1 && hours <= 720 {
		return nil
	}
	var currentDocument struct {
		Feast json.RawMessage `json:"feast"`
	}
	if len(current) > 0 && json.Unmarshal(current, &currentDocument) == nil &&
		jsonValuesEqual(proposedDocument.Feast, currentDocument.Feast) {
		return nil
	}
	return Localization.WithError(fmt.Errorf("%w: feast minimum remaining hours must be a whole number from 1 to 720", ErrInvalidUpdate), Localization.New("server.configuration.configuration_update_is_invalid.e178c459", "configuration update is invalid: feast minimum remaining hours must be a whole number from 1 to 720", nil))
}

func jsonValuesEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
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
		return nil, Localization.WithError(fmt.Errorf("invalid configuration section %q", section), Localization.New("server.configuration.invalid_configuration_section_p.471cc72a", "invalid configuration section {p0}", Localization.Params{"p0": fmt.Sprintf("%q", section)}))
	}
	if len(value) == 0 || len(value) > maxSectionBytes {
		return nil, Localization.WithError(fmt.Errorf("configuration section %q must contain 1 to %d bytes", section, maxSectionBytes), Localization.New("server.configuration.configuration_section_p_must.3ae33460", "configuration section {p0} must contain 1 to {p1} bytes", Localization.Params{"p0": fmt.Sprintf("%q", section), "p1": maxSectionBytes}))
	}
	if !json.Valid(value) {
		return nil, Localization.WithError(fmt.Errorf("configuration section %q is not valid JSON", section), Localization.New("server.configuration.configuration_section_p_is.f52b7d28", "configuration section {p0} is not valid JSON", Localization.Params{"p0": fmt.Sprintf("%q", section)}))
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
		return Localization.WithError(fmt.Errorf("create configuration directory: %w", err), Localization.ErrorContext(Localization.New("server.configuration.create_configuration_directory.f59ff1c7", "create configuration directory", nil), err))
	}
	contents, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return Localization.WithError(fmt.Errorf("encode configuration: %w", err), Localization.ErrorContext(Localization.New("server.configuration.encode_configuration.79b65cde", "encode configuration", nil), err))
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, ".Settings-*")
	if err != nil {
		return Localization.WithError(fmt.Errorf("create configuration file: %w", err), Localization.ErrorContext(Localization.New("server.configuration.create_configuration_file.c65e583b", "create configuration file", nil), err))
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
			return Localization.WithError(fmt.Errorf("save configuration: %w", retryErr), Localization.ErrorContext(Localization.New("server.configuration.save_configuration.2d5c4c94", "save configuration", nil), retryErr))
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
