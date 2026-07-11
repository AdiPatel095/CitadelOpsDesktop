package reportnotice

import (
	"encoding/json"
	"testing"
)

func TestIsSpyFetchableRow(t *testing.T) {
	maxSeconds := int64(MaxSpyFetchAge.Seconds())
	tests := []struct {
		name string
		row  []interface{}
		want bool
	}{
		{name: "live", row: []interface{}{1, 6, "key", "", -1, 0}, want: true},
		{name: "under six hours", row: []interface{}{1, 3, "key", "", -1, maxSeconds - 1}, want: true},
		{name: "six hours old", row: []interface{}{1, 3, "key", "", -1, maxSeconds}, want: false},
		{name: "json number", row: []interface{}{1, 6, "key", "", -1, json.Number("60")}, want: true},
		{name: "missing age", row: []interface{}{1, 6}, want: false},
		{name: "invalid age", row: []interface{}{1, 6, "key", "", -1, "old"}, want: false},
		{name: "negative age", row: []interface{}{1, 6, "key", "", -1, -1}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsSpyFetchableRow(test.row); got != test.want {
				t.Fatalf("IsSpyFetchableRow() = %t, want %t", got, test.want)
			}
		})
	}
}
