package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

// BarracksRecruitStackBoost is one equipped construction item contributing to recruit stack size.
type BarracksRecruitStackBoost struct {
	CID       int
	Level     int
	StackSize int
	Name      string
	Comment2  string
}

// BarracksRecruitStackCapacityBreakdown describes the per-castle recruit stack amount calculation.
type BarracksRecruitStackCapacityBreakdown struct {
	BuildingOID       int
	BuildingWID       int
	BuildingLevel     int
	BaseStackSize     int
	ConstructionBonus int
	TotalStackSize    int
	Boosts            []BarracksRecruitStackBoost
}

var (
	barracksStackSizeOnce sync.Once
	barracksStackSizeByID map[int]int
)

var fallbackBarracksStackSizeByID = map[int]int{
	475:  5,
	160:  80,
	161:  80,
	162:  80,
	163:  80,
	164:  80,
	1741: 90,
	1742: 90,
	1743: 100,
	1744: 100,
	1748: 110,
	1749: 110,
	1750: 110,
	1751: 110,
	1938: 110,
	1939: 110,
	3112: 130,
	3113: 150,
	3114: 175,
	3115: 200,
	3116: 250,
}

func buildBarracksStackSizeByID() {
	barracksStackSizeByID = make(map[int]int, len(fallbackBarracksStackSizeByID))
	for wid, stackSize := range fallbackBarracksStackSizeByID {
		barracksStackSizeByID[wid] = stackSize
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
		if r.WodID <= 0 || !strings.Contains(strings.ToLower(strings.TrimSpace(r.Name)), "barrack") {
			continue
		}
		stackSize, err := strconv.Atoi(strings.TrimSpace(r.StackSize))
		if err != nil || stackSize <= 0 {
			continue
		}
		barracksStackSizeByID[r.WodID] = stackSize
	}
}

func barracksStackSizeForWID(wid int) (int, bool) {
	if wid <= 0 {
		return 0, false
	}
	barracksStackSizeOnce.Do(buildBarracksStackSizeByID)
	stackSize, ok := barracksStackSizeByID[wid]
	return stackSize, ok
}

func buildingLevel(row castle.BuildingData) int {
	if row.Level > 0 {
		return row.Level
	}
	info := castle.GetBuildingInfo(row.BuildingID)
	return info.Level
}

func constructionStackBoostAppliesToBuilding(meta ConstructionItemMeta, buildingWID int) bool {
	if meta.StackSize <= 0 {
		return false
	}
	if meta.ConstructionItemGroupID > 0 {
		wids, ok := BuildingWodIDsForConstructionItemGroup(meta.ConstructionItemGroupID)
		if ok && len(wids) > 0 {
			_, applies := wids[buildingWID]
			return applies
		}
	}
	if strings.EqualFold(strings.TrimSpace(meta.LockRemoval), "SOLDIER_RECRUITMENT") {
		return true
	}
	name := strings.ToLower(meta.Name + " " + meta.Comment2 + " " + meta.DisplayNameKey)
	return strings.Contains(name, "barrack") && strings.Contains(name, "stack")
}

func barracksConstructionStackBoosts(c *castle.PlayerCastleInfo, row castle.BuildingData) []BarracksRecruitStackBoost {
	if c == nil || row.OID <= 0 {
		return nil
	}
	var boosts []BarracksRecruitStackBoost
	for _, b := range c.ConstructionByBuilding {
		if b.OID != row.OID {
			continue
		}
		for _, slot := range b.Slots {
			meta, ok := ConstructionItemMetaByCID(slot.CID)
			if !ok || !constructionStackBoostAppliesToBuilding(meta, row.BuildingID) {
				continue
			}
			level := slot.Level
			if level <= 0 {
				level = meta.Level
			}
			boosts = append(boosts, BarracksRecruitStackBoost{
				CID:       slot.CID,
				Level:     level,
				StackSize: meta.StackSize,
				Name:      meta.Name,
				Comment2:  meta.Comment2,
			})
		}
	}
	return boosts
}

func constructionBoostTotal(boosts []BarracksRecruitStackBoost) int {
	total := 0
	for _, boost := range boosts {
		total += boost.StackSize
	}
	return total
}

// BarracksRecruitStackCapacity returns the best static unit count for one recruit queue stack in a castle.
func BarracksRecruitStackCapacity(c *castle.PlayerCastleInfo) int {
	return BarracksRecruitStackCapacityDetails(c).TotalStackSize
}

// BarracksRecruitStackCapacityDetails returns the best per-castle recruit stack amount and its sources.
func BarracksRecruitStackCapacityDetails(c *castle.PlayerCastleInfo) BarracksRecruitStackCapacityBreakdown {
	if c == nil {
		return BarracksRecruitStackCapacityBreakdown{}
	}
	best := BarracksRecruitStackCapacityBreakdown{}
	for _, row := range c.AllBuildingRows() {
		base, ok := barracksStackSizeForWID(row.BuildingID)
		if !ok {
			continue
		}
		boosts := barracksConstructionStackBoosts(c, row)
		bonus := constructionBoostTotal(boosts)
		total := base + bonus
		if total > best.TotalStackSize {
			best = BarracksRecruitStackCapacityBreakdown{
				BuildingOID:       row.OID,
				BuildingWID:       row.BuildingID,
				BuildingLevel:     buildingLevel(row),
				BaseStackSize:     base,
				ConstructionBonus: bonus,
				TotalStackSize:    total,
				Boosts:            boosts,
			}
		}
	}
	return best
}
