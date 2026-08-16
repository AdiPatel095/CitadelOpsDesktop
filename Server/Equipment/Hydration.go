package Equipment

import (
	"reflect"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

// HydrateState migrates legacy socket identities and fills fields that are
// defined by the current official game-data snapshot rather than by the wire.
func HydrateState(gameState *State.GameState, gameData *GameData.Store) bool {
	if gameState == nil {
		return false
	}
	changed := migrateLegacyGemIdentities(gameState)
	var hydrated map[State.GemInstanceID]State.GemInstance
	for id, gem := range gameState.Inventory.Gems {
		if HydrateGem(&gem, gameData) {
			if hydrated == nil {
				hydrated = make(map[State.GemInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
				for existingID, existing := range gameState.Inventory.Gems {
					hydrated[existingID] = existing
				}
			}
			hydrated[id] = gem
			changed = true
		}
	}
	if hydrated != nil {
		gameState.ReplaceInventoryGems(hydrated)
	}
	return rebuildLeaderGemAssignments(gameState) || changed
}

// HydrateGem adds official compatibility, mode, set, and catalog effects to a
// parsed gem while retaining instance-only relic effects from the wire.
func HydrateGem(gem *State.GemInstance, gameData *GameData.Store) bool {
	if gem == nil || gameData == nil {
		return false
	}
	before := *gem
	before.Effects = append(State.EquipmentEffects(nil), gem.Effects...)

	if catalog, err := gameData.Catalog("gems"); err == nil {
		if raw, found := catalog.Find(strconv.FormatInt(int64(gem.DefinitionID), 10)); found {
			if record, decodeErr := GameData.DecodeRecord(raw); decodeErr == nil {
				if wearerID, ok := record.Int64("wearerID"); ok {
					gem.CompatibleWearerID = int(wearerID)
				}
				if setID, ok := record.Int64("setID"); ok {
					gem.SetID = setID
				}
				if encoded, ok := record.String("effects"); ok {
					gem.Effects = parseOfficialEffects(encoded, nil)
				}
				gem.CombatMode = officialGemCombatMode(gameData, record, gem.Effects)
			}
		}
	}

	if gem.TypeID > 0 {
		if catalog, err := gameData.Catalog("relicTypes"); err == nil {
			if raw, found := catalog.Find(strconv.Itoa(gem.TypeID)); found {
				if record, decodeErr := GameData.DecodeRecord(raw); decodeErr == nil {
					if wearerID, ok := record.Int64("wearerID"); ok {
						gem.CompatibleWearerID = int(wearerID)
					}
					if name, ok := record.String("name"); ok {
						gem.CombatMode = modeFromText(name)
					}
				}
			}
		}
	}
	if gem.CombatMode == "" {
		gem.CombatMode = officialEffectCombatMode(gameData, gem.Effects)
	}
	return !reflect.DeepEqual(before, *gem)
}

func migrateLegacyGemIdentities(gameState *State.GameState) bool {
	next := make(map[State.GemInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
	for mapID, source := range gameState.Inventory.Gems {
		gem := source
		parentID := gem.EquipmentInstanceID
		if parentID == 0 {
			candidate := State.EquipmentInstanceID(mapID)
			if _, found := gameState.Inventory.Equipment[candidate]; found && gem.WearerKind != "" {
				parentID = candidate
			}
		}
		newID := mapID
		if parent, found := gameState.Inventory.Equipment[parentID]; found {
			gem.EquipmentInstanceID = parent.ID
			gem.Slot = parent.Slot
			gem.WearerKind = parent.WearerKind
			gem.WearerID = parent.WearerID
			if mapID == State.GemInstanceID(parent.ID) {
				if (parent.RarityID == 5 || parent.RarityID == 15) && gem.DefinitionID > 0 {
					newID = State.GemInstanceID(gem.DefinitionID)
				} else {
					newID = -State.GemInstanceID(parent.ID)
				}
			}
		}
		gem.ID = newID
		next[newID] = gem
	}
	if reflect.DeepEqual(gameState.Inventory.Gems, next) {
		return false
	}
	gameState.ReplaceInventoryGems(next)
	return true
}

func rebuildLeaderGemAssignments(gameState *State.GameState) bool {
	changed := false
	commanderAssignments := make(map[State.CommanderID]map[string]State.GemInstanceID, len(gameState.Commanders))
	for id, leader := range gameState.Commanders {
		commanderAssignments[id] = leader.Gems
		leader.Gems = map[string]State.GemInstanceID{}
		if leader.Equipment == nil {
			leader.Equipment = map[string]State.EquipmentInstanceID{}
			changed = true
		}
		gameState.Commanders[id] = leader
	}
	castellanAssignments := make(map[State.CastellanID]map[string]State.GemInstanceID, len(gameState.Castellans))
	for id, leader := range gameState.Castellans {
		castellanAssignments[id] = leader.Gems
		leader.Gems = map[string]State.GemInstanceID{}
		if leader.Equipment == nil {
			leader.Equipment = map[string]State.EquipmentInstanceID{}
			changed = true
		}
		gameState.Castellans[id] = leader
	}
	for id, gem := range gameState.Inventory.Gems {
		parent, found := gameState.Inventory.Equipment[gem.EquipmentInstanceID]
		if !found || parent.WearerKind == "" || parent.Slot < 1 || parent.Slot > 4 {
			continue
		}
		slot := strconv.Itoa(parent.Slot)
		switch parent.WearerKind {
		case "commander":
			leaderID := State.CommanderID(parent.WearerID)
			leader, found := gameState.Commanders[leaderID]
			if found {
				leader.Gems[slot] = id
				gameState.Commanders[leaderID] = leader
			}
		case "castellan":
			leaderID := State.CastellanID(parent.WearerID)
			leader, found := gameState.Castellans[leaderID]
			if found {
				leader.Gems[slot] = id
				gameState.Castellans[leaderID] = leader
			}
		}
	}
	for id, before := range commanderAssignments {
		if !reflect.DeepEqual(before, gameState.Commanders[id].Gems) {
			changed = true
		}
	}
	for id, before := range castellanAssignments {
		if !reflect.DeepEqual(before, gameState.Castellans[id].Gems) {
			changed = true
		}
	}
	return changed
}

func officialGemCombatMode(gameData *GameData.Store, record GameData.Record, effects State.EquipmentEffects) string {
	mode := officialEffectCombatMode(gameData, effects)
	if mode != "any" {
		return mode
	}
	for _, field := range []string{"comment1", "comment2"} {
		if value, ok := record.String(field); ok {
			if textMode := modeFromText(value); textMode != "any" {
				return textMode
			}
		}
	}
	return mode
}

func officialEffectCombatMode(gameData *GameData.Store, effects State.EquipmentEffects) string {
	if gameData == nil {
		return "any"
	}
	catalog, err := gameData.Catalog("effects")
	if err != nil {
		return "any"
	}
	pvp, pve := false, false
	for _, effect := range effects {
		raw, found := catalog.Find(strconv.FormatInt(effect.DefinitionID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		if value, ok := record.Int64("isPvPFight"); ok && value == 1 {
			pvp = true
		}
		if value, ok := record.Int64("isPvEFight"); ok && value == 1 {
			pve = true
		}
		if name, ok := record.String("name"); ok {
			switch modeFromText(name) {
			case "pvp":
				pvp = true
			case "pve":
				pve = true
			}
		}
	}
	if pvp != pve {
		if pvp {
			return "pvp"
		}
		return "pve"
	}
	return "any"
}

func modeFromText(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "pvp"):
		return "pvp"
	case strings.Contains(lower, "pve"), strings.Contains(lower, "npc"):
		return "pve"
	default:
		return "any"
	}
}

func parseOfficialEffects(encoded string, resolve func(int64) int64) State.EquipmentEffects {
	effects := State.EquipmentEffects{}
	for _, entry := range strings.Split(encoded, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "&", 2)
		if len(parts) != 2 {
			continue
		}
		wireID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || wireID <= 0 {
			continue
		}
		values := []float64{}
		for _, rawValue := range strings.FieldsFunc(parts[1], func(value rune) bool { return value == '+' || value == '#' }) {
			if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(rawValue), 64); parseErr == nil {
				values = append(values, parsed)
			}
		}
		if len(values) == 0 {
			continue
		}
		definitionID := wireID
		if resolve != nil {
			definitionID = resolve(wireID)
		}
		effects = append(effects, State.EquipmentEffect{WireID: wireID, DefinitionID: definitionID, Values: values})
	}
	return effects
}
