package State

import "time"

type CastleChange struct {
	ID      CastleID     `json:"id"`
	Castle  *CastleState `json:"castle,omitempty"`
	Patch   *CastlePatch `json:"patch,omitempty"`
	Deleted bool         `json:"deleted,omitempty"`
}

type CastlePatch struct {
	KingdomID                   *KingdomID                                 `json:"kingdomId,omitempty"`
	SlotType                    *int                                       `json:"slotType,omitempty"`
	Name                        *string                                    `json:"name,omitempty"`
	X                           *int                                       `json:"x,omitempty"`
	Y                           *int                                       `json:"y,omitempty"`
	Focused                     *bool                                      `json:"focused,omitempty"`
	ContextSnapshotObservedAt   *time.Time                                 `json:"contextSnapshotObservedAt,omitempty"`
	Resources                   *map[ResourceID]ResourceBalance            `json:"resources,omitempty"`
	FoodStateObservedAt         *time.Time                                 `json:"foodStateObservedAt,omitempty"`
	Units                       *CastleUnits                               `json:"units,omitempty"`
	UnitsObservedAt             *time.Time                                 `json:"unitsObservedAt,omitempty"`
	Defense                     *CastleDefenseState                        `json:"defense,omitempty"`
	Buildings                   *map[BuildingInstanceID]Building           `json:"buildings,omitempty"`
	BuildingProduction          *map[BuildingInstanceID]BuildingProduction `json:"buildingProduction,omitempty"`
	Layout                      *CastleLayout                              `json:"layout,omitempty"`
	BuildingQueue               *BuildingConstructionQueue                 `json:"buildingQueue,omitempty"`
	ConstructionSlots           *map[BuildingInstanceID][]ConstructionSlot `json:"constructionSlots,omitempty"`
	ConstructionSlotsObservedAt *time.Time                                 `json:"constructionSlotsObservedAt,omitempty"`
	Production                  *map[int]ProductionQueue                   `json:"production,omitempty"`
	QueueableProduction         *map[int][]DefinitionRef                   `json:"queueableProduction,omitempty"`
	QueueableObservedAt         *time.Time                                 `json:"queueableObservedAt,omitempty"`
	Crafting                    *CraftingState                             `json:"crafting,omitempty"`
}

type EquipmentChange struct {
	ID        EquipmentInstanceID `json:"id"`
	Equipment *EquipmentInstance  `json:"equipment,omitempty"`
	Deleted   bool                `json:"deleted,omitempty"`
}

type GemChange struct {
	ID      GemInstanceID `json:"id"`
	Gem     *GemInstance  `json:"gem,omitempty"`
	Deleted bool          `json:"deleted,omitempty"`
}

type InventoryItemChange struct {
	Collection string           `json:"collection"`
	Items      *map[int64]int64 `json:"items,omitempty"`
	Deleted    bool             `json:"deleted,omitempty"`
}

type InventoryPatch struct {
	ConstructionItems               *map[ConstructionItemID]int64              `json:"constructionItems,omitempty"`
	ConstructionItemsObservedAt     *time.Time                                 `json:"constructionItemsObservedAt,omitempty"`
	ConstructionSpaceLeft           *int64                                     `json:"constructionSpaceLeft,omitempty"`
	ConstructionSpaceLeftObservedAt *time.Time                                 `json:"constructionSpaceLeftObservedAt,omitempty"`
	ConstructionOffers              *map[PackageID]int64                       `json:"constructionOffers,omitempty"`
	ConstructionOffersObservedAt    *time.Time                                 `json:"constructionOffersObservedAt,omitempty"`
	ConstructionOffersCastleID      *CastleID                                  `json:"constructionOffersCastleId,omitempty"`
	ConstructionOffersKingdomID     *KingdomID                                 `json:"constructionOffersKingdomId,omitempty"`
	ConstructionOffersByCastle      *map[CastleID]ConstructionOfferSnapshot    `json:"constructionOffersByCastle,omitempty"`
	Equipment                       *map[EquipmentInstanceID]EquipmentInstance `json:"equipment,omitempty"`
	EquipmentChanges                *[]EquipmentChange                         `json:"equipmentChanges,omitempty"`
	Gems                            *map[GemInstanceID]GemInstance             `json:"gems,omitempty"`
	GemChanges                      *[]GemChange                               `json:"gemChanges,omitempty"`
	GemStacks                       *map[GemID]int64                           `json:"gemStacks,omitempty"`
	Items                           *map[string]map[int64]int64                `json:"items,omitempty"`
	ItemChanges                     *[]InventoryItemChange                     `json:"itemChanges,omitempty"`
}

