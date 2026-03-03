package ReconfigureLoadout

import (
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"log"
	"time"
)

// ReconfigureCommander is the entry point for Commander loadout optimization
// It filters equipment, heroes, and gems for Commander type before calling the shared optimizer
// Returns the result and an error string (empty if successful)
func ReconfigureCommander(payload ReconfigurePayload) (Models.CommStatModel, string) {
	log.Println("[Reconfigure] Starting Commander reconfiguration")
	// 1. Refresh game state
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	// 2. Filter equipment for Commander (EquipType == 2)
	gs := Models.GetGameState()
	var equipment []Models.EquipmentModel
	var heroes []Models.EquipmentModel

	for _, item := range gs.EquipmentStorage {
		if item.EquipType == 2 {
			// Slot 6 = Hero, everything else = equipment
			if item.EquipSlotNumber == 6 {
				heroes = append(heroes, item)
			} else {
				equipment = append(equipment, item)
			}
		}
	}

	// 3. Filter gems for Commander using strict range logic
	gems := collectAndFilterGems(payload)

	// 5. Validate equipment slots - check for empty slot types
	if emptySlot := ValidateEquipmentSlots(equipment); emptySlot != "" {
		return Models.CommStatModel{}, "Cannot reconfigure: Found 0 " + emptySlot + " equipment"
	}

	// 6. Create optimizer input
	input := OptimizerInput{
		Equipment: equipment,
		Heroes:    heroes,
		Gems:      gems,
		Payload:   payload,
	}

	// 7. Run shared optimizer
	log.Printf("[Reconfigure] Running optimizer for Commander with %d equipment, %d heroes, %d gems", len(equipment), len(heroes), len(gems))
	result := Optimize(input)

	// 8. Convert result to CommStatModel
	log.Println("[Reconfigure] Commander reconfiguration optimization successful")
	return BuildCommStatModel(result, equipment, heroes, gems), ""
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

// ReconfigureCastellan is the entry point for Castellan loadout optimization
// It filters equipment, heroes, and gems for Castellan type before calling the shared optimizer
// Returns the result and an error string (empty if successful)
func ReconfigureCastellan(payload ReconfigurePayload) (Models.CastStatModel, string) {
	log.Println("[Reconfigure] Starting Castellan reconfiguration")
	// 1. Refresh game state
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	// 2. Filter equipment for Castellan (EquipType == 1)
	gs := Models.GetGameState()
	var equipment []Models.EquipmentModel
	var heroes []Models.EquipmentModel

	for _, item := range gs.EquipmentStorage {
		if item.EquipType == 1 {
			// Slot 6 = Hero, everything else = equipment
			if item.EquipSlotNumber == 6 {
				heroes = append(heroes, item)
			} else {
				equipment = append(equipment, item)
			}
		}
	}

	// 3. Filter gems for Castellan using strict range logic
	gems := collectAndFilterGems(payload)

	// 5. Validate equipment slots - check for empty slot types
	if emptySlot := ValidateEquipmentSlots(equipment); emptySlot != "" {
		return Models.CastStatModel{}, "Cannot reconfigure: Found 0 " + emptySlot + " equipment"
	}

	// 6. Create optimizer input
	input := OptimizerInput{
		Equipment: equipment,
		Heroes:    heroes,
		Gems:      gems,
		Payload:   payload,
	}

	// 7. Run shared optimizer
	log.Printf("[Reconfigure] Running optimizer for Castellan with %d equipment, %d heroes, %d gems", len(equipment), len(heroes), len(gems))
	result := Optimize(input)

	// 8. Convert result to CastStatModel
	log.Println("[Reconfigure] Castellan reconfiguration optimization successful")
	return BuildCastStatModel(result, equipment, heroes, gems), ""
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
	// Manually merging since we removed the helper, or we can inline it
	// Actually, let's just sum them directly
	// Or even better, just use the `+` operator if Models allows, but they are structs.
	// For now, I will manually sum the fields in the replacement or re-add the helper.
	// Re-adding the helper in the SAME file locally (Reconfigure.go).

	mergeCastStats(&model, equipModel)
	mergeCastStats(&model, heroModel)
	mergeCastStats(&model, gemModel)

	return model
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

// ValidateEquipmentSlots checks if any equipment slot type has 0 items
// Returns the slot type name if empty, or empty string if all slots have items
// Slot 1 = Armor, Slot 2 = Weapon, Slot 3 = Helmet, Slot 4 = Artifact
func ValidateEquipmentSlots(equipment []Models.EquipmentModel) string {
	slotCounts := make(map[int]int)

	for _, item := range equipment {
		slotNum := int(item.EquipSlotNumber)
		if slotNum >= 1 && slotNum <= 4 {
			slotCounts[slotNum]++
		}
	}

	slotNames := map[int]string{
		1: "Armor",
		2: "Weapon",
		3: "Helmet",
		4: "Artifact",
	}

	for slot := 1; slot <= 4; slot++ {
		if slotCounts[slot] == 0 {
			return slotNames[slot]
		}
	}

	return ""
}

// collectAndFilterGems gathers all gems from account and filters by strict ID ranges based on mode
func collectAndFilterGems(payload ReconfigurePayload) []Models.Gem {
	gs := Models.GetGameState()
	var allGems []Models.Gem

	// 1. Gather ALL gems via pointers to avoid heavy copying initially
	// From Inventory
	for _, gem := range gs.GemsStorage {
		allGems = append(allGems, gem)
	}
	// From ALL Equipment (embedded) - user requested "mega list of all gems on players account"
	for _, item := range gs.EquipmentStorage {
		if item.GemSlot.Gem != nil {
			allGems = append(allGems, *item.GemSlot.Gem)
		}
	}

	// 2. Determine Filter Range
	isCastellan := payload.EquipmentMode == "Castellan"
	isPvP := payload.CombatMode == "PVP" || payload.CombatMode == "PvP"

	var minID, maxID float64
	if isCastellan {
		if isPvP {
			minID, maxID = 10300, 10400 // Castellan PvP (103xx)
		} else {
			minID, maxID = 10200, 10300 // Castellan NPC (102xx)
		}
	} else {
		// Commander
		if isPvP {
			minID, maxID = 300, 400 // Commander PvP (3xx)
		} else {
			minID, maxID = 200, 300 // Commander NPC (2xx)
		}
	}

	// 3. Filter
	var filtered []Models.Gem
	for _, gem := range allGems {
		if len(gem.GemStats) == 0 {
			continue
		}
		// "The way to filter this will be to just take the gem and look for the statID of its first stat"
		firstStatID := gem.GemStats[0].ID
		if firstStatID >= minID && firstStatID < maxID {
			filtered = append(filtered, gem)
		}
	}

	return filtered
}
