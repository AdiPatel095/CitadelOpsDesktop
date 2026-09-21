package API

import (
	"math"
	"net/http"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type autoBuyerProjection struct {
	Metadata GameData.SourceMetadata `json:"metadata"`
	GameData.AutoBuyerCatalog
	Locale            *GameData.LocaleResolution `json:"locale,omitempty"`
	SpecialistRuntime autoBuyerSpecialistRuntime `json:"specialistRuntime"`
}

type autoBuyerSpecialistRuntime struct {
	TimersObservedAt     *time.Time `json:"timersObservedAt,omitempty"`
	TimersCurrentSession bool       `json:"timersCurrentSession"`
	RubyBalance          *int64     `json:"rubyBalance,omitempty"`
	RubyObservedAt       *time.Time `json:"rubyObservedAt,omitempty"`
	RubyCurrentSession   bool       `json:"rubyCurrentSession"`
}

func (server *Server) handleAutoBuyerProjection(writer http.ResponseWriter, request *http.Request) {
	store, ok := server.currentGameData(writer)
	if !ok {
		return
	}
	language, _ := server.config.GameData.Language()
	var locale *GameData.LocaleResolution
	if request.URL.Query().Has("locale") {
		var ready bool
		var resolution GameData.LocaleResolution
		language, resolution, ready = server.requestLanguage(writer, request)
		if !ready {
			return
		}
		locale = &resolution
	}
	catalog, err := store.LocalizedAutoBuyerCatalog(language)
	if err != nil {
		writeErrorFromError(writer, http.StatusServiceUnavailable, "auto_buyer_unavailable", err)
		return
	}
	projection := autoBuyerProjection{Metadata: store.Metadata(), AutoBuyerCatalog: catalog, Locale: locale}
	if server.config.State != nil {
		state := server.config.State.ReadOnlyView()
		if !state.Market.BoostersObservedAt.IsZero() {
			observedAt := state.Market.BoostersObservedAt
			projection.SpecialistRuntime.TimersObservedAt = &observedAt
			projection.SpecialistRuntime.TimersCurrentSession = state.Session.ConnectionGeneration > 0 &&
				state.Market.BoostersObservedGeneration == state.Session.ConnectionGeneration
		}
		if rubyResourceID, found := store.ResourceIDForJSONKey("C2"); found && rubyResourceID > 0 {
			resourceID := State.ResourceID(rubyResourceID)
			observation := state.Player.ResourceObservations[resourceID]
			if !observation.ObservedAt.IsZero() {
				observedAt := observation.ObservedAt
				balance := int64(math.Floor(state.Player.Resources[resourceID]))
				projection.SpecialistRuntime.RubyObservedAt = &observedAt
				projection.SpecialistRuntime.RubyBalance = &balance
				projection.SpecialistRuntime.RubyCurrentSession = state.Session.ConnectionGeneration > 0 &&
					observation.ConnectionGeneration == state.Session.ConnectionGeneration
			}
		}
	}
	writeJSON(writer, http.StatusOK, projection)
}
