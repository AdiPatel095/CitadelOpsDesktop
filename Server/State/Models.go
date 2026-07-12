package State

import (
	"encoding/json"
	"strconv"
	"time"
)

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
	PackageID           int64
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
	VIP         VIPState               `json:"vip"`
}

type VIPState struct {
	Points       int64 `json:"points,omitempty"`
	Level        int   `json:"level,omitempty"`
	RemainingSec int   `json:"remainingSec,omitempty"`
	Upgrade      int   `json:"upgrade,omitempty"`
}

type CastleState struct {
	ID                  CastleID                                  `json:"id"`
	KingdomID           KingdomID                                 `json:"kingdomId"`
	SlotType            int                                       `json:"slotType,omitempty"`
	Name                string                                    `json:"name,omitempty"`
	X                   int                                       `json:"x"`
	Y                   int                                       `json:"y"`
	Focused             bool                                      `json:"focused"`
	Resources           map[ResourceID]ResourceBalance            `json:"resources"`
	Units               CastleUnits                               `json:"units"`
	Buildings           map[BuildingInstanceID]Building           `json:"buildings"`
	ConstructionSlots   map[BuildingInstanceID][]ConstructionSlot `json:"constructionSlots"`
	Production          map[int]ProductionQueue                   `json:"production"`
	QueueableProduction map[int][]DefinitionRef                   `json:"queueableProduction"`
	QueueableObservedAt time.Time                                 `json:"queueableObservedAt,omitempty"`
	Crafting            CraftingState                             `json:"crafting"`
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

type ProductionQueue struct {
	LineID     int         `json:"lineId"`
	Active     *QueueItem  `json:"active,omitempty"`
	Queued     []QueueItem `json:"queued"`
	Capacity   int         `json:"capacity"`
	ObservedAt time.Time   `json:"observedAt"`
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
	ID              MovementID             `json:"id"`
	TypeID          int                    `json:"typeId,omitempty"`
	Direction       int                    `json:"direction"`
	OwnerPlayerID   PlayerID               `json:"ownerPlayerId,omitempty"`
	TargetPlayerID  PlayerID               `json:"targetPlayerId,omitempty"`
	SourceCastleID  CastleID               `json:"sourceCastleId,omitempty"`
	TargetCastleID  CastleID               `json:"targetCastleId,omitempty"`
	CommanderID     *CommanderID           `json:"commanderId,omitempty"`
	KingdomID       KingdomID              `json:"kingdomId"`
	SourceX         int                    `json:"sourceX,omitempty"`
	SourceY         int                    `json:"sourceY,omitempty"`
	TargetX         int                    `json:"targetX"`
	TargetY         int                    `json:"targetY"`
	TravelSeconds   int                    `json:"travelSeconds,omitempty"`
	ProgressSeconds int                    `json:"progressSeconds,omitempty"`
	SpyCount        int                    `json:"spyCount,omitempty"`
	ArrivesAt       *time.Time             `json:"arrivesAt,omitempty"`
	ReturnsAt       *time.Time             `json:"returnsAt,omitempty"`
	Units           map[UnitID]int64       `json:"units"`
	MarketBarrows   int                    `json:"marketBarrows,omitempty"`
	MarketGoods     []KingdomTransportGood `json:"marketGoods,omitempty"`
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
	Effects      EquipmentEffects    `json:"effects"`
}

type EquipmentEffect struct {
	WireID       int64     `json:"wireId"`
	DefinitionID int64     `json:"definitionId"`
	RollPercent  *float64  `json:"rollPercent,omitempty"`
	Values       []float64 `json:"values"`
}

type EquipmentEffects []EquipmentEffect

func (effects *EquipmentEffects) UnmarshalJSON(raw []byte) error {
	var ordered []EquipmentEffect
	if err := json.Unmarshal(raw, &ordered); err == nil {
		*effects = ordered
		return nil
	}
	// Schema 2 snapshots written before ordered effects used an id-keyed map.
	// Accept it for migration; the next live equipment refresh restores exact
	// roll percentages and any duplicate rows that the old shape could not hold.
	var legacy map[string][]float64
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return err
	}
	ordered = make([]EquipmentEffect, 0, len(legacy))
	for rawID, values := range legacy {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			continue
		}
		ordered = append(ordered, EquipmentEffect{WireID: id, DefinitionID: id, Values: append([]float64(nil), values...)})
	}
	*effects = ordered
	return nil
}

