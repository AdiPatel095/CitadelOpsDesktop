package Reports

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/State"
)

const (
	defaultCloudBackendURL = "https://citadelops.app/api"
	cloudReportBatchMax    = 25
	cloudReportListMax     = 5000
	cloudRequestTimeout    = 20 * time.Second
)

type CloudConfig struct {
	Client      *http.Client
	UploadURL   string
	FetchURL    string
	TrainingURL string
	UploadKey   string
}

type CloudBattleReportQuery struct {
	AllianceID int64
	PlayerID   int64
	Limit      int
}

type CloudClient struct {
	client      *http.Client
	uploadURL   string
	fetchURL    string
	trainingURL string
	uploadKey   string
}

type cloudBattleReportEnvelope struct {
	BattleDateUnixMillis int64  `json:"battleDateUnixMillis"`
	MID                  int64  `json:"mid"`
	LID                  int64  `json:"lid"`
	AttackerPlayerID     int64  `json:"attackerPlayerID"`
	AttackerAllianceID   int64  `json:"attackerAllianceID"`
	DefenderPlayerID     int64  `json:"defenderPlayerID"`
	DefenderAllianceID   int64  `json:"defenderAllianceID"`
	AttackWon            bool   `json:"attackWon"`
	RichnessScore        int    `json:"richnessScore"`
	Payload              string `json:"payload"`
}

type legacyCloudBattleCapture struct {
	Version              int             `json:"version"`
	ID                   string          `json:"id"`
	Source               string          `json:"source"`
	CapturedAtUnixMillis int64           `json:"capturedAtUnixMillis"`
	MID                  int64           `json:"mid"`
	LID                  int64           `json:"lid"`
	NoticeType           int             `json:"noticeType,omitempty"`
	BattleKey            string          `json:"battleKey,omitempty"`
	BLS                  json.RawMessage `json:"bls,omitempty"`
	BLM                  json.RawMessage `json:"blm,omitempty"`
	BLD                  json.RawMessage `json:"bld,omitempty"`
}

func NewCloudClient(config CloudConfig) *CloudClient {
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: cloudRequestTimeout}
	}
	uploadURL := strings.TrimSpace(config.UploadURL)
	if uploadURL == "" {
		uploadURL = cloudReportUploadURL()
	}
	fetchURL := strings.TrimSpace(config.FetchURL)
	if fetchURL == "" {
		fetchURL = cloudReportFetchURL()
	}
	trainingURL := strings.TrimSpace(config.TrainingURL)
	if trainingURL == "" {
		trainingURL = cloudBattleTrainingUploadURL()
	}
	uploadKey := strings.TrimSpace(config.UploadKey)
	if uploadKey == "" {
		uploadKey = reportUploadKey()
	}
	return &CloudClient{
		client: client, uploadURL: uploadURL, fetchURL: fetchURL, trainingURL: trainingURL, uploadKey: uploadKey,
	}
}

