package API

import (
	"crypto/sha256"
	"encoding/hex"
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

func (server *Server) handleWorldIntelligenceEventRuns(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	eventKey := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("eventKey")))
	if eventKey != "" && !validWorldIntelligenceEventKey(eventKey) {
		writeError(writer, http.StatusBadRequest, "invalid_event_key", "eventKey is invalid")
		return
	}
	result, err := server.config.WorldIntel.EventRuns(
		request.Context(), request.URL.Query().Get("worldId"), eventKey, queryLimit(request, 50, 250),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligenceEventRunRankings(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	occurrenceID := strings.ToLower(strings.TrimSpace(request.PathValue("id")))
	if !validWorldIntelligenceOccurrenceID(occurrenceID) {
		writeError(writer, http.StatusBadRequest, "invalid_occurrence_id", "event occurrence id is invalid")
		return
	}
	listType, err := optionalIntegerQuery(request, "listType", 0, 0, 1_000_000)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_list_type", err.Error())
		return
	}
	leagueID, err := optionalIntegerQuery(request, "leagueId", -2, -1, 1_000_000)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_league_id", err.Error())
		return
	}
	result, err := server.config.WorldIntel.EventRunRankings(
		request.Context(), request.URL.Query().Get("worldId"), occurrenceID,
		listType, leagueID, queryLimit(request, 250, 5_000),
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "world_intelligence_query_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleWorldIntelligencePlayerEventScores(writer http.ResponseWriter, request *http.Request) {
	if server.config.WorldIntel == nil {
		writeError(writer, http.StatusServiceUnavailable, "world_intelligence_unavailable", "World Intelligence is unavailable")
		return
	}
	playerID, err := positivePathID(request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_player_id", "player id must be positive")
		return
	}
	eventKey := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("eventKey")))
	if eventKey != "" && !validWorldIntelligenceEventKey(eventKey) {
		writeError(writer, http.StatusBadRequest, "invalid_event_key", "eventKey is invalid")
		return
	}
	occurrenceID := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("occurrenceId")))
	if occurrenceID != "" && !validWorldIntelligenceOccurrenceID(occurrenceID) {
		writeError(writer, http.StatusBadRequest, "invalid_occurrence_id", "event occurrence id is invalid")
		return
	}
	result, err := server.config.WorldIntel.PlayerEventScores(
		request.Context(), request.URL.Query().Get("worldId"), playerID,
		eventKey, occurrenceID, queryLimit(request, 1_000, 5_000),
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

func optionalIntegerQuery(request *http.Request, name string, fallback int64, minimum int64, maximum int64) (int64, error) {
	raw := strings.TrimSpace(request.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func validWorldIntelligenceEventKey(value string) bool {
	if value == "" || len(value) > 80 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func validWorldIntelligenceOccurrenceID(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
