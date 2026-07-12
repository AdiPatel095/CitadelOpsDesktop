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
	AllianceTargets *AllianceTargets.Service
	Updates         *AppUpdate.Manager
	Diagnostics     *Diagnostics.Monitor
	Session         interface{ Status() Session.Status }
}

type Server struct {
	config   Config
	upgrader websocket.Upgrader
}

func NewServer(config Config) *Server {
	if config.AllianceTargets == nil {
		config.AllianceTargets = AllianceTargets.NewService(nil)
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
	mux.HandleFunc("GET /api/v2/config/{section}", server.handleConfigurationSection)
	mux.HandleFunc("GET /api/v2/game-data", server.handleGameDataManifest)
	mux.HandleFunc("GET /api/v2/game-data/{collection}", server.handleGameDataCollection)
	mux.HandleFunc("POST /api/v2/game-data/localize", server.handleLocalization)
	mux.HandleFunc("GET /api/v2/projections/crafting", server.handleCraftingProjection)
	mux.HandleFunc("POST /api/v2/equipment/optimize", server.handleEquipmentOptimize)
	mux.HandleFunc("GET /api/v2/alliance-targets", server.handleAllianceTargets)
	mux.HandleFunc("GET /api/v2/history/player-tracker", server.handlePlayerTrackerHistory)
	mux.HandleFunc("GET /api/v2/history/spy-reports", server.handleSpyReportHistory)
	mux.HandleFunc("GET /api/v2/history/battle-reports", server.handleBattleReportHistory)
	mux.HandleFunc("GET /api/v2/telemetry/channels", server.handleTelemetryChannels)
	mux.HandleFunc("GET /api/v2/telemetry/{channel}", server.handleTelemetryTail)
	mux.HandleFunc("GET /api/v2/intents", server.handleIntentDefinitions)
	mux.HandleFunc("POST /api/v2/intents/{name}", server.handleIntentSubmit)
	mux.HandleFunc("GET /api/v2/operations/{id}", server.handleOperation)
	mux.HandleFunc("GET /api/v2/events", server.handleEvents)
	return mux
}

func (server *Server) handleBrowsers(writer http.ResponseWriter, _ *http.Request) {
	var selected any
	if server.config.Session != nil {
		status := server.config.Session.Status()
		if status.BrowserID != "" {
			selected = map[string]string{"id": status.BrowserID, "name": status.BrowserName}
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"selected":        selected,
		"available":       Session.DiscoverChromiumBrowsers(),
		"selectionIntent": "session.select_browser",
	})
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
	}
	if server.config.Configuration != nil {
		response["configurationRevision"] = server.config.Configuration.Snapshot().Revision
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
	writeJSON(writer, http.StatusOK, server.config.State.Snapshot())
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
	go readEnvelopes(connection, incoming, readErrors)

	if err := connection.WriteJSON(newEnvelope("", "state.snapshot", server.config.State.Revision(), server.config.State.Snapshot())); err != nil {
		return
	}
	if server.config.Configuration != nil {
		if err := connection.WriteJSON(newEnvelope("", "config.changed", server.config.State.Revision(), server.config.Configuration.Snapshot())); err != nil {
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
			if err := connection.WriteJSON(newEnvelope("", "state.changed", event.Revision, event)); err != nil {
				return
			}
		case receipt := <-operationEvents:
			if err := connection.WriteJSON(newEnvelope(receipt.ID, "operation.changed", server.config.State.Revision(), receipt)); err != nil {
				return
			}
		case event := <-configurationEvents:
			if err := connection.WriteJSON(newEnvelope("", "config.changed", server.config.State.Revision(), event.Snapshot)); err != nil {
				return
			}
		case response := <-responses:
			if err := connection.WriteJSON(response); err != nil {
				return
			}
		case message := <-incoming:
			switch message.Type {
			case "query.state":
				responses <- newEnvelope(message.ID, "state.snapshot", server.config.State.Revision(), server.config.State.Snapshot())
			case "query.catalogs":
				if store, ready := server.config.GameData.Current(); ready {
					responses <- newEnvelope(message.ID, "catalog.changed", server.config.State.Revision(), map[string]any{
						"metadata": store.Metadata(), "catalogs": store.Summaries(),
					})
				} else {
					responses <- errorEnvelope(message.ID, "game_data_unavailable", "Official game data is unavailable")
				}
			case "query.config":
				if server.config.Configuration != nil {
					responses <- newEnvelope(message.ID, "config.changed", server.config.State.Revision(), server.config.Configuration.Snapshot())
				} else {
					responses <- errorEnvelope(message.ID, "configuration_unavailable", "Configuration store is unavailable")
				}
			case "intent.submit":
				var intentRequest Intent.Request
				if err := json.Unmarshal(message.Payload, &intentRequest); err != nil {
					responses <- errorEnvelope(message.ID, "invalid_request", err.Error())
					continue
				}
				if intentRequest.ID == "" {
					intentRequest.ID = message.ID
				}
				go func() {
					receipt := server.config.Intents.Submit(ctx, intentRequest)
					responses <- newEnvelope(message.ID, "intent.receipt", server.config.State.Revision(), receipt)
				}()
			default:
				responses <- errorEnvelope(message.ID, "unsupported_message", "Unsupported websocket message type")
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

func readEnvelopes(connection *websocket.Conn, output chan<- Envelope, errors chan<- error) {
	for {
		var envelope Envelope
		if err := connection.ReadJSON(&envelope); err != nil {
			errors <- err
			return
		}
		if envelope.Version != ContractVersion {
			errors <- fmt.Errorf("unsupported API contract version %d", envelope.Version)
			return
		}
		output <- envelope
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
