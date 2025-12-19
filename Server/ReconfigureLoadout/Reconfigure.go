package ReconfigureLoadout

import (
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/Models"
	"time"
)

// ReconfigureCommander is the entry point for Commander loadout optimization
// It filters equipment, heroes, and gems for Commander type before calling the shared optimizer
func ReconfigureCommander(payload ReconfigurePayload) Models.CommStatModel {
	// 1. Refresh game state
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	// 2. Filter equipment for Commander (EquipType == 2)
	var equipment []Models.EquipmentModel
	var heroes []Models.EquipmentModel

	for _, item := range Models.EquipmentStorage {
		if item.EquipType == 2 {
			// Slot 6 = Hero, everything else = equipment
			if item.EquipSlotNumber == 6 {
				heroes = append(heroes, item)
			} else {
				equipment = append(equipment, item)
			}
		}
	}

	// 3. Filter gems for Commander
	// Commander gems: Type 32 = Relic 1.0 Commander, Type 132 = Relic 2.0 Commander
	var gems []Models.Gem
	for _, gem := range Models.GemsStorage {
		if gem.GemType == 32 || gem.GemType == 132 {
			gems = append(gems, gem)
		}
	}

	// 4. Also extract embedded gems from equipment
	for _, item := range Models.EquipmentStorage {
		if item.EquipType == 2 && item.GemSlot.Gem != nil {
			gems = append(gems, *item.GemSlot.Gem)
		}
	}

	// 5. Create optimizer input
	input := OptimizerInput{
		Equipment: equipment,
		Heroes:    heroes,
		Gems:      gems,
		Payload:   payload,
	}

	// 6. Run shared optimizer
	result := Optimize(input)

	// 7. Convert result to CommStatModel
	return BuildCommStatModel(result, equipment, heroes, gems)
}

// ReconfigureCastellan is the entry point for Castellan loadout optimization
// It filters equipment, heroes, and gems for Castellan type before calling the shared optimizer
func ReconfigureCastellan(payload ReconfigurePayload) Models.CastStatModel {
	// 1. Refresh game state
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	// 2. Filter equipment for Castellan (EquipType == 1)
	var equipment []Models.EquipmentModel
	var heroes []Models.EquipmentModel

	for _, item := range Models.EquipmentStorage {
		if item.EquipType == 1 {
			// Slot 6 = Hero, everything else = equipment
			if item.EquipSlotNumber == 6 {
				heroes = append(heroes, item)
			} else {
				equipment = append(equipment, item)
			}
		}
	}

	// 3. Filter gems for Castellan
	// Castellan gems: Type 31 = Relic 1.0 Castellan, Type 131 = Relic 2.0 Castellan
	var gems []Models.Gem
	for _, gem := range Models.GemsStorage {
		if gem.GemType == 31 || gem.GemType == 131 {
			gems = append(gems, gem)
		}
	}

	// 4. Also extract embedded gems from equipment
	for _, item := range Models.EquipmentStorage {
		if item.EquipType == 1 && item.GemSlot.Gem != nil {
			gems = append(gems, *item.GemSlot.Gem)
		}
	}

	// 5. Create optimizer input
	input := OptimizerInput{
		Equipment: equipment,
		Heroes:    heroes,
		Gems:      gems,
		Payload:   payload,
	}

	// 6. Run shared optimizer
	result := Optimize(input)

	// 7. Convert result to CastStatModel
	return BuildCastStatModel(result, equipment, heroes, gems)
}

