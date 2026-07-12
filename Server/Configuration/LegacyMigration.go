package Configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type legacySectionMigration struct {
	fileName  string
	section   string
	transform func([]byte) (json.RawMessage, error)
}

var legacySectionMigrations = []legacySectionMigration{
	{fileName: "SchedulerSettings.json", section: "scheduler", transform: migrateLegacyScheduler},
	{fileName: "RecruitTroops.json", section: "automation.recruitTroops", transform: migrateLegacyProduction},
	{fileName: "AutoTool.json", section: "automation.autoTool", transform: migrateLegacyProduction},
	{fileName: "AutoHospital.json", section: "automation.autoHospital", transform: migrateLegacyDocument},
	{fileName: "AutoSceatRes.json", section: "automation.autoSceatResources", transform: migrateLegacyDocument},
	{fileName: "AutoTCI.json", section: "automation.constructionItems", transform: migrateLegacyConstruction},
	{fileName: "AutoBird.json", section: "automation.autoBird", transform: migrateLegacyDocument},
	{fileName: "AutoStation.json", section: "automation.autoStation", transform: migrateLegacyDocument},
	{fileName: "AutoBeriWorld.json", section: "automation.autoBeriWorld", transform: migrateLegacyBeri},
	{fileName: "RiftMaidenComms.json", section: "rift.maidenComms", transform: migrateLegacyDocument},
}

func migrateLegacySections(dataDir string, sections map[string]json.RawMessage) (bool, error) {
	changed := false
	for _, migration := range legacySectionMigrations {
		if _, exists := sections[migration.section]; exists {
			continue
		}
		path := filepath.Join(dataDir, migration.fileName)
		contents, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return false, fmt.Errorf("read legacy configuration %s: %w", migration.fileName, err)
		}
		value, err := migration.transform(contents)
		if err != nil {
			return false, fmt.Errorf("migrate legacy configuration %s: %w", migration.fileName, err)
		}
		canonical, err := canonicalSection(migration.section, value)
		if err != nil {
			return false, fmt.Errorf("migrate legacy configuration %s: %w", migration.fileName, err)
		}
		sections[migration.section] = canonical
		changed = true
	}
	return changed, nil
}

func migrateLegacyDocument(contents []byte) (json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return json.Marshal(document)
}

func migrateLegacyScheduler(contents []byte) (json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	delete(document, "version")
	return json.Marshal(document)
}

type legacyProductionDocument struct {
	Mode             string                      `json:"mode"`
	CheckIntervalSec int                         `json:"checkIntervalSec"`
	GlobalTargets    map[string]int64            `json:"globalTargets"`
	EnabledCastles   map[string]bool             `json:"enabledCastles"`
	Targets          map[string]map[string]int64 `json:"targets"`
	Castles          map[string]productionCastle `json:"castles"`
	GlobalItems      []productionMigrationTarget `json:"globalItems"`
}

type productionMigrationTarget struct {
	ID     int64 `json:"id"`
	Amount int64 `json:"amount,omitempty"`
}

type productionCastle struct {
	Enabled bool                        `json:"enabled"`
	Items   []productionMigrationTarget `json:"items"`
}

func migrateLegacyProduction(contents []byte) (json.RawMessage, error) {
	var legacy legacyProductionDocument
	if err := json.Unmarshal(contents, &legacy); err != nil {
		return nil, err
	}
	mode := legacy.Mode
	if mode == "" && len(legacy.Targets) > 0 && len(legacy.GlobalTargets) == 0 {
		mode = "perCastle"
	}
	if mode != "perCastle" {
		mode = "global"
	}
	globalItems := normalizeProductionItems(legacy.GlobalItems)
	if len(globalItems) == 0 {
		globalItems = productionTargetsFromMap(legacy.GlobalTargets)
	}
	castles := make(map[string]productionCastle, len(legacy.Targets)+len(legacy.EnabledCastles)+len(legacy.Castles))
	for castleID, targetMap := range legacy.Targets {
		if !validPositiveNumericKey(castleID) {
			continue
		}
		items := productionTargetsFromMap(targetMap)
		enabled := len(items) > 0
		if legacy.EnabledCastles != nil {
			enabled = legacy.EnabledCastles[castleID]
		}
		castles[castleID] = productionCastle{Enabled: enabled, Items: items}
	}
	for castleID, enabled := range legacy.EnabledCastles {
		if !validPositiveNumericKey(castleID) {
			continue
		}
		castle := castles[castleID]
		castle.Enabled = enabled
		if castle.Items == nil {
			castle.Items = []productionMigrationTarget{}
		}
		castles[castleID] = castle
	}
	for castleID, castle := range legacy.Castles {
		if !validPositiveNumericKey(castleID) {
			continue
		}
		castle.Items = normalizeProductionItems(castle.Items)
		castles[castleID] = castle
	}
	return json.Marshal(map[string]any{
		"version": 1, "mode": mode, "checkIntervalSec": legacy.CheckIntervalSec,
		"globalItems": globalItems, "castles": castles,
	})
}

func productionTargetsFromMap(targets map[string]int64) []productionMigrationTarget {
	items := make([]productionMigrationTarget, 0, len(targets))
	for rawID, amount := range targets {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 || amount < 0 {
			continue
		}
		items = append(items, productionMigrationTarget{ID: id, Amount: amount})
	}
	return normalizeProductionItems(items)
}