type GemInstance struct {
	ID                  GemInstanceID       `json:"id"`
	DefinitionID        GemID               `json:"definitionId"`
	TypeID              int                 `json:"typeId,omitempty"`
	CompatibleWearerID  int                 `json:"compatibleWearerId,omitempty"`
	CombatMode          string              `json:"combatMode,omitempty"`
	SetID               int64               `json:"setId,omitempty"`
	Slot                int                 `json:"slot,omitempty"`
	Level               int                 `json:"level,omitempty"`
	EquipmentInstanceID EquipmentInstanceID `json:"equipmentInstanceId,omitempty"`
	WearerID            int64               `json:"wearerId,omitempty"`
	WearerKind          string              `json:"wearerKind,omitempty"`
	Effects             EquipmentEffects    `json:"effects"`
}

type InventoryState struct {
	ConstructionItems            map[ConstructionItemID]int64              `json:"constructionItems"`
	ConstructionItemsObservedAt  time.Time                                 `json:"constructionItemsObservedAt,omitempty"`
	ConstructionOffers           map[PackageID]int64                       `json:"constructionOffers"`
	ConstructionOffersObservedAt time.Time                                 `json:"constructionOffersObservedAt,omitempty"`
	Equipment                    map[EquipmentInstanceID]EquipmentInstance `json:"equipment"`
	Gems                         map[GemInstanceID]GemInstance             `json:"gems"`
	GemStacks                    map[GemID]int64                           `json:"gemStacks"`
	Items                        map[string]map[int64]int64                `json:"items"`
}

type SubscriptionState struct {
	TypeID         int `json:"typeId"`
	RemainingSec   int `json:"remainingSec,omitempty"`
	GracePeriodSec int `json:"gracePeriodSec,omitempty"`
}

type MarketAreaEffect struct {
	EffectID int64     `json:"effectId"`
	Values   []float64 `json:"values"`
	Source   string    `json:"source,omitempty"`
}

type MarketCastleState struct {
	CastleID         CastleID               `json:"castleId"`
	KingdomID        KingdomID              `json:"kingdomId"`
	TotalBarrows     int                    `json:"totalBarrows"`
	AvailableBarrows int                    `json:"availableBarrows"`
	Resources        map[ResourceID]float64 `json:"resources"`
	AreaEffects      []MarketAreaEffect     `json:"areaEffects"`
}

type MarketState struct {
	Castles            map[CastleID]MarketCastleState `json:"castles"`
	CaravanLevel       int                            `json:"caravanLevel,omitempty"`
	CaravanLevelLoaded bool                           `json:"caravanLevelLoaded"`
	ObservedAt         time.Time                      `json:"observedAt,omitempty"`
}

type KingdomTransportUnlock struct {
	KingdomID KingdomID `json:"kingdomId"`
	Unlocked  bool      `json:"unlocked"`
	Created   bool      `json:"created"`
	Stage     int       `json:"stage,omitempty"`
}

type KingdomTransportGood struct {
	ResourceID ResourceID `json:"resourceId"`
	Amount     float64    `json:"amount"`
}

type KingdomResourceTransport struct {
	KingdomID    KingdomID              `json:"kingdomId"`
	RemainingSec int                    `json:"remainingSec,omitempty"`
	Goods        []KingdomTransportGood `json:"goods"`
}

type KingdomTransportState struct {
	Unlocks    map[KingdomID]KingdomTransportUnlock `json:"unlocks"`
	Pending    []KingdomResourceTransport           `json:"pending"`
	ObservedAt time.Time                            `json:"observedAt,omitempty"`
}

type BeriState struct {
	AvailableTroops int64            `json:"availableTroops"`
	TroopsByUnit    map[UnitID]int64 `json:"troopsByUnit"`
	ParsedSourceID  CastleID         `json:"parsedSourceId,omitempty"`
	ObservedAt      time.Time        `json:"observedAt,omitempty"`
	ConsumedAt      time.Time        `json:"consumedAt,omitempty"`
}

