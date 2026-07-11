package equipment

import "strings"

type CatalogEffectSource string

const (
	CatalogEffectSourceEquipment      CatalogEffectSource = "equipment"
	CatalogEffectSourceRelicEquipment CatalogEffectSource = "relic_equipment"
	CatalogEffectSourceGem            CatalogEffectSource = "gem"
	CatalogEffectSourceRelicGem       CatalogEffectSource = "relic_gem"
	CatalogEffectSourceSetBonus       CatalogEffectSource = "set_bonus"
)

func CanApplyCommanderLiveStat(id float64) bool {
	if _, ok := CommStatUpdaterMap[id]; ok {
		return true
	}
	return canResolveAnyLiveCatalogEffect(id)
}

func CanApplyCastellanLiveStat(id float64) bool {
	if _, ok := CastStatUpdaterMap[id]; ok {
		return true
	}
	return canResolveAnyLiveCatalogEffect(id)
}

func ApplyCommanderLiveStat(dst *CommStatModel, id float64, values []float64, source CatalogEffectSource) bool {
	updater, ok := CommStatUpdaterMap[id]
	if dst == nil || len(values) == 0 {
		return false
	}
	effect, resolved := resolveLiveCatalogEffect(id, source, 2)
	if resolved {
		addEquipmentEffects(&dst.Effects, effect, values, source)
		if applyCommanderCatalogEffect(dst, effect, values) {
			return true
		}
	}
	if ok {
		updater(dst, commanderLiveStatValue(id, values, source))
		return true
	}
	if resolved {
		addEquipmentExtraStat(&dst.ExtraStats, effect, values)
		return true
	}
	return false
}

func ApplyCastellanLiveStat(dst *CastStatModel, id float64, values []float64, source CatalogEffectSource) bool {
	updater, ok := CastStatUpdaterMap[id]
	if dst == nil || len(values) == 0 {
		return false
	}
	effect, resolved := resolveLiveCatalogEffect(id, source, 1)
	if resolved {
		addEquipmentEffects(&dst.Effects, effect, values, source)
		if applyCastellanCatalogEffect(dst, effect, values) {
			return true
		}
	}
	if ok {
		updater(dst, castellanLiveStatValue(id, values, source))
		return true
	}
	if resolved {
		addEquipmentExtraStat(&dst.ExtraStats, effect, values)
		return true
	}
	return false
}

func canResolveAnyLiveCatalogEffect(id float64) bool {
	for _, source := range []CatalogEffectSource{
		CatalogEffectSourceEquipment,
		CatalogEffectSourceRelicEquipment,
		CatalogEffectSourceGem,
		CatalogEffectSourceRelicGem,
		CatalogEffectSourceSetBonus,
	} {
		if _, ok := resolveLiveCatalogEffect(id, source, 0); ok {
			return true
		}
	}
	return false
}

func applyCommanderCatalogEffect(dst *CommStatModel, effect liveResolvedEffect, values []float64) bool {
	if !effect.HasDef {
		return false
	}
	value := liveCatalogEffectValue(effect, values)
	if value == 0 {
		return true
	}
	name := strings.ToLower(effect.Definition.Name)
	scope := liveEffectScope(effect)
	switch effect.Definition.TypeID {
	case 23:
		applyCommanderScoped(scope, value, &dst.MeleeCbtStr, &dst.NPCMelee, &dst.CLMelee)
	case 24:
		applyCommanderScoped(scope, value, &dst.RangeCbtStr, &dst.NPCRange, &dst.CLRange)
	case 19:
		applyCommanderScoped(scope, value, &dst.WallStr, &dst.NPCWall, &dst.CLWall)
	case 20:
		applyCommanderScoped(scope, value, &dst.GateStr, &dst.NPCGate, &dst.CLGate)
	case 21:
		applyCommanderScoped(scope, value, &dst.MoatStr, &dst.NPCMoat, &dst.CLMoat)
	case 15, 38:
		dst.Travel += value
	case 16, 37:
		dst.Loot += value
	case 28:
		applyCommanderScoped(scope, value, &dst.FlankLimit, &dst.NPCFlank, &dst.CLFlank)
	case 34:
		applyCommanderScoped(scope, value, &dst.FrontLimit, &dst.NPCFront, &dst.CLFront)
	case 33:
		applyCommanderScoped(scope, value, &dst.CyCbtStr, &dst.NPCCy, &dst.CLCy)
	case 36:
		dst.AllCbtStr += value
	case 53:
		dst.FrontCbtStr += value
	case 54:
		dst.FlankCbtStr += value
	case 51:
		if strings.Contains(name, "reinforcement") {
			dst.AttackReinforcement += value
		} else {
			dst.MaidenSupp += value
		}
	case 179:
		dst.AttackReinforcement += value
	case 156:
		dst.Wave += value
	case 27:
		dst.Cooldown += value
	case 17:
		dst.CLLater += value
	case 18:
		dst.CLFire += value
	case 13:
		dst.CLGlory += value
	case 105:
		dst.NPCGlory += value
	case 148:
		switch {
		case strings.Contains(name, "mead"):
			dst.MeadStr += value
		case strings.Contains(name, "relicbarracks"):
			dst.RelicStr += value
		case strings.Contains(name, "valkyrie") || strings.Contains(name, "beef"):
			dst.BeserkerStr += value
		case strings.Contains(name, "rankreward"):
			dst.HorrorStr += value
		case strings.Contains(name, "kingsguard") || strings.Contains(name, "berimond"):
			dst.EliteStr += value
		default:
			return false
		}
	default:
		return false
	}
	return true
}

