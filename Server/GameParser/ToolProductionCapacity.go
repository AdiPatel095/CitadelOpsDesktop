package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

// ToolProductionStackCapacityBreakdown describes the per-castle tool stack amount calculation.
type ToolProductionStackCapacityBreakdown struct {
	BuildingOID    int
	BuildingWID    int
	BuildingLevel  int
	TotalStackSize int
	Source         string
}

type toolWorkshopKind string

const (
	toolWorkshopKindAny     toolWorkshopKind = ""
	toolWorkshopKindSiege   toolWorkshopKind = "workshop"
	toolWorkshopKindDefense toolWorkshopKind = "dworkshop"
)

var (
	toolWorkshopStackSizeOnce sync.Once
	toolWorkshopStackSizeByID map[int]int
	toolWorkshopKindOnce      sync.Once
	toolWorkshopKindByToolID  map[int]toolWorkshopKind
)

// fallbackToolWorkshopStackSizeByID is keyed by the regular siege/defense workshop WOD IDs.
// Official building JSON snapshots do not always expose stackSize on these rows, so the fallback
// preserves the building-level-only behavior for Auto Tool.
var fallbackToolWorkshopStackSizeByID = map[int]int{
	165: 20,
	166: 40,
	167: 60,
	256: 80,
	176: 20,
	177: 40,
	178: 60,
	257: 80,
}

var toolWorkshopKindByBuildingID = map[int]toolWorkshopKind{
	165: toolWorkshopKindSiege,
	166: toolWorkshopKindSiege,
	167: toolWorkshopKindSiege,
	256: toolWorkshopKindSiege,
	176: toolWorkshopKindDefense,
	177: toolWorkshopKindDefense,
	178: toolWorkshopKindDefense,
	257: toolWorkshopKindDefense,
}

func buildToolWorkshopStackSizeByID() {
	toolWorkshopStackSizeByID = make(map[int]int, len(fallbackToolWorkshopStackSizeByID))
	for wid, stackSize := range fallbackToolWorkshopStackSizeByID {
		toolWorkshopStackSizeByID[wid] = stackSize
	}

	b, err := serverdata.ReadBuildingsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID     int    `json:"wodID"`
		Name      string `json:"name"`
		StackSize string `json:"stackSize"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		name := strings.TrimSpace(strings.ToLower(r.Name))
		if r.WodID <= 0 || (name != "workshop" && name != "dworkshop") {
			continue
		}
		stackSize, err := strconv.Atoi(strings.TrimSpace(r.StackSize))
		if err != nil || stackSize <= 0 {
			continue
		}
		toolWorkshopStackSizeByID[r.WodID] = stackSize
	}
}

func toolWorkshopStackSizeForWID(wid int) (int, bool) {
	if wid <= 0 {
		return 0, false
	}
	toolWorkshopStackSizeOnce.Do(buildToolWorkshopStackSizeByID)
	stackSize, ok := toolWorkshopStackSizeByID[wid]
	return stackSize, ok
}

func buildToolWorkshopKindByToolID() {
	toolWorkshopKindByToolID = make(map[int]toolWorkshopKind)

	b, err := serverdata.ReadToolsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID int    `json:"wodID"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		if r.WodID <= 0 {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(r.Name)) {
		case string(toolWorkshopKindSiege):
			toolWorkshopKindByToolID[r.WodID] = toolWorkshopKindSiege
		case string(toolWorkshopKindDefense):
			toolWorkshopKindByToolID[r.WodID] = toolWorkshopKindDefense
		}
	}
}

func toolWorkshopKindForTool(toolID int) toolWorkshopKind {
	if toolID <= 0 {
		return toolWorkshopKindAny
	}
	toolWorkshopKindOnce.Do(buildToolWorkshopKindByToolID)
	return toolWorkshopKindByToolID[toolID]
}

// ToolProductionStackCapacity returns the best static tool count for one tool queue stack in a castle.
func ToolProductionStackCapacity(c *castle.PlayerCastleInfo) int {
	return ToolProductionStackCapacityDetails(c).TotalStackSize
}

// ToolProductionStackCapacityForTool returns the best static tool count for one selected tool queue stack in a castle.
func ToolProductionStackCapacityForTool(c *castle.PlayerCastleInfo, toolID int) int {
	return ToolProductionStackCapacityDetailsForTool(c, toolID).TotalStackSize
}

// ToolProductionStackCapacityDetails returns the best per-castle tool stack amount from workshop level only.
func ToolProductionStackCapacityDetails(c *castle.PlayerCastleInfo) ToolProductionStackCapacityBreakdown {
	return toolProductionStackCapacityDetails(c, toolWorkshopKindAny)
}

// ToolProductionStackCapacityDetailsForTool returns the best per-castle tool stack amount for a selected tool.
func ToolProductionStackCapacityDetailsForTool(c *castle.PlayerCastleInfo, toolID int) ToolProductionStackCapacityBreakdown {
	kind := toolWorkshopKindForTool(toolID)
	if kind == toolWorkshopKindAny {
		return ToolProductionStackCapacityBreakdown{Source: "unknown-tool"}
	}
	return toolProductionStackCapacityDetails(c, kind)
}

func toolProductionStackCapacityDetails(c *castle.PlayerCastleInfo, kind toolWorkshopKind) ToolProductionStackCapacityBreakdown {
	if c == nil {
		return ToolProductionStackCapacityBreakdown{}
	}
	best := ToolProductionStackCapacityBreakdown{}
	for _, row := range c.AllBuildingRows() {
		if kind != toolWorkshopKindAny && toolWorkshopKindByBuildingID[row.BuildingID] != kind {
			continue
		}
		stackSize, ok := toolWorkshopStackSizeForWID(row.BuildingID)
		if !ok {
			continue
		}
		if stackSize > best.TotalStackSize {
			source := "workshop-level"
			if kind != toolWorkshopKindAny {
				source = string(kind) + "-level"
			}
			best = ToolProductionStackCapacityBreakdown{
				BuildingOID:    row.OID,
				BuildingWID:    row.BuildingID,
				BuildingLevel:  buildingLevel(row),
				TotalStackSize: stackSize,
				Source:         source,
			}
		}
	}
	return best
}