type AllianceMember struct {
	PlayerID            PlayerID `json:"playerId"`
	Name                string   `json:"name,omitempty"`
	RankID              int      `json:"rankId,omitempty"`
	Level               int      `json:"level,omitempty"`
	LegendLevel         int      `json:"legendLevel,omitempty"`
	Might               float64  `json:"might,omitempty"`
	ReturnProtectionSec int      `json:"returnProtectionSec,omitempty"`
}

type AllianceHolding struct {
	CastleID  CastleID  `json:"castleId"`
	PlayerID  PlayerID  `json:"playerId"`
	KingdomID KingdomID `json:"kingdomId"`
	X         int       `json:"x"`
	Y         int       `json:"y"`
	SlotType  int       `json:"slotType"`
}

type AllianceState struct {
	ID         AllianceID        `json:"id"`
	Name       string            `json:"name,omitempty"`
	Members    []AllianceMember  `json:"members"`
	Holdings   []AllianceHolding `json:"holdings"`
	ObservedAt time.Time         `json:"observedAt,omitempty"`
}

type MovementSnapshot struct {
	Version    uint64    `json:"version"`
	ObservedAt time.Time `json:"observedAt,omitempty"`
}

type StationingOperation struct {
	ID             string           `json:"id"`
	Purpose        string           `json:"purpose"`
	SourceCastleID CastleID         `json:"sourceCastleId"`
	TargetCastleID CastleID         `json:"targetCastleId"`
	MovementID     MovementID       `json:"movementId,omitempty"`
	Units          map[UnitID]int64 `json:"units"`
	SafeAfter      *time.Time       `json:"safeAfter,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

type ScheduledOperation struct {
	ID              string          `json:"id"`
	Intent          string          `json:"intent"`
	Actor           string          `json:"actor"`
	Arguments       json.RawMessage `json:"arguments"`
	ExecuteAt       time.Time       `json:"executeAt"`
	CreatedAt       time.Time       `json:"createdAt"`
	Status          string          `json:"status"`
	LastOperationID string          `json:"lastOperationId,omitempty"`
	LastError       string          `json:"lastError,omitempty"`
}

type RiftLaunch struct {
	ID                string          `json:"id"`
	DisplayName       string          `json:"displayName,omitempty"`
	SavedAtUnix       int64           `json:"savedAtUnix"`
	Body              json.RawMessage `json:"body"`
	CommanderID       CommanderID     `json:"commanderID,omitempty"`
	SourceX           int             `json:"sourceX,omitempty"`
	SourceY           int             `json:"sourceY,omitempty"`
	TargetX           int             `json:"targetX,omitempty"`
	TargetY           int             `json:"targetY,omitempty"`
	KingdomID         KingdomID       `json:"kingdomID,omitempty"`
	AttackValid       int             `json:"attackValid,omitempty"`
	WaveCount         int             `json:"waveCount,omitempty"`
	UseTravelFeather  bool            `json:"useTravelFeather"`
	OneWayTTSeconds   int             `json:"oneWayTTSeconds,omitempty"`
	LastSuccessAtUnix int64           `json:"lastSuccessAtUnix,omitempty"`
}

type RiftState struct {
	Launches        map[string]RiftLaunch `json:"launches"`
	PendingLaunchID string                `json:"pendingLaunchId,omitempty"`
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

type ReportNotice struct {
	MessageID     int64     `json:"messageId"`
	TypeID        int       `json:"typeId"`
	BattleKey     string    `json:"battleKey,omitempty"`
	ReportID      int64     `json:"reportId,omitempty"`
	AgeSec        int64     `json:"ageSec,omitempty"`
	Status        string    `json:"status"`
	OwnedByPlayer bool      `json:"ownedByPlayer,omitempty"`
	ObservedAt    time.Time `json:"observedAt"`
}

type SpyReportCapture struct {
	MessageID  int64           `json:"messageId"`
	Payload    json.RawMessage `json:"payload"`
	CapturedAt time.Time       `json:"capturedAt"`
}

type BattleReportCapture struct {
	MessageID  int64           `json:"messageId"`
	ReportID   int64           `json:"reportId,omitempty"`
	BattleKey  string          `json:"battleKey,omitempty"`
	Summary    json.RawMessage `json:"summary,omitempty"`
	Waves      json.RawMessage `json:"waves,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	CapturedAt time.Time       `json:"capturedAt"`
}

