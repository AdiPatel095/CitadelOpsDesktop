package GameParser

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Models"
	"fmt"
	"time"
)

// Constants for magic strings and indices to improve readability and maintainability.
const (
	keyGCLData         = "gcl"
	keyKingdoms        = "C"
	keyKingdomID       = "KID"
	keyCastleInfoArray = "AI"

	kingdomIDMain    = 0.0
	kingdomIDDesert  = 1.0
	kingdomIDIce     = 2.0
	kingdomIDDungeon = 3.0
	kingdomIDStorm   = 4.0

	// Kingdom 0 castle types from GCL details[0] (verify against live packets if Metropolis/Capital do not populate).
	kingdomCastleTypeMain       = 1
	kingdomCastleTypeOutpost    = 4
	kingdomCastleTypeMetropolis = 5
	kingdomCastleTypeCapital    = 6

	castleAIDIndex  = 3
	castleNameIndex = 10
	castleXIndex    = 1
	castleYIndex    = 2

	// Constants for resource keys in DCL data
	keyGPA = "gpa"
	keyW   = "W"
	keyS   = "S"
	keyF   = "F"
	keyC   = "C"
	keyO   = "O"
	keyG   = "G"
	keyI   = "I"
	keyH   = "HONEY"
	keyM   = "MEAD"
	keyB   = "BEEF"

	// Prefixes for production and storage keys
	prefixProd    = "D"
	prefixStorage = "MR"

	// Suffix for castle ID in DCL data
	keyCastleID = "AID"
)

func CastleDetailParser(gcl map[string]interface{}, dcl map[string]interface{}) {
	parseGCL(gcl)
	if dcl != nil {
		parseDCL(dcl)
	}

	// Fetch troops for all castles after GCL/DCL parsing completes, with a 10s delay to allow the game to load.
	go func() {
		time.Sleep(25 * time.Millisecond)
		FetchAllCastleTroopsAndConsumption()
	}()
}

// parseGCL processes the Game Castle List data.
// It now returns an error instead of calling log.Fatal, preventing server crashes.
// Also extracts player castle locations used for focusing/troop fetch.
func parseGCL(gcl map[string]interface{}) error {
	gs := Models.GetGameState()

	// Clear previous player castle locations and gcl map coords on fixed slots
	gs.Alliance.PlayerCastleLocations = nil
	gs.Castle.ClearGCLMapPositions()

	kingdomArray, ok := gcl[keyKingdoms].([]interface{})
	if !ok {
		return fmt.Errorf("kingdomArray type assertion failed")
	}

	for _, item := range kingdomArray {
		kingdomMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		kingdomID, ok := kingdomMap[keyKingdomID].(float64)
		if !ok {
			continue
		}

		castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
		if !ok {
			continue
		}

		switch kingdomID {
		case kingdomIDMain:
			parseMainKingdomCastles(castleArray)
		case kingdomIDIce:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					gs.Castle.IceCastle.Aid, gs.Castle.IceCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 2)
		case kingdomIDDesert:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					gs.Castle.DesertCastle.Aid, gs.Castle.DesertCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 1)
		case kingdomIDDungeon:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					gs.Castle.DungeonCastle.Aid, gs.Castle.DungeonCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 3)
		case kingdomIDStorm:
			parseSingleCastle(castleArray, func(id float64, name string) {
				gs.Castle.StormCastle.Aid, gs.Castle.StormCastle.Name = id, name
			}, 4)
		}
	}

	return nil
}

