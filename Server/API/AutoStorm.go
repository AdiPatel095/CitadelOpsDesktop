package API

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"net/http"
	"time"

	"CitadelDesktop/Server/Automation"
)

type autoStormTroopCapPreviewRequest struct {
	Settings json.RawMessage `json:"settings"`
}

func (server *Server) handleAutoStormTroopCapPreview(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil || server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "auto_storm_preview_unavailable", "Auto Storm preview state is unavailable", Localization.New("server.api.auto_storm_preview_state.5d110df7", "Auto Storm preview state is unavailable", nil))
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input autoStormTroopCapPreviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeErrorFromError(writer, http.StatusBadRequest, "invalid_request", err)
		return
	}
	result, err := Automation.PreviewAutoStormTroopCap(
		server.config.State.ReadOnlyView(),
		server.config.Configuration.Snapshot(),
		gameData,
		server.config.Telemetry,
		input.Settings,
		time.Now().UTC(),
	)
	if err != nil {
		writeErrorFromError(writer, http.StatusUnprocessableEntity, "auto_storm_preview_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}
