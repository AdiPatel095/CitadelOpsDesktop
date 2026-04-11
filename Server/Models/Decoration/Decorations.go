package decoration

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	gamedata "CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Models/Castle"
)

// DecorationCatalogVersion and DecorationWIDs are defined in GameData (fixed catalog).
const DecorationCatalogVersion = gamedata.DecorationCatalogVersion

var DecorationWIDs = gamedata.DecorationWIDs

var (
	decorationWIDOnce   sync.Once
	decorationWIDLookup map[int]struct{}
)

func decorationWIDInit() {
	decorationWIDOnce.Do(func() {
		decorationWIDLookup = make(map[int]struct{}, len(DecorationWIDs))
		for _, id := range DecorationWIDs {
			decorationWIDLookup[id] = struct{}{}
		}
	})
}

var (
	buildingsJSONOnce    sync.Once
	decoNameByWID        map[int]string // type "deco" only (legacy DecorationDisplayName)
	allBuildingName      map[int]string // every wodID → name from buildings.json (lazy-loaded once)
	donationEventDecoWID map[int]struct{}
	buildingsJSONErr     error
)

var (
	decorationIndexOnce sync.Once
	decorationIndexName map[int]string // wodID → name from embedded index (matches client MetadataContext)
	decorationIndexErr  error
)

func loadDecorationIndexNames() {
	decorationIndexOnce.Do(func() {
		raw := embeddedDecorationIndexJSON
		if len(raw) == 0 {
			decorationIndexErr = fmt.Errorf("embedded decoration index is empty")
			return
		}
		var rows []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			decorationIndexErr = err
			return
		}
		decorationIndexName = make(map[int]string, len(rows))
		for _, r := range rows {
			nm := strings.TrimSpace(r.Name)
			if r.ID != 0 && nm != "" {
				decorationIndexName[r.ID] = nm
			}
		}
	})
}

