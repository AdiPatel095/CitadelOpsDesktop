package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

var (
	constructionGroupBuildingsOnce sync.Once
	groupToBuildingWodIDs          map[int]map[int]struct{}
	constructionGroupBuildingsErr  error
)

func buildConstructionGroupBuildingWodIDs() {
	groupToBuildingWodIDs = make(map[int]map[int]struct{})

	if b, err := serverdata.ReadConstructionItemGroupBuildingsJSON(); err == nil {
		if parseConstructionItemGroupBuildingsJSON(b) {
			return
		}
	} else {
		constructionGroupBuildingsErr = err
	}

	// Dev/Docker: fall back to full buildings catalog on disk when compact map is missing.
	b, err := serverdata.ReadBuildingsJSON()
	if err != nil {
		if constructionGroupBuildingsErr == nil {
			constructionGroupBuildingsErr = err
		}
		return
	}
	parseConstructionItemGroupBuildingsFromRows(b)
}

func parseConstructionItemGroupBuildingsJSON(b []byte) bool {
	var compact map[string][]int
	if err := json.Unmarshal(b, &compact); err != nil || len(compact) == 0 {
		return false
	}
	for gidStr, wods := range compact {
		gid, err := strconv.Atoi(strings.TrimSpace(gidStr))
		if err != nil || gid <= 0 || len(wods) == 0 {
			continue
		}
		set, ok := groupToBuildingWodIDs[gid]
		if !ok {
			set = make(map[int]struct{})
			groupToBuildingWodIDs[gid] = set
		}
		for _, wid := range wods {
			if wid > 0 {
				set[wid] = struct{}{}
			}
		}
	}
	constructionGroupBuildingsErr = nil
	return len(groupToBuildingWodIDs) > 0
}

func parseConstructionItemGroupBuildingsFromRows(b []byte) {
	var rows []struct {
		WodID                    int    `json:"wodID"`
		ConstructionItemGroupIDs string `json:"constructionItemGroupIDs"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		constructionGroupBuildingsErr = err
		return
	}
	for _, r := range rows {
		if r.WodID <= 0 {
			continue
		}
		raw := strings.TrimSpace(r.ConstructionItemGroupIDs)
		if raw == "" {
			continue
		}
		for _, part := range strings.Split(raw, ",") {
			gid, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || gid <= 0 {
				continue
			}
			set, ok := groupToBuildingWodIDs[gid]
			if !ok {
				set = make(map[int]struct{})
				groupToBuildingWodIDs[gid] = set
			}
			set[r.WodID] = struct{}{}
		}
	}
}

// ConstructionItemGroupBuildingsReady reports whether the group→wodID catalog loaded.
func ConstructionItemGroupBuildingsReady() (groupCount int, err error) {
	constructionGroupBuildingsOnce.Do(buildConstructionGroupBuildingWodIDs)
	if constructionGroupBuildingsErr != nil {
		return 0, constructionGroupBuildingsErr
	}
	return len(groupToBuildingWodIDs), nil
}

// BuildingWodIDsForConstructionItemGroup returns building type wodIDs that accept the given
// constructionItemGroupID per buildings/items.json (constructionItemGroupIDs field).
func BuildingWodIDsForConstructionItemGroup(groupID int) (map[int]struct{}, bool) {
	if groupID <= 0 {
		return nil, false
	}
	constructionGroupBuildingsOnce.Do(buildConstructionGroupBuildingWodIDs)
	if constructionGroupBuildingsErr != nil || len(groupToBuildingWodIDs) == 0 {
		return nil, false
	}
	set, ok := groupToBuildingWodIDs[groupID]
	if !ok || len(set) == 0 {
		return nil, false
	}
	return set, true
}

// BuildingWodIDsForConstructionWireCID resolves wire CID → constructionItemGroupID → allowed wodIDs.
func BuildingWodIDsForConstructionWireCID(wireCID int) (map[int]struct{}, bool) {
	meta, ok := ConstructionItemMetaByCID(wireCID)
	if !ok || meta.ConstructionItemGroupID <= 0 {
		return nil, false
	}
	return BuildingWodIDsForConstructionItemGroup(meta.ConstructionItemGroupID)
}
