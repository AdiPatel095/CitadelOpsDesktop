package gamestate

import (
	"fmt"
	"sort"
	"strconv"

	"CitadelDesktop/Server/Data/craftingrecipes"
	alliance "CitadelDesktop/Server/Models/Alliance"
	"CitadelDesktop/Server/Models/Decoration"
)

// UpdateCastleFocusPosition sets which castle is focused and its map coordinates.
func (gs *GameState) UpdateCastleFocusPosition(castleAID, kingdomID, mapPX, mapPY int) {
	gs.CastleFocus.CastleAID = castleAID
	gs.CastleFocus.KingdomID = kingdomID
	gs.CastleFocus.MapPX = mapPX
	gs.CastleFocus.MapPY = mapPY
}

// ResolveCastleMapCoords returns map PX/PY for JAA, or stored X/Y for JCA castles that still carry coords in state.
func (gs *GameState) ResolveCastleMapCoords(castleAID, kingdomID int) (x, y int, ok bool) {
	if c := gs.GetCastleByID(castleAID); c != nil {
		if c.Troops.X != 0 || c.Troops.Y != 0 {
			return c.Troops.X, c.Troops.Y, true
		}
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleAID && loc.KingdomID == kingdomID {
			if loc.X != 0 || loc.Y != 0 {
				return loc.X, loc.Y, true
			}
		}
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleAID && (loc.X != 0 || loc.Y != 0) {
			return loc.X, loc.Y, true
		}
	}
	return 0, 0, false
}

// IsKnownPlayerCastleID is true if this castle id is one of ours (GameState castle slots or alliance location list).
func (gs *GameState) IsKnownPlayerCastleID(castleID int) bool {
	if castleID <= 0 {
		return false
	}
	if gs.GetCastleByID(castleID) != nil {
		return true
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleID {
			return true
		}
	}
	return false
}

// playerCastlesPayload lists all player castles from GCL (alliance locations) with JAA/JCA inputs — global app data, not tied to resource views.
func playerCastlesPayload(gs *GameState) []map[string]interface{} {
	locs := append([]alliance.PlayerCastleLocation(nil), gs.Alliance.PlayerCastleLocations...)
	mainAID := gs.Castle.MainCastle.Aid
	sort.SliceStable(locs, func(i, j int) bool {
		c1, c2 := locs[i], locs[j]
		if c1.KingdomID != c2.KingdomID {
			return c1.KingdomID < c2.KingdomID
		}
		if c1.KingdomID == 0 {
			if float64(c1.CastleID) == mainAID {
				return true
			}
			if float64(c2.CastleID) == mainAID {
				return false
			}
		}
		return c1.CastleID < c2.CastleID
	})
	out := make([]map[string]interface{}, 0, len(locs))
	for _, loc := range locs {
		name := ""
		if c := gs.GetCastleByID(loc.CastleID); c != nil && c.Name != "" {
			name = c.Name
		}
		if name == "" {
			name = fmt.Sprintf("Castle %d", loc.CastleID)
		}
		out = append(out, map[string]interface{}{
			"aid":       loc.CastleID,
			"kingdomID": loc.KingdomID,
			"name":      name,
			"mapX":      loc.X,
			"mapY":      loc.Y,
		})
	}
	return out
}

// CastleFocusMessagePayload builds the JSON-safe map for WebSocket castleFocus (JAA updates and getCastleFocus).
func CastleFocusMessagePayload() map[string]interface{} {
	gs := GetGameState()
	f := gs.CastleFocus
	castleName := ""
	var bgRows, bdRows interface{}
	var decorationSummary []string
	if c := gs.GetCastleByID(f.CastleAID); c != nil {
		castleName = c.Name
		bgRows = c.BGRows
		bdRows = c.BDRows
		decorationSummary = decoration.DecorationSummaryLinesForCastle(c)
	}
	var slotProductionByLid map[string]interface{}
	var craftingQueues interface{}
	if c := gs.GetCastleByID(f.CastleAID); c != nil && len(c.SlotProductionByLID) > 0 {
		slotProductionByLid = make(map[string]interface{}, len(c.SlotProductionByLID))
		for lid, q := range c.SlotProductionByLID {
			if q != nil {
				slotProductionByLid[strconv.Itoa(lid)] = q
			}
		}
	}
	if c := gs.GetCastleByID(f.CastleAID); c != nil && len(c.CraftingQueues) > 0 {
		craftingQueues = craftingrecipes.EnrichCraftingQueues(c.CraftingQueues)
	}
	return map[string]interface{}{
		"aid":                 f.CastleAID,
		"kingdomID":           f.KingdomID,
		"mapPX":               f.MapPX,
		"mapPY":               f.MapPY,
		"castleName":          castleName,
		"decorationSummary":   decorationSummary,
		"bgRows":              bgRows,
		"bdRows":              bdRows,
		"slotProductionByLid": slotProductionByLid,
		"craftingQueues":      craftingQueues,
		"catalogVersion":      decoration.DecorationCatalogVersion,
		"playerCastles":       playerCastlesPayload(gs),
	}
}

// SetCastleFocusCoords updates focus position from the best-known map coordinates in GameState.
func SetCastleFocusCoords(castleAID, kingdomID int) {
	gs := GetGameState()
	x, y, _ := gs.ResolveCastleMapCoords(castleAID, kingdomID)
	gs.UpdateCastleFocusPosition(castleAID, kingdomID, x, y)
}

// GetCastleFocusCoords returns stored focus castle id, kingdom, and map coordinates.
func GetCastleFocusCoords() (castleAID, kingdomID, mapPX, mapPY int) {
	f := GetGameState().CastleFocus
	return f.CastleAID, f.KingdomID, f.MapPX, f.MapPY
}
