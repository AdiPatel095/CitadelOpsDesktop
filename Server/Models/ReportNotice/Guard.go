package reportnotice

import (
	"encoding/json"
	"time"
)

const MaxSpyFetchAge = 6 * time.Hour

// IsSpyFetchableRow rejects spy-report inbox notices older than the auto-fetch window.
// SNE row index 5 is the notice age in seconds; live notices use zero.
func IsSpyFetchableRow(row []interface{}) bool {
	if len(row) <= 5 {
		return false
	}
	ageSeconds, ok := int64FromValue(row[5])
	return ok && ageSeconds >= 0 && ageSeconds < int64(MaxSpyFetchAge/time.Second)
}

func int64FromValue(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