// GetCastleLocationName returns the string identifier for a castle given its ID
func GetCastleLocationName(castleID int) string {
	gs := Models.GetGameState()

	c := &gs.Castle
	if int(c.MainCastle.Aid) == castleID {
		return "mainCastle"
	}
	if int(c.Outpost1.Aid) == castleID {
		return "outpost1"
	}
	if int(c.Outpost2.Aid) == castleID {
		return "outpost2"
	}
	if int(c.Outpost3.Aid) == castleID {
		return "outpost3"
	}
	if int(c.IceCastle.Aid) == castleID {
		return "iceCastle"
	}
	if int(c.DesertCastle.Aid) == castleID {
		return "desertCastle"
	}
	if int(c.DungeonCastle.Aid) == castleID {
		return "dungeonCastle"
	}
	if int(c.StormCastle.Aid) == castleID {
		return "stormCastle"
	}
	if int(c.Metropolis.Aid) == castleID {
		return "metropolisCastle"
	}
	if int(c.Capital.Aid) == castleID {
		return "capitalCastle"
	}

	return ""
}

// sumTroopResourceConsumption returns hourly food/mead/beef consumption from unit counts (in castle + in transit).
func sumTroopResourceConsumption(troopsI, troopsTU map[int]int) (foodConsumption, meadConsumption, beefConsumption float64) {
	for _, m := range []map[int]int{troopsI, troopsTU} {
		for unitID, count := range m {
			if troopInfo, exists := Models.TroopIDs[unitID]; exists {
				consumption := float64(count * troopInfo.ConsumptionAmount)
				switch troopInfo.ConsumptionType {
				case "food":
					foodConsumption += consumption
				case "mead":
					meadConsumption += consumption
				case "beef":
					beefConsumption += consumption
				}
			}
		}
	}
	return
}

// RecalculateCastleConsumption finds the specific castle by ID, recalculates its food/mead/beef consumption based on its troop data, and returns the castle location string so the frontend can be updated.
func RecalculateCastleConsumption(castleID int) string {
	gs := Models.GetGameState()

	castle := gs.GetCastleByID(castleID)
	if castle == nil {
		return ""
	}

	foodConsumption, meadConsumption, beefConsumption := sumTroopResourceConsumption(
		castle.Troops.TroopsI, castle.Troops.TroopsTU,
	)

	foodConsumption, meadConsumption, beefConsumption = gamedata.ApplyConsumptionBuildingReductions(
		consumptionReductionWodIDs(castle.BGRows, castle.BDRows),
		foodConsumption, meadConsumption, beefConsumption,
	)

	castle.Production.FoodConsumption = foodConsumption
	castle.Production.MeadConsumption = meadConsumption
	castle.Production.BeefConsumption = beefConsumption

	return GetCastleLocationName(castleID)
}

// applyGCLCastleMapCoords copies GCL map position onto the matching GS.Castle slot (not Troops; those come from JAA/troop fetch).
func applyGCLCastleMapCoords(gs *Models.GameState, castleID int, kingdomID int, x, y int) {
	if x <= 0 || y <= 0 {
		return
	}
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return
	}
	c.MapKingdomID = kingdomID
	c.MapX = x
	c.MapY = y
}

// parseMainKingdomCastles handles Kingdom 0 where castles can be Main (type 1) or Outposts (type 4)
func parseMainKingdomCastles(castleArray []interface{}) {
	gs := Models.GetGameState()
	outpostIndex := 1
	for i, castle := range castleArray {
		id, name, x, y, cType, ok := extractCastleDetails(castle, i)
		if ok {
			if cType == kingdomCastleTypeMain {
				gs.Castle.MainCastle.Aid, gs.Castle.MainCastle.Name = id, name
			} else if cType == kingdomCastleTypeOutpost {
				if outpostIndex == 1 {
					gs.Castle.Outpost1.Aid, gs.Castle.Outpost1.Name = id, name
				} else if outpostIndex == 2 {
					gs.Castle.Outpost2.Aid, gs.Castle.Outpost2.Name = id, name
				} else if outpostIndex == 3 {
					gs.Castle.Outpost3.Aid, gs.Castle.Outpost3.Name = id, name
				}
				outpostIndex++
			} else if cType == kingdomCastleTypeMetropolis {
				gs.Castle.Metropolis.Aid, gs.Castle.Metropolis.Name = id, name
			} else if cType == kingdomCastleTypeCapital {
				gs.Castle.Capital.Aid, gs.Castle.Capital.Name = id, name
			}

			// Store player castle location for AutoBird
			if x > 0 && y > 0 {
				applyGCLCastleMapCoords(gs, int(id), 0, x, y)
				gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, Models.PlayerCastleLocation{
					KingdomID: 0,
					CastleID:  int(id),
					X:         x,
					Y:         y,
				})
			}
		}
	}
}

