package GameParser

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

type QueueableProduction struct {
	BuildingRowsLoaded bool  `json:"buildingRowsLoaded"`
	RecruitUnitIDs     []int `json:"recruitUnitIds"`
	ToolIDs            []int `json:"toolIds"`
}

type queueableProductionKind int

const (
	queueableProductionRecruit queueableProductionKind = iota + 1
	queueableProductionTool
)

type queueableBuildingProduction struct {
	Kind          queueableProductionKind
	IDs           []int
	ToolKind      toolWorkshopKind
	BuildingLevel int
}

type queueableToolCatalogEntry struct {
	ID            int
	BuildingLevel int
	Level         int
	FamilyKey     string
}

type queueableRecruitCatalogEntry struct {
	ID            int
	BuildingLevel int
	Level         int
	Leveled       bool
	FamilyKey     string
}

var (
	queueableProductionOnce       sync.Once
	queueableProductionByBuilding map[int]queueableBuildingProduction
	queueableToolCatalogByKind    map[toolWorkshopKind][]queueableToolCatalogEntry
	queueableLeveledToolIDs       map[int]struct{}
	queueableRecruitCatalog       []queueableRecruitCatalogEntry
	queueableLeveledRecruitIDs    map[int]struct{}
)

func buildQueueableProductionByBuilding() {
	queueableProductionByBuilding = make(map[int]queueableBuildingProduction)
	queueableToolCatalogByKind = make(map[toolWorkshopKind][]queueableToolCatalogEntry)
	queueableLeveledToolIDs = make(map[int]struct{})
	queueableLeveledRecruitIDs = make(map[int]struct{})

	b, err := serverdata.ReadBuildingsJSON()
	if err == nil {
		var rows []struct {
			WodID     int    `json:"wodID"`
			Name      string `json:"name"`
			Level     string `json:"level"`
			UnlockIDs string `json:"unlockIDs"`
		}
		if err := json.Unmarshal(b, &rows); err == nil {
			for _, r := range rows {
				if r.WodID <= 0 {
					continue
				}
				ids := parseQueueableUnlockIDs(r.UnlockIDs)
				name := strings.TrimSpace(strings.ToLower(r.Name))
				switch {
				case strings.Contains(name, "barrack") && len(ids) > 0:
					queueableProductionByBuilding[r.WodID] = queueableBuildingProduction{
						Kind:          queueableProductionRecruit,
						IDs:           ids,
						BuildingLevel: parseQueueablePositiveInt(r.Level),
					}
				case name == "workshop":
					queueableProductionByBuilding[r.WodID] = queueableBuildingProduction{
						Kind:          queueableProductionTool,
						IDs:           ids,
						ToolKind:      toolWorkshopKindSiege,
						BuildingLevel: parseQueueablePositiveInt(r.Level),
					}
				case name == "dworkshop":
					queueableProductionByBuilding[r.WodID] = queueableBuildingProduction{
						Kind:          queueableProductionTool,
						IDs:           ids,
						ToolKind:      toolWorkshopKindDefense,
						BuildingLevel: parseQueueablePositiveInt(r.Level),
					}
				}
			}
		}
	}

	buildQueueableRecruitIDsByLevel()
	buildQueueableToolIDsByKindAndLevel()
}

func buildQueueableRecruitIDsByLevel() {
	b, err := serverdata.ReadTroopsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID         int    `json:"wodID"`
		Name          string `json:"name"`
		BuildingLevel string `json:"buildingLevel"`
		Level         string `json:"level"`
		Type          string `json:"type"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		if r.WodID <= 0 || strings.TrimSpace(strings.ToLower(r.Name)) != "barracks" {
			continue
		}
		buildingLevel := parseQueueablePositiveInt(r.BuildingLevel)
		if buildingLevel <= 0 {
			continue
		}
		unitLevel, leveled := parseQueueableLevel(r.Level)
		if leveled {
			queueableLeveledRecruitIDs[r.WodID] = struct{}{}
		}
		familyKey := strings.TrimSpace(r.Type)
		if familyKey == "" {
			familyKey = strconv.Itoa(r.WodID)
		}
		queueableRecruitCatalog = append(queueableRecruitCatalog, queueableRecruitCatalogEntry{
			ID:            r.WodID,
			BuildingLevel: buildingLevel,
			Level:         unitLevel,
			Leveled:       leveled,
			FamilyKey:     familyKey,
		})
	}
}

func buildQueueableToolIDsByKindAndLevel() {
	b, err := serverdata.ReadToolsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID         int    `json:"wodID"`
		Name          string `json:"name"`
		BuildingLevel string `json:"buildingLevel"`
		Level         string `json:"level"`
		Type          string `json:"type"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		if r.WodID <= 0 {
			continue
		}
		level := parseQueueablePositiveInt(r.BuildingLevel)
		if level <= 0 {
			continue
		}
		var kind toolWorkshopKind
		switch strings.TrimSpace(strings.ToLower(r.Name)) {
		case string(toolWorkshopKindSiege):
			kind = toolWorkshopKindSiege
		case string(toolWorkshopKindDefense):
			kind = toolWorkshopKindDefense
		default:
			continue
		}
		toolLevel := parseQueueablePositiveInt(r.Level)
		if toolLevel > 0 {
			queueableLeveledToolIDs[r.WodID] = struct{}{}
		}
		familyKey := strings.TrimSpace(r.Type)
		if familyKey == "" {
			familyKey = strconv.Itoa(r.WodID)
		}
		queueableToolCatalogByKind[kind] = append(queueableToolCatalogByKind[kind], queueableToolCatalogEntry{
			ID:            r.WodID,
			BuildingLevel: level,
			Level:         toolLevel,
			FamilyKey:     familyKey,
		})
	}
}

func parseQueueablePositiveInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func parseQueueableLevel(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func parseQueueableUnlockIDs(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[int]struct{})
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			continue
		}
		seen[id] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func sortedIDsFromSet(set map[int]struct{}) []int {
	if len(set) == 0 {
		return nil
	}
	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func addQueueableRecruitCatalogEntries(recruit map[int]struct{}, liveRecruit map[int]struct{}, useLiveQueueable bool, maxBuildingLevel int) {
	bestLeveledByFamily := make(map[string]queueableRecruitCatalogEntry)
	for _, unit := range queueableRecruitCatalog {
		if maxBuildingLevel > 0 && unit.BuildingLevel > maxBuildingLevel {
			continue
		}
		if useLiveQueueable {
			if _, ok := liveRecruit[unit.ID]; !ok {
				continue
			}
		}
		if !unit.Leveled {
			recruit[unit.ID] = struct{}{}
			continue
		}
		best, ok := bestLeveledByFamily[unit.FamilyKey]
		if !ok || unit.Level > best.Level || (unit.Level == best.Level && unit.ID > best.ID) {
			bestLeveledByFamily[unit.FamilyKey] = unit
		}
	}
	for _, unit := range bestLeveledByFamily {
		recruit[unit.ID] = struct{}{}
	}
}

func QueueableProductionForCastle(c *castle.PlayerCastleInfo) QueueableProduction {
	if c == nil {
		return QueueableProduction{}
	}
	queueableProductionOnce.Do(buildQueueableProductionByBuilding)

	recruit := make(map[int]struct{})
	tools := make(map[int]struct{})
	liveRecruit := setFromIDs(c.QueueableUnitIDs)
	liveTools := setFromIDs(c.QueueableToolIDs)
	useLiveQueueable := c.QueueableIDsLoaded
	rows := c.AllBuildingRows()
	for _, row := range rows {
		production, ok := queueableProductionByBuilding[row.BuildingID]
		if !ok {
			continue
		}
		switch production.Kind {
		case queueableProductionRecruit:
			for _, id := range production.IDs {
				if _, isLeveledRecruit := queueableLeveledRecruitIDs[id]; isLeveledRecruit {
					continue
				}
				if useLiveQueueable {
					if _, ok := liveRecruit[id]; !ok {
						continue
					}
				}
				recruit[id] = struct{}{}
			}
			level := production.BuildingLevel
			if level <= 0 {
				level = buildingLevel(row)
			}
			addQueueableRecruitCatalogEntries(recruit, liveRecruit, useLiveQueueable, level)
		case queueableProductionTool:
			for _, id := range production.IDs {
				if _, isLeveledTool := queueableLeveledToolIDs[id]; isLeveledTool {
					continue
				}
				if useLiveQueueable {
					if _, ok := liveTools[id]; !ok {
						continue
					}
				}
				tools[id] = struct{}{}
			}
			level := production.BuildingLevel
			if level <= 0 {
				level = buildingLevel(row)
			}
			bestLeveledByFamily := make(map[string]queueableToolCatalogEntry)
			for _, tool := range queueableToolCatalogByKind[production.ToolKind] {
				if tool.BuildingLevel > level {
					continue
				}
				if useLiveQueueable {
					if _, ok := liveTools[tool.ID]; !ok {
						continue
					}
				}
				if tool.Level <= 0 {
					tools[tool.ID] = struct{}{}
					continue
				}
				best, ok := bestLeveledByFamily[tool.FamilyKey]
				if !ok || tool.Level > best.Level || (tool.Level == best.Level && tool.ID > best.ID) {
					bestLeveledByFamily[tool.FamilyKey] = tool
				}
			}
			for _, tool := range bestLeveledByFamily {
				tools[tool.ID] = struct{}{}
			}
		}
	}

	return QueueableProduction{
		BuildingRowsLoaded: c.BuildingRowsLoaded,
		RecruitUnitIDs:     sortedIDsFromSet(recruit),
		ToolIDs:            sortedIDsFromSet(tools),
	}
}
