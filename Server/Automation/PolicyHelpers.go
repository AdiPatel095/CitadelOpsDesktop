package Automation

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
)

func decodeSection(snapshot Configuration.Snapshot, section string, destination any) bool {
	raw := snapshot.Sections[section]
	return len(raw) > 0 && json.Unmarshal(raw, destination) == nil
}

func policyInterval(seconds int, fallback int) time.Duration {
	if seconds < 30 || seconds > 86_400 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

func sortedNumericKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		leftID, _ := strconv.ParseInt(keys[left], 10, 64)
		rightID, _ := strconv.ParseInt(keys[right], 10, 64)
		return leftID < rightID
	})
	return keys
}

func recordNumber(store *GameData.Store, collection string, id int64, field string) (float64, bool) {
	if store == nil || id <= 0 {
		return 0, false
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return 0, false
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	return record.Float64(field)
}

func recordInteger(store *GameData.Store, collection string, id int64, field string) int64 {
	value, ok := recordNumber(store, collection, id, field)
	if !ok {
		return 0
	}
	return int64(value)
}

func commaSeparatedIDs(value string) map[int64]struct{} {
	result := map[int64]struct{}{}
	for _, part := range strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '#'
	}) {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}
