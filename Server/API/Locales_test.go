package API

import (
	"CitadelDesktop/Server/GameData"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localeAPI(t *testing.T) http.Handler {
	t.Helper()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metadata" {
			fmt.Fprint(w, `{"@metadata":{"versionNo":"7"}}`)
			return
		}
		fmt.Fprint(w, `{"@metadata":{"versionNo":"7"},"hello":"Bonjour","Farm_name":"Ferme","markup":"<b>{0}</b>"}`)
	}))
	t.Cleanup(origin.Close)
	cache := t.TempDir()
	for name, body := range map[string]string{
		"Items-v1.json":       `{"versionInfo":{},"buildings":[{"wodID":"1","type":"Farm","name":"Farm","width":"1","height":"1"}],"units":[]}`,
		"Language-en-v7.json": `{"@metadata":{"versionNo":"7"},"hello":"Hello","Farm_name":"Farm","missing":"English fallback"}`,
	} {
		if err := os.WriteFile(filepath.Join(cache, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cache, LanguageMetadataURL: origin.URL + "/metadata", LanguageURL: origin.URL + "/{language}/{version}"})
	if err := manager.LoadCache(); err != nil {
		t.Fatal(err)
	}
	return NewServer(Config{GameData: manager}).Handler()
}

func TestLocaleEndpointsAndLegacyCompatibility(t *testing.T) {
	handler := localeAPI(t)
	for _, test := range []struct {
		method, url, body string
		status            int
		contains          string
	}{
		{"GET", "/api/v2/locales", "", 200, `"defaultLocale":"en"`},
		{"GET", "/api/locales", "", 200, `"code":"lt"`},
		{"POST", "/api/v2/game-data/localize", `{"keys":["hello"]}`, 200, `"hello":"Hello"`},
		{"POST", "/api/v2/game-data/localize?locale=fr", `{"keys":["hello","missing"]}`, 200, `"hello":"Bonjour"`},
		{"GET", "/api/v2/game-data/translations?locale=fr", "", 200, `"missing":"English fallback"`},
		{"GET", "/api/v2/game-data?locale=fr", "", 200, `"resolvedLocale":"fr"`},
		{"GET", "/api/v2/game-data/buildings?locale=fr", "", 200, `"resolvedLocale":"fr"`},
		{"GET", "/api/v2/buildings/catalog?locale=fr&q=Ferme", "", 200, `"displayName":"Ferme"`},
		{"GET", "/api/v2/buildings/catalog?locale=en", "", 200, `"displayName":"Farm"`},
		{"GET", "/api/v2/game-data/translations?locale=..%2Ffr", "", 400, `"invalid_locale"`},
		{"POST", "/api/v2/game-data/localize?locale=not-supported", `{"keys":["hello"]}`, 400, `"invalid_locale"`},
	} {
		t.Run(test.url+test.body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.url, strings.NewReader(test.body)))
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.contains) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v2/game-data/translations?locale=fr", nil))
	var response struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Values["markup"] != "<b>{0}</b>" {
		t.Fatal("markup/interpolation was changed")
	}
	// Re-request English after French: locale cache cannot contaminate the default.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/v2/game-data/localize", strings.NewReader(`{"keys":["hello"]}`)))
	if !strings.Contains(recorder.Body.String(), `"hello":"Hello"`) {
		t.Fatal(recorder.Body.String())
	}
}
