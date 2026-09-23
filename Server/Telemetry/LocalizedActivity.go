package Telemetry

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"strings"
)

// ActivityEntry is one indivisible persisted record, never an index sidecar.
// Raw legacy lines remain untranslated. Diagnostic channels stay plaintext.
type ActivityEntry struct {
	Line              string                `json:"line"`
	MessageDescriptor *Localization.Message `json:"messageDescriptor,omitempty"`
	TranslationStatus string                `json:"translationStatus"`
}

type activityRecord struct {
	Schema string `json:"schema"`
	ActivityEntry
}

const activityRecordSchema = "citadel.activity.v1"

func encodeActivityRecord(line string, descriptor *Localization.Message) string {
	if descriptor == nil {
		return line
	}
	record := activityRecord{Schema: activityRecordSchema, ActivityEntry: ActivityEntry{Line: line, MessageDescriptor: Localization.Clone(descriptor), TranslationStatus: Localization.Status(descriptor)}}
	raw, err := json.Marshal(record)
	if err != nil {
		return line
	}
	return string(raw)
}

func decodeActivityRecord(raw string) ActivityEntry {
	legacy := ActivityEntry{Line: raw, TranslationStatus: "untranslated"}
	if !strings.HasPrefix(raw, `{"schema":"`+activityRecordSchema+`"`) {
		return legacy
	}
	var record activityRecord
	if json.Unmarshal([]byte(raw), &record) != nil || record.Schema != activityRecordSchema || record.Line == "" {
		return ActivityEntry{TranslationStatus: "untranslated"}
	}
	if Localization.Validate(record.MessageDescriptor) != nil {
		record.MessageDescriptor = nil
	}
	record.MessageDescriptor = Localization.Clone(record.MessageDescriptor)
	record.TranslationStatus = Localization.Status(record.MessageDescriptor)
	return record.ActivityEntry
}

func (store *Store) UserFacingEntries(channel string, limit int) ([]ActivityEntry, bool) {
	if store == nil || !isKnownChannel(channel) || isDiagnosticChannel(channel) {
		return []ActivityEntry{}, false
	}
	records := store.tailRecords(channel, limit)
	entries := make([]ActivityEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, decodeActivityRecord(record))
	}
	return entries, true
}
