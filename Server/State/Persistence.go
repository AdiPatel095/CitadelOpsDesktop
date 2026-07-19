package State

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const snapshotFileName = "GameState.json"

type persistedSnapshot struct {
	SchemaVersion int       `json:"schemaVersion"`
	SavedAt       time.Time `json:"savedAt"`
	State         GameState `json:"state"`
}

func LoadSnapshot(dataDir string) (GameState, error) {
	contents, err := os.ReadFile(snapshotPath(dataDir))
	if err != nil {
		return GameState{}, err
	}
	var document persistedSnapshot
	if err := json.Unmarshal(contents, &document); err != nil {
		return GameState{}, fmt.Errorf("decode state snapshot: %w", err)
	}
	if document.SchemaVersion != SchemaVersion || document.State.SchemaVersion != SchemaVersion {
		return GameState{}, fmt.Errorf("state snapshot schema %d is not supported by schema %d", document.SchemaVersion, SchemaVersion)
	}
	state := document.State
	normalizeStateMaps(&state)
	lastServerURL := state.Session.ServerURL
	lastGeneration := state.Session.Generation
	state.Session = NewGameState().Session
	state.Session.ServerURL = lastServerURL
	state.Session.Generation = lastGeneration
	return state, nil
}

func SaveSnapshot(dataDir string, state GameState) error {
	directory := filepath.Join(dataDir, "State")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	state = cloneGameState(state)
	normalizeStateMaps(&state)
	document := persistedSnapshot{
		SchemaVersion: SchemaVersion,
		SavedAt:       time.Now().UTC(),
		State:         state,
	}
	contents, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode state snapshot: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".GameState-*")
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write state snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync state snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, snapshotPath(dataDir)); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	return nil
}

func snapshotPath(dataDir string) string {
	return filepath.Join(dataDir, "State", snapshotFileName)
}

