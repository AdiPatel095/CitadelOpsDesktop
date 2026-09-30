package Ingest

import (
	"bytes"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
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

// rawInt64Reference is the exact-rational implementation that rawInt64 used
// before the plain-integer fast path; the fast path must never disagree with it.
func rawInt64Reference(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return 0, false
		}
		integer, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		return integer, err == nil
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return 0, false
	}
	rational, ok := new(big.Rat).SetString(number.String())
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, false
	}
	return rational.Num().Int64(), true
}

func TestRawInt64FastPathTable(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		value int64
		valid bool
		plain bool // handled by the byte-level fast path
	}{
		{name: "zero", raw: "0", value: 0, valid: true, plain: true},
		{name: "negative zero", raw: "-0", value: 0, valid: true, plain: true},
		{name: "single digit", raw: "7", value: 7, valid: true, plain: true},
		{name: "multi digit", raw: "845", value: 845, valid: true, plain: true},
		{name: "negative", raw: "-1002", value: -1002, valid: true, plain: true},
		{name: "epoch milliseconds", raw: "1790000000000", value: 1790000000000, valid: true, plain: true},
		{name: "max int64", raw: "9223372036854775807", value: math.MaxInt64, valid: true, plain: true},
		{name: "min int64", raw: "-9223372036854775808", value: math.MinInt64, valid: true, plain: true},
		{name: "max int64 plus one", raw: "9223372036854775808"},
		{name: "min int64 minus one", raw: "-9223372036854775809"},
		{name: "nineteen nines overflow", raw: "9999999999999999999"},
		{name: "twenty digits overflow", raw: "10000000000000000000"},
		{name: "huge", raw: "123456789012345678901234567890"},
		{name: "leading zero", raw: "007"},
		{name: "leading zero negative", raw: "-007"},
		{name: "double zero", raw: "00"},
		{name: "plus sign", raw: "+7"},
		{name: "bare minus", raw: "-"},
		{name: "double minus", raw: "--7"},
		{name: "exponent", raw: "3e2", value: 300, valid: true},
		{name: "upper exponent", raw: "3E2", value: 300, valid: true},
		{name: "signed exponent", raw: "3e+2", value: 300, valid: true},
		{name: "negative exponent integral", raw: "300e-2", value: 3, valid: true},
		{name: "negative exponent fractional", raw: "1e-1"},
		{name: "exponent overflow", raw: "1e19"},
		{name: "decimal integral", raw: "3.0", value: 3, valid: true},
		{name: "decimal negative integral", raw: "-3.00", value: -3, valid: true},
		{name: "decimal fractional", raw: "0.9"},
		{name: "decimal fractional negative", raw: "-2.5"},
		{name: "trailing dot", raw: "12."},
		{name: "leading dot", raw: ".5"},
		{name: "negative zero decimal", raw: "-0.0", value: 0, valid: true},
		{name: "quoted", raw: `"3"`, value: 3, valid: true},
		{name: "quoted negative", raw: `"-12"`, value: -12, valid: true},
		{name: "quoted padded", raw: `" 12 "`, value: 12, valid: true},
		{name: "quoted plus", raw: `"+5"`, value: 5, valid: true},
		{name: "quoted leading zeros", raw: `"007"`, value: 7, valid: true},
		{name: "quoted decimal", raw: `"3.0"`},
		{name: "quoted exponent", raw: `"3e2"`},
		// Unchanged legacy quirk: the quoted path returns strconv's clamped value alongside ok=false.
		{name: "quoted overflow", raw: `"9223372036854775808"`, value: math.MaxInt64},
		{name: "quoted text", raw: `"abc"`},
		{name: "quoted empty", raw: `""`},
		{name: "null", raw: "null"},
		{name: "empty", raw: ""},
		{name: "whitespace only", raw: "  \n"},
		{name: "surrounding whitespace", raw: " \t12\n", value: 12, valid: true, plain: true},
		{name: "surrounding whitespace negative", raw: "\n-5 ", value: -5, valid: true, plain: true},
		{name: "interior whitespace", raw: "1 2"},
		{name: "trailing comma", raw: "12,"},
		{name: "trailing text", raw: "12abc"},
		{name: "hex", raw: "0x10"},
		{name: "boolean", raw: "true"},
		{name: "array", raw: "[1]"},
		{name: "object", raw: "{}"},
		{name: "non-ascii digits", raw: "١٢"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(test.raw)
			value, valid := rawInt64(raw)
			if value != test.value || valid != test.valid {
				t.Fatalf("rawInt64(%q) = (%d, %t), want (%d, %t)", test.raw, value, valid, test.value, test.valid)
			}
			if referenceValue, referenceValid := rawInt64Reference(raw); referenceValue != value || referenceValid != valid {
				t.Fatalf("rawInt64(%q) = (%d, %t) but the exact-rational implementation gives (%d, %t)",
					test.raw, value, valid, referenceValue, referenceValid)
			}
			_, plain := plainInt64(bytes.TrimSpace(raw))
			if plain != test.plain {
				t.Fatalf("plainInt64(%q) handled = %t, want %t", test.raw, plain, test.plain)
			}
		})
	}
}