func applyCastellanCatalogEffect(dst *CastStatModel, effect liveResolvedEffect, values []float64) bool {
	if !effect.HasDef {
		return false
	}
	value := liveCatalogEffectValue(effect, values)
	if value == 0 {
		return true
	}
	name := strings.ToLower(effect.Definition.Name)
	scope := liveEffectScope(effect)
	switch effect.Definition.TypeID {
	case 9:
		applyCastellanScoped(scope, value, &dst.MeleeCbtStr, &dst.NPCMelee, &dst.CLMelee)
	case 10:
		applyCastellanScoped(scope, value, &dst.RangeCbtStr, &dst.NPCRange, &dst.CLRange)
	case 6:
		applyCastellanScoped(scope, value, &dst.WallStr, &dst.NPCWall, &dst.CLWall)
	case 7:
		applyCastellanScoped(scope, value, &dst.GateStr, &dst.NPCGate, &dst.CLGate)
	case 8:
		applyCastellanScoped(scope, value, &dst.MoatStr, &dst.NPCMoat, &dst.CLMoat)
	case 12, 46:
		applyCastellanScoped(scope, value, &dst.WallLimit, &dst.NPCWallLimit, &dst.CLWallLimit)
	case 32:
		applyCastellanScoped(scope, value, &dst.CyCbtStr, &dst.NPCCy, &dst.CLCy)
	case 31:
		switch {
		case strings.Contains(name, "notmain"):
			dst.OpCbtStr += value
		case strings.Contains(name, "maincastle"):
			dst.MainCbtStr += value
		default:
			dst.AllCbtStr += value
		}
	case 49:
		dst.FrontCbtStr += value
	case 50:
		dst.FlankCbtStr += value
	case 47:
		dst.ProtectorSupp += value
	case 2, 16, 37:
		dst.Loot += value
	case 86:
		dst.Recruit += value
	case 107:
		dst.Research += value
	case 119:
		dst.Hospital += value
	case 145:
		dst.Construction += value
	case 146:
		dst.BaseRes += value
	case 147:
		dst.KingRes += value
	case 123:
		dst.PO += value
	case 151:
		dst.ResTransport += value
	case 164:
		dst.MeadProd += value
	case 165:
		dst.HoneyProd += value
	case 166:
		dst.MeadStorage += value
	case 167:
		dst.HoneyStorage += value
	case 3:
		dst.CLEarly += value
	case 4, 18:
		dst.CLFire += value
	case 0, 13:
		dst.CLGlory += value
	default:
		return false
	}
	return true
}

func applyCommanderScoped(scope string, value float64, generic *float64, pve *float64, pvp *float64) {
	switch scope {
	case "pve":
		*pve += value
	case "pvp":
		*pvp += value
	default:
		*generic += value
	}
}

func applyCastellanScoped(scope string, value float64, generic *float64, pve *float64, pvp *float64) {
	switch scope {
	case "pve":
		*pve += value
	case "pvp":
		*pvp += value
	default:
		*generic += value
	}
}

func commanderLiveStatValue(id float64, values []float64, source CatalogEffectSource) float64 {
	if (source == CatalogEffectSourceEquipment || source == CatalogEffectSourceRelicEquipment) && (id == 121 || inRange(id, 20012, 20020)) {
		return valueAt(values, 1)
	}
	if (source == CatalogEffectSourceGem || source == CatalogEffectSourceRelicGem) && inRange(id, 20012, 20020) {
		return valueAt(values, 1)
	}
	return valueAt(values, 0)
}

func castellanLiveStatValue(id float64, values []float64, source CatalogEffectSource) float64 {
	if (source == CatalogEffectSourceEquipment || source == CatalogEffectSourceRelicEquipment) && (id == 10118 || inRange(id, 20012, 20020)) {
		return valueAt(values, 1)
	}
	if (source == CatalogEffectSourceGem || source == CatalogEffectSourceRelicGem) && inRange(id, 20012, 20020) {
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