func normalizeStateMaps(state *GameState) {
	defaults := NewGameState()
	if state.Account.WorldID == "" && state.Player.ID > 0 {
		state.Account.WorldID = state.Session.ServerURL
	}
	if state.Account.PlayerID == 0 && state.Player.ID > 0 {
		state.Account.PlayerID = state.Player.ID
	}
	if state.CommandContext.ProductionObservedAt != nil && state.CommandContext.ProductionObservedAt.IsZero() {
		state.CommandContext.ProductionObservedAt = nil
	}
	if state.Player.Resources == nil {
		state.Player.Resources = defaults.Player.Resources
	}
	if state.Player.Currencies == nil {
		state.Player.Currencies = defaults.Player.Currencies
	}
	if state.Player.LegendSkills.ActiveIDs == nil {
		state.Player.LegendSkills.ActiveIDs = defaults.Player.LegendSkills.ActiveIDs
	}
	if state.Player.LegendSkills.SceatSkillIDs == nil {
		state.Player.LegendSkills.SceatSkillIDs = defaults.Player.LegendSkills.SceatSkillIDs
	}
	if state.Player.LegendSkills.SceatActivations == nil {
		state.Player.LegendSkills.SceatActivations = defaults.Player.LegendSkills.SceatActivations
	}
	if state.Castles == nil {
		state.Castles = defaults.Castles
	}
	for id, castle := range state.Castles {
		if castle.Resources == nil {
			castle.Resources = map[ResourceID]ResourceBalance{}
		}
		if castle.Units.Stationed == nil {
			castle.Units.Stationed = map[UnitID]int64{}
		}
		if castle.Units.Traveling == nil {
			castle.Units.Traveling = map[UnitID]int64{}
		}
		if castle.Units.Hospital == nil {
			castle.Units.Hospital = map[UnitID]int64{}
		}
		if castle.Units.SpecialHospital == nil {
			castle.Units.SpecialHospital = map[UnitID]int64{}
		}
		if castle.Units.Total == nil {
			castle.Units.Total = map[UnitID]int64{}
		}
		normalizeCastleDefense(&castle.Defense)
		if castle.Buildings == nil {
			castle.Buildings = map[BuildingInstanceID]Building{}
		}
		for buildingID, building := range castle.Buildings {
			if building.Layer == "" {
				building.Placed = building.GridX >= 0 && building.GridY >= 0
				castle.Buildings[buildingID] = building
			}
		}
		if castle.Layout.Ground == nil {
			castle.Layout.Ground = map[BuildingInstanceID]Building{}
		}
		if castle.Layout.Objects == nil {
			castle.Layout.Objects = map[BuildingInstanceID]Building{}
		}
		if castle.Layout.Fixed == nil {
			castle.Layout.Fixed = map[BuildingInstanceID]Building{}
		}
		if castle.BuildingQueue.Slots == nil {
			castle.BuildingQueue.Slots = []BuildingConstructionQueueSlot{}
		}
		if castle.ConstructionSlots == nil {
			castle.ConstructionSlots = map[BuildingInstanceID][]ConstructionSlot{}
		}
		if castle.Production == nil {
			castle.Production = map[int]ProductionQueue{}
		}
		for lineID, queue := range castle.Production {
			if queue.Queued == nil {
				queue.Queued = []QueueItem{}
			}
			castle.Production[lineID] = queue
		}
		if castle.QueueableProduction == nil {
			castle.QueueableProduction = map[int][]DefinitionRef{}
		}
		for lineID, definitions := range castle.QueueableProduction {
			if definitions == nil {
				castle.QueueableProduction[lineID] = []DefinitionRef{}
			}
		}
		if castle.Crafting.Buildings == nil {
			castle.Crafting.Buildings = map[BuildingInstanceID]CraftingBuilding{}
		}
		for buildingID, building := range castle.Crafting.Buildings {
			if building.ActiveSlotRentals == nil {
				building.ActiveSlotRentals = []int{}
			}
			if building.QueueSlotRentals == nil {
				building.QueueSlotRentals = []int{}
			}
			if building.Active == nil {
				building.Active = []CraftingQueueItem{}
			}
			if building.Queued == nil {
				building.Queued = []CraftingQueueItem{}
			}
			castle.Crafting.Buildings[buildingID] = building
		}
		if castle.Crafting.EnabledRecipeIDs == nil {
			castle.Crafting.EnabledRecipeIDs = []int64{}
		}
		if castle.Crafting.EnabledRecipeGroupIDs == nil {
			castle.Crafting.EnabledRecipeGroupIDs = []int64{}
		}
		if castle.Crafting.OutputBoostByQueueType == nil {
			castle.Crafting.OutputBoostByQueueType = map[int]float64{}
		}
		state.Castles[id] = castle
	}
	if state.Commanders == nil {
		state.Commanders = defaults.Commanders
	}
	for id, commander := range state.Commanders {
		if commander.Equipment == nil {
			commander.Equipment = map[string]EquipmentInstanceID{}
		}
		if commander.Gems == nil {
			commander.Gems = map[string]GemInstanceID{}
		}
		state.Commanders[id] = commander
	}
	if state.Generals == nil {
		state.Generals = defaults.Generals
	}
	for id, general := range state.Generals {
		if general.ActiveSkillIDs == nil {
			general.ActiveSkillIDs = []int64{}
		}
		state.Generals[id] = general
	}
	if state.Castellans == nil {
		state.Castellans = defaults.Castellans
	}
	for id, castellan := range state.Castellans {
		if castellan.Equipment == nil {
			castellan.Equipment = map[string]EquipmentInstanceID{}
		}
		if castellan.Gems == nil {
			castellan.Gems = map[string]GemInstanceID{}
		}
		state.Castellans[id] = castellan
	}
	if state.Movements == nil {
		state.Movements = defaults.Movements
	}
	if state.Stationing == nil {
		state.Stationing = defaults.Stationing
	}
	for id, operation := range state.Stationing {
		if operation.Units == nil {
			operation.Units = map[UnitID]int64{}
		}
		state.Stationing[id] = operation
	}
	if state.Scheduled == nil {
		state.Scheduled = defaults.Scheduled
	}
	if state.Rift.Launches == nil {
		state.Rift.Launches = defaults.Rift.Launches
	}
	if state.Rift.DeletedLaunchIDs == nil {
		state.Rift.DeletedLaunchIDs = defaults.Rift.DeletedLaunchIDs
	}
	if state.Inventory.ConstructionItems == nil {
		state.Inventory.ConstructionItems = defaults.Inventory.ConstructionItems
	}
	if state.Inventory.ConstructionOffers == nil {
		state.Inventory.ConstructionOffers = defaults.Inventory.ConstructionOffers
	}
	if state.Inventory.Equipment == nil {
		state.Inventory.Equipment = defaults.Inventory.Equipment
	}
	for id, item := range state.Inventory.Equipment {
		if item.Effects == nil {
			item.Effects = EquipmentEffects{}
		}
		state.Inventory.Equipment[id] = item
	}
	if state.Inventory.Gems == nil {
		state.Inventory.Gems = defaults.Inventory.Gems
	}
	for id, gem := range state.Inventory.Gems {
		if gem.Effects == nil {
			gem.Effects = EquipmentEffects{}
		}
		state.Inventory.Gems[id] = gem
	}
	if state.Inventory.GemStacks == nil {
		state.Inventory.GemStacks = defaults.Inventory.GemStacks
	}
	if state.Inventory.Items == nil {
		state.Inventory.Items = defaults.Inventory.Items
	}
	if state.Subscriptions == nil {
		state.Subscriptions = defaults.Subscriptions
	}
	if state.Market.Castles == nil {
		state.Market.Castles = defaults.Market.Castles
	}
	for id, castle := range state.Market.Castles {
		if castle.Resources == nil {
			castle.Resources = map[ResourceID]float64{}
		}
		if castle.AreaEffects == nil {
			castle.AreaEffects = []MarketAreaEffect{}
		}
		state.Market.Castles[id] = castle
	}
	if state.KingdomTransport.Unlocks == nil {
		state.KingdomTransport.Unlocks = defaults.KingdomTransport.Unlocks
	}
	if state.KingdomTransport.Pending == nil {
		state.KingdomTransport.Pending = defaults.KingdomTransport.Pending
	}
	for index := range state.KingdomTransport.Pending {
		if state.KingdomTransport.Pending[index].Goods == nil {
			state.KingdomTransport.Pending[index].Goods = []KingdomTransportGood{}
		}
	}
	if state.KingdomTransport.PendingUnits == nil {
		state.KingdomTransport.PendingUnits = defaults.KingdomTransport.PendingUnits
	}
	for index := range state.KingdomTransport.PendingUnits {
		if state.KingdomTransport.PendingUnits[index].Units == nil {
			state.KingdomTransport.PendingUnits[index].Units = []KingdomTransportUnit{}
		}
	}
	if state.Beri.TroopsByUnit == nil {
		state.Beri.TroopsByUnit = defaults.Beri.TroopsByUnit
	}
	if state.AttackPresets == nil {
		state.AttackPresets = defaults.AttackPresets
	}
	if state.EventScores.ByEvent == nil {
		state.EventScores.ByEvent = defaults.EventScores.ByEvent
	}
	if state.EventScores.ShopByPackage == nil {
		state.EventScores.ShopByPackage = defaults.EventScores.ShopByPackage
	}
	if state.Invasion.LastScannedAt == nil {
		state.Invasion.LastScannedAt = defaults.Invasion.LastScannedAt
	}
	if state.Storm.LastScannedAt == nil {
		state.Storm.LastScannedAt = defaults.Storm.LastScannedAt
	}
	if state.Storm.Map.Targets == nil {
		state.Storm.Map.Targets = defaults.Storm.Map.Targets
	}
	if state.Storm.IslandReturns == nil {
		state.Storm.IslandReturns = defaults.Storm.IslandReturns
	}
	for key, operation := range state.Storm.IslandReturns {
		if operation.Survivors == nil {
			operation.Survivors = map[UnitID]int64{}
			state.Storm.IslandReturns[key] = operation
		}
	}
	if state.NomadCamps.LastScannedAt == nil {
		state.NomadCamps.LastScannedAt = defaults.NomadCamps.LastScannedAt
	}
	if state.NomadCamps.Cooldowns == nil {
		state.NomadCamps.Cooldowns = defaults.NomadCamps.Cooldowns
	}
	if state.Alliance.Members == nil {
		state.Alliance.Members = defaults.Alliance.Members
	}
	if state.Alliance.Holdings == nil {
		state.Alliance.Holdings = defaults.Alliance.Holdings
	}
	if state.Alliances == nil {
		state.Alliances = defaults.Alliances
	}
	if state.Alliance.ID > 0 {
		state.Alliances[state.Alliance.ID] = state.Alliance
	}
	if state.Player.AllianceID > 0 && state.Alliance.ID != state.Player.AllianceID {
		if own, found := state.Alliances[state.Player.AllianceID]; found {
			state.Alliance = own
		} else {
			state.Alliance = AllianceState{
				ID: state.Player.AllianceID, Members: []AllianceMember{}, Holdings: []AllianceHolding{},
			}
		}
	}
	for id, alliance := range state.Alliances {
		if alliance.Members == nil {
			alliance.Members = []AllianceMember{}
		}
		if alliance.Holdings == nil {
			alliance.Holdings = []AllianceHolding{}
		}
		state.Alliances[id] = alliance
	}
	if state.Map == nil {
		state.Map = defaults.Map
	}
	if state.TowerCooldowns == nil {
		state.TowerCooldowns = defaults.TowerCooldowns
	}
	if state.TowerQueue.EntriesByCastle == nil {
		state.TowerQueue.EntriesByCastle = defaults.TowerQueue.EntriesByCastle
	}
	if state.TowerQueue.LastScannedAt == nil {
		state.TowerQueue.LastScannedAt = defaults.TowerQueue.LastScannedAt
	}
	if state.Khan.Launches == nil {
		state.Khan.Launches = defaults.Khan.Launches
	}
	if state.Khan.Taunts == nil {
		state.Khan.Taunts = defaults.Khan.Taunts
	}
	for castleID, entries := range state.TowerQueue.EntriesByCastle {
		if entries == nil {
			state.TowerQueue.EntriesByCastle[castleID] = []TowerQueueEntry{}
		}
	}
	if state.Automations == nil {
		state.Automations = defaults.Automations
	}
	for id, automation := range state.Automations {
		if automation.Metrics == nil {
			automation.Metrics = map[string]float64{}
		}
		state.Automations[id] = automation
	}
	if state.Reports.Notices == nil {
		state.Reports.Notices = defaults.Reports.Notices
	}
	if state.Reports.SpyCaptures == nil {
		state.Reports.SpyCaptures = defaults.Reports.SpyCaptures
	}
	if state.Reports.BattleCaptures == nil {
		state.Reports.BattleCaptures = defaults.Reports.BattleCaptures
	}
	if state.Observations == nil {
		state.Observations = defaults.Observations
	}
}

