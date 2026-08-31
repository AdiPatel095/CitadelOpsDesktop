package History

import (
	"bufio"
	"fmt"
	"os"
)

const (
	playerSamplesStorageSampleLimit      = 256
	defaultPlayerSampleBytesPerRecording = int64(4 * 1024)
)

// PlayerSamplesStorageEstimate describes the local PlayerSamples JSONL file
// and the row-size basis used for Settings projections. Rows vary with the
// profile's troop and currency maps, so this is deliberately an estimate.
type PlayerSamplesStorageEstimate struct {
	Format                     string `json:"format"`
	CurrentBytes               int64  `json:"currentBytes"`
	EstimatedBytesPerRecording int64  `json:"estimatedBytesPerRecording"`
	Basis                      string `json:"basis"`
	SampledRecordings          int64  `json:"sampledRecordings,omitempty"`
}

func DefaultPlayerSamplesStorageEstimate() PlayerSamplesStorageEstimate {
	return PlayerSamplesStorageEstimate{
		Format:                     "jsonl",
		EstimatedBytesPerRecording: defaultPlayerSampleBytesPerRecording,
		Basis:                      "conservative-fallback",
	}
}

// EstimatePlayerSamplesStorage samples a bounded number of existing rows and
// compares their average with the current encoded sample. The larger value is
// used so a profile whose maps have grown does not immediately understate its
// projected history size.
func (store *Store) EstimatePlayerSamplesStorage(current PlayerSample) (PlayerSamplesStorageEstimate, error) {
	estimate := DefaultPlayerSamplesStorageEstimate()
	if store == nil {
		return estimate, fmt.Errorf("history store is unavailable")
	}
	path, err := store.collectionPath(CollectionPlayerSamples)
	if err != nil {
		return estimate, err
	}
	currentBytes := int64(0)
	if current.PlayerID > 0 {
		line, encodeErr := encodeHistoryEntry(CollectionPlayerSamples, current)
		if encodeErr == nil {
			currentBytes = int64(len(line) + 1)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		if currentBytes > 0 {
			estimate.EstimatedBytesPerRecording = currentBytes
			estimate.Basis = "current-sample"
		}
		return estimate, nil
	}
	if err != nil {
		return estimate, fmt.Errorf("open %s history for storage estimate: %w", CollectionPlayerSamples, err)
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr == nil {
		estimate.CurrentBytes = info.Size()
	}

	var sampledBytes int64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if _, valid := decodePlayerSampleLine(scanner.Bytes()); !valid {
			continue
		}
		sampledBytes += int64(len(scanner.Bytes()) + 1)
		estimate.SampledRecordings++
		if estimate.SampledRecordings >= playerSamplesStorageSampleLimit {
			break
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return estimate, fmt.Errorf("scan %s history for storage estimate: %w", CollectionPlayerSamples, scanErr)
	}
	if estimate.SampledRecordings > 0 {
		average := (sampledBytes + estimate.SampledRecordings - 1) / estimate.SampledRecordings
		estimate.EstimatedBytesPerRecording = average
		estimate.Basis = "saved-samples"
	}
	if currentBytes > estimate.EstimatedBytesPerRecording {
		estimate.EstimatedBytesPerRecording = currentBytes
		estimate.Basis = "current-sample"
	}
	return estimate, nil
}
