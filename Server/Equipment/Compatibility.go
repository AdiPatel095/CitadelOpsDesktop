package Equipment

import (
	"fmt"

	"CitadelDesktop/Server/State"
)

// LoadoutFamily is the official client's equipment-class boundary. The client
// permits ordinary equipment and gems together or relic equipment and gems
// together, but rejects movement across those families.
type LoadoutFamily uint8

const (
	LoadoutFamilyUnknown LoadoutFamily = iota
	LoadoutFamilyOrdinary
	LoadoutFamilyRelic
)

// EquipmentFamily uses the exact wire discriminator retained by ingestion.
// Rarity is deliberately ignored because ordinary and relic equipment can
// share rarity IDs, including ordinary rarity-5 equipment and slot-6 heroes.
func EquipmentFamily(item State.EquipmentInstance) LoadoutFamily {
	if !item.RelicKnown {
		return LoadoutFamilyUnknown
	}
	if item.Relic {
		return LoadoutFamilyRelic
	}
	return LoadoutFamilyOrdinary
}

// GemFamily follows the canonical instance identity used by gem commands:
// ordinary catalog gems have parent-derived negative IDs, while relic gems
// retain their positive server instance IDs.
func GemFamily(gem State.GemInstance) LoadoutFamily {
	if gem.ID < 0 {
		return LoadoutFamilyOrdinary
	}
	if gem.ID > 0 {
		return LoadoutFamilyRelic
	}
	return LoadoutFamilyUnknown
}

func GemMatchesEquipmentFamily(gem State.GemInstance, item State.EquipmentInstance) bool {
	gemFamily, equipmentFamily := GemFamily(gem), EquipmentFamily(item)
	return gemFamily != LoadoutFamilyUnknown && gemFamily == equipmentFamily
}

// RetainedAppearanceFamily reports the family constraint imposed by a gemmed
// slot-5 appearance item. The official client permits a skin without a gem to
// remain while either loadout family is equipped, but a gemmed skin keeps the
// lord in the skin's family. Reconfiguration never removes the user's skin.
func RetainedAppearanceFamily(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
) (LoadoutFamily, bool, error) {
	appearanceID := equipment["5"]
	if appearanceID == 0 {
		return LoadoutFamilyUnknown, false, nil
	}
	appearance, found := gameState.Inventory.Equipment[appearanceID]
	if !found || appearance.Slot != 5 {
		return LoadoutFamilyUnknown, false, fmt.Errorf("equipped appearance item %d is not in current state", appearanceID)
	}
	var socketed *State.GemInstance
	for _, candidate := range gameState.Inventory.Gems {
		if candidate.EquipmentInstanceID != appearanceID {
			continue
		}
		if socketed != nil {
			return LoadoutFamilyUnknown, false, fmt.Errorf("appearance item %d has more than one socketed gem", appearanceID)
		}
		gem := candidate
		socketed = &gem
	}
	if socketed == nil {
		return LoadoutFamilyUnknown, false, nil
	}
	family := EquipmentFamily(appearance)
	if family == LoadoutFamilyUnknown {
		return LoadoutFamilyUnknown, true, fmt.Errorf("gemmed appearance item %d has no verified ordinary or relic classification", appearanceID)
	}
	if !GemMatchesEquipmentFamily(*socketed, appearance) {
		return LoadoutFamilyUnknown, true, fmt.Errorf("gem %d does not match appearance item %d's ordinary or relic family", socketed.ID, appearanceID)
	}
	return family, true, nil
}
