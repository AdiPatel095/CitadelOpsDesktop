package GameParser

import "encoding/json"

// gcaJSONInt coerces JSON-decoded numbers (float64, json.Number, etc.) to int.
func gcaJSONInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
