package API

import "net/http"

func (server *Server) handleApplicationUpdate(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Updates == nil {
		writeError(writer, http.StatusServiceUnavailable, "update_unavailable", "Application update service is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Updates.Snapshot())
}
