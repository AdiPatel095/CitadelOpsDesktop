package GameParser

import (
	"CitadelDesktop/Server/Models"
	equip "CitadelDesktop/Server/Models/Equipment"
	"math"
)

func UpdateEquipmentList(gliMap map[string]interface{}) {
	gs := Models.GetGameState()
	castArray, ok := gliMap["B"].([]interface{})
	if !ok {
		return // castArray is not a slice, skip processing
	}
	commArray, ok := gliMap["C"].([]interface{})
	if !ok {
		return // commArray is not a slice, skip processing
	}

	gs.Equipment.CommActualArray = make([]Models.CommActualModel, len(commArray))
	equip.CommStatArray = make([]Models.CommStatModel, len(commArray))

	gs.Equipment.CastActualArray = make([]Models.CastActualModel, 0, len(castArray))
	equip.CastStatArray = make([]Models.CastStatModel, 0, len(castArray))

	ProcessCastArray(castArray)
	ProcessCommArray(commArray)
}

func ProcessCastArray(castArray []interface{}) {
	gs := Models.GetGameState()
	for _, castData := range castArray {
		castMap, ok := castData.(map[string]interface{})
		if !ok {
			continue // castMap is not a map, skip
		}
		castleID, ok := castMap["LICID"].(float64)
		if !ok {
			continue // castleID is not a float64, skip
		}

		var tempCastStat Models.CastStatModel
		var tempCastActual Models.CastActualModel

		c := &gs.Castle
		// Set default or known values (CastlePosition 0..10 = fixed slots)
		switch castleID {
		case c.MainCastle.Aid:
			tempCastStat.Name = c.MainCastle.Name
			tempCastStat.CastlePosition = 0
		case c.Outpost1.Aid:
			tempCastStat.Name = c.Outpost1.Name
			tempCastStat.CastlePosition = 1
		case c.Outpost2.Aid:
			tempCastStat.Name = c.Outpost2.Name
			tempCastStat.CastlePosition = 2
		case c.Outpost3.Aid:
			tempCastStat.Name = c.Outpost3.Name
			tempCastStat.CastlePosition = 3
		case c.IceCastle.Aid:
			tempCastStat.Name = c.IceCastle.Name
			tempCastStat.CastlePosition = 4
		case c.DesertCastle.Aid:
			tempCastStat.Name = c.DesertCastle.Name
			tempCastStat.CastlePosition = 5
		case c.DungeonCastle.Aid:
			tempCastStat.Name = c.DungeonCastle.Name
			tempCastStat.CastlePosition = 6
		case c.StormCastle.Aid:
			tempCastStat.Name = c.StormCastle.Name
			tempCastStat.CastlePosition = 7
		case c.Metropolis.Aid:
			tempCastStat.Name = c.Metropolis.Name
			tempCastStat.CastlePosition = 8
		case c.Capital.Aid:
			tempCastStat.Name = c.Capital.Name
			tempCastStat.CastlePosition = 9
		default:
			tempCastStat.Name = "Unknown Castle"
			tempCastStat.CastlePosition = 99
		}

		ProcessCast(castMap, &tempCastActual, &tempCastStat)

		gs.Equipment.CastActualArray = append(gs.Equipment.CastActualArray, tempCastActual)
		equip.CastStatArray = append(equip.CastStatArray, tempCastStat)
	}
}

