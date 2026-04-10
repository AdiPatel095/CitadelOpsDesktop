package gamestate

import (
	"sync"

	"CitadelDesktop/Server/Models/Alliance"
	"CitadelDesktop/Server/Models/Castle"
	"CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/Models/MapState"
	"CitadelDesktop/Server/Models/Movement"
	"CitadelDesktop/Server/Models/Resources"
)

// GameState holds all dynamic game state in a single unified struct.
// Access via GetGameState() singleton and reset with Reset() on websocket connection.
type GameState struct {
	Alliance        alliance.Alliance               `json:"alliance"`
	GlobalResources resources.PlayerGlobalResources `json:"globalResources"`
	Castle          castle.PlayerCastles            `json:"castle"`
	Equipment       equipment.PlayerEquipment       `json:"equipment"`
	Movement        movement.PlayerMovement         `json:"movement"`
	PlayerID        int                             `json:"playerId"` // Session player OID; used by auto-bird persistence and parsers
	CastleFocus     castle.CastleFocus              `json:"castleFocus"`
}

var (
	instanceGameState *GameState
	onceGameState     sync.Once
)

// GetGameState returns the singleton instance of GameState.
func GetGameState() *GameState {
	onceGameState.Do(func() {
		instanceGameState = &GameState{
			Equipment: equipment.PlayerEquipment{
				NonRelicGemIDs: make(map[float64]float64),
			},
			Movement: movement.PlayerMovement{
				BirdMovements: make(map[int][]movement.BirdMovement),
				LastSDI:       make(map[int]movement.SDIContext),
			},
		}
	})
	return instanceGameState
}

// Reset initializes all game state fields to zero/empty values.
// Call this on websocket connection to start fresh.
func (gs *GameState) Reset() {
	gs.Alliance = alliance.Alliance{}
	gs.GlobalResources = resources.PlayerGlobalResources{}
	gs.Castle = castle.PlayerCastles{}
	gs.Equipment = equipment.PlayerEquipment{NonRelicGemIDs: make(map[float64]float64)}
	gs.Movement = movement.PlayerMovement{
		BirdMovements: make(map[int][]movement.BirdMovement),
		LastSDI:       make(map[int]movement.SDIContext),
	}
	gs.CastleFocus = castle.CastleFocus{}
	mapstate.GetMapState().Reset()
}

// GetCastleByID returns a pointer to the PlayerCastleInfo with the given castle ID, or nil if not found.
func (gs *GameState) GetCastleByID(castleID int) *castle.PlayerCastleInfo {
	cID := float64(castleID)
	c := &gs.Castle
	switch {
	case c.MainCastle.Aid == cID:
		return &c.MainCastle
	case c.Outpost1.Aid == cID:
		return &c.Outpost1
	case c.Outpost2.Aid == cID:
		return &c.Outpost2
	case c.Outpost3.Aid == cID:
		return &c.Outpost3
	case c.IceCastle.Aid == cID:
		return &c.IceCastle
	case c.DesertCastle.Aid == cID:
		return &c.DesertCastle
	case c.DungeonCastle.Aid == cID:
		return &c.DungeonCastle
	case c.StormCastle.Aid == cID:
		return &c.StormCastle
	case c.Metropolis.Aid == cID:
		return &c.Metropolis
	case c.Capital.Aid == cID:
		return &c.Capital
	default:
		return nil
	}
}
