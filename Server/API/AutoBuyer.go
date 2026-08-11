package API

import (
	"net/http"

	"CitadelDesktop/Server/GameData"
)

type autoBuyerProjection struct {
	Metadata GameData.SourceMetadata `json:"metadata"`
	GameData.AutoBuyerCatalog
}

func (server *Server) handleAutoBuyerProjection(writer http.ResponseWriter, _ *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	catalog, err := store.AutoBuyerCatalog()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "auto_buyer_unavailable", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, autoBuyerProjection{
		Metadata: store.Metadata(), AutoBuyerCatalog: catalog,
	})
}