// parseCastles iterates through a list of castles and applies an updater function for each.
// Also stores player castle locations for AutoBird feature.
func parseCastles(castleArray []interface{}, updaters []func(id float64, name string), kingdomID int) {
	gs := Models.GetGameState()
	for i, castle := range castleArray {
		if i >= len(updaters) {
			break // No more updaters defined for the remaining castles
		}
		id, name, x, y, _, ok := extractCastleDetails(castle, i)
		if ok {
			updaters[i](id, name)
			// Store player castle location for AutoBird
			if x > 0 && y > 0 {
				applyGCLCastleMapCoords(gs, int(id), kingdomID, x, y)
				gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, Models.PlayerCastleLocation{
					KingdomID: kingdomID,
					CastleID:  int(id),
					X:         x,
					Y:         y,
				})
			}
		}
	}
}

// parseSingleCastle handles kingdoms that are expected to have only one castle.
// Also stores player castle location for AutoBird feature.
func parseSingleCastle(castleArray []interface{}, updater func(id float64, name string), kingdomID int) {
	gs := Models.GetGameState()
	if len(castleArray) > 0 {
		id, name, x, y, _, ok := extractCastleDetails(castleArray[0], 0)
		if ok {
			updater(id, name)
			// Store player castle location for AutoBird
			if x > 0 && y > 0 {
				applyGCLCastleMapCoords(gs, int(id), kingdomID, x, y)
				gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, Models.PlayerCastleLocation{
					KingdomID: kingdomID,
					CastleID:  int(id),
					X:         x,
					Y:         y,
				})
			}
		}
	}
}

// extractCastleDetails safely pulls the ID, Name, X, Y, and CastleType from a castle data structure.
func extractCastleDetails(castleData interface{}, index int) (id float64, name string, x int, y int, cType int, ok bool) {
	castleMap, ok := castleData.(map[string]interface{})
	if !ok {
		return 0, "", 0, 0, 0, false
	}

	details, ok := castleMap[keyCastleInfoArray].([]interface{})
	if !ok || len(details) <= castleNameIndex {
		return 0, "", 0, 0, 0, false
	}

	id, idOk := details[castleAIDIndex].(float64)
	name, nameOk := details[castleNameIndex].(string)
	xVal, xOk := details[castleXIndex].(float64)
	yVal, yOk := details[castleYIndex].(float64)
	cTypeFloat, typeOk := details[0].(float64)

	if !idOk || !nameOk {
		return 0, "", 0, 0, 0, false
	}

	cTypeInt := 0
	if typeOk {
		cTypeInt = int(cTypeFloat)
	}

	if xOk && yOk {
		x = int(xVal)
		y = int(yVal)
	}

	return id, name, x, y, cTypeInt, true
}