type EventScoreChange struct {
	EventID         int64               `json:"eventId"`
	Score           *ScalableEventScore `json:"score,omitempty"`
	ScoreDeleted    bool                `json:"scoreDeleted,omitempty"`
	Activity        *EventActivityState `json:"activity,omitempty"`
	ActivityDeleted bool                `json:"activityDeleted,omitempty"`
	Ranking         *EventRankingState  `json:"ranking,omitempty"`
	RankingDeleted  bool                `json:"rankingDeleted,omitempty"`
}

type EventScorePatch struct {
	ActiveEventID *int64                        `json:"activeEventId,omitempty"`
	Inventory     *EventInventoryState          `json:"inventory,omitempty"`
	ShopByPackage *map[PackageID]EventShopRoute `json:"shopByPackage,omitempty"`
	Changes       []EventScoreChange            `json:"changes,omitempty"`
}

type MovementChange struct {
	ID       MovementID     `json:"id"`
	Movement *MovementState `json:"movement,omitempty"`
	Deleted  bool           `json:"deleted,omitempty"`
}

type componentChanges struct {
	mapChanges         []MapChange
	replaceMap         bool
	castleIDs          []CastleID
	castleParts        map[CastleID]CastleMutationPart
	replaceCastles     bool
	inventoryParts     inventoryMutationPart
	replaceInventory   bool
	equipmentIDs       []EquipmentInstanceID
	replaceEquipment   bool
	gemIDs             []GemInstanceID
	replaceGems        bool
	itemKeys           []string
	replaceItems       bool
	stormTargetKeys    []string
	replaceStorm       bool
	towerCooldownKeys  []string
	replaceCooldowns   bool
	towerQueueCastles  []CastleID
	replaceTowerQueue  bool
	reportMessageIDs   []int64
	replaceReports     bool
	eventScoreIDs      []int64
	eventScoreMeta     bool
	eventScoreShop     bool
	replaceEventScores bool
	movementIDs        []MovementID
	replaceMovements   bool
}

// ComponentPatch is an exact projection of one committed immutable
// generation. Pointer fields distinguish an unchanged component from a
// component intentionally changed to an empty value. They point into the
// retained generation, so publication does not deep-clone state again.
type ComponentPatch struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      uint64    `json:"revision"`
	UpdatedAt     time.Time `json:"updatedAt"`

	CatalogVersion       *string                         `json:"catalogVersion,omitempty"`
	LanguageVersion      *string                         `json:"languageVersion,omitempty"`
	Session              *SessionState                   `json:"session,omitempty"`
	Account              *AccountBindingState            `json:"account,omitempty"`
	Player               *PlayerState                    `json:"player,omitempty"`
	Castles              *map[CastleID]CastleState       `json:"castles,omitempty"`
	CastleChanges        *[]CastleChange                 `json:"castleChanges,omitempty"`
	Commanders           *map[CommanderID]CommanderState `json:"commanders,omitempty"`
	Generals             *map[int64]GeneralState         `json:"generals,omitempty"`
	Castellans           *map[CastellanID]CastellanState `json:"castellans,omitempty"`
	Movements            *map[MovementID]MovementState   `json:"movements,omitempty"`
	MovementChanges      *[]MovementChange               `json:"movementChanges,omitempty"`
	MovementSnapshot     *MovementSnapshot               `json:"movementSnapshot,omitempty"`
	Stationing           *map[string]StationingOperation `json:"stationing,omitempty"`
	Scheduled            *map[string]ScheduledOperation  `json:"scheduled,omitempty"`
	Rift                 *RiftState                      `json:"rift,omitempty"`
	Inventory            *InventoryState                 `json:"inventory,omitempty"`
	InventoryChanges     *InventoryPatch                 `json:"inventoryChanges,omitempty"`
	Subscriptions        *map[int]SubscriptionState      `json:"subscriptions,omitempty"`
	Market               *MarketState                    `json:"market,omitempty"`
	KingdomTransport     *KingdomTransportState          `json:"kingdomTransport,omitempty"`
	Beri                 *BeriState                      `json:"beri,omitempty"`
	Alliance             *AllianceState                  `json:"alliance,omitempty"`
	Alliances            *map[AllianceID]AllianceState   `json:"alliances,omitempty"`
	AllianceHelpRequests *AllianceHelpRequestState       `json:"allianceHelpRequests,omitempty"`
	Map                  *WorldMap                       `json:"map,omitempty"`
	MapChanges           *[]MapChange                    `json:"mapChanges,omitempty"`
	TowerCooldowns       *map[string]TowerCooldownState  `json:"towerCooldowns,omitempty"`
	TowerQueue           *TowerQueueState                `json:"towerQueue,omitempty"`
	Invasion             *InvasionState                  `json:"invasion,omitempty"`
	Storm                *StormState                     `json:"storm,omitempty"`
	NomadCamps           *NomadCampState                 `json:"nomadCamps,omitempty"`
	Advisor              *AdvisorState                   `json:"advisor,omitempty"`
	Khan                 *KhanState                      `json:"khan,omitempty"`
	DailyAttacks         *DailyAttackState               `json:"dailyAttacks,omitempty"`
	AttackDialog         *AttackDialogState              `json:"attackDialog,omitempty"`
	CombatCooldown       *CombatCooldownState            `json:"combatCooldown,omitempty"`
	AttackPresets        *[]AttackPreset                 `json:"attackPresets,omitempty"`
	AttackAnalytics      *AttackAnalyticsState           `json:"attackAnalytics,omitempty"`
	EventScores          *EventScoreState                `json:"eventScores,omitempty"`
	EventScoreChanges    *EventScorePatch                `json:"eventScoreChanges,omitempty"`
	CommandContext       *CommandContextState            `json:"commandContext,omitempty"`
	Automations          *map[string]AutomationState     `json:"automations,omitempty"`
	Reports              *ReportState                    `json:"reports,omitempty"`
	Observations         *map[string]ProtocolObservation `json:"observations,omitempty"`
}

