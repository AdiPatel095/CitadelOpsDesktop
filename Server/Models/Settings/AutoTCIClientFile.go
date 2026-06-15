package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"CitadelDesktop/Server/Paths"
)

// Auto TCI UI state (active targets + named presets) lives next to AutoBird.json under paths.DataDir().
const autoTCIClientFileName = "AutoTCI.json"

var (
	autoTCIClientMu    sync.Mutex
	defaultAutoTCIJSON = []byte(`{"version":1,"targets":{},"presets":{"version":1,"lastSelectedPresetId":null,"presets":[]}}`)
	defaultPresetsObj  = map[string]interface{}{
		"version":              1,
		"lastSelectedPresetId": nil,
		"presets":              []interface{}{},
	}
)

func autoTCIClientPath() string {
	return filepath.Join(Paths.DataDir(), autoTCIClientFileName)
}

func normalizeLevelTarget(t AutoTCILevelTarget) AutoTCILevelTarget {
	return t.Normalize()
}

// TargetsToWire converts in-memory targets to legacy JSON-safe string-key maps (max level only).
func TargetsToWire(targets map[int]map[int]AutoTCILevelTarget) map[string]map[string]int {
	if len(targets) == 0 {
		return map[string]map[string]int{}
	}
	out := make(map[string]map[string]int, len(targets))
	for castleID, perCastle := range targets {
		if len(perCastle) == 0 {
			out[strconv.Itoa(castleID)] = map[string]int{}
			continue
		}
		row := make(map[string]int, len(perCastle))
		for tciID, tgt := range perCastle {
			row[strconv.Itoa(tciID)] = normalizeLevelTarget(tgt).MaxLevel
		}
		out[strconv.Itoa(castleID)] = row
	}
	return out
}

// TargetsToWireItems converts targets to the array form stored in AutoTCI.json.
func TargetsToWireItems(targets map[int]map[int]AutoTCILevelTarget) map[string]interface{} {
	if len(targets) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(targets))
	for castleID, perCastle := range targets {
		if len(perCastle) == 0 {
			out[strconv.Itoa(castleID)] = []interface{}{}
			continue
		}
		items := make([]interface{}, 0, len(perCastle))
		for tciID, tgt := range perCastle {
			n := normalizeLevelTarget(tgt)
			item := map[string]interface{}{
				"id":     tciID,
				"amount": n.MaxLevel,
			}
			if n.MinLevel > 1 {
				item["minLevel"] = n.MinLevel
			}
			items = append(items, item)
		}
		out[strconv.Itoa(castleID)] = items
	}
	return out
}

func targetsFromWire(wire map[string]map[string]int) map[int]map[int]AutoTCILevelTarget {
	if len(wire) == 0 {
		return map[int]map[int]AutoTCILevelTarget{}
	}
	out := make(map[int]map[int]AutoTCILevelTarget, len(wire))
	for castleStr, perCastle := range wire {
		castleID, err := strconv.Atoi(castleStr)
		if err != nil || castleID == 0 {
			continue
		}
		if len(perCastle) == 0 {
			out[castleID] = map[int]AutoTCILevelTarget{}
			continue
		}
		row := make(map[int]AutoTCILevelTarget, len(perCastle))
		for tciStr, ceiling := range perCastle {
			tciID, err := strconv.Atoi(tciStr)
			if err != nil || tciID <= 0 {
				continue
			}
			row[tciID] = AutoTCILevelTargetFromMax(ceiling)
		}
		out[castleID] = row
	}
	return out
}

func levelTargetFromClientItem(item map[string]interface{}) (AutoTCILevelTarget, bool) {
	tciID := 0
	if v, ok := item["id"].(float64); ok {
		tciID = int(v)
	}
	if tciID <= 0 {
		return AutoTCILevelTarget{}, false
	}
	maxLevel := 1
	if v, ok := item["amount"].(float64); ok {
		maxLevel = int(v)
	}
	minLevel := 1
	if v, ok := item["minLevel"].(float64); ok {
		minLevel = int(v)
	}
	return normalizeLevelTarget(AutoTCILevelTarget{MinLevel: minLevel, MaxLevel: maxLevel}), true
}

// AutoTCITargetsFromClientMap parses UI targets (array or legacy wire map per castle).
func AutoTCITargetsFromClientMap(raw map[string]interface{}) map[int]map[int]AutoTCILevelTarget {
	if len(raw) == 0 {
		return map[int]map[int]AutoTCILevelTarget{}
	}
	out := make(map[int]map[int]AutoTCILevelTarget, len(raw))
	for castleIDStr, itemsRaw := range raw {
		castleID, err := strconv.Atoi(castleIDStr)
		if err != nil || castleID == 0 {
			continue
		}
		switch items := itemsRaw.(type) {
		case []interface{}:
			row := make(map[int]AutoTCILevelTarget)
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				tgt, ok := levelTargetFromClientItem(item)
				if !ok {
					continue
				}
				tciID := int(item["id"].(float64))
				row[tciID] = tgt
			}
			out[castleID] = row
		case map[string]interface{}:
			row := make(map[int]AutoTCILevelTarget)
			for tciStr, ceilingRaw := range items {
				tciID, err := strconv.Atoi(tciStr)
				if err != nil || tciID <= 0 {
					continue
				}
				if ceiling, ok := ceilingRaw.(float64); ok {
					row[tciID] = AutoTCILevelTargetFromMax(int(ceiling))
				}
			}
			out[castleID] = row
		}
	}
	return out
}

