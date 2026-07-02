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

func TestParseSPLObjectPIDLHospitalQueue(t *testing.T) {
	raw := map[string]interface{}{
		"LID": float64(2),
		"PIDL": []interface{}{
			[]interface{}{float64(238), float64(5), float64(323), float64(1378), float64(0), float64(1374447446), float64(0), float64(-1)},
			[]interface{}{float64(493), float64(1), float64(74), float64(1378), float64(0), float64(841860786), float64(0), float64(-1)},
			[]interface{}{float64(-1), float64(0), float64(0), float64(0), float64(0), float64(0), float64(0), float64(-1)},
		},
	}

	q, ok := parseSPLObject(raw)
	if !ok {
		t.Fatal("parseSPLObject returned ok=false")
	}
	if q.LID != 2 {
		t.Fatalf("LID = %d, want 2", q.LID)
	}
	if q.QueueCapacity != 3 {
		t.Fatalf("QueueCapacity = %d, want 3", q.QueueCapacity)
	}
	if len(q.Queued) != 2 {
		t.Fatalf("len(Queued) = %d, want 2", len(q.Queued))
	}
	if q.Queued[0].WID != 238 || q.Queued[0].TUA != 5 || q.Queued[0].PID != 1374447446 {
		t.Fatalf("Queued[0] = %+v, want WID=238 TUA=5 PID=1374447446", q.Queued[0])
	}
}