func normalizeProductionItems(items []productionMigrationTarget) []productionMigrationTarget {
	byID := map[int64]int64{}
	for _, item := range items {
		if item.ID > 0 && item.Amount >= 0 {
			byID[item.ID] = item.Amount
		}
	}
	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	result := make([]productionMigrationTarget, 0, len(ids))
	for _, id := range ids {
		result = append(result, productionMigrationTarget{ID: id, Amount: byID[id]})
	}
	return result
}

type constructionMigrationTarget struct {
	ID       int64 `json:"id"`
	Amount   int   `json:"amount"`
	MinLevel int   `json:"minLevel,omitempty"`
}

func migrateLegacyConstruction(contents []byte) (json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var legacyTargets map[string]json.RawMessage
	if raw := document["targets"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &legacyTargets); err != nil {
			return nil, fmt.Errorf("decode targets: %w", err)
		}
	}
	targets := make(map[string][]constructionMigrationTarget, len(legacyTargets))
	for castleID, raw := range legacyTargets {
		if !validPositiveNumericKey(castleID) {
			continue
		}
		items, err := migrateConstructionTargets(raw)
		if err != nil {
			return nil, fmt.Errorf("decode targets for castle %s: %w", castleID, err)
		}
		targets[castleID] = items
	}
	result := map[string]any{"version": 1, "targets": targets}
	if presets := document["presets"]; len(presets) > 0 && !bytes.Equal(bytes.TrimSpace(presets), []byte("null")) {
		result["presets"] = presets
	} else {
		result["presets"] = map[string]any{"version": 1, "lastSelectedPresetId": nil, "presets": []any{}}
	}
	return json.Marshal(result)
}

func migrateConstructionTargets(raw json.RawMessage) ([]constructionMigrationTarget, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []constructionMigrationTarget{}, nil
	}
	if trimmed[0] == '[' {
		var items []constructionMigrationTarget
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		return normalizeConstructionTargets(items), nil
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &legacy); err != nil {
		return nil, err
	}
	items := make([]constructionMigrationTarget, 0, len(legacy))
	for rawID, rawTarget := range legacy {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		var ceiling int
		if json.Unmarshal(rawTarget, &ceiling) != nil {
			var levels struct {
				Amount   int `json:"amount"`
				MaxLevel int `json:"maxLevel"`
				MinLevel int `json:"minLevel"`
			}
			if err := json.Unmarshal(rawTarget, &levels); err != nil {
				continue
			}
			ceiling = levels.Amount
			if ceiling == 0 {
				ceiling = levels.MaxLevel
			}
			items = append(items, constructionMigrationTarget{ID: id, Amount: ceiling, MinLevel: levels.MinLevel})
			continue
		}
		items = append(items, constructionMigrationTarget{ID: id, Amount: ceiling, MinLevel: 1})
	}
	return normalizeConstructionTargets(items), nil
}

func normalizeConstructionTargets(items []constructionMigrationTarget) []constructionMigrationTarget {
	byID := map[int64]constructionMigrationTarget{}
	for _, item := range items {
		if item.ID <= 0 {
			continue
		}
		item.Amount = clampLevel(item.Amount)
		item.MinLevel = clampLevel(item.MinLevel)
		if item.MinLevel > item.Amount {
			item.MinLevel = item.Amount
		}
		if item.MinLevel == 1 {
			item.MinLevel = 0
		}
		byID[item.ID] = item
	}
	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	result := make([]constructionMigrationTarget, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result
}

func clampLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 4 {
		return 4
	}
	return level
}

type legacyBeriDocument struct {
	MinTroopsToTransfer        int64  `json:"minTroopsToTransfer"`
	BeriCastleID               *int64 `json:"beriCastleId"`
	BeriCastleCID              *int64 `json:"beriCastleCID"`
	TransferTroopID            *int64 `json:"transferTroopId"`
	TransferTroopWID           *int64 `json:"transferTroopWID"`
	SourceCastleID             *int64 `json:"sourceCastleId"`
	KutSourceCastleSCID        *int64 `json:"kutSourceCastleSCID"`
	WireCastleID               *int64 `json:"wireCastleId"`
	KutCastleCID               *int64 `json:"kutCastleCID"`
	TroopSpaceCheckIntervalSec int    `json:"troopSpaceCheckIntervalSec"`
}

func migrateLegacyBeri(contents []byte) (json.RawMessage, error) {
	var legacy legacyBeriDocument
	if err := json.Unmarshal(contents, &legacy); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"minTroopsToTransfer":        legacy.MinTroopsToTransfer,
		"beriCastleId":               firstInt64(legacy.BeriCastleID, legacy.BeriCastleCID, 0),
		"transferTroopId":            firstInt64(legacy.TransferTroopID, legacy.TransferTroopWID, 0),
		"sourceCastleId":             firstInt64(legacy.SourceCastleID, legacy.KutSourceCastleSCID, 0),
		"wireCastleId":               firstInt64(legacy.WireCastleID, legacy.KutCastleCID, -1),
		"troopSpaceCheckIntervalSec": legacy.TroopSpaceCheckIntervalSec,
	})
}

func firstInt64(primary *int64, fallback *int64, defaultValue int64) int64 {
	if primary != nil {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return defaultValue
}

func validPositiveNumericKey(value string) bool {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed > 0
}
