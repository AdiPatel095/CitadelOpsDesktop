package contractfill

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFillDeterministicCompleteValues(t *testing.T) {
	type child struct {
		Name string `json:"name"`
	}
	type sample struct {
		Text     string           `json:"text"`
		Flag     bool             `json:"flag"`
		Count    int              `json:"count"`
		Other    int              `json:"other"`
		Fraction float64          `json:"fraction"`
		Large    uint64           `json:"large"`
		Tiny     uint8            `json:"tiny"`
		At       time.Time        `json:"at"`
		Pointer  *child           `json:"pointer,omitempty"`
		List     []child          `json:"list"`
		Map      map[string]child `json:"map"`
		Raw      json.RawMessage  `json:"raw"`
		Ignored  string           `json:"-"`
	}
	var a, b sample
	for _, v := range []*sample{&a, &b} {
		if err := Fill(v, nil); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("non-deterministic filler")
	}
	if a.Text != "text" || !a.Flag || a.Count == 0 || a.Count == a.Other || math.Trunc(a.Fraction) == a.Fraction || a.Large <= 1<<63 || a.Tiny < 128 || a.At.Nanosecond() == 0 || a.Pointer == nil || a.Pointer.Name != "name" || len(a.List) != 1 || len(a.Map) != 1 || !json.Valid(a.Raw) || a.Ignored != "" {
		t.Fatalf("incomplete sample: %+v", a)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatal(err)
	}
}

func TestFillOverridesAndErrors(t *testing.T) {
	type sample struct {
		Window float64 `json:"cpuWindowSeconds"`
		Bytes  uint64  `json:"memoryLimitBytes"`
	}
	var value sample
	if err := Fill(&value, map[string]any{"cpuWindowSeconds": 20.030486354, "memoryLimitBytes": uint64(9223372036854775807)}); err != nil {
		t.Fatal(err)
	}
	if value.Window != 20.030486354 || value.Bytes != 9223372036854775807 {
		t.Fatal(value)
	}
	if err := Fill(&value, map[string]any{"cpuWindowSeconds": 20}); err == nil {
		t.Fatal("wrong-typed override accepted")
	}
	if err := Fill(value, nil); err == nil {
		t.Fatal("non-pointer accepted")
	}
}

func TestVerifyContractDrift(t *testing.T) {
	type nested struct {
		Value string `json:"value"`
	}
	type receiver struct {
		Required string   `json:"required"`
		Items    []nested `json:"items"`
	}
	keys := Keys(receiver{})
	good := []byte(`{"required":"x","items":[{"value":"x"}]}`)
	for _, test := range []struct {
		name     string
		raw      []byte
		sender   map[string]bool
		optional map[string]string
		want     string
	}{
		{"compatible", good, keys, nil, ""},
		{"renamed", []byte(`{"renamed":"x","items":[{"value":"x"}]}`), keys, nil, "missing receiver key required"},
		{"nested missing", []byte(`{"required":"x","items":[{}]}`), keys, nil, "missing receiver key items[].value"},
		{"sender dropped", good, map[string]bool{"required": false, "items": false}, nil, "sender does not declare receiver key items[].value"},
		{"sender optional", good, map[string]bool{"required": true, "items": false, "items[].value": false}, nil, "sender may omit required receiver key required"},
		{"reviewed optional", good, map[string]bool{"required": true, "items": false, "items[].value": false}, map[string]string{"required": "Compatibility default"}, ""},
		{"reason missing", good, keys, map[string]string{"required": " "}, "needs a reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := Verify(test.raw, receiver{}, test.sender, test.optional)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v; want %s", err, test.want)
			}
		})
	}
}
