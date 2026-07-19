package API

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
)

func (server *Server) handleBuildingPreview(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "Castle state is unavailable")
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input Buildings.PreviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := Buildings.Preview(server.config.State.Snapshot(), gameData, input)
	if err != nil {
		var mismatch Buildings.RevisionMismatchError
		if errors.As(err, &mismatch) {
			writeError(writer, http.StatusConflict, "state_revision_mismatch", err.Error())
			return
		}
		writeError(writer, http.StatusUnprocessableEntity, "building_preview_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleExpansionPreview(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "Castle state is unavailable")
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input Buildings.ExpansionPreviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := Buildings.PreviewExpansion(server.config.State.Snapshot(), gameData, input)
	if err != nil {
		var mismatch Buildings.RevisionMismatchError
		if errors.As(err, &mismatch) {
			writeError(writer, http.StatusConflict, "state_revision_mismatch", err.Error())
			return
		}
		writeError(writer, http.StatusUnprocessableEntity, "expansion_preview_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleBuildingTargetDiff(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "Castle state is unavailable")
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input Buildings.TargetDiffRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := Buildings.CompileTargetDiff(server.config.State.Snapshot(), gameData, input)
	if err != nil {
		var mismatch Buildings.RevisionMismatchError
		if errors.As(err, &mismatch) {
			writeError(writer, http.StatusConflict, "state_revision_mismatch", err.Error())
			return
		}
		writeError(writer, http.StatusUnprocessableEntity, "building_target_diff_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleBuildingTargetCapture(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "Castle state is unavailable")
		return
	}
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	var input Buildings.TargetCaptureRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := Buildings.CaptureTarget(server.config.State.Snapshot(), gameData, input)
	if err != nil {
		var mismatch Buildings.RevisionMismatchError
		if errors.As(err, &mismatch) {
			writeError(writer, http.StatusConflict, "state_revision_mismatch", err.Error())
			return
		}
		writeError(writer, http.StatusUnprocessableEntity, "building_target_capture_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleBuildingCatalog(writer http.ResponseWriter, request *http.Request) {
	gameData, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "building_catalog_unavailable", err.Error())
		return
	}
	filter, err := parseBuildingCatalogFilter(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	definitions := catalog.Definitions()
	filtered := make([]GameData.BuildingDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if filter.matches(definition) {
			filtered = append(filtered, definition)
		}
	}
	total := len(filtered)
	if filter.offset > total {
		filter.offset = total
	}
	end := filter.offset + filter.limit
	if end > total {
		end = total
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"metadata": gameData.Metadata(),
		"total":    total,
		"offset":   filter.offset,
		"limit":    filter.limit,
		"items":    filtered[filter.offset:end],
	})
}

type buildingCatalogFilter struct {
	ids        map[int64]struct{}
	kingdomID  *int64
	eventID    *int64
	areaTypeID *int64
	mapID      *int64
	maxLevel   *int64
	rootOnly   bool
	query      string
	offset     int
	limit      int
}

func parseBuildingCatalogFilter(request *http.Request) (buildingCatalogFilter, error) {
	query := request.URL.Query()
	filter := buildingCatalogFilter{ids: map[int64]struct{}{}, query: strings.ToLower(strings.TrimSpace(query.Get("q")))}
	for _, raw := range query["id"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return buildingCatalogFilter{}, fmt.Errorf("invalid building id %q", part)
			}
			filter.ids[id] = struct{}{}
		}
	}
	var err error
	if filter.kingdomID, err = optionalBuildingQueryInt(query.Get("kingdomId")); err != nil {
		return buildingCatalogFilter{}, err
	}
	if filter.eventID, err = optionalBuildingQueryInt(query.Get("eventId")); err != nil {
		return buildingCatalogFilter{}, err
	}
	if filter.areaTypeID, err = optionalBuildingQueryInt(query.Get("areaTypeId")); err != nil {
		return buildingCatalogFilter{}, err
	}
	if filter.mapID, err = optionalBuildingQueryInt(query.Get("mapId")); err != nil {
		return buildingCatalogFilter{}, err
	}
	if filter.maxLevel, err = optionalBuildingQueryInt(query.Get("maxLevel")); err != nil {
		return buildingCatalogFilter{}, err
	}
	filter.rootOnly = query.Get("rootOnly") == "1" || strings.EqualFold(query.Get("rootOnly"), "true")
	filter.offset, err = nonNegativeBuildingQueryInt(query.Get("offset"), 0)
	if err != nil {
		return buildingCatalogFilter{}, err
	}
	filter.limit, err = nonNegativeBuildingQueryInt(query.Get("limit"), 250)
	if err != nil {
		return buildingCatalogFilter{}, err
	}
	if filter.limit < 1 {
		filter.limit = 250
	}
	if filter.limit > 1000 {
		filter.limit = 1000
	}
	return filter, nil
}

func (filter buildingCatalogFilter) matches(definition GameData.BuildingDefinition) bool {
	if len(filter.ids) > 0 {
		if _, found := filter.ids[definition.ID]; !found {
			return false
		}
	}
	if filter.kingdomID != nil && len(definition.KingdomIDs) > 0 && !buildingIntListContains(definition.KingdomIDs, *filter.kingdomID) {
		return false
	}
	if filter.eventID != nil && !buildingIntListContains(definition.EventIDs, *filter.eventID) {
		return false
	}
	if filter.areaTypeID != nil && len(definition.AreaTypeIDs) > 0 && !buildingIntListContains(definition.AreaTypeIDs, *filter.areaTypeID) {
		return false
	}
	if filter.mapID != nil && !buildingIntListContains(definition.MapIDs, *filter.mapID) {
		return false
	}
	if filter.maxLevel != nil && definition.Level > *filter.maxLevel {
		return false
	}
	if filter.rootOnly && (definition.DowngradeDefinitionID != 0 || !strings.EqualFold(definition.Group, "Building")) {
		return false
	}
	if filter.query != "" {
		haystack := strings.ToLower(fmt.Sprintf("%d %s %s %s %s", definition.ID, definition.InternalName, definition.DisplayName, definition.Type, definition.Group))
		if !strings.Contains(haystack, filter.query) {
			return false
		}
	}
	return true
}

func optionalBuildingQueryInt(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer %q", raw)
	}
	return &value, nil
}

func nonNegativeBuildingQueryInt(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid non-negative integer %q", raw)
	}
	return value, nil
}

func buildingIntListContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
