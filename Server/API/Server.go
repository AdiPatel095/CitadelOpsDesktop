package API

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AllianceTargets"
	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Diagnostics"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
	"github.com/gorilla/websocket"
)

type Config struct {
	Version         string
	State           *State.Store
	GameData        *GameData.Manager
	Configuration   *Configuration.Store
	History         *History.Store
	Telemetry       *Telemetry.Store
	Intents         *Intent.Engine
	ReportAnalytics *Reports.SQLiteStore
	CloudReports    *Reports.CloudClient
	AllianceTargets *AllianceTargets.Service
	Updates         *AppUpdate.Manager
	Diagnostics     *Diagnostics.Monitor
	Session         interface{ Status() Session.Status }
	Persistence     interface{ PersistenceError() error }
}

type Server struct {
	config   Config
	upgrader websocket.Upgrader
}

func NewServer(config Config) *Server {
	if config.AllianceTargets == nil {
		config.AllianceTargets = AllianceTargets.NewService(nil, config.History)
	}
	server := &Server{config: config}
	server.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     server.originAllowed,
	}
	return server
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/health", server.handleHealth)
	mux.HandleFunc("GET /api/v2/update", server.handleApplicationUpdate)
	mux.HandleFunc("GET /api/v2/diagnostics", server.handleDiagnostics)
	mux.HandleFunc("GET /api/v2/state", server.handleState)
	mux.HandleFunc("GET /api/v2/browsers", server.handleBrowsers)
	mux.HandleFunc("GET /api/v2/config", server.handleConfiguration)
	mux.HandleFunc("GET /api/v2/config/export", server.handleConfigurationExport)
	mux.HandleFunc("POST /api/v2/config/import", server.handleConfigurationImport)
	mux.HandleFunc("GET /api/v2/config/{section}", server.handleConfigurationSection)
	mux.HandleFunc("GET /api/v2/game-data", server.handleGameDataManifest)
	mux.HandleFunc("GET /api/v2/game-data/currency-icons", server.handleCurrencyIcons)
	mux.HandleFunc("GET /api/v2/game-data/construction-item-icons", server.handleConstructionItemIcons)
	mux.HandleFunc("GET /api/v2/game-data/construction-item-building-icons", server.handleConstructionItemBuildingIcons)
	mux.HandleFunc("GET /api/v2/game-data/{collection}", server.handleGameDataCollection)
	mux.HandleFunc("POST /api/v2/game-data/localize", server.handleLocalization)
	mux.HandleFunc("GET /api/v2/projections/crafting", server.handleCraftingProjection)
	mux.HandleFunc("POST /api/v2/equipment/optimize", server.handleEquipmentOptimize)
	mux.HandleFunc("GET /api/v2/buildings/catalog", server.handleBuildingCatalog)
	mux.HandleFunc("POST /api/v2/buildings/preview", server.handleBuildingPreview)
	mux.HandleFunc("POST /api/v2/buildings/target/capture", server.handleBuildingTargetCapture)
	mux.HandleFunc("POST /api/v2/buildings/target/diff", server.handleBuildingTargetDiff)
	mux.HandleFunc("POST /api/v2/buildings/expansion/preview", server.handleExpansionPreview)
	mux.HandleFunc("GET /api/v2/alliance-targets", server.handleAllianceTargets)
	mux.HandleFunc("POST /api/v2/alliance-targets/attack-preview", server.handleAllianceTargetAttackPreview)
	mux.HandleFunc("GET /api/v2/history/player-tracker", server.handlePlayerTrackerHistory)
	mux.HandleFunc("GET /api/v2/history/spy-reports", server.handleSpyReportHistory)
	mux.HandleFunc("GET /api/v2/history/battle-reports/cloud", server.handleCloudBattleReportHistory)
	mux.HandleFunc("GET /api/v2/history/battle-reports", server.handleBattleReportHistory)
	mux.HandleFunc("GET /api/v2/analytics/battle-reports", server.handleBattleReportAnalytics)
	mux.HandleFunc("GET /api/v2/telemetry/channels", server.handleTelemetryChannels)
	mux.HandleFunc("GET /api/v2/telemetry/{channel}", server.handleTelemetryTail)
	mux.HandleFunc("GET /api/v2/intents", server.handleIntentDefinitions)
	mux.HandleFunc("POST /api/v2/intents/{name}", server.handleIntentSubmit)
	mux.HandleFunc("GET /api/v2/operations", server.handleOperations)
	mux.HandleFunc("GET /api/v2/operations/{id}", server.handleOperation)
	mux.HandleFunc("POST /api/v2/operations/{id}/cancel", server.handleOperationCancel)
	mux.HandleFunc("GET /api/v2/events", server.handleEvents)
	return mux
}

