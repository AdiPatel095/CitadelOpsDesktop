package API

import "net/http"

func (server *Server) handleDiagnostics(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Diagnostics == nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable", "Runtime diagnostics are unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Diagnostics.Snapshot())
}
