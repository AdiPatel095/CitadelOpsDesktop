package GameData

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func localeTestManager(t *testing.T, handler http.HandlerFunc) *Manager {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	manager := NewManager(UpdaterConfig{CacheDir: t.TempDir(), LanguageMetadataURL: server.URL + "/metadata", LanguageURL: server.URL + "/{language}/{version}", RequestTimeout: time.Second})
	manager.language, _ = DecodeLanguage([]byte(`{"hello":"Hello","onlyEnglish":"Fallback","markup":"<b>{0}</b>"}`), LanguageMetadata{Language: "en", Version: "7"})
	return manager
}

func TestOfficialLocaleManifest(t *testing.T) {
	locales := OfficialLocales()
	if len(locales) != 26 {
		t.Fatalf("locales=%d", len(locales))
	}
	seen := map[string]bool{}
	for _, locale := range locales {
		if seen[locale.Code] || locale.NativeName == "" || locale.Name == "" {
			t.Fatalf("invalid locale: %+v", locale)
		}
		seen[locale.Code] = true
		canonical, err := NormalizeLocale(locale.GameCode)
		if err != nil || canonical != locale {
			t.Fatalf("alias %s: %+v %v", locale.GameCode, canonical, err)
		}
		if (locale.Direction == "rtl") != (locale.Code == "ar") {
			t.Fatal("direction")
		}
	}
	for _, code := range []string{"../../en", "fr/../en", "en-US", "unknown", "fr?foo", "*"} {
		if _, err := NormalizeLocale(code); err == nil {
			t.Fatalf("accepted %q", code)
		}
	}
	locales[0].Name = "mutated"
	if OfficialLocales()[0].Name == "mutated" {
		t.Fatal("manifest leaked mutable slice")
	}
}

func TestLocaleIsolationDedupFallbackAndPlaceholders(t *testing.T) {
	var downloads atomic.Int32
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metadata" {
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"}}`)
			return
		}
		downloads.Add(1)
		fmt.Fprintf(w, `{"@metadata":{"versionNo":"7"},"hello":%q,"markup":"<b>{0}</b>","MixedCase":"local"}`, r.URL.Path)
	})
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			language, resolution, err := manager.LanguageFor(context.Background(), "fr")
			if err != nil || resolution.ResolvedLocale != "fr" {
				t.Errorf("resolution: %+v %v", resolution, err)
				return
			}
			if v, _ := language.Text("hello"); v != "/fr/7" {
				t.Errorf("French=%s", v)
			}
		}()
	}
	wait.Wait()
	if downloads.Load() != 1 {
		t.Fatalf("duplicate downloads=%d", downloads.Load())
	}
	de, _, err := manager.LanguageFor(context.Background(), "de")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := de.Text("hello"); v != "/de/7" {
		t.Fatal(v)
	}
	if v, _ := de.Text("onlyEnglish"); v != "Fallback" {
		t.Fatal(v)
	}
	if v := de.ResolveMany([]string{"mixedcase"})["mixedcase"]; v != "local" {
		t.Fatal(v)
	}
	values := de.Values()
	if values["markup"] != "<b>{0}</b>" {
		t.Fatal("markup changed")
	}
	values["hello"] = "mutated"
	if v, _ := de.Text("hello"); v != "/de/7" {
		t.Fatal("mutable dictionary leaked")
	}
	original, _ := manager.Language()
	if v, _ := original.Text("hello"); v != "Hello" {
		t.Fatal("runtime language changed")
	}
}

func TestLocaleOfflineValidatedCacheThenEnglish(t *testing.T) {
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) })
	os.WriteFile(filepath.Join(manager.config.CacheDir, "Language-fr-v6.json"), []byte(`{"@metadata":{"versionNo":"6"},"hello":"Bonjour"}`), 0600)
	os.WriteFile(filepath.Join(manager.config.CacheDir, "Language-fr-v8.json"), []byte(`{"@metadata":{"versionNo":"99"},"hello":"wrong version"}`), 0600)
	fr, res, err := manager.LanguageFor(context.Background(), "fr")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := fr.Text("hello"); v != "Bonjour" || res.Source != "cache" || res.Fallback {
		t.Fatalf("%q %+v", v, res)
	}
	_, res, err = manager.LanguageFor(context.Background(), "de")
	if err != nil || !res.Fallback || res.ResolvedLocale != "en" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestLocaleCancellationDoesNotPoisonOtherWaiters(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metadata" {
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"}}`)
			return
		}
		close(started)
		<-release
		fmt.Fprint(w, `{"@metadata":{"versionNo":"7"},"hello":"Bonjour"}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, _, err := manager.LanguageFor(ctx, "fr"); result <- err }()
	<-started
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatal(err)
	}
	close(release)
	_, res, err := manager.LanguageFor(context.Background(), "fr")
	if err != nil || res.Fallback {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestLocaleCacheBoundAndInvalidInputNoFetch(t *testing.T) {
	var requests atomic.Int32
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/metadata" {
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"}}`)
			return
		}
		fmt.Fprint(w, `{"@metadata":{"versionNo":"7"},"hello":"localized"}`)
	})
	if _, _, err := manager.LanguageFor(context.Background(), "../fr"); err == nil || requests.Load() != 0 {
		t.Fatal("unsafe input fetched")
	}
	for _, code := range []string{"de", "fr", "pl", "it", "nl", "pt"} {
		if _, _, err := manager.LanguageFor(context.Background(), code); err != nil {
			t.Fatal(err)
		}
	}
	manager.localeMu.Lock()
	defer manager.localeMu.Unlock()
	if len(manager.localeCache) > localeCacheLimit {
		t.Fatal("unbounded cache")
	}
}

