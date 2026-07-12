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
	state.Session = NewGameState().Session
	state.Session.ServerURL = lastServerURL
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
	if state.CommandContext.ProductionObservedAt != nil && state.CommandContext.ProductionObservedAt.IsZero() {
		state.CommandContext.ProductionObservedAt = nil
	}
	if state.Player.Resources == nil {
		state.Player.Resources = defaults.Player.Resources
	}
	if state.Player.Currencies == nil {
		state.Player.Currencies = defaults.Player.Currencies
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
		if castle.Buildings == nil {
			castle.Buildings = map[BuildingInstanceID]Building{}
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
	if state.Inventory.ConstructionItems == nil {
		state.Inventory.ConstructionItems = defaults.Inventory.ConstructionItems
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
