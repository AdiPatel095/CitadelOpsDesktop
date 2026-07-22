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
	UID             int64              `json:"uid,omitempty"`
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

func (store *Store) PlayerSamplesForAccount(
	since time.Time,
	limit int,
	uid int64,
	playerID State.PlayerID,
) ([]PlayerSample, error) {
	if uid <= 0 && playerID <= 0 {
		return []PlayerSample{}, nil
	}
	samples, err := store.PlayerSamples(since, 100_000)
	if err != nil {
		return nil, err
	}
	filtered := make([]PlayerSample, 0, min(len(samples), max(1, limit)))
	for _, sample := range samples {
		if uid > 0 {
			if sample.UID == uid {
				filtered = append(filtered, sample)
				continue
			}
			if sample.UID == 0 && playerID > 0 && sample.PlayerID == playerID {
				sample.UID = uid
				filtered = append(filtered, sample)
			}
			continue
		}
		if sample.PlayerID == playerID {
			filtered = append(filtered, sample)
		}
	}
	if limit < 1 {
		limit = 1000
	}
	if limit > 100_000 {
		limit = 100_000
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
}

func NewPlayerSample(snapshot State.GameState, gameData *GameData.Manager) PlayerSample {
	sample := PlayerSample{
		TimestampUnix: time.Now().Unix(), UID: snapshot.Account.UID, PlayerID: snapshot.Player.ID,
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
