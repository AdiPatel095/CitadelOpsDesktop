package gamestate

import (
	"fmt"
	"sort"
	"strconv"

	alliance "CitadelDesktop/Server/Models/Alliance"
	castle "CitadelDesktop/Server/Models/Castle"
	"CitadelDesktop/Server/Models/Decoration"
)

// UpdateCastleFocusPosition sets which castle is focused and its map coordinates.
func (gs *GameState) UpdateCastleFocusPosition(castleAID, kingdomID, mapPX, mapPY int) {
	gs.CastleFocus.CastleAID = castleAID
	gs.CastleFocus.KingdomID = kingdomID
	gs.CastleFocus.MapPX = mapPX
	gs.CastleFocus.MapPY = mapPY
}

// ResolveCastleMapCoords returns map PX/PY: gcl fields on the castle slot, else JAA/troop Troops coords, else alliance list.
func (gs *GameState) ResolveCastleMapCoords(castleAID, kingdomID int) (x, y int, ok bool) {
	if c := gs.GetCastleByID(castleAID); c != nil {
		if c.MapX != 0 || c.MapY != 0 {
			return c.MapX, c.MapY, true
		}
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

// UpsertPlayerCastleLocation records or updates a player-owned castle tile (e.g. Beri world KID 10 from JAA).
func (gs *GameState) UpsertPlayerCastleLocation(kingdomID, castleID, x, y int) {
	if gs == nil || castleID <= 0 {
		return
	}
	for i := range gs.Alliance.PlayerCastleLocations {
		L := &gs.Alliance.PlayerCastleLocations[i]
		if L.CastleID == castleID && L.KingdomID == kingdomID {
			if x > 0 {
				L.X = x
			}
			if y > 0 {
				L.Y = y
			}
			return
		}
	}
	gs.Alliance.PlayerCastleLocations = append(gs.Alliance.PlayerCastleLocations, alliance.PlayerCastleLocation{
		KingdomID: kingdomID,
		CastleID:  castleID,
		X:         x,
		Y:         y,
	})
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

type playerCastleSlot struct {
	c          *castle.PlayerCastleInfo
	defaultKID int
	rank       int
}

func playerCastleSlots(gs *GameState) []playerCastleSlot {
	c := &gs.Castle
	return []playerCastleSlot{
		{&c.MainCastle, 0, 0},
		{&c.Outpost1, 0, 1},
		{&c.Outpost2, 0, 2},
		{&c.Outpost3, 0, 3},
		{&c.IceCastle, 2, 4},
		{&c.DesertCastle, 1, 5},
		{&c.DungeonCastle, 3, 6},
		{&c.StormCastle, 4, 7},
		{&c.Metropolis, 0, 8},
		{&c.Capital, 0, 9},
		{&c.BeriWorldCastle, castle.BeriWorldKingdomID, 10},
	}
}

func playerCastleRank(gs *GameState, castleID int) int {
	if castleID <= 0 {
		return 1000
	}
	aid := float64(castleID)
	for _, slot := range playerCastleSlots(gs) {
		if slot.c != nil && slot.c.Aid == aid {
			return slot.rank
		}
	}
	return 1000
}

func appendKnownSlotLocations(gs *GameState, locs []alliance.PlayerCastleLocation) []alliance.PlayerCastleLocation {
	seenAid := make(map[int]bool, len(locs))
	for _, loc := range locs {
		if loc.CastleID > 0 {
			seenAid[loc.CastleID] = true
		}
	}

	for _, slot := range playerCastleSlots(gs) {
		if slot.c == nil {
			continue
		}
		aid := int(slot.c.Aid)
		if aid <= 0 || seenAid[aid] {
			continue
		}
		kid := slot.defaultKID
		if slot.c.MapKingdomID != 0 {
			kid = slot.c.MapKingdomID
		}
		x, y := slot.c.MapX, slot.c.MapY
		if x == 0 && y == 0 {
			x, y = slot.c.Troops.X, slot.c.Troops.Y
			if slot.c.Troops.KingdomID != 0 {
				kid = slot.c.Troops.KingdomID
			}
		}
		locs = append(locs, alliance.PlayerCastleLocation{
			KingdomID: kid,
			CastleID:  aid,
			X:         x,
			Y:         y,
		})
		seenAid[aid] = true
	}
	return locs
}

// playerCastlesPayload lists all known player castles with JAA/JCA inputs — global app data, not tied to resource views.
func playerCastlesPayload(gs *GameState) []map[string]interface{} {
	locs := append([]alliance.PlayerCastleLocation(nil), gs.Alliance.PlayerCastleLocations...)
	locs = appendKnownSlotLocations(gs, locs)
	sort.SliceStable(locs, func(i, j int) bool {
		c1, c2 := locs[i], locs[j]
		r1, r2 := playerCastleRank(gs, c1.CastleID), playerCastleRank(gs, c2.CastleID)
		if r1 != r2 {
			return r1 < r2
		}
		if c1.KingdomID != c2.KingdomID {
			return c1.KingdomID < c2.KingdomID
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
		craftingQueues = c.CraftingQueues
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
