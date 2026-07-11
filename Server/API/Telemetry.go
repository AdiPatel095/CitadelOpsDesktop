package API

import (
	"net/http"
	"strconv"
)

func (server *Server) handleTelemetryChannels(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"channels": server.config.Telemetry.Channels()})
}

func (server *Server) handleTelemetryTail(writer http.ResponseWriter, request *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable")
		return
	}
	limit := 800
	if raw := request.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 5000)
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"lines": server.config.Telemetry.Tail(request.PathValue("channel"), limit),
	})
}
