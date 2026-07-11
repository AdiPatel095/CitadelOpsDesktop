package playertracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	defaultGGETrackerAPI = "https://api.gge-tracker.com/api/v1"
	fallbackCacheTTL     = time.Hour
	maxFallbackRange     = 365 * 24 * time.Hour
	missingRangeGrace    = 90 * time.Minute
	missingGapThreshold  = 2 * time.Hour
	localPointPriority   = 30 * time.Minute
)

var fallbackMetricConfig = map[string]string{
	"might": "player_might_history",
	"loot":  "player_loot_history",
}

var fallbackHTTPClient = &http.Client{Timeout: 8 * time.Second}

var fallbackCache = struct {
	sync.Mutex
	serversFetchedAt time.Time
	servers          []string
	series           map[string]cachedMetricSeries
}{series: make(map[string]cachedMetricSeries)}

type cachedMetricSeries struct {
	FetchedAt time.Time
	Points    []MetricPoint
}

func requestedRangeStart(r *http.Request, now time.Time) int64 {
	duration := 24 * time.Hour
	if raw := strings.TrimSpace(r.URL.Query().Get("rangeSeconds")); raw != "" {
		if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
			duration = time.Duration(seconds) * time.Second
		}
	}
	if duration > maxFallbackRange {
		duration = maxFallbackRange
	}
	return now.Add(-duration).Unix()
}

func localMetricSeries(samples []Sample, current *Sample) map[string][]MetricPoint {
	series := make(map[string][]MetricPoint)
	appendSample := func(sample Sample) {
		values := map[string]float64{
			"might":           sample.Might,
			"glory":           sample.Glory,
			"gallantry":       sample.Gallantry,
			"troopsTotal":     float64(sample.TroopsTotal),
			"troopsStationed": float64(sample.TroopsStationed),
			"troopsTraveling": float64(sample.TroopsTraveling),
			"troopsHospital":  float64(sample.TroopsHospital),
			"coins":           sample.Coins,
			"rubies":          sample.Rubies,
		}
		for metric, value := range values {
			series[metric] = append(series[metric], MetricPoint{
				TimestampUnix: sample.TimestampUnix,
				Value:         value,
				Source:        "local",
			})
		}
	}
	for _, sample := range samples {
		appendSample(sample)
	}
	if current != nil {
		appendSample(*current)
	}
	return series
}

func augmentWithGGETracker(
	ctx context.Context,
	playerID int,
	playerName string,
	rangeStart int64,
	series map[string][]MetricPoint,
) FallbackInfo {
	info := FallbackInfo{Provider: "gge-tracker", Status: "not-needed"}
	if playerID <= 0 {
		info.Status = "identity-unavailable"
		return info
	}

	missingMetrics := make([]string, 0, len(fallbackMetricConfig))
	for metric := range fallbackMetricConfig {
		if metricNeedsFallback(series[metric], rangeStart) {
			missingMetrics = append(missingMetrics, metric)
		}
	}
	if len(missingMetrics) == 0 {
		return info
	}
	sort.Strings(missingMetrics)

	identity, err := resolveGGETrackerIdentity(ctx, playerID, playerName)
	if err != nil {
		info.Status = "identity-unavailable"
		return info
	}
	info.Server = identity.Server
	info.PlayerName = identity.PlayerName

	duration := time.Since(time.Unix(rangeStart, 0))
	days := int((duration + 24*time.Hour - 1) / (24 * time.Hour))
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}

	var latestFetch time.Time
	errors := 0
	for _, metric := range missingMetrics {
		points, fetchedAt, fetchErr := fetchGGETrackerMetric(ctx, identity, metric, days)
		if fetchErr != nil {
			errors++
			continue
		}
		if fetchedAt.After(latestFetch) {
			latestFetch = fetchedAt
		}
		merged, added := mergeExternalMetricPoints(series[metric], points, rangeStart)
		series[metric] = merged
		info.PointsAdded += added
	}
	if !latestFetch.IsZero() {
		info.FetchedAtUnix = latestFetch.Unix()
	}
	switch {
	case info.PointsAdded > 0 && errors > 0:
		info.Status = "partial"
	case info.PointsAdded > 0:
		info.Status = "backfilled"
	case errors == len(missingMetrics):
		info.Status = "unavailable"
	default:
		info.Status = "no-data"
	}
	return info
}

