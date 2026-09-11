package Ingest

import (
	"encoding/json"
	"testing"
)

func TestRawInt64RequiresExactIntegralInRangeValue(t *testing.T) {
	tests := []struct {
		raw   string
		value int64
		valid bool
	}{
		{raw: "3", value: 3, valid: true},
		{raw: "3.0", value: 3, valid: true},
		{raw: "3e2", value: 300, valid: true},
		{raw: `"3"`, value: 3, valid: true},
		{raw: "0.9", valid: false},
		{raw: "1e-1", valid: false},
		{raw: "9223372036854775808", valid: false},
		{raw: "-9223372036854775809", valid: false},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			value, valid := rawInt64(json.RawMessage(test.raw))
			if value != test.value || valid != test.valid {
				t.Fatalf("rawInt64(%s) = (%d, %t), want (%d, %t)", test.raw, value, valid, test.value, test.valid)
			}
		})
	}
}
