package API

import (
	"net/http"
	"strconv"
	"time"

	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func (server *Server) handleTelemetryChannels(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"channels": server.config.Telemetry.Channels()})
}

func (server *Server) handleAttackLaunchRates(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable")
		return
	}
	const window = time.Hour
	observedAt := time.Now()
	counts := server.config.Telemetry.AttackLaunchCounts(observedAt)
	writeJSON(writer, http.StatusOK, map[string]any{
		"observedAt":    observedAt.UTC(),
		"windowMinutes": int(window / time.Minute),
		"launchesByFeature": map[string]int{
			string(State.AttackFeatureAutoTowers):    counts[Telemetry.ChannelAutoTowers],
			string(State.AttackFeatureAutoInvasion):  counts[Telemetry.ChannelAutoInvasion],
			string(State.AttackFeatureAutoNomad):     counts[Telemetry.ChannelAutoNomad],
			string(State.AttackFeatureAutoAdvisor):   counts[Telemetry.ChannelAutoAdvisor],
			string(State.AttackFeatureAutoKhan):      counts[Telemetry.ChannelAutoKhan],
			string(State.AttackFeatureAutoBeriWorld): counts[Telemetry.ChannelAutoBeriWorld],
			string(State.AttackFeatureAutoStorm):     counts[Telemetry.ChannelAutoStorm],
		},
	})
}

func (server *Server) handleTelemetryTail(writer http.ResponseWriter, request *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable")
		return
	}
	limit := 800
	if raw := request.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 5000)
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"lines": server.config.Telemetry.Tail(request.PathValue("channel"), limit),
	})
}
