package GameParser

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
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
	kingdomIDBeri    = 10.0

	// Kingdom 0 castle slot types — see MapCastleTypes.go (GCL AI[] details[0]).
	// Aliases kept for readability in GCL parsing below.
	kingdomCastleTypeMain       = CastleSlotMain
	kingdomCastleTypeOutpost    = CastleSlotOutpost
	kingdomCastleTypeMetropolis = CastleSlotMetropolis
	kingdomCastleTypeCapital    = CastleSlotCapital

	castleAIDIndex  = 3
	castleNameIndex = 10
	castleXIndex    = 1
	castleYIndex    = 2

	// KID 10 (Beri) GCL AI[] row: 4th element = Beri castle CID (fuc). Kut SCID is the main castle (KID 0).
	gclBeriDetailsCIDIndex = 3

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
	keyMC  = "MC"

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
			parseForeignKingdomCastles(castleArray, func(id float64, name string) {
				gs.Castle.IceCastle.Aid, gs.Castle.IceCastle.Name = id, name
			}, 2)
		case kingdomIDDesert:
			parseForeignKingdomCastles(castleArray, func(id float64, name string) {
				gs.Castle.DesertCastle.Aid, gs.Castle.DesertCastle.Name = id, name
			}, 1)
		case kingdomIDDungeon:
			parseForeignKingdomCastles(castleArray, func(id float64, name string) {
				gs.Castle.DungeonCastle.Aid, gs.Castle.DungeonCastle.Name = id, name
			}, 3)
		case kingdomIDStorm:
			parseForeignKingdomCastles(castleArray, func(id float64, name string) {
				gs.Castle.StormCastle.Aid, gs.Castle.StormCastle.Name = id, name
			}, 4)
		case kingdomIDBeri:
			parseBeriWorldCastlesFromGCL(castleArray)
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
	if int(c.BeriWorldCastle.Aid) == castleID {
		return "beriWorldCastle"
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

func isMetropolisCastleType(cType int) bool {
	return cType == kingdomCastleTypeMetropolis || cType == CastleSlotMetropolisLegacy
}

func isCapitalCastleType(cType int) bool {
	return cType == kingdomCastleTypeCapital || cType == CastleSlotCapitalLegacy
}

func recordGCLPlayerCastleLocation(gs *Models.GameState, kingdomID int, id float64, x, y int) {
	if x <= 0 || y <= 0 {
		return
	}
	castleID := int(id)
	applyGCLCastleMapCoords(gs, castleID, kingdomID, x, y)
	gs.UpsertPlayerCastleLocation(kingdomID, castleID, x, y)
}

func setSpecialCastleSlot(gs *Models.GameState, id float64, name string, cType int) bool {
	switch {
	case isMetropolisCastleType(cType):
		gs.Castle.Metropolis.Aid, gs.Castle.Metropolis.Name = id, name
		return true
	case isCapitalCastleType(cType):
		gs.Castle.Capital.Aid, gs.Castle.Capital.Name = id, name
		return true
	default:
		return false
	}
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
				if NotifyMainCastleAIDForAutoBeri != nil {
					go NotifyMainCastleAIDForAutoBeri(int(id))
				}
			} else if cType == kingdomCastleTypeOutpost {
				if outpostIndex == 1 {
					gs.Castle.Outpost1.Aid, gs.Castle.Outpost1.Name = id, name
				} else if outpostIndex == 2 {
					gs.Castle.Outpost2.Aid, gs.Castle.Outpost2.Name = id, name
				} else if outpostIndex == 3 {
					gs.Castle.Outpost3.Aid, gs.Castle.Outpost3.Name = id, name
				}
				outpostIndex++
			} else {
				setSpecialCastleSlot(gs, id, name, cType)
			}

			recordGCLPlayerCastleLocation(gs, 0, id, x, y)
		}
	}
}