func parseDCL(dcl map[string]interface{}) error {
	gs := Models.GetGameState()
	// DCL data seems to be a map of castles, not a kingdom array.
	// The top-level keys are string representations of castle IDs.

	kingdomArray := dcl[keyKingdoms].([]interface{})
	for _, item := range kingdomArray {
		kingdomMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		kingdomID, ok := kingdomMap[keyKingdomID].(float64)
		if !ok {
			continue
		}
		switch kingdomID {
		case 0:
			{
				castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
				if !ok {
					continue
				}
				for _, castleData := range castleArray {
					castleMap, ok := castleData.(map[string]interface{})
					if !ok {
						continue
					}
					castleID, ok := castleMap[keyCastleID].(float64)
					c := &gs.Castle
					switch castleID {
					case c.MainCastle.Aid:
						parseCastleResources(castleMap, &c.MainCastle.Amount, &c.MainCastle.Production, &c.MainCastle.Storage, castleID)
					case c.Outpost1.Aid:
						parseCastleResources(castleMap, &c.Outpost1.Amount, &c.Outpost1.Production, &c.Outpost1.Storage, castleID)
					case c.Outpost2.Aid:
						parseCastleResources(castleMap, &c.Outpost2.Amount, &c.Outpost2.Production, &c.Outpost2.Storage, castleID)
					case c.Outpost3.Aid:
						parseCastleResources(castleMap, &c.Outpost3.Amount, &c.Outpost3.Production, &c.Outpost3.Storage, castleID)
					case c.Metropolis.Aid:
						parseCastleResources(castleMap, &c.Metropolis.Amount, &c.Metropolis.Production, &c.Metropolis.Storage, castleID)
					case c.Capital.Aid:
						parseCastleResources(castleMap, &c.Capital.Amount, &c.Capital.Production, &c.Capital.Storage, castleID)
					}
				}

			}
		case 2:
			{
				castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
				if !ok {
					continue
				}
				for _, castleData := range castleArray {
					castleMap, ok := castleData.(map[string]interface{})
					if !ok {
						continue
					}
					castleID, ok := castleMap[keyCastleID].(float64)
					if castleID == gs.Castle.IceCastle.Aid {
						parseCastleResources(castleMap, &gs.Castle.IceCastle.Amount, &gs.Castle.IceCastle.Production, &gs.Castle.IceCastle.Storage, castleID)
					}
				}

			}
		case 1:
			{
				castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
				if !ok {
					continue
				}
				for _, castleData := range castleArray {
					castleMap, ok := castleData.(map[string]interface{})
					if !ok {
						continue
					}
					castleID, ok := castleMap[keyCastleID].(float64)
					if castleID == gs.Castle.DesertCastle.Aid {
						parseCastleResources(castleMap, &gs.Castle.DesertCastle.Amount, &gs.Castle.DesertCastle.Production, &gs.Castle.DesertCastle.Storage, castleID)
					}
				}

			}
		case 3:
			{
				castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
				if !ok {
					continue
				}
				for _, castleData := range castleArray {
					castleMap, ok := castleData.(map[string]interface{})
					if !ok {
						continue
					}
					castleID, ok := castleMap[keyCastleID].(float64)
					if castleID == gs.Castle.DungeonCastle.Aid {
						parseCastleResources(castleMap, &gs.Castle.DungeonCastle.Amount, &gs.Castle.DungeonCastle.Production, &gs.Castle.DungeonCastle.Storage, castleID)
					}
				}

			}
		case 4:
			{
				castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
				if !ok {
					continue
				}
				for _, castleData := range castleArray {
					castleMap, ok := castleData.(map[string]interface{})
					if !ok {
						continue
					}
					castleID, ok := castleMap[keyCastleID].(float64)
					if castleID == gs.Castle.StormCastle.Aid {
						parseCastleResources(castleMap, &gs.Castle.StormCastle.Amount, &gs.Castle.StormCastle.Production, &gs.Castle.StormCastle.Storage, castleID)
					}
				}

			}
		}
	}
	return nil
}

