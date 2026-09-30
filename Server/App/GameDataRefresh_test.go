package App

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestRefreshGameDataRehydratesRuntimeOnlyWhenTheStoreChanges(t *testing.T) {
	var itemVersion atomic.Value
	itemVersion.Store("1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			fmt.Fprintf(w, "CastleItemXMLVersion=%s\n", itemVersion.Load())
		case "/items":
			fmt.Fprint(w, `{"versionInfo":{},"buildings":[],"units":[]}`)
		case "/langmeta":
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7","branch":"b"}}`)
		case "/lang":
			fmt.Fprint(w, `{"hello":"Hello"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager := GameData.NewManager(GameData.UpdaterConfig{
		CacheDir:            filepath.Join(t.TempDir(), "cache"),
		VersionURL:          server.URL + "/version",
		ItemsURL:            server.URL + "/items?v={version}",
		LanguageMetadataURL: server.URL + "/langmeta",
		LanguageURL:         server.URL + "/lang?v={version}&l={language}",
		RequestTimeout:      5 * time.Second,
	})
	state := State.NewStore(State.NewGameState())
	var pending atomic.Bool
	ctx := context.Background()
	catalogVersion := func() string { return state.ReadOnlyView().CatalogVersion }
	tamper := func() {
		t.Helper()
		if _, err := state.ApplyComponents(State.Components(State.ComponentCatalog), func(gameState *State.GameState) ([]string, bool, error) {
			gameState.CatalogVersion = "tampered"
			return []string{"game-data"}, true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := refreshGameDataStore(ctx, state, manager, &pending); err != nil || catalogVersion() != "1" {
		t.Fatalf("first refresh: err %v, catalog version %q; want 1", err, catalogVersion())
	}
	tamper()
	if err := refreshGameDataStore(ctx, state, manager, &pending); err != nil {
		t.Fatal(err)
	}
	if got := catalogVersion(); got != "tampered" {
		t.Fatalf("a refresh with unchanged versions rehydrated the runtime (catalog version %q)", got)
	}
	itemVersion.Store("2")
	if err := refreshGameDataStore(ctx, state, manager, &pending); err != nil || catalogVersion() != "2" {
		t.Fatalf("refresh with a new version: err %v, catalog version %q; want 2", err, catalogVersion())
	}
	// A rehydration that failed earlier is retried even when the versions did not change.
	tamper()
	pending.Store(true)
	if err := refreshGameDataStore(ctx, state, manager, &pending); err != nil || catalogVersion() != "2" || pending.Load() {
		t.Fatalf("retry: err %v, catalog version %q, still pending %t; want 2 and cleared", err, catalogVersion(), pending.Load())
	}
}
