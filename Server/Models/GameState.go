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
	MainCastle      PlayerCastleInfo
	Outpost1        PlayerCastleInfo
	Outpost2        PlayerCastleInfo
	Outpost3        PlayerCastleInfo
	IceCastle       PlayerCastleInfo
	DesertCastle    PlayerCastleInfo
	DungeonCastle   PlayerCastleInfo
	StormCastle     PlayerCastleInfo
	BeriWorldCastle PlayerCastleInfo

	// Equipment & Gems
	EquipmentStorage []EquipmentModel
	GemsStorage      []Gem
	NonRelicGemIDs   map[float64]float64

	// Commander/Castellan Data
	CommActualArray []CommActualModel
	CastActualArray []CastActualModel

	// Auto Bird Data
	PlayerID        int
	BirdMovements   map[int][]BirdMovement // CastleID -> List of active movements
	ActiveMovements []GAMMovement          // Parsed from GAM message
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
	gs.BeriWorldCastle = PlayerCastleInfo{}
	gs.EquipmentStorage = nil
	gs.GemsStorage = nil
	gs.NonRelicGemIDs = make(map[float64]float64)
	gs.CommActualArray = nil
	gs.CastActualArray = nil

	gs.BirdMovements = make(map[int][]BirdMovement)
	gs.ActiveMovements = nil

	GetMapState().Reset()
}

// GetCastleByID returns a pointer to the PlayerCastleInfo with the given castle ID, or nil if not found.
func (gs *GameState) GetCastleByID(castleID int) *PlayerCastleInfo {
	cID := float64(castleID)
	switch {
	case gs.MainCastle.Aid == cID:
		return &gs.MainCastle
	case gs.Outpost1.Aid == cID:
		return &gs.Outpost1
	case gs.Outpost2.Aid == cID:
		return &gs.Outpost2
	case gs.Outpost3.Aid == cID:
		return &gs.Outpost3
	case gs.IceCastle.Aid == cID:
		return &gs.IceCastle
	case gs.DesertCastle.Aid == cID:
		return &gs.DesertCastle
	case gs.DungeonCastle.Aid == cID:
		return &gs.DungeonCastle
	case gs.StormCastle.Aid == cID:
		return &gs.StormCastle
	case gs.BeriWorldCastle.Aid == cID:
		return &gs.BeriWorldCastle
	default:
		return nil
	}
}
