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
	PlayerName      string                          `json:"playerName,omitempty"`
	VIP             VipState                        `json:"vip,omitempty"`
	Subscriptions   SubscriptionState               `json:"subscriptions,omitempty"`
	CastleFocus     castle.CastleFocus              `json:"castleFocus"`
	// Tci is ephemeral (SIN, gbc PL) for AutoTCI buy/equip; not a substitute for gca.CI.
	Tci TciSession `json:"tciSession,omitempty"`
	// AutoBeriWorld holds ephemeral **fuc** parse results for the Beri troop-transfer loop.
	AutoBeriWorld AutoBeriWorldSession `json:"autoBeriWorldSession,omitempty"`
	// AutoRecruit holds the resolved active unit per castle for the Auto Recruit queue loop.
	AutoRecruit AutoRecruitSession `json:"autoRecruitSession,omitempty"`
	// AutoTool holds the resolved active tool per castle for the Auto Tool queue loop.
	AutoTool AutoToolSession `json:"autoToolSession,omitempty"`
	// KingdomTransport is the latest **kpi** unlock and pending resource-transfer snapshot.
	KingdomTransport KingdomTransportState `json:"kingdomTransport,omitempty"`
	// MarketTransport holds cmi barrow state, the permanent caravan level, and sends started this session.
	MarketTransport MarketTransportState `json:"marketTransport,omitempty"`
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
				NonRelicGemIDs: make(map[int]float64),
			},
			Movement: movement.NewPlayerMovement(),
			Tci: TciSession{
				SINItemCounts: make(map[int]int),
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
	gs.PlayerID = 0
	gs.PlayerName = ""
	gs.Equipment = equipment.PlayerEquipment{NonRelicGemIDs: make(map[int]float64)}
	gs.Movement.Reset()
	gs.VIP = VipState{}
	gs.Subscriptions = SubscriptionState{}
	gs.CastleFocus = castle.CastleFocus{}
	gs.Tci = TciSession{
		SINItemCounts: make(map[int]int),
	}
	gs.AutoBeriWorld = AutoBeriWorldSession{}
	gs.AutoRecruit = AutoRecruitSession{}
	gs.AutoTool = AutoToolSession{}
	gs.KingdomTransport = KingdomTransportState{}
	gs.MarketTransport = MarketTransportState{}
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
	case c.BeriWorldCastle.Aid == cID:
		return &c.BeriWorldCastle
	case c.Metropolis.Aid == cID:
		return &c.Metropolis
	case c.Capital.Aid == cID:
		return &c.Capital
	default:
		return nil
	}
}
