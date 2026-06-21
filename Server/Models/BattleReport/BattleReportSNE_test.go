package battlereport

import (
	"encoding/json"
	"testing"
)

func TestCapturesFromSNEPayloadStoresOnlyMatchingMessageRow(t *testing.T) {
	const payload = `{
		"TS": 123,
		"MSG": [
			[2150313880, 6, "2+0+0#0+-203", "", -1, 40, 0, 0, 0],
			[2150313861, 6, "2+0+0#1+-220", "", -1, 45, 0, 0, 0],
			[2150313860, 2, "not+a+battle", "", -1, 45, 0, 0, 0]
		]
	}`

	captures, err := CapturesFromSNEPayload(payload)
	if err != nil {
		t.Fatalf("CapturesFromSNEPayload() error = %v", err)
	}
	if len(captures) != 2 {
		t.Fatalf("len(captures) = %d, want 2", len(captures))
	}
	for _, capture := range captures {
		rows, ok := capture.SNE["MSG"].([]interface{})
		if !ok {
			t.Fatalf("capture %d SNE MSG missing or wrong type", capture.MID)
		}
		if len(rows) != 1 {
			t.Fatalf("capture %d SNE MSG len = %d, want 1", capture.MID, len(rows))
		}
		row, ok := rows[0].([]interface{})
		if !ok {
			t.Fatalf("capture %d SNE MSG row wrong type", capture.MID)
		}
		rowMID, ok := int64FromValue(rowValue(row, 0))
		if !ok || rowMID != capture.MID {
			t.Fatalf("capture %d row MID = %d (ok=%t)", capture.MID, rowMID, ok)
		}
		if capture.SNE["TS"] != float64(123) {
			t.Fatalf("capture %d compact SNE did not preserve TS", capture.MID)
		}
	}
}

func TestCompactSNEForCaptureNarrowsExistingFullMessageList(t *testing.T) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(`{
		"MSG": [
			[2150313880, 6, "2+0+0#0+-203", "", -1, 40, 0, 0, 0],
			[2150313861, 6, "2+0+0#1+-220", "", -1, 45, 0, 0, 0]
		]
	}`), &root); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	capture := Capture{
		MID: 2150313861,
		SNE: root,
	}

	compact := compactSNEForCapture(capture, capture.SNE)
	rows, ok := compact["MSG"].([]interface{})
	if !ok {
		t.Fatal("compact SNE MSG missing or wrong type")
	}
	if len(rows) != 1 {
		t.Fatalf("compact SNE MSG len = %d, want 1", len(rows))
	}
	row, ok := rows[0].([]interface{})
	if !ok {
		t.Fatal("compact SNE MSG row wrong type")
	}
	rowMID, ok := int64FromValue(rowValue(row, 0))
	if !ok || rowMID != capture.MID {
		t.Fatalf("compact row MID = %d (ok=%t), want %d", rowMID, ok, capture.MID)
	}
}
