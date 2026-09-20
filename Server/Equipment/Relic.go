package Equipment

import "CitadelDesktop/Server/State"

// IsRelic2Equipment mirrors the game's two Relic 2.0 wire shapes: standard
// equipment carries four relic effects, while the hero carries six.
func IsRelic2Equipment(item State.EquipmentInstance) bool {
	standard := item.RarityID == 5 && len(item.Effects) == 4 && item.Slot != 6
	hero := item.RarityID == 15 && len(item.Effects) == 6 && item.Slot == 6
	return standard || hero
}

// CommanderRelic2EffectTotal sums one normalized effect only across equipped
// Relic 2.0 items. It deliberately excludes legacy equipment, gems, set
// bonuses, and general skills so a mixed commander cannot impersonate a
// maxed Relic 2.0 roll by reaching the same aggregate through other sources.
func CommanderRelic2EffectTotal(
	gameState State.GameState,
	commanderID State.CommanderID,
	effectID int64,
) (float64, bool) {
	commander, found := gameState.Commanders[commanderID]
	if !found || effectID <= 0 {
		return 0, false
	}
	total := 0.0
	matched := false
	for _, instanceID := range commander.Equipment {
		item, exists := gameState.Inventory.Equipment[instanceID]
		if !exists || !IsRelic2Equipment(item) {
			continue
		}
		for _, effect := range item.Effects {
			definitionID := effect.DefinitionID
			if definitionID <= 0 {
				definitionID = effect.WireID
			}
			if definitionID != effectID || len(effect.Values) == 0 {
				continue
			}
			total += effect.Values[len(effect.Values)-1]
			matched = true
		}
	}
	return total, matched
}
