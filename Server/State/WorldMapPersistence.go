package State

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	RuntimeKernel "CitadelDesktop/Server/Runtime"
	_ "modernc.org/sqlite"
)

const worldMapPersistenceInterval = 2 * time.Second

type persistedWorldMapChange struct {
	WorldID string
	Change  MapChange
}

func OpenWorldMapStore(dataRoot string) (*WorldMapStore, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return nil, fmt.Errorf("world-map data root is required")
	}
	directory := filepath.Join(dataRoot, "Shared", "State")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create shared world-map directory: %w", err)
	}
	databasePath := filepath.Join(directory, "WorldMap.sqlite")
	databaseURL, err := RuntimeKernel.SQLiteFileDSN(databasePath, "busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("configure shared world-map database: %w", err)
	}
	db, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open shared world-map database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := NewWorldMapStore()
	store.db = db
	if err := store.initializePersistence(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure shared world-map database: %w", err)
	}
	if err := store.loadPersistedFacts(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *WorldMapStore) initializePersistence(ctx context.Context) error {
	if store == nil || store.db == nil {
		return nil
	}
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE IF NOT EXISTS world_map_facts (
			world_id TEXT NOT NULL,
			kingdom_id INTEGER NOT NULL,
			coordinate_key TEXT NOT NULL,
			fact_json BLOB NOT NULL,
			observed_at_ms INTEGER NOT NULL,
			PRIMARY KEY (world_id, kingdom_id, coordinate_key)
		) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS world_storm_scan_windows (
			world_id TEXT NOT NULL,
			kingdom_id INTEGER NOT NULL,
			window_key TEXT NOT NULL,
			x1 INTEGER NOT NULL,
			y1 INTEGER NOT NULL,
			x2 INTEGER NOT NULL,
			y2 INTEGER NOT NULL,
			completed_at_ms INTEGER NOT NULL,
			PRIMARY KEY (world_id, kingdom_id, window_key)
		) WITHOUT ROWID`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize shared world-map persistence: %w", err)
		}
	}
	return nil
}

func (store *WorldMapStore) loadPersistedFacts(ctx context.Context) error {
	if store == nil || store.db == nil {
		return nil
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT world_id, kingdom_id, coordinate_key, fact_json
		FROM world_map_facts
		ORDER BY world_id, kingdom_id, coordinate_key
	`)
	if err != nil {
		return fmt.Errorf("load shared world-map facts: %w", err)
	}
	defer rows.Close()
	loaded := map[string]worldFactMap{}
	for rows.Next() {
		var worldID string
		var kingdomID int64
		var key string
		var raw []byte
		if err := rows.Scan(&worldID, &kingdomID, &key, &raw); err != nil {
			return fmt.Errorf("scan shared world-map fact: %w", err)
		}
		var fact WorldMapFact
		if err := json.Unmarshal(raw, &fact); err != nil {
			return fmt.Errorf("decode shared world-map fact: %w", err)
		}
		worldID = CanonicalWorldID(worldID)
		fact.KingdomID = KingdomID(kingdomID)
		if worldID == "" || key == "" || !ShareableMapObservation(fact.observation()) {
			return fmt.Errorf("shared world-map database contains an invalid fact")
		}
		world := loaded[worldID]
		if world == nil {
			world = worldFactMap{}
			loaded[worldID] = world
		}
		kingdom := world[fact.KingdomID]
		if kingdom == nil {
			kingdom = &worldFactRegion{}
			world[fact.KingdomID] = kingdom
		}
		addWorldFact(kingdom, key, fact)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate shared world-map facts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close shared world-map fact rows: %w", err)
	}
	windowRows, err := store.db.QueryContext(ctx, `
		SELECT world_id, kingdom_id, window_key, x1, y1, x2, y2, completed_at_ms
		FROM world_storm_scan_windows
		ORDER BY world_id, kingdom_id, window_key
	`)
	if err != nil {
		return fmt.Errorf("load shared Storm scan windows: %w", err)
	}
	defer windowRows.Close()
	loadedWindows := map[string]map[KingdomID]map[string]StormScanWindowState{}
	for windowRows.Next() {
		var worldID string
		var kingdomID int64
		var key string
		var bounds StormMapBounds
		var completedAtMillis int64
		if err := windowRows.Scan(
			&worldID, &kingdomID, &key, &bounds.X1, &bounds.Y1, &bounds.X2, &bounds.Y2, &completedAtMillis,
		); err != nil {
			return fmt.Errorf("scan shared Storm scan window: %w", err)
		}
		worldID = CanonicalWorldID(worldID)
		if worldID == "" || key == "" || key != stormScanWindowKey(bounds) || !bounds.IsValid() || completedAtMillis <= 0 {
			return fmt.Errorf("shared Storm scan database contains an invalid window")
		}
		world := loadedWindows[worldID]
		if world == nil {
			world = map[KingdomID]map[string]StormScanWindowState{}
			loadedWindows[worldID] = world
		}
		kingdom := KingdomID(kingdomID)
		if world[kingdom] == nil {
			world[kingdom] = map[string]StormScanWindowState{}
		}
		world[kingdom][key] = StormScanWindowState{
			Bounds: bounds, CompletedAt: time.UnixMilli(completedAtMillis).UTC(),
		}
	}
	if err := windowRows.Err(); err != nil {
		return fmt.Errorf("iterate shared Storm scan windows: %w", err)
	}
	store.mu.Lock()
	for worldID, facts := range loaded {
		store.worlds[worldID] = &worldMapGeneration{
			version: 1, updatedAt: time.Now().UTC(), values: facts, stormWindows: loadedWindows[worldID],
			stormPlans: buildWorldStormScanPlans(facts),
		}
		delete(loadedWindows, worldID)
	}
	for worldID, windows := range loadedWindows {
		store.worlds[worldID] = &worldMapGeneration{
			version: 1, updatedAt: time.Now().UTC(), values: worldFactMap{}, stormWindows: windows,
			stormPlans: initialStormScanPlans(),
		}
	}
	store.mu.Unlock()
	// Queue bounded-retention deletes before the persistence worker starts. The
	// first grouped flush removes them from SQLite without loading them again.
	store.Prune(time.Now().UTC())
	return nil
}