func loadBuildingsJSONNameMaps() {
	buildingsJSONOnce.Do(func() {
		raw := embeddedDecorationItemsJSON
		if len(raw) == 0 {
			buildingsJSONErr = fmt.Errorf("embedded decoration items is empty")
			return
		}
		var rows []struct {
			WodID       int    `json:"wodID"`
			DisplayName string `json:"_display_name"`
			RawName     string `json:"name"`
			Type        string `json:"type"`
			Comment1    string `json:"comment1"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			buildingsJSONErr = err
			return
		}
		decoNameByWID = make(map[int]string)
		allBuildingName = make(map[int]string, len(rows))
		donationEventDecoWID = make(map[int]struct{})
		for _, r := range rows {
			display := strings.TrimSpace(r.DisplayName)
			if display == "" {
				display = strings.TrimSpace(r.RawName)
			}
			allBuildingName[r.WodID] = display
			if strings.EqualFold(r.Type, "deco") {
				decoNameByWID[r.WodID] = display
				nm := strings.ToLower(display)
				c1 := strings.ToLower(strings.TrimSpace(r.Comment1))
				if strings.Contains(c1, "donationevent") || strings.Contains(nm, "donation event") {
					donationEventDecoWID[r.WodID] = struct{}{}
				}
			}
		}
	})
}

func loadDecoNamesFromBuildings() { loadBuildingsJSONNameMaps() }

// IsKnownDecorationWID reports whether wid is listed as a decoration in EmpireItems buildings.json.
func IsKnownDecorationWID(wid int) bool {
	decorationWIDInit()
	_, ok := decorationWIDLookup[wid]
	return ok
}

// DecorationDisplayName returns the display name from buildings.json for a decoration WID, if present.
func DecorationDisplayName(wid int) (string, bool) {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return "", false
	}
	s, ok := decoNameByWID[wid]
	s = strings.TrimSpace(s)
	return s, ok && s != ""
}

// EmpireBuildingDisplayName returns the localized name for any wodID in EmpireItems buildings.json.
func EmpireBuildingDisplayName(wid int) (string, bool) {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return "", false
	}
	s, ok := allBuildingName[wid]
	s = strings.TrimSpace(s)
	return s, ok && s != ""
}

type decorationCount struct {
	wid   int
	count int
	name  string
}

// isDonationEventDecorationWID is true for EmpireItems deco rows tied to donation events
// (comment1 DonationEvent* / name "Donation Event …") — not player cosmetics; omit from focus summary.
func isDonationEventDecorationWID(wid int) bool {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return false
	}
	_, ok := donationEventDecoWID[wid]
	return ok
}

// DecorationSummaryLinesForCastle returns sorted lines like "1x Rose Bush" / "3x Supplies" for rows whose WID is a
// known EmpireItems decoration (type "deco" in buildings.json), excluding donation-event decos.
// IsDecorationPickupCandidateWID stays broader for preset/SOB flows (e.g. generic tower WIDs).
// Names prefer WodDisplayNames.json (regenerate: go run ./Server/cmd/genwoddisplaynames),
// then raw buildings.json (see resolvedDisplayNameForWodID).
func DecorationSummaryLinesForCastle(c *castle.PlayerCastleInfo) []string {
	if c == nil {
		return nil
	}
	countByWID := make(map[int]int)
	for _, b := range c.BGRows {
		if IsKnownDecorationWID(b.BuildingID) && !isDonationEventDecorationWID(b.BuildingID) {
			countByWID[b.BuildingID]++
		}
	}
	for _, b := range c.BDRows {
		if IsKnownDecorationWID(b.BuildingID) && !isDonationEventDecorationWID(b.BuildingID) {
			countByWID[b.BuildingID]++
		}
	}
	if len(countByWID) == 0 {
		return nil
	}
	pairs := make([]decorationCount, 0, len(countByWID))
	for wid, n := range countByWID {
		name := resolvedDisplayNameForWodID(wid)
		pairs = append(pairs, decorationCount{wid: wid, count: n, name: name})
	}
	sort.Slice(pairs, func(i, j int) bool {
		ci := strings.ToLower(pairs[i].name)
		cj := strings.ToLower(pairs[j].name)
		if ci != cj {
			return ci < cj
		}
		return pairs[i].wid < pairs[j].wid
	})
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = fmt.Sprintf("%dx %s", p.count, p.name)
	}
	return out
}

// resolvedDisplayNameForWodID uses EmpireItems-style items.json (_display_name, else name),
// then decorations/index.json (same ids/names as the client MetadataContext), then a generic fallback.
func resolvedDisplayNameForWodID(wid int) string {
	if s, ok := EmpireBuildingDisplayName(wid); ok {
		return s
	}
	loadDecorationIndexNames()
	if decorationIndexErr == nil && decorationIndexName != nil {
		if s, ok := decorationIndexName[wid]; ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return fmt.Sprintf("Item #%d", wid)
}

// ResolvedWodDisplayName returns the same label as DecorationSummaryLinesForCastle / client decoration metadata (e.g. hover tooltips).
func ResolvedWodDisplayName(wid int) string {
	return resolvedDisplayNameForWodID(wid)
}

// IsEssentialCastleStructureByName matches core / production / defense buildings that must not be
// bulk-removed as "decorations". Cosmetic items typically do not match these substrings.
func IsEssentialCastleStructureByName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "unknown" {
		return true
	}
	essentials := []string{
		"barracks", "tower", "wall", "gate", "moat", "keep", "hospital", "stables",
		"farm", "woodcutter", "quarry", "mine", "mill", "market", "storehouse", "dwelling",
		"academy", "temple", "winery", "bakery", "apiary", "armory", "drill ground",
		"training grounds", "headquarters", "field kitchen", "ballista", "flame tower",
		"wood stock", "stone stock", "vault", "cartographer", "town house", "townhouse",
		"relic woodcutter", "relic quarry", "relic mine", "relic farmstead", "relic mill",
		"construction crane", "watchtower", "sawmill", "brickworks", "foundry", "glassworks",
		"estate", "granary", "workshop", "forge", "furnace", "kiln", "smelter",
	}
	for _, e := range essentials {
		if strings.Contains(n, e) {
			return true
		}
	}
	return false
}

// IsDecorationPickupCandidateWID is true when wid is in the decoration WID list, or when castle
// building metadata suggests a non-essential cosmetic (fallback for newer WIDs).
func IsDecorationPickupCandidateWID(wid int) bool {
	if IsKnownDecorationWID(wid) {
		return true
	}
	info := castle.GetBuildingInfo(wid)
	if info.Name == "Unknown" {
		return false
	}
	return !IsEssentialCastleStructureByName(info.Name)
}

// DecorationSOBBlockedWID is true for building type IDs the server rejects for EmpireEx sob pickup (e.g. status 61).
// Do not use IsDecorationPickupCandidateWID here: many live decorations share generic WIDs (e.g. 201 / "Tower") that
// must still be cleared off preset tiles.
func DecorationSOBBlockedWID(wid int) bool {
	switch wid {
	case 756, 1422, 2027: // construction yard, hall of legends, mead distillery — observed SOB 61
		return true
	default:
		return false
	}
}