func componentPatch(
	generation *storeGeneration,
	components ComponentSet,
	changes componentChanges,
) *ComponentPatch {
	if generation == nil {
		return nil
	}
	state := generation.state
	patch := &ComponentPatch{
		SchemaVersion: state.SchemaVersion,
		Revision:      state.Revision,
		UpdatedAt:     state.UpdatedAt,
	}
	if components.Has(ComponentCatalog) {
		patch.CatalogVersion = &state.CatalogVersion
		patch.LanguageVersion = &state.LanguageVersion
	}
	if components.Has(ComponentSession) {
		patch.Session = &state.Session
	}
	if components.Has(ComponentAccount) {
		patch.Account = &state.Account
	}
	if components.Has(ComponentPlayer) {
		patch.Player = &state.Player
	}
	if components.Has(ComponentCastles) {
		if changes.replaceCastles || len(changes.castleIDs) == 0 {
			patch.Castles = &state.Castles
		} else {
			castleChanges := make([]CastleChange, 0, len(changes.castleIDs))
			for _, id := range changes.castleIDs {
				castle, found := state.Castles[id]
				if !found {
					castleChanges = append(castleChanges, CastleChange{ID: id, Deleted: true})
					continue
				}
				parts := changes.castleParts[id]
				if parts == 0 || parts == AllCastleMutationParts {
					value := castle
					castleChanges = append(castleChanges, CastleChange{ID: id, Castle: &value})
					continue
				}
				castleChanges = append(castleChanges, CastleChange{ID: id, Patch: castleComponentPatch(&castle, parts)})
			}
			patch.CastleChanges = &castleChanges
		}
	}
	if components.Has(ComponentCommanders) {
		patch.Commanders = &state.Commanders
	}
	if components.Has(ComponentGenerals) {
		patch.Generals = &state.Generals
	}
	if components.Has(ComponentCastellans) {
		patch.Castellans = &state.Castellans
	}
	if components.Has(ComponentMovements) {
		if changes.replaceMovements || len(changes.movementIDs) == 0 {
			value := state.materializedMovements()
			patch.Movements = &value
		} else {
			values := make([]MovementChange, 0, len(changes.movementIDs))
			for _, id := range changes.movementIDs {
				change := MovementChange{ID: id}
				if movement, found := state.LookupMovement(id); found {
					value := movement
					change.Movement = &value
				} else {
					change.Deleted = true
				}
				values = append(values, change)
			}
			patch.MovementChanges = &values
		}
	}
	if components.Has(ComponentMovementSnapshot) {
		patch.MovementSnapshot = &state.MovementSnapshot
	}
	if components.Has(ComponentStationing) {
		patch.Stationing = &state.Stationing
	}
	if components.Has(ComponentScheduled) {
		patch.Scheduled = &state.Scheduled
	}
	if components.Has(ComponentRift) {
		patch.Rift = &state.Rift
	}
	if components.Has(ComponentInventory) {
		if changes.replaceInventory || changes.inventoryParts == 0 {
			patch.Inventory = &state.Inventory
		} else {
			patch.InventoryChanges = inventoryComponentPatch(&state.Inventory, changes)
		}
	}
	if components.Has(ComponentSubscriptions) {
		patch.Subscriptions = &state.Subscriptions
	}
	if components.Has(ComponentMarket) {
		patch.Market = &state.Market
	}
	if components.Has(ComponentKingdomTransport) {
		patch.KingdomTransport = &state.KingdomTransport
	}
	if components.Has(ComponentBeri) {
		patch.Beri = &state.Beri
	}
	if components.Has(ComponentAlliance) {
		patch.Alliance = &state.Alliance
	}
	if components.Has(ComponentAlliances) {
		patch.Alliances = &state.Alliances
	}
	if components.Has(ComponentAllianceHelp) {
		patch.AllianceHelpRequests = &state.AllianceHelpRequests
	}
	if components.Has(ComponentWorldMap) {
		mapChanges := normalizeMapChanges(changes.mapChanges)
		if !changes.replaceMap && len(mapChanges) > 0 {
			patch.MapChanges = &mapChanges
		} else {
			materialized := state.materializedMap()
			patch.Map = &materialized
		}
	}
	if components.Has(ComponentTowerCooldowns) {
		patch.TowerCooldowns = &state.TowerCooldowns
	}
	if components.Has(ComponentTowerQueue) {
		patch.TowerQueue = &state.TowerQueue
	}
	if components.Has(ComponentInvasion) {
		patch.Invasion = &state.Invasion
	}
	if components.Has(ComponentStorm) {
		patch.Storm = &state.Storm
	}
	if components.Has(ComponentNomadCamps) {
		patch.NomadCamps = &state.NomadCamps
	}
	if components.Has(ComponentAdvisor) {
		patch.Advisor = &state.Advisor
	}
	if components.Has(ComponentKhan) {
		patch.Khan = &state.Khan
	}
	if components.Has(ComponentDailyAttacks) {
		patch.DailyAttacks = &state.DailyAttacks
	}
	if components.Has(ComponentAttackDialog) {
		patch.AttackDialog = &state.AttackDialog
	}
	if components.Has(ComponentCombatCooldown) {
		patch.CombatCooldown = &state.CombatCooldown
	}
	if components.Has(ComponentAttackPresets) {
		patch.AttackPresets = &state.AttackPresets
	}
	if components.Has(ComponentAttackAnalytics) {
		patch.AttackAnalytics = &state.AttackAnalytics
	}
	if components.Has(ComponentEventScores) {
		if changes.replaceEventScores || len(changes.eventScoreIDs) == 0 && !changes.eventScoreMeta && !changes.eventScoreShop {
			value := state.materializedEventScores()
			patch.EventScores = &value
		} else {
			eventPatch := &EventScorePatch{}
			if changes.eventScoreMeta {
				eventPatch.ActiveEventID = &state.EventScores.ActiveEventID
				eventPatch.Inventory = &state.EventScores.Inventory
			}
			if changes.eventScoreShop {
				eventPatch.ShopByPackage = &state.EventScores.ShopByPackage
			}
			for _, eventID := range changes.eventScoreIDs {
				change := EventScoreChange{EventID: eventID}
				if score, found := state.LookupScalableEventScore(eventID); found {
					value := score
					change.Score = &value
				} else {
					change.ScoreDeleted = true
				}
				if activity, found := state.LookupEventActivity(eventID); found {
					value := activity
					change.Activity = &value
				} else {
					change.ActivityDeleted = true
				}
				if ranking, found := state.LookupEventRanking(eventID); found {
					value := ranking
					change.Ranking = &value
				} else {
					change.RankingDeleted = true
				}
				eventPatch.Changes = append(eventPatch.Changes, change)
			}
			patch.EventScoreChanges = eventPatch
		}
	}
	if components.Has(ComponentCommandContext) {
		patch.CommandContext = &state.CommandContext
	}
	if components.Has(ComponentAutomations) {
		patch.Automations = &state.Automations
	}
	if components.Has(ComponentReports) {
		patch.Reports = &state.Reports
	}
	if components.Has(ComponentObservations) {
		patch.Observations = &state.Observations
	}
	return patch
}