func (server *Server) handleBrowsers(writer http.ResponseWriter, _ *http.Request) {
	if provider, ok := server.config.Session.(interface {
		BrowserInventory() Session.BrowserInventory
	}); ok {
		writeJSON(writer, http.StatusOK, provider.BrowserInventory())
		return
	}
	available := Session.DiscoverChromiumBrowsers()
	var current Session.BrowserCandidate
	if server.config.Session != nil {
		status := server.config.Session.Status()
		current = Session.BrowserCandidate{ID: status.BrowserID, Name: status.BrowserName}
	}
	writeJSON(writer, http.StatusOK, Session.BrowserInventory{
		Selected:        browserCandidatePointer(current),
		Current:         browserCandidatePointer(current),
		Available:       available,
		SelectionIntent: "session.select_browser",
	})
}

func browserCandidatePointer(candidate Session.BrowserCandidate) *Session.BrowserCandidate {
	if candidate.ID == "" {
		return nil
	}
	return &candidate
}

func (server *Server) handleConfiguration(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Configuration.Snapshot())
}

func (server *Server) handleConfigurationSection(writer http.ResponseWriter, request *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	section := request.PathValue("section")
	value, ok := server.config.Configuration.Section(section)
	if !ok {
		writeError(writer, http.StatusNotFound, "configuration_section_not_found", fmt.Sprintf("Configuration section %q was not found", section))
		return
	}
	snapshot := server.config.Configuration.Snapshot()
	writeJSON(writer, http.StatusOK, map[string]any{
		"schemaVersion": snapshot.SchemaVersion,
		"revision":      snapshot.Revision,
		"updatedAt":     snapshot.UpdatedAt,
		"section":       section,
		"value":         value,
	})
}

