package WorldIntel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudClientRegistersUploadsAndQueries(t *testing.T) {
	credentials := InstallationCredentials{InstallationID: "install", Secret: "secret"}
	occurrenceID := strings.Repeat("a", 64)
	now := time.Now().UTC().Truncate(time.Second)
	batch, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		Players: []PlayerObservation{{PlayerID: 1, Name: "Player", Source: "account", ObservedAt: now}},
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogSnapshot, err := FinalizeCatalogSnapshot(CatalogDatasetSnapshot{
		Source: OfficialCatalogSource, SourceVersion: "781.02",
		SourceURL:    "https://empire-html5.goodgamestudios.com/default/items/items_v781.02.json",
		SourceDigest: strings.Repeat("a", 64), DatasetKey: "islandrewardranks",
		DatasetLabel: "Storm alliance rank rewards", Category: "storm",
		Rows: json.RawMessage(`[{"cargoPointRequirement":"500"}]`), CapturedAt: now, CollectorPlayerID: 17334928,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/installations":
			var registration InstallationRegistration
			_ = json.NewDecoder(request.Body).Decode(&registration)
			if registration.InstallationID != credentials.InstallationID || registration.Secret != credentials.Secret {
				t.Errorf("registration = %#v", registration)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"registered":true}`))
		case "/v1/observations":
			if request.Header.Get("Authorization") != "CitadelInstall install.secret" {
				t.Errorf("authorization = %q", request.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(writer).Encode(IngestResponse{Accepted: true, BatchID: batch.BatchID})
		case "/v1/catalog-snapshots":
			if request.Header.Get("Authorization") != "CitadelInstall install.secret" {
				t.Errorf("catalog authorization = %q", request.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(writer).Encode(CatalogIngestResponse{Accepted: true, SnapshotID: catalogSnapshot.SnapshotID})
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
	if err := client.Register(context.Background(), credentials); err != nil {
		t.Fatal(err)
	}
	response, err := client.Upload(context.Background(), credentials, batch)
	if err != nil || !response.Accepted {
		t.Fatalf("upload = %#v, %v", response, err)
	}
	catalogUpload, err := client.UploadCatalog(context.Background(), credentials, catalogSnapshot)
	if err != nil || !catalogUpload.Accepted {
		t.Fatalf("catalog upload = %#v, %v", catalogUpload, err)
	}
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