func (client *CloudClient) UploadBattleResearch(ctx context.Context, trial BattleResearchTrial) error {
	if client == nil || client.client == nil {
		return fmt.Errorf("cloud battle research client is unavailable")
	}
	if strings.TrimSpace(trial.ID) == "" || trial.Phase != "complete" || trial.PlayerID <= 0 ||
		trial.Movement == nil || trial.Movement.ID <= 0 || trial.PreSpy == nil || trial.Prediction == nil ||
		trial.Prediction.GeneratedAt.IsZero() || trial.Battle == nil || strings.TrimSpace(trial.Battle.Report.ID) == "" ||
		trial.PostSpy == nil || trial.CompletedAt.IsZero() || trial.PreSpy.CapturedAt.IsZero() ||
		trial.Battle.CapturedAt.IsZero() || trial.PostSpy.CapturedAt.IsZero() ||
		trial.Prediction.GeneratedAt.Before(trial.PreSpy.CapturedAt) ||
		trial.Movement.ArrivesAt == nil || !trial.Prediction.GeneratedAt.Before(*trial.Movement.ArrivesAt) ||
		trial.Battle.Report.DateMs <= 0 || trial.Prediction.GeneratedAt.UnixMilli() >= trial.Battle.Report.DateMs ||
		trial.Battle.CapturedAt.Before(trial.Prediction.GeneratedAt) ||
		trial.PostSpy.CapturedAt.Before(trial.Battle.CapturedAt) ||
		trial.PostSpy.CapturedAt.UnixMilli() != trial.CompletedAt.UnixMilli() {
		return fmt.Errorf("battle research trial is incomplete")
	}
	payload, err := json.Marshal(trial)
	if err != nil {
		return fmt.Errorf("encode battle research trial: %w", err)
	}
	envelope := struct {
		TrialID                 string          `json:"trialID"`
		BattleReportID          string          `json:"battleReportID"`
		MovementID              int64           `json:"movementID"`
		PlayerID                int64           `json:"playerID"`
		Status                  string          `json:"status"`
		CapturedAtUnixMillis    int64           `json:"capturedAtUnixMillis"`
		PredictionGeneratedAtMs int64           `json:"predictionGeneratedAtUnixMillis"`
		Payload                 json.RawMessage `json:"payload"`
	}{
		TrialID: trial.ID, BattleReportID: trial.Battle.Report.ID,
		MovementID: int64(trial.Movement.ID), PlayerID: int64(trial.PlayerID), Status: "complete",
		CapturedAtUnixMillis:    trial.CompletedAt.UnixMilli(),
		PredictionGeneratedAtMs: trial.Prediction.GeneratedAt.UnixMilli(), Payload: payload,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode battle research upload: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.trainingURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create battle research upload request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client.applyUploadKey(request)
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("upload battle research trial: %w", err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("upload battle research trial: %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (client *CloudClient) Upload(ctx context.Context, reports []cloudBattleReportEnvelope) error {
	if client == nil || client.client == nil {
		return fmt.Errorf("cloud battle report client is unavailable")
	}
	if len(reports) == 0 {
		return nil
	}
	payload, err := json.Marshal(reports)
	if err != nil {
		return fmt.Errorf("encode cloud battle report batch: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.uploadURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create cloud battle report request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client.applyUploadKey(request)
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("upload cloud battle reports: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return fmt.Errorf("upload cloud battle reports: %s: %s", response.Status, strings.TrimSpace(string(body)))
}

func (client *CloudClient) FetchReports(
	ctx context.Context,
	query CloudBattleReportQuery,
	ownPlayerID State.PlayerID,
) ([]BattleReport, error) {
	payloads, err := client.fetchPayloads(ctx, query)
	if err != nil {
		return nil, err
	}
	reports := make([]BattleReport, 0, len(payloads))
	for _, payload := range payloads {
		report, parsed := decodeCloudBattleReport(payload, ownPlayerID)
		if parsed && IsPvPBattleReport(report) {
			reports = append(reports, report)
		}
	}
	sort.SliceStable(reports, func(left, right int) bool {
		if reports[left].DateMs != reports[right].DateMs {
			return reports[left].DateMs > reports[right].DateMs
		}
		return reports[left].LID > reports[right].LID
	})
	return reports, nil
}

func (client *CloudClient) RemoteLIDs(ctx context.Context) (map[int64]struct{}, error) {
	payloads, err := client.fetchPayloads(ctx, CloudBattleReportQuery{Limit: cloudReportListMax})
	if err != nil {
		return nil, err
	}
	result := make(map[int64]struct{}, len(payloads))
	for _, payload := range payloads {
		if lid := cloudPayloadLID(payload); lid > 0 {
			result[lid] = struct{}{}
		}
	}
	return result, nil
}

func (client *CloudClient) fetchPayloads(ctx context.Context, query CloudBattleReportQuery) ([]string, error) {
	if client == nil || client.client == nil {
		return nil, fmt.Errorf("cloud battle report client is unavailable")
	}
	endpoint, err := url.Parse(client.fetchURL)
	if err != nil {
		return nil, fmt.Errorf("parse cloud battle report URL: %w", err)
	}
	values := endpoint.Query()
	if query.AllianceID > 0 {
		values.Set("allianceId", strconv.FormatInt(query.AllianceID, 10))
	} else if query.PlayerID > 0 {
		values.Set("playerId", strconv.FormatInt(query.PlayerID, 10))
	}
	limit := query.Limit
	if limit <= 0 || limit > cloudReportListMax {
		limit = cloudReportListMax
	}
	values.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create cloud battle report fetch: %w", err)
	}
	client.applyUploadKey(request)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch cloud battle reports: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("fetch cloud battle reports: %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Reports []json.RawMessage `json:"reports"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 128<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode cloud battle reports: %w", err)
	}
	result := make([]string, 0, len(payload.Reports))
	for _, raw := range payload.Reports {
		var encoded string
		if json.Unmarshal(raw, &encoded) == nil {
			encoded = strings.TrimSpace(encoded)
			if encoded != "" {
				result = append(result, encoded)
			}
			continue
		}
		var wrapper struct {
			Payload string `json:"payload"`
		}
		if json.Unmarshal(raw, &wrapper) == nil && strings.TrimSpace(wrapper.Payload) != "" {
			result = append(result, strings.TrimSpace(wrapper.Payload))
			continue
		}
		if json.Valid(raw) {
			result = append(result, string(raw))
		}
	}
	return result, nil
}

func (client *CloudClient) applyUploadKey(request *http.Request) {
	if client != nil && request != nil && client.uploadKey != "" {
		request.Header.Set("X-Citadel-Report-Key", client.uploadKey)
	}
}

func buildCloudEnvelopeFromCapture(
	report BattleReport,
	capture State.BattleReportCapture,
) (cloudBattleReportEnvelope, bool, error) {
	if !IsPvPBattleReport(report) {
		return cloudBattleReportEnvelope{}, false, nil
	}
	if err := validateCloudBattleReport(report); err != nil {
		return cloudBattleReportEnvelope{}, true, err
	}
	capturedAt := int64(0)
	if !capture.CapturedAt.IsZero() {
		capturedAt = capture.CapturedAt.UnixMilli()
	}
	if capturedAt <= 0 {
		capturedAt = report.DateMs
	}
	payload, err := json.Marshal(legacyCloudBattleCapture{
		Version: 1, ID: report.ID, Source: "local", CapturedAtUnixMillis: capturedAt,
		MID: report.MID, LID: report.LID, NoticeType: 6, BattleKey: report.BattleKey,
		BLS: capture.Summary, BLM: capture.Waves, BLD: capture.Details,
	})
	if err != nil {
		return cloudBattleReportEnvelope{}, true, fmt.Errorf("encode cloud battle report capture: %w", err)
	}
	return cloudEnvelope(report, string(payload), cloudCaptureRichness(report, capture)), true, nil
}

func buildCloudEnvelopeFromReport(report BattleReport) (cloudBattleReportEnvelope, bool, error) {
	if !IsPvPBattleReport(report) {
		return cloudBattleReportEnvelope{}, false, nil
	}
	if err := validateCloudBattleReport(report); err != nil {
		return cloudBattleReportEnvelope{}, true, err
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return cloudBattleReportEnvelope{}, true, fmt.Errorf("encode canonical cloud battle report: %w", err)
	}
	return cloudEnvelope(report, string(payload), canonicalReportRichness(report)), true, nil
}

func cloudEnvelope(report BattleReport, payload string, richness int) cloudBattleReportEnvelope {
	return cloudBattleReportEnvelope{
		BattleDateUnixMillis: report.DateMs,
		MID:                  report.MID,
		LID:                  report.LID,
		AttackerPlayerID:     report.Attacker.PlayerID,
		AttackerAllianceID:   report.Attacker.AllianceID,
		DefenderPlayerID:     report.Defender.PlayerID,
		DefenderAllianceID:   report.Defender.AllianceID,
		AttackWon:            battleReportAttackWon(report),
		RichnessScore:        max(1, richness),
		Payload:              payload,
	}
}

func validateCloudBattleReport(report BattleReport) error {
	if report.MID <= 0 || report.LID <= 0 {
		return fmt.Errorf("cloud battle report requires positive MID and LID")
	}
	if report.DateMs <= 0 {
		return fmt.Errorf("cloud battle report requires a battle timestamp")
	}
	if !IsPvPBattleReport(report) {
		return fmt.Errorf("cloud battle report requires two real players")
	}
	return nil
}

func cloudCaptureRichness(report BattleReport, capture State.BattleReportCapture) int {
	score := canonicalReportRichness(report)
	if len(capture.Summary) > 0 {
		score += 1000
	}
	if len(capture.Waves) > 0 {
		score += 1000
	}
	if len(capture.Details) > 0 {
		score += 1000
	}
	return score
}

func canonicalReportRichness(report BattleReport) int {
	score := 100
	if report.ID != "" {
		score += 25
	}
	if report.DateMs > 0 {
		score += 25
	}
	if report.BattleKey != "" || report.CastleName != "" || report.TargetName != "" {
		score += 25
	}
	if report.Attacker != nil {
		score += 100
	}
	if report.Defender != nil {
		score += 100
	}
	score += min(len(report.TopUnits), 40) * 15
	return score
}

func battleReportAttackWon(report BattleReport) bool {
	switch strings.ToLower(strings.TrimSpace(report.Result)) {
	case "victory", "attack won", "defense lost":
		return true
	case "defeat", "attack lost", "defense win":
		return false
	}
	return report.Metrics.AttackerSent > 0 && report.Metrics.AttackerLost < report.Metrics.AttackerSent
}

func decodeCloudBattleReport(payload string, ownPlayerID State.PlayerID) (BattleReport, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return BattleReport{}, false
	}
	var canonical BattleReport
	if json.Unmarshal([]byte(payload), &canonical) == nil && strings.TrimSpace(canonical.ID) != "" &&
		canonical.Attacker != nil && canonical.Defender != nil {
		return canonical, true
	}
	var capture legacyCloudBattleCapture
	if json.Unmarshal([]byte(payload), &capture) != nil || len(capture.BLS) == 0 {
		return BattleReport{}, false
	}
	capturedAt := time.UnixMilli(capture.CapturedAtUnixMillis).UTC()
	report, err := ParseBattleCapture(State.BattleReportCapture{
		MessageID: capture.MID, ReportID: capture.LID, BattleKey: capture.BattleKey,
		Summary: capture.BLS, Waves: capture.BLM, Details: capture.BLD,
		OccurredAt: capturedAt, CapturedAt: capturedAt,
	}, ownPlayerID)
	if err != nil {
		return BattleReport{}, false
	}
	return report, true
}

func cloudPayloadLID(payload string) int64 {
	var identity struct {
		LID int64 `json:"lid"`
		BLS struct {
			LID int64 `json:"LID"`
		} `json:"bls"`
	}
	if json.Unmarshal([]byte(payload), &identity) != nil {
		return 0
	}
	if identity.LID > 0 {
		return identity.LID
	}
	return identity.BLS.LID
}

func cloudReportUploadURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_UPLOAD_URL")); endpoint != "" {
		return endpoint
	}
	return strings.TrimRight(cloudBackendURL(), "/") + "/reports/battle"
}

func cloudReportFetchURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_FETCH_URL")); endpoint != "" {
		return endpoint
	}
	if endpoint := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_UPLOAD_URL")); endpoint != "" {
		return endpoint
	}
	return strings.TrimRight(cloudBackendURL(), "/") + "/reports/battle"
}

func cloudBattleTrainingUploadURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("BATTLE_TRAINING_UPLOAD_URL")); endpoint != "" {
		return endpoint
	}
	return strings.TrimRight(cloudBackendURL(), "/") + "/reports/battle-training"
}

func cloudBackendURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("CLOUD_BACKEND_URL")); endpoint != "" {
		return endpoint
	}
	return defaultCloudBackendURL
}

func reportUploadKey() string {
	if key := strings.TrimSpace(os.Getenv("REPORT_UPLOAD_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("CITADEL_REPORT_UPLOAD_KEY"))
}
