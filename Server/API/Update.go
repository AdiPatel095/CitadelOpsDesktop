package API

import (
	"CitadelDesktop/Server/Localization"
	"net/http"
)

func (server *Server) handleApplicationUpdate(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Updates == nil {
		writeError(writer, http.StatusServiceUnavailable, "update_unavailable", "Application update service is unavailable", Localization.New("server.api.application_update_service_is.e3a45426", "Application update service is unavailable", nil))
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Updates.Snapshot())
}
