package Models

import (
	"sync"
)

// GameState holds all dynamic game state in a single unified struct.
// Access via GetGameState() singleton and reset with Reset() on websocket connection.
type GameState struct {
	// Alliance
	Alliance Alliance

	// Player Resources
	GlobalResources PlayerGlobalResources

	// Castle Resources
	MainCastle    PlayerCastleInfo
	Outpost1      PlayerCastleInfo
	Outpost2      PlayerCastleInfo
	Outpost3      PlayerCastleInfo
	IceCastle     PlayerCastleInfo
	DesertCastle  PlayerCastleInfo
	DungeonCastle PlayerCastleInfo
	StormCastle   PlayerCastleInfo

	// Equipment & Gems
	EquipmentStorage []EquipmentModel
	GemsStorage      []Gem
	NonRelicGemIDs   map[float64]float64

	// Commander/Castellan Data
	CommActualArray []CommActualModel
	CastActualArray []CastActualModel

	// Feature Toggles
	AutoBirdEnabled bool

	// Auto Bird Data
	PlayerCastleTroops []CastleTroops         // Troop counts for each player castle
	BirdMovements      map[int][]BirdMovement // CastleID -> List of active movements
	ActiveMovements    []GAMMovement          // Parsed from GAM message
}

var (
	instanceGameState *GameState
	onceGameState     sync.Once
)

// GetGameState returns the singleton instance of GameState.
func GetGameState() *GameState {
	onceGameState.Do(func() {
		instanceGameState = &GameState{
			NonRelicGemIDs: make(map[float64]float64),
			BirdMovements:  make(map[int][]BirdMovement),
		}
	})
	return instanceGameState
}

// Reset initializes all game state fields to zero/empty values.
// Call this on websocket connection to start fresh.
func (gs *GameState) Reset() {
	gs.Alliance = Alliance{}
	gs.GlobalResources = PlayerGlobalResources{}
	gs.MainCastle = PlayerCastleInfo{}
	gs.Outpost1 = PlayerCastleInfo{}
	gs.Outpost2 = PlayerCastleInfo{}
	gs.Outpost3 = PlayerCastleInfo{}
	gs.IceCastle = PlayerCastleInfo{}
	gs.DesertCastle = PlayerCastleInfo{}
	gs.DungeonCastle = PlayerCastleInfo{}
	gs.StormCastle = PlayerCastleInfo{}
	gs.EquipmentStorage = nil
	gs.GemsStorage = nil
	gs.NonRelicGemIDs = make(map[float64]float64)
	gs.CommActualArray = nil
	gs.CastActualArray = nil
	gs.AutoBirdEnabled = false
	gs.PlayerCastleTroops = nil
	gs.BirdMovements = make(map[int][]BirdMovement)
	gs.ActiveMovements = nil
}
