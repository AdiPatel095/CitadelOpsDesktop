package History

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const SchemaVersion = 2

const (
	CollectionPlayerSamples = "PlayerSamples"
	CollectionSpyReports    = "SpyReports"
	CollectionBattleReports = "BattleReports"

	PlayerSamplesFullResolutionRetention   = 24 * time.Hour
	PlayerSamplesHourlyResolutionRetention = 7 * 24 * time.Hour
	PlayerSamplesHistoryRetention          = 30 * 24 * time.Hour
	playerSamplesCompactionInterval        = time.Hour
)

type Store struct {
	directory                   string
	mu                          sync.Mutex
	lastPlayerSamplesCompaction time.Time
	lastPlayerSamplesRetention  PlayerSamplesRetention
}

type entry struct {
	SchemaVersion int             `json:"schemaVersion"`
	CapturedAt    time.Time       `json:"capturedAt"`
	Payload       json.RawMessage `json:"payload"`
}

type PlayerSample struct {
	TimestampUnix   int64              `json:"timestampUnix"`
	PlayerID        State.PlayerID     `json:"playerId"`
	Might           float64            `json:"might"`
	Glory           float64            `json:"glory"`
	Gallantry       float64            `json:"gallantry"`
	TroopsTotal     int64              `json:"troopsTotal"`
	TroopsStationed int64              `json:"troopsStationed"`
	TroopsTraveling int64              `json:"troopsTraveling"`
	TroopsHospital  int64              `json:"troopsHospital"`
	TroopsByUnit    map[string]int64   `json:"troopsByUnit,omitempty"`
	Coins           float64            `json:"coins"`
	Rubies          float64            `json:"rubies"`
	Currencies      map[string]float64 `json:"currencies,omitempty"`
}

func Open(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("history data directory is required")
	}
	directory := filepath.Join(dataDir, "History")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	store := &Store{directory: directory}
	if _, err := store.migrateLegacyPlayerTracker(filepath.Join(dataDir, "PlayerTracker.json")); err != nil {
		return nil, fmt.Errorf("migrate legacy player history: %w", err)
	}
	return store, nil
}

func (store *Store) Append(collection string, value any) error {
	if store == nil {
		return fmt.Errorf("history store is unavailable")
	}
	path, err := store.collectionPath(collection)
	if err != nil {
		return err
	}
	line, err := encodeHistoryEntry(collection, value)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.appendLocked(path, collection, line)
}

// CapturePlayerSampleWithRetention is the serialized capture boundary for My
// Stats. A capture that read an older configuration cannot append after a
// retention change has been durably applied, because both the applied-policy
// check and the file write run under the same lock as compaction.
func (store *Store) CapturePlayerSampleWithRetention(
	sample PlayerSample,
	observedAt time.Time,
	requested PlayerSamplesRetention,
) (appended bool, ranCompaction bool, err error) {
	if store == nil {
		return false, false, fmt.Errorf("history store is unavailable")
	}
	requested = NormalizePlayerSamplesRetention(requested)
	path, err := store.collectionPath(CollectionPlayerSamples)
	if err != nil {
		return false, false, err
	}
	line, err := encodeHistoryEntry(CollectionPlayerSamples, sample)
	if err != nil {
		return false, false, err
	}
	observedAt = normalizeRetentionNow(observedAt)

	store.mu.Lock()
	defer store.mu.Unlock()
	if requested == PlayerSamplesRetentionNone || store.lastPlayerSamplesRetention != requested {
		return false, false, nil
	}
	if err := store.appendLocked(path, CollectionPlayerSamples, line); err != nil {
		return false, false, err
	}
	if !store.lastPlayerSamplesCompaction.IsZero() &&
		observedAt.Sub(store.lastPlayerSamplesCompaction) < playerSamplesCompactionInterval {
		return true, false, nil
	}
	_, err = store.compactPlayerSamplesLocked(observedAt, requested)
	if err != nil {
		return true, true, err
	}
	store.lastPlayerSamplesCompaction = observedAt
	store.lastPlayerSamplesRetention = requested
	return true, true, nil
}

