package GameParser

import (
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

	kingdomIDMain      = 0.0
	kingdomIDDesert    = 1.0
	kingdomIDIce       = 2.0
	kingdomIDDungeon   = 3.0
	kingdomIDStorm     = 4.0
	kingdomIDBeriWorld = 10.0

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
		time.Sleep(10 * time.Second)
		FetchAllCastleTroopsAndConsumption()
	}()
}

// parseGCL processes the Game Castle List data.
// It now returns an error instead of calling log.Fatal, preventing server crashes.
// Also extracts player castle locations for AutoBird feature.
func parseGCL(gcl map[string]interface{}) error {
	gs := Models.GetGameState()

	// Clear previous player castle locations
	gs.Alliance.PlayerCastleLocations = nil

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
					gs.IceCastle.Aid, gs.IceCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 2)
		case kingdomIDDesert:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					gs.DesertCastle.Aid, gs.DesertCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 1)
		case kingdomIDDungeon:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					gs.DungeonCastle.Aid, gs.DungeonCastle.Name = id, name
				},
			}
			parseCastles(castleArray, updaters, 3)
		case kingdomIDStorm:
			parseSingleCastle(castleArray, func(id float64, name string) {
				gs.StormCastle.Aid, gs.StormCastle.Name = id, name
			}, 4)
		case kingdomIDBeriWorld:
			parseSingleCastle(castleArray, func(id float64, name string) {
				gs.BeriWorldCastle.Aid, gs.BeriWorldCastle.Name = id, name
			}, 10)
		}
	}

	return nil
}

// GetCastleLocationName returns the string identifier for a castle given its ID
func GetCastleLocationName(castleID int) string {
	gs := Models.GetGameState()

	if int(gs.MainCastle.Aid) == castleID {
		return "mainCastle"
	}
	if int(gs.Outpost1.Aid) == castleID {
		return "outpost1"
	}
	if int(gs.Outpost2.Aid) == castleID {
		return "outpost2"
	}
	if int(gs.Outpost3.Aid) == castleID {
		return "outpost3"
	}
	if int(gs.IceCastle.Aid) == castleID {
		return "iceCastle"
	}
	if int(gs.DesertCastle.Aid) == castleID {
		return "desertCastle"
	}
	if int(gs.DungeonCastle.Aid) == castleID {
		return "dungeonCastle"
	}
	if int(gs.StormCastle.Aid) == castleID {
		return "stormCastle"
	}
	if int(gs.BeriWorldCastle.Aid) == castleID {
		return "beriWorldCastle"
	}

	return ""
}

// RecalculateCastleConsumption finds the specific castle by ID, recalculates its food/mead/beef consumption based on its troop data, and returns the castle location string so the frontend can be updated.
func RecalculateCastleConsumption(castleID int) string {
	gs := Models.GetGameState()

	castle := gs.GetCastleByID(castleID)
	if castle == nil {
		return ""
	}

	// Calculate new consumptions from the castle's troop data
	foodConsumption := 0.0
	meadConsumption := 0.0
	beefConsumption := 0.0

	for unitID, count := range castle.Troops.TroopsI {
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

	castle.Production.FoodConsumption = foodConsumption
	castle.Production.MeadConsumption = meadConsumption
	castle.Production.BeefConsumption = beefConsumption

	return GetCastleLocationName(castleID)
}

// parseMainKingdomCastles handles Kingdom 0 where castles can be Main (type 1) or Outposts (type 4)
func parseMainKingdomCastles(castleArray []interface{}) {
	gs := Models.GetGameState()
	outpostIndex := 1
	for i, castle := range castleArray {
		id, name, x, y, cType, ok := extractCastleDetails(castle, i)
		if ok {
			if cType == 1 {
				gs.MainCastle.Aid, gs.MainCastle.Name = id, name
			} else if cType == 4 {
				if outpostIndex == 1 {
					gs.Outpost1.Aid, gs.Outpost1.Name = id, name
				} else if outpostIndex == 2 {
					gs.Outpost2.Aid, gs.Outpost2.Name = id, name
				} else if outpostIndex == 3 {
					gs.Outpost3.Aid, gs.Outpost3.Name = id, name
				}
				outpostIndex++
			}

			// Store player castle location for AutoBird
			if x > 0 && y > 0 {
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
					switch castleID {
					case gs.MainCastle.Aid:
						parseCastleResources(castleMap, &gs.MainCastle.Amount, &gs.MainCastle.Production, &gs.MainCastle.Storage, castleID)
					case gs.Outpost1.Aid:
						parseCastleResources(castleMap, &gs.Outpost1.Amount, &gs.Outpost1.Production, &gs.Outpost1.Storage, castleID)
					case gs.Outpost2.Aid:
						parseCastleResources(castleMap, &gs.Outpost2.Amount, &gs.Outpost2.Production, &gs.Outpost2.Storage, castleID)
					case gs.Outpost3.Aid:
						parseCastleResources(castleMap, &gs.Outpost3.Amount, &gs.Outpost3.Production, &gs.Outpost3.Storage, castleID)
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
					if castleID == gs.IceCastle.Aid {
						parseCastleResources(castleMap, &gs.IceCastle.Amount, &gs.IceCastle.Production, &gs.IceCastle.Storage, castleID)
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
					if castleID == gs.DesertCastle.Aid {
						parseCastleResources(castleMap, &gs.DesertCastle.Amount, &gs.DesertCastle.Production, &gs.DesertCastle.Storage, castleID)
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
					if castleID == gs.DungeonCastle.Aid {
						parseCastleResources(castleMap, &gs.DungeonCastle.Amount, &gs.DungeonCastle.Production, &gs.DungeonCastle.Storage, castleID)
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
					if castleID == gs.StormCastle.Aid {
						parseCastleResources(castleMap, &gs.StormCastle.Amount, &gs.StormCastle.Production, &gs.StormCastle.Storage, castleID)
					}
				}

			}
		case 10:
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
					if castleID == gs.BeriWorldCastle.Aid {
						parseCastleResources(castleMap, &gs.BeriWorldCastle.Amount, &gs.BeriWorldCastle.Production, &gs.BeriWorldCastle.Storage, castleID)
					}
				}

			}
		}
	}
	return nil
}

// parseCastleResources is a generic function to parse resources for any castle.
// It safely extracts amounts, production, and storage data, and calculates food consumption from troops.
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
	if castle != nil {
		for unitID, count := range castle.Troops.TroopsI {
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
