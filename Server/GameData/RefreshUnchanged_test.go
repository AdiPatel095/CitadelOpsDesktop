package GameData

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// officialDataServer serves a minimal official item and language document whose
// versions the test can change, counting document downloads.
type officialDataServer struct {
	mu              sync.Mutex
	itemVersion     string
	languageVersion string
	itemDownloads   atomic.Int32
	langDownloads   atomic.Int32
}

func (server *officialDataServer) versions() (string, string) {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.itemVersion, server.languageVersion
}

func (server *officialDataServer) set(itemVersion string, languageVersion string) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.itemVersion, server.languageVersion = itemVersion, languageVersion
}

func newOfficialDataManager(t *testing.T) (*Manager, *officialDataServer) {
	t.Helper()
	data := &officialDataServer{itemVersion: "1", languageVersion: "7"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itemVersion, languageVersion := data.versions()
		switch {
		case r.URL.Path == "/version":
			fmt.Fprintf(w, "CastleItemXMLVersion=%s\n", itemVersion)
		case r.URL.Path == "/items":
			data.itemDownloads.Add(1)
			fmt.Fprintf(w, `{"versionInfo":{"v":%q},"buildings":[],"units":[{"unitID":"1"}]}`, r.URL.Query().Get("v"))
		case r.URL.Path == "/langmeta":
			fmt.Fprintf(w, `{"@metadata":{"versionNo":%q,"branch":"b"}}`, languageVersion)
		case r.URL.Path == "/lang":
			data.langDownloads.Add(1)
			fmt.Fprint(w, `{"hello":"Hello"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	manager := NewManager(UpdaterConfig{
		CacheDir:            filepath.Join(t.TempDir(), "cache"),
		VersionURL:          server.URL + "/version",
		ItemsURL:            server.URL + "/items?v={version}",
		LanguageMetadataURL: server.URL + "/langmeta",
		LanguageURL:         server.URL + "/lang?v={version}&l={language}",
		RequestTimeout:      5 * time.Second,
	})
	return manager, data
}

func TestRefreshWithUnchangedVersionsKeepsStoreAndSkipsDocuments(t *testing.T) {
	manager, data := newOfficialDataManager(t)
	ctx := context.Background()

	changed, err := manager.RefreshChanged(ctx)
	if err != nil || !changed {
		t.Fatalf("first refresh = changed %t, err %v; want changed", changed, err)
	}
	store, _ := manager.Current()
	language, _ := manager.Language()
	catalog, err := store.Catalog("units")
	if err != nil {
		t.Fatal(err)
	}
	// Prove that no document is re-read or re-parsed: the cached files are gone and downloads are counted.
	cached, err := filepath.Glob(filepath.Join(manager.config.CacheDir, "*.json"))
	if err != nil || len(cached) != 2 {
		t.Fatalf("cached documents = %v, %v; want the items and language files", cached, err)
	}
	for _, path := range cached {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	itemsBefore, languageBefore := data.itemDownloads.Load(), data.langDownloads.Load()

	for refresh := 0; refresh < 3; refresh++ {
		changed, err = manager.RefreshChanged(ctx)
		if err != nil || changed {
			t.Fatalf("unchanged refresh %d = changed %t, err %v; want unchanged", refresh, changed, err)
		}
		if err := manager.Refresh(ctx); err != nil {
			t.Fatalf("Refresh with unchanged versions: %v", err)
		}
	}
	if current, _ := manager.Current(); current != store {
		t.Fatal("unchanged refresh replaced the store")
	}
	if current, _ := manager.Language(); current != language {
		t.Fatal("unchanged refresh replaced the language store")
	}
	if again, _ := store.Catalog("units"); again != catalog {
		t.Fatal("unchanged refresh discarded a lazily built catalog")
	}
	if data.itemDownloads.Load() != itemsBefore || data.langDownloads.Load() != languageBefore {
		t.Fatal("unchanged refresh downloaded a document")
	}
	if remaining, _ := filepath.Glob(filepath.Join(manager.config.CacheDir, "*.json")); len(remaining) != 0 {
		t.Fatalf("unchanged refresh re-wrote cached documents: %v", remaining)
	}
}

func TestRefreshWithNewItemVersionSwitchesStore(t *testing.T) {
	manager, data := newOfficialDataManager(t)
	ctx := context.Background()
	if _, err := manager.RefreshChanged(ctx); err != nil {
		t.Fatal(err)
	}
	first, _ := manager.Current()
	data.set("2", "7")
	changed, err := manager.RefreshChanged(ctx)
	if err != nil || !changed {
		t.Fatalf("item version bump = changed %t, err %v; want changed", changed, err)
	}
	second, _ := manager.Current()
	if second == first || second.Metadata().ItemVersion != "2" {
		t.Fatalf("store after item bump = %p version %q, want a new store at version 2", second, second.Metadata().ItemVersion)
	}
	if language := second.Metadata().LanguageVersion; language != "7" {
		t.Fatalf("language version = %q, want 7", language)
	}
	if first.Metadata().ItemVersion != "1" {
		t.Fatal("the retired store was mutated")
	}
}

func TestRefreshWithNewLanguageVersionRebuildsStoreFromCachedItems(t *testing.T) {
	manager, data := newOfficialDataManager(t)
	ctx := context.Background()
	if _, err := manager.RefreshChanged(ctx); err != nil {
		t.Fatal(err)
	}
	first, _ := manager.Current()
	itemsBefore := data.itemDownloads.Load()
	data.set("1", "8")
	changed, err := manager.RefreshChanged(ctx)
	if err != nil || !changed {
		t.Fatalf("language version bump = changed %t, err %v; want changed", changed, err)
	}
	second, _ := manager.Current()
	if second == first {
		t.Fatal("published stores are immutable: a language change must publish a new store")
	}
	if second.Metadata().LanguageVersion != "8" || first.Metadata().LanguageVersion != "7" {
		t.Fatalf("language versions = %q (new) / %q (retired), want 8 / 7", second.Metadata().LanguageVersion, first.Metadata().LanguageVersion)
	}
	if data.itemDownloads.Load() != itemsBefore {
		t.Fatal("a language-only change re-downloaded the unchanged items document")
	}
}

func TestStoreDerivedBuildsOncePerKeyAndIsHeldByTheStore(t *testing.T) {
	store, err := DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	builds := 0
	build := func() any {
		builds++
		return builds
	}
	if first, second := store.Derived("k", build), store.Derived("k", build); first != 1 || second != 1 || builds != 1 {
		t.Fatalf("derived = %v/%v after %d builds, want one build", first, second, builds)
	}
	if other := store.Derived("other", build); other != 2 {
		t.Fatalf("second key = %v, want a separate build", other)
	}
	fresh, _ := DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), SourceMetadata{})
	if got := fresh.Derived("k", build); got != 3 {
		t.Fatalf("another store shared a derived value: %v", got)
	}
}
