package WorldIntel

import (
	"context"
	"encoding/json"
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
}
