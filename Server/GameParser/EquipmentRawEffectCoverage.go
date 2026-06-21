package GameParser

import (
	equip "CitadelDesktop/Server/Models/Equipment"
)

// RawEquipmentEffectCoverageFromGLI walks the unmarshalled GLI map ("C" / "B" + "EQ") and collects
// effect IDs from the same indices as ProcessEquipment / ProcessGem (stat row [0], gem stats at
// gem array [4]) before any CommStatUpdaterMap / CastStatUpdaterMap filter.
func RawEquipmentEffectCoverageFromGLI(gli map[string]interface{}) equip.EffectCoverageReport {
	if gli == nil {
		return equip.EffectCoverageReport{}
	}
	commMiss := make(map[float64]struct{})
	castMiss := make(map[float64]struct{})

	if ca, ok := gli["C"].([]interface{}); ok {
		for _, entry := range ca {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			eq, ok := m["EQ"].([]interface{})
			if !ok {
				continue
			}
			for _, piece := range eq {
				arr, ok := piece.([]interface{})
				if !ok {
					continue
				}
				ids := rawEffectIDsFromEquipmentArray(arr)
				recordMissingComm(ids, commMiss)
			}
		}
	}
	if ba, ok := gli["B"].([]interface{}); ok {
		for _, entry := range ba {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			eq, ok := m["EQ"].([]interface{})
			if !ok {
				continue
			}
			for _, piece := range eq {
				arr, ok := piece.([]interface{})
				if !ok {
					continue
				}
				ids := rawEffectIDsFromEquipmentArray(arr)
				recordMissingCast(ids, castMiss)
			}
		}
	}
	return equip.EffectCoverageReportFromSets(commMiss, castMiss, nil)
}

// RawEquipmentEffectCoverageFromStorageMap expects the unmarshalled equipment storage object (key "I").
func RawEquipmentEffectCoverageFromStorageMap(m map[string]interface{}) equip.EffectCoverageReport {
	if m == nil {
		return equip.EffectCoverageReport{}
	}
	commMiss := make(map[float64]struct{})
	castMiss := make(map[float64]struct{})
	arr, ok := m["I"].([]interface{})
	if !ok {
		return equip.EffectCoverageReportFromSets(commMiss, castMiss, nil)
	}
	for _, piece := range arr {
		arr2, ok := piece.([]interface{})
		if !ok {
			continue
		}
		equipType, _ := floatFrom(arr2[2])
		ids := rawEffectIDsFromEquipmentArray(arr2)
		switch equipType {
		case 2:
			recordMissingComm(ids, commMiss)
		case 1:
			recordMissingCast(ids, castMiss)
		default:
			recordMissingBothLoadoutMaps(ids, commMiss, castMiss)
		}
	}
	return equip.EffectCoverageReportFromSets(commMiss, castMiss, nil)
}

// RawEquipmentEffectCoverageFromGemStorageMap expects the unmarshalled gem storage object (key "RGEM").
func RawEquipmentEffectCoverageFromGemStorageMap(m map[string]interface{}) equip.EffectCoverageReport {
	if m == nil {
		return equip.EffectCoverageReport{}
	}
	stashMiss := make(map[float64]struct{})
	arr, ok := m["RGEM"].([]interface{})
	if !ok {
		return equip.EffectCoverageReportFromSets(nil, nil, stashMiss)
	}
	for _, g := range arr {
		gemArr, ok := g.([]interface{})
		if !ok {
			continue
		}
		ids := rawEffectIDsFromGemArray(gemArr)
		recordMissingGemStash(ids, stashMiss)
	}
	return equip.EffectCoverageReportFromSets(nil, nil, stashMiss)
}

func floatFrom(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

func rawEffectIDsFromEquipmentArray(arr []interface{}) []float64 {
	if len(arr) <= 5 {
		return nil
	}
	stats, ok := arr[5].([]interface{})
	if !ok {
		return nil
	}
	var ids []float64
	ids = append(ids, statRowEffectIDs(stats)...)
	rarity, _ := floatFrom(arr[3])
	if (rarity == 5 || rarity == 15) && len(arr) > 12 {
		if slot, ok := arr[12].([]interface{}); ok {
			ids = append(ids, rawEffectIDsFromExpandedGemSlot(slot)...)
		}
	}
	return ids
}

func rawEffectIDsFromExpandedGemSlot(slot []interface{}) []float64 {
	if len(slot) < 4 {
		return nil
	}
	gemArr, ok := slot[3].([]interface{})
	if !ok || len(gemArr) == 0 {
		return nil
	}
	return rawEffectIDsFromGemArray(gemArr)
}

func rawEffectIDsFromGemArray(gemArr []interface{}) []float64 {
	if len(gemArr) < 5 {
		return nil
	}
	stats, ok := gemArr[4].([]interface{})
	if !ok {
		return nil
	}
	return statRowEffectIDs(stats)
}

func statRowEffectIDs(stats []interface{}) []float64 {
	var ids []float64
	for _, s := range stats {
		row, ok := s.([]interface{})
		if !ok || len(row) < 1 {
			continue
		}
		id, ok := floatFrom(row[0])
		if !ok || id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func recordMissingComm(ids []float64, miss map[float64]struct{}) {
	for _, id := range ids {
		if !equip.CanApplyCommanderLiveStat(id) {
			miss[id] = struct{}{}
		}
	}
}

func recordMissingCast(ids []float64, miss map[float64]struct{}) {
	for _, id := range ids {
		if !equip.CanApplyCastellanLiveStat(id) {
			miss[id] = struct{}{}
		}
	}
}

func recordMissingGemStash(ids []float64, miss map[float64]struct{}) {
	for _, id := range ids {
		if !equip.CanApplyCommanderLiveStat(id) && !equip.CanApplyCastellanLiveStat(id) {
			miss[id] = struct{}{}
		}
	}
}

func recordMissingBothLoadoutMaps(ids []float64, commMiss, castMiss map[float64]struct{}) {
	for _, id := range ids {
		if !equip.CanApplyCommanderLiveStat(id) {
			commMiss[id] = struct{}{}
		}
		if !equip.CanApplyCastellanLiveStat(id) {
			castMiss[id] = struct{}{}
		}
	}
}