func (server *Server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	response := map[string]any{
		"status":  "ok",
		"version": server.config.Version,
		"api":     ContractVersion,
	}
	if server.config.State != nil {
		response["revision"] = server.config.State.Revision()
	}
	if server.config.Session != nil {
		response["session"] = server.config.Session.Status()
		if provider, ok := server.config.Session.(interface {
			DispatchLatency() Outbound.DispatchLatencyStats
		}); ok {
			response["dispatchLatency"] = provider.DispatchLatency()
		}
	}
	if server.config.Configuration != nil {
		response["configurationRevision"] = server.config.Configuration.Snapshot().Revision
	}
	if server.config.Persistence != nil {
		if err := server.config.Persistence.PersistenceError(); err != nil {
			response["status"] = "degraded"
			response["persistenceError"] = err.Error()
		}
	}
	if server.config.GameData == nil {
		response["status"] = "degraded"
		response["gameDataReady"] = false
	} else if store, ready := server.config.GameData.Current(); ready {
		response["gameDataReady"] = true
		response["gameData"] = store.Metadata()
		if err := server.config.GameData.LastError(); err != nil {
			response["status"] = "degraded"
			response["gameDataWarning"] = err.Error()
		}
	} else {
		response["status"] = "degraded"
		response["gameDataReady"] = false
		if err := server.config.GameData.LastError(); err != nil {
			response["gameDataError"] = err.Error()
		}
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleState(writer http.ResponseWriter, _ *http.Request) {
	if server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "state_unavailable", "State store is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.State.ReadOnlyView())
}

func (server *Server) handleGameDataManifest(writer http.ResponseWriter, _ *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	response := map[string]any{
		"metadata": store.Metadata(),
		"catalogs": store.Summaries(),
	}
	if language, ready := server.config.GameData.Language(); ready {
		response["language"] = language.Metadata()
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleGameDataCollection(writer http.ResponseWriter, request *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	name := request.PathValue("collection")
	catalog, err := store.Catalog(name)
	if err != nil {
		writeError(writer, http.StatusNotFound, "catalog_not_found", err.Error())
		return
	}
	if id := strings.TrimSpace(request.URL.Query().Get("id")); id != "" {
		item, found := catalog.Find(id)
		if !found {
			writeError(writer, http.StatusNotFound, "item_not_found", fmt.Sprintf("No %s item has id %s", name, id))
			return
		}
		writeJSON(writer, http.StatusOK, struct {
			Metadata GameData.SourceMetadata `json:"metadata"`
			Catalog  GameData.CatalogSummary `json:"catalog"`
			Item     json.RawMessage         `json:"item"`
		}{store.Metadata(), catalog.Summary(), item})
		return
	}
	raw, _ := store.RawCollection(name)
	writeJSON(writer, http.StatusOK, struct {
		Metadata GameData.SourceMetadata `json:"metadata"`
		Catalog  GameData.CatalogSummary `json:"catalog"`
		Items    json.RawMessage         `json:"items"`
	}{store.Metadata(), catalog.Summary(), raw})
}

func (server *Server) handleIntentDefinitions(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Intents == nil {
		writeError(writer, http.StatusServiceUnavailable, "intents_unavailable", "Intent engine is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.Intents.Registry().Definitions())
}

func (server *Server) handleLocalization(writer http.ResponseWriter, request *http.Request) {
	if server.config.GameData == nil {
		writeError(writer, http.StatusServiceUnavailable, "game_data_unavailable", "Official game data is unavailable")
		return
	}
	language, ready := server.config.GameData.Language()
	if !ready {
		writeError(writer, http.StatusServiceUnavailable, "language_unavailable", "Official language data is unavailable")
		return
	}
	var input struct {
		Keys []string `json:"keys"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(input.Keys) > 5000 {
		writeError(writer, http.StatusRequestEntityTooLarge, "too_many_keys", "At most 5000 language keys may be resolved at once")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"metadata": language.Metadata(),
		"values":   language.ResolveMany(input.Keys),
	})
}

func (server *Server) handleIntentSubmit(writer http.ResponseWriter, request *http.Request) {
	if server.config.Intents == nil {
		writeError(writer, http.StatusServiceUnavailable, "intents_unavailable", "Intent engine is unavailable")
		return
	}
	var intentRequest Intent.Request
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intentRequest); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	intentRequest.Name = request.PathValue("name")
	intentRequest.Actor = "ui"
	intentRequest.Priority = Outbound.PriorityInteractive
	receipt := server.config.Intents.Submit(request.Context(), intentRequest)
	status := http.StatusOK
	if receipt.Status == Intent.StatusFailed {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(writer, status, receipt)
}

func (server *Server) handleOperation(writer http.ResponseWriter, request *http.Request) {
	if server.config.Intents == nil {
		writeError(writer, http.StatusServiceUnavailable, "intents_unavailable", "Intent engine is unavailable")
		return
	}
	receipt, ok := server.config.Intents.Operation(request.PathValue("id"))
	if !ok {
		writeError(writer, http.StatusNotFound, "operation_not_found", "Operation was not found")
		return
	}
	writeJSON(writer, http.StatusOK, receipt)
}

func (server *Server) handleOperations(writer http.ResponseWriter, request *http.Request) {
	if server.config.Intents == nil {
		writeError(writer, http.StatusServiceUnavailable, "intents_unavailable", "Intent engine is unavailable")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(writer, http.StatusBadRequest, "invalid_limit", "Operation limit must be between 1 and 1000")
			return
		}
		limit = parsed
	}
	receipts, err := server.config.Intents.RecentOperations(request.Context(), limit)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "operations_unavailable", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, receipts)
}

func (server *Server) handleOperationCancel(writer http.ResponseWriter, request *http.Request) {
	if server.config.Intents == nil {
		writeError(writer, http.StatusServiceUnavailable, "intents_unavailable", "Intent engine is unavailable")
		return
	}
	id := request.PathValue("id")
	if !server.config.Intents.Cancel(id) {
		writeError(writer, http.StatusConflict, "operation_not_running", "Operation is not currently running")
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{"id": id, "cancelled": true})
}

func (server *Server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	connection, err := server.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	connection.SetReadLimit(1 << 20)
	_ = connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	stateEvents, cancelState := server.config.State.Subscribe(64)
	defer cancelState()
	operationEvents, cancelOperations := server.config.Intents.Subscribe(64)
	defer cancelOperations()
	var configurationEvents <-chan Configuration.Event
	cancelConfiguration := func() {}
	if server.config.Configuration != nil {
		configurationEvents, cancelConfiguration = server.config.Configuration.Subscribe(16)
	}
	defer cancelConfiguration()
	incoming := make(chan Envelope, 8)
	readErrors := make(chan error, 1)
	responses := make(chan Envelope, 8)
	go readEnvelopes(ctx, connection, incoming, readErrors)

	initialState := server.config.State.ReadOnlyView()
	initialRevision := initialState.Revision
	if err := connection.WriteJSON(streamEnvelope("", "state.snapshot", initialRevision, initialRevision, false, initialState)); err != nil {
		return
	}
	if server.config.Configuration != nil {
		snapshot := server.config.Configuration.Snapshot()
		if err := connection.WriteJSON(streamEnvelope("", "config.changed", server.config.State.Revision(), snapshot.Revision, false, snapshot)); err != nil {
			return
		}
	}
	if receipts, err := server.config.Intents.RecentOperations(ctx, 100); err == nil {
		if err := connection.WriteJSON(newEnvelope("", "operations.snapshot", server.config.State.Revision(), receipts)); err != nil {
			return
		}
	}
	if store, ready := server.config.GameData.Current(); ready {
		if err := connection.WriteJSON(newEnvelope("", "catalog.changed", server.config.State.Revision(), map[string]any{
			"metadata": store.Metadata(), "catalogs": store.Summaries(),
		})); err != nil {
			return
		}
	}

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-readErrors:
			return
		case <-ping.C:
			if err := connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case event := <-stateEvents:
			if err := connection.WriteJSON(streamEnvelope("", "state.changed", event.Revision, event.Sequence, event.Gap, event)); err != nil {
				return
			}
		case receipt := <-operationEvents:
			if err := connection.WriteJSON(streamEnvelope(
				receipt.ID, "operation.changed", server.config.State.Revision(),
				receipt.StreamSequence, receipt.StreamGap, receipt,
			)); err != nil {
				return
			}
		case event := <-configurationEvents:
			if err := connection.WriteJSON(streamEnvelope(
				"", "config.changed", server.config.State.Revision(), event.Sequence, event.Gap, event.Snapshot,
			)); err != nil {
				return
			}
		case response := <-responses:
			if err := connection.WriteJSON(response); err != nil {
				return
			}
		case message := <-incoming:
			switch message.Type {
			case "query.state":
				state := server.config.State.ReadOnlyView()
				revision := state.Revision
				if err := connection.WriteJSON(newEnvelope(message.ID, "state.snapshot", revision, state)); err != nil {
					return
				}
			case "query.catalogs":
				if store, ready := server.config.GameData.Current(); ready {
					if err := connection.WriteJSON(newEnvelope(message.ID, "catalog.changed", server.config.State.Revision(), map[string]any{
						"metadata": store.Metadata(), "catalogs": store.Summaries(),
					})); err != nil {
						return
					}
				} else {
					if err := connection.WriteJSON(errorEnvelope(message.ID, "game_data_unavailable", "Official game data is unavailable")); err != nil {
						return
					}
				}
			case "query.config":
				if server.config.Configuration != nil {
					if err := connection.WriteJSON(newEnvelope(message.ID, "config.changed", server.config.State.Revision(), server.config.Configuration.Snapshot())); err != nil {
						return
					}
				} else {
					if err := connection.WriteJSON(errorEnvelope(message.ID, "configuration_unavailable", "Configuration store is unavailable")); err != nil {
						return
					}
				}
			case "intent.submit":
				var intentRequest Intent.Request
				if err := json.Unmarshal(message.Payload, &intentRequest); err != nil {
					if writeErr := connection.WriteJSON(errorEnvelope(message.ID, "invalid_request", err.Error())); writeErr != nil {
						return
					}
					continue
				}
				if intentRequest.ID == "" {
					intentRequest.ID = message.ID
				}
				intentRequest.Actor = "ui"
				intentRequest.Priority = Outbound.PriorityInteractive
				go func() {
					receipt := server.config.Intents.Submit(ctx, intentRequest)
					select {
					case responses <- newEnvelope(message.ID, "intent.receipt", server.config.State.Revision(), receipt):
					case <-ctx.Done():
					}
				}()
			default:
				if err := connection.WriteJSON(errorEnvelope(message.ID, "unsupported_message", "Unsupported websocket message type")); err != nil {
					return
				}
			}
		}
	}
}

func (server *Server) currentGameData(writer http.ResponseWriter) (*GameData.Store, bool) {
	if server.config.GameData == nil {
		writeError(writer, http.StatusServiceUnavailable, "game_data_unavailable", "Official game data is unavailable")
		return nil, false
	}
	store, ok := server.config.GameData.Current()
	if !ok {
		writeError(writer, http.StatusServiceUnavailable, "game_data_unavailable", "Official game data is unavailable")
		return nil, false
	}
	return store, true
}

func (server *Server) originAllowed(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

func readEnvelopes(ctx context.Context, connection *websocket.Conn, output chan<- Envelope, errors chan<- error) {
	reportError := func(err error) {
		select {
		case errors <- err:
		case <-ctx.Done():
		}
	}
	for {
		var envelope Envelope
		if err := connection.ReadJSON(&envelope); err != nil {
			reportError(err)
			return
		}
		if envelope.Version != ContractVersion {
			reportError(fmt.Errorf("unsupported API contract version %d", envelope.Version))
			return
		}
		select {
		case output <- envelope:
		case <-ctx.Done():
			return
		}
	}
}

func errorEnvelope(id string, code string, message string) Envelope {
	return newEnvelope(id, "error", 0, map[string]string{"code": code, "message": message})
}

func writeError(writer http.ResponseWriter, status int, code string, message string) {
	writeJSON(writer, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