// Every short string over an alphabet of digit, sign, decimal, exponent, quote and
// space characters must give the same answer as the exact-rational implementation.
func TestRawInt64FastPathAgreesWithExactImplementation(t *testing.T) {
	alphabet := []byte("019-+.eE\" x")
	var build func(prefix []byte, remaining int)
	checked := 0
	build = func(prefix []byte, remaining int) {
		if len(prefix) > 0 {
			raw := json.RawMessage(prefix)
			gotValue, gotValid := rawInt64(raw)
			wantValue, wantValid := rawInt64Reference(raw)
			if gotValue != wantValue || gotValid != wantValid {
				t.Fatalf("rawInt64(%q) = (%d, %t), exact implementation gives (%d, %t)", prefix, gotValue, gotValid, wantValue, wantValid)
			}
			checked++
		}
		if remaining == 0 {
			return
		}
		for _, character := range alphabet {
			build(append(append([]byte(nil), prefix...), character), remaining-1)
		}
	}
	build(nil, 5)
	// Boundary magnitudes around every digit count and both int64 limits.
	for _, digits := range []string{"9", "99", "999999999999999999", "9223372036854775806", "9223372036854775807", "9223372036854775808", "9223372036854775809", "9999999999999999999"} {
		for _, sign := range []string{"", "-"} {
			raw := json.RawMessage(sign + digits)
			gotValue, gotValid := rawInt64(raw)
			wantValue, wantValid := rawInt64Reference(raw)
			if gotValue != wantValue || gotValid != wantValid {
				t.Fatalf("rawInt64(%q) = (%d, %t), exact implementation gives (%d, %t)", raw, gotValue, gotValid, wantValue, wantValid)
			}
			checked++
		}
	}
	if checked < 100_000 {
		t.Fatalf("only %d inputs compared", checked)
	}
}

func TestRawJSONInt64StillRejectsQuotedNumbers(t *testing.T) {
	if value, ok := rawJSONInt64(json.RawMessage(`"5"`)); ok || value != 0 {
		t.Fatalf("rawJSONInt64 accepted a quoted number: (%d, %t)", value, ok)
	}
	if value, ok := rawJSONInt64(json.RawMessage("5")); !ok || value != 5 {
		t.Fatalf("rawJSONInt64(5) = (%d, %t)", value, ok)
	}
}

func TestWireInt64UnmarshalUsesTheSameParse(t *testing.T) {
	var values []wireInt64
	if err := json.Unmarshal([]byte(`[12,-3,"7",1e2,2.5,null,"x",9223372036854775808]`), &values); err != nil {
		t.Fatal(err)
	}
	want := []wireInt64{12, -3, 7, 100, 0, 0, 0, 0}
	if len(values) != len(want) {
		t.Fatalf("decoded %v", values)
	}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("value %d = %d, want %d (all: %v)", index, values[index], want[index], values)
		}
	}
}
