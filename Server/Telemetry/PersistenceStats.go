package Telemetry

import "time"

const (
	featureLiveTailMaxBytes    = 1024 * 1024
	PersistenceMaxBytes        = 16 * 1024 * 1024
	PersistenceMaxRecords      = 8192
	persistenceMaxBatchBytes   = 1024 * 1024
	persistenceMaxBatchRecords = 128
	persistenceMaxFlushWaiters = 8
	persistenceFlushTimeout    = 5 * time.Second
)

// PersistenceStats contains no channel names, account identifiers or payloads.
// Retained totals include both pending records and the writer's detached batch.
type PersistenceStats struct {
	FlushCancelled          uint64    `json:"flushCancelled"`
	PendingBytes            int       `json:"pendingBytes"`
	PendingRecords          int       `json:"pendingRecords"`
	FlushWaiters            int       `json:"flushWaiters"`
	MaxFlushWaiters         int       `json:"maxFlushWaiters"`
	TailReaders             int       `json:"tailReaders"`
	MaxTailReaders          int       `json:"maxTailReaders"`
	OverflowRecords         uint64    `json:"overflowRecords"`
	OverflowBytes           uint64    `json:"overflowBytes"`
	OversizeBytes           uint64    `json:"oversizeBytes"`
	LiveTailEvictedBytes    uint64    `json:"liveTailEvictedBytes"`
	LiveTailEvictedRecords  uint64    `json:"liveTailEvictedRecords"`
	LiveTailOversizeRecords uint64    `json:"liveTailOversizeRecords"`
	RetainedBytes           int       `json:"retainedBytes"`
	RetainedRecords         int       `json:"retainedRecords"`
	InFlightBytes           int       `json:"inFlightBytes"`
	InFlightRecords         int       `json:"inFlightRecords"`
	MaxBytes                int       `json:"maxBytes"`
	MaxRecords              int       `json:"maxRecords"`
	HighWaterBytes          int       `json:"highWaterBytes"`
	HighWaterRecords        int       `json:"highWaterRecords"`
	DroppedRecords          uint64    `json:"droppedRecords"`
	DroppedBytes            uint64    `json:"droppedBytes"`
	OversizeRecords         uint64    `json:"oversizeRecords"`
	WriteFailures           uint64    `json:"writeFailures"`
	FlushRejected           uint64    `json:"flushRejected"`
	FlushTimeouts           uint64    `json:"flushTimeouts"`
	LastDrainDurationMillis int64     `json:"lastDrainDurationMillis"`
	LastSuccessfulDrain     time.Time `json:"lastSuccessfulDrain"`
}

func (store *Store) PersistenceSnapshot() PersistenceStats {
	if store == nil {
		return PersistenceStats{}
	}
	store.persistMu.Lock()
	defer store.persistMu.Unlock()
	stats := store.persistStats
	stats.PendingBytes = stats.RetainedBytes - stats.InFlightBytes
	stats.PendingRecords = stats.RetainedRecords - stats.InFlightRecords
	stats.FlushWaiters = store.persistFlushWaiters
	stats.MaxFlushWaiters = persistenceMaxFlushWaiters
	stats.TailReaders = store.persistTailReaders
	stats.MaxTailReaders = 2
	stats.MaxBytes = PersistenceMaxBytes
	stats.MaxRecords = PersistenceMaxRecords
	return stats
}

// Add aggregates fixed numeric fields without account/channel labels.
func (total *PersistenceStats) Add(s PersistenceStats) {
	total.PendingBytes += s.PendingBytes
	total.PendingRecords += s.PendingRecords
	total.FlushWaiters += s.FlushWaiters
	total.MaxFlushWaiters += s.MaxFlushWaiters
	total.TailReaders += s.TailReaders
	total.MaxTailReaders += s.MaxTailReaders
	total.OverflowRecords += s.OverflowRecords
	total.OverflowBytes += s.OverflowBytes
	total.OversizeBytes += s.OversizeBytes
	total.RetainedBytes += s.RetainedBytes
	total.RetainedRecords += s.RetainedRecords
	total.InFlightBytes += s.InFlightBytes
	total.InFlightRecords += s.InFlightRecords
	total.MaxBytes += s.MaxBytes
	total.MaxRecords += s.MaxRecords
	total.HighWaterBytes += s.HighWaterBytes
	total.HighWaterRecords += s.HighWaterRecords
	total.DroppedRecords += s.DroppedRecords
	total.DroppedBytes += s.DroppedBytes
	total.OversizeRecords += s.OversizeRecords
	total.WriteFailures += s.WriteFailures
	total.FlushRejected += s.FlushRejected
	total.FlushTimeouts += s.FlushTimeouts
	total.FlushCancelled += s.FlushCancelled
	total.LiveTailEvictedBytes += s.LiveTailEvictedBytes
	total.LiveTailEvictedRecords += s.LiveTailEvictedRecords
	total.LiveTailOversizeRecords += s.LiveTailOversizeRecords
	total.LastDrainDurationMillis = max(total.LastDrainDurationMillis, s.LastDrainDurationMillis)
	if total.LastSuccessfulDrain.IsZero() || s.LastSuccessfulDrain.Before(total.LastSuccessfulDrain) {
		total.LastSuccessfulDrain = s.LastSuccessfulDrain
	}
}
