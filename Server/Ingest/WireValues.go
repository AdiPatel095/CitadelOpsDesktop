package Ingest

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
)

type wireInt64 int64

func (value *wireInt64) UnmarshalJSON(raw []byte) error {
	integer, ok := rawInt64(raw)
	if !ok {
		*value = 0
		return nil
	}
	*value = wireInt64(integer)
	return nil
}

type wireFloat64 float64

func (value *wireFloat64) UnmarshalJSON(raw []byte) error {
	number, ok := rawFloat64(raw)
	if !ok {
		*value = 0
		return nil
	}
	*value = wireFloat64(number)
	return nil
}

func rawInt64(raw json.RawMessage) (int64, bool) {
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
	if integer, ok := plainInt64(raw); ok {
		return integer, true
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

// plainInt64 parses a plain JSON integer literal, -?(0|[1-9][0-9]*) with at most
// 19 digits, straight from its bytes. It reports false for everything else
// (exponents, decimals, leading zeros, a bare sign, out-of-range values, any
// other byte) and rawInt64 then applies its exact-rational path, so results for
// those forms are unchanged. Every input it accepts is a valid JSON number that
// the exact path would convert to the same int64.
func plainInt64(raw []byte) (int64, bool) {
	digits := raw
	negative := len(raw) > 0 && raw[0] == '-'
	if negative {
		digits = raw[1:]
	}
	if len(digits) == 0 || len(digits) > 19 || (digits[0] == '0' && len(digits) > 1) {
		return 0, false
	}
	var magnitude uint64
	for _, character := range digits {
		if character < '0' || character > '9' {
			return 0, false
		}
		magnitude = magnitude*10 + uint64(character-'0')
	}
	// Nineteen digits cannot overflow uint64; only int64's range remains to check.
	if negative {
		if magnitude > 1<<63 {
			return 0, false
		}
		return int64(-magnitude), true
	}
	if magnitude > 1<<63-1 {
		return 0, false
	}
	return int64(magnitude), true
}

// rawJSONInt64 accepts only an integral JSON number. Some legacy payloads use
// quoted numeric strings, which rawInt64 intentionally tolerates; protocol
// identity fields must not silently accept that type mismatch.
func rawJSONInt64(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] == '"' {
		return 0, false
	}
	return rawInt64(raw)
}

func rawFloat64(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return 0, false
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		return number, err == nil
	}
	number, err := strconv.ParseFloat(string(raw), 64)
	return number, err == nil
}

func rowInt(row []json.RawMessage, index int) int64 {
	value, _ := rowIntValue(row, index)
	return value
}

func rowIntValue(row []json.RawMessage, index int) (int64, bool) {
	if index < 0 || index >= len(row) {
		return 0, false
	}
	return rawInt64(row[index])
}

func rowExactInt(row []json.RawMessage, index int) (int, bool) {
	if index < 0 || index >= len(row) {
		return 0, false
	}
	value, ok := rawJSONInt64(row[index])
	if !ok {
		return 0, false
	}
	converted := int(value)
	return converted, int64(converted) == value
}

func rowString(row []json.RawMessage, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	var value string
	_ = json.Unmarshal(row[index], &value)
	return value
}

func decodeRows(raw json.RawMessage) ([][]json.RawMessage, bool) {
	var rows [][]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &rows) != nil {
		return nil, false
	}
	return rows, true
}

func decodeUnitCounts(raw json.RawMessage) map[int64]int64 {
	result := map[int64]int64{}
	rows, ok := decodeRows(raw)
	if !ok {
		return result
	}
	for _, row := range rows {
		definitionID := rowInt(row, 0)
		amount := rowInt(row, 1)
		if definitionID > 0 && amount >= 0 {
			result[definitionID] += amount
		}
	}
	return result
}

func floatPointer(value float64) *float64 {
	copy := value
	return &copy
}

func intPointer(value int) *int {
	copy := value
	return &copy
}