func ProcessCast(castMap map[string]interface{}, castActual *Models.CastActualModel, castStat *Models.CastStatModel) {
	castActual.ID = castMap["ID"].(float64)
	castActual.CastleID = castMap["LICID"].(float64)
	equipmentRawArray, _ := castMap["EQ"].([]interface{})
	// Initialize a slice with 0 length but pre-allocated capacity for efficiency.
	equipmentArray := make([]Models.EquipmentModel, 0, len(equipmentRawArray))
	for _, equipmentData := range equipmentRawArray {
		var equipmentFinal Models.EquipmentModel
		equipmentDataArray, _ := equipmentData.([]interface{})
		ProcessEquipment(equipmentDataArray, &equipmentFinal)
		// The return from append must be assigned back to the slice.
		equipmentArray = append(equipmentArray, equipmentFinal)
	}
	castActual.Equipment = equipmentArray

	castStat.ID = castActual.ID
	castStat.CastleID = castActual.CastleID
	var tempEquipStat Models.CastStatModel
	var tempHeroStat Models.CastStatModel
	var tempGemStat Models.CastStatModel
	for _, equipment := range castActual.Equipment {
		switch equipment.EquipSlotNumber {
		case 1:
			castStat.Equip1 = equipment.ID
			ProcessEquipStatCast(equipment, &tempEquipStat, &equip.CastEquipCeiling)
		case 2:
			castStat.Equip2 = equipment.ID
			ProcessEquipStatCast(equipment, &tempEquipStat, &equip.CastEquipCeiling)
		case 3:
			castStat.Equip3 = equipment.ID
			ProcessEquipStatCast(equipment, &tempEquipStat, &equip.CastEquipCeiling)
		case 4:
			castStat.Equip4 = equipment.ID
			ProcessEquipStatCast(equipment, &tempEquipStat, &equip.CastEquipCeiling)
		case 6:
			castStat.Hero = equipment.ID
			ProcessEquipStatCast(equipment, &tempHeroStat, &equip.CastHeroCeiling)
		}
		// Process gems for Relic equipment (Rarity 5 or 15) with valid gems
		if (equipment.EquipRarity == 5 || equipment.EquipRarity == 15) && equipment.GemSlot.Gem != nil {
			// Castellan gem slots use 101-104 for equipment, 161 for hero
			// We need to map these to Gem1-4
			gemSlotNum := equipment.GemSlot.SlotNumber
			switch {
			case gemSlotNum == 101 || gemSlotNum == 1:
				castStat.Gem1 = equipment.GemSlot.Gem.ID
			case gemSlotNum == 102 || gemSlotNum == 2:
				castStat.Gem2 = equipment.GemSlot.Gem.ID
			case gemSlotNum == 103 || gemSlotNum == 3:
				castStat.Gem3 = equipment.GemSlot.Gem.ID
			case gemSlotNum == 104 || gemSlotNum == 4:
				castStat.Gem4 = equipment.GemSlot.Gem.ID
			}
			ProcessGemStatCast(equipment, &tempGemStat, &equip.CastGemCeiling)
		}
	}
	MergeCast(castStat, &tempEquipStat, &tempHeroStat, &tempGemStat)
	Models.ApplyCastCeiling(castStat, &equip.CastEquipCeiling)
}

func ProcessEquipStatCast(equipment Models.EquipmentModel, dstCast *Models.CastStatModel, ceilingCast *Models.CastStatModel) {
	for _, stat := range equipment.EquipStats {
		if len(stat.Value) == 0 {
			continue
		}
		equip.ApplyCastellanLiveStat(dstCast, stat.ID, stat.Value, equip.CatalogEffectSourceEquipment)
		Models.ApplyCastCeiling(dstCast, ceilingCast)
	}
}

func ProcessGemStatCast(equipment Models.EquipmentModel, dstCast *Models.CastStatModel, ceilingCast *Models.CastStatModel) {
	for _, stat := range equipment.GemSlot.Gem.GemStats {
		if len(stat.Value) == 0 {
			continue
		}
		equip.ApplyCastellanLiveStat(dstCast, stat.ID, stat.Value, equip.CatalogEffectSourceGem)
		Models.ApplyCastCeiling(dstCast, ceilingCast)
	}
}

