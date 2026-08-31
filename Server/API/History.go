package API

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
)

const maximumPlayerTrackerResponsePoints = 20_000

func (server *Server) handlePlayerTrackerHistory(writer http.ResponseWriter, request *http.Request) {
	if server.config.History == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "history_unavailable", "Player history is unavailable")
		return
	}
	policy := server.playerSamplesRetentionPolicyWithStorage()
	since := playerTrackerHistorySince(time.Now().UTC(), request.URL.Query().Get("rangeSeconds"), policy.Effective)
	recordingIntervalSeconds := policy.RecordingIntervalSeconds
	servedIntervalSeconds := recordingIntervalSeconds
	current := History.NewPlayerSample(server.config.State.ReadOnlyView(), server.config.GameData)
	samples := []History.PlayerSample{}
	if policy.Effective != History.PlayerSamplesRetentionNone {
		var err error
		samples, servedIntervalSeconds, err = server.config.History.PlayerSamplesForPlayerBounded(
			since,
			maximumPlayerTrackerResponsePoints,
			current.PlayerID,
			recordingIntervalSeconds,
		)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "history_read_failed", err.Error())
			return
		}
	}
	samples = History.NormalizePlayerSamplesTroops(samples, server.config.GameData)
	if current.PlayerID == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{
			"current": nil, "samples": samples, "series": map[string]any{},
			"intervalSeconds": servedIntervalSeconds, "recordingIntervalSeconds": recordingIntervalSeconds,
			"resampled": servedIntervalSeconds > recordingIntervalSeconds,
			"fallback":  map[string]any{"provider": "citadel-history", "status": "not-needed"},
			"coverage":  map[string]bool{"loot": false, "eventScores": false},
		})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"current": current, "samples": samples, "series": map[string]any{},
		"intervalSeconds": servedIntervalSeconds, "recordingIntervalSeconds": recordingIntervalSeconds,
		"resampled": servedIntervalSeconds > recordingIntervalSeconds,
		"fallback":  map[string]any{"provider": "citadel-history", "status": "not-needed"},
		"coverage":  map[string]bool{"loot": false, "eventScores": false},
	})
}

func (server *Server) handlePlayerTrackerRetention(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, server.playerSamplesRetentionPolicyWithStorage())
}

type playerSamplesRetentionApplyResponse struct {
	Policy History.PlayerSamplesRetentionPolicy `json:"policy"`
	Report History.PlayerSamplesRetentionReport `json:"report"`
}

type playerSamplesRetentionApplyRequest struct {
	Retention                *History.PlayerSamplesRetention `json:"retention"`
	RecordingIntervalSeconds *int64                          `json:"recordingIntervalSeconds"`
	ExpectedRevision         *uint64                         `json:"expectedRevision"`
}