func encodeHistoryEntry(collection string, value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s history: %w", collection, err)
	}
	line, err := json.Marshal(entry{
		SchemaVersion: SchemaVersion,
		CapturedAt:    time.Now().UTC(),
		Payload:       payload,
	})
	if err != nil {
		return nil, err
	}
	return line, nil
}

func (store *Store) appendLocked(path, collection string, line []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s history: %w", collection, err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append %s history: %w", collection, err)
	}
	return file.Sync()
}

func (store *Store) Replace(collection string, values []json.RawMessage) error {
	if store == nil {
		return fmt.Errorf("history store is unavailable")
	}
	path, err := store.collectionPath(collection)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.replaceLocked(path, collection, values)
}

func (store *Store) RemoveWhere(collection string, remove func(json.RawMessage) bool) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("history store is unavailable")
	}
	if remove == nil {
		return 0, nil
	}
	path, err := store.collectionPath(collection)
	if err != nil {
		return 0, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open %s history: %w", collection, err)
	}
	defer file.Close()

	values := make([]json.RawMessage, 0, 1024)
	removed := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var item entry
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		if decoder.Decode(&item) != nil || item.SchemaVersion < 1 || item.SchemaVersion > SchemaVersion || len(item.Payload) == 0 {
			return 0, fmt.Errorf("decode %s history while removing entries", collection)
		}
		if remove(item.Payload) {
			removed++
			continue
		}
		values = append(values, append(json.RawMessage(nil), item.Payload...))
		if len(values) > 100_000 {
			return 0, fmt.Errorf("%s history reached the 100000-row safe rewrite limit", collection)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read %s history while removing entries: %w", collection, err)
	}
	if removed == 0 {
		return 0, nil
	}
	if err := store.replaceLocked(path, collection, values); err != nil {
		return 0, err
	}
	return removed, nil
}

func (store *Store) replaceLocked(path, collection string, values []json.RawMessage) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+collection+"-*")
	if err != nil {
		return fmt.Errorf("create compacted %s history: %w", collection, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	writer := bufio.NewWriter(temporary)
	capturedAt := time.Now().UTC()
	for _, payload := range values {
		if len(payload) == 0 {
			continue
		}
		line, marshalErr := json.Marshal(entry{
			SchemaVersion: SchemaVersion,
			CapturedAt:    capturedAt,
			Payload:       payload,
		})
		if marshalErr != nil {
			temporary.Close()
			return fmt.Errorf("encode compacted %s history: %w", collection, marshalErr)
		}
		if _, writeErr := writer.Write(append(line, '\n')); writeErr != nil {
			temporary.Close()
			return fmt.Errorf("write compacted %s history: %w", collection, writeErr)
		}
	}
	if err := writer.Flush(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush compacted %s history: %w", collection, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync compacted %s history: %w", collection, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close compacted %s history: %w", collection, err)
	}
	return commitReplacement(temporaryPath, path, collection)
}

func commitReplacement(temporaryPath, path, collection string) error {
	if err := os.Rename(temporaryPath, path); err != nil {
		backup, backupErr := os.CreateTemp(filepath.Dir(path), "."+collection+"-backup-*")
		if backupErr != nil {
			return fmt.Errorf("prepare existing %s history backup: %w", collection, backupErr)
		}
		backupPath := backup.Name()
		if closeErr := backup.Close(); closeErr != nil {
			return fmt.Errorf("close existing %s history backup: %w", collection, closeErr)
		}
		if removeErr := os.Remove(backupPath); removeErr != nil {
			return fmt.Errorf("prepare existing %s history backup path: %w", collection, removeErr)
		}
		if backupErr := os.Rename(path, backupPath); backupErr != nil {
			return fmt.Errorf("back up existing %s history: %w", collection, backupErr)
		}
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			if restoreErr := os.Rename(backupPath, path); restoreErr != nil {
				return fmt.Errorf(
					"replace compacted %s history: %v; restore original: %w",
					collection, retryErr, restoreErr,
				)
			}
			return fmt.Errorf("replace compacted %s history: %w", collection, retryErr)
		}
		_ = os.Remove(backupPath)
	}
	return nil
}

