package WorldIntel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloudClientRegistersUploadsAndQueries(t *testing.T) {
	credentials := InstallationCredentials{InstallationID: "install", Secret: "secret"}
	now := time.Now().UTC().Truncate(time.Second)
	batch, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		Players: []PlayerObservation{{PlayerID: 1, Name: "Player", Source: "account", ObservedAt: now}},
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
		case "/v1/search":
			_ = json.NewEncoder(writer).Encode(SearchResponse{WorldID: request.URL.Query().Get("worldId"), Query: request.URL.Query().Get("q"), Results: []SearchResult{}})
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
	search, err := client.Search(context.Background(), "https://WORLD.EXAMPLE/socket", "Player", "player", 10)
	if err != nil || search.WorldID != "world.example" || search.Query != "Player" {
		t.Fatalf("search = %#v, %v", search, err)
	}
}
