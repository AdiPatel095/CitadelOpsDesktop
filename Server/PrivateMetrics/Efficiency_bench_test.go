package PrivateMetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// syntheticDashboardState builds a large dashboard-shaped state document so
// the digest cost is measured at a realistic order of magnitude (about 0.8 MB).
func syntheticDashboardState() json.RawMessage {
	castles := map[string]any{}
	for index := 0; index < 6000; index++ {
		castles[fmt.Sprintf("castle-%d", index)] = map[string]any{
			"id": index, "name": fmt.Sprintf("Outpost %d", index), "x": index * 3, "y": index * 7,
			"resources": map[string]any{"W": index * 11, "S": index * 5, "F": index * 13}, "buildings": []int{1, 2, 3, index % 40},
		}
	}
	automations := map[string]any{}
	for index := 0; index < 20; index++ {
		automations[fmt.Sprintf("auto%d", index)] = map[string]any{
			"id": fmt.Sprintf("auto%d", index), "enabled": true, "status": "running",
			"nextCheckAt": "2026-09-30T12:00:00Z", "updatedAt": "2026-09-30T11:59:00Z",
		}
	}
	document, _ := json.Marshal(map[string]any{
		"revision": 12345, "updatedAt": "2026-09-30T12:00:00Z", "castles": castles, "automations": automations,
	})
	return document
}

// BenchmarkCheckpointDigest is the cost of one content-gate evaluation of a
// large dashboard checkpoint (it replaces an upload, not an addition to one).
func BenchmarkCheckpointDigest(b *testing.B) {
	checkpoint := Checkpoint{State: syntheticDashboardState(), Session: CheckpointSession{State: "connected"}, ObservedAt: time.Now()}
	b.SetBytes(int64(len(checkpoint.State)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = checkpointDigest(checkpoint)
	}
}

// BenchmarkCheckpointEncode is the cost the gate avoids when content is
// unchanged: compressing and encoding the request as three gzip members.
func BenchmarkCheckpointEncode(b *testing.B) {
	checkpoint := Checkpoint{State: syntheticDashboardState(), Session: CheckpointSession{State: "connected"}, ObservedAt: time.Now()}
	request := CheckpointRequest{SchemaVersion: 1, CellID: "c", TenantID: "t", RuntimeID: "r", PlacementEpoch: 1, Checkpoint: checkpoint}
	b.SetBytes(int64(len(checkpoint.State)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := encodeCheckpointRequest(request); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSampleDigest is the per-minute cost of the sample content gate.
func BenchmarkSampleDigest(b *testing.B) {
	now := time.Now().UTC()
	sample, err := NewSampleBuilder(readyPrivateMetricsState(b, now), nil, nil).Build(context.Background(), now)
	if err != nil {
		b.Fatal(err)
	}
	encoded, _ := json.Marshal(sample)
	b.SetBytes(int64(len(encoded)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = sampleDigest(sample)
	}
}
