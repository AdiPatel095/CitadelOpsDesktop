package API

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"CitadelDesktop/Server/AllianceTargets"
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
	values := request.URL.Query()
	page, _ := strconv.Atoi(values.Get("page"))
	query := AllianceTargets.Query{
		Search: values.Get("search"), Status: values.Get("status"),
		Sort: values.Get("sort"), Direction: values.Get("direction"), Page: page,
		IncludeAlliances: values.Get("includeAlliances") != "0" && values.Get("includeAlliances") != "false",
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	view, err := server.config.AllianceTargets.View(
		ctx,
		server.config.State.Snapshot(),
		gameData,
		values.Get("server"),
		values.Get("allianceId"),
		values.Get("refresh") == "1" || values.Get("refresh") == "true",
		query,
	)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "alliance_targets_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, view)
}
