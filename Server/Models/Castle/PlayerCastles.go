package castle

// NumPlayerCastleSlots is the count of fixed PlayerCastleInfo slots on GameState.Castle.
const NumPlayerCastleSlots = 10

// PlayerCastles groups all fixed-slot per-castle state (main, outposts, kingdom castles, Metropolis, Capital).
type PlayerCastles struct {
	MainCastle      PlayerCastleInfo `json:"mainCastle"`
	Outpost1        PlayerCastleInfo `json:"outpost1"`
	Outpost2        PlayerCastleInfo `json:"outpost2"`
	Outpost3        PlayerCastleInfo `json:"outpost3"`
	IceCastle       PlayerCastleInfo `json:"iceCastle"`
	DesertCastle    PlayerCastleInfo `json:"desertCastle"`
	DungeonCastle   PlayerCastleInfo `json:"dungeonCastle"`
	StormCastle     PlayerCastleInfo `json:"stormCastle"`
	BeriWorldCastle PlayerCastleInfo `json:"beriWorldCastle"` // KID 10 (Berimond); not a castellan slot (see NumPlayerCastleSlots).
	Metropolis      PlayerCastleInfo `json:"metropolis"`
	Capital         PlayerCastleInfo `json:"capital"`
}

// ClearGCLMapPositions zeros gcl-derived map fields on every slot before a fresh **gcl** parse.
func (pc *PlayerCastles) ClearGCLMapPositions() {
	clear := func(p *PlayerCastleInfo) {
		p.MapKingdomID, p.MapX, p.MapY = 0, 0, 0
	}
	clear(&pc.MainCastle)
	clear(&pc.Outpost1)
	clear(&pc.Outpost2)
	clear(&pc.Outpost3)
	clear(&pc.IceCastle)
	clear(&pc.DesertCastle)
	clear(&pc.DungeonCastle)
	clear(&pc.StormCastle)
	clear(&pc.BeriWorldCastle)
	clear(&pc.Metropolis)
	clear(&pc.Capital)
}