func MergeCast(dstCast *Models.CastStatModel, tempEquipStat *Models.CastStatModel, tempHeroStat *Models.CastStatModel, tempGemStat *Models.CastStatModel) {
	dstCast.MeleeCbtStr += tempEquipStat.MeleeCbtStr + tempHeroStat.MeleeCbtStr + tempGemStat.MeleeCbtStr
	dstCast.RangeCbtStr += tempEquipStat.RangeCbtStr + tempHeroStat.RangeCbtStr + tempGemStat.RangeCbtStr
	dstCast.OpCbtStr += tempEquipStat.OpCbtStr + tempHeroStat.OpCbtStr + tempGemStat.OpCbtStr
	dstCast.MainCbtStr += tempEquipStat.MainCbtStr + tempHeroStat.MainCbtStr + tempGemStat.MainCbtStr
	dstCast.CyCbtStr += tempEquipStat.CyCbtStr + tempHeroStat.CyCbtStr + tempGemStat.CyCbtStr
	dstCast.AllCbtStr += tempEquipStat.AllCbtStr + tempHeroStat.AllCbtStr + tempGemStat.AllCbtStr
	dstCast.FrontCbtStr += tempEquipStat.FrontCbtStr + tempHeroStat.FrontCbtStr + tempGemStat.FrontCbtStr
	dstCast.FlankCbtStr += tempEquipStat.FlankCbtStr + tempHeroStat.FlankCbtStr + tempGemStat.FlankCbtStr
	dstCast.WallStr += tempEquipStat.WallStr + tempHeroStat.WallStr + tempGemStat.WallStr
	dstCast.GateStr += tempEquipStat.GateStr + tempHeroStat.GateStr + tempGemStat.GateStr
	dstCast.MoatStr += tempEquipStat.MoatStr + tempHeroStat.MoatStr + tempGemStat.MoatStr
	dstCast.WallLimit += tempEquipStat.WallLimit + tempHeroStat.WallLimit + tempGemStat.WallLimit
	dstCast.ProtectorSupp += tempEquipStat.ProtectorSupp + tempHeroStat.ProtectorSupp + tempGemStat.ProtectorSupp
	dstCast.Loot += tempEquipStat.Loot + tempHeroStat.Loot + tempGemStat.Loot
	dstCast.Recruit += tempEquipStat.Recruit + tempHeroStat.Recruit + tempGemStat.Recruit
	dstCast.MeadProd += tempEquipStat.MeadProd + tempHeroStat.MeadProd + tempGemStat.MeadProd
	dstCast.Research += tempEquipStat.Research + tempHeroStat.Research + tempGemStat.Research
	dstCast.Hospital += tempEquipStat.Hospital + tempHeroStat.Hospital + tempGemStat.Hospital
	dstCast.Construction += tempEquipStat.Construction + tempHeroStat.Construction + tempGemStat.Construction
	dstCast.BaseRes += tempEquipStat.BaseRes + tempHeroStat.BaseRes + tempGemStat.BaseRes
	dstCast.KingRes += tempEquipStat.KingRes + tempHeroStat.KingRes + tempGemStat.KingRes
	dstCast.PO += tempEquipStat.PO + tempHeroStat.PO + tempGemStat.PO
	dstCast.ResTransport += tempEquipStat.ResTransport + tempHeroStat.ResTransport + tempGemStat.ResTransport
	dstCast.HoneyProd += tempEquipStat.HoneyProd + tempHeroStat.HoneyProd + tempGemStat.HoneyProd
	dstCast.MeadStorage += tempEquipStat.MeadStorage + tempHeroStat.MeadStorage + tempGemStat.MeadStorage
	dstCast.HoneyStorage += tempEquipStat.HoneyStorage + tempHeroStat.HoneyStorage + tempGemStat.HoneyStorage
	dstCast.NPCMelee += tempEquipStat.NPCMelee + tempHeroStat.NPCMelee + tempGemStat.NPCMelee
	dstCast.NPCRange += tempEquipStat.NPCRange + tempHeroStat.NPCRange + tempGemStat.NPCRange
	dstCast.NPCFront += tempEquipStat.NPCFront + tempHeroStat.NPCFront + tempGemStat.NPCFront
	dstCast.NPCFlank += tempEquipStat.NPCFlank + tempHeroStat.NPCFlank + tempGemStat.NPCFlank
	dstCast.NPCCy += tempEquipStat.NPCCy + tempHeroStat.NPCCy + tempGemStat.NPCCy
	dstCast.NPCWall += tempEquipStat.NPCWall + tempHeroStat.NPCWall + tempGemStat.NPCWall
	dstCast.NPCGate += tempEquipStat.NPCGate + tempHeroStat.NPCGate + tempGemStat.NPCGate
	dstCast.NPCMoat += tempEquipStat.NPCMoat + tempHeroStat.NPCMoat + tempGemStat.NPCMoat
	dstCast.NPCWallLimit += tempEquipStat.NPCWallLimit + tempHeroStat.NPCWallLimit + tempGemStat.NPCWallLimit
	dstCast.CLMelee += tempEquipStat.CLMelee + tempHeroStat.CLMelee + tempGemStat.CLMelee
	dstCast.CLRange += tempEquipStat.CLRange + tempHeroStat.CLRange + tempGemStat.CLRange
	dstCast.CLCy += tempEquipStat.CLCy + tempHeroStat.CLCy + tempGemStat.CLCy
	dstCast.CLWall += tempEquipStat.CLWall + tempHeroStat.CLWall + tempGemStat.CLWall
	dstCast.CLGate += tempEquipStat.CLGate + tempHeroStat.CLGate + tempGemStat.CLGate
	dstCast.CLMoat += tempEquipStat.CLMoat + tempHeroStat.CLMoat + tempGemStat.CLMoat
	dstCast.CLWallLimit += tempEquipStat.CLWallLimit + tempHeroStat.CLWallLimit + tempGemStat.CLWallLimit
	dstCast.CLFire += tempEquipStat.CLFire + tempHeroStat.CLFire + tempGemStat.CLFire
	dstCast.CLGlory += tempEquipStat.CLGlory + tempHeroStat.CLGlory + tempGemStat.CLGlory
	dstCast.CLEarly += tempEquipStat.CLEarly + tempHeroStat.CLEarly + tempGemStat.CLEarly
}

