package Diagnostics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProcessSampleCachedAndAggregate(t *testing.T) {
	a, b := SampleProcess(), SampleProcess()
	if a != b || a.Scope != "process" || a.ObservedAt.IsZero() || a.GoMemoryLimit == 0 || a.HeapSys < a.HeapInuse {
		t.Fatalf("invalid process snapshot: %+v", a)
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"account", "payload", "profile", "path", "token"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatal("private label present")
		}
	}
}
