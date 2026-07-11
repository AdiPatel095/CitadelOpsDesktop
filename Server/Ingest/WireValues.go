package Ingest

import (
	"bytes"
	"encoding/json"
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
	integer, err := strconv.ParseInt(string(raw), 10, 64)
	if err == nil {
		return integer, true
	}
	number, floatErr := strconv.ParseFloat(string(raw), 64)
	return int64(number), floatErr == nil
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
	if index < 0 || index >= len(row) {
		return 0
	}
	value, _ := rawInt64(row[index])
	return value
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
