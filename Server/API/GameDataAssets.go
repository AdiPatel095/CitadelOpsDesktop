package API

import (
	"net/http"
	"sort"
	"strings"

	"CitadelDesktop/Server/GameData"
)

type currencyIconItem struct {
	AssetName string `json:"assetName"`
	URL       string `json:"url"`
}

type constructionItemIconItem struct {
	AssetName string `json:"assetName"`
	URL       string `json:"url"`
}

type constructionItemBuildingIconItem struct {
	ConstructionItemGroupID int64  `json:"constructionItemGroupId"`
	ConstructionItemName    string `json:"constructionItemName"`
	BuildingID              int64  `json:"buildingId"`
	BuildingName            string `json:"buildingName"`
	AssetName               string `json:"assetName"`
	URL                     string `json:"url"`
}

type constructionItemBuildingIdentity struct {
	groupID int64
	name    string
}

func (server *Server) handleCurrencyIcons(writer http.ResponseWriter, request *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	manifest, err := server.config.GameData.CurrencyAssets(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "currency_icons_unavailable", err.Error())
		return
	}
	assetNames := make([]string, 0, len(manifest.Icons))
	for assetName := range manifest.Icons {
		assetNames = append(assetNames, assetName)
	}
	sort.Strings(assetNames)
	items := make([]currencyIconItem, 0, len(assetNames))
	for _, assetName := range assetNames {
		items = append(items, currencyIconItem{AssetName: assetName, URL: manifest.Icons[assetName]})
	}
	writeJSON(writer, http.StatusOK, struct {
		Metadata     GameData.SourceMetadata `json:"metadata"`
		Catalog      GameData.CatalogSummary `json:"catalog"`
		Items        []currencyIconItem      `json:"items"`
		AssetVersion string                  `json:"assetVersion"`
		AssetSource  string                  `json:"assetSource"`
	}{
		Metadata: store.Metadata(),
		Catalog: GameData.CatalogSummary{
			Name: "currency-icons", Kind: "array", Count: len(items), PrimaryKey: "assetName",
			Fields: []string{"assetName", "url"},
		},
		Items: items, AssetVersion: manifest.Version, AssetSource: manifest.SourceURL,
	})
}

func (server *Server) handleConstructionItemIcons(writer http.ResponseWriter, request *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	manifest, err := server.config.GameData.ConstructionItemAssets(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "construction_item_icons_unavailable", err.Error())
		return
	}
	assetNames := make([]string, 0, len(manifest.Icons))
	for assetName := range manifest.Icons {
		assetNames = append(assetNames, assetName)
	}
	sort.Strings(assetNames)
	items := make([]constructionItemIconItem, 0, len(assetNames))
	for _, assetName := range assetNames {
		items = append(items, constructionItemIconItem{AssetName: assetName, URL: manifest.Icons[assetName]})
	}
	writeJSON(writer, http.StatusOK, struct {
		Metadata     GameData.SourceMetadata    `json:"metadata"`
		Catalog      GameData.CatalogSummary    `json:"catalog"`
		Items        []constructionItemIconItem `json:"items"`
		AssetVersion string                     `json:"assetVersion"`
		AssetSource  string                     `json:"assetSource"`
	}{
		Metadata: store.Metadata(),
		Catalog: GameData.CatalogSummary{
			Name: "construction-item-icons", Kind: "array", Count: len(items), PrimaryKey: "assetName",
			Fields: []string{"assetName", "url"},
		},
		Items: items, AssetVersion: manifest.Version, AssetSource: manifest.SourceURL,
	})
}

func (server *Server) handleConstructionItemBuildingIcons(writer http.ResponseWriter, request *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	manifest, err := server.config.GameData.ConstructionItemAssets(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "construction_item_building_icons_unavailable", err.Error())
		return
	}
	buildingCatalog, err := store.BuildingCatalog()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "building_catalog_unavailable", err.Error())
		return
	}
	constructionItems, err := store.Catalog("constructionItems")
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "construction_item_catalog_unavailable", err.Error())
		return
	}
	items := constructionItemBuildingIcons(
		buildingCatalog.Definitions(),
		constructionItemBuildingIdentities(constructionItems),
		manifest.BuildingIcons,
	)
	writeJSON(writer, http.StatusOK, struct {
		Metadata     GameData.SourceMetadata            `json:"metadata"`
		Catalog      GameData.CatalogSummary            `json:"catalog"`
		Items        []constructionItemBuildingIconItem `json:"items"`
		AssetVersion string                             `json:"assetVersion"`
		AssetSource  string                             `json:"assetSource"`
	}{
		Metadata: store.Metadata(),
		Catalog: GameData.CatalogSummary{
			Name: "construction-item-building-icons", Kind: "array", Count: len(items), PrimaryKey: "constructionItemGroupId,constructionItemName",
			Fields: []string{"constructionItemGroupId", "constructionItemName", "buildingId", "buildingName", "assetName", "url"},
		},
		Items: items, AssetVersion: manifest.Version, AssetSource: manifest.SourceURL,
	})
}

