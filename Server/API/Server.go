package API

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	"CitadelDesktop/Server/WorldIntel"
	"github.com/gorilla/websocket"
)

type Config struct {
	Version         string
	BuildRevision   string
	BuildID         string
	State           *State.Store
	GameData        *GameData.Manager
	Configuration   *Configuration.Store
	History         *History.Store
	Telemetry       *Telemetry.Store
	Intents         *Intent.Engine
	ReportAnalytics *Reports.SQLiteStore
	CloudReports    *Reports.CloudClient
	BattleResearch  *Reports.BattleResearchManager
	AllianceTargets *AllianceTargets.Service
	Updates         *AppUpdate.Manager
	Diagnostics     *Diagnostics.Monitor
	Session         interface{ Status() Session.Status }
	BackgroundLogin *Session.BackgroundLoginStore
	// BackgroundOnly identifies the hosted worker composition. Its login
	// credential is installed only by the authenticated orchestrator control
	// plane, never by the runtime's public dashboard API.
	BackgroundOnly bool
	Persistence    interface{ PersistenceError() error }
	WorldIntel     *WorldIntel.DesktopService
}

type Server struct {
	config                         Config
	externalConfigurationAuthority atomic.Bool
	playerHistoryRetentionMu       sync.Mutex
	upgrader                       websocket.Upgrader
}

// SetExternalConfigurationAuthority makes the hosted account control plane
// the only writer for portable configuration. Installation-scoped retention
// and battle-research consent remain local by design.
func (server *Server) SetExternalConfigurationAuthority(enabled bool) {
	if server != nil {
		server.externalConfigurationAuthority.Store(enabled)
	}
}