// parseForeignKingdomCastles handles one normal kingdom castle plus optional metropolis/capital rows in the same GCL kingdom.
func parseForeignKingdomCastles(castleArray []interface{}, updater func(id float64, name string), kingdomID int) {
	gs := Models.GetGameState()
	assignedKingdomCastle := false
	var fallbackID float64
	var fallbackName string

	for i, castle := range castleArray {
		id, name, x, y, cType, ok := extractCastleDetails(castle, i)
		if !ok {
			continue
		}

		switch {
		case setSpecialCastleSlot(gs, id, name, cType):
		case cType == CastleSlotForeign:
			if !assignedKingdomCastle {
				updater(id, name)
				assignedKingdomCastle = true
			}
		default:
			if fallbackID <= 0 {
				fallbackID, fallbackName = id, name
			}
		}

		recordGCLPlayerCastleLocation(gs, kingdomID, id, x, y)
	}

	if !assignedKingdomCastle && fallbackID > 0 {
		updater(fallbackID, fallbackName)
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

// parseBeriWorldCastlesFromGCL records the Berimond castle (KID 10) from gbd **gcl**.
// Beri rows use details[3]=CID (fuc). Kut SCID is the main castle instance id (KID 0 GCL).
func parseBeriWorldCastlesFromGCL(castleArray []interface{}) {
	gs := Models.GetGameState()
	kid := castle.BeriWorldKingdomID
	if len(castleArray) == 0 {
		return
	}
	cid, name, mapX, mapY, ok := extractBeriGCLCastleDetails(castleArray[0], 0)
	if !ok || cid <= 0 {
		return
	}
	gs.Castle.BeriWorldCastle.Aid = cid
	gs.Castle.BeriWorldCastle.Name = name
	if mapX > 0 || mapY > 0 {
		gs.Castle.BeriWorldCastle.MapKingdomID = kid
		gs.Castle.BeriWorldCastle.MapX = mapX
		gs.Castle.BeriWorldCastle.MapY = mapY
		gs.UpsertPlayerCastleLocation(kid, int(cid), mapX, mapY)
	}
	if NotifyBeriCastleDiscovered != nil {
		go NotifyBeriCastleDiscovered(int(cid), mapX, mapY)
	}
}

// extractBeriGCLCastleDetails reads KID-10 GCL layout: details[3]=Beri castle CID (fuc).
func extractBeriGCLCastleDetails(castleData interface{}, index int) (cid float64, name string, x, y int, ok bool) {
	castleMap, okMap := castleData.(map[string]interface{})
	if !okMap {
		return 0, "", 0, 0, false
	}
	details, okArr := castleMap[keyCastleInfoArray].([]interface{})
	if !okArr || len(details) <= castleNameIndex || len(details) <= gclBeriDetailsCIDIndex {
		return 0, "", 0, 0, false
	}
	cidVal, cidOk := details[gclBeriDetailsCIDIndex].(float64)
	name, nameOk := details[castleNameIndex].(string)
	if !cidOk || !nameOk || cidVal <= 0 {
		return 0, "", 0, 0, false
	}
	x, y = 0, 0
	if xVal, xOk := details[castleXIndex].(float64); xOk {
		x = int(xVal)
	}
	if yVal, yOk := details[castleYIndex].(float64); yOk {
		y = int(yVal)
	}
	return cidVal, name, x, y, true
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

	kingdomArray, ok := dcl[keyKingdoms].([]interface{})
	if !ok {
		return fmt.Errorf("dcl kingdomArray type assertion failed")
	}
	for _, item := range kingdomArray {
		kingdomMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		kingdomID := 0
		if value, ok := kingdomMap[keyKingdomID].(float64); ok {
			kingdomID = int(value)
		}

		castleArray, ok := kingdomMap[keyCastleInfoArray].([]interface{})
		if !ok {
			continue
		}
		for _, castleData := range castleArray {
			parseDCLCastleResources(gs, castleData, kingdomID)
		}
	}
	return nil
}

func parseDCLCastleResources(gs *Models.GameState, castleData interface{}, kingdomID int) {
	castleMap, ok := castleData.(map[string]interface{})
	if !ok {
		return
	}
	castleID, ok := castleMap[keyCastleID].(float64)
	if !ok || castleID <= 0 {
		return
	}
	castle := gs.GetCastleByID(int(castleID))
	if castle == nil {
		return
	}

	parseUnitArray := func(key string) map[int]int {
		units := make(map[int]int)
		rows, ok := castleMap[key].([]interface{})
		if !ok {
			return units
		}
		for _, row := range rows {
			pair, ok := row.([]interface{})
			if !ok || len(pair) < 2 {
				continue
			}
			unitID, unitOK := pair[0].(float64)
			count, countOK := pair[1].(float64)
			if !unitOK || !countOK || count < 0 || !Models.IsTroop(int(unitID)) {
				continue
			}
			units[int(unitID)] += int(count)
		}
		return units
	}

	stationed := parseUnitArray("AC")
	traveling := parseUnitArray("TU")
	hospital := parseUnitArray("HI")
	specialHospital := parseUnitArray("SHI")
	mixed := make(map[int]int, len(stationed)+len(traveling))
	for unitID, count := range stationed {
		mixed[unitID] += count
	}
	for unitID, count := range traveling {
		mixed[unitID] += count
	}
	castle.Troops = Models.CastleTroopData{
		KingdomID:   kingdomID,
		X:           castle.Troops.X,
		Y:           castle.Troops.Y,
		TroopsI:     stationed,
		TroopsTU:    traveling,
		TroopsHI:    hospital,
		TroopsSHI:   specialHospital,
		TroopsMixed: mixed,
	}
	if marketBarrows, exists := castleMap[keyMC]; exists {
		castle.MarketBarrowsAvailable = jsonIntFromAny(marketBarrows)
	}
	parseCastleResources(castleMap, &castle.Amount, &castle.Production, &castle.Storage, castleID)
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
