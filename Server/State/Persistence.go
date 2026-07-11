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
	state.Session = NewGameState().Session
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
		if castle.Queues == nil {
			castle.Queues = map[string][]QueueItem{}
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
	if state.Castellans == nil {
		state.Castellans = defaults.Castellans
	}
	if state.Movements == nil {
		state.Movements = defaults.Movements
	}
	if state.Inventory.ConstructionItems == nil {
		state.Inventory.ConstructionItems = defaults.Inventory.ConstructionItems
	}
	if state.Inventory.Equipment == nil {
		state.Inventory.Equipment = defaults.Inventory.Equipment
	}
	if state.Inventory.Gems == nil {
		state.Inventory.Gems = defaults.Inventory.Gems
	}
	if state.Inventory.Items == nil {
		state.Inventory.Items = defaults.Inventory.Items
	}
	if state.Alliance.Members == nil {
		state.Alliance.Members = defaults.Alliance.Members
	}
	if state.Map == nil {
		state.Map = defaults.Map
	}
	if state.Observations == nil {
		state.Observations = defaults.Observations
	}
}
