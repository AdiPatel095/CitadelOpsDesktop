package API

import (
	"net/http"
	"sort"

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
