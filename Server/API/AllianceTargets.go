package API

import (
	"context"
	"net/http"
	"time"

	"CitadelDesktop/Server/GameData"
)

func (server *Server) handleAllianceTargets(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil || server.config.AllianceTargets == nil {
		writeError(writer, http.StatusServiceUnavailable, "alliance_targets_unavailable", "Alliance target state is unavailable")
		return
	}
	var gameData *GameData.Store
	if server.config.GameData != nil {
		gameData, _ = server.config.GameData.Current()
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	view, err := server.config.AllianceTargets.View(
		ctx,
		server.config.State.Snapshot(),
		gameData,
		request.URL.Query().Get("server"),
		request.URL.Query().Get("allianceId"),
		request.URL.Query().Get("refresh") == "1" || request.URL.Query().Get("refresh") == "true",
	)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "alliance_targets_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, view)
}