// parseCastleResources is a generic function to parse resources for any castle.
// It safely extracts amounts, production, and storage data, and calculates food/mead/beef consumption from
// troops in castle (TroopsI) and in transit (TroopsTU), then applies consumption-building reductions.
func parseCastleResources(castleMap map[string]interface{}, amount *Models.CastleResourcesAmount, production *Models.CastleProductionTotal, storage *Models.CastleStorageMax, castleID float64) {
	// Helper to safely get a float64 from the map
	getFloat := func(key string) float64 {
		if val, ok := castleMap[key].(float64); ok {
			return val
		}
		return 0.0
	}

	// Parse direct resource amounts
	amount.WoodAmount = getFloat(keyW)
	amount.StoneAmount = getFloat(keyS)
	amount.FoodAmount = getFloat(keyF)
	amount.CoalAmount = getFloat(keyC)
	amount.OilAmount = getFloat(keyO)
	amount.GlassAmount = getFloat(keyG)
	amount.IronAmount = getFloat(keyI)
	amount.HoneyAmount = getFloat(keyH)
	amount.MeadAmount = getFloat(keyM)
	amount.BeefAmount = getFloat(keyB)

	// Parse production and storage from the 'gpa' sub-map
	gpaMap, ok := castleMap[keyGPA].(map[string]interface{})
	if !ok {
		return // No 'gpa' map, so can't parse production or storage.
	}

	// Helper to safely get a float64 from the gpaMap
	getGpaFloat := func(key string) float64 {
		if val, ok := gpaMap[key].(float64); ok {
			return val
		}
		return 0.0
	}

	// Parse production rates
	production.WoodProd = getGpaFloat(prefixProd+keyW) / 10
	production.StoneProd = getGpaFloat(prefixProd+keyS) / 10
	production.FoodProd = getGpaFloat(prefixProd+keyF) / 10
	production.CoalProd = getGpaFloat(prefixProd+keyC) / 10
	production.OilProd = getGpaFloat(prefixProd+keyO) / 10
	production.GlassProd = getGpaFloat(prefixProd+keyG) / 10
	production.IronProd = getGpaFloat(prefixProd+keyI) / 10
	production.HoneyProd = getGpaFloat(prefixProd+keyH) / 10
	production.MeadProd = getGpaFloat(prefixProd+keyM) / 10
	production.BeefProd = getGpaFloat(prefixProd+keyB) / 10

	// Deduct consumption from production
	gs := Models.GetGameState()
	foodConsumption := 0.0
	meadConsumption := 0.0
	beefConsumption := 0.0

	// Read troop data from the castle object itself
	castle := gs.GetCastleByID(int(castleID))
	var wodIDs []int
	if castle != nil {
		foodConsumption, meadConsumption, beefConsumption = sumTroopResourceConsumption(
			castle.Troops.TroopsI, castle.Troops.TroopsTU,
		)
		wodIDs = consumptionReductionWodIDs(castle.BGRows, castle.BDRows)
	}

	foodConsumption, meadConsumption, beefConsumption = gamedata.ApplyConsumptionBuildingReductions(
		wodIDs,
		foodConsumption, meadConsumption, beefConsumption,
	)

	production.FoodConsumption = foodConsumption
	production.MeadConsumption = meadConsumption
	production.BeefConsumption = beefConsumption

	// Parse storage maximums
	storage.WoodMax = getGpaFloat(prefixStorage + keyW)
	storage.StoneMax = getGpaFloat(prefixStorage + keyS)
	storage.FoodMax = getGpaFloat(prefixStorage + keyF)
	storage.CoalMax = getGpaFloat(prefixStorage + keyC)
	storage.OilMax = getGpaFloat(prefixStorage + keyO)
	storage.GlassMax = getGpaFloat(prefixStorage + keyG)
	storage.IronMax = getGpaFloat(prefixStorage + keyI)
	storage.HoneyMax = getGpaFloat(prefixStorage + keyH)
	storage.MeadMax = getGpaFloat(prefixStorage + keyM)
	storage.BeefMax = getGpaFloat(prefixStorage + keyB)
}

// consumptionReductionWodIDs collects JAA gca building IDs from BG/BD (same as items.json wodID) for consumption-reduction buildings.
func consumptionReductionWodIDs(bg, bd []Models.BuildingData) []int {
	n := len(bg) + len(bd)
	if n == 0 {
		return nil
	}
	out := make([]int, 0, n)
	for _, r := range bg {
		if r.BuildingID > 0 {
			out = append(out, r.BuildingID)
		}
	}
	for _, r := range bd {
		if r.BuildingID > 0 {
			out = append(out, r.BuildingID)
		}
	}
	return out
}