func (store *WorldMapStore) queuePersistence(event WorldMapEvent) {
	if store == nil || store.db == nil || event.WorldID == "" || len(event.Changes) == 0 {
		return
	}
	store.persistMu.Lock()
	for _, change := range event.Changes {
		key := persistedWorldMapKey(event.WorldID, change.KingdomID, change.Key)
		store.dirtyFacts[key] = persistedWorldMapChange{WorldID: event.WorldID, Change: change}
	}
	store.persistMu.Unlock()
	select {
	case store.persistWake <- struct{}{}:
	default:
	}
}

func (store *WorldMapStore) StartPersistence() {
	if store == nil {
		return
	}
	store.persistStartOnce.Do(func() {
		go store.runPersistence()
	})
}

func (store *WorldMapStore) runPersistence() {
	defer close(store.persistDone)
	if store.db == nil {
		<-store.persistStop
		return
	}
	var timer *time.Timer
	var timerChannel <-chan time.Time
	for {
		select {
		case <-store.persistWake:
			if timer == nil {
				timer = time.NewTimer(worldMapPersistenceInterval)
				timerChannel = timer.C
			}
		case <-timerChannel:
			if store.flushPersistence(context.Background()) == nil {
				timer = nil
				timerChannel = nil
			} else {
				timer = time.NewTimer(worldMapPersistenceInterval)
				timerChannel = timer.C
			}
		case <-store.persistStop:
			if timer != nil {
				timer.Stop()
			}
			_ = store.flushPersistence(context.Background())
			return
		}
	}
}