func NewServer(config Config) *Server {
	if config.AllianceTargets == nil {
		config.AllianceTargets = AllianceTargets.NewService(config.WorldIntel, config.History)
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
	mux.HandleFunc("GET /api/v2/session/background-login", server.handleBackgroundLoginStatus)
	mux.HandleFunc("GET /api/v2/session/game-servers", server.handleGameServers)
	mux.HandleFunc("POST /api/v2/session/background-login", server.handleBackgroundLoginConfigure)
	mux.HandleFunc("GET /api/v2/config", server.handleConfiguration)
	mux.HandleFunc("GET /api/v2/config/export", server.handleConfigurationExport)
	mux.HandleFunc("POST /api/v2/config/import", server.handleConfigurationImport)
	mux.HandleFunc("GET /api/v2/config/{section}", server.handleConfigurationSection)
	mux.HandleFunc("PUT /api/v2/config/{section}", server.handleConfigurationUpdate)
	mux.HandleFunc("GET /api/v2/game-data", server.handleGameDataManifest)
	mux.HandleFunc("GET /api/v2/game-data/currency-icons", server.handleCurrencyIcons)
	mux.HandleFunc("GET /api/v2/game-data/construction-item-icons", server.handleConstructionItemIcons)
	mux.HandleFunc("GET /api/v2/game-data/construction-item-building-icons", server.handleConstructionItemBuildingIcons)
	mux.HandleFunc("GET /api/v2/game-data/{collection}", server.handleGameDataCollection)
	mux.HandleFunc("POST /api/v2/game-data/localize", server.handleLocalization)
	mux.HandleFunc("GET /api/v2/projections/crafting", server.handleCraftingProjection)
	mux.HandleFunc("GET /api/v2/projections/auto-buyer", server.handleAutoBuyerProjection)
	mux.HandleFunc("POST /api/v2/equipment/optimize", server.handleEquipmentOptimize)
	mux.HandleFunc("GET /api/v2/buildings/catalog", server.handleBuildingCatalog)
	mux.HandleFunc("POST /api/v2/buildings/preview", server.handleBuildingPreview)
	mux.HandleFunc("POST /api/v2/buildings/target/capture", server.handleBuildingTargetCapture)
	mux.HandleFunc("POST /api/v2/buildings/target/diff", server.handleBuildingTargetDiff)
	mux.HandleFunc("POST /api/v2/buildings/blueprint/diff", server.handleBuildingBlueprintDiff)
	mux.HandleFunc("POST /api/v2/buildings/expansion/preview", server.handleExpansionPreview)
	mux.HandleFunc("POST /api/v2/automations/auto-storm/troop-cap-preview", server.handleAutoStormTroopCapPreview)
	mux.HandleFunc("GET /api/v2/alliance-targets", server.handleAllianceTargets)
	mux.HandleFunc("POST /api/v2/alliance-targets/attack-preview", server.handleAllianceTargetAttackPreview)
	mux.HandleFunc("GET /api/v2/world-intelligence/status", server.handleWorldIntelligenceStatus)
	mux.HandleFunc("GET /api/v2/world-intelligence/search", server.handleWorldIntelligenceSearch)
	mux.HandleFunc("GET /api/v2/world-intelligence/players/{id}", server.handleWorldIntelligencePlayer)
	mux.HandleFunc("GET /api/v2/world-intelligence/players/{id}/event-scores", server.handleWorldIntelligencePlayerEventScores)
	mux.HandleFunc("GET /api/v2/world-intelligence/alliances/{id}", server.handleWorldIntelligenceAlliance)
	mux.HandleFunc("GET /api/v2/world-intelligence/event-runs", server.handleWorldIntelligenceEventRuns)
	mux.HandleFunc("GET /api/v2/world-intelligence/event-runs/{id}/rankings", server.handleWorldIntelligenceEventRunRankings)
	mux.HandleFunc("GET /api/v2/world-intelligence/event-runs/{id}/subscribe", server.handleWorldIntelligenceEventRunSubscribe)
	mux.HandleFunc("GET /api/v2/world-intelligence/ranking-metrics/{type}", server.handleWorldIntelligenceRankingMetrics)
	mux.HandleFunc("GET /api/v2/world-intelligence/rankings/{type}", server.handleWorldIntelligenceRankings)
	mux.HandleFunc("GET /api/v2/world-intelligence/coverage", server.handleWorldIntelligenceCoverage)
	mux.HandleFunc("GET /api/v2/world-intelligence/subscribe", server.handleWorldIntelligenceSubscribe)
	mux.HandleFunc("GET /api/v2/world-intelligence/catalog-datasets", server.handleWorldIntelligenceCatalogDatasets)
	mux.HandleFunc("GET /api/v2/world-intelligence/catalog-datasets/{key}", server.handleWorldIntelligenceCatalogDataset)
	mux.HandleFunc("GET /api/v2/history/player-tracker", server.handlePlayerTrackerHistory)
	mux.HandleFunc("GET /api/v2/history/player-tracker/retention", server.handlePlayerTrackerRetention)
	mux.HandleFunc("POST /api/v2/history/player-tracker/retention/apply", server.handlePlayerTrackerRetentionApply)
	mux.HandleFunc("GET /api/v2/history/spy-reports", server.handleSpyReportHistory)
	mux.HandleFunc("GET /api/v2/history/battle-reports/cloud", server.handleCloudBattleReportHistory)
	mux.HandleFunc("GET /api/v2/history/battle-reports", server.handleBattleReportHistory)
	mux.HandleFunc("GET /api/v2/analytics/battle-reports", server.handleBattleReportAnalytics)
	mux.HandleFunc("GET /api/v2/analytics/resource-aggregates", server.handleResourceAggregates)
	mux.HandleFunc("GET /api/v2/battle-research", server.handleBattleResearchStatus)
	mux.HandleFunc("GET /api/v2/telemetry/channels", server.handleTelemetryChannels)
	mux.HandleFunc("GET /api/v2/telemetry/attack-rates", server.handleAttackLaunchRates)
	mux.HandleFunc("GET /api/v2/telemetry/{channel}", server.handleTelemetryTail)
	mux.HandleFunc("GET /api/v2/intents", server.handleIntentDefinitions)
	mux.HandleFunc("POST /api/v2/intents/{name}", server.handleIntentSubmit)
	mux.HandleFunc("GET /api/v2/operations", server.handleOperations)
	mux.HandleFunc("GET /api/v2/operations/{id}", server.handleOperation)
	mux.HandleFunc("POST /api/v2/operations/{id}/cancel", server.handleOperationCancel)
	mux.HandleFunc("GET /api/v2/events", server.handleEvents)
	return mux
}

func (server *Server) handleBattleResearchStatus(writer http.ResponseWriter, _ *http.Request) {
	if server.config.BattleResearch == nil {
		writeError(writer, http.StatusServiceUnavailable, "battle_research_unavailable", "Battle research beta is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, server.config.BattleResearch.Status())
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

// handleGameServers lists the selectable game worlds — code, label, secure
// websocket URL and SmartFox zone — so login forms offer the official
// directory instead of a free-text code.
func (server *Server) handleGameServers(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, Session.GameServers())
}

func (server *Server) handleBackgroundLoginStatus(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if server.config.BackgroundLogin == nil {
		writeError(writer, http.StatusServiceUnavailable, "background_login_unavailable", "Background login storage is unavailable")
		return
	}
	status, err := server.config.BackgroundLogin.Status()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "background_login_unavailable", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (server *Server) handleBackgroundLoginConfigure(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if server.config.BackgroundOnly {
		writeError(
			writer,
			http.StatusForbidden,
			"background_login_managed",
			"Background login is managed by the hosted account control plane",
		)
		return
	}
	if !server.originAllowed(request) {
		writeError(writer, http.StatusForbidden, "origin_not_allowed", "Background login can only be configured from the local CitadelOps application")
		return
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Type"))), "application/json") {
		writeError(writer, http.StatusUnsupportedMediaType, "content_type_not_supported", "Background login requires an application/json request")
		return
	}
	if server.config.BackgroundLogin == nil {
		writeError(writer, http.StatusServiceUnavailable, "background_login_unavailable", "Background login storage is unavailable")
		return
	}
	var input Session.BackgroundLoginInput
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(writer, http.StatusBadRequest, "invalid_request", "Background login requires exactly one JSON object")
		return
	}
	status, err := server.config.BackgroundLogin.Configure(input)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "background_login_invalid", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (server *Server) handleConfiguration(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
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
	writer.Header().Set("Cache-Control", "no-store")
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
		"status":        "ok",
		"version":       server.config.Version,
		"buildRevision": server.config.BuildRevision,
		"buildId":       server.config.BuildID,
		"api":           ContractVersion,
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
	writeJSON(writer, http.StatusOK, State.NewClientStateSnapshot(server.config.State.ReadOnlyView()))
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
	// The dashboard is a control panel, not the runtime: execution is detached
	// from this request so a closed tab, a sleeping laptop, or a gateway
	// timeout never cancels an operation. Completion flows through the
	// operation stream; `?wait=true` keeps synchronous semantics for callers
	// that want the final receipt in the response, still without coupling the
	// operation's lifetime to the connection.
	receipt := server.config.Intents.SubmitDetached(intentRequest)
	if !receipt.Terminal() && waitRequested(request) {
		awaited, err := server.config.Intents.Await(request.Context(), receipt.ID)
		if err == nil || awaited.ID != "" {
			receipt = awaited
		}
	}
	status := http.StatusAccepted
	switch {
	case receipt.Terminal() && receipt.Status == Intent.StatusFailed:
		status = http.StatusUnprocessableEntity
	case receipt.Terminal():
		status = http.StatusOK
	}
	writeJSON(writer, status, receipt)
}

func waitRequested(request *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(request.URL.Query().Get("wait"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
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
	if err := connection.WriteJSON(streamEnvelope(
		"", "state.snapshot", initialRevision, initialRevision, false, State.NewClientStateSnapshot(initialState),
	)); err != nil {
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
			payload, err := State.ClientEventPayload(event)
			if err != nil {
				return
			}
			if err := connection.WriteJSON(streamEnvelopeRaw("", "state.changed", event.Revision, event.Sequence, event.Gap, payload)); err != nil {
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
				if err := connection.WriteJSON(newEnvelope(
					message.ID, "state.snapshot", revision, State.NewClientStateSnapshot(state),
				)); err != nil {
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
				// Detached like the HTTP path: the socket receives the accepted
				// receipt now and every later change through operation.changed,
				// and closing the socket never cancels the operation.
				go func() {
					receipt := server.config.Intents.SubmitDetached(intentRequest)
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
