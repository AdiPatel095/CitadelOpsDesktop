package AttackCapacity

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const travelSpeedEffectTypeID = 15

// TravelSpeedResult is the commander-specific part of an attack's travel
// speed. Castle, account, and target-wide effects are deliberately omitted
// because they are identical for every commander in the same attack chain.
type TravelSpeedResult struct {
	CommanderID    State.CommanderID `json:"commanderId"`
	AppliedPercent float64           `json:"appliedPercent"`
}

// ResolveTravelSpeed ranks commanders for a shared target so faster marches
// can be launched first and cannot overtake slower marches in the same chain.
func (Resolver) ResolveTravelSpeed(
	gameState State.GameState,
	gameData *GameData.Store,
	request Request,
) (TravelSpeedResult, error) {
	if gameData == nil {
		return TravelSpeedResult{}, fmt.Errorf("official game data is unavailable")
	}
	commander, exists := gameState.Commanders[request.CommanderID]
	if !exists {
		return TravelSpeedResult{}, fmt.Errorf("commander %d is not in the current player state", request.CommanderID)
	}
	target := targetWithCastleType(request.Target)
	effectsCatalog, err := gameData.Catalog("effects")
	if err != nil {
		return TravelSpeedResult{}, fmt.Errorf("official effect catalog is unavailable: %w", err)
	}

	effects := State.EquipmentEffects{}
	setCounts := map[int64]int{}
	for _, instanceID := range sortedEquipmentIDs(commander.Equipment) {
		item, found := gameState.Inventory.Equipment[instanceID]
		if !found {
			continue
		}
		if item.SetID > 0 {
			setCounts[item.SetID]++
		}
		itemEffects := item.Effects
		if len(itemEffects) == 0 {
			itemEffects = optionalCatalogEffects(gameData, "equipments", int64(item.DefinitionID), nil)
		}
		effects = append(effects, itemEffects...)
	}
	for _, instanceID := range sortedGemIDs(commander.Gems) {
		gem, found := gameState.Inventory.Gems[instanceID]
		if !found {
			continue
		}
		if gem.SetID > 0 {
			setCounts[gem.SetID]++
		}
		gemEffects := gem.Effects
		if len(gemEffects) == 0 {
			gemEffects = optionalCatalogEffects(gameData, "gems", int64(gem.DefinitionID), nil)
		}
		effects = append(effects, gemEffects...)
	}
	effects = append(effects, activeTravelSetEffects(gameData, setCounts)...)
	effects = append(effects, activeGeneralTravelEffects(gameState, gameData, commander)...)

	rawByCap := map[int64]float64{}
	for _, effect := range effects {
		effectID := effect.DefinitionID
		if effectID <= 0 {
			effectID = effect.WireID
		}
		raw, found := effectsCatalog.Find(strconv.FormatInt(effectID, 10))
		if !found {
			continue
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		effectTypeID, found := record.Int64("effectTypeID")
		if !found || effectTypeID != travelSpeedEffectTypeID || targetMismatch(record, effectName(record), target) != "" ||
			travelSpaceMismatch(record, target) {
			continue
		}
		magnitude := effectMagnitude(effect.Values)
		if magnitude == 0 || !isFinite(magnitude) {
			continue
		}
		capID, _ := record.Int64("capID")
		rawByCap[capID] += magnitude
	}

	limits := loadCapLimits(gameData)
	result := TravelSpeedResult{CommanderID: request.CommanderID}
	for capID, raw := range rawByCap {
		applied := raw
		if maximum := limits[capID]; maximum > 0 && applied > maximum {
			applied = maximum
		}
		result.AppliedPercent += applied
	}
	return result, nil
}

func activeTravelSetEffects(gameData *GameData.Store, setCounts map[int64]int) State.EquipmentEffects {
	if len(setCounts) == 0 {
		return nil
	}
	catalog, err := gameData.Catalog("equipment_sets")
	if err != nil {
		return nil
	}
	result := State.EquipmentEffects{}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		setID, setOK := record.Int64("setID")
		needed, neededOK := record.Int64("neededItems")
		encoded, effectsOK := record.String("effects")
		if !setOK || !neededOK || !effectsOK || setCounts[setID] < int(needed) {
			continue
		}
		result = append(result, parseOfficialEffects(encoded, func(wireID int64) int64 {
			return normalEquipmentEffectID(gameData, wireID)
		})...)
	}
	return result
}

func activeGeneralTravelEffects(
	gameState State.GameState,
	gameData *GameData.Store,
	commander State.CommanderState,
) State.EquipmentEffects {
	if commander.GeneralID <= 0 {
		return nil
	}
	general, found := gameState.Generals[commander.GeneralID]
	if !found {
		return nil
	}
	catalog, err := gameData.Catalog("generalSkills")
	if err != nil {
		return nil
	}
	skillIDs := append([]int64(nil), general.ActiveSkillIDs...)
	sort.Slice(skillIDs, func(left, right int) bool { return skillIDs[left] < skillIDs[right] })
	result := State.EquipmentEffects{}
	for _, skillID := range skillIDs {
		raw, found := catalog.Find(strconv.FormatInt(skillID, 10))
		if !found {
			continue
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		encoded, found := record.String("effects")
		if found {
			result = append(result, parseOfficialEffects(encoded, nil)...)
		}
	}
	return result
}

func travelSpaceMismatch(record GameData.Record, target TargetContext) bool {
	spaceIDs, restricted := record.String("spaceIDs")
	if !restricted || strings.TrimSpace(spaceIDs) == "" {
		return false
	}
	if target.Map == nil {
		return true
	}
	wanted := int(target.Map.KingdomID)
	for _, value := range strings.FieldsFunc(spaceIDs, func(character rune) bool {
		return character == ',' || character == '#'
	}) {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && parsed == wanted {
			return false
		}
	}
	return true
}
