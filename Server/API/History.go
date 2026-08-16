package API

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
)

func (server *Server) handlePlayerTrackerHistory(writer http.ResponseWriter, request *http.Request) {
	if server.config.History == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "history_unavailable", "Player history is unavailable")
		return
	}
	rangeSeconds := int64(365 * 24 * 60 * 60)
	if raw := request.URL.Query().Get("rangeSeconds"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			rangeSeconds = min(parsed, 365*24*60*60)
		}
	}
	since := time.Now().UTC().Add(-time.Duration(rangeSeconds) * time.Second)
	current := History.NewPlayerSample(server.config.State.ReadOnlyView(), server.config.GameData)
	samples, err := server.config.History.PlayerSamplesForPlayer(since, 100_000, current.PlayerID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "history_read_failed", err.Error())
		return
	}
	samples = History.NormalizePlayerSamplesTroops(samples, server.config.GameData)
	if current.PlayerID == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{
			"current": nil, "samples": samples, "series": map[string]any{}, "intervalSeconds": 60,
			"fallback": map[string]any{"provider": "citadel-history", "status": "not-needed"},
			"coverage": map[string]bool{"loot": false, "eventScores": false},
		})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"current": current, "samples": samples, "series": map[string]any{}, "intervalSeconds": 60,
		"fallback": map[string]any{"provider": "citadel-history", "status": "not-needed"},
		"coverage": map[string]bool{"loot": false, "eventScores": false},
	})
}

func (server *Server) handleSpyReportHistory(writer http.ResponseWriter, request *http.Request) {
	server.handleRawHistory(writer, request, History.CollectionSpyReports, false)
}

func (server *Server) handleBattleReportHistory(writer http.ResponseWriter, request *http.Request) {
	server.handleRawHistory(writer, request, History.CollectionBattleReports, true)
}

func (server *Server) handleCloudBattleReportHistory(writer http.ResponseWriter, request *http.Request) {
	if server.config.CloudReports == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "cloud_reports_unavailable", "Cloud battle reports are unavailable")
		return
	}
	snapshot := server.config.State.ReadOnlyView()
	allianceID := int64(snapshot.Player.AllianceID)
	if allianceID <= 0 {
		allianceID = int64(snapshot.Alliance.ID)
	}
	query := Reports.CloudBattleReportQuery{
		AllianceID: allianceID,
		PlayerID:   int64(snapshot.Player.ID),
		Limit:      historyLimit(request, 5000),
	}
	reports, err := server.config.CloudReports.FetchReports(request.Context(), query, snapshot.Player.ID)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "cloud_reports_fetch_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"reports": reports})
}

func (server *Server) handleBattleReportAnalytics(writer http.ResponseWriter, request *http.Request) {
	if server.config.ReportAnalytics == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "report_analytics_unavailable", "Battle report analytics are unavailable")
		return
	}
	limit := historyLimit(request, 2000)
	eventID, _ := strconv.ParseInt(request.URL.Query().Get("eventId"), 10, 64)
	snapshot := server.config.State.ReadOnlyView()
	reports, err := server.config.ReportAnalytics.Recent(request.Context(), Reports.BattleReportQuery{
		AccountUID: snapshot.Account.UID,
		WorldID:    snapshot.Account.WorldID,
		PlayerID:   int64(snapshot.Player.ID),
		FeatureID:  request.URL.Query().Get("feature"),
		EventID:    eventID,
		Limit:      limit,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "report_analytics_read_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"reports": reports})
}

func (server *Server) handleRawHistory(writer http.ResponseWriter, request *http.Request, collection string, wrapped bool) {
	if server.config.History == nil {
		writeError(writer, http.StatusServiceUnavailable, "history_unavailable", "Report history is unavailable")
		return
	}
	limit := historyLimit(request, 2000)
	rows, err := server.config.History.Read(collection, time.Time{}, limit)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "history_read_failed", err.Error())
		return
	}
	items := make([]json.RawMessage, len(rows))
	copy(items, rows)
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if wrapped {
		writeJSON(writer, http.StatusOK, map[string]any{"reports": items})
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func historyLimit(request *http.Request, fallback int) int {
	limit := fallback
	if raw := request.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 10_000)
		}
	}
	return limit
}
