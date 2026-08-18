package WorldIntel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCloudURL = "https://citadelops.app/api/world-intel/v1"
	requestTimeout  = 20 * time.Second
	coverageTimeout = 5 * time.Second
)

type ClientConfig struct {
	Client        *http.Client
	BaseURL       string
	ClientVersion string
}

type CloudClient struct {
	client        *http.Client
	streamClient  *http.Client
	baseURL       string
	clientVersion string
}

func NewCloudClient(config ClientConfig) *CloudClient {
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	streamClient := &http.Client{
		Transport:     client.Transport,
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("CITADEL_WORLD_INTEL_URL")), "/")
	}
	if baseURL == "" {
		baseURL = defaultCloudURL
	}
	return &CloudClient{
		client: client, streamClient: streamClient,
		baseURL: baseURL, clientVersion: strings.TrimSpace(config.ClientVersion),
	}
}

func (client *CloudClient) Endpoint() string {
	if client == nil {
		return ""
	}
	return client.baseURL
}

func (client *CloudClient) CatalogDatasets(ctx context.Context) (CatalogDatasetCatalogResponse, error) {
	var result CatalogDatasetCatalogResponse
	err := client.getJSON(ctx, "/catalog-datasets", &result)
	return result, err
}

func (client *CloudClient) CatalogDataset(
	ctx context.Context,
	datasetKey string,
	historyLimit int,
) (CatalogDatasetResponse, error) {
	values := url.Values{"historyLimit": {strconv.Itoa(boundedLimit(historyLimit, 25))}}
	var result CatalogDatasetResponse
	err := client.getJSON(ctx, "/catalog-datasets/"+url.PathEscape(datasetKey)+"?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) Search(
	ctx context.Context,
	worldID string,
	query string,
	entityType string,
	limit int,
) (SearchResponse, error) {
	values := url.Values{"worldId": {NormalizeWorldID(worldID)}, "q": {strings.TrimSpace(query)}}
	if entityType != "" {
		values.Set("type", entityType)
	}
	values.Set("limit", strconv.Itoa(boundedLimit(limit, 50)))
	var result SearchResponse
	err := client.getJSON(ctx, "/search?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) Player(
	ctx context.Context,
	worldID string,
	playerID int64,
	limit int,
) (PlayerProfile, error) {
	values := url.Values{"worldId": {NormalizeWorldID(worldID)}, "limit": {strconv.Itoa(boundedLimit(limit, 365))}}
	var result PlayerProfile
	err := client.getJSON(ctx, "/players/"+strconv.FormatInt(playerID, 10)+"?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) Alliance(
	ctx context.Context,
	worldID string,
	allianceID int64,
	limit int,
) (AllianceProfile, error) {
	values := url.Values{"worldId": {NormalizeWorldID(worldID)}, "limit": {strconv.Itoa(boundedLimit(limit, 365))}}
	var result AllianceProfile
	err := client.getJSON(ctx, "/alliances/"+strconv.FormatInt(allianceID, 10)+"?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) EventRuns(
	ctx context.Context,
	worldID string,
	eventKey string,
	limit int,
) (EventRunListResponse, error) {
	values := url.Values{
		"worldId": {NormalizeWorldID(worldID)},
		"limit":   {strconv.Itoa(boundedLimitMaximum(limit, 50, 250))},
	}
	if normalized := strings.ToLower(strings.TrimSpace(eventKey)); normalized != "" {
		values.Set("eventKey", normalized)
	}
	var result EventRunListResponse
	err := client.getJSON(ctx, "/event-runs?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) EventRunRankings(
	ctx context.Context,
	worldID string,
	occurrenceID string,
	listType int64,
	leagueID int64,
	limit int,
) (EventRunRankingResponse, error) {
	values := url.Values{
		"worldId": {NormalizeWorldID(worldID)},
		"limit":   {strconv.Itoa(boundedLimitMaximum(limit, 250, 5_000))},
	}
	if listType > 0 {
		values.Set("listType", strconv.FormatInt(listType, 10))
	}
	if leagueID >= -1 {
		values.Set("leagueId", strconv.FormatInt(leagueID, 10))
	}
	var result EventRunRankingResponse
	err := client.getJSON(ctx, "/event-runs/"+url.PathEscape(strings.ToLower(strings.TrimSpace(occurrenceID)))+"/rankings?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) PlayerEventScores(
	ctx context.Context,
	worldID string,
	playerID int64,
	eventKey string,
	occurrenceID string,
	limit int,
) (PlayerEventScoreResponse, error) {
	values := url.Values{
		"worldId": {NormalizeWorldID(worldID)},
		"limit":   {strconv.Itoa(boundedLimitMaximum(limit, 1_000, 5_000))},
	}
	if normalized := strings.ToLower(strings.TrimSpace(eventKey)); normalized != "" {
		values.Set("eventKey", normalized)
	}
	if normalized := strings.ToLower(strings.TrimSpace(occurrenceID)); normalized != "" {
		values.Set("occurrenceId", normalized)
	}
	var result PlayerEventScoreResponse
	err := client.getJSON(ctx, "/players/"+strconv.FormatInt(playerID, 10)+"/event-scores?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) Rankings(
	ctx context.Context,
	worldID string,
	entityType string,
	metric string,
	limit int,
) (RankingResponse, error) {
	values := url.Values{
		"worldId": {NormalizeWorldID(worldID)}, "metric": {metric},
		"limit": {strconv.Itoa(boundedLimit(limit, 100))},
	}
	var result RankingResponse
	err := client.getJSON(ctx, "/rankings/"+url.PathEscape(entityType)+"?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) RankingMetrics(
	ctx context.Context,
	worldID string,
	entityType string,
) (RankingMetricCatalogResponse, error) {
	values := url.Values{"worldId": {NormalizeWorldID(worldID)}}
	var result RankingMetricCatalogResponse
	err := client.getJSON(ctx, "/ranking-metrics/"+url.PathEscape(entityType)+"?"+values.Encode(), &result)
	return result, err
}

func (client *CloudClient) Coverage(ctx context.Context, worldID string) (CoverageResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, coverageTimeout)
	defer cancel()
	values := url.Values{}
	if normalized := NormalizeWorldID(worldID); normalized != "" {
		values.Set("worldId", normalized)
	}
	path := "/coverage"
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	var result CoverageResponse
	err := client.getJSON(ctx, path, &result)
	return result, err
}

func (client *CloudClient) Subscribe(ctx context.Context, worldID string, lastEventID string) (*http.Response, error) {
	values := url.Values{"worldId": {NormalizeWorldID(worldID)}}
	return client.subscribe(ctx, "/subscribe?"+values.Encode(), lastEventID)
}

func (client *CloudClient) SubscribeEventRun(
	ctx context.Context,
	worldID string,
	occurrenceID string,
	listType int64,
	leagueID int64,
	lastEventID string,
) (*http.Response, error) {
	values := url.Values{
		"worldId": {NormalizeWorldID(worldID)},
	}
	if listType > 0 {
		values.Set("listType", strconv.FormatInt(listType, 10))
	}
	if leagueID >= -1 {
		values.Set("leagueId", strconv.FormatInt(leagueID, 10))
	}
	path := "/event-runs/" + url.PathEscape(strings.ToLower(strings.TrimSpace(occurrenceID))) + "/subscribe?" + values.Encode()
	return client.subscribe(ctx, path, lastEventID)
}

func (client *CloudClient) subscribe(ctx context.Context, path string, lastEventID string) (*http.Response, error) {
	if client == nil || client.streamClient == nil || client.baseURL == "" {
		return nil, fmt.Errorf("world intelligence cloud client is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create World Intelligence subscription: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "CitadelOpsDesktop/"+client.clientVersion)
	if lastEventID = strings.TrimSpace(lastEventID); lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := client.streamClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("World Intelligence subscription failed: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("World Intelligence subscription returned %s: %s", response.Status, message)
	}
	return response, nil
}

func (client *CloudClient) getJSON(ctx context.Context, path string, target any) error {
	if client == nil || client.client == nil || client.baseURL == "" {
		return fmt.Errorf("world intelligence cloud client is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create world intelligence request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CitadelOpsDesktop/"+client.clientVersion)
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("world intelligence request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		var structured struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		message := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &structured) == nil && structured.Error.Message != "" {
			message = structured.Error.Message
		}
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("world intelligence returned %s: %s", response.Status, message)
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode world intelligence response: %w", err)
	}
	return nil
}

func boundedLimit(value int, fallback int) int {
	return boundedLimitMaximum(value, fallback, 1_000)
}

func boundedLimitMaximum(value int, fallback int, maximum int) int {
	if value <= 0 {
		return fallback
	}
	return min(value, maximum)
}
