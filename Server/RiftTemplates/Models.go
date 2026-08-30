package RiftTemplates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/State"
)

const (
	ConfigurationSection = "rift.templates"
	MaximumTemplates     = 15
	MaximumTombstones    = 4096
	MaximumDocumentBytes = 1 << 20
	MaximumBodyBytes     = 256 << 10
	MaximumLaunchIDBytes = 128
	MaximumDisplayName   = 80
)

// Document is the account-owned durable Rift replay catalog. Launches retain
// the complete captured record needed to replay after a worker replacement;
// tombstones prevent an older runtime snapshot from resurrecting a deletion.
type Document struct {
	Version          int                         `json:"version"`
	Launches         map[string]State.RiftLaunch `json:"launches"`
	DeletedLaunchIDs map[string]int64            `json:"deletedLaunchIds,omitempty"`
}

func EmptyDocument() Document {
	return Document{
		Version:          1,
		Launches:         map[string]State.RiftLaunch{},
		DeletedLaunchIDs: map[string]int64{},
	}
}

// Decode rejects malformed, future, oversized, or internally ambiguous
// documents. Wire-specific CRA validation remains in App because it needs the
// current account state before any command can be admitted.
func Decode(raw json.RawMessage) (Document, error) {
	if len(raw) == 0 {
		return EmptyDocument(), nil
	}
	if len(raw) > MaximumDocumentBytes {
		return Document{}, fmt.Errorf("Rift template document exceeds %d bytes", MaximumDocumentBytes)
	}
	document := EmptyDocument()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode Rift templates: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Document{}, fmt.Errorf("decode Rift templates: %w", err)
	}
	if document.Version != 1 {
		return Document{}, fmt.Errorf("unsupported Rift template document version %d", document.Version)
	}
	if document.Launches == nil {
		document.Launches = map[string]State.RiftLaunch{}
	}
	if document.DeletedLaunchIDs == nil {
		document.DeletedLaunchIDs = map[string]int64{}
	}
	if len(document.Launches) > MaximumTemplates {
		return Document{}, fmt.Errorf("Rift template document contains %d launches; at most %d are supported", len(document.Launches), MaximumTemplates)
	}
	if len(document.DeletedLaunchIDs) > MaximumTombstones {
		return Document{}, fmt.Errorf("Rift template document contains %d tombstones; at most %d are supported", len(document.DeletedLaunchIDs), MaximumTombstones)
	}
	for key, launch := range document.Launches {
		if err := validateLaunchID(key); err != nil {
			return Document{}, fmt.Errorf("Rift template key: %w", err)
		}
		if launch.ID != key {
			return Document{}, fmt.Errorf("Rift template %q record id %q does not match its key", key, launch.ID)
		}
		if err := validateLaunch(launch); err != nil {
			return Document{}, fmt.Errorf("Rift template %q: %w", key, err)
		}
		if _, deleted := document.DeletedLaunchIDs[key]; deleted {
			return Document{}, fmt.Errorf("Rift template %q is both active and deleted", key)
		}
	}
	for key, deletedAt := range document.DeletedLaunchIDs {
		if err := validateLaunchID(key); err != nil {
			return Document{}, fmt.Errorf("Rift tombstone key: %w", err)
		}
		if deletedAt <= 0 {
			return Document{}, fmt.Errorf("Rift tombstone %q must contain a positive deletion timestamp", key)
		}
	}
	return document, nil
}

func validateLaunch(launch State.RiftLaunch) error {
	if launch.SavedAtUnix <= 0 {
		return fmt.Errorf("savedAtUnix must be positive")
	}
	if len(launch.DisplayName) > MaximumDisplayName {
		return fmt.Errorf("displayName may contain at most %d bytes", MaximumDisplayName)
	}
	if strings.TrimSpace(launch.DisplayName) != launch.DisplayName {
		return fmt.Errorf("displayName must not contain leading or trailing whitespace")
	}
	if launch.CommanderID <= 0 {
		return fmt.Errorf("commanderID must be positive")
	}
	if launch.SourceX < 0 || launch.SourceY < 0 || launch.TargetX < 0 || launch.TargetY < 0 {
		return fmt.Errorf("source and target coordinates must be non-negative")
	}
	if launch.KingdomID < 0 {
		return fmt.Errorf("kingdomID must be non-negative")
	}
	if launch.AttackValid < 0 {
		return fmt.Errorf("attackValid must be non-negative")
	}
	if launch.WaveCount < 1 || launch.WaveCount > AttackPresets.MaximumWaves {
		return fmt.Errorf("waveCount must be between 1 and %d", AttackPresets.MaximumWaves)
	}
	if launch.OneWayTTSeconds < 0 || launch.LastSuccessAtUnix < 0 {
		return fmt.Errorf("travel and success timestamps must be non-negative")
	}
	if len(launch.Body) == 0 || len(launch.Body) > MaximumBodyBytes {
		return fmt.Errorf("body must contain 1 to %d bytes", MaximumBodyBytes)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(launch.Body, &body); err != nil || body == nil {
		return fmt.Errorf("body must be a valid JSON object")
	}
	return nil
}

func validateLaunchID(value string) error {
	if value == "" || len(value) > MaximumLaunchIDBytes || strings.TrimSpace(value) != value {
		return fmt.Errorf("launch id must contain 1 to %d bytes without surrounding whitespace", MaximumLaunchIDBytes)
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("launch id %q contains an unsupported character", value)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains more than one JSON value")
		}
		return err
	}
	return nil
}
