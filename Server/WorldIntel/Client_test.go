package WorldIntel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudClientOnlyQueriesSharedWorldIntelligence(t *testing.T) {
	occurrenceID := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s", request.Method)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("desktop query sent authorization %q", authorization)
		}
		if userAgent := request.Header.Get("User-Agent"); userAgent != "CitadelOpsDesktop/test" {
			t.Errorf("user agent = %q", userAgent)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/catalog-datasets":
			_ = json.NewEncoder(writer).Encode(CatalogDatasetCatalogResponse{Source: OfficialCatalogSource, Datasets: []CatalogDatasetSummary{}})
		case "/v1/catalog-datasets/islandrewardranks":
			_ = json.NewEncoder(writer).Encode(CatalogDatasetResponse{
				CatalogDatasetSummary: CatalogDatasetSummary{DatasetKey: "islandrewardranks"}, Rows: json.RawMessage(`[]`), History: []CatalogDatasetSummary{},
			})
		case "/v1/search":
			_ = json.NewEncoder(writer).Encode(SearchResponse{WorldID: request.URL.Query().Get("worldId"), Query: request.URL.Query().Get("q"), Results: []SearchResult{}})
		case "/v1/event-runs":
			if got := request.URL.Query().Get("eventKey"); got != "nomad-invasion" {
				t.Errorf("event key = %q", got)
			}
			if got := request.URL.Query().Get("limit"); got != "250" {
				t.Errorf("event run limit = %q", got)
			}
			_ = json.NewEncoder(writer).Encode(EventRunListResponse{WorldID: request.URL.Query().Get("worldId"), EventKey: request.URL.Query().Get("eventKey"), Runs: []EventRun{{OccurrenceID: occurrenceID, EventKey: "nomad-invasion", EventName: "Nomad Invasion"}}})
		case "/v1/event-runs/" + occurrenceID + "/rankings":
			if got := request.URL.Query().Get("listType"); got != "47" {
				t.Errorf("list type = %q", got)
			}
			if got := request.URL.Query().Get("leagueId"); got != "-1" {
				t.Errorf("league id = %q", got)
			}
			if got := request.URL.Query().Get("limit"); got != "5000" {
				t.Errorf("event ranking limit = %q", got)
			}
			_ = json.NewEncoder(writer).Encode(EventRunRankingResponse{Run: EventRun{OccurrenceID: occurrenceID}, Entries: []EventScoreObservation{{OccurrenceID: occurrenceID, PlayerID: 7, Rank: 3, ScoreKnown: false}}})
		case "/v1/players/7/event-scores":
			if got := request.URL.Query().Get("occurrenceId"); got != occurrenceID {
				t.Errorf("occurrence id = %q", got)
			}
			if got := request.URL.Query().Get("limit"); got != "5000" {
				t.Errorf("player history limit = %q", got)
			}
			_ = json.NewEncoder(writer).Encode(PlayerEventScoreResponse{WorldID: request.URL.Query().Get("worldId"), PlayerID: 7, EventKey: request.URL.Query().Get("eventKey"), OccurrenceID: request.URL.Query().Get("occurrenceId"), History: []EventScoreObservation{{OccurrenceID: occurrenceID, PlayerID: 7, Rank: 3, ScoreKnown: false}}})
		case "/v1/rankings/players":
			if request.URL.Query().Get("metric") != "public:storm-cargo-points" || request.URL.Query().Get("limit") != "5000" {
				t.Errorf("Storm metric query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(RankingResponse{WorldID: request.URL.Query().Get("worldId"), Type: "players", Metric: request.URL.Query().Get("metric"), Entries: []RankingEntry{}})
		case "/v1/subscribe":
			if request.Header.Get("Accept") != "text/event-stream" || request.Header.Get("Last-Event-ID") != "41" {
				t.Errorf("subscription headers = Accept %q Last-Event-ID %q", request.Header.Get("Accept"), request.Header.Get("Last-Event-ID"))
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "id: 42\nevent: world-intel.update\ndata: {}\n\n")
		case "/v1/event-runs/" + occurrenceID + "/subscribe":
			if request.URL.Query().Has("limit") || request.URL.Query().Get("leagueId") != "-1" ||
				request.URL.Query().Get("listType") != "47" {
				t.Errorf("leaderboard subscription query = %q", request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "id: 43\nevent: world-intel.leaderboard.snapshot\ndata: {}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := NewCloudClient(ClientConfig{Client: server.Client(), BaseURL: server.URL + "/v1", ClientVersion: "test"})
	if datasets, err := client.CatalogDatasets(context.Background()); err != nil || datasets.Source != OfficialCatalogSource {
		t.Fatalf("catalog datasets = %#v, %v", datasets, err)
	}
	if dataset, err := client.CatalogDataset(context.Background(), "islandrewardranks", 25); err != nil || dataset.DatasetKey != "islandrewardranks" {
		t.Fatalf("catalog dataset = %#v, %v", dataset, err)
	}
	search, err := client.Search(context.Background(), "https://WORLD.EXAMPLE/socket", "Player", "player", 10)
	if err != nil || search.WorldID != "world.example" || search.Query != "Player" {
		t.Fatalf("search = %#v, %v", search, err)
	}
	runs, err := client.EventRuns(context.Background(), "https://WORLD.EXAMPLE/socket", "Nomad-Invasion", 999)
	if err != nil || runs.WorldID != "world.example" || len(runs.Runs) != 1 || runs.Runs[0].OccurrenceID != occurrenceID {
		t.Fatalf("event runs = %#v, %v", runs, err)
	}
	rankings, err := client.EventRunRankings(context.Background(), "world.example", occurrenceID, 47, -1, 9_999)
	if err != nil || len(rankings.Entries) != 1 || rankings.Entries[0].ScoreKnown || rankings.Entries[0].Score != 0 {
		t.Fatalf("event rankings = %#v, %v", rankings, err)
	}
	history, err := client.PlayerEventScores(context.Background(), "world.example", 7, "nomad-invasion", occurrenceID, 9_999)
	if err != nil || history.PlayerID != 7 || len(history.History) != 1 || history.History[0].ScoreKnown {
		t.Fatalf("player event history = %#v, %v", history, err)
	}
	stormMetrics, err := client.Rankings(context.Background(), "world.example", "players", "public:storm-cargo-points", 9_999)
	if err != nil || stormMetrics.Metric != "public:storm-cargo-points" {
		t.Fatalf("Storm metrics = %#v, %v", stormMetrics, err)
	}
	subscription, err := client.Subscribe(context.Background(), "https://WORLD.EXAMPLE/socket", "41")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := io.ReadAll(subscription.Body)
	subscription.Body.Close()
	if err != nil || !strings.Contains(string(stream), "id: 42") {
		t.Fatalf("subscription stream = %q, %v", stream, err)
	}
	leaderboard, err := client.SubscribeEventRun(context.Background(), "world.example", occurrenceID, 47, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	leaderboardStream, err := io.ReadAll(leaderboard.Body)
	leaderboard.Body.Close()
	if err != nil || !strings.Contains(string(leaderboardStream), "world-intel.leaderboard.snapshot") {
		t.Fatalf("leaderboard stream = %q, %v", leaderboardStream, err)
	}
}