// BuildCommStatModel converts OptimizationResult to CommStatModel with calculated stats
func BuildCommStatModel(result OptimizationResult, equipment []Models.EquipmentModel, heroes []Models.EquipmentModel, gems []Models.Gem) Models.CommStatModel {
	model := Models.CommStatModel{
		Equip1: result.Equip1,
		Equip2: result.Equip2,
		Equip3: result.Equip3,
		Equip4: result.Equip4,
		Hero:   result.Hero,
		Gem1:   result.Gem1,
		Gem2:   result.Gem2,
		Gem3:   result.Gem3,
		Gem4:   result.Gem4,
	}

	// Retrieve actual objects
	equip1 := findEquipByID(result.Equip1, equipment)
	equip2 := findEquipByID(result.Equip2, equipment)
	equip3 := findEquipByID(result.Equip3, equipment)
	equip4 := findEquipByID(result.Equip4, equipment)
	hero := findEquipByID(result.Hero, heroes)

	gem1 := findGemByID(result.Gem1, gems)
	gem2 := findGemByID(result.Gem2, gems)
	gem3 := findGemByID(result.Gem3, gems)
	gem4 := findGemByID(result.Gem4, gems)

	// Process Equipment Stats (Bucket 1)
	equipModel := Models.CommStatModel{}
	GameParser.ProcessEquipStatComm(equip1, &equipModel, &Models.CommEquipCeiling)
	GameParser.ProcessEquipStatComm(equip2, &equipModel, &Models.CommEquipCeiling)
	GameParser.ProcessEquipStatComm(equip3, &equipModel, &Models.CommEquipCeiling)
	GameParser.ProcessEquipStatComm(equip4, &equipModel, &Models.CommEquipCeiling)
	Models.ApplyCommCeiling(&equipModel, &Models.CommEquipCeiling)

	// Process Hero Stats (Bucket 2)
	heroModel := Models.CommStatModel{}
	if result.Hero > 0 {
		GameParser.ProcessEquipStatComm(hero, &heroModel, &Models.CommHeroCeiling)
		Models.ApplyCommCeiling(&heroModel, &Models.CommHeroCeiling)
	}

	// Process Gem Stats (Bucket 3)
	gemModel := Models.CommStatModel{}
	gemCeiling := Models.CommGemCeiling
	if result.Gem1 > 0 {
		GameParser.ProcessEquipStatComm(Models.EquipmentModel{EquipStats: gem1.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem2 > 0 {
		GameParser.ProcessEquipStatComm(Models.EquipmentModel{EquipStats: gem2.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem3 > 0 {
		GameParser.ProcessEquipStatComm(Models.EquipmentModel{EquipStats: gem3.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem4 > 0 {
		GameParser.ProcessEquipStatComm(Models.EquipmentModel{EquipStats: gem4.GemStats}, &gemModel, &gemCeiling)
	}
	Models.ApplyCommCeiling(&gemModel, &gemCeiling)

	// Merge Buckets
	mergeCommStats(&model, equipModel)
	mergeCommStats(&model, heroModel)
	mergeCommStats(&model, gemModel)

	return model
}

// BuildCastStatModel converts OptimizationResult to CastStatModel with calculated stats
func BuildCastStatModel(result OptimizationResult, equipment []Models.EquipmentModel, heroes []Models.EquipmentModel, gems []Models.Gem) Models.CastStatModel {
	model := Models.CastStatModel{
		Equip1: result.Equip1,
		Equip2: result.Equip2,
		Equip3: result.Equip3,
		Equip4: result.Equip4,
		Hero:   result.Hero,
		Gem1:   result.Gem1,
		Gem2:   result.Gem2,
		Gem3:   result.Gem3,
		Gem4:   result.Gem4,
	}

	// Retrieve actual objects
	equip1 := findEquipByID(result.Equip1, equipment)
	equip2 := findEquipByID(result.Equip2, equipment)
	equip3 := findEquipByID(result.Equip3, equipment)
	equip4 := findEquipByID(result.Equip4, equipment)
	hero := findEquipByID(result.Hero, heroes)

	gem1 := findGemByID(result.Gem1, gems)
	gem2 := findGemByID(result.Gem2, gems)
	gem3 := findGemByID(result.Gem3, gems)
	gem4 := findGemByID(result.Gem4, gems)

	// Process Equipment Stats (Bucket 1)
	equipModel := Models.CastStatModel{}
	GameParser.ProcessEquipStatCast(equip1, &equipModel, &Models.CastEquipCeiling)
	GameParser.ProcessEquipStatCast(equip2, &equipModel, &Models.CastEquipCeiling)
	GameParser.ProcessEquipStatCast(equip3, &equipModel, &Models.CastEquipCeiling)
	GameParser.ProcessEquipStatCast(equip4, &equipModel, &Models.CastEquipCeiling)
	Models.ApplyCastCeiling(&equipModel, &Models.CastEquipCeiling)

	// Process Hero Stats (Bucket 2)
	heroModel := Models.CastStatModel{}
	if result.Hero > 0 {
		GameParser.ProcessEquipStatCast(hero, &heroModel, &Models.CastHeroCeiling)
		Models.ApplyCastCeiling(&heroModel, &Models.CastHeroCeiling)
	}

	// Process Gem Stats (Bucket 3)
	gemModel := Models.CastStatModel{}
	gemCeiling := Models.CastGemCeiling
	if result.Gem1 > 0 {
		GameParser.ProcessEquipStatCast(Models.EquipmentModel{EquipStats: gem1.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem2 > 0 {
		GameParser.ProcessEquipStatCast(Models.EquipmentModel{EquipStats: gem2.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem3 > 0 {
		GameParser.ProcessEquipStatCast(Models.EquipmentModel{EquipStats: gem3.GemStats}, &gemModel, &gemCeiling)
	}
	if result.Gem4 > 0 {
		GameParser.ProcessEquipStatCast(Models.EquipmentModel{EquipStats: gem4.GemStats}, &gemModel, &gemCeiling)
	}
	Models.ApplyCastCeiling(&gemModel, &gemCeiling)

	// Merge Buckets
	mergeCastStats(&model, equipModel)
	mergeCastStats(&model, heroModel)
	mergeCastStats(&model, gemModel)

	return model
}

func findGemByID(id float64, gems []Models.Gem) Models.Gem {
	for _, gem := range gems {
		if gem.ID == id {
			return gem
		}
	}
	return Models.Gem{}
}

func mergeCommStats(target *Models.CommStatModel, source Models.CommStatModel) {
	target.MeleeCbtStr += source.MeleeCbtStr
	target.RangeCbtStr += source.RangeCbtStr
	target.FrontCbtStr += source.FrontCbtStr
	target.FlankCbtStr += source.FlankCbtStr
	target.AllCbtStr += source.AllCbtStr
	target.CyCbtStr += source.CyCbtStr
	target.WallStr += source.WallStr
	target.GateStr += source.GateStr
	target.MoatStr += source.MoatStr
	target.FlankLimit += source.FlankLimit
	target.FrontLimit += source.FrontLimit
	target.MeadStr += source.MeadStr
	target.HorrorStr += source.HorrorStr
	target.EliteStr += source.EliteStr
	target.Wave += source.Wave
	target.Cooldown += source.Cooldown
	target.RelicStr += source.RelicStr
	target.BeserkerStr += source.BeserkerStr
	target.MaidenSupp += source.MaidenSupp
	target.Travel += source.Travel
	target.Loot += source.Loot
	target.NPCMelee += source.NPCMelee
	target.NPCRange += source.NPCRange
	target.NPCFront += source.NPCFront
	target.NPCFlank += source.NPCFlank
	target.NPCCy += source.NPCCy
	target.NPCWall += source.NPCWall
	target.NPCGate += source.NPCGate
	target.NPCMoat += source.NPCMoat
	target.NPCGlory += source.NPCGlory
	target.CLMelee += source.CLMelee
	target.CLRange += source.CLRange
	target.CLFront += source.CLFront
	target.CLFlank += source.CLFlank
	target.CLCy += source.CLCy
	target.CLWall += source.CLWall
	target.CLGate += source.CLGate
	target.CLMoat += source.CLMoat
	target.CLLater += source.CLLater
	target.CLFire += source.CLFire
	target.CLGlory += source.CLGlory
}

func mergeCastStats(target *Models.CastStatModel, source Models.CastStatModel) {
	target.MeleeCbtStr += source.MeleeCbtStr
	target.RangeCbtStr += source.RangeCbtStr
	target.OpCbtStr += source.OpCbtStr
	target.MainCbtStr += source.MainCbtStr
	target.CyCbtStr += source.CyCbtStr
	target.AllCbtStr += source.AllCbtStr
	target.FrontCbtStr += source.FrontCbtStr
	target.FlankCbtStr += source.FlankCbtStr
	target.WallStr += source.WallStr
	target.GateStr += source.GateStr
	target.MoatStr += source.MoatStr
	target.WallLimit += source.WallLimit
	target.ProtectorSupp += source.ProtectorSupp
	target.Loot += source.Loot
	target.Recruit += source.Recruit
	target.MeadProd += source.MeadProd
	target.Research += source.Research
	target.Hospital += source.Hospital
	target.Construction += source.Construction
	target.BaseRes += source.BaseRes
	target.KingRes += source.KingRes
	target.PO += source.PO
	target.ResTransport += source.ResTransport
	target.HoneyProd += source.HoneyProd
	target.MeadStorage += source.MeadStorage
	target.HoneyStorage += source.HoneyStorage
	target.NPCMelee += source.NPCMelee
	target.NPCRange += source.NPCRange
	target.NPCFront += source.NPCFront
	target.NPCFlank += source.NPCFlank
	target.NPCCy += source.NPCCy
	target.NPCWall += source.NPCWall
	target.NPCGate += source.NPCGate
	target.NPCMoat += source.NPCMoat
	target.NPCWallLimit += source.NPCWallLimit
	target.CLMelee += source.CLMelee
	target.CLRange += source.CLRange
	target.CLCy += source.CLCy
	target.CLWall += source.CLWall
	target.CLGate += source.CLGate
	target.CLMoat += source.CLMoat
	target.CLWallLimit += source.CLWallLimit
	target.CLFire += source.CLFire
	target.CLGlory += source.CLGlory
	target.CLEarly += source.CLEarly
}