func (store *Store) Read(collection string, since time.Time, limit int) ([]json.RawMessage, error) {
	if store == nil {
		return nil, fmt.Errorf("history store is unavailable")
	}
	path, err := store.collectionPath(collection)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1000
	}
	if limit > 100_000 {
		limit = 100_000
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s history: %w", collection, err)
	}
	defer file.Close()
	result := make([]json.RawMessage, 0, min(limit, 1024))
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var item entry
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		if decoder.Decode(&item) != nil || item.SchemaVersion < 1 || item.SchemaVersion > SchemaVersion || len(item.Payload) == 0 {
			continue
		}
		if !since.IsZero() && item.CapturedAt.Before(since) {
			continue
		}
		result = append(result, append(json.RawMessage(nil), item.Payload...))
		if len(result) > limit {
			copy(result, result[len(result)-limit:])
			result = result[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s history: %w", collection, err)
	}
	return result, nil
}

func (store *Store) PlayerSamples(since time.Time, limit int) ([]PlayerSample, error) {
	rows, err := store.Read(CollectionPlayerSamples, since, limit)
	if err != nil {
		return nil, err
	}
	samples := make([]PlayerSample, 0, len(rows))
	for _, row := range rows {
		var sample PlayerSample
		if json.Unmarshal(row, &sample) == nil && sample.TimestampUnix > 0 {
			samples = append(samples, sample)
		}
	}
	sort.Slice(samples, func(left, right int) bool { return samples[left].TimestampUnix < samples[right].TimestampUnix })
	return samples, nil
}

func (store *Store) PlayerSamplesForPlayer(
	since time.Time,
	limit int,
	playerID State.PlayerID,
) ([]PlayerSample, error) {
	if store == nil {
		return nil, fmt.Errorf("history store is unavailable")
	}
	if playerID <= 0 {
		return []PlayerSample{}, nil
	}
	if limit < 1 {
		limit = 1000
	}
	if limit > 100_000 {
		limit = 100_000
	}
	path, err := store.collectionPath(CollectionPlayerSamples)
	if err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []PlayerSample{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s history: %w", CollectionPlayerSamples, err)
	}
	defer file.Close()

	samples := make([]PlayerSample, 0, min(limit, 1024))
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var item entry
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		if decoder.Decode(&item) != nil || item.SchemaVersion < 1 || item.SchemaVersion > SchemaVersion || len(item.Payload) == 0 {
			continue
		}
		var sample PlayerSample
		if json.Unmarshal(item.Payload, &sample) != nil || sample.TimestampUnix <= 0 || sample.PlayerID != playerID {
			continue
		}
		if !since.IsZero() && time.Unix(sample.TimestampUnix, 0).Before(since) {
			continue
		}
		samples = append(samples, sample)
		if len(samples) > limit {
			copy(samples, samples[len(samples)-limit:])
			samples = samples[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s history: %w", CollectionPlayerSamples, err)
	}
	sort.Slice(samples, func(left, right int) bool { return samples[left].TimestampUnix < samples[right].TimestampUnix })
	return samples, nil
}

type PlayerSamplesRetentionReport struct {
	Retention            PlayerSamplesRetention `json:"retention"`
	StartedAt            time.Time              `json:"startedAt"`
	FinishedAt           time.Time              `json:"finishedAt"`
	FullResolutionCutoff time.Time              `json:"fullResolutionCutoff"`
	HourlyCutoff         time.Time              `json:"hourlyCutoff"`
	RetentionCutoff      time.Time              `json:"retentionCutoff,omitempty"`
	Unlimited            bool                   `json:"unlimited,omitempty"`
	ScannedRows          int                    `json:"scannedRows"`
	DeletedRows          int                    `json:"deletedRows"`
	KeptRows             int                    `json:"keptRows"`
	Complete             bool                   `json:"complete"`
}

type playerSampleBucket struct {
	PlayerID   State.PlayerID
	Resolution uint8
	BucketUnix int64
}

type playerSampleKeeper struct {
	TimestampUnix int64
	LineIndex     int
}

// CompactPlayerSamples applies the default 30-day graph storage contract.
func (store *Store) CompactPlayerSamples(now time.Time) (PlayerSamplesRetentionReport, error) {
	return store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention30Days)
}