func TestLocaleRejectsUnsafeVersionAndMalformedDownload(t *testing.T) {
	for _, body := range []string{`{"@metadata":{"versionNo":"../../x"}}`, `{"@metadata":{"versionNo":"7"}}`} {
		t.Run(strings.ReplaceAll(body, "/", "_"), func(t *testing.T) {
			manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/metadata" {
					fmt.Fprint(w, body)
					return
				}
				fmt.Fprint(w, `{"@metadata":{"versionNo":"8"},"hello":"wrong"}`)
			})
			_, res, err := manager.LanguageFor(context.Background(), "fr")
			if err != nil || !res.Fallback {
				t.Fatalf("%+v %v", res, err)
			}
			files, _ := filepath.Glob(filepath.Join(manager.config.CacheDir, "*"))
			if len(files) != 0 {
				t.Fatal("persisted invalid document")
			}
		})
	}
}

func TestLocaleRuntimeRefreshInvalidatesViewerCache(t *testing.T) {
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) })
	before, _, err := manager.LanguageFor(context.Background(), "en")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, _ := DecodeLanguage([]byte(`{"hello":"New English"}`), LanguageMetadata{Language: "en", Version: "8"})
	manager.mu.Lock()
	manager.language = refreshed
	manager.mu.Unlock()
	after, _, err := manager.LanguageFor(context.Background(), "en")
	if err != nil || after == before {
		t.Fatalf("stale dictionary %v", err)
	}
	if text, _ := after.Text("hello"); text != "New English" {
		t.Fatal(text)
	}
}

func TestLocaleMissingKeyProvenanceAndPriority(t *testing.T) {
	en, _ := DecodeLanguage([]byte(`{"first":"English first","missing":"fallback"}`), LanguageMetadata{Language: "en"})
	fr, _ := DecodeLanguage([]byte(`{"second":"French second"}`), LanguageMetadata{Language: "fr"})
	fr.fallback = en
	if value, _ := fr.Resolve("first", "second"); value != "French second" {
		t.Fatal("English took priority over requested language")
	}
	if keys := fr.FallbackKeys(); len(keys) != 2 || keys[0] != "first" || keys[1] != "missing" {
		t.Fatal(keys)
	}
}

func TestLocaleNegativeCacheAvoidsRepeatedOfflineRequests(t *testing.T) {
	var requests atomic.Int32
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) { requests.Add(1); http.Error(w, "offline", 503) })
	manager.language = nil
	for i := 0; i < 2; i++ {
		if _, _, err := manager.LanguageFor(context.Background(), "fr"); err == nil {
			t.Fatal("expected unavailable")
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d, expected one French and one English attempt", requests.Load())
	}
}

func TestLocaleBlankValuesUseEnglishConsistently(t *testing.T) {
	en, _ := DecodeLanguage([]byte(`{"blank":"English","missing":"Other"}`), LanguageMetadata{})
	fr, _ := DecodeLanguage([]byte(`{"blank":"  "}`), LanguageMetadata{})
	fr.fallback = en
	if v, _ := fr.Text("blank"); v != "English" {
		t.Fatal(v)
	}
	if v, _ := fr.Resolve("blank"); v != "English" {
		t.Fatal(v)
	}
	if v := fr.ResolveMany([]string{"blank"})["blank"]; v != "English" {
		t.Fatal(v)
	}
	if v := fr.Values()["blank"]; v != "English" {
		t.Fatal(v)
	}
	if keys := fr.FallbackKeys(); len(keys) != 2 {
		t.Fatal(keys)
	}
	if keys := fr.FallbackKeysFor([]string{"blank"}); len(keys) != 1 || keys[0] != "blank" {
		t.Fatal(keys)
	}
}

func TestChineseServiceCodesAreCaseSensitive(t *testing.T) {
	manager := localeTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata":
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"}}`)
		case "/zh_CN/7":
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"},"castle":"城堡","equipment":"装备"}`)
		case "/zh_TW/7":
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"},"castle":"城堡","equipment":"裝備"}`)
		default:
			http.Error(w, "unsupported case-sensitive service code", 500)
		}
	})
	for _, test := range []struct{ alias, canonical, gameCode, equipment string }{{"zh_cn", "zh-CN", "zh_CN", "装备"}, {"zh_tw", "zh-TW", "zh_TW", "裝備"}} {
		language, res, err := manager.LanguageFor(context.Background(), test.alias)
		if err != nil || res.Fallback || res.ResolvedLocale != test.canonical {
			t.Fatalf("%+v %v", res, err)
		}
		if value, _ := language.Text("equipment"); value != test.equipment {
			t.Fatal(value)
		}
		if _, err := os.Stat(filepath.Join(manager.config.CacheDir, "Language-"+test.gameCode+"-v7.json")); err != nil {
			t.Fatal(err)
		}
	}
}
