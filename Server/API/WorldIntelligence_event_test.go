package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"CitadelDesktop/Server/WorldIntel"
)

func TestWorldIntelligenceEventRoutesProxyBackendContract(t *testing.T) {
	occurrenceID := strings.Repeat("a", 64)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if got := request.URL.Query().Get("worldId"); got != "world.example" {
			t.Errorf("upstream world id = %q", got)
		}
		switch request.URL.Path {
		case "/v1/event-runs":
			if request.URL.Query().Get("eventKey") != "nomad-invasion" || request.URL.Query().Get("limit") != "250" {
				t.Errorf("event run query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(WorldIntel.EventRunListResponse{WorldID: "world.example", Runs: []WorldIntel.EventRun{{OccurrenceID: occurrenceID}}})
		case "/v1/event-runs/" + occurrenceID + "/rankings":
			if request.URL.Query().Get("listType") != "47" || request.URL.Query().Get("leagueId") != "-1" || request.URL.Query().Get("limit") != "5000" {
				t.Errorf("event ranking query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(WorldIntel.EventRunRankingResponse{Run: WorldIntel.EventRun{OccurrenceID: occurrenceID}, Entries: []WorldIntel.EventScoreObservation{{PlayerID: 7, Rank: 3, ScoreKnown: false}}})
		case "/v1/players/7/event-scores":
			if request.URL.Query().Get("eventKey") != "nomad-invasion" || request.URL.Query().Get("occurrenceId") != occurrenceID || request.URL.Query().Get("limit") != "5000" {
				t.Errorf("player event query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(WorldIntel.PlayerEventScoreResponse{WorldID: "world.example", PlayerID: 7, History: []WorldIntel.EventScoreObservation{{PlayerID: 7, Rank: 3, ScoreKnown: false}}})
		case "/v1/rankings/players":
			if request.URL.Query().Get("metric") != "public:storm-cargo-points" || request.URL.Query().Get("limit") != "5000" {
				t.Errorf("Storm metric query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(WorldIntel.RankingResponse{WorldID: "world.example", Type: "players", Metric: "public:storm-cargo-points", Entries: []WorldIntel.RankingEntry{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer upstream.Close()
	service := WorldIntel.NewDesktopService(
		nil,
		WorldIntel.NewCloudClient(WorldIntel.ClientConfig{Client: upstream.Client(), BaseURL: upstream.URL + "/v1", ClientVersion: "test"}),
	)
	handler := NewServer(Config{WorldIntel: service}).Handler()

	tests := []struct {
		name string
		path string
	}{
		{name: "event runs", path: "/api/v2/world-intelligence/event-runs?worldId=https%3A%2F%2FWORLD.EXAMPLE%2Fsocket&eventKey=Nomad-Invasion&limit=999"},
		{name: "run rankings", path: "/api/v2/world-intelligence/event-runs/" + occurrenceID + "/rankings?worldId=world.example&listType=47&leagueId=-1&limit=9999"},
		{name: "player event history", path: "/api/v2/world-intelligence/players/7/event-scores?worldId=world.example&eventKey=Nomad-Invasion&occurrenceId=" + occurrenceID + "&limit=9999"},
		{name: "full Storm metrics", path: "/api/v2/world-intelligence/rankings/players?worldId=world.example&metric=public%3Astorm-cargo-points&limit=9999"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestWorldIntelligenceEventRoutesRejectInvalidFilters(t *testing.T) {
	service := WorldIntel.NewDesktopService(nil, WorldIntel.NewCloudClient(WorldIntel.ClientConfig{}))
	handler := NewServer(Config{WorldIntel: service}).Handler()
	tests := []string{
		"/api/v2/world-intelligence/event-runs?worldId=world.example&eventKey=-invalid",
		"/api/v2/world-intelligence/event-runs/not-a-digest/rankings?worldId=world.example",
		"/api/v2/world-intelligence/event-runs/" + strings.Repeat("a", 64) + "/rankings?worldId=world.example&leagueId=-2",
		"/api/v2/world-intelligence/players/7/event-scores?worldId=world.example&occurrenceId=short",
	}
	for _, path := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestWorldIntelligenceSubscriptionProxiesServerSentEvents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/subscribe" || request.URL.Query().Get("worldId") != "world.example" {
			t.Errorf("subscription request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Last-Event-ID") != "8" {
			t.Errorf("last event id = %q", request.Header.Get("Last-Event-ID"))
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("id: 9\nevent: world-intel.update\ndata: {\"revision\":9}\n\n"))
	}))
	defer upstream.Close()
	service := WorldIntel.NewDesktopService(nil, WorldIntel.NewCloudClient(WorldIntel.ClientConfig{
		Client: upstream.Client(), BaseURL: upstream.URL + "/v1", ClientVersion: "test",
	}))
	handler := NewServer(Config{WorldIntel: service}).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/world-intelligence/subscribe?worldId=world.example", nil)
	request.Header.Set("Last-Event-ID", "8")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("subscription response = %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	if body := recorder.Body.String(); !strings.Contains(body, "id: 9") || !strings.Contains(body, "world-intel.update") {
		t.Fatalf("subscription body = %q", body)
	}
}

func TestWorldIntelligenceLeaderboardSubscriptionProxiesFullBase(t *testing.T) {
	occurrenceID := strings.Repeat("b", 64)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/event-runs/"+occurrenceID+"/subscribe" ||
			request.URL.Query().Get("worldId") != "world.example" || request.URL.Query().Has("limit") {
			t.Errorf("leaderboard subscription = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("id: 10\nevent: world-intel.leaderboard.snapshot\ndata: {\"revision\":10,\"complete\":true,\"run\":{},\"entries\":[]}\n\n"))
	}))
	defer upstream.Close()
	service := WorldIntel.NewDesktopService(nil, WorldIntel.NewCloudClient(WorldIntel.ClientConfig{
		Client: upstream.Client(), BaseURL: upstream.URL + "/v1", ClientVersion: "test",
	}))
	handler := NewServer(Config{WorldIntel: service}).Handler()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v2/world-intelligence/event-runs/"+occurrenceID+"/subscribe?worldId=world.example", nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "world-intel.leaderboard.snapshot") {
		t.Fatalf("leaderboard proxy = %d %q", recorder.Code, recorder.Body.String())
	}
}