func castleComponentPatch(castle *CastleState, parts CastleMutationPart) *CastlePatch {
	patch := &CastlePatch{}
	if parts&CastlePartIdentity != 0 {
		patch.KingdomID = &castle.KingdomID
		patch.SlotType = &castle.SlotType
		patch.Name = &castle.Name
		patch.X = &castle.X
		patch.Y = &castle.Y
		patch.Focused = &castle.Focused
		patch.ContextSnapshotObservedAt = &castle.ContextSnapshotObservedAt
	}
	if parts&CastlePartResources != 0 {
		patch.Resources = &castle.Resources
		patch.FoodStateObservedAt = &castle.FoodStateObservedAt
	}
	if parts&CastlePartUnits != 0 {
		patch.Units = &castle.Units
		patch.UnitsObservedAt = &castle.UnitsObservedAt
	}
	if parts&CastlePartDefense != 0 {
		patch.Defense = &castle.Defense
	}
	if parts&CastlePartBuildings != 0 {
		patch.Buildings = &castle.Buildings
		patch.BuildingProduction = &castle.BuildingProduction
		patch.Layout = &castle.Layout
	}
	if parts&CastlePartConstruction != 0 {
		patch.BuildingQueue = &castle.BuildingQueue
		patch.ConstructionSlots = &castle.ConstructionSlots
		patch.ConstructionSlotsObservedAt = &castle.ConstructionSlotsObservedAt
	}
	if parts&CastlePartProduction != 0 {
		patch.Production = &castle.Production
		patch.QueueableProduction = &castle.QueueableProduction
		patch.QueueableObservedAt = &castle.QueueableObservedAt
	}
	if parts&CastlePartCrafting != 0 {
		patch.Crafting = &castle.Crafting
	}
	return patch
}

