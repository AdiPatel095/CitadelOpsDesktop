package Automation

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const commanderFeatureSection = "automation.commanderFeatures"

type commanderFeatureConfiguration struct {
	Assignments map[string][]State.CommanderID `json:"assignments"`
}

func decodeSection(snapshot Configuration.Snapshot, section string, destination any) bool {
	raw := snapshot.Sections[section]
	return len(raw) > 0 && json.Unmarshal(raw, destination) == nil
}

func commanderFeatureCandidates(
	gameState State.GameState,
	configuration Configuration.Snapshot,
	featureID string,
) ([]State.CommanderID, bool) {
	settings := commanderFeatureConfiguration{}
	if !decodeSection(configuration, commanderFeatureSection, &settings) {
		return nil, false
	}
	configured, restricted := settings.Assignments[featureID]
	if !restricted {
		return nil, false
	}
	seen := map[State.CommanderID]struct{}{}
	candidates := make([]State.CommanderID, 0, len(configured))
	for _, commanderID := range configured {
		if commanderID < 0 {
			continue
		}
		if _, exists := gameState.Commanders[commanderID]; !exists {
			continue
		}
		if _, duplicate := seen[commanderID]; duplicate {
			continue
		}
		seen[commanderID] = struct{}{}
		candidates = append(candidates, commanderID)
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	return candidates, true
}

func hasAvailableFeatureCommander(
	gameState State.GameState,
	candidates []State.CommanderID,
	restricted bool,
) bool {
	_, found := nextAvailableFeatureCommander(gameState, candidates, restricted)
	return found
}

func nextAvailableFeatureCommander(
	gameState State.GameState,
	candidates []State.CommanderID,
	restricted bool,
) (State.CommanderID, bool) {
	if !restricted {
		candidates = make([]State.CommanderID, 0, len(gameState.Commanders))
		for commanderID := range gameState.Commanders {
			if commanderID >= 0 {
				candidates = append(candidates, commanderID)
			}
		}
		sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	}
	for _, commanderID := range candidates {
		if commander, exists := gameState.Commanders[commanderID]; exists && commander.Available {
			return commanderID, true
		}
	}
	return 0, false
}

func validHorseTravelBoostID(value int) bool {
	switch value {
	case 0, -1, 1007, 1008, 1009:
		return true
	default:
		return false
	}
}

func policyInterval(seconds int, fallback int) time.Duration {
	if seconds < 30 || seconds > 86_400 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

func sortedNumericKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		leftID, _ := strconv.ParseInt(keys[left], 10, 64)
		rightID, _ := strconv.ParseInt(keys[right], 10, 64)
		return leftID < rightID
	})
	return keys
}

func recordNumber(store *GameData.Store, collection string, id int64, field string) (float64, bool) {
	if store == nil || id <= 0 {
		return 0, false
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return 0, false
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	return record.Float64(field)
}

func recordInteger(store *GameData.Store, collection string, id int64, field string) int64 {
	value, ok := recordNumber(store, collection, id, field)
	if !ok {
		return 0
	}
	return int64(value)
}

func commaSeparatedIDs(value string) map[int64]struct{} {
	result := map[int64]struct{}{}
	for _, part := range strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '#'
	}) {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}

func eligibleAllianceHelpProductionID(queue State.ProductionQueue) int64 {
	if queue.LineID != 0 && queue.LineID != 2 {
		return 0
	}
	eligible := func(item State.QueueItem) bool {
		return item.ProductionID > 0 && !item.AllianceHelpRequested && (queue.LineID != 0 || item.Amount >= 5)
	}
	if queue.Active != nil && eligible(*queue.Active) {
		return queue.Active.ProductionID
	}
	for _, item := range queue.Queued {
		if eligible(item) {
			return item.ProductionID
		}
	}
	return 0
}
