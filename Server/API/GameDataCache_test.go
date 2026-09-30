package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
)

func catalogDigest(t *testing.T, handler http.Handler) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/game-data", nil))
	var manifest struct {
		Metadata struct {
			Digest string `json:"digestSha256"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil || manifest.Metadata.Digest == "" {
		t.Fatalf("manifest = %d %s (%v)", recorder.Code, recorder.Body.String(), err)
	}
	return manifest.Metadata.Digest
}

func getCollection(handler http.Handler, url string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, url, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestCollectionNamedByItsDigestIsImmutableAndRevalidatesWithAnETag(t *testing.T) {
	handler := localeAPI(t)
	digest := catalogDigest(t, handler)
	url := "/api/v2/game-data/buildings?locale=fr&digest=" + digest
	first := getCollection(handler, url, nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || etag == "" || !strings.Contains(first.Body.String(), `"Farm"`) {
		t.Fatalf("first read = %d %v", first.Code, first.Header())
	}
	revalidated := getCollection(handler, url, map[string]string{"If-None-Match": etag})
	if revalidated.Code != http.StatusNotModified || revalidated.Body.Len() != 0 || revalidated.Header().Get("Cache-Control") != first.Header().Get("Cache-Control") {
		t.Fatalf("conditional read = %d body %d bytes %v", revalidated.Code, revalidated.Body.Len(), revalidated.Header())
	}
	for _, header := range []string{`"other", ` + etag, "*", "W/" + etag} {
		if recorder := getCollection(handler, url, map[string]string{"If-None-Match": header}); recorder.Code != http.StatusNotModified {
			t.Errorf("If-None-Match %q = %d, want 304", header, recorder.Code)
		}
	}
	if recorder := getCollection(handler, url, map[string]string{"If-None-Match": `"stale"`}); recorder.Code != http.StatusOK {
		t.Errorf("a different validator = %d, want a full 200", recorder.Code)
	}
}

func TestCollectionWithoutADigestRevalidatesInsteadOfCachingForever(t *testing.T) {
	handler := localeAPI(t)
	first := getCollection(handler, "/api/v2/game-data/buildings", nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "no-cache" || etag == "" {
		t.Fatalf("read without a digest = %d %v", first.Code, first.Header())
	}
	if again := getCollection(handler, "/api/v2/game-data/buildings", map[string]string{"If-None-Match": etag}); again.Code != http.StatusNotModified {
		t.Fatalf("revalidation = %d, want 304", again.Code)
	}
}

// A stale manifest must never poison the cache: the current content is served under a URL that names
// different content, so it is marked no-store and carries no validator.
func TestCollectionNamingAnotherDigestIsServedButNeverCached(t *testing.T) {
	handler := localeAPI(t)
	current := catalogDigest(t, handler)
	stale := strings.Repeat("0", 64)
	recorder := getCollection(handler, "/api/v2/game-data/buildings?digest="+stale, nil)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("ETag") != "" ||
		recorder.Header().Get("X-Catalog-Digest") != current || !strings.Contains(recorder.Body.String(), `"Farm"`) {
		t.Fatalf("stale-digest read = %d %v", recorder.Code, recorder.Header())
	}
	if conditional := getCollection(handler, "/api/v2/game-data/buildings?digest="+stale, map[string]string{"If-None-Match": "*"}); conditional.Code != http.StatusOK {
		t.Fatalf("a stale digest was answered 304: %d", conditional.Code)
	}
}

func TestCollectionCacheKeyIncludesTheLocaleAndCollection(t *testing.T) {
	handler := localeAPI(t)
	digest := catalogDigest(t, handler)
	tags := map[string]bool{}
	for _, url := range []string{
		"/api/v2/game-data/buildings?digest=" + digest,
		"/api/v2/game-data/buildings?locale=en&digest=" + digest,
		"/api/v2/game-data/buildings?locale=fr&digest=" + digest,
		"/api/v2/game-data/units?locale=fr&digest=" + digest,
	} {
		tag := getCollection(handler, url, nil).Header().Get("ETag")
		if tag == "" || tags[tag] {
			t.Fatalf("%s: validator %q is empty or shared with another key", url, tag)
		}
		tags[tag] = true
	}
	// A validator from one language never validates another.
	fr := getCollection(handler, "/api/v2/game-data/buildings?locale=fr&digest="+digest, nil).Header().Get("ETag")
	if recorder := getCollection(handler, "/api/v2/game-data/buildings?locale=en&digest="+digest, map[string]string{"If-None-Match": fr}); recorder.Code != http.StatusOK {
		t.Fatalf("a French validator answered an English read with %d", recorder.Code)
	}
}

func TestNewGameDataVersionChangesTheDigestAndTheKey(t *testing.T) {
	build := func(buildings string) http.Handler {
		cache := t.TempDir()
		if err := os.WriteFile(filepath.Join(cache, "Items-v1.json"), []byte(`{"versionInfo":{},"buildings":`+buildings+`,"units":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cache})
		if err := manager.LoadCache(); err != nil {
			t.Fatal(err)
		}
		return NewServer(Config{GameData: manager}).Handler()
	}
	before := build(`[{"wodID":"1","type":"Farm","name":"Farm","width":"1","height":"1"}]`)
	after := build(`[{"wodID":"1","type":"Farm","name":"Farm","width":"2","height":"2"}]`)
	oldDigest, newDigest := catalogDigest(t, before), catalogDigest(t, after)
	if oldDigest == newDigest {
		t.Fatal("different content produced the same digest")
	}
	oldTag := getCollection(before, "/api/v2/game-data/buildings?digest="+oldDigest, nil).Header().Get("ETag")
	newRead := getCollection(after, "/api/v2/game-data/buildings?digest="+newDigest, map[string]string{"If-None-Match": oldTag})
	if newRead.Code != http.StatusOK || newRead.Header().Get("ETag") == oldTag {
		t.Fatalf("the new version reused the old key: %d %v", newRead.Code, newRead.Header())
	}
	// The old URL, asked of the new worker, is the stale case: content served, never cached.
	if stale := getCollection(after, "/api/v2/game-data/buildings?digest="+oldDigest, nil); stale.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("the old digest against the new data = %v", stale.Header())
	}
}

func TestItemLookupAndManifestKeepTheirHeaders(t *testing.T) {
	handler := localeAPI(t)
	for _, url := range []string{"/api/v2/game-data", "/api/v2/game-data/buildings?id=1"} {
		recorder := getCollection(handler, url, nil)
		if recorder.Header().Get("ETag") != "" || strings.Contains(recorder.Header().Get("Cache-Control"), "immutable") {
			t.Errorf("%s gained caching headers: %v", url, recorder.Header())
		}
	}
}
