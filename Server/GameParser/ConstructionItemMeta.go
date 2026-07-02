package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

// ConstructionItemMeta is one row of construction_items/items.json (GGE constructionItemID / wire CID).
// For informal human cross-checks (names, IDs, effects), community overviews exist, e.g.
// https://generalscamp.github.io/forum/overviews/building_items/index.html — not authoritative; automation uses this file and LangEn.
// Name is the **internal** `name` field used for language keys (ci_{appearance|primary|secondary}_*).
// DisplayNameKey is the `_display_name` field (also often a key, not the final English string).
// Level is the numeric tier in items.json; use for ordering tiers within a design group.
// IsTCI is true when the client TCI catalog includes this row (non-empty **duration** or **Appearance** in comment1).
type ConstructionItemMeta struct {
	SlotType                int
	LockRemoval             string
	Comment2                string
	Name                    string
	Comment1                string
	HasAppearance           bool
	Level                   int
	StackSize               int
	DisplayNameKey          string
	ConstructionItemGroupID int
	IsTCI                   bool
}

var (
	constructionItemMetaOnce sync.Once
	constructionItemMetaMap  map[int]ConstructionItemMeta
	constructionItemMetaErr  error
)

func buildConstructionItemMeta() {
	constructionItemMetaMap = make(map[int]ConstructionItemMeta)
	b, err := serverdata.ReadConstructionItemsJSON()
	if err != nil {
		constructionItemMetaErr = err
		return
	}
	var rows []struct {
		ConstructionItemID      string `json:"constructionItemID"`
		Comment1                string `json:"comment1"`
		Comment2                string `json:"comment2"`
		Name                    string `json:"name"`
		SlotTypeID              string `json:"slotTypeID"`
		Level                   string `json:"level"`
		LockRemoval             string `json:"lockRemoval"`
		DisplayName             string `json:"_display_name"`
		ConstructionItemGroupID string `json:"constructionItemGroupID"`
		StackSize               string `json:"stackSize"`
		Duration                string `json:"duration"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		constructionItemMetaErr = err
		return
	}
	for _, r := range rows {
		id, err := strconv.Atoi(r.ConstructionItemID)
		if err != nil || id <= 0 {
			continue
		}
		st, _ := strconv.Atoi(strings.TrimSpace(r.SlotTypeID))
		lvl, _ := strconv.Atoi(r.Level)
		if lvl <= 0 {
			lvl = 1
		}
		stackSize, _ := strconv.Atoi(strings.TrimSpace(r.StackSize))
		gid, _ := strconv.Atoi(strings.TrimSpace(r.ConstructionItemGroupID))
		c1 := strings.TrimSpace(r.Comment1)
		hasApp := strings.EqualFold(c1, "Appearance")
		isTCI := r.Duration != "" || hasApp
		constructionItemMetaMap[id] = ConstructionItemMeta{
			SlotType:                st,
			LockRemoval:             strings.TrimSpace(r.LockRemoval),
			Comment2:                r.Comment2,
			Name:                    r.Name,
			Comment1:                r.Comment1,
			HasAppearance:           hasApp,
			Level:                   lvl,
			StackSize:               stackSize,
			DisplayNameKey:          r.DisplayName,
			ConstructionItemGroupID: gid,
			IsTCI:                   isTCI,
		}
	}
}

// ConstructionItemMetaByCID returns static row metadata for a constructionItemID, if present.
// Level and [ConstructionItemLevelByCID] read the same row in one pass.
func ConstructionItemMetaByCID(cid int) (ConstructionItemMeta, bool) {
	if cid <= 0 {
		return ConstructionItemMeta{}, false
	}
	constructionItemMetaOnce.Do(buildConstructionItemMeta)
	if constructionItemMetaErr != nil || len(constructionItemMetaMap) == 0 {
		return ConstructionItemMeta{}, false
	}
	m, ok := constructionItemMetaMap[cid]
	return m, ok
}

// ConstructionItemLevelByCID returns the items.json "level" field for a constructionItemID (wire CID).
// Same data as [ConstructionItemMetaByCID] .Level; used by GCA and AutoTCI tier ordering.
func ConstructionItemLevelByCID(cid int) (int, bool) {
	m, ok := ConstructionItemMetaByCID(cid)
	if !ok {
		return 0, false
	}
	return m.Level, true
}

// ExportConstructionItemMetaMap returns a copy of the construction-items index after a single load of items.json.
// Intended for `cmd/ConstructionTCIReport` and ad-hoc checks; keys are wire CIDs.
func ExportConstructionItemMetaMap() (map[int]ConstructionItemMeta, error) {
	constructionItemMetaOnce.Do(buildConstructionItemMeta)
	if constructionItemMetaErr != nil {
		return nil, constructionItemMetaErr
	}
	out := make(map[int]ConstructionItemMeta, len(constructionItemMetaMap))
	for k, v := range constructionItemMetaMap {
		out[k] = v
	}
	return out, nil
}
