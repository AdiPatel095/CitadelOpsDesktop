package History

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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
)

type Store struct {
	directory string
	mu        sync.Mutex
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
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s history: %w", collection, err)
	}
	line, err := json.Marshal(entry{
		SchemaVersion: SchemaVersion,
		CapturedAt:    time.Now().UTC(),
		Payload:       payload,
	})
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
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

func NewPlayerSample(snapshot State.GameState, gameData *GameData.Manager) PlayerSample {
	sample := PlayerSample{
		TimestampUnix: time.Now().Unix(), PlayerID: snapshot.Player.ID,
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
