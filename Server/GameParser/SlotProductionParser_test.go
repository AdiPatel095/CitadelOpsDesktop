package GameParser

import "testing"

func TestParseSPLObjectCountsOnlyUsableQueueSlots(t *testing.T) {
	raw := map[string]interface{}{
		"LID": float64(0),
		"PS":  map[string]interface{}{},
		"QS": []interface{}{
			map[string]interface{}{"SI": map[string]interface{}{"RUT": float64(-1), "VIP": float64(0)}},
			map[string]interface{}{"SI": map[string]interface{}{"RUT": float64(-1), "VIP": float64(0)}},
			map[string]interface{}{"P": map[string]interface{}{"WID": float64(228), "TUA": float64(100)}, "SI": map[string]interface{}{"RUT": float64(53427), "VIP": float64(1)}},
			map[string]interface{}{"SI": map[string]interface{}{"RUT": float64(53427), "VIP": float64(1)}},
			map[string]interface{}{"SI": map[string]interface{}{"RUT": float64(0), "VIP": float64(0)}},
		},
	}

	q, ok := parseSPLObject(raw)
	if !ok {
		t.Fatal("parseSPLObject returned ok=false")
	}
	if q.QueueCapacity != 4 {
		t.Fatalf("QueueCapacity = %d, want 4", q.QueueCapacity)
	}
	if q.VIPSlots != 2 {
		t.Fatalf("VIPSlots = %d, want 2", q.VIPSlots)
	}
	if len(q.Queued) != 1 {
		t.Fatalf("len(Queued) = %d, want 1", len(q.Queued))
	}
}