func targetsFromJSONValue(v interface{}) map[int]map[int]AutoTCILevelTarget {
	if v == nil {
		return map[int]map[int]AutoTCILevelTarget{}
	}
	if wire, ok := v.(map[string]interface{}); ok {
		return AutoTCITargetsFromClientMap(wire)
	}
	return map[int]map[int]AutoTCILevelTarget{}
}

func readRawAutoTCIClientFileUnlocked() []byte {
	path := autoTCIClientPath()
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		return data
	}
	if d := Paths.LegacyDotCitadelOpsDir(); d != "" {
		if leg, err := os.ReadFile(filepath.Join(d, autoTCIClientFileName)); err == nil && len(leg) > 0 {
			return leg
		}
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if leg, err := os.ReadFile(filepath.Join(filepath.Dir(exe), autoTCIClientFileName)); err == nil && len(leg) > 0 {
			return leg
		}
	}
	out := make([]byte, len(defaultAutoTCIJSON))
	copy(out, defaultAutoTCIJSON)
	return out
}

func ensureAutoTCIClientDoc(doc map[string]interface{}) {
	if doc == nil {
		return
	}
	if doc["version"] == nil {
		doc["version"] = 1
	}
	if doc["targets"] == nil {
		doc["targets"] = map[string]interface{}{}
	}
	if doc["presets"] == nil {
		doc["presets"] = defaultPresetsObj
	}
}

func migrateAutoTCIClientData(data []byte) []byte {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return append([]byte(nil), defaultAutoTCIJSON...)
	}
	ensureAutoTCIClientDoc(doc)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return append([]byte(nil), defaultAutoTCIJSON...)
	}
	return append(out, '\n')
}

func readAutoTCIClientDocUnlocked() map[string]interface{} {
	data := migrateAutoTCIClientData(readRawAutoTCIClientFileUnlocked())
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		doc = map[string]interface{}{}
		ensureAutoTCIClientDoc(doc)
	}
	ensureAutoTCIClientDoc(doc)
	return doc
}

// ReadAutoTCIClientFile returns JSON for the unified Auto TCI file (defaults or migrated legacy).
func ReadAutoTCIClientFile() []byte {
	autoTCIClientMu.Lock()
	defer autoTCIClientMu.Unlock()
	return migrateAutoTCIClientData(readRawAutoTCIClientFileUnlocked())
}

// DefaultAutoTCIClientJSON returns a copy of the built-in default document.
func DefaultAutoTCIClientJSON() []byte {
	out := make([]byte, len(defaultAutoTCIJSON))
	copy(out, defaultAutoTCIJSON)
	return out
}

// ReadAutoTCITargets loads persisted targets from AutoTCI.json (empty map if missing).
func ReadAutoTCITargets() map[int]map[int]AutoTCILevelTarget {
	autoTCIClientMu.Lock()
	defer autoTCIClientMu.Unlock()
	doc := readAutoTCIClientDocUnlocked()
	return targetsFromJSONValue(doc["targets"])
}

func writeAutoTCIClientDocUnlocked(doc map[string]interface{}) error {
	ensureAutoTCIClientDoc(doc)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := autoTCIClientPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteAutoTCIClientFile writes the full AutoTCI.json document (atomic replace).
func WriteAutoTCIClientFile(data []byte) error {
	if len(data) == 0 {
		data = append([]byte(nil), defaultAutoTCIJSON...)
	}
	data = migrateAutoTCIClientData(data)
	var check interface{}
	if err := json.Unmarshal(data, &check); err != nil {
		return err
	}
	autoTCIClientMu.Lock()
	defer autoTCIClientMu.Unlock()
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	path := autoTCIClientPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteAutoTCITargetsOnly updates active targets in AutoTCI.json without touching presets.
func WriteAutoTCITargetsOnly(targets map[int]map[int]AutoTCILevelTarget) error {
	if targets == nil {
		targets = map[int]map[int]AutoTCILevelTarget{}
	}
	autoTCIClientMu.Lock()
	defer autoTCIClientMu.Unlock()
	doc := readAutoTCIClientDocUnlocked()
	doc["targets"] = TargetsToWireItems(targets)
	return writeAutoTCIClientDocUnlocked(doc)
}
