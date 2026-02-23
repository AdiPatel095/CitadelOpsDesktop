package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
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
}

// parseGCL processes the Game Castle List data.
// It now returns an error instead of calling log.Fatal, preventing server crashes.
// Also extracts player castle locations for AutoBird feature.
func parseGCL(gcl map[string]interface{}) error {
	gs := Models.GetGameState()

	// Clear previous player castle locations
	gs.Alliance.PlayerCastleLocations = nil

	log.Println("[CastleParser] Starting to parse GCL data...")

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
					gs.MainCastle.Aid, gs.MainCastle.Name = id, name
				},
				func(id float64, name string) { gs.Outpost1.Aid, gs.Outpost1.Name = id, name },
				func(id float64, name string) { gs.Outpost2.Aid, gs.Outpost2.Name = id, name },
				func(id float64, name string) { gs.Outpost3.Aid, gs.Outpost3.Name = id, name },
			}
			parseCastles(castleArray, updaters, 0)
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
		}
	}

	// Save debug data to file
	saveDebugData(gcl, gs.Alliance.PlayerCastleLocations)

	return nil
}

// parseCastles iterates through a list of castles and applies an updater function for each.
// Also stores player castle locations for AutoBird feature.
func parseCastles(castleArray []interface{}, updaters []func(id float64, name string), kingdomID int) {
	gs := Models.GetGameState()
	log.Printf("[CastleParser] Processing %d castles for KingdomID %d", len(castleArray), kingdomID)
	for i, castle := range castleArray {
		if i >= len(updaters) {
			log.Printf("[CastleParser] No more updaters for castle index %d in KingdomID %d", i, kingdomID)
			break // No more updaters defined for the remaining castles
		}
		id, name, x, y, ok := extractCastleDetails(castle, i)
		log.Printf("[CastleParser] Castle %d: ID=%.0f, Name=%s, X=%d, Y=%d, OK=%v", i, id, name, x, y, ok)
		if ok {
			updaters[i](id, name)
			// Store player castle location for AutoBird
			log.Printf("[CastleParser] Checking coords: X=%d, Y=%d, condition (x > 0 && y > 0) = %v", x, y, x > 0 && y > 0)
			if x > 0 && y > 0 {
				gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, Models.PlayerCastleLocation{
					KingdomID: kingdomID,
					CastleID:  int(id),
					X:         x,
					Y:         y,
				})
				log.Printf("[CastleParser] ✓ Added castle %s (ID: %.0f) to PlayerCastleLocations for KingdomID %d", name, id, kingdomID)
			} else {
				log.Printf("[CastleParser] ✗ SKIPPED castle %s (ID: %.0f) - coords X=%d, Y=%d failed check", name, id, x, y)
			}
		}
	}
}

// parseSingleCastle handles kingdoms that are expected to have only one castle.
// Also stores player castle location for AutoBird feature.
func parseSingleCastle(castleArray []interface{}, updater func(id float64, name string), kingdomID int) {
	gs := Models.GetGameState()
	log.Printf("[CastleParser] Processing single castle for KingdomID %d", kingdomID)
	if len(castleArray) > 0 {
		id, name, x, y, ok := extractCastleDetails(castleArray[0], 0)
		log.Printf("[CastleParser] Single Castle: ID=%.0f, Name=%s, X=%d, Y=%d, OK=%v", id, name, x, y, ok)
		if ok {
			updater(id, name)
			// Store player castle location for AutoBird
			log.Printf("[CastleParser] Checking coords: X=%d, Y=%d, condition (x > 0 && y > 0) = %v", x, y, x > 0 && y > 0)
			if x > 0 && y > 0 {
				gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, Models.PlayerCastleLocation{
					KingdomID: kingdomID,
					CastleID:  int(id),
					X:         x,
					Y:         y,
				})
				log.Printf("[CastleParser] ✓ Added castle %s (ID: %.0f) to PlayerCastleLocations for KingdomID %d", name, id, kingdomID)
			} else {
				log.Printf("[CastleParser] ✗ SKIPPED castle %s (ID: %.0f) - coords X=%d, Y=%d failed check", name, id, x, y)
			}
		}
	}
}

