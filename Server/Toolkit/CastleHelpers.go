package Toolkit

import "CitadelDesktop/Server/Models"

type castleSlot struct {
	Key  string
	Type string
	Info *Models.PlayerCastleInfo
}

func liveCastleSlots() []castleSlot {
	castles := &Models.GetGameState().Castle
	return []castleSlot{
		{Key: "mainCastle", Type: "main", Info: &castles.MainCastle},
		{Key: "outpost1", Type: "outpost", Info: &castles.Outpost1},
		{Key: "outpost2", Type: "outpost", Info: &castles.Outpost2},
		{Key: "outpost3", Type: "outpost", Info: &castles.Outpost3},
		{Key: "iceCastle", Type: "ice", Info: &castles.IceCastle},
		{Key: "desertCastle", Type: "desert", Info: &castles.DesertCastle},
		{Key: "dungeonCastle", Type: "dungeon", Info: &castles.DungeonCastle},
		{Key: "stormCastle", Type: "storm", Info: &castles.StormCastle},
		{Key: "beriWorldCastle", Type: "beri_world", Info: &castles.BeriWorldCastle},
		{Key: "metropolis", Type: "metropolis", Info: &castles.Metropolis},
		{Key: "capital", Type: "capital", Info: &castles.Capital},
	}
}