func ProcessCommArray(commArray []interface{}) {
	for index, commData := range commArray {
		commMap, ok := commData.(map[string]interface{})
		if !ok {
			continue // commMap is not a map, skip
		}
		ProcessComm(commMap, index)
	}
}

func ProcessComm(commMap map[string]interface{}, index int) {
	actualComm := &Models.GetGameState().Equipment.CommActualArray[index]
	actualComm.ID = commMap["ID"].(float64)
	actualComm.Name = commMap["N"].(string)
	actualComm.VisiblePosition = commMap["VIS"].(float64) + 1
	equipmentRawArray, _ := commMap["EQ"].([]interface{})
	equipmentArray := make([]Models.EquipmentModel, 0, len(equipmentRawArray))
	for _, equipmentData := range equipmentRawArray {
		var equipmentFinal Models.EquipmentModel
		equipmentDataArray, _ := equipmentData.([]interface{})
		ProcessEquipment(equipmentDataArray, &equipmentFinal)
		equipmentArray = append(equipmentArray, equipmentFinal)
	}
	actualComm.Equipment = equipmentArray
	statComm := &equip.CommStatArray[index]
	statComm.ID = actualComm.ID
	statComm.Name = actualComm.Name
	var tempEquipStat Models.CommStatModel
	var tempHeroStat Models.CommStatModel
	var tempGemStat Models.CommStatModel
	for _, equipment := range actualComm.Equipment {
		switch equipment.EquipSlotNumber {
		case 1:
			statComm.Equip1 = equipment.ID
			ProcessEquipStatComm(equipment, &tempEquipStat, &equip.CommEquipCeiling)
		case 2:
			statComm.Equip2 = equipment.ID
			ProcessEquipStatComm(equipment, &tempEquipStat, &equip.CommEquipCeiling)
		case 3:
			statComm.Equip3 = equipment.ID
			ProcessEquipStatComm(equipment, &tempEquipStat, &equip.CommEquipCeiling)
		case 4:
			statComm.Equip4 = equipment.ID
			ProcessEquipStatComm(equipment, &tempEquipStat, &equip.CommEquipCeiling)
		case 6:
			statComm.Hero = equipment.ID
			ProcessEquipStatComm(equipment, &tempHeroStat, &equip.CommHeroCeiling)
		}
		// Check if the equipment is gem-capable AND a gem is actually present (Gem.ID will be non-zero).
		if (equipment.EquipRarity == 5 || equipment.EquipRarity == 15) && equipment.GemSlot.Gem != nil && equipment.GemSlot.Gem.ID != 0 {
			switch equipment.GemSlot.SlotNumber {
			case 1:
				statComm.Gem1 = equipment.GemSlot.Gem.ID
			case 2:
				statComm.Gem2 = equipment.GemSlot.Gem.ID
			case 3:
				statComm.Gem3 = equipment.GemSlot.Gem.ID
			case 4:
				statComm.Gem4 = equipment.GemSlot.Gem.ID
			}
			ProcessGemStatComm(equipment, &tempGemStat, &equip.CommGemCeiling)
		}

	}
	MergeComm(statComm, &tempEquipStat, &tempHeroStat, &tempGemStat)
	Models.ApplyCommCeiling(statComm, &equip.CommEquipCeiling)
}

func ProcessEquipStatComm(equipment Models.EquipmentModel, dstComm *Models.CommStatModel, ceilingComm *Models.CommStatModel) {
	for _, stat := range equipment.EquipStats {
		if len(stat.Value) == 0 {
			continue
		}
		equip.ApplyCommanderLiveStat(dstComm, stat.ID, stat.Value, equip.CatalogEffectSourceEquipment)
		Models.ApplyCommCeiling(dstComm, ceilingComm)

	}
}

