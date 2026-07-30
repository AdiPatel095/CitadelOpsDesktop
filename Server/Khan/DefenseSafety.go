package Khan

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const DefensePresetsSection = "defense.presets"

type DefensePresetDocument struct {
	Version int             `json:"version"`
	Presets []DefensePreset `json:"presets"`
}

type DefensePreset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Wall struct {
		Left   State.DefenseWallSection `json:"left"`
		Middle State.DefenseWallSection `json:"middle"`
		Right  State.DefenseWallSection `json:"right"`
	} `json:"wall"`
	Moat struct {
		LeftToolSlots   []State.DefenseToolSlot `json:"leftToolSlots"`
		MiddleToolSlots []State.DefenseToolSlot `json:"middleToolSlots"`
		RightToolSlots  []State.DefenseToolSlot `json:"rightToolSlots"`
	} `json:"moat"`
	Keep *struct {
		MAUCT           int64 `json:"mauct"`
		UnitTypePercent int   `json:"unitTypePercent"`
	} `json:"keep,omitempty"`
}

type WallRisk struct {
	Capacity       int64
	Assigned       int64
	OffensiveUnits int64
}

func DecodeDefensePreset(raw json.RawMessage, presetID string) (DefensePreset, error) {
	var document DefensePresetDocument
	if len(raw) == 0 {
		return DefensePreset{}, fmt.Errorf("defense presets are not configured")
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return DefensePreset{}, fmt.Errorf("decode defense presets: %w", err)
	}
	if document.Version != 1 {
		return DefensePreset{}, fmt.Errorf("unsupported defense preset document version %d", document.Version)
	}
	presetID = strings.TrimSpace(presetID)
	for _, preset := range document.Presets {
		if preset.ID == presetID {
			return preset, nil
		}
	}
	return DefensePreset{}, fmt.Errorf("defense preset %q no longer exists", presetID)
}

func Matches(castle State.CastleState, preset DefensePreset) bool {
	wall := castle.Defense.Wall
	if !reflect.DeepEqual(wall.Left, preset.Wall.Left) ||
		!reflect.DeepEqual(wall.Middle, preset.Wall.Middle) ||
		!reflect.DeepEqual(wall.Right, preset.Wall.Right) {
		return false
	}
	moat := castle.Defense.Moat
	if !reflect.DeepEqual(moat.LeftToolSlots, preset.Moat.LeftToolSlots) ||
		!reflect.DeepEqual(moat.MiddleToolSlots, preset.Moat.MiddleToolSlots) ||
		!reflect.DeepEqual(moat.RightToolSlots, preset.Moat.RightToolSlots) {
		return false
	}
	return preset.Keep == nil || castle.Defense.Keep.MAUCT == preset.Keep.MAUCT &&
		castle.Defense.Keep.UnitTypePercent == preset.Keep.UnitTypePercent
}

func OffensiveWallUnits(castle State.CastleState, gameData *GameData.Store, preset DefensePreset) (WallRisk, error) {
	if gameData == nil {
		return WallRisk{}, fmt.Errorf("official game data is unavailable")
	}
	units, err := gameData.Catalog("units")
	if err != nil {
		return WallRisk{}, fmt.Errorf("load official units: %w", err)
	}
	capacity := max(int64(0), castle.Defense.Wall.UnitCount)
	if capacity == 0 {
		return WallRisk{}, fmt.Errorf("main castle wall capacity is unavailable")
	}
	meleeDemand, rangedDemand := wallRoleDemand(capacity, preset)
	risk := WallRisk{Capacity: capacity}
	assigned, offensive, err := assignWallRole(units, castle.Defense.Inventory, castle.Defense.MeleeUnitIDs, "melee", meleeDemand)
	if err != nil {
		return WallRisk{}, err
	}
	risk.Assigned += assigned
	risk.OffensiveUnits += offensive
	assigned, offensive, err = assignWallRole(units, castle.Defense.Inventory, castle.Defense.RangedUnitIDs, "ranged", rangedDemand)
	if err != nil {
		return WallRisk{}, err
	}
	risk.Assigned += assigned
	risk.OffensiveUnits += offensive
	return risk, nil
}

func wallRoleDemand(capacity int64, preset DefensePreset) (int64, int64) {
	sections := []State.DefenseWallSection{preset.Wall.Left, preset.Wall.Middle, preset.Wall.Right}
	remaining := capacity
	var melee int64
	for index, section := range sections {
		sectionCapacity := capacity * int64(section.UnitPercent) / 100
		if index == len(sections)-1 {
			sectionCapacity = remaining
		}
		sectionCapacity = min(max(int64(0), sectionCapacity), remaining)
		remaining -= sectionCapacity
		melee += sectionCapacity * int64(section.UnitTypePercent) / 100
	}
	return melee, capacity - melee
}

func assignWallRole(
	catalog *GameData.Catalog,
	inventory map[State.UnitID]int64,
	priority []State.UnitID,
	role string,
	demand int64,
) (int64, int64, error) {
	var assigned int64
	var offensive int64
	seen := map[State.UnitID]struct{}{}
	for _, unitID := range priority {
		if assigned >= demand {
			break
		}
		if unitID <= 0 {
			continue
		}
		if _, duplicate := seen[unitID]; duplicate {
			continue
		}
		seen[unitID] = struct{}{}
		available := max(int64(0), inventory[unitID])
		if available == 0 {
			continue
		}
		raw, found := catalog.Find(strconv.FormatInt(int64(unitID), 10))
		if !found {
			return 0, 0, fmt.Errorf("official unit %d is unavailable", unitID)
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("decode official unit %d: %w", unitID, err)
		}
		unitRole, _ := record.String("role")
		if !strings.EqualFold(strings.TrimSpace(unitRole), role) {
			continue
		}
		amount := min(available, demand-assigned)
		assigned += amount
		fightType, known := record.Int64("fightType")
		if !known {
			return 0, 0, fmt.Errorf("official unit %d has no fight type", unitID)
		}
		if fightType == 0 {
			offensive += amount
		}
	}
	return assigned, offensive, nil
}
