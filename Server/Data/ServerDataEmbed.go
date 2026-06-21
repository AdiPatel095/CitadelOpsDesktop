// Package serverdata embeds official game JSON from this tree for shipped binaries.
// Runtime still prefers on-disk Server/Data when present (dev, Docker, manual updates).
package serverdata

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed construction_items/items.json
var embeddedConstructionItemsJSON []byte

//go:embed effects/items.json
var embeddedEffectsItemsJSON []byte

//go:embed equipment_effects/items.json
var embeddedEquipmentEffectsItemsJSON []byte

//go:embed effect_caps/items.json
var embeddedEffectCapsItemsJSON []byte

//go:embed relic_effects/items.json
var embeddedRelicEffectsItemsJSON []byte

//go:embed equipments/items.json
var embeddedEquipmentsItemsJSON []byte

//go:embed gems/items.json
var embeddedGemsItemsJSON []byte

//go:embed equipment_sets/items.json
var embeddedEquipmentSetsItemsJSON []byte

//go:embed construction_item_group_buildings.json
var embeddedConstructionItemGroupBuildingsJSON []byte

//go:embed LangEn.json
var embeddedLangEnJSON []byte

//go:embed packages/items.json
var embeddedPackagesItemsJSON []byte

func readFilePreferDisk(relPath string, embedded []byte) ([]byte, error) {
	if b, err := readFromDisk(relPath); err == nil {
		return b, nil
	}
	if len(embedded) == 0 {
		return nil, fmt.Errorf("%s not found on disk and embedded copy is empty", relPath)
	}
	return embedded, nil
}

func readFromDisk(relPath string) ([]byte, error) {
	try := func(p string) ([]byte, error) {
		return os.ReadFile(p)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, p := range []string{
			filepath.Join(dir, "Server", "Data", relPath),
			filepath.Join(dir, "..", "Server", "Data", relPath),
		} {
			if b, err := try(p); err == nil {
				return b, nil
			}
		}
	}
	for _, base := range []string{".", "..", filepath.Join("..", "..")} {
		p := filepath.Join(base, "Server", "Data", relPath)
		if b, err := try(p); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("Server/Data/%s not found", relPath)
}

// ReadConstructionItemsJSON returns construction_items/items.json (disk first, else embedded).
func ReadConstructionItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("construction_items", "items.json"), embeddedConstructionItemsJSON)
}

// ReadEffectsItemsJSON returns effects/items.json (disk first, else embedded).
func ReadEffectsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("effects", "items.json"), embeddedEffectsItemsJSON)
}

// ReadEquipmentEffectsItemsJSON returns equipment_effects/items.json (disk first, else embedded).
func ReadEquipmentEffectsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("equipment_effects", "items.json"), embeddedEquipmentEffectsItemsJSON)
}

// ReadEffectCapsItemsJSON returns effect_caps/items.json.
func ReadEffectCapsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("effect_caps", "items.json"), embeddedEffectCapsItemsJSON)
}

// ReadRelicEffectsItemsJSON returns relic_effects/items.json.
func ReadRelicEffectsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("relic_effects", "items.json"), embeddedRelicEffectsItemsJSON)
}

// ReadEquipmentsItemsJSON returns equipments/items.json.
func ReadEquipmentsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("equipments", "items.json"), embeddedEquipmentsItemsJSON)
}

// ReadGemsItemsJSON returns gems/items.json.
func ReadGemsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("gems", "items.json"), embeddedGemsItemsJSON)
}

// ReadEquipmentSetsItemsJSON returns equipment_sets/items.json.
func ReadEquipmentSetsItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("equipment_sets", "items.json"), embeddedEquipmentSetsItemsJSON)
}

// ReadBuildingsJSON returns buildings/items.json (official EmpireItems building catalog).
// Used for dev/Docker overrides; production exe-only installs use the embedded compact map instead.
func ReadBuildingsJSON() ([]byte, error) {
	return readFromDisk(filepath.Join("buildings", "items.json"))
}

// ReadConstructionItemGroupBuildingsJSON returns construction_item_group_buildings.json
// (groupID → wodID list). Disk first for dev overrides, else embedded for production binaries.
func ReadConstructionItemGroupBuildingsJSON() ([]byte, error) {
	return readFilePreferDisk(
		"construction_item_group_buildings.json",
		embeddedConstructionItemGroupBuildingsJSON,
	)
}

// ReadLangEnJSON returns LangEn.json (GGE English strings for ci_* construction item names).
// Disk first for dev overrides, else embedded for production binaries (GeneralsCamp-style labels).
func ReadLangEnJSON() ([]byte, error) {
	return readFilePreferDisk("LangEn.json", embeddedLangEnJSON)
}

// ReadPackagesItemsJSON returns packages/items.json (official shop package catalog).
// Disk first for dev overrides, else embedded for production binaries.
func ReadPackagesItemsJSON() ([]byte, error) {
	return readFilePreferDisk(filepath.Join("packages", "items.json"), embeddedPackagesItemsJSON)
}
