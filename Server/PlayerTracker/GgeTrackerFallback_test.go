package playertracker

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestGGETrackerProviderResolvesAndReadsMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("gge-server") != "US1" {
			http.Error(w, "missing server", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/players/TestPlayer":
			_, _ = w.Write([]byte(`{"player_id":"123080","player_name":"TestPlayer"}`))
		case "/statistics/player/123080/player_might_history/1":
			_, _ = w.Write([]byte(`{"points":{"player_might_history":[{"date":"2026-07-10T17:00:00Z","point":"456"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GGE_TRACKER_API_URL", server.URL)

	identity, err := lookupGGETrackerPlayer(t.Context(), "US1", "TestPlayer", 123)
	if err != nil {
		t.Fatalf("lookupGGETrackerPlayer: %v", err)
	}
	points, _, err := fetchGGETrackerMetric(t.Context(), identity, "might", 1)
	if err != nil {
		t.Fatalf("fetchGGETrackerMetric: %v", err)
	}
	if len(points) != 1 || points[0].Value != 456 || points[0].Source != "gge-tracker" {
		t.Fatalf("points = %#v", points)
	}
}

func TestServerCandidatesFromGameSocket(t *testing.T) {
	supported := []string{"US1", "INT1", "SK1", "GB1", "ASIA", "HANT1"}
	tests := []struct {
		url  string
		want []string
	}{
		{"wss://ep-live-us1-game.goodgamestudios.com/socket", []string{"US1"}},
		{"wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com/socket", []string{"INT1", "SK1", "GB1"}},
		{"wss://ep-live-asia1-hant1-game.goodgamestudios.com/socket", []string{"ASIA", "HANT1"}},
	}
	for _, test := range tests {
		if got := serverCandidates(test.url, supported); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("serverCandidates(%q) = %v, want %v", test.url, got, test.want)
		}
	}
}

func TestRequestedRangeStartUsesRequestedCoverage(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	request := httptest.NewRequest("GET", "/api/player-tracker?rangeSeconds=7200", nil)
	if got, want := requestedRangeStart(request, now), now.Add(-2*time.Hour).Unix(); got != want {
		t.Fatalf("requestedRangeStart = %d, want %d", got, want)
	}
}

func TestMergeExternalMetricPointsKeepsLocalPriority(t *testing.T) {
	local := []MetricPoint{{TimestampUnix: 10_000, Value: 100, Source: "local"}}
	external := []MetricPoint{
		{TimestampUnix: 8_000, Value: 80, Source: "gge-tracker"},
		{TimestampUnix: 10_010, Value: 90, Source: "gge-tracker"},
		{TimestampUnix: 14_000, Value: 140, Source: "gge-tracker"},
	}
	merged, added := mergeExternalMetricPoints(local, external, 0)
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if len(merged) != 3 || merged[1].Source != "local" {
		t.Fatalf("merged = %#v, want two external points around the authoritative local point", merged)
	}
}