// extractCastleDetails safely pulls the ID, Name, X, and Y from a castle data structure.
func extractCastleDetails(castleData interface{}, index int) (id float64, name string, x int, y int, ok bool) {
	castleMap, ok := castleData.(map[string]interface{})
	if !ok {
		return 0, "", 0, 0, false
	}

	details, ok := castleMap[keyCastleInfoArray].([]interface{})
	if !ok || len(details) <= castleNameIndex {
		return 0, "", 0, 0, false
	}

	id, idOk := details[castleAIDIndex].(float64)
	name, nameOk := details[castleNameIndex].(string)
	xVal, xOk := details[castleXIndex].(float64)
	yVal, yOk := details[castleYIndex].(float64)

	if !idOk || !nameOk {
		return 0, "", 0, 0, false
	}

	if xOk && yOk {
		x = int(xVal)
		y = int(yVal)
	}

	return id, name, x, y, true
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
						parseCastleResources(castleMap, &gs.MainCastle.Amount, &gs.MainCastle.Production, &gs.MainCastle.Storage)
					case gs.Outpost1.Aid:
						parseCastleResources(castleMap, &gs.Outpost1.Amount, &gs.Outpost1.Production, &gs.Outpost1.Storage)
					case gs.Outpost2.Aid:
						parseCastleResources(castleMap, &gs.Outpost2.Amount, &gs.Outpost2.Production, &gs.Outpost2.Storage)
					case gs.Outpost3.Aid:
						parseCastleResources(castleMap, &gs.Outpost3.Amount, &gs.Outpost3.Production, &gs.Outpost3.Storage)
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
						parseCastleResources(castleMap, &gs.IceCastle.Amount, &gs.IceCastle.Production, &gs.IceCastle.Storage)
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
						parseCastleResources(castleMap, &gs.DesertCastle.Amount, &gs.DesertCastle.Production, &gs.DesertCastle.Storage)
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
						parseCastleResources(castleMap, &gs.DungeonCastle.Amount, &gs.DungeonCastle.Production, &gs.DungeonCastle.Storage)
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
						parseCastleResources(castleMap, &gs.StormCastle.Amount, &gs.StormCastle.Production, &gs.StormCastle.Storage)
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

// saveDebugData saves the raw GCL data and parsed castle locations to a debug file
func saveDebugData(rawGCL map[string]interface{}, parsedLocations []Models.PlayerCastleLocation) {
	// Get user home directory for saving debug file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[CastleParser] Failed to get home directory: %v", err)
		return
	}

	debugDir := filepath.Join(homeDir, ".citadel_debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		log.Printf("[CastleParser] Failed to create debug directory: %v", err)
		return
	}

	debugData := map[string]interface{}{
		"timestamp":               time.Now().Format(time.RFC3339),
		"raw_gcl_data":            rawGCL,
		"parsed_castle_locations": parsedLocations,
		"total_castles_parsed":    len(parsedLocations),
	}

	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	if err != nil {
		log.Printf("[CastleParser] Failed to marshal debug data: %v", err)
		return
	}

	debugFile := filepath.Join(debugDir, "castle_parser_debug.json")
	if err := os.WriteFile(debugFile, jsonData, 0644); err != nil {
		log.Printf("[CastleParser] Failed to write debug file: %v", err)
		return
	}

	log.Printf("[CastleParser] Debug data saved to %s", debugFile)
	log.Printf("[CastleParser] Total castles added to PlayerCastleLocations: %d", len(parsedLocations))
	for i, loc := range parsedLocations {
		log.Printf("[CastleParser]   [%d] KingdomID=%d, CastleID=%d, X=%d, Y=%d", i, loc.KingdomID, loc.CastleID, loc.X, loc.Y)
	}
}
