package Toolkit

import (
	"context"
	"encoding/json"

	gamedata "CitadelDesktop/Server/GameData"
	featureview "CitadelDesktop/Server/GameFeatures/FeatureView"
	settingsview "CitadelDesktop/Server/GameFeatures/SettingsView"
	"CitadelDesktop/Server/GameParser"
)

type catalogReadInput struct {
	Catalog  string `json:"catalog"`
	CastleID int    `json:"castleId,omitempty"`
	IDs      []int  `json:"ids,omitempty"`
}

type queueableCatalogRow struct {
	CastleID           int   `json:"castleId"`
	BuildingRowsLoaded bool  `json:"buildingRowsLoaded"`
	RecruitUnitIDs     []int `json:"recruitUnitIds"`
	ToolIDs            []int `json:"toolIds"`
}

func registerCatalogTools(harness *Harness) error {
	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.catalog.read",
			Description: "Read official/static catalogs or live-derived production choices. Use ids to limit large building, troop, tool, or construction-item metadata results.",
			InputSchema: objectSchema(map[string]interface{}{
				"catalog": enumProperty(
					"Catalog to read.",
					"construction_items", "construction_item_meta", "auto_sceat_resources",
					"queueable_production", "buildings", "troops", "tools",
				),
				"castleId": schemaProperty("integer", "Optional castle filter for queueable_production."),
				"ids": map[string]interface{}{
					"type":        "array",
					"description": "Optional exact numeric IDs to return from a static catalog (maximum 200).",
					"items":       schemaProperty("integer", "Catalog ID."),
					"maxItems":    200,
				},
			}, "catalog"),
			Effect: EffectRead,
			Tags:   []string{"catalog", "read"},
		},
		Handler: readCatalog,
	})
}

func readCatalog(_ context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[catalogReadInput](raw)
	if err != nil {
		return nil, err
	}
	if len(input.IDs) > 200 {
		return nil, toolError("invalid_arguments", "at most 200 catalog IDs may be requested")
	}

	switch input.Catalog {
	case "construction_items":
		catalog, catalogErr := featureview.ConstructionItemsCatalog()
		if catalogErr != nil {
			return nil, toolError("catalog_unavailable", "construction items: %v", catalogErr)
		}
		if len(input.IDs) == 0 {
			return catalog, nil
		}
		wanted := idSet(input.IDs)
		filtered := make([]featureview.ConstructionItemCatalogEntry, 0, len(input.IDs))
		for _, entry := range catalog {
			if wanted[entry.ID] {
				filtered = append(filtered, entry)
				continue
			}
			for _, groupID := range entry.GroupIDs {
				if wanted[groupID] {
					filtered = append(filtered, entry)
					break
				}
			}
		}
		return filtered, nil
	case "construction_item_meta":
		catalog, catalogErr := GameParser.ExportConstructionItemMetaMap()
		if catalogErr != nil {
			return nil, toolError("catalog_unavailable", "construction item metadata: %v", catalogErr)
		}
		return filterMapByIDs(catalog, input.IDs), nil
	case "auto_sceat_resources":
		catalog, catalogErr := settingsview.BuildAutoSceatResCatalog()
		if catalogErr != nil {
			return nil, toolError("catalog_unavailable", "auto sceat resources: %v", catalogErr)
		}
		return catalog, nil
	case "queueable_production":
		return queueableProductionCatalog(input.CastleID)
	case "buildings":
		return filterMapByIDs(gamedata.BuildingIDMap, input.IDs), nil
	case "troops":
		return filterMapByIDs(gamedata.TroopIDs, input.IDs), nil
	case "tools":
		return filterMapByIDs(gamedata.ToolIDs, input.IDs), nil
	default:
		return nil, toolError("invalid_arguments", "unsupported catalog %q", input.Catalog)
	}
}

func queueableProductionCatalog(castleID int) (interface{}, error) {
	rows := make([]queueableCatalogRow, 0, len(liveCastleSlots()))
	for _, slot := range liveCastleSlots() {
		if slot.Info == nil || slot.Info.Aid <= 0 {
			continue
		}
		id := int(slot.Info.Aid)
		if castleID > 0 && castleID != id {
			continue
		}
		production := GameParser.QueueableProductionForCastle(slot.Info)
		recruitIDs := production.RecruitUnitIDs
		toolIDs := production.ToolIDs
		if recruitIDs == nil {
			recruitIDs = []int{}
		}
		if toolIDs == nil {
			toolIDs = []int{}
		}
		rows = append(rows, queueableCatalogRow{
			CastleID:           id,
			BuildingRowsLoaded: production.BuildingRowsLoaded,
			RecruitUnitIDs:     recruitIDs,
			ToolIDs:            toolIDs,
		})
	}
	if castleID > 0 && len(rows) == 0 {
		return nil, toolError("not_found", "castle %d is not present in live state", castleID)
	}
	return rows, nil
}

func idSet(ids []int) map[int]bool {
	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func filterMapByIDs[T any](catalog map[int]T, ids []int) map[int]T {
	if len(ids) == 0 {
		out := make(map[int]T, len(catalog))
		for id, value := range catalog {
			out[id] = value
		}
		return out
	}
	out := make(map[int]T, len(ids))
	for _, id := range ids {
		if value, ok := catalog[id]; ok {
			out[id] = value
		}
	}
	return out
}