func metricNeedsFallback(points []MetricPoint, rangeStart int64) bool {
	if len(points) == 0 {
		return true
	}
	local := append([]MetricPoint(nil), points...)
	sort.Slice(local, func(i, j int) bool { return local[i].TimestampUnix < local[j].TimestampUnix })
	if local[0].TimestampUnix > rangeStart+int64(missingRangeGrace/time.Second) {
		return true
	}
	for i := 1; i < len(local); i++ {
		if local[i].TimestampUnix-local[i-1].TimestampUnix > int64(missingGapThreshold/time.Second) {
			return true
		}
	}
	return false
}

func resolveGGETrackerIdentity(ctx context.Context, playerID int, playerName string) (TrackerIdentity, error) {
	store.Lock()
	cached, ok := store.identities[playerID]
	store.Unlock()
	if ok && cached.ExternalPlayerID != "" && cached.Server != "" &&
		(playerName == "" || strings.EqualFold(cached.PlayerName, playerName)) {
		return cached, nil
	}
	if playerName == "" {
		return TrackerIdentity{}, fmt.Errorf("player name is unavailable")
	}

	servers, err := supportedGGETrackerServers(ctx)
	if err != nil {
		return TrackerIdentity{}, err
	}
	candidates := serverCandidates(ResponseRegistry.CurrentGameServerURL(), servers)
	if len(candidates) == 0 {
		return TrackerIdentity{}, fmt.Errorf("could not identify the active GGE server")
	}

	for _, server := range candidates {
		identity, lookupErr := lookupGGETrackerPlayer(ctx, server, playerName, playerID)
		if lookupErr != nil {
			continue
		}
		store.Lock()
		store.identities[playerID] = identity
		writeLocked()
		store.Unlock()
		return identity, nil
	}
	return TrackerIdentity{}, fmt.Errorf("player was not found on active server candidates")
}

func supportedGGETrackerServers(ctx context.Context) ([]string, error) {
	fallbackCache.Lock()
	if len(fallbackCache.servers) > 0 && time.Since(fallbackCache.serversFetchedAt) < 6*time.Hour {
		servers := append([]string(nil), fallbackCache.servers...)
		fallbackCache.Unlock()
		return servers, nil
	}
	fallbackCache.Unlock()

	var servers []string
	if err := getGGETrackerJSON(ctx, "/servers", "", &servers); err != nil {
		return nil, err
	}
	fallbackCache.Lock()
	fallbackCache.servers = append([]string(nil), servers...)
	fallbackCache.serversFetchedAt = time.Now()
	fallbackCache.Unlock()
	return servers, nil
}

// ActiveGGETrackerServer resolves the connected game socket to the GGE Tracker server code.
func ActiveGGETrackerServer(ctx context.Context) (string, error) {
	servers, err := supportedGGETrackerServers(ctx)
	if err != nil {
		return "", err
	}
	candidates := serverCandidates(ResponseRegistry.CurrentGameServerURL(), servers)
	if len(candidates) == 0 {
		return "", fmt.Errorf("could not identify the active GGE server")
	}
	return candidates[0], nil
}

