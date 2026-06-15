package featureview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/GameParser"
)

// formatDisplayName converts camelCase to Title Case (e.g. "winterBakery" -> "Winter Bakery")
func formatDisplayName(name string) string {
	if name == "" {
		return ""
	}
	var sb strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			sb.WriteRune(' ')
		}
		if i == 0 {
			sb.WriteRune(unicode.ToUpper(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// TCIDisplayNameFromInternal matches GeneralsCamp getCIName: GGE LangEn ci_* keys on the internal
// `name` field, then title-case `_display_name` only when LangEn has no entry.
func TCIDisplayNameFromInternal(name, displayNameKey string) string {
	label := GetTCIDisplayName(name)
	if label == name {
		fallback := displayNameKey
		if fallback == "" {
			fallback = name
		}
		label = formatDisplayName(fallback)
	}
	return label
}

// TCIDisplayNameForItemMeta uses [TCIDisplayNameFromInternal] with [GameParser.ConstructionItemMeta] fields.
func TCIDisplayNameForItemMeta(m GameParser.ConstructionItemMeta) string {
	return TCIDisplayNameFromInternal(m.Name, m.DisplayNameKey)
}

// CatalogGroupTier is one wire CID tier in a design group (all tiers of one temporary/appearance line).
type CatalogGroupTier struct {
	WireCID int `json:"wireCid"`
	Level   int `json:"level"`
}

// ConstructionItemCatalogEntry is one TCI design group: several wire CIDs (one per in-game level tier) with
// a single display name, shared effect list, and [MinLevel, MaxLevel] for the tier range.
type ConstructionItemCatalogEntry struct {
	ID         int                `json:"id"`
	GroupIDs   []int              `json:"groupIds"`
	GroupTiers []CatalogGroupTier `json:"groupTiers"`
	MinLevel   int                `json:"minLevel"`
	MaxLevel   int                `json:"maxLevel"`
	Label      string             `json:"label"`
	Internal   string             `json:"internal"`
	Level      string             `json:"level"`
	Category   string             `json:"category"`
	Effects    string             `json:"effects"`
}

var (
	catalogOnce     sync.Once
	catalogCached   []ConstructionItemCatalogEntry
	catalogErr      error
	effectNamesMap  map[string]string
	effectNamesOnce sync.Once
)

func loadEffectNames() {
	effectNamesMap = make(map[string]string)
	rawBytes, err := serverdata.ReadEffectsItemsJSON()
	if err == nil && len(rawBytes) > 0 {
		var raw []struct {
			EffectID string `json:"effectID"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(rawBytes, &raw); err == nil {
			for _, r := range raw {
				effectNamesMap[r.EffectID] = r.Name
			}
		}
	}
}

func findConstructionItemsJSON() ([]byte, error) {
	return serverdata.ReadConstructionItemsJSON()
}

// ConstructionItemsCatalog returns parsed temporary construction item rows for the client picker.
// Appearance items and legacy 4-digit wire CIDs (1000–9999) are omitted.
// Groups follow GeneralsCamp building_items overview: same display name, split by effect signature
// (wire effects + legacy stat columns), each group is up to four upgrade tiers.
func ConstructionItemsCatalog() ([]ConstructionItemCatalogEntry, error) {
	catalogOnce.Do(func() {
		rawBytes, err := findConstructionItemsJSON()
		if err != nil {
			catalogErr = err
			return
		}
		var raw []map[string]interface{}
		if err := json.Unmarshal(rawBytes, &raw); err != nil {
			catalogErr = err
			return
		}

		type grouped struct {
			items []map[string]interface{}
		}
		groups := make(map[string]*grouped)
		var orderedKeys []string

		for _, row := range raw {
			if !isTCIRow(row) {
				continue
			}
			key := tciGroupKey(row)
			g, ok := groups[key]
			if !ok {
				g = &grouped{}
				groups[key] = g
				orderedKeys = append(orderedKeys, key)
			}
			g.items = append(g.items, row)
		}

		out := make([]ConstructionItemCatalogEntry, 0, len(orderedKeys))
		for _, key := range orderedKeys {
			g := groups[key]
			if len(g.items) == 0 {
				continue
			}
			sortTCIGroupItems(g.items)
			first := g.items[0]

			label := TCIDisplayNameFromInternal(mapString(first, "name"), mapString(first, "_display_name"))
			effects := buildTCIDisplayEffects(first)
			category := mapString(first, "comment1")

			tiers := make([]CatalogGroupTier, 0, len(g.items))
			minLevel, maxLevel := 0, 0
			for _, row := range g.items {
				cid := mapInt(row, "constructionItemID")
				lvl := mapInt(row, "level")
				if lvl <= 0 {
					lvl = 1
				}
				if cid <= 0 {
					continue
				}
				if minLevel == 0 || lvl < minLevel {
					minLevel = lvl
				}
				if lvl > maxLevel {
					maxLevel = lvl
				}
				tiers = append(tiers, CatalogGroupTier{WireCID: cid, Level: lvl})
			}
			if len(tiers) == 0 {
				continue
			}
			sort.Slice(tiers, func(i, j int) bool {
				if tiers[i].Level != tiers[j].Level {
					return tiers[i].Level < tiers[j].Level
				}
				return tiers[i].WireCID < tiers[j].WireCID
			})
			baseID := tiers[0].WireCID
			orderedIDs := make([]int, len(tiers))
			for i := range tiers {
				orderedIDs[i] = tiers[i].WireCID
			}
			levelStr := fmt.Sprintf("%d", minLevel)
			if minLevel != maxLevel {
				levelStr = fmt.Sprintf("%d-%d", minLevel, maxLevel)
			}
			out = append(out, ConstructionItemCatalogEntry{
				ID:         baseID,
				GroupIDs:   orderedIDs,
				GroupTiers: tiers,
				MinLevel:   minLevel,
				MaxLevel:   maxLevel,
				Label:      label,
				Internal:   mapString(first, "name"),
				Level:      levelStr,
				Category:   category,
				Effects:    effects,
			})
			_ = key
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Label != out[j].Label {
				return out[i].Label < out[j].Label
			}
			if out[i].Effects != out[j].Effects {
				return out[i].Effects < out[j].Effects
			}
			return out[i].ID < out[j].ID
		})
		catalogCached = out
	})
	return catalogCached, catalogErr
}