// CompactPlayerSamplesWithRetention preserves every valid sample for 24
// hours, the earliest sample in each UTC hour through day 7, and the earliest
// sample in each UTC day for the configured remaining window. Malformed or
// future-schema lines are copied unchanged unless the user explicitly selected
// no history, which atomically clears the complete PlayerSamples collection.
func (store *Store) CompactPlayerSamplesWithRetention(
	now time.Time,
	retention PlayerSamplesRetention,
) (PlayerSamplesRetentionReport, error) {
	return store.CompactPlayerSamplesWithResolvedRetention(now, func() PlayerSamplesRetention {
		return retention
	})
}

// CompactPlayerSamplesWithResolvedRetention resolves the authoritative policy
// only after taking the same lock used by capture. This prevents a background
// pass that observed an older configuration from overwriting a newer policy
// after the newer apply has already acknowledged.
func (store *Store) CompactPlayerSamplesWithResolvedRetention(
	now time.Time,
	resolve func() PlayerSamplesRetention,
) (PlayerSamplesRetentionReport, error) {
	if store == nil {
		return PlayerSamplesRetentionReport{}, fmt.Errorf("history store is unavailable")
	}
	if resolve == nil {
		return PlayerSamplesRetentionReport{}, fmt.Errorf("player samples retention resolver is required")
	}
	now = normalizeRetentionNow(now)
	store.mu.Lock()
	defer store.mu.Unlock()
	retention := NormalizePlayerSamplesRetention(resolve())
	report, err := store.compactPlayerSamplesLocked(now, retention)
	if err == nil {
		store.lastPlayerSamplesCompaction = now
		store.lastPlayerSamplesRetention = retention
	}
	return report, err
}

// CompactPlayerSamplesIfDue avoids re-reading the JSONL file on every
// one-minute capture. The first live capture compacts immediately; subsequent
// passes run at most once per hour.
func (store *Store) CompactPlayerSamplesIfDue(now time.Time) (PlayerSamplesRetentionReport, bool, error) {
	return store.CompactPlayerSamplesIfDueWithRetention(now, PlayerSamplesRetention30Days)
}

// CompactPlayerSamplesIfDueWithRetention runs immediately when the effective
// policy changes, even if the previous hourly pass was recent. This makes a
// shorter retention choice (especially no history) take effect at once.
func (store *Store) CompactPlayerSamplesIfDueWithRetention(
	now time.Time,
	retention PlayerSamplesRetention,
) (PlayerSamplesRetentionReport, bool, error) {
	return store.CompactPlayerSamplesIfDueResolved(now, func() PlayerSamplesRetention {
		return retention
	})
}

// CompactPlayerSamplesIfDueResolved is the retry/maintenance form of the
// resolved policy boundary. Policy mismatches retry immediately; matching
// policies retain the hourly compaction guard.
func (store *Store) CompactPlayerSamplesIfDueResolved(
	now time.Time,
	resolve func() PlayerSamplesRetention,
) (PlayerSamplesRetentionReport, bool, error) {
	if store == nil {
		return PlayerSamplesRetentionReport{}, false, fmt.Errorf("history store is unavailable")
	}
	if resolve == nil {
		return PlayerSamplesRetentionReport{}, false, fmt.Errorf("player samples retention resolver is required")
	}
	now = normalizeRetentionNow(now)
	store.mu.Lock()
	defer store.mu.Unlock()
	retention := NormalizePlayerSamplesRetention(resolve())
	if store.lastPlayerSamplesRetention == retention &&
		!store.lastPlayerSamplesCompaction.IsZero() &&
		now.Sub(store.lastPlayerSamplesCompaction) < playerSamplesCompactionInterval {
		return PlayerSamplesRetentionReport{}, false, nil
	}
	report, err := store.compactPlayerSamplesLocked(now, retention)
	if err == nil {
		store.lastPlayerSamplesCompaction = now
		store.lastPlayerSamplesRetention = retention
	}
	return report, true, err
}

func normalizeRetentionNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func newPlayerSamplesRetentionReport(now time.Time, retention PlayerSamplesRetention) PlayerSamplesRetentionReport {
	retention = NormalizePlayerSamplesRetention(retention)
	report := PlayerSamplesRetentionReport{
		Retention:            retention,
		StartedAt:            now,
		FullResolutionCutoff: now.Add(-PlayerSamplesFullResolutionRetention),
		HourlyCutoff:         now.Add(-PlayerSamplesHourlyResolutionRetention),
	}
	duration, unlimited := PlayerSamplesRetentionDuration(retention)
	report.Unlimited = unlimited
	if unlimited {
		return report
	}
	report.RetentionCutoff = now.Add(-duration)
	if report.RetentionCutoff.After(report.HourlyCutoff) {
		report.HourlyCutoff = report.RetentionCutoff
	}
	if report.RetentionCutoff.After(report.FullResolutionCutoff) {
		report.FullResolutionCutoff = report.RetentionCutoff
	}
	return report
}

func (store *Store) compactPlayerSamplesLocked(
	now time.Time,
	retention PlayerSamplesRetention,
) (PlayerSamplesRetentionReport, error) {
	report := newPlayerSamplesRetentionReport(now, retention)
	path, err := store.collectionPath(CollectionPlayerSamples)
	if err != nil {
		return report, err
	}
	if retention == PlayerSamplesRetentionNone {
		return store.clearPlayerSamplesLocked(path, report)
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		report.Complete = true
		report.FinishedAt = time.Now().UTC()
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("open %s history for retention: %w", CollectionPlayerSamples, err)
	}

	keepers := map[playerSampleBucket]playerSampleKeeper{}
	plannedDeletes := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for lineIndex := 0; scanner.Scan(); lineIndex++ {
		report.ScannedRows++
		sample, valid := decodePlayerSampleLine(scanner.Bytes())
		if !valid {
			continue
		}
		bucket, keepAll, expired := playerSampleRetentionBucket(sample, report)
		if expired {
			plannedDeletes++
			continue
		}
		if keepAll {
			continue
		}
		candidate := playerSampleKeeper{TimestampUnix: sample.TimestampUnix, LineIndex: lineIndex}
		existing, exists := keepers[bucket]
		if !exists {
			keepers[bucket] = candidate
			continue
		}
		plannedDeletes++
		if candidate.TimestampUnix < existing.TimestampUnix {
			keepers[bucket] = candidate
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		file.Close()
		return report, fmt.Errorf("scan %s history for retention: %w", CollectionPlayerSamples, scanErr)
	}
	if err := file.Close(); err != nil {
		return report, fmt.Errorf("close %s history after retention scan: %w", CollectionPlayerSamples, err)
	}
	if plannedDeletes == 0 {
		report.KeptRows = report.ScannedRows
		report.Complete = true
		report.FinishedAt = time.Now().UTC()
		return report, nil
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), "."+CollectionPlayerSamples+"-retention-*")
	if err != nil {
		return report, fmt.Errorf("create compacted %s history: %w", CollectionPlayerSamples, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return report, err
	}
	source, err := os.Open(path)
	if err != nil {
		temporary.Close()
		return report, fmt.Errorf("reopen %s history for retention: %w", CollectionPlayerSamples, err)
	}
	writer := bufio.NewWriter(temporary)
	scanner = bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for lineIndex := 0; scanner.Scan(); lineIndex++ {
		keep := true
		if sample, valid := decodePlayerSampleLine(scanner.Bytes()); valid {
			bucket, keepAll, expired := playerSampleRetentionBucket(sample, report)
			switch {
			case expired:
				keep = false
			case !keepAll:
				keep = keepers[bucket].LineIndex == lineIndex
			}
		}
		if !keep {
			report.DeletedRows++
			continue
		}
		report.KeptRows++
		if _, err := writer.Write(scanner.Bytes()); err != nil {
			source.Close()
			temporary.Close()
			return report, fmt.Errorf("write compacted %s history: %w", CollectionPlayerSamples, err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			source.Close()
			temporary.Close()
			return report, fmt.Errorf("write compacted %s history: %w", CollectionPlayerSamples, err)
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		source.Close()
		temporary.Close()
		return report, fmt.Errorf("rewrite %s history for retention: %w", CollectionPlayerSamples, scanErr)
	}
	if err := source.Close(); err != nil {
		temporary.Close()
		return report, fmt.Errorf("close %s history after retention rewrite: %w", CollectionPlayerSamples, err)
	}
	if err := writer.Flush(); err != nil {
		temporary.Close()
		return report, fmt.Errorf("flush compacted %s history: %w", CollectionPlayerSamples, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return report, fmt.Errorf("sync compacted %s history: %w", CollectionPlayerSamples, err)
	}
	if err := temporary.Close(); err != nil {
		return report, fmt.Errorf("close compacted %s history: %w", CollectionPlayerSamples, err)
	}
	if report.DeletedRows > 0 {
		if err := commitReplacement(temporaryPath, path, CollectionPlayerSamples); err != nil {
			return report, err
		}
	}
	report.Complete = true
	report.FinishedAt = time.Now().UTC()
	return report, nil
}

func (store *Store) clearPlayerSamplesLocked(
	path string,
	report PlayerSamplesRetentionReport,
) (PlayerSamplesRetentionReport, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		report.Complete = true
		report.FinishedAt = time.Now().UTC()
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("open %s history for clearing: %w", CollectionPlayerSamples, err)
	}
	rows, err := countHistoryLines(file)
	closeErr := file.Close()
	if err != nil {
		return report, fmt.Errorf("count %s history for clearing: %w", CollectionPlayerSamples, err)
	}
	if closeErr != nil {
		return report, fmt.Errorf("close %s history for clearing: %w", CollectionPlayerSamples, closeErr)
	}
	report.ScannedRows = rows
	if err := store.replaceLocked(path, CollectionPlayerSamples, nil); err != nil {
		return report, fmt.Errorf("clear %s history: %w", CollectionPlayerSamples, err)
	}
	report.DeletedRows = rows
	report.Complete = true
	report.FinishedAt = time.Now().UTC()
	return report, nil
}

func countHistoryLines(reader io.Reader) (int, error) {
	buffer := make([]byte, 64*1024)
	lines := 0
	hasData := false
	last := byte('\n')
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			hasData = true
			last = buffer[count-1]
			lines += bytes.Count(buffer[:count], []byte{'\n'})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if hasData && last != '\n' {
		lines++
	}
	return lines, nil
}

func decodePlayerSampleLine(line []byte) (PlayerSample, bool) {
	var item entry
	decoder := json.NewDecoder(bytes.NewReader(line))
	if decoder.Decode(&item) != nil || item.SchemaVersion < 1 || item.SchemaVersion > SchemaVersion || len(item.Payload) == 0 {
		return PlayerSample{}, false
	}
	var sample PlayerSample
	if json.Unmarshal(item.Payload, &sample) != nil || sample.TimestampUnix <= 0 || sample.PlayerID <= 0 {
		return PlayerSample{}, false
	}
	return sample, true
}

func playerSampleRetentionBucket(sample PlayerSample, cutoffs PlayerSamplesRetentionReport) (playerSampleBucket, bool, bool) {
	at := time.Unix(sample.TimestampUnix, 0).UTC()
	switch {
	case !cutoffs.RetentionCutoff.IsZero() && at.Before(cutoffs.RetentionCutoff):
		return playerSampleBucket{}, false, true
	case at.Before(cutoffs.HourlyCutoff):
		return playerSampleBucket{PlayerID: sample.PlayerID, Resolution: 2, BucketUnix: at.Truncate(24 * time.Hour).Unix()}, false, false
	case at.Before(cutoffs.FullResolutionCutoff):
		return playerSampleBucket{PlayerID: sample.PlayerID, Resolution: 1, BucketUnix: at.Truncate(time.Hour).Unix()}, false, false
	default:
		return playerSampleBucket{}, true, false
	}
}

func NewPlayerSample(snapshot State.GameState, gameData *GameData.Manager) PlayerSample {
	return NewPlayerSampleAt(snapshot, gameData, time.Now().UTC())
}

// NewPlayerSampleAt builds the same complete My Stats projection at a caller-
// supplied observation time. Cloud publishers use this to keep the sample
// timestamp, idempotency key, and enclosing runtime observation consistent.
func NewPlayerSampleAt(snapshot State.GameState, gameData *GameData.Manager, observedAt time.Time) PlayerSample {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	sample := PlayerSample{
		TimestampUnix: observedAt.UTC().Unix(), PlayerID: snapshot.Player.ID,
		Might: snapshot.Player.Might, Glory: snapshot.Player.Glory, Gallantry: snapshot.Player.Gallantry,
		TroopsByUnit: map[string]int64{}, Currencies: map[string]float64{},
	}
	for id, amount := range snapshot.Player.Resources {
		key := fmt.Sprintf("resource:%d", id)
		if gameData != nil {
			if definition, err := gameData.Resource(int64(id)); err == nil {
				if definition.JSONKey == "C1" {
					sample.Coins = amount
				} else if definition.JSONKey == "C2" {
					sample.Rubies = amount
				}
			}
		}
		sample.Currencies[key] = amount
	}
	for id, amount := range snapshot.Player.Currencies {
		sample.Currencies[fmt.Sprintf("currency:%d", id)] = amount
	}
	classifier := newTroopClassifier(gameData)
	addGroup := func(values map[State.UnitID]int64, total *int64) {
		for id, amount := range values {
			if amount <= 0 || !classifier.isTroop(id) {
				continue
			}
			*total += amount
			sample.TroopsByUnit[strconv.FormatInt(int64(id), 10)] += amount
		}
	}
	for _, castle := range snapshot.Castles {
		addGroup(castle.Units.Stationed, &sample.TroopsStationed)
		addGroup(castle.Units.Traveling, &sample.TroopsTraveling)
		addGroup(castle.Units.Hospital, &sample.TroopsHospital)
		addGroup(castle.Units.SpecialHospital, &sample.TroopsHospital)
	}
	sample.TroopsTotal = sample.TroopsStationed + sample.TroopsTraveling + sample.TroopsHospital
	return sample
}

func NormalizePlayerSamplesTroops(samples []PlayerSample, gameData *GameData.Manager) []PlayerSample {
	classifier := newTroopClassifier(gameData)
	for index := range samples {
		samples[index] = normalizePlayerSampleTroops(samples[index], classifier)
	}
	return samples
}

func normalizePlayerSampleTroops(sample PlayerSample, classifier *troopClassifier) PlayerSample {
	if len(sample.TroopsByUnit) == 0 || classifier == nil || !classifier.enabled {
		return sample
	}
	filtered := make(map[string]int64, len(sample.TroopsByUnit))
	var total int64
	for rawID, amount := range sample.TroopsByUnit {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || amount <= 0 || !classifier.isTroop(State.UnitID(id)) {
			continue
		}
		filtered[rawID] = amount
		total += amount
	}
	sample.TroopsByUnit = filtered
	sample.TroopsTotal = total
	return sample
}

type troopClassifier struct {
	catalog *GameData.Catalog
	cache   map[State.UnitID]bool
	enabled bool
}

func newTroopClassifier(gameData *GameData.Manager) *troopClassifier {
	classifier := &troopClassifier{cache: map[State.UnitID]bool{}}
	if gameData == nil {
		return classifier
	}
	store, ready := gameData.Current()
	if !ready {
		return classifier
	}
	catalog, err := store.Catalog("units")
	if err != nil {
		return classifier
	}
	classifier.catalog = catalog
	classifier.enabled = true
	return classifier
}

func (classifier *troopClassifier) isTroop(id State.UnitID) bool {
	if classifier == nil || !classifier.enabled {
		return true
	}
	if troop, known := classifier.cache[id]; known {
		return troop
	}
	raw, found := classifier.catalog.Find(strconv.FormatInt(int64(id), 10))
	if !found {
		classifier.cache[id] = false
		return false
	}
	record, err := GameData.DecodeRecord(raw)
	troop := err == nil && !GameData.IsToolRecord(record)
	classifier.cache[id] = troop
	return troop
}

func (store *Store) collectionPath(collection string) (string, error) {
	switch collection {
	case CollectionPlayerSamples, CollectionSpyReports, CollectionBattleReports:
		return filepath.Join(store.directory, collection+".jsonl"), nil
	default:
		return "", fmt.Errorf("unknown history collection %q", collection)
	}
}