func (store *WorldMapStore) flushPersistence(ctx context.Context) error {
	if store == nil || store.db == nil {
		return nil
	}
	store.persistMu.Lock()
	if len(store.dirtyFacts) == 0 && len(store.dirtyStormScans) == 0 {
		store.persistMu.Unlock()
		return nil
	}
	batch := store.dirtyFacts
	store.dirtyFacts = map[string]persistedWorldMapChange{}
	scanBatch := store.dirtyStormScans
	store.dirtyStormScans = map[string]persistedStormScanWindow{}
	store.persistMu.Unlock()

	tx, err := store.db.BeginTx(ctx, nil)
	if err == nil {
		var deleteFact *sql.Stmt
		var upsertFact *sql.Stmt
		var upsertScan *sql.Stmt
		if len(batch) > 0 {
			deleteFact, err = tx.PrepareContext(ctx, `
				DELETE FROM world_map_facts
				WHERE world_id = ? AND kingdom_id = ? AND coordinate_key = ?
			`)
			if err == nil {
				upsertFact, err = tx.PrepareContext(ctx, `
					INSERT INTO world_map_facts (world_id, kingdom_id, coordinate_key, fact_json, observed_at_ms)
					VALUES (?, ?, ?, ?, ?)
					ON CONFLICT (world_id, kingdom_id, coordinate_key) DO UPDATE SET
						fact_json = excluded.fact_json,
						observed_at_ms = excluded.observed_at_ms
				`)
			}
		}
		if err == nil && len(scanBatch) > 0 {
			upsertScan, err = tx.PrepareContext(ctx, `
				INSERT INTO world_storm_scan_windows (
					world_id, kingdom_id, window_key, x1, y1, x2, y2, completed_at_ms
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (world_id, kingdom_id, window_key) DO UPDATE SET
					x1 = excluded.x1,
					y1 = excluded.y1,
					x2 = excluded.x2,
					y2 = excluded.y2,
					completed_at_ms = excluded.completed_at_ms
			`)
		}
		for _, pending := range batch {
			if err != nil {
				break
			}
			change := pending.Change
			if change.Deleted {
				_, err = deleteFact.ExecContext(ctx, pending.WorldID, change.KingdomID, change.Key)
			} else if change.Observation != nil {
				fact := worldMapFact(*change.Observation)
				var raw []byte
				raw, err = json.Marshal(fact)
				if err == nil {
					_, err = upsertFact.ExecContext(
						ctx, pending.WorldID, change.KingdomID, change.Key, raw, fact.ObservedAt.UnixMilli(),
					)
				}
			}
			if err != nil {
				break
			}
		}
		if err == nil {
			for _, pending := range scanBatch {
				_, err = upsertScan.ExecContext(ctx, pending.WorldID, pending.KingdomID, pending.Key,
					pending.Bounds.X1, pending.Bounds.Y1, pending.Bounds.X2, pending.Bounds.Y2,
					pending.CompletedAt.UnixMilli())
				if err != nil {
					break
				}
			}
		}
		for _, statement := range []*sql.Stmt{deleteFact, upsertFact, upsertScan} {
			if statement != nil {
				if closeErr := statement.Close(); err == nil {
					err = closeErr
				}
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	store.persistMu.Lock()
	store.persistenceErr = err
	if err != nil {
		for key, pending := range batch {
			if _, newer := store.dirtyFacts[key]; !newer {
				store.dirtyFacts[key] = pending
			}
		}
		for key, pending := range scanBatch {
			if _, newer := store.dirtyStormScans[key]; !newer {
				store.dirtyStormScans[key] = pending
			}
		}
	}
	store.persistMu.Unlock()
	if err != nil {
		return fmt.Errorf("persist shared world-map facts: %w", err)
	}
	return nil
}

func (store *WorldMapStore) PersistenceError() error {
	if store == nil {
		return nil
	}
	store.persistMu.Lock()
	defer store.persistMu.Unlock()
	return store.persistenceErr
}

func (store *WorldMapStore) Close(ctx context.Context) error {
	if store == nil {
		return nil
	}
	store.StartPersistence()
	store.persistStopOnce.Do(func() { close(store.persistStop) })
	select {
	case <-store.persistDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	store.persistMu.Lock()
	persistErr := store.persistenceErr
	store.persistMu.Unlock()
	var closeErr error
	if store.db != nil {
		store.dbCloseOnce.Do(func() { store.dbCloseErr = store.db.Close() })
		closeErr = store.dbCloseErr
	}
	return errors.Join(persistErr, closeErr)
}

func persistedWorldMapKey(worldID string, kingdomID KingdomID, key string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", CanonicalWorldID(worldID), kingdomID, key)
}