func constructionItemBuildingIcons(
	definitions []GameData.BuildingDefinition,
	identities []constructionItemBuildingIdentity,
	buildingIcons map[string]string,
) []constructionItemBuildingIconItem {
	iconsByLowerName := make(map[string]constructionItemIconItem, len(buildingIcons))
	for assetName, url := range buildingIcons {
		iconsByLowerName[strings.ToLower(assetName)] = constructionItemIconItem{AssetName: assetName, URL: url}
	}
	definitionsByGroupID := map[int64][]GameData.BuildingDefinition{}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.InternalName) == "" {
			continue
		}
		seenGroupIDs := map[int64]struct{}{}
		for _, groupID := range definition.ConstructionItemGroupIDs {
			if groupID <= 0 {
				continue
			}
			if _, seen := seenGroupIDs[groupID]; seen {
				continue
			}
			seenGroupIDs[groupID] = struct{}{}
			definitionsByGroupID[groupID] = append(definitionsByGroupID[groupID], definition)
		}
	}
	for groupID := range definitionsByGroupID {
		candidates := definitionsByGroupID[groupID]
		sort.Slice(candidates, func(left, right int) bool {
			return preferConstructionItemBuilding(candidates[left], candidates[right])
		})
		definitionsByGroupID[groupID] = candidates
	}
	items := make([]constructionItemBuildingIconItem, 0, len(identities))
	for _, identity := range identities {
		definitions := definitionsByGroupID[identity.groupID]
		item, found := constructionItemBuildingIcon(identity, definitions, iconsByLowerName, true)
		if !found {
			item, found = constructionItemBuildingIcon(identity, definitions, iconsByLowerName, false)
		}
		if found {
			items = append(items, item)
		}
	}
	return items
}

func constructionItemBuildingIdentities(catalog *GameData.Catalog) []constructionItemBuildingIdentity {
	seen := map[constructionItemBuildingIdentity]struct{}{}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		groupID, groupFound := record.Int64("constructionItemGroupID")
		name, nameFound := record.String("name")
		name = strings.TrimSpace(name)
		if !groupFound || groupID <= 0 || !nameFound || name == "" {
			continue
		}
		seen[constructionItemBuildingIdentity{groupID: groupID, name: name}] = struct{}{}
	}
	identities := make([]constructionItemBuildingIdentity, 0, len(seen))
	for identity := range seen {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(left, right int) bool {
		if identities[left].groupID != identities[right].groupID {
			return identities[left].groupID < identities[right].groupID
		}
		return identities[left].name < identities[right].name
	})
	return identities
}

func constructionItemBuildingIcon(
	identity constructionItemBuildingIdentity,
	definitions []GameData.BuildingDefinition,
	icons map[string]constructionItemIconItem,
	appearance bool,
) (constructionItemBuildingIconItem, bool) {
	for _, definition := range definitions {
		suffix := strings.TrimSpace(definition.Type)
		if appearance {
			suffix = identity.name
		}
		assetName := strings.TrimSpace(definition.InternalName) + "_Building_" + suffix
		icon, found := icons[strings.ToLower(assetName)]
		if !found {
			continue
		}
		return constructionItemBuildingIconItem{
			ConstructionItemGroupID: identity.groupID,
			ConstructionItemName:    identity.name,
			BuildingID:              definition.ID,
			BuildingName:            definition.DisplayName,
			AssetName:               icon.AssetName,
			URL:                     icon.URL,
		}, true
	}
	return constructionItemBuildingIconItem{}, false
}

func preferConstructionItemBuilding(candidate GameData.BuildingDefinition, current GameData.BuildingDefinition) bool {
	candidateRank := constructionItemBuildingPreviewLevelRank(candidate.Level)
	currentRank := constructionItemBuildingPreviewLevelRank(current.Level)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}
	return candidate.ID < current.ID
}

func constructionItemBuildingPreviewLevelRank(level int64) int64 {
	if level == 1 {
		return 0
	}
	if level > 1 {
		return level
	}
	return 1 << 30
}