func ProcessGemStatComm(equipment Models.EquipmentModel, dstComm *Models.CommStatModel, ceilingComm *Models.CommStatModel) {
	for _, stat := range equipment.GemSlot.Gem.GemStats {
		if len(stat.Value) == 0 {
			continue
		}
		equip.ApplyCommanderLiveStat(dstComm, stat.ID, stat.Value, equip.CatalogEffectSourceGem)
		Models.ApplyCommCeiling(dstComm, ceilingComm)
	}
}

func MergeComm(dstComm *Models.CommStatModel, tempEquipStat *Models.CommStatModel, tempHeroStat *Models.CommStatModel, tempGemStat *Models.CommStatModel) {
	dstComm.MeleeCbtStr += tempEquipStat.MeleeCbtStr + tempHeroStat.MeleeCbtStr + tempGemStat.MeleeCbtStr
	dstComm.RangeCbtStr += tempEquipStat.RangeCbtStr + tempHeroStat.RangeCbtStr + tempGemStat.RangeCbtStr
	dstComm.FrontCbtStr += tempEquipStat.FrontCbtStr + tempHeroStat.FrontCbtStr + tempGemStat.FrontCbtStr
	dstComm.FlankCbtStr += tempEquipStat.FlankCbtStr + tempHeroStat.FlankCbtStr + tempGemStat.FlankCbtStr
	dstComm.AllCbtStr += tempEquipStat.AllCbtStr + tempHeroStat.AllCbtStr + tempGemStat.AllCbtStr
	dstComm.CyCbtStr += tempEquipStat.CyCbtStr + tempHeroStat.CyCbtStr + tempGemStat.CyCbtStr
	dstComm.WallStr += tempEquipStat.WallStr + tempHeroStat.WallStr + tempGemStat.WallStr
	dstComm.GateStr += tempEquipStat.GateStr + tempHeroStat.GateStr + tempGemStat.GateStr
	dstComm.MoatStr += tempEquipStat.MoatStr + tempHeroStat.MoatStr + tempGemStat.MoatStr
	dstComm.FlankLimit += tempEquipStat.FlankLimit + tempHeroStat.FlankLimit + tempGemStat.FlankLimit
	dstComm.FrontLimit += tempEquipStat.FrontLimit + tempHeroStat.FrontLimit + tempGemStat.FrontLimit
	dstComm.MeadStr += tempEquipStat.MeadStr + tempHeroStat.MeadStr + tempGemStat.MeadStr
	dstComm.HorrorStr += tempEquipStat.HorrorStr + tempHeroStat.HorrorStr + tempGemStat.HorrorStr
	dstComm.EliteStr += tempEquipStat.EliteStr + tempHeroStat.EliteStr + tempGemStat.EliteStr
	dstComm.Wave += tempEquipStat.Wave + tempHeroStat.Wave + tempGemStat.Wave
	dstComm.Cooldown += tempEquipStat.Cooldown + tempHeroStat.Cooldown + tempGemStat.Cooldown
	dstComm.RelicStr += tempEquipStat.RelicStr + tempHeroStat.RelicStr + tempGemStat.RelicStr
	dstComm.BeserkerStr += tempEquipStat.BeserkerStr + tempHeroStat.BeserkerStr + tempGemStat.BeserkerStr
	dstComm.MaidenSupp += tempEquipStat.MaidenSupp + tempHeroStat.MaidenSupp + tempGemStat.MaidenSupp
	dstComm.Travel += tempEquipStat.Travel + tempHeroStat.Travel + tempGemStat.Travel
	dstComm.Loot += tempEquipStat.Loot + tempHeroStat.Loot + tempGemStat.Loot
	dstComm.NPCMelee += tempEquipStat.NPCMelee + tempHeroStat.NPCMelee + tempGemStat.NPCMelee
	dstComm.NPCRange += tempEquipStat.NPCRange + tempHeroStat.NPCRange + tempGemStat.NPCRange
	dstComm.NPCFront += tempEquipStat.NPCFront + tempHeroStat.NPCFront + tempGemStat.NPCFront
	dstComm.NPCFlank += tempEquipStat.NPCFlank + tempHeroStat.NPCFlank + tempGemStat.NPCFlank
	dstComm.NPCCy += tempEquipStat.NPCCy + tempHeroStat.NPCCy + tempGemStat.NPCCy
	dstComm.NPCWall += tempEquipStat.NPCWall + tempHeroStat.NPCWall + tempGemStat.NPCWall
	dstComm.NPCGate += tempEquipStat.NPCGate + tempHeroStat.NPCGate + tempGemStat.NPCGate
	dstComm.NPCMoat += tempEquipStat.NPCMoat + tempHeroStat.NPCMoat + tempGemStat.NPCMoat
	dstComm.NPCGlory += tempEquipStat.NPCGlory + tempHeroStat.NPCGlory + tempGemStat.NPCGlory
	dstComm.CLMelee += tempEquipStat.CLMelee + tempHeroStat.CLMelee + tempGemStat.CLMelee
	dstComm.CLRange += tempEquipStat.CLRange + tempHeroStat.CLRange + tempGemStat.CLRange
	dstComm.CLFront += tempEquipStat.CLFront + tempHeroStat.CLFront + tempGemStat.CLFront
	dstComm.CLFlank += tempEquipStat.CLFlank + tempHeroStat.CLFlank + tempGemStat.CLFlank
	dstComm.CLCy += tempEquipStat.CLCy + tempHeroStat.CLCy + tempGemStat.CLCy
	dstComm.CLWall += tempEquipStat.CLWall + tempHeroStat.CLWall + tempGemStat.CLWall
	dstComm.CLGate += tempEquipStat.CLGate + tempHeroStat.CLGate + tempGemStat.CLGate
	dstComm.CLMoat += tempEquipStat.CLMoat + tempHeroStat.CLMoat + tempGemStat.CLMoat
	dstComm.CLLater += tempEquipStat.CLLater + tempHeroStat.CLLater + tempGemStat.CLLater
	dstComm.CLFire += tempEquipStat.CLFire + tempHeroStat.CLFire + tempGemStat.CLFire
	dstComm.CLGlory += tempEquipStat.CLGlory + tempHeroStat.CLGlory + tempGemStat.CLGlory
}

