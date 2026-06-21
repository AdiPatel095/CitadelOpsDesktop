package equipment

type CatalogEffectSource string

const (
	CatalogEffectSourceEquipment CatalogEffectSource = "equipment"
	CatalogEffectSourceGem       CatalogEffectSource = "gem"
)

func CanApplyCommanderLiveStat(id float64) bool {
	_, ok := CommStatUpdaterMap[id]
	return ok
}

func CanApplyCastellanLiveStat(id float64) bool {
	_, ok := CastStatUpdaterMap[id]
	return ok
}

func ApplyCommanderLiveStat(dst *CommStatModel, id float64, values []float64, source CatalogEffectSource) bool {
	updater, ok := CommStatUpdaterMap[id]
	if !ok || dst == nil || len(values) == 0 {
		return false
	}
	updater(dst, commanderLiveStatValue(id, values, source))
	return true
}

func ApplyCastellanLiveStat(dst *CastStatModel, id float64, values []float64, source CatalogEffectSource) bool {
	updater, ok := CastStatUpdaterMap[id]
	if !ok || dst == nil || len(values) == 0 {
		return false
	}
	updater(dst, castellanLiveStatValue(id, values, source))
	return true
}

func commanderLiveStatValue(id float64, values []float64, source CatalogEffectSource) float64 {
	if source == CatalogEffectSourceEquipment && (id == 121 || inRange(id, 20012, 20017)) {
		return valueAt(values, 1)
	}
	if source == CatalogEffectSourceGem && inRange(id, 20012, 20017) {
		return valueAt(values, 1)
	}
	return valueAt(values, 0)
}

func castellanLiveStatValue(id float64, values []float64, source CatalogEffectSource) float64 {
	if source == CatalogEffectSourceEquipment && (id == 10118 || inRange(id, 20012, 20017)) {
		return valueAt(values, 1)
	}
	if source == CatalogEffectSourceGem && inRange(id, 20012, 20017) {
		return valueAt(values, 1)
	}
	return valueAt(values, 0)
}

func valueAt(values []float64, index int) float64 {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return values[0]
}

func inRange(id, min, max float64) bool {
	return id >= min && id <= max
}