func inventoryComponentPatch(inventory *InventoryState, changes componentChanges) *InventoryPatch {
	patch := &InventoryPatch{}
	parts := changes.inventoryParts
	if parts&inventoryConstructionItemsMutable != 0 {
		patch.ConstructionItems = &inventory.ConstructionItems
		patch.ConstructionItemsObservedAt = &inventory.ConstructionItemsObservedAt
		patch.ConstructionSpaceLeft = &inventory.ConstructionSpaceLeft
		patch.ConstructionSpaceLeftObservedAt = &inventory.ConstructionSpaceLeftObservedAt
	}
	if parts&inventoryConstructionOffersMutable != 0 {
		patch.ConstructionOffers = &inventory.ConstructionOffers
		patch.ConstructionOffersObservedAt = &inventory.ConstructionOffersObservedAt
		patch.ConstructionOffersCastleID = &inventory.ConstructionOffersCastleID
		patch.ConstructionOffersKingdomID = &inventory.ConstructionOffersKingdomID
		patch.ConstructionOffersByCastle = &inventory.ConstructionOffersByCastle
	}
	if parts&inventoryEquipmentMutable != 0 {
		if changes.replaceEquipment || len(changes.equipmentIDs) == 0 {
			patch.Equipment = &inventory.Equipment
		} else {
			equipmentChanges := make([]EquipmentChange, 0, len(changes.equipmentIDs))
			for _, id := range changes.equipmentIDs {
				item, found := inventory.Equipment[id]
				if !found {
					equipmentChanges = append(equipmentChanges, EquipmentChange{ID: id, Deleted: true})
					continue
				}
				value := item
				equipmentChanges = append(equipmentChanges, EquipmentChange{ID: id, Equipment: &value})
			}
			patch.EquipmentChanges = &equipmentChanges
		}
	}
	if parts&inventoryGemsMutable != 0 {
		if changes.replaceGems || len(changes.gemIDs) == 0 {
			patch.Gems = &inventory.Gems
		} else {
			gemChanges := make([]GemChange, 0, len(changes.gemIDs))
			for _, id := range changes.gemIDs {
				gem, found := inventory.Gems[id]
				if !found {
					gemChanges = append(gemChanges, GemChange{ID: id, Deleted: true})
					continue
				}
				value := gem
				gemChanges = append(gemChanges, GemChange{ID: id, Gem: &value})
			}
			patch.GemChanges = &gemChanges
		}
	}
	if parts&inventoryGemStacksMutable != 0 {
		patch.GemStacks = &inventory.GemStacks
	}
	if parts&inventoryItemsMutable != 0 {
		if changes.replaceItems || len(changes.itemKeys) == 0 {
			patch.Items = &inventory.Items
		} else {
			itemChanges := make([]InventoryItemChange, 0, len(changes.itemKeys))
			for _, key := range changes.itemKeys {
				items, found := inventory.Items[key]
				if !found {
					itemChanges = append(itemChanges, InventoryItemChange{Collection: key, Deleted: true})
					continue
				}
				value := items
				itemChanges = append(itemChanges, InventoryItemChange{Collection: key, Items: &value})
			}
			patch.ItemChanges = &itemChanges
		}
	}
	return patch
}