func normalizeCastleDefense(defense *CastleDefenseState) {
	if defense.RangedUnitIDs == nil {
		defense.RangedUnitIDs = []UnitID{}
	}
	if defense.MeleeUnitIDs == nil {
		defense.MeleeUnitIDs = []UnitID{}
	}
	if defense.Inventory == nil {
		defense.Inventory = map[UnitID]int64{}
	}
	if defense.Wall.Left.ToolSlots == nil {
		defense.Wall.Left.ToolSlots = []DefenseToolSlot{}
	}
	if defense.Wall.Middle.ToolSlots == nil {
		defense.Wall.Middle.ToolSlots = []DefenseToolSlot{}
	}
	if defense.Wall.Right.ToolSlots == nil {
		defense.Wall.Right.ToolSlots = []DefenseToolSlot{}
	}
	if defense.Keep.PrimaryToolSlots == nil {
		defense.Keep.PrimaryToolSlots = []DefenseToolSlot{}
	}
	if defense.Keep.SecondaryToolSlots == nil {
		defense.Keep.SecondaryToolSlots = []DefenseToolSlot{}
	}
	if defense.Moat.LeftToolSlots == nil {
		defense.Moat.LeftToolSlots = []DefenseToolSlot{}
	}
	if defense.Moat.MiddleToolSlots == nil {
		defense.Moat.MiddleToolSlots = []DefenseToolSlot{}
	}
	if defense.Moat.RightToolSlots == nil {
		defense.Moat.RightToolSlots = []DefenseToolSlot{}
	}
}
