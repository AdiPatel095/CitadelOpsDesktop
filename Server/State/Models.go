package State

import "time"

const SchemaVersion = 2

type (
	PlayerID            int64
	AllianceID          int64
	CastleID            int64
	KingdomID           int64
	BuildingInstanceID  int64
	BuildingID          int64
	ConstructionItemID  int64
	UnitID              int64
	ResourceID          int64
	CurrencyID          int64
	CommanderID         int64
	CastellanID         int64
	MovementID          int64
	EquipmentInstanceID int64
	EquipmentID         int64
	GemInstanceID       int64
	GemID               int64
)

type DefinitionRef struct {
	Collection string `json:"collection"`
	ID         int64  `json:"id"`
}

type SessionState struct {
	Status      string    `json:"status"`
	LoggedIn    bool      `json:"loggedIn"`
	SocketReady bool      `json:"socketReady"`
	BrowserID   string    `json:"browserId,omitempty"`
	BrowserName string    `json:"browserName,omitempty"`
	ServerURL   string    `json:"serverUrl,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	ChangedAt   time.Time `json:"changedAt"`
}

type PlayerState struct {
	ID          PlayerID               `json:"id"`
	Name        string                 `json:"name,omitempty"`
	AllianceID  AllianceID             `json:"allianceId,omitempty"`
	Level       int                    `json:"level,omitempty"`
	LegendLevel int                    `json:"legendLevel,omitempty"`
	Might       float64                `json:"might,omitempty"`
	Glory       float64                `json:"glory,omitempty"`
	Gallantry   float64                `json:"gallantry,omitempty"`
	Resources   map[ResourceID]float64 `json:"resources"`
	Currencies  map[CurrencyID]float64 `json:"currencies"`
}

type CastleState struct {
	ID                CastleID                                  `json:"id"`
	KingdomID         KingdomID                                 `json:"kingdomId"`
	SlotType          int                                       `json:"slotType,omitempty"`
	Name              string                                    `json:"name,omitempty"`
	X                 int                                       `json:"x"`
	Y                 int                                       `json:"y"`
	Focused           bool                                      `json:"focused"`
	Resources         map[ResourceID]ResourceBalance            `json:"resources"`
	Units             CastleUnits                               `json:"units"`
	Buildings         map[BuildingInstanceID]Building           `json:"buildings"`
	ConstructionSlots map[BuildingInstanceID][]ConstructionSlot `json:"constructionSlots"`
	Queues            map[string][]QueueItem                    `json:"queues"`
	Crafting          CraftingState                             `json:"crafting"`
}

type ResourceBalance struct {
	Amount            float64  `json:"amount"`
	ProductionPerHour *float64 `json:"productionPerHour,omitempty"`
	Capacity          *float64 `json:"capacity,omitempty"`
}

type CastleUnits struct {
	Stationed       map[UnitID]int64 `json:"stationed"`
	Traveling       map[UnitID]int64 `json:"traveling"`
	Hospital        map[UnitID]int64 `json:"hospital"`
	SpecialHospital map[UnitID]int64 `json:"specialHospital"`
	Total           map[UnitID]int64 `json:"total"`
}

type Building struct {
	InstanceID   BuildingInstanceID `json:"instanceId"`
	DefinitionID BuildingID         `json:"definitionId"`
	GridX        int                `json:"gridX,omitempty"`
	GridY        int                `json:"gridY,omitempty"`
	Rotation     int                `json:"rotation,omitempty"`
	Level        int                `json:"level,omitempty"`
}

type ConstructionSlot struct {
	DefinitionID ConstructionItemID `json:"definitionId"`
	Slot         int                `json:"slot"`
	RemainingSec *int               `json:"remainingSec,omitempty"`
	Level        int                `json:"level,omitempty"`
}

type QueueItem struct {
	Definition  DefinitionRef `json:"definition"`
	Amount      int64         `json:"amount,omitempty"`
	StartedAt   *time.Time    `json:"startedAt,omitempty"`
	CompletesAt *time.Time    `json:"completesAt,omitempty"`
}

type CraftingState struct {
	Buildings              map[BuildingInstanceID]CraftingBuilding `json:"buildings"`
	EnabledRecipeIDs       []int64                                 `json:"enabledRecipeIds"`
	EnabledRecipeGroupIDs  []int64                                 `json:"enabledRecipeGroupIds"`
	OutputBoostByQueueType map[int]float64                         `json:"outputBoostByQueueType"`
}

type CraftingBuilding struct {
	KingdomID         KingdomID           `json:"kingdomId"`
	CastleID          CastleID            `json:"castleId"`
	InstanceID        BuildingInstanceID  `json:"instanceId"`
	DefinitionID      BuildingID          `json:"definitionId"`
	QueueTypeID       int                 `json:"queueTypeId"`
	SlotCount         int                 `json:"slotCount,omitempty"`
	ActiveSlotRentals []int               `json:"activeSlotRentals"`
	QueueSlotRentals  []int               `json:"queueSlotRentals"`
	Active            []CraftingQueueItem `json:"active"`
	Queued            []CraftingQueueItem `json:"queued"`
	ObservedAt        time.Time           `json:"observedAt"`
}

type CraftingQueueItem struct {
	RecipeID     int64   `json:"recipeId"`
	BatchValue   float64 `json:"batchValue,omitempty"`
	RemainingSec *int    `json:"remainingSec,omitempty"`
	RuntimeSec   *int    `json:"runtimeSec,omitempty"`
}

type CommanderState struct {
	ID              CommanderID                    `json:"id"`
	Name            string                         `json:"name,omitempty"`
	VisiblePosition int                            `json:"visiblePosition,omitempty"`
	Available       bool                           `json:"available"`
	Equipment       map[string]EquipmentInstanceID `json:"equipment"`
	Gems            map[string]GemInstanceID       `json:"gems"`
}

type CastellanState struct {
	ID        CastellanID                    `json:"id"`
	CastleID  CastleID                       `json:"castleId,omitempty"`
	Name      string                         `json:"name,omitempty"`
	Equipment map[string]EquipmentInstanceID `json:"equipment"`
	Gems      map[string]GemInstanceID       `json:"gems"`
}

type MovementState struct {
	ID              MovementID       `json:"id"`
	TypeID          int              `json:"typeId,omitempty"`
	Direction       int              `json:"direction"`
	OwnerPlayerID   PlayerID         `json:"ownerPlayerId,omitempty"`
	TargetPlayerID  PlayerID         `json:"targetPlayerId,omitempty"`
	SourceCastleID  CastleID         `json:"sourceCastleId,omitempty"`
	TargetCastleID  CastleID         `json:"targetCastleId,omitempty"`
	CommanderID     *CommanderID     `json:"commanderId,omitempty"`
	KingdomID       KingdomID        `json:"kingdomId"`
	SourceX         int              `json:"sourceX,omitempty"`
	SourceY         int              `json:"sourceY,omitempty"`
	TargetX         int              `json:"targetX"`
	TargetY         int              `json:"targetY"`
	TravelSeconds   int              `json:"travelSeconds,omitempty"`
	ProgressSeconds int              `json:"progressSeconds,omitempty"`
	ArrivesAt       *time.Time       `json:"arrivesAt,omitempty"`
	ReturnsAt       *time.Time       `json:"returnsAt,omitempty"`
	Units           map[UnitID]int64 `json:"units"`
}

type EquipmentInstance struct {
	ID           EquipmentInstanceID `json:"id"`
	DefinitionID EquipmentID         `json:"definitionId"`
	Slot         int                 `json:"slot"`
	TypeID       int                 `json:"typeId,omitempty"`
	RarityID     int                 `json:"rarityId,omitempty"`
	SetID        int64               `json:"setId,omitempty"`
	Level        int                 `json:"level,omitempty"`
	WearerID     int64               `json:"wearerId,omitempty"`
	WearerKind   string              `json:"wearerKind,omitempty"`
	Effects      map[int64][]float64 `json:"effects"`
}

type GemInstance struct {
	ID           GemInstanceID       `json:"id"`
	DefinitionID GemID               `json:"definitionId"`
	Slot         int                 `json:"slot,omitempty"`
	Level        int                 `json:"level,omitempty"`
	WearerID     int64               `json:"wearerId,omitempty"`
	WearerKind   string              `json:"wearerKind,omitempty"`
	Effects      map[int64][]float64 `json:"effects"`
}

type InventoryState struct {
	ConstructionItems map[ConstructionItemID]int64              `json:"constructionItems"`
	Equipment         map[EquipmentInstanceID]EquipmentInstance `json:"equipment"`
	Gems              map[GemInstanceID]GemInstance             `json:"gems"`
	Items             map[string]map[int64]int64                `json:"items"`
}

type AllianceMember struct {
	PlayerID    PlayerID `json:"playerId"`
	Name        string   `json:"name,omitempty"`
	RankID      int      `json:"rankId,omitempty"`
	Level       int      `json:"level,omitempty"`
	LegendLevel int      `json:"legendLevel,omitempty"`
	Might       float64  `json:"might,omitempty"`
}

type AllianceState struct {
	ID      AllianceID       `json:"id"`
	Name    string           `json:"name,omitempty"`
	Members []AllianceMember `json:"members"`
}

type MapObservation struct {
	KingdomID  KingdomID `json:"kingdomId"`
	X          int       `json:"x"`
	Y          int       `json:"y"`
	TypeID     int       `json:"typeId"`
	Name       string    `json:"name,omitempty"`
	Level      int       `json:"level,omitempty"`
	OwnerID    PlayerID  `json:"ownerId,omitempty"`
	ObjectID   int64     `json:"objectId,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

type ProtocolObservation struct {
	Opcode        string    `json:"opcode"`
	Count         uint64    `json:"count"`
	InboundCount  uint64    `json:"inboundCount"`
	OutboundCount uint64    `json:"outboundCount"`
	LastDirection string    `json:"lastDirection"`
	LastCode      *int      `json:"lastCode,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	LastRevision  uint64    `json:"lastRevision"`
}

type GameState struct {
	SchemaVersion   int                                     `json:"schemaVersion"`
	Revision        uint64                                  `json:"revision"`
	UpdatedAt       time.Time                               `json:"updatedAt"`
	CatalogVersion  string                                  `json:"catalogVersion,omitempty"`
	LanguageVersion string                                  `json:"languageVersion,omitempty"`
	Session         SessionState                            `json:"session"`
	Player          PlayerState                             `json:"player"`
	Castles         map[CastleID]CastleState                `json:"castles"`
	Commanders      map[CommanderID]CommanderState          `json:"commanders"`
	Castellans      map[CastellanID]CastellanState          `json:"castellans"`
	Movements       map[MovementID]MovementState            `json:"movements"`
	Inventory       InventoryState                          `json:"inventory"`
	Alliance        AllianceState                           `json:"alliance"`
	Map             map[KingdomID]map[string]MapObservation `json:"map"`
	Observations    map[string]ProtocolObservation          `json:"observations"`
}

func NewGameState() GameState {
	now := time.Now().UTC()
	return GameState{
		SchemaVersion: SchemaVersion,
		UpdatedAt:     now,
		Session:       SessionState{Status: "stopped", Namespace: "EmpireEx_21", ChangedAt: now},
		Player: PlayerState{
			Resources: map[ResourceID]float64{}, Currencies: map[CurrencyID]float64{},
		},
		Castles:    map[CastleID]CastleState{},
		Commanders: map[CommanderID]CommanderState{},
		Castellans: map[CastellanID]CastellanState{},
		Movements:  map[MovementID]MovementState{},
		Inventory: InventoryState{
			ConstructionItems: map[ConstructionItemID]int64{},
			Equipment:         map[EquipmentInstanceID]EquipmentInstance{},
			Gems:              map[GemInstanceID]GemInstance{},
			Items:             map[string]map[int64]int64{},
		},
		Alliance:     AllianceState{Members: []AllianceMember{}},
		Map:          map[KingdomID]map[string]MapObservation{},
		Observations: map[string]ProtocolObservation{},
	}
}
