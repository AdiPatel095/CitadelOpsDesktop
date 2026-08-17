package State

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	componentStateDirectory = "Components"
	componentManifestName   = "manifest.json"
)

type componentManifest struct {
	SchemaVersion      int               `json:"schemaVersion"`
	Revision           uint64            `json:"revision"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	SavedAt            time.Time         `json:"savedAt"`
	Files              map[string]string `json:"files"`
	CastleFiles        map[string]string `json:"castleFiles,omitempty"`
	InventoryFiles     map[string]string `json:"inventoryFiles,omitempty"`
	MapFiles           map[string]string `json:"mapFiles,omitempty"`
	StormFiles         map[string]string `json:"stormFiles,omitempty"`
	TowerCooldownFiles map[string]string `json:"towerCooldownFiles,omitempty"`
	TowerQueueFiles    map[string]string `json:"towerQueueFiles,omitempty"`
	ReportFiles        map[string]string `json:"reportFiles,omitempty"`
	EventScoreFiles    map[string]string `json:"eventScoreFiles,omitempty"`
	MovementFiles      map[string]string `json:"movementFiles,omitempty"`
	Partitioned        map[string]bool   `json:"partitioned,omitempty"`
}

// ComponentSnapshotWriter owns the durable manifest for one account data
// directory. Account persistence is single-writer, so retaining the last
// successfully fsynced manifest avoids rereading and decoding a potentially
// large shard index on every two-second group commit.
type ComponentSnapshotWriter struct {
	dataDir string
	mu      sync.Mutex
	current *componentManifest
}

func NewComponentSnapshotWriter(dataDir string) *ComponentSnapshotWriter {
	return &ComponentSnapshotWriter{dataDir: dataDir}
}

func (writer *ComponentSnapshotWriter) Save(event Event, dirty ComponentSet) error {
	if writer == nil {
		return fmt.Errorf("component snapshot writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	var next componentManifest
	if err := saveComponentSnapshot(writer.dataDir, event, dirty, writer.current, &next); err != nil {
		return err
	}
	writer.current = &next
	return nil
}

type persistedComponent struct {
	SchemaVersion int             `json:"schemaVersion"`
	Revision      uint64          `json:"revision"`
	SavedAt       time.Time       `json:"savedAt"`
	Component     Component       `json:"component"`
	Patch         *ComponentPatch `json:"patch"`
}

type persistedCastleIndex struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      uint64     `json:"revision"`
	SavedAt       time.Time  `json:"savedAt"`
	IDs           []CastleID `json:"ids"`
}

type persistedMovementIndex struct {
	SchemaVersion int          `json:"schemaVersion"`
	Revision      uint64       `json:"revision"`
	SavedAt       time.Time    `json:"savedAt"`
	IDs           []MovementID `json:"ids"`
}

type persistedMovement struct {
	SchemaVersion int           `json:"schemaVersion"`
	Revision      uint64        `json:"revision"`
	SavedAt       time.Time     `json:"savedAt"`
	ID            MovementID    `json:"id"`
	Movement      MovementState `json:"movement"`
}

type persistedCastleShard struct {
	SchemaVersion int         `json:"schemaVersion"`
	Revision      uint64      `json:"revision"`
	SavedAt       time.Time   `json:"savedAt"`
	ID            CastleID    `json:"id"`
	Castle        CastleState `json:"castle"`
}

type persistedInventoryIndex struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	SavedAt       time.Time `json:"savedAt"`
	Parts         []string  `json:"parts"`
}

type persistedInventoryPart struct {
	SchemaVersion int             `json:"schemaVersion"`
	Revision      uint64          `json:"revision"`
	SavedAt       time.Time       `json:"savedAt"`
	Part          string          `json:"part"`
	Patch         *InventoryPatch `json:"patch"`
}

type persistedMapIndex struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	SavedAt       time.Time `json:"savedAt"`
	Shards        []string  `json:"shards"`
}

type persistedMapShard struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Revision      uint64                    `json:"revision"`
	SavedAt       time.Time                 `json:"savedAt"`
	KingdomID     KingdomID                 `json:"kingdomId"`
	Shard         uint8                     `json:"shard"`
	Observations  map[string]MapObservation `json:"observations"`
}

type persistedStormIndex struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	SavedAt       time.Time `json:"savedAt"`
	Parts         []string  `json:"parts"`
}

type persistedStormMetadata struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      uint64     `json:"revision"`
	SavedAt       time.Time  `json:"savedAt"`
	Storm         StormState `json:"storm"`
}

type persistedStormTargets struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	SavedAt       time.Time `json:"savedAt"`
	Mode          string    `json:"mode,omitempty"`
	Keys          []string  `json:"keys"`
}

type persistedTowerCooldownIndex struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	SavedAt       time.Time `json:"savedAt"`
	Shards        []string  `json:"shards"`
}

type persistedTowerCooldownShard struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Revision      uint64                        `json:"revision"`
	SavedAt       time.Time                     `json:"savedAt"`
	Shard         uint8                         `json:"shard"`
	Cooldowns     map[string]TowerCooldownState `json:"cooldowns"`
}

type persistedTowerQueueIndex struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      uint64     `json:"revision"`
	SavedAt       time.Time  `json:"savedAt"`
	CursorVersion int        `json:"cursorVersion"`
	CastleIDs     []CastleID `json:"castleIds"`
}

type persistedTowerQueueCastle struct {
	SchemaVersion     int                       `json:"schemaVersion"`
	Revision          uint64                    `json:"revision"`
	SavedAt           time.Time                 `json:"savedAt"`
	CastleID          CastleID                  `json:"castleId"`
	Entries           []TowerQueueEntry         `json:"entries"`
	LastScannedAt     time.Time                 `json:"lastScannedAt,omitempty"`
	LastAttemptedAt   time.Time                 `json:"lastAttemptedAt,omitempty"`
	ConfirmedLaunches int64                     `json:"confirmedLaunches,omitempty"`
	Capacity          *TowerCapacityObservation `json:"capacity,omitempty"`
}

type persistedReportIndex struct {
	SchemaVersion      int       `json:"schemaVersion"`
	Revision           uint64    `json:"revision"`
	SavedAt            time.Time `json:"savedAt"`
	ActiveBattleReport int64     `json:"activeBattleReport,omitempty"`
	MessageIDs         []int64   `json:"messageIds"`
}

type persistedReportMessage struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Revision      uint64               `json:"revision"`
	SavedAt       time.Time            `json:"savedAt"`
	MessageID     int64                `json:"messageId"`
	Notice        *ReportNotice        `json:"notice,omitempty"`
	Spy           *SpyReportCapture    `json:"spy,omitempty"`
	Battle        *BattleReportCapture `json:"battle,omitempty"`
}

type persistedEventScoreIndex struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Revision      uint64                       `json:"revision"`
	SavedAt       time.Time                    `json:"savedAt"`
	ActiveEventID int64                        `json:"activeEventId,omitempty"`
	ShopByPackage map[PackageID]EventShopRoute `json:"shopByPackage"`
	Inventory     EventInventoryState          `json:"inventory"`
	EventIDs      []int64                      `json:"eventIds"`
}

type persistedEventScore struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Revision      uint64              `json:"revision"`
	SavedAt       time.Time           `json:"savedAt"`
	EventID       int64               `json:"eventId"`
	Score         *ScalableEventScore `json:"score,omitempty"`
	Activity      *EventActivityState `json:"activity,omitempty"`
	Ranking       *EventRankingState  `json:"ranking,omitempty"`
}

type inventoryPersistencePart struct {
	name string
	bit  inventoryMutationPart
}

var inventoryPersistenceParts = []inventoryPersistencePart{
	{name: "construction-items", bit: inventoryConstructionItemsMutable},
	{name: "construction-offers", bit: inventoryConstructionOffersMutable},
	{name: "equipment", bit: inventoryEquipmentMutable},
	{name: "gems", bit: inventoryGemsMutable},
	{name: "gem-stacks", bit: inventoryGemStacksMutable},
	{name: "items", bit: inventoryItemsMutable},
}

// SaveComponentSnapshot persists exactly the immutable generation referenced
// by event. Only dirty components are encoded. On the first component save all
// components are written once so a legacy GameState.json can be migrated
// without a second full-state clone.
func SaveComponentSnapshot(dataDir string, event Event, dirty ComponentSet) error {
	var next componentManifest
	return saveComponentSnapshot(dataDir, event, dirty, nil, &next)
}

func saveComponentSnapshot(
	dataDir string,
	event Event,
	dirty ComponentSet,
	cached *componentManifest,
	saved *componentManifest,
) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("state data directory is required")
	}
	if event.generation == nil || event.Revision == 0 || event.generation.state.Revision != event.Revision {
		return fmt.Errorf("state event does not retain committed generation %d", event.Revision)
	}
	if dirty == 0 || dirty&^AllComponents != 0 {
		return fmt.Errorf("valid dirty state components are required")
	}
	directory := componentStatePath(dataDir)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create component state directory: %w", err)
	}

	manifest := componentManifest{}
	if cached != nil {
		manifest = *cached
	} else {
		loaded, err := readComponentManifest(dataDir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			manifest = componentManifest{SchemaVersion: SchemaVersion, Files: map[string]string{}}
			dirty = AllComponents
		} else {
			manifest = loaded
		}
	}
	if event.Revision < manifest.Revision {
		if saved != nil {
			*saved = manifest
		}
		return nil
	}
	if manifest.Files == nil {
		manifest.Files = map[string]string{}
	}

	nextFiles := make(map[string]string, len(manifest.Files)+len(dirty.List()))
	for name, filename := range manifest.Files {
		nextFiles[name] = filename
	}
	nextCastleFiles := cloneStringMap(manifest.CastleFiles)
	nextInventoryFiles := cloneStringMap(manifest.InventoryFiles)
	nextMapFiles := cloneStringMap(manifest.MapFiles)
	nextStormFiles := cloneStringMap(manifest.StormFiles)
	nextTowerCooldownFiles := cloneStringMap(manifest.TowerCooldownFiles)
	nextTowerQueueFiles := cloneStringMap(manifest.TowerQueueFiles)
	nextReportFiles := cloneStringMap(manifest.ReportFiles)
	nextEventScoreFiles := cloneStringMap(manifest.EventScoreFiles)
	nextMovementFiles := cloneStringMap(manifest.MovementFiles)
	nextPartitioned := cloneBoolMap(manifest.Partitioned)
	oldReferences := componentManifestReferences(manifest)
	now := time.Now().UTC()
	for _, component := range dirty.List() {
		if component == ComponentMovements {
			filename, files, saveErr := saveMovementComponent(directory, event, nextMovementFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextMovementFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentCastles {
			filename, files, saveErr := saveCastleComponent(
				directory, event, nextCastleFiles, now,
			)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextCastleFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentInventory {
			filename, files, saveErr := saveInventoryComponent(
				directory, event, nextInventoryFiles, now,
			)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextInventoryFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentWorldMap {
			filename, files, saveErr := saveMapComponent(directory, event, nextMapFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextMapFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentStorm {
			filename, files, saveErr := saveStormComponent(directory, event, nextStormFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextStormFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentTowerCooldowns {
			filename, files, saveErr := saveTowerCooldownComponent(
				directory, event, nextTowerCooldownFiles, now,
			)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextTowerCooldownFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentTowerQueue {
			filename, files, saveErr := saveTowerQueueComponent(directory, event, nextTowerQueueFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextTowerQueueFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentReports {
			filename, files, saveErr := saveReportComponent(directory, event, nextReportFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextReportFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		if component == ComponentEventScores {
			filename, files, saveErr := saveEventScoreComponent(directory, event, nextEventScoreFiles, now)
			if saveErr != nil {
				return saveErr
			}
			nextFiles[component.String()] = filename
			nextEventScoreFiles = files
			nextPartitioned[component.String()] = true
			continue
		}
		patch := componentPatch(event.generation, Components(component), componentChanges{
			replaceMap: false,
		})
		document := persistedComponent{
			SchemaVersion: SchemaVersion,
			Revision:      event.Revision,
			SavedAt:       now,
			Component:     component,
			Patch:         patch,
		}
		contents, marshalErr := json.Marshal(document)
		if marshalErr != nil {
			return fmt.Errorf("encode %s state component: %w", component, marshalErr)
		}
		filename := fmt.Sprintf("%s-%020d.json", component.String(), event.Revision)
		if writeErr := writeAtomicStateFile(directory, filename, contents); writeErr != nil {
			return fmt.Errorf("persist %s state component: %w", component, writeErr)
		}
		nextFiles[component.String()] = filename
	}
	for _, component := range AllComponents.List() {
		if strings.TrimSpace(nextFiles[component.String()]) == "" {
			return fmt.Errorf("component manifest is missing %s", component)
		}
	}

	next := componentManifest{
		SchemaVersion:      SchemaVersion,
		Revision:           event.Revision,
		UpdatedAt:          event.generation.state.UpdatedAt,
		SavedAt:            now,
		Files:              nextFiles,
		CastleFiles:        nextCastleFiles,
		InventoryFiles:     nextInventoryFiles,
		MapFiles:           nextMapFiles,
		StormFiles:         nextStormFiles,
		TowerCooldownFiles: nextTowerCooldownFiles,
		TowerQueueFiles:    nextTowerQueueFiles,
		ReportFiles:        nextReportFiles,
		EventScoreFiles:    nextEventScoreFiles,
		MovementFiles:      nextMovementFiles,
		Partitioned:        nextPartitioned,
	}
	contents, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("encode component state manifest: %w", err)
	}
	if err := writeAtomicStateFile(directory, componentManifestName, contents); err != nil {
		return fmt.Errorf("persist component state manifest: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync component state directory: %w", err)
	}

	// The new manifest is durable. Files it no longer references are now safe
	// to remove; an interrupted cleanup only leaves harmless old revisions.
	nextReferences := componentManifestReferences(next)
	for oldFilename := range oldReferences {
		if _, retained := nextReferences[oldFilename]; retained || !safeComponentFilename(oldFilename) {
			continue
		}
		_ = os.Remove(filepath.Join(directory, oldFilename))
	}
	if saved != nil {
		*saved = next
	}
	return nil
}

func saveMovementComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	ids := append([]MovementID(nil), event.movementIDs...)
	if len(files) == 0 || event.replaceMovements || len(ids) == 0 {
		files = map[string]string{}
		state.RangeMovements(func(id MovementID, _ MovementState) bool {
			ids = append(ids, id)
			return true
		})
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	}
	for _, id := range ids {
		if id <= 0 {
			return "", nil, fmt.Errorf("movements contain invalid id %d", id)
		}
		key := strconv.FormatInt(int64(id), 10)
		movement, found := state.LookupMovement(id)
		if !found {
			delete(files, key)
			continue
		}
		document := persistedMovement{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			ID: id, Movement: movement,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode movement %d: %w", id, err)
		}
		filename := fmt.Sprintf("movement-%d-%020d.json", id, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist movement %d: %w", id, err)
		}
		files[key] = filename
	}
	indexIDs := make([]MovementID, 0, len(files))
	for key := range files {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, fmt.Errorf("movement manifest contains invalid id %q", key)
		}
		indexIDs = append(indexIDs, MovementID(id))
	}
	sort.Slice(indexIDs, func(left, right int) bool { return indexIDs[left] < indexIDs[right] })
	index := persistedMovementIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, IDs: indexIDs,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode movement index: %w", err)
	}
	filename := fmt.Sprintf("movements-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist movement index: %w", err)
	}
	return filename, files, nil
}

func saveCastleComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	ids := append([]CastleID(nil), event.castleIDs...)
	if len(files) == 0 || event.replaceCastles || len(ids) == 0 {
		files = map[string]string{}
		ids = make([]CastleID, 0, len(state.Castles))
		for id := range state.Castles {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	}
	for _, id := range ids {
		key := strconv.FormatInt(int64(id), 10)
		castle, found := state.Castles[id]
		if !found {
			delete(files, key)
			continue
		}
		document := persistedCastleShard{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			ID: id, Castle: castle,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode castle %d state shard: %w", id, err)
		}
		filename := fmt.Sprintf("castle-%d-%020d.json", id, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist castle %d state shard: %w", id, err)
		}
		files[key] = filename
	}

	indexIDs := make([]CastleID, 0, len(files))
	for key := range files {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, fmt.Errorf("castle shard index contains invalid id %q", key)
		}
		indexIDs = append(indexIDs, CastleID(id))
	}
	sort.Slice(indexIDs, func(left, right int) bool { return indexIDs[left] < indexIDs[right] })
	index := persistedCastleIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, IDs: indexIDs,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode castle state index: %w", err)
	}
	filename := fmt.Sprintf("castles-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist castle state index: %w", err)
	}
	return filename, files, nil
}

func saveInventoryComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	files := cloneStringMap(currentFiles)
	parts := event.inventoryParts
	if len(files) != len(inventoryPersistenceParts) || event.replaceInventory || parts == 0 {
		files = map[string]string{}
		parts = allInventoryMutationParts()
	}
	for _, descriptor := range inventoryPersistenceParts {
		if parts&descriptor.bit == 0 {
			continue
		}
		patch := inventoryComponentPatch(&event.generation.state.Inventory, componentChanges{
			inventoryParts:   descriptor.bit,
			replaceEquipment: true,
			replaceGems:      true,
			replaceItems:     true,
		})
		document := persistedInventoryPart{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			Part: descriptor.name, Patch: patch,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode inventory %s state part: %w", descriptor.name, err)
		}
		filename := fmt.Sprintf("inventory-%s-%020d.json", descriptor.name, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist inventory %s state part: %w", descriptor.name, err)
		}
		files[descriptor.name] = filename
	}
	partNames := make([]string, 0, len(files))
	for name := range files {
		partNames = append(partNames, name)
	}
	sort.Strings(partNames)
	index := persistedInventoryIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, Parts: partNames,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode inventory state index: %w", err)
	}
	filename := fmt.Sprintf("inventory-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist inventory state index: %w", err)
	}
	return filename, files, nil
}

func saveMapComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	dirtyShards := map[string]struct{}{}
	if len(files) == 0 || event.replaceMap || len(event.mapChanges) == 0 {
		files = map[string]string{}
		state.rangePrivateMapShards(func(kingdomID KingdomID, shard uint8) {
			dirtyShards[mapPersistenceShardKey(kingdomID, shard)] = struct{}{}
		})
	} else {
		for _, change := range event.mapChanges {
			if change.Key == "" {
				continue
			}
			dirtyShards[mapPersistenceShardKey(change.KingdomID, mapShardIndex(change.Key))] = struct{}{}
		}
	}
	keys := make([]string, 0, len(dirtyShards))
	for key := range dirtyShards {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		kingdomID, shard, err := parseMapPersistenceShardKey(key)
		if err != nil {
			return "", nil, err
		}
		observations := state.privateMapShard(kingdomID, shard)
		if len(observations) == 0 {
			delete(files, key)
			continue
		}
		document := persistedMapShard{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			KingdomID: kingdomID, Shard: shard, Observations: observations,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode map state shard %s: %w", key, err)
		}
		filename := fmt.Sprintf("map-%d-%03d-%020d.json", kingdomID, shard, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist map state shard %s: %w", key, err)
		}
		files[key] = filename
	}

	indexKeys := make([]string, 0, len(files))
	for key := range files {
		if _, _, err := parseMapPersistenceShardKey(key); err != nil {
			return "", nil, err
		}
		indexKeys = append(indexKeys, key)
	}
	sort.Strings(indexKeys)
	index := persistedMapIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, Shards: indexKeys,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode map state index: %w", err)
	}
	filename := fmt.Sprintf("map-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist map state index: %w", err)
	}
	return filename, files, nil
}

func saveStormComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	const metadataPart = "metadata"
	const targetsPart = "targets"
	files := cloneStringMap(currentFiles)
	state := event.generation.state

	metadata := state.Storm
	metadata.Map.Targets = nil
	metadataDocument := persistedStormMetadata{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, Storm: metadata,
	}
	contents, err := json.Marshal(metadataDocument)
	if err != nil {
		return "", nil, fmt.Errorf("encode Storm metadata state part: %w", err)
	}
	metadataFilename := fmt.Sprintf("storm-metadata-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, metadataFilename, contents); err != nil {
		return "", nil, fmt.Errorf("persist Storm metadata state part: %w", err)
	}
	files[metadataPart] = metadataFilename

	if files[targetsPart] == "" || event.replaceStorm || len(event.stormTargetKeys) > 0 {
		targetDocument := persistedStormTargets{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			Mode: "suppressed", Keys: state.stormTargetKeys(),
		}
		contents, err = json.Marshal(targetDocument)
		if err != nil {
			return "", nil, fmt.Errorf("encode Storm target index state part: %w", err)
		}
		targetFilename := fmt.Sprintf("storm-targets-%020d.json", event.Revision)
		if err := writeAtomicStateFile(directory, targetFilename, contents); err != nil {
			return "", nil, fmt.Errorf("persist Storm target index state part: %w", err)
		}
		files[targetsPart] = targetFilename
	}

	parts := []string{metadataPart, targetsPart}
	index := persistedStormIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, Parts: parts,
	}
	contents, err = json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode Storm state index: %w", err)
	}
	filename := fmt.Sprintf("storm-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist Storm state index: %w", err)
	}
	return filename, files, nil
}

func saveTowerCooldownComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	dirtyShards := map[uint8]struct{}{}
	if len(files) == 0 || event.replaceCooldowns || len(event.towerCooldownKeys) == 0 {
		files = map[string]string{}
		state.rangeTowerCooldownShards(func(shard uint8) { dirtyShards[shard] = struct{}{} })
	} else {
		for _, key := range event.towerCooldownKeys {
			if key != "" {
				dirtyShards[mapShardIndex(key)] = struct{}{}
			}
		}
	}
	shards := make([]int, 0, len(dirtyShards))
	for shard := range dirtyShards {
		shards = append(shards, int(shard))
	}
	sort.Ints(shards)
	for _, rawShard := range shards {
		shard := uint8(rawShard)
		key := strconv.Itoa(rawShard)
		cooldowns := state.towerCooldownShard(shard)
		if len(cooldowns) == 0 {
			delete(files, key)
			continue
		}
		document := persistedTowerCooldownShard{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
			Shard: shard, Cooldowns: cooldowns,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode tower cooldown shard %d: %w", shard, err)
		}
		filename := fmt.Sprintf("tower-cooldowns-%03d-%020d.json", shard, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist tower cooldown shard %d: %w", shard, err)
		}
		files[key] = filename
	}
	indexKeys := make([]int, 0, len(files))
	for key := range files {
		shard, err := strconv.ParseUint(key, 10, 8)
		if err != nil {
			return "", nil, fmt.Errorf("tower cooldown manifest contains invalid shard %q", key)
		}
		indexKeys = append(indexKeys, int(shard))
	}
	sort.Ints(indexKeys)
	indexShards := make([]string, 0, len(indexKeys))
	for _, shard := range indexKeys {
		indexShards = append(indexShards, strconv.Itoa(shard))
	}
	index := persistedTowerCooldownIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, Shards: indexShards,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode tower cooldown index: %w", err)
	}
	filename := fmt.Sprintf("tower-cooldowns-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist tower cooldown index: %w", err)
	}
	return filename, files, nil
}

func saveTowerQueueComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	ids := append([]CastleID(nil), event.towerQueueCastles...)
	if len(files) == 0 || event.replaceTowerQueue || len(ids) == 0 {
		files = map[string]string{}
		ids = state.towerQueueCastleIDs()
	}
	for _, castleID := range ids {
		if castleID <= 0 {
			return "", nil, fmt.Errorf("tower queue contains invalid castle id %d", castleID)
		}
		entries, hasEntries := state.TowerQueue.EntriesByCastle[castleID]
		lastScannedAt, hasLastScanned := state.TowerQueue.LastScannedAt[castleID]
		lastAttemptedAt, hasLastAttempted := state.TowerQueue.LastAttemptedAt[castleID]
		confirmed, hasConfirmed := state.TowerQueue.ConfirmedLaunchesByCastle[castleID]
		capacity, hasCapacity := state.TowerQueue.CapacityByCastle[castleID]
		key := strconv.FormatInt(int64(castleID), 10)
		if !hasEntries && !hasLastScanned && !hasLastAttempted && !hasConfirmed && !hasCapacity {
			delete(files, key)
			continue
		}
		var capacityPointer *TowerCapacityObservation
		if hasCapacity {
			value := capacity
			capacityPointer = &value
		}
		document := persistedTowerQueueCastle{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, CastleID: castleID,
			Entries: entries, LastScannedAt: lastScannedAt, LastAttemptedAt: lastAttemptedAt,
			ConfirmedLaunches: confirmed, Capacity: capacityPointer,
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode tower queue castle %d: %w", castleID, err)
		}
		filename := fmt.Sprintf("tower-queue-%d-%020d.json", castleID, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist tower queue castle %d: %w", castleID, err)
		}
		files[key] = filename
	}
	indexIDs := make([]CastleID, 0, len(files))
	for key := range files {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, fmt.Errorf("tower queue manifest contains invalid castle id %q", key)
		}
		indexIDs = append(indexIDs, CastleID(id))
	}
	sort.Slice(indexIDs, func(left, right int) bool { return indexIDs[left] < indexIDs[right] })
	index := persistedTowerQueueIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
		CursorVersion: state.TowerQueue.CursorVersion, CastleIDs: indexIDs,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode tower queue index: %w", err)
	}
	filename := fmt.Sprintf("tower-queue-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist tower queue index: %w", err)
	}
	return filename, files, nil
}

func saveReportComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	ids := append([]int64(nil), event.reportMessageIDs...)
	if len(files) == 0 || event.replaceReports {
		files = map[string]string{}
		ids = state.reportMessageIDs()
	}
	for _, messageID := range ids {
		if messageID <= 0 {
			return "", nil, fmt.Errorf("reports contain invalid message id %d", messageID)
		}
		record, found := state.reportRecord(messageID)
		key := strconv.FormatInt(messageID, 10)
		if !found {
			delete(files, key)
			continue
		}
		document := persistedReportMessage{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, MessageID: messageID,
		}
		if record.HasNotice {
			value := record.Notice
			document.Notice = &value
		}
		if record.HasSpy {
			value := record.Spy
			document.Spy = &value
		}
		if record.HasBattle {
			value := record.Battle
			document.Battle = &value
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode report message %d: %w", messageID, err)
		}
		filename := fmt.Sprintf("report-%d-%020d.json", messageID, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist report message %d: %w", messageID, err)
		}
		files[key] = filename
	}
	indexIDs := make([]int64, 0, len(files))
	for key := range files {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, fmt.Errorf("report manifest contains invalid message id %q", key)
		}
		indexIDs = append(indexIDs, id)
	}
	sort.Slice(indexIDs, func(left, right int) bool { return indexIDs[left] < indexIDs[right] })
	index := persistedReportIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
		ActiveBattleReport: state.Reports.ActiveBattleReport, MessageIDs: indexIDs,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode report index: %w", err)
	}
	filename := fmt.Sprintf("reports-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist report index: %w", err)
	}
	return filename, files, nil
}

func saveEventScoreComponent(
	directory string,
	event Event,
	currentFiles map[string]string,
	savedAt time.Time,
) (string, map[string]string, error) {
	state := event.generation.state
	files := cloneStringMap(currentFiles)
	ids := append([]int64(nil), event.eventScoreIDs...)
	if len(files) == 0 || event.replaceEventScores {
		files = map[string]string{}
		ids = state.eventScoreIDs()
	}
	for _, eventID := range ids {
		if eventID <= 0 {
			return "", nil, fmt.Errorf("event scores contain invalid event id %d", eventID)
		}
		score, hasScore := state.LookupScalableEventScore(eventID)
		activity, hasActivity := state.LookupEventActivity(eventID)
		ranking, hasRanking := state.LookupEventRanking(eventID)
		key := strconv.FormatInt(eventID, 10)
		if !hasScore && !hasActivity && !hasRanking {
			delete(files, key)
			continue
		}
		document := persistedEventScore{
			SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt, EventID: eventID,
		}
		if hasScore {
			value := score
			document.Score = &value
		}
		if hasActivity {
			value := activity
			document.Activity = &value
		}
		if hasRanking {
			value := ranking
			document.Ranking = &value
		}
		contents, err := json.Marshal(document)
		if err != nil {
			return "", nil, fmt.Errorf("encode event score %d: %w", eventID, err)
		}
		filename := fmt.Sprintf("event-score-%d-%020d.json", eventID, event.Revision)
		if err := writeAtomicStateFile(directory, filename, contents); err != nil {
			return "", nil, fmt.Errorf("persist event score %d: %w", eventID, err)
		}
		files[key] = filename
	}
	indexIDs := make([]int64, 0, len(files))
	for key := range files {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, fmt.Errorf("event score manifest contains invalid event id %q", key)
		}
		indexIDs = append(indexIDs, id)
	}
	sort.Slice(indexIDs, func(left, right int) bool { return indexIDs[left] < indexIDs[right] })
	index := persistedEventScoreIndex{
		SchemaVersion: SchemaVersion, Revision: event.Revision, SavedAt: savedAt,
		ActiveEventID: state.EventScores.ActiveEventID, ShopByPackage: state.EventScores.ShopByPackage,
		Inventory: state.EventScores.Inventory,
		EventIDs:  indexIDs,
	}
	contents, err := json.Marshal(index)
	if err != nil {
		return "", nil, fmt.Errorf("encode event score index: %w", err)
	}
	filename := fmt.Sprintf("event-scores-index-%020d.json", event.Revision)
	if err := writeAtomicStateFile(directory, filename, contents); err != nil {
		return "", nil, fmt.Errorf("persist event score index: %w", err)
	}
	return filename, files, nil
}

func mapPersistenceShardKey(kingdomID KingdomID, shard uint8) string {
	return fmt.Sprintf("%d:%d", kingdomID, shard)
}

func parseMapPersistenceShardKey(key string) (KingdomID, uint8, error) {
	kingdomRaw, shardRaw, found := strings.Cut(key, ":")
	if !found || kingdomRaw == "" || shardRaw == "" {
		return 0, 0, fmt.Errorf("map shard manifest contains invalid key %q", key)
	}
	kingdomID, err := strconv.ParseInt(kingdomRaw, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("map shard manifest contains invalid kingdom %q", kingdomRaw)
	}
	shard, err := strconv.ParseUint(shardRaw, 10, 8)
	if err != nil {
		return 0, 0, fmt.Errorf("map shard manifest contains invalid shard %q", shardRaw)
	}
	return KingdomID(kingdomID), uint8(shard), nil
}

func allInventoryMutationParts() inventoryMutationPart {
	var parts inventoryMutationPart
	for _, descriptor := range inventoryPersistenceParts {
		parts |= descriptor.bit
	}
	return parts
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func componentManifestReferences(manifest componentManifest) map[string]struct{} {
	references := map[string]struct{}{}
	for _, files := range []map[string]string{
		manifest.Files, manifest.CastleFiles, manifest.InventoryFiles, manifest.MapFiles, manifest.StormFiles,
		manifest.TowerCooldownFiles,
		manifest.TowerQueueFiles,
		manifest.ReportFiles,
		manifest.EventScoreFiles,
		manifest.MovementFiles,
	} {
		for _, filename := range files {
			if safeComponentFilename(filename) {
				references[filename] = struct{}{}
			}
		}
	}
	return references
}

func loadComponentSnapshot(dataDir string) (GameState, error) {
	manifest, err := readComponentManifest(dataDir)
	if err != nil {
		return GameState{}, err
	}
	state := NewGameState()
	for _, component := range AllComponents.List() {
		filename := manifest.Files[component.String()]
		if !safeComponentFilename(filename) {
			return GameState{}, fmt.Errorf("component manifest has invalid %s filename %q", component, filename)
		}
		if component == ComponentMovements && manifest.Partitioned[component.String()] {
			movements, loadErr := loadMovementComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Movements = movements
			continue
		}
		if component == ComponentCastles && manifest.Partitioned[component.String()] {
			castles, loadErr := loadCastleComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Castles = castles
			continue
		}
		if component == ComponentInventory && manifest.Partitioned[component.String()] {
			inventory, loadErr := loadInventoryComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Inventory = inventory
			continue
		}
		if component == ComponentWorldMap && manifest.Partitioned[component.String()] {
			worldMap, loadErr := loadMapComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Map = worldMap
			continue
		}
		if component == ComponentStorm && manifest.Partitioned[component.String()] {
			storm, loadErr := loadStormComponent(componentStatePath(dataDir), manifest, filename, state)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Storm = storm
			continue
		}
		if component == ComponentTowerCooldowns && manifest.Partitioned[component.String()] {
			cooldowns, loadErr := loadTowerCooldownComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.TowerCooldowns = cooldowns
			continue
		}
		if component == ComponentTowerQueue && manifest.Partitioned[component.String()] {
			queue, loadErr := loadTowerQueueComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.TowerQueue = queue
			continue
		}
		if component == ComponentReports && manifest.Partitioned[component.String()] {
			reports, loadErr := loadReportComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.Reports = reports
			continue
		}
		if component == ComponentEventScores && manifest.Partitioned[component.String()] {
			eventScores, loadErr := loadEventScoreComponent(componentStatePath(dataDir), manifest, filename)
			if loadErr != nil {
				return GameState{}, loadErr
			}
			state.EventScores = eventScores
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(componentStatePath(dataDir), filename))
		if readErr != nil {
			return GameState{}, fmt.Errorf("read %s state component: %w", component, readErr)
		}
		var document persistedComponent
		if unmarshalErr := json.Unmarshal(contents, &document); unmarshalErr != nil {
			return GameState{}, fmt.Errorf("decode %s state component: %w", component, unmarshalErr)
		}
		if document.SchemaVersion != SchemaVersion || document.Component != component || document.Patch == nil {
			return GameState{}, fmt.Errorf("%s state component metadata is invalid", component)
		}
		if document.Revision > manifest.Revision || document.Patch.Revision != document.Revision {
			return GameState{}, fmt.Errorf("%s state component revision is inconsistent with manifest", component)
		}
		if err := applyComponentPatch(&state, component, document.Patch); err != nil {
			return GameState{}, err
		}
	}
	state.SchemaVersion = manifest.SchemaVersion
	state.Revision = manifest.Revision
	state.UpdatedAt = manifest.UpdatedAt
	return prepareLoadedState(state), nil
}

func loadMovementComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (map[MovementID]MovementState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return nil, fmt.Errorf("read movement index: %w", err)
	}
	var index persistedMovementIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return nil, fmt.Errorf("decode movement index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision || !sort.SliceIsSorted(index.IDs, func(left, right int) bool {
		return index.IDs[left] < index.IDs[right]
	}) {
		return nil, fmt.Errorf("movement index metadata is invalid")
	}
	expected := make([]MovementID, 0, len(manifest.MovementFiles))
	for key := range manifest.MovementFiles {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("movement manifest contains invalid id %q", key)
		}
		expected = append(expected, MovementID(id))
	}
	sort.Slice(expected, func(left, right int) bool { return expected[left] < expected[right] })
	if !reflect.DeepEqual(index.IDs, expected) {
		return nil, fmt.Errorf("movement index does not match its manifest")
	}
	movements := make(map[MovementID]MovementState, len(index.IDs))
	for _, id := range index.IDs {
		filename := manifest.MovementFiles[strconv.FormatInt(int64(id), 10)]
		if !safeComponentFilename(filename) {
			return nil, fmt.Errorf("movement %d filename is invalid", id)
		}
		contents, err := os.ReadFile(filepath.Join(directory, filename))
		if err != nil {
			return nil, fmt.Errorf("read movement %d: %w", id, err)
		}
		var document persistedMovement
		if err := json.Unmarshal(contents, &document); err != nil {
			return nil, fmt.Errorf("decode movement %d: %w", id, err)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision ||
			document.ID != id || document.Movement.ID != id {
			return nil, fmt.Errorf("movement %d metadata is invalid", id)
		}
		movements[id] = document.Movement
	}
	return movements, nil
}

func loadCastleComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (map[CastleID]CastleState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return nil, fmt.Errorf("read castle state index: %w", err)
	}
	var index persistedCastleIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return nil, fmt.Errorf("decode castle state index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return nil, fmt.Errorf("castle state index metadata is invalid")
	}
	expectedIDs := make([]CastleID, 0, len(manifest.CastleFiles))
	for key := range manifest.CastleFiles {
		id, parseErr := strconv.ParseInt(key, 10, 64)
		if parseErr != nil || id <= 0 {
			return nil, fmt.Errorf("castle state manifest contains invalid id %q", key)
		}
		expectedIDs = append(expectedIDs, CastleID(id))
	}
	sort.Slice(expectedIDs, func(left, right int) bool { return expectedIDs[left] < expectedIDs[right] })
	if !reflect.DeepEqual(index.IDs, expectedIDs) {
		return nil, fmt.Errorf("castle state index does not match its manifest")
	}
	castles := make(map[CastleID]CastleState, len(index.IDs))
	for _, id := range index.IDs {
		filename := manifest.CastleFiles[strconv.FormatInt(int64(id), 10)]
		if !safeComponentFilename(filename) {
			return nil, fmt.Errorf("castle %d state shard filename is invalid", id)
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, filename))
		if readErr != nil {
			return nil, fmt.Errorf("read castle %d state shard: %w", id, readErr)
		}
		var document persistedCastleShard
		if decodeErr := json.Unmarshal(contents, &document); decodeErr != nil {
			return nil, fmt.Errorf("decode castle %d state shard: %w", id, decodeErr)
		}
		if document.SchemaVersion != SchemaVersion || document.ID != id || document.Revision > index.Revision {
			return nil, fmt.Errorf("castle %d state shard metadata is invalid", id)
		}
		castles[id] = document.Castle
	}
	return castles, nil
}

func loadInventoryComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (InventoryState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return InventoryState{}, fmt.Errorf("read inventory state index: %w", err)
	}
	var index persistedInventoryIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return InventoryState{}, fmt.Errorf("decode inventory state index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return InventoryState{}, fmt.Errorf("inventory state index metadata is invalid")
	}
	expectedParts := make([]string, 0, len(inventoryPersistenceParts))
	for _, descriptor := range inventoryPersistenceParts {
		expectedParts = append(expectedParts, descriptor.name)
	}
	sort.Strings(expectedParts)
	if !reflect.DeepEqual(index.Parts, expectedParts) || len(manifest.InventoryFiles) != len(expectedParts) {
		return InventoryState{}, fmt.Errorf("inventory state index does not contain every required part")
	}
	inventory := NewGameState().Inventory
	for _, part := range expectedParts {
		filename := manifest.InventoryFiles[part]
		if !safeComponentFilename(filename) {
			return InventoryState{}, fmt.Errorf("inventory %s state filename is invalid", part)
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, filename))
		if readErr != nil {
			return InventoryState{}, fmt.Errorf("read inventory %s state part: %w", part, readErr)
		}
		var document persistedInventoryPart
		if decodeErr := json.Unmarshal(contents, &document); decodeErr != nil {
			return InventoryState{}, fmt.Errorf("decode inventory %s state part: %w", part, decodeErr)
		}
		if document.SchemaVersion != SchemaVersion || document.Part != part || document.Patch == nil ||
			document.Revision > index.Revision {
			return InventoryState{}, fmt.Errorf("inventory %s state part metadata is invalid", part)
		}
		if applyErr := applyInventoryPersistencePart(&inventory, part, document.Patch); applyErr != nil {
			return InventoryState{}, applyErr
		}
	}
	return inventory, nil
}

func loadMapComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (WorldMap, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return nil, fmt.Errorf("read map state index: %w", err)
	}
	var index persistedMapIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return nil, fmt.Errorf("decode map state index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return nil, fmt.Errorf("map state index metadata is invalid")
	}
	expected := make([]string, 0, len(manifest.MapFiles))
	for key := range manifest.MapFiles {
		if _, _, parseErr := parseMapPersistenceShardKey(key); parseErr != nil {
			return nil, parseErr
		}
		expected = append(expected, key)
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(index.Shards, expected) {
		return nil, fmt.Errorf("map state index does not match its manifest")
	}
	worldMap := WorldMap{}
	for _, key := range index.Shards {
		kingdomID, shard, parseErr := parseMapPersistenceShardKey(key)
		if parseErr != nil {
			return nil, parseErr
		}
		filename := manifest.MapFiles[key]
		if !safeComponentFilename(filename) {
			return nil, fmt.Errorf("map state shard %s filename is invalid", key)
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, filename))
		if readErr != nil {
			return nil, fmt.Errorf("read map state shard %s: %w", key, readErr)
		}
		var document persistedMapShard
		if decodeErr := json.Unmarshal(contents, &document); decodeErr != nil {
			return nil, fmt.Errorf("decode map state shard %s: %w", key, decodeErr)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision ||
			document.KingdomID != kingdomID || document.Shard != shard {
			return nil, fmt.Errorf("map state shard %s metadata is invalid", key)
		}
		for coordinate, observation := range document.Observations {
			if coordinate == "" || mapShardIndex(coordinate) != shard || observation.KingdomID != kingdomID {
				return nil, fmt.Errorf("map state shard %s contains an invalid observation", key)
			}
			if worldMap[kingdomID] == nil {
				worldMap[kingdomID] = map[string]MapObservation{}
			}
			worldMap[kingdomID][coordinate] = observation
		}
	}
	return worldMap, nil
}

func loadStormComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
	state GameState,
) (StormState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return StormState{}, fmt.Errorf("read Storm state index: %w", err)
	}
	var index persistedStormIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return StormState{}, fmt.Errorf("decode Storm state index: %w", err)
	}
	expectedParts := []string{"metadata", "targets"}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision ||
		!reflect.DeepEqual(index.Parts, expectedParts) || len(manifest.StormFiles) != len(expectedParts) {
		return StormState{}, fmt.Errorf("Storm state index metadata is invalid")
	}

	metadataFilename := manifest.StormFiles["metadata"]
	if !safeComponentFilename(metadataFilename) {
		return StormState{}, fmt.Errorf("Storm metadata state filename is invalid")
	}
	contents, err = os.ReadFile(filepath.Join(directory, metadataFilename))
	if err != nil {
		return StormState{}, fmt.Errorf("read Storm metadata state part: %w", err)
	}
	var metadata persistedStormMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return StormState{}, fmt.Errorf("decode Storm metadata state part: %w", err)
	}
	if metadata.SchemaVersion != SchemaVersion || metadata.Revision > index.Revision {
		return StormState{}, fmt.Errorf("Storm metadata state part is invalid")
	}
	metadata.Storm.Map.Targets = map[string]MapObservation{}

	targetFilename := manifest.StormFiles["targets"]
	if !safeComponentFilename(targetFilename) {
		return StormState{}, fmt.Errorf("Storm target state filename is invalid")
	}
	contents, err = os.ReadFile(filepath.Join(directory, targetFilename))
	if err != nil {
		return StormState{}, fmt.Errorf("read Storm target state part: %w", err)
	}
	var targets persistedStormTargets
	if err := json.Unmarshal(contents, &targets); err != nil {
		return StormState{}, fmt.Errorf("decode Storm target state part: %w", err)
	}
	if targets.SchemaVersion != SchemaVersion || targets.Revision > index.Revision || !sort.StringsAreSorted(targets.Keys) {
		return StormState{}, fmt.Errorf("Storm target state part is invalid")
	}
	previous := ""
	for _, key := range targets.Keys {
		if key == "" || key == previous {
			return StormState{}, fmt.Errorf("Storm target state part contains an invalid key")
		}
		previous = key
		if targets.Mode == "suppressed" {
			metadata.Storm.Map.suppressedTargets = append(metadata.Storm.Map.suppressedTargets, key)
			continue
		}
		// Pre-2.3 component snapshots used Keys as positive membership. Preserve
		// that migration path once, then the runtime compacts it into shared map
		// facts plus an ordinarily empty private negative overlay.
		observation, found := state.Map[stormKingdomID][key]
		if found && (observation.TypeID == MapTypeStormIsland || observation.TypeID == MapTypeStormFort) {
			metadata.Storm.Map.Targets[key] = observation
		}
	}
	return metadata.Storm, nil
}

func loadTowerCooldownComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (map[string]TowerCooldownState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return nil, fmt.Errorf("read tower cooldown index: %w", err)
	}
	var index persistedTowerCooldownIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return nil, fmt.Errorf("decode tower cooldown index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return nil, fmt.Errorf("tower cooldown index metadata is invalid")
	}
	expected := make([]string, 0, len(manifest.TowerCooldownFiles))
	for key := range manifest.TowerCooldownFiles {
		if _, err := strconv.ParseUint(key, 10, 8); err != nil {
			return nil, fmt.Errorf("tower cooldown manifest contains invalid shard %q", key)
		}
		expected = append(expected, key)
	}
	sort.Slice(expected, func(left, right int) bool {
		leftValue, _ := strconv.Atoi(expected[left])
		rightValue, _ := strconv.Atoi(expected[right])
		return leftValue < rightValue
	})
	if !reflect.DeepEqual(index.Shards, expected) {
		return nil, fmt.Errorf("tower cooldown index does not match its manifest")
	}
	result := map[string]TowerCooldownState{}
	for _, key := range index.Shards {
		rawShard, _ := strconv.ParseUint(key, 10, 8)
		shard := uint8(rawShard)
		filename := manifest.TowerCooldownFiles[key]
		if !safeComponentFilename(filename) {
			return nil, fmt.Errorf("tower cooldown shard %s filename is invalid", key)
		}
		contents, err := os.ReadFile(filepath.Join(directory, filename))
		if err != nil {
			return nil, fmt.Errorf("read tower cooldown shard %s: %w", key, err)
		}
		var document persistedTowerCooldownShard
		if err := json.Unmarshal(contents, &document); err != nil {
			return nil, fmt.Errorf("decode tower cooldown shard %s: %w", key, err)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision || document.Shard != shard {
			return nil, fmt.Errorf("tower cooldown shard %s metadata is invalid", key)
		}
		for cooldownKey, cooldown := range document.Cooldowns {
			if cooldownKey == "" || mapShardIndex(cooldownKey) != shard {
				return nil, fmt.Errorf("tower cooldown shard %s contains an invalid key", key)
			}
			result[cooldownKey] = cooldown
		}
	}
	return result, nil
}

func loadTowerQueueComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (TowerQueueState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return TowerQueueState{}, fmt.Errorf("read tower queue index: %w", err)
	}
	var index persistedTowerQueueIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return TowerQueueState{}, fmt.Errorf("decode tower queue index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return TowerQueueState{}, fmt.Errorf("tower queue index metadata is invalid")
	}
	expectedIDs := make([]CastleID, 0, len(manifest.TowerQueueFiles))
	for key := range manifest.TowerQueueFiles {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return TowerQueueState{}, fmt.Errorf("tower queue manifest contains invalid castle id %q", key)
		}
		expectedIDs = append(expectedIDs, CastleID(id))
	}
	sort.Slice(expectedIDs, func(left, right int) bool { return expectedIDs[left] < expectedIDs[right] })
	if !reflect.DeepEqual(index.CastleIDs, expectedIDs) {
		return TowerQueueState{}, fmt.Errorf("tower queue index does not match its manifest")
	}
	queue := TowerQueueState{
		EntriesByCastle: map[CastleID][]TowerQueueEntry{}, LastScannedAt: map[CastleID]time.Time{},
		LastAttemptedAt: map[CastleID]time.Time{}, ConfirmedLaunchesByCastle: map[CastleID]int64{},
		CursorVersion: index.CursorVersion, CapacityByCastle: map[CastleID]TowerCapacityObservation{},
	}
	for _, castleID := range index.CastleIDs {
		filename := manifest.TowerQueueFiles[strconv.FormatInt(int64(castleID), 10)]
		if !safeComponentFilename(filename) {
			return TowerQueueState{}, fmt.Errorf("tower queue castle %d filename is invalid", castleID)
		}
		contents, err := os.ReadFile(filepath.Join(directory, filename))
		if err != nil {
			return TowerQueueState{}, fmt.Errorf("read tower queue castle %d: %w", castleID, err)
		}
		var document persistedTowerQueueCastle
		if err := json.Unmarshal(contents, &document); err != nil {
			return TowerQueueState{}, fmt.Errorf("decode tower queue castle %d: %w", castleID, err)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision || document.CastleID != castleID {
			return TowerQueueState{}, fmt.Errorf("tower queue castle %d metadata is invalid", castleID)
		}
		queue.EntriesByCastle[castleID] = document.Entries
		if !document.LastScannedAt.IsZero() {
			queue.LastScannedAt[castleID] = document.LastScannedAt
		}
		if !document.LastAttemptedAt.IsZero() {
			queue.LastAttemptedAt[castleID] = document.LastAttemptedAt
		}
		if document.ConfirmedLaunches != 0 {
			queue.ConfirmedLaunchesByCastle[castleID] = document.ConfirmedLaunches
		}
		if document.Capacity != nil {
			queue.CapacityByCastle[castleID] = *document.Capacity
		}
	}
	return queue, nil
}

func loadReportComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (ReportState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return ReportState{}, fmt.Errorf("read report index: %w", err)
	}
	var index persistedReportIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return ReportState{}, fmt.Errorf("decode report index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return ReportState{}, fmt.Errorf("report index metadata is invalid")
	}
	expectedIDs := make([]int64, 0, len(manifest.ReportFiles))
	for key := range manifest.ReportFiles {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return ReportState{}, fmt.Errorf("report manifest contains invalid message id %q", key)
		}
		expectedIDs = append(expectedIDs, id)
	}
	sort.Slice(expectedIDs, func(left, right int) bool { return expectedIDs[left] < expectedIDs[right] })
	if !reflect.DeepEqual(index.MessageIDs, expectedIDs) {
		return ReportState{}, fmt.Errorf("report index does not match its manifest")
	}
	reports := ReportState{
		Notices: map[int64]ReportNotice{}, SpyCaptures: map[int64]SpyReportCapture{},
		BattleCaptures: map[int64]BattleReportCapture{}, ActiveBattleReport: index.ActiveBattleReport,
	}
	for _, messageID := range index.MessageIDs {
		filename := manifest.ReportFiles[strconv.FormatInt(messageID, 10)]
		if !safeComponentFilename(filename) {
			return ReportState{}, fmt.Errorf("report message %d filename is invalid", messageID)
		}
		contents, err := os.ReadFile(filepath.Join(directory, filename))
		if err != nil {
			return ReportState{}, fmt.Errorf("read report message %d: %w", messageID, err)
		}
		var document persistedReportMessage
		if err := json.Unmarshal(contents, &document); err != nil {
			return ReportState{}, fmt.Errorf("decode report message %d: %w", messageID, err)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision ||
			document.MessageID != messageID || document.Notice == nil && document.Spy == nil && document.Battle == nil {
			return ReportState{}, fmt.Errorf("report message %d metadata is invalid", messageID)
		}
		if document.Notice != nil {
			reports.Notices[messageID] = *document.Notice
		}
		if document.Spy != nil {
			reports.SpyCaptures[messageID] = *document.Spy
		}
		if document.Battle != nil {
			reports.BattleCaptures[messageID] = *document.Battle
		}
	}
	return reports, nil
}

func loadEventScoreComponent(
	directory string,
	manifest componentManifest,
	indexFilename string,
) (EventScoreState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, indexFilename))
	if err != nil {
		return EventScoreState{}, fmt.Errorf("read event score index: %w", err)
	}
	var index persistedEventScoreIndex
	if err := json.Unmarshal(contents, &index); err != nil {
		return EventScoreState{}, fmt.Errorf("decode event score index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion || index.Revision > manifest.Revision {
		return EventScoreState{}, fmt.Errorf("event score index metadata is invalid")
	}
	expectedIDs := make([]int64, 0, len(manifest.EventScoreFiles))
	for key := range manifest.EventScoreFiles {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			return EventScoreState{}, fmt.Errorf("event score manifest contains invalid event id %q", key)
		}
		expectedIDs = append(expectedIDs, id)
	}
	sort.Slice(expectedIDs, func(left, right int) bool { return expectedIDs[left] < expectedIDs[right] })
	if !reflect.DeepEqual(index.EventIDs, expectedIDs) {
		return EventScoreState{}, fmt.Errorf("event score index does not match its manifest")
	}
	state := EventScoreState{
		ActiveEventID: index.ActiveEventID, ByEvent: map[int64]ScalableEventScore{},
		ShopByPackage: index.ShopByPackage, ActivityByEvent: map[int64]EventActivityState{},
		RankingByEvent: map[int64]EventRankingState{}, Inventory: index.Inventory,
	}
	if state.ShopByPackage == nil {
		state.ShopByPackage = map[PackageID]EventShopRoute{}
	}
	if state.Inventory.ActiveByEvent == nil {
		state.Inventory.ActiveByEvent = map[int64]EventAvailability{}
	}
	for _, eventID := range index.EventIDs {
		filename := manifest.EventScoreFiles[strconv.FormatInt(eventID, 10)]
		if !safeComponentFilename(filename) {
			return EventScoreState{}, fmt.Errorf("event score %d filename is invalid", eventID)
		}
		contents, err := os.ReadFile(filepath.Join(directory, filename))
		if err != nil {
			return EventScoreState{}, fmt.Errorf("read event score %d: %w", eventID, err)
		}
		var document persistedEventScore
		if err := json.Unmarshal(contents, &document); err != nil {
			return EventScoreState{}, fmt.Errorf("decode event score %d: %w", eventID, err)
		}
		if document.SchemaVersion != SchemaVersion || document.Revision > index.Revision ||
			document.EventID != eventID || document.Score == nil && document.Activity == nil && document.Ranking == nil {
			return EventScoreState{}, fmt.Errorf("event score %d metadata is invalid", eventID)
		}
		if document.Score != nil {
			state.ByEvent[eventID] = *document.Score
		}
		if document.Activity != nil {
			state.ActivityByEvent[eventID] = *document.Activity
		}
		if document.Ranking != nil {
			state.RankingByEvent[eventID] = *document.Ranking
		}
	}
	return state, nil
}

func applyInventoryPersistencePart(inventory *InventoryState, part string, patch *InventoryPatch) error {
	switch part {
	case "construction-items":
		if patch.ConstructionItems == nil || patch.ConstructionItemsObservedAt == nil {
			return fmt.Errorf("inventory construction-items state part has no value")
		}
		inventory.ConstructionItems = *patch.ConstructionItems
		inventory.ConstructionItemsObservedAt = *patch.ConstructionItemsObservedAt
	case "construction-offers":
		if patch.ConstructionOffers == nil || patch.ConstructionOffersObservedAt == nil ||
			patch.ConstructionOffersCastleID == nil || patch.ConstructionOffersKingdomID == nil {
			return fmt.Errorf("inventory construction-offers state part has no value")
		}
		inventory.ConstructionOffers = *patch.ConstructionOffers
		inventory.ConstructionOffersObservedAt = *patch.ConstructionOffersObservedAt
		inventory.ConstructionOffersCastleID = *patch.ConstructionOffersCastleID
		inventory.ConstructionOffersKingdomID = *patch.ConstructionOffersKingdomID
		if patch.ConstructionOffersByCastle != nil {
			inventory.ConstructionOffersByCastle = *patch.ConstructionOffersByCastle
		} else if inventory.ConstructionOffersCastleID > 0 {
			if inventory.ConstructionOffersByCastle == nil {
				inventory.ConstructionOffersByCastle = map[CastleID]ConstructionOfferSnapshot{}
			}
			inventory.ConstructionOffersByCastle[inventory.ConstructionOffersCastleID] = ConstructionOfferSnapshot{
				Offers: inventory.ConstructionOffers, ObservedAt: inventory.ConstructionOffersObservedAt,
				CastleID: inventory.ConstructionOffersCastleID, KingdomID: inventory.ConstructionOffersKingdomID,
			}
		}
	case "equipment":
		if patch.Equipment == nil {
			return fmt.Errorf("inventory equipment state part has no value")
		}
		inventory.Equipment = *patch.Equipment
	case "gems":
		if patch.Gems == nil {
			return fmt.Errorf("inventory gems state part has no value")
		}
		inventory.Gems = *patch.Gems
	case "gem-stacks":
		if patch.GemStacks == nil {
			return fmt.Errorf("inventory gem-stacks state part has no value")
		}
		inventory.GemStacks = *patch.GemStacks
	case "items":
		if patch.Items == nil {
			return fmt.Errorf("inventory items state part has no value")
		}
		inventory.Items = *patch.Items
	default:
		return fmt.Errorf("unknown inventory state part %q", part)
	}
	return nil
}

func readComponentManifest(dataDir string) (componentManifest, error) {
	contents, err := os.ReadFile(filepath.Join(componentStatePath(dataDir), componentManifestName))
	if err != nil {
		return componentManifest{}, err
	}
	var manifest componentManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return componentManifest{}, fmt.Errorf("decode component state manifest: %w", err)
	}
	if manifest.SchemaVersion != SchemaVersion {
		return componentManifest{}, fmt.Errorf(
			"component state schema %d is not supported by schema %d", manifest.SchemaVersion, SchemaVersion,
		)
	}
	if len(manifest.Files) == 0 {
		return componentManifest{}, fmt.Errorf("component state manifest is empty")
	}
	return manifest, nil
}

func applyComponentPatch(state *GameState, component Component, patch *ComponentPatch) error {
	require := func(value any) error {
		if value == nil || (reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()) {
			return fmt.Errorf("persisted %s state component has no value", component)
		}
		return nil
	}
	switch component {
	case ComponentCatalog:
		if patch.CatalogVersion == nil || patch.LanguageVersion == nil {
			return fmt.Errorf("persisted catalog state component has no value")
		}
		state.CatalogVersion, state.LanguageVersion = *patch.CatalogVersion, *patch.LanguageVersion
	case ComponentSession:
		if err := require(patch.Session); err != nil {
			return err
		}
		state.Session = *patch.Session
	case ComponentAccount:
		if err := require(patch.Account); err != nil {
			return err
		}
		state.Account = *patch.Account
	case ComponentPlayer:
		if err := require(patch.Player); err != nil {
			return err
		}
		state.Player = *patch.Player
	case ComponentCastles:
		if err := require(patch.Castles); err != nil {
			return err
		}
		state.Castles = *patch.Castles
	case ComponentCommanders:
		if err := require(patch.Commanders); err != nil {
			return err
		}
		state.Commanders = *patch.Commanders
	case ComponentGenerals:
		if err := require(patch.Generals); err != nil {
			return err
		}
		state.Generals = *patch.Generals
	case ComponentCastellans:
		if err := require(patch.Castellans); err != nil {
			return err
		}
		state.Castellans = *patch.Castellans
	case ComponentMovements:
		if err := require(patch.Movements); err != nil {
			return err
		}
		state.Movements = *patch.Movements
	case ComponentMovementSnapshot:
		if err := require(patch.MovementSnapshot); err != nil {
			return err
		}
		state.MovementSnapshot = *patch.MovementSnapshot
	case ComponentStationing:
		if err := require(patch.Stationing); err != nil {
			return err
		}
		state.Stationing = *patch.Stationing
	case ComponentScheduled:
		if err := require(patch.Scheduled); err != nil {
			return err
		}
		state.Scheduled = *patch.Scheduled
	case ComponentRift:
		if err := require(patch.Rift); err != nil {
			return err
		}
		state.Rift = *patch.Rift
	case ComponentInventory:
		if err := require(patch.Inventory); err != nil {
			return err
		}
		state.Inventory = *patch.Inventory
	case ComponentSubscriptions:
		if err := require(patch.Subscriptions); err != nil {
			return err
		}
		state.Subscriptions = *patch.Subscriptions
	case ComponentMarket:
		if err := require(patch.Market); err != nil {
			return err
		}
		state.Market = *patch.Market
	case ComponentKingdomTransport:
		if err := require(patch.KingdomTransport); err != nil {
			return err
		}
		state.KingdomTransport = *patch.KingdomTransport
	case ComponentBeri:
		if err := require(patch.Beri); err != nil {
			return err
		}
		state.Beri = *patch.Beri
	case ComponentAlliance:
		if err := require(patch.Alliance); err != nil {
			return err
		}
		state.Alliance = *patch.Alliance
	case ComponentAlliances:
		if err := require(patch.Alliances); err != nil {
			return err
		}
		state.Alliances = *patch.Alliances
	case ComponentAllianceHelp:
		if err := require(patch.AllianceHelpRequests); err != nil {
			return err
		}
		state.AllianceHelpRequests = *patch.AllianceHelpRequests
	case ComponentWorldMap:
		if err := require(patch.Map); err != nil {
			return err
		}
		state.Map = *patch.Map
	case ComponentTowerCooldowns:
		if err := require(patch.TowerCooldowns); err != nil {
			return err
		}
		state.TowerCooldowns = *patch.TowerCooldowns
	case ComponentTowerQueue:
		if err := require(patch.TowerQueue); err != nil {
			return err
		}
		state.TowerQueue = *patch.TowerQueue
	case ComponentInvasion:
		if err := require(patch.Invasion); err != nil {
			return err
		}
		state.Invasion = *patch.Invasion
	case ComponentStorm:
		if err := require(patch.Storm); err != nil {
			return err
		}
		state.Storm = *patch.Storm
	case ComponentNomadCamps:
		if err := require(patch.NomadCamps); err != nil {
			return err
		}
		state.NomadCamps = *patch.NomadCamps
	case ComponentAdvisor:
		if err := require(patch.Advisor); err != nil {
			return err
		}
		state.Advisor = *patch.Advisor
	case ComponentKhan:
		if err := require(patch.Khan); err != nil {
			return err
		}
		state.Khan = *patch.Khan
	case ComponentDailyAttacks:
		if err := require(patch.DailyAttacks); err != nil {
			return err
		}
		state.DailyAttacks = *patch.DailyAttacks
	case ComponentAttackDialog:
		if err := require(patch.AttackDialog); err != nil {
			return err
		}
		state.AttackDialog = *patch.AttackDialog
	case ComponentAttackPresets:
		if err := require(patch.AttackPresets); err != nil {
			return err
		}
		state.AttackPresets = *patch.AttackPresets
	case ComponentAttackAnalytics:
		if err := require(patch.AttackAnalytics); err != nil {
			return err
		}
		state.AttackAnalytics = *patch.AttackAnalytics
	case ComponentEventScores:
		if err := require(patch.EventScores); err != nil {
			return err
		}
		state.EventScores = *patch.EventScores
	case ComponentCommandContext:
		if err := require(patch.CommandContext); err != nil {
			return err
		}
		state.CommandContext = *patch.CommandContext
	case ComponentAutomations:
		if err := require(patch.Automations); err != nil {
			return err
		}
		state.Automations = *patch.Automations
	case ComponentReports:
		if err := require(patch.Reports); err != nil {
			return err
		}
		state.Reports = *patch.Reports
	case ComponentObservations:
		if err := require(patch.Observations); err != nil {
			return err
		}
		state.Observations = *patch.Observations
	case ComponentCombatCooldown:
		if err := require(patch.CombatCooldown); err != nil {
			return err
		}
		state.CombatCooldown = *patch.CombatCooldown
	default:
		return fmt.Errorf("unknown persisted state component %d", component)
	}
	return nil
}

func writeAtomicStateFile(directory string, filename string, contents []byte) error {
	if !safeComponentFilename(filename) && filename != componentManifestName {
		return fmt.Errorf("invalid state filename %q", filename)
	}
	temporary, err := os.CreateTemp(directory, ".state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, filepath.Join(directory, filename))
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func safeComponentFilename(filename string) bool {
	return filename != "" && filepath.Base(filename) == filename && !strings.Contains(filename, "..")
}

func componentStatePath(dataDir string) string {
	return filepath.Join(dataDir, "State", componentStateDirectory)
}
