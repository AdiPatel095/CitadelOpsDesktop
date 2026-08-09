package API

import (
	"net/http"
	"strconv"
	"strings"
)

func (server *Server) handleWorldIntelligenceStatus(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.WorldIntel.Status(request.Context()))
}

func (server *Server) handleWorldIntelligenceSearch(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	entityType := strings.TrimSpace(request.URL.Query().Get("type"))
	if entityType != "" && entityType != "player" && entityType != "alliance" {
		writeError(writer, http.StatusBadRequest, "invalid_entity_type", "type must be player or alliance")
		return
	}
	result, err := server.config.WorldIntel.Search(
		request.Context(), request.URL.Query().Get("worldId"), request.URL.Query().Get("q"),
		entityType, queryLimit(request, 50, 100),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligencePlayer(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	playerID, err := positivePathID(request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_player_id", "player id must be positive")
		return
	}
	result, err := server.config.WorldIntel.Player(
		request.Context(), request.URL.Query().Get("worldId"), playerID, queryLimit(request, 365, 1_000),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceAlliance(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	allianceID, err := positivePathID(request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_alliance_id", "alliance id must be positive")
		return
	}
	result, err := server.config.WorldIntel.Alliance(
		request.Context(), request.URL.Query().Get("worldId"), allianceID, queryLimit(request, 365, 1_000),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceRankings(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	entityType := strings.TrimSpace(request.PathValue("type"))
	if entityType != "players" && entityType != "alliances" {
		writeError(writer, http.StatusBadRequest, "invalid_ranking_type", "ranking type must be players or alliances")
		return
	}
	metric := strings.TrimSpace(request.URL.Query().Get("metric"))
	result, err := server.config.WorldIntel.Rankings(
		request.Context(), request.URL.Query().Get("worldId"), entityType, metric,
		queryLimit(request, 100, 250),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceRankingMetrics(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	entityType := strings.TrimSpace(request.PathValue("type"))
	if entityType != "players" && entityType != "alliances" {
		writeError(writer, http.StatusBadRequest, "invalid_ranking_type", "ranking metric type must be players or alliances")
		return
	}
	result, err := server.config.WorldIntel.RankingMetrics(
		request.Context(), request.URL.Query().Get("worldId"), entityType,
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceCoverage(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	result, err := server.config.WorldIntel.Coverage(request.Context(), request.URL.Query().Get("worldId"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceCatalogDatasets(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	result, err := server.config.WorldIntel.CatalogDatasets(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceCatalogDataset(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	datasetKey := strings.TrimSpace(request.PathValue("key"))
	if datasetKey == "" || len(datasetKey) > 80 {
		writeError(writer, http.StatusBadRequest, "invalid_dataset_key", "catalog dataset key is invalid")
		return
	}
	result, err := server.config.WorldIntel.CatalogDataset(
		request.Context(), datasetKey, queryLimitNamed(request, "historyLimit", 25, 100),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func positivePathID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func queryLimit(request *http.Request, fallback int, maximum int) int {
	return queryLimitNamed(request, "limit", fallback, maximum)
}

func queryLimitNamed(request *http.Request, name string, fallback int, maximum int) int {
	value, err := strconv.Atoi(request.URL.Query().Get(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return min(value, maximum)
}