// handlePlayerTrackerRetentionApply is the single durable update boundary for
// My Stats retention. It serializes competing choices, persists the requested
// value with optimistic concurrency, and only acknowledges after the matching
// effective policy has been applied to PlayerSamples on disk.
func (server *Server) handlePlayerTrackerRetentionApply(writer http.ResponseWriter, request *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	if server.config.History == nil {
		writeError(writer, http.StatusServiceUnavailable, "history_unavailable", "Player history is unavailable")
		return
	}
	var input playerSamplesRetentionApplyRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("retention update must contain exactly one JSON object")
		}
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.ExpectedRevision == nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", "expectedRevision is required")
		return
	}
	if input.Retention == nil && input.RecordingIntervalSeconds == nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", "retention or recordingIntervalSeconds is required")
		return
	}
	if input.Retention != nil && !History.ValidPlayerSamplesRetention(*input.Retention) {
		writeError(writer, http.StatusUnprocessableEntity, "history_retention_invalid", "Unknown My Stats retention value")
		return
	}
	if input.RecordingIntervalSeconds != nil &&
		!History.ValidPlayerSamplesRecordingIntervalSeconds(*input.RecordingIntervalSeconds) {
		writeError(writer, http.StatusUnprocessableEntity, "history_recording_interval_invalid", "Unknown My Stats recording interval")
		return
	}

	server.playerHistoryRetentionMu.Lock()
	defer server.playerHistoryRetentionMu.Unlock()
	requestedSnapshot := server.config.Configuration.Snapshot()
	requestedPolicy := playerSamplesRetentionPolicyForSnapshot(requestedSnapshot, server.config.BackgroundOnly)
	requestedRetention := requestedPolicy.Configured
	if input.Retention != nil {
		requestedRetention = *input.Retention
	}
	requestedIntervalSeconds := requestedPolicy.RecordingIntervalSeconds
	if input.RecordingIntervalSeconds != nil {
		requestedIntervalSeconds = *input.RecordingIntervalSeconds
	}
	rawConfiguration, err := json.Marshal(History.PlayerSamplesConfiguration{
		Version:                  1,
		Retention:                requestedRetention,
		RecordingIntervalSeconds: requestedIntervalSeconds,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "history_retention_apply_failed", err.Error())
		return
	}
	_, err = server.config.Configuration.UpdateConditional(
		History.PlayerSamplesConfigurationSection,
		rawConfiguration,
		input.ExpectedRevision,
		nil,
	)
	if err != nil {
		if strings.Contains(err.Error(), "configuration revision changed") {
			writeError(writer, http.StatusConflict, "history_retention_conflict", err.Error())
			return
		}
		writeError(writer, http.StatusInternalServerError, "history_retention_update_failed", err.Error())
		return
	}
	var report History.PlayerSamplesRetentionReport
	var currentSnapshot Configuration.Snapshot
	var currentPolicy History.PlayerSamplesRetentionPolicy
	for attempt := 0; attempt < 3; attempt++ {
		var ran bool
		report, ran, err = server.config.History.CompactPlayerSamplesIfDueResolvedPolicy(
			time.Now().UTC(),
			func() History.PlayerSamplesStoragePolicy {
				snapshot := server.config.Configuration.Snapshot()
				return History.PlayerSamplesStoragePolicyFromRetentionPolicy(
					playerSamplesRetentionPolicyForSnapshot(snapshot, server.config.BackgroundOnly),
				)
			},
		)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "history_retention_apply_failed", err.Error())
			return
		}
		// Derive both response fields from one exact snapshot. If a non-HTTP
		// writer changed the policy during compaction, retry before acknowledging.
		currentSnapshot = server.config.Configuration.Snapshot()
		currentPolicy = playerSamplesRetentionPolicyForSnapshot(currentSnapshot, server.config.BackgroundOnly)
		if !ran {
			observedAt := time.Now().UTC()
			report = History.PlayerSamplesRetentionReport{
				Retention:                currentPolicy.Effective,
				RecordingIntervalSeconds: currentPolicy.RecordingIntervalSeconds,
				StartedAt:                observedAt,
				FinishedAt:               observedAt,
				Complete:                 true,
			}
		}
		if currentPolicy.Configured != requestedRetention ||
			currentPolicy.RecordingIntervalSeconds != requestedIntervalSeconds {
			_, _ = server.config.History.CompactPlayerSamplesWithResolvedPolicy(
				time.Now().UTC(),
				func() History.PlayerSamplesStoragePolicy {
					snapshot := server.config.Configuration.Snapshot()
					return History.PlayerSamplesStoragePolicyFromRetentionPolicy(
						playerSamplesRetentionPolicyForSnapshot(snapshot, server.config.BackgroundOnly),
					)
				},
			)
			writeError(writer, http.StatusConflict, "history_retention_conflict", "My Stats storage policy changed while it was being applied")
			return
		}
		if report.Retention == currentPolicy.Effective &&
			report.RecordingIntervalSeconds == currentPolicy.RecordingIntervalSeconds {
			break
		}
		if attempt == 2 {
			writeError(writer, http.StatusConflict, "history_retention_conflict", "My Stats retention did not remain stable while it was being applied")
			return
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	currentPolicy = server.enrichPlayerSamplesRetentionPolicy(currentPolicy)
	writeJSON(writer, http.StatusOK, playerSamplesRetentionApplyResponse{
		Policy: currentPolicy,
		Report: report,
	})
}

func (server *Server) playerSamplesRetentionPolicy() History.PlayerSamplesRetentionPolicy {
	if server == nil || server.config.Configuration == nil {
		return History.ResolvePlayerSamplesRetention(nil, server != nil && server.config.BackgroundOnly)
	}
	return playerSamplesRetentionPolicyForSnapshot(server.config.Configuration.Snapshot(), server.config.BackgroundOnly)
}

func (server *Server) playerSamplesRetentionPolicyWithStorage() History.PlayerSamplesRetentionPolicy {
	return server.enrichPlayerSamplesRetentionPolicy(server.playerSamplesRetentionPolicy())
}

func (server *Server) enrichPlayerSamplesRetentionPolicy(
	policy History.PlayerSamplesRetentionPolicy,
) History.PlayerSamplesRetentionPolicy {
	if server == nil || server.config.History == nil {
		return policy
	}
	current := History.PlayerSample{}
	if server.config.State != nil {
		current = History.NewPlayerSample(server.config.State.ReadOnlyView(), server.config.GameData)
	}
	estimate, _ := server.config.History.EstimatePlayerSamplesStorage(current)
	policy.Storage = estimate
	return policy
}

func playerSamplesRetentionPolicyForSnapshot(
	snapshot Configuration.Snapshot,
	hosted bool,
) History.PlayerSamplesRetentionPolicy {
	policy := History.ResolvePlayerSamplesRetention(
		snapshot.Sections[History.PlayerSamplesConfigurationSection],
		hosted,
	)
	policy.Revision = snapshot.Revision
	return policy
}

func playerTrackerHistorySince(
	now time.Time,
	rawRange string,
	effective History.PlayerSamplesRetention,
) time.Time {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	effective = History.NormalizePlayerSamplesRetention(effective)
	effectiveDuration, effectiveUnlimited := History.PlayerSamplesRetentionDuration(effective)
	if strings.EqualFold(strings.TrimSpace(rawRange), "all") {
		if effectiveUnlimited {
			return time.Time{}
		}
		return now.Add(-effectiveDuration)
	}

	requestedSeconds := int64(30 * 24 * 60 * 60)
	if parsed, err := strconv.ParseInt(strings.TrimSpace(rawRange), 10, 64); err == nil && parsed > 0 {
		const maximumRangeSeconds = int64(1<<63-1) / int64(time.Second)
		requestedSeconds = min(parsed, maximumRangeSeconds)
	}
	requestedDuration := time.Duration(requestedSeconds) * time.Second
	if !effectiveUnlimited && requestedDuration > effectiveDuration {
		requestedDuration = effectiveDuration
	}
	return now.Add(-requestedDuration)
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