type ReportState struct {
	Notices            map[int64]ReportNotice        `json:"notices"`
	SpyCaptures        map[int64]SpyReportCapture    `json:"spyCaptures"`
	BattleCaptures     map[int64]BattleReportCapture `json:"battleCaptures"`
	ActiveBattleReport int64                         `json:"activeBattleReport,omitempty"`
}

type CommandContextState struct {
	ProductionSessionKey int        `json:"productionSessionKey,omitempty"`
	ProductionObservedAt *time.Time `json:"productionObservedAt,omitempty"`
}

type AutomationState struct {
	ID              string             `json:"id"`
	Enabled         bool               `json:"enabled"`
	Status          string             `json:"status"`
	Detail          string             `json:"detail,omitempty"`
	NextCheckAt     *time.Time         `json:"nextCheckAt,omitempty"`
	LastRunAt       *time.Time         `json:"lastRunAt,omitempty"`
	LastOperationID string             `json:"lastOperationId,omitempty"`
	LastError       string             `json:"lastError,omitempty"`
	Metrics         map[string]float64 `json:"metrics,omitempty"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

type GameState struct {
	SchemaVersion    int                                     `json:"schemaVersion"`
	Revision         uint64                                  `json:"revision"`
	UpdatedAt        time.Time                               `json:"updatedAt"`
	CatalogVersion   string                                  `json:"catalogVersion,omitempty"`
	LanguageVersion  string                                  `json:"languageVersion,omitempty"`
	Session          SessionState                            `json:"session"`
	Player           PlayerState                             `json:"player"`
	Castles          map[CastleID]CastleState                `json:"castles"`
	Commanders       map[CommanderID]CommanderState          `json:"commanders"`
	Castellans       map[CastellanID]CastellanState          `json:"castellans"`
	Movements        map[MovementID]MovementState            `json:"movements"`
	MovementSnapshot MovementSnapshot                        `json:"movementSnapshot"`
	Stationing       map[string]StationingOperation          `json:"stationing"`
	Scheduled        map[string]ScheduledOperation           `json:"scheduled"`
	Rift             RiftState                               `json:"rift"`
	Inventory        InventoryState                          `json:"inventory"`
	Subscriptions    map[int]SubscriptionState               `json:"subscriptions"`
	Market           MarketState                             `json:"market"`
	KingdomTransport KingdomTransportState                   `json:"kingdomTransport"`
	Beri             BeriState                               `json:"beri"`
	Alliance         AllianceState                           `json:"alliance"`
	Alliances        map[AllianceID]AllianceState            `json:"alliances"`
	Map              map[KingdomID]map[string]MapObservation `json:"map"`
	CommandContext   CommandContextState                     `json:"commandContext"`
	Automations      map[string]AutomationState              `json:"automations"`
	Reports          ReportState                             `json:"reports"`
	Observations     map[string]ProtocolObservation          `json:"observations"`
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
		Stationing: map[string]StationingOperation{},
		Scheduled:  map[string]ScheduledOperation{},
		Rift:       RiftState{Launches: map[string]RiftLaunch{}},
		Inventory: InventoryState{
			ConstructionItems:  map[ConstructionItemID]int64{},
			ConstructionOffers: map[PackageID]int64{},
			Equipment:          map[EquipmentInstanceID]EquipmentInstance{},
			Gems:               map[GemInstanceID]GemInstance{},
			GemStacks:          map[GemID]int64{},
			Items:              map[string]map[int64]int64{},
		},
		Subscriptions: map[int]SubscriptionState{},
		Market:        MarketState{Castles: map[CastleID]MarketCastleState{}},
		KingdomTransport: KingdomTransportState{
			Unlocks: map[KingdomID]KingdomTransportUnlock{}, Pending: []KingdomResourceTransport{},
		},
		Beri:        BeriState{TroopsByUnit: map[UnitID]int64{}},
		Alliance:    AllianceState{Members: []AllianceMember{}, Holdings: []AllianceHolding{}},
		Alliances:   map[AllianceID]AllianceState{},
		Map:         map[KingdomID]map[string]MapObservation{},
		Automations: map[string]AutomationState{},
		Reports: ReportState{
			Notices: map[int64]ReportNotice{}, SpyCaptures: map[int64]SpyReportCapture{},
			BattleCaptures: map[int64]BattleReportCapture{},
		},
		Observations: map[string]ProtocolObservation{},
	}
}