func ProcessEquipment(equipmentDataArray []interface{}, equipmentFinal *Models.EquipmentModel) {
	equipmentFinal.ID, _ = equipmentDataArray[0].(float64)
	equipmentFinal.EquipSlotNumber, _ = equipmentDataArray[1].(float64)
	equipmentFinal.EquipType, _ = equipmentDataArray[2].(float64)
	equipmentFinal.EquipRarity, _ = equipmentDataArray[3].(float64)
	equipmentFinal.PlaceHolder6, _ = equipmentDataArray[4].(float64)
	equipmentFinal.EquipLevel, _ = equipmentDataArray[8].(float64)

	equipStatsRawArray, _ := equipmentDataArray[5].([]interface{})

	// Ensure array has enough elements to avoid panic
	if len(equipmentDataArray) > 6 {
		equipmentFinal.TemplateID, _ = equipmentDataArray[6].(float64)
	}
	if len(equipmentDataArray) > 10 {
		equipmentFinal.GemID, _ = equipmentDataArray[10].(float64)
	}
	// Initialize a slice with 0 length but pre-allocated capacity.
	equipStatsArray := make([]Models.Stat, 0, len(equipStatsRawArray))
	for _, equipStatData := range equipStatsRawArray {
		var equipStatFinal Models.Stat
		equipStatDataArray, _ := equipStatData.([]interface{})
		ProcessStatToActual(equipStatDataArray, &equipStatFinal, equipmentFinal.EquipRarity)
		// Assign the result of append back to the slice.
		equipStatsArray = append(equipStatsArray, equipStatFinal)
	}

	// Gem Processing
	// Case 1: Castellan (Type 1) - Compressed Data Structure (Index 9 = GemID, Index 11 = Slot?)
	if equipmentFinal.EquipType == 1 {
		// Log indicates Castellan data is length 12 with GemID at index 9 (if > -1)
		if len(equipmentDataArray) >= 10 {
			gemIDRaw, ok := equipmentDataArray[9].(float64)
			if ok && gemIDRaw > 0 {
				var gemSlot Models.GemSlot
				if len(equipmentDataArray) >= 12 {
					gemSlot.SlotNumber, _ = equipmentDataArray[11].(float64)
				}

				// Construct Gem
				gem := Models.Gem{
					ID: gemIDRaw,
					// Type/Stats are unknown in this compressed format, but ID allows unequip
				}
				gemSlot.Gem = &gem
				equipmentFinal.GemSlot = gemSlot
			}
		}
	}

	// Case 2: Special Post-2026 Sets - GemID stored at index 10 (without the expanded gem object at index 12)
	if equipmentFinal.GemID > 0 {
		var gemSlot Models.GemSlot
		if len(equipmentDataArray) >= 12 {
			gemSlot.SlotNumber, _ = equipmentDataArray[11].(float64)
		}

		gem := Models.Gem{
			ID: equipmentFinal.GemID,
		}
		gemSlot.Gem = &gem
		equipmentFinal.GemSlot = gemSlot
	}

	// Case 3: Standard Relic/Hero (Rarity 5/15) - Expanded Data Structure (Index 12 = GemSlotRaw)
	// Applies to both Commander (Type 2) and Castellan (Type 1) if Rarity matches
	if equipmentFinal.EquipRarity == 5 || equipmentFinal.EquipRarity == 15 {
		if len(equipmentDataArray) > 12 {
			gemSlotRaw, ok := equipmentDataArray[12].([]interface{})
			if ok {
				var gemSlot Models.GemSlot
				ProcessGemSlot(gemSlotRaw, &gemSlot, equipmentFinal.EquipRarity)
				equipmentFinal.GemSlot = gemSlot
			}
		}
	}
	equipmentFinal.EquipStats = equipStatsArray
}

