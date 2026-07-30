package API

import (
	"encoding/json"
	"net/http"

	"CitadelDesktop/Server/Equipment"
)

func (server *Server) handleEquipmentOptimize(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "Equipment state is unavailable")
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input Equipment.OptimizeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := Equipment.Optimize(server.config.State.Snapshot(), gameData, input)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "equipment_optimization_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}
