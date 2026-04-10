package castle

// NumPlayerCastleSlots is the count of fixed PlayerCastleInfo slots on GameState.Castle.
const NumPlayerCastleSlots = 10

// PlayerCastles groups all fixed-slot per-castle state (main, outposts, kingdom castles, Metropolis, Capital).
type PlayerCastles struct {
	MainCastle    PlayerCastleInfo
	Outpost1      PlayerCastleInfo
	Outpost2      PlayerCastleInfo
	Outpost3      PlayerCastleInfo
	IceCastle     PlayerCastleInfo
	DesertCastle  PlayerCastleInfo
	DungeonCastle PlayerCastleInfo
	StormCastle   PlayerCastleInfo
	Metropolis    PlayerCastleInfo
	Capital       PlayerCastleInfo
}