func ProcessStatToActual(equipStatDataArray []interface{}, equipStatFinal *Models.Stat, equipRarity float64) {
	if equipRarity != 5 && equipRarity != 15 {
		equipStatFinal.ID, _ = equipStatDataArray[0].(float64)
		valueFinal, _ := equipStatDataArray[1].([]interface{})
		valueArray := make([]float64, len(valueFinal))
		for i, v := range valueFinal {
			unroundedValue, _ := v.(float64)
			valueArray[i] = math.Round(unroundedValue*10) / 10
		}
		equipStatFinal.Value = valueArray
	}
	if equipRarity == 5 || equipRarity == 15 {
		equipStatFinal.ID, _ = equipStatDataArray[0].(float64)
		equipStatFinal.Percent, _ = equipStatDataArray[1].(float64)
		valueFinal, _ := equipStatDataArray[2].([]interface{})
		valueArray := make([]float64, len(valueFinal))
		for i, v := range valueFinal {
			unroundedValue, _ := v.(float64)
			valueArray[i] = math.Round(unroundedValue*10) / 10
		}
		equipStatFinal.Value = valueArray
	}
}

func ProcessGemSlot(gemSlotRaw []interface{}, gemSlot *Models.GemSlot, equipRarity float64) {
	gemSlot.SlotNumber, _ = gemSlotRaw[0].(float64)
	gemRawArray, ok := gemSlotRaw[3].([]interface{})
	if ok && len(gemRawArray) > 0 {
		var gemFinal Models.Gem
		ProcessGem(gemRawArray, &gemFinal, equipRarity)
		// Only set the pointer if we actually got a valid gem ID
		if gemFinal.ID > 0 {
			gemSlot.Gem = &gemFinal
		}
	}
	// If no valid gem, gemSlot.Gem remains nil
}

func ProcessGem(gemRawArray []interface{}, gemFinal *Models.Gem, equipRarity float64) {
	gemFinal.ID, _ = gemRawArray[0].(float64)
	gemFinal.GemType, _ = gemRawArray[1].(float64)
	gemFinal.PlaceHolder24, _ = gemRawArray[2].(float64)
	gemFinal.PlaceHolder25, _ = gemRawArray[3].(float64)
	gemStatsRawArray, _ := gemRawArray[4].([]interface{})
	// Initialize a slice with 0 length but pre-allocated capacity.
	gemStatsArray := make([]Models.Stat, 0, len(gemStatsRawArray))
	for _, gemStatData := range gemStatsRawArray {
		var gemStatFinal Models.Stat
		gemStatDataArray, _ := gemStatData.([]interface{})
		ProcessStatToActual(gemStatDataArray, &gemStatFinal, equipRarity)
		// Assign the result of append back to the slice.
		gemStatsArray = append(gemStatsArray, gemStatFinal)
	}
	gemFinal.GemStats = gemStatsArray
	gemFinal.GemLevel, _ = gemRawArray[5].(float64)
}
