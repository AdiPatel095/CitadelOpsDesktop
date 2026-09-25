package API

import (
	"CitadelDesktop/Server/Localization"
	"net/http"
	"strconv"
	"time"

	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func (server *Server) handleTelemetryChannels(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable", Localization.New("server.api.telemetry_is_unavailable.3daba4e0", "Telemetry is unavailable", nil))
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"channels": server.config.Telemetry.Channels()})
}

func (server *Server) handleAttackLaunchRates(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable", Localization.New("server.api.telemetry_is_unavailable.3daba4e0", "Telemetry is unavailable", nil))
		return
	}
	observedAt := time.Now()
	hourlyCounts := server.config.Telemetry.AttackLaunchCounts(observedAt)
	var dailySession *attackLaunchDailySession
	if server.config.State != nil {
		startedAt := server.config.State.ReadOnlyView().DailyAttacks.SessionStartedAt
		if dailyCounts, available := server.config.Telemetry.AttackLaunchCountsSince(startedAt, observedAt); available {
			dailySession = &attackLaunchDailySession{
				StartedAt:         startedAt.UTC(),
				LaunchesByFeature: attackLaunchCountsByFeature(dailyCounts),
			}
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"observedAt":        observedAt.UTC(),
		"windowMinutes":     int(time.Hour / time.Minute),
		"launchesByFeature": attackLaunchCountsByFeature(hourlyCounts),
		"dailySession":      dailySession,
	})
}

type attackLaunchDailySession struct {
	StartedAt         time.Time      `json:"startedAt"`
	LaunchesByFeature map[string]int `json:"launchesByFeature"`
}

func attackLaunchCountsByFeature(counts map[string]int) map[string]int {
	return map[string]int{
		string(State.AttackFeatureAutoTowers):    counts[Telemetry.ChannelAutoTowers],
		string(State.AttackFeatureAutoFortress):  counts[Telemetry.ChannelAutoFortress],
		string(State.AttackFeatureAutoInvasion):  counts[Telemetry.ChannelAutoInvasion],
		string(State.AttackFeatureAutoNomad):     counts[Telemetry.ChannelAutoNomad],
		string(State.AttackFeatureAutoAdvisor):   counts[Telemetry.ChannelAutoAdvisor],
		string(State.AttackFeatureAutoKhan):      counts[Telemetry.ChannelAutoKhan],
		string(State.AttackFeatureAutoBeriWorld): counts[Telemetry.ChannelAutoBeriWorld],
		string(State.AttackFeatureAutoStorm):     counts[Telemetry.ChannelAutoStorm],
	}
}

func (server *Server) handleTelemetryTail(writer http.ResponseWriter, request *http.Request) {
	if server.config.Telemetry == nil {
		writeError(writer, http.StatusServiceUnavailable, "telemetry_unavailable", "Telemetry is unavailable", Localization.New("server.api.telemetry_is_unavailable.3daba4e0", "Telemetry is unavailable", nil))
		return
	}
	limit := 800
	if raw := request.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 5000)
		}
	}
	entries, available := server.config.Telemetry.UserFacingEntries(request.PathValue("channel"), limit)
	if !available {
		writeError(writer, http.StatusNotFound, "activity_channel_not_found", "Activity channel was not found", Localization.New("server.api.activity_channel_was_not.ff46c116", "Activity channel was not found", nil))
		return
	}
	lines := make([]string, len(entries))
	for i, entry := range entries {
		lines[i] = entry.Line
	}
	writeJSON(writer, http.StatusOK, map[string]any{"lines": lines, "entries": entries})
}
