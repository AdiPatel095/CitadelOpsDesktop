package History

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEstimatePlayerSamplesStorageUsesCurrentShapeBeforeHistoryExists(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	estimate, err := store.EstimatePlayerSamplesStorage(PlayerSample{
		TimestampUnix: time.Now().UTC().Unix(),
		PlayerID:      1,
		TroopsByUnit:  map[string]int64{"10": 100, "20": 200},
		Currencies:    map[string]float64{"resource:1": 12345},
	})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Format != "jsonl" || estimate.Basis != "current-sample" ||
		estimate.EstimatedBytesPerRecording <= 0 || estimate.CurrentBytes != 0 {
		t.Fatalf("estimate = %+v", estimate)
	}
}

func TestEstimatePlayerSamplesStorageSamplesExistingJSONLRows(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := 0; index < 2; index++ {
		if err := store.Append(CollectionPlayerSamples, PlayerSample{
			TimestampUnix: now.Add(time.Duration(index) * time.Hour).Unix(),
			PlayerID:      1,
			Might:         float64(index + 1),
		}); err != nil {
			t.Fatal(err)
		}
	}
	estimate, err := store.EstimatePlayerSamplesStorage(PlayerSample{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dataDir, "History", "PlayerSamples.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Basis != "saved-samples" || estimate.SampledRecordings != 2 ||
		estimate.CurrentBytes != info.Size() || estimate.EstimatedBytesPerRecording <= 0 {
		t.Fatalf("estimate = %+v, file bytes = %d", estimate, info.Size())
	}
}
