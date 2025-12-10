package GameParser

import (
	"CitadelDesktop/Server/Models"
	"fmt"
)

// Constants for magic strings and indices to improve readability and maintainability.
const (
	keyGCLData         = "gcl"
	keyKingdoms        = "C"
	keyKingdomID       = "KID"
	keyCastleInfoArray = "AI"

	kingdomIDMain    = 0.0
	kingdomIDIce     = 1.0
	kingdomIDDesert  = 2.0
	kingdomIDDungeon = 3.0
	kingdomIDStorm   = 4.0

	castleAIDIndex  = 3
	castleNameIndex = 10

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
}

// parseGCL processes the Game Castle List data.
// It now returns an error instead of calling log.Fatal, preventing server crashes.
func parseGCL(gcl map[string]interface{}) error {
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
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					Models.MainCastleResources.Aid, Models.MainCastleResources.Name = id, name
				},
				func(id float64, name string) { Models.Outpost1Resources.Aid, Models.Outpost1Resources.Name = id, name },
				func(id float64, name string) { Models.Outpost2Resources.Aid, Models.Outpost2Resources.Name = id, name },
				func(id float64, name string) { Models.Outpost3Resources.Aid, Models.Outpost3Resources.Name = id, name },
			}
			parseCastles(castleArray, updaters)
		case kingdomIDIce:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					Models.IceCastleResources.Aid, Models.IceCastleResources.Name = id, name
				},
			}
			parseCastles(castleArray, updaters)
		case kingdomIDDesert:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					Models.DesertCastleResources.Aid, Models.DesertCastleResources.Name = id, name
				},
			}
			parseCastles(castleArray, updaters)
		case kingdomIDDungeon:
			updaters := []func(id float64, name string){
				func(id float64, name string) {
					Models.DungeonCastleResources.Aid, Models.DungeonCastleResources.Name = id, name
				},
			}
			parseCastles(castleArray, updaters)
		case kingdomIDStorm:
			parseSingleCastle(castleArray, func(id float64, name string) {
				Models.StormCastleResources.Aid, Models.StormCastleResources.Name = id, name
			})
		}
	}
	return nil
}

// parseCastles iterates through a list of castles and applies an updater function for each.
func parseCastles(castleArray []interface{}, updaters []func(id float64, name string)) {
	for i, castle := range castleArray {
		if i >= len(updaters) {
			break // No more updaters defined for the remaining castles
		}
		id, name, ok := extractCastleDetails(castle, i)
		if ok {
			updaters[i](id, name)
		}
	}
}

// parseSingleCastle handles kingdoms that are expected to have only one castle.
func parseSingleCastle(castleArray []interface{}, updater func(id float64, name string)) {
	if len(castleArray) > 0 {
		id, name, ok := extractCastleDetails(castleArray[0], 0)
		if ok {
			updater(id, name)
		}
	}
}

// extractCastleDetails safely pulls the ID and Name from a castle data structure.
func extractCastleDetails(castleData interface{}, index int) (id float64, name string, ok bool) {
	castleMap, ok := castleData.(map[string]interface{})
	if !ok {
		return 0, "", false
	}

	details, ok := castleMap[keyCastleInfoArray].([]interface{})
	if !ok || len(details) <= castleNameIndex {
		return 0, "", false
	}

	id, idOk := details[castleAIDIndex].(float64)
	name, nameOk := details[castleNameIndex].(string)

	if !idOk || !nameOk {
		return 0, "", false
	}

	return id, name, true
}

func parseDCL(dcl map[string]interface{}) error {
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
					case Models.MainCastleResources.Aid:
						parseCastleResources(castleMap, &Models.MainCastleResources.Amount, &Models.MainCastleResources.Production, &Models.MainCastleResources.Storage)
					case Models.Outpost1Resources.Aid:
						parseCastleResources(castleMap, &Models.Outpost1Resources.Amount, &Models.Outpost1Resources.Production, &Models.Outpost1Resources.Storage)
					case Models.Outpost2Resources.Aid:
						parseCastleResources(castleMap, &Models.Outpost2Resources.Amount, &Models.Outpost2Resources.Production, &Models.Outpost2Resources.Storage)
					case Models.Outpost3Resources.Aid:
						parseCastleResources(castleMap, &Models.Outpost3Resources.Amount, &Models.Outpost3Resources.Production, &Models.Outpost3Resources.Storage)
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
					if castleID == Models.IceCastleResources.Aid {
						parseCastleResources(castleMap, &Models.IceCastleResources.Amount, &Models.IceCastleResources.Production, &Models.IceCastleResources.Storage)
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
					if castleID == Models.DesertCastleResources.Aid {
						parseCastleResources(castleMap, &Models.DesertCastleResources.Amount, &Models.DesertCastleResources.Production, &Models.DesertCastleResources.Storage)
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
					if castleID == Models.DungeonCastleResources.Aid {
						parseCastleResources(castleMap, &Models.DungeonCastleResources.Amount, &Models.DungeonCastleResources.Production, &Models.DungeonCastleResources.Storage)
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
					if castleID == Models.StormCastleResources.Aid {
						parseCastleResources(castleMap, &Models.StormCastleResources.Amount, &Models.StormCastleResources.Production, &Models.StormCastleResources.Storage)
					}
				}

			}
		}
	}
	return nil
}

// parseCastleResources is a generic function to parse resources for any castle.
// It safely extracts amounts, production, and storage data.
func parseCastleResources(castleMap map[string]interface{}, amount *Models.CastleResourcesAmount, production *Models.CastleProductionTotal, storage *Models.CastleStorageMax) {
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
