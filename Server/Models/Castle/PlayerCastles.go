package castle

// NumPlayerCastleSlots is the count of fixed PlayerCastleInfo slots on GameState.Castle.
const NumPlayerCastleSlots = 10

// PlayerCastles groups all fixed-slot per-castle state (main, outposts, kingdom castles, Metropolis, Capital).
type PlayerCastles struct {
	MainCastle    PlayerCastleInfo `json:"mainCastle"`
	Outpost1      PlayerCastleInfo `json:"outpost1"`
	Outpost2      PlayerCastleInfo `json:"outpost2"`
	Outpost3      PlayerCastleInfo `json:"outpost3"`
	IceCastle     PlayerCastleInfo `json:"iceCastle"`
	DesertCastle  PlayerCastleInfo `json:"desertCastle"`
	DungeonCastle PlayerCastleInfo `json:"dungeonCastle"`
	StormCastle   PlayerCastleInfo `json:"stormCastle"`
	Metropolis    PlayerCastleInfo `json:"metropolis"`
	Capital       PlayerCastleInfo `json:"capital"`
}