func serverCandidates(socketURL string, supported []string) []string {
	parsed, err := url.Parse(socketURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(parsed.Hostname())
	cluster := strings.TrimPrefix(strings.Split(host, ".")[0], "ep-live-")
	cluster = strings.TrimSuffix(cluster, "-game")
	cluster = strings.TrimPrefix(cluster, "mz-")
	if cluster == "" {
		return nil
	}
	tokens := strings.Split(cluster, "-")
	seen := make(map[string]struct{})
	var candidates []string
	for _, token := range tokens {
		for _, server := range supported {
			serverToken := strings.ToLower(server)
			if token != serverToken && token != serverToken+"1" {
				continue
			}
			if _, exists := seen[server]; exists {
				continue
			}
			seen[server] = struct{}{}
			candidates = append(candidates, server)
		}
	}
	return candidates
}

func lookupGGETrackerPlayer(ctx context.Context, server, playerName string, playerID int) (TrackerIdentity, error) {
	var result struct {
		PlayerID   string `json:"player_id"`
		PlayerName string `json:"player_name"`
	}
	path := "/players/" + url.PathEscape(playerName)
	if err := getGGETrackerJSON(ctx, path, server, &result); err != nil {
		return TrackerIdentity{}, err
	}
	if result.PlayerID == "" || !strings.EqualFold(result.PlayerName, playerName) {
		return TrackerIdentity{}, fmt.Errorf("GGE Tracker returned a different player")
	}
	if !strings.HasPrefix(result.PlayerID, strconv.Itoa(playerID)) {
		return TrackerIdentity{}, fmt.Errorf("GGE Tracker player id does not match the active account")
	}
	return TrackerIdentity{
		PlayerID:         playerID,
		PlayerName:       result.PlayerName,
		Server:           server,
		ExternalPlayerID: result.PlayerID,
	}, nil
}

func fetchGGETrackerMetric(
	ctx context.Context,
	identity TrackerIdentity,
	metric string,
	days int,
) ([]MetricPoint, time.Time, error) {
	externalSeries, ok := fallbackMetricConfig[metric]
	if !ok {
		return nil, time.Time{}, fmt.Errorf("unsupported fallback metric %q", metric)
	}
	cacheKey := strings.Join([]string{identity.Server, identity.ExternalPlayerID, metric, strconv.Itoa(days)}, ":")
	fallbackCache.Lock()
	if cached, found := fallbackCache.series[cacheKey]; found && time.Since(cached.FetchedAt) < fallbackCacheTTL {
		points := append([]MetricPoint(nil), cached.Points...)
		fallbackCache.Unlock()
		return points, cached.FetchedAt, nil
	}
	fallbackCache.Unlock()

	var result struct {
		Points map[string][]struct {
			Date  string      `json:"date"`
			Point interface{} `json:"point"`
		} `json:"points"`
	}
	path := fmt.Sprintf("/statistics/player/%s/%s/%d", url.PathEscape(identity.ExternalPlayerID), externalSeries, days)
	if err := getGGETrackerJSON(ctx, path, identity.Server, &result); err != nil {
		return nil, time.Time{}, err
	}

	points := make([]MetricPoint, 0, len(result.Points[externalSeries]))
	for _, raw := range result.Points[externalSeries] {
		stamp, err := time.Parse(time.RFC3339, raw.Date)
		if err != nil {
			continue
		}
		value, ok := fallbackNumber(raw.Point)
		if !ok {
			continue
		}
		points = append(points, MetricPoint{TimestampUnix: stamp.Unix(), Value: value, Source: "gge-tracker"})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].TimestampUnix < points[j].TimestampUnix })
	fetchedAt := time.Now()
	fallbackCache.Lock()
	fallbackCache.series[cacheKey] = cachedMetricSeries{FetchedAt: fetchedAt, Points: append([]MetricPoint(nil), points...)}
	fallbackCache.Unlock()
	return points, fetchedAt, nil
}

func mergeExternalMetricPoints(local, external []MetricPoint, rangeStart int64) ([]MetricPoint, int) {
	merged := append([]MetricPoint(nil), local...)
	localOnly := make([]MetricPoint, 0, len(local))
	for _, point := range local {
		if point.Source == "local" {
			localOnly = append(localOnly, point)
		}
	}
	sort.Slice(localOnly, func(i, j int) bool { return localOnly[i].TimestampUnix < localOnly[j].TimestampUnix })
	added := 0
	for _, point := range external {
		if point.TimestampUnix < rangeStart || hasNearbyLocalPoint(localOnly, point.TimestampUnix) {
			continue
		}
		merged = append(merged, point)
		added++
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].TimestampUnix < merged[j].TimestampUnix })
	return merged, added
}

func hasNearbyLocalPoint(local []MetricPoint, timestamp int64) bool {
	index := sort.Search(len(local), func(i int) bool { return local[i].TimestampUnix >= timestamp })
	window := int64(localPointPriority / time.Second)
	if index < len(local) && absInt64(local[index].TimestampUnix-timestamp) <= window {
		return true
	}
	return index > 0 && absInt64(local[index-1].TimestampUnix-timestamp) <= window
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func fallbackNumber(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func getGGETrackerJSON(ctx context.Context, path, server string, target interface{}) error {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GGE_TRACKER_API_URL")), "/")
	if baseURL == "" {
		baseURL = defaultGGETrackerAPI
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CitadelOpsDesktop/player-tracker")
	if server != "" {
		request.Header.Set("gge-server", server)
	}
	response, err := fallbackHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GGE Tracker returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode GGE Tracker response: %w", err)
	}
	return nil
}
