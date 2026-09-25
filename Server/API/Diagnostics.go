package API

import (
	"CitadelDesktop/Server/Localization"
	"net/http"
)

func (server *Server) handleDiagnostics(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Diagnostics == nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable", "Runtime diagnostics are unavailable", Localization.New("server.api.runtime_diagnostics_are_unavailable.1b27e807", "Runtime diagnostics are unavailable", nil))
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Diagnostics.Snapshot())
}
