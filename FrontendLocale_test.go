package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/GameData"
)

func TestBootstrapCatalogCompletenessAndProvenance(t *testing.T) {
	hash := func(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }
	catalog := bootstrapCatalog
	if catalog.SchemaVersion != 1 || catalog.Source.Key != "bootstrap.missingClient" || catalog.Source.Text != missingFrontendEnglish || catalog.Source.SHA256 != hash(missingFrontendEnglish) {
		t.Fatal("bootstrap source identity drift")
	}
	if catalog.Authorship.Method != "direct-model-authored" || catalog.Authorship.NativeSpeakerReview {
		t.Fatal("unexpected authorship claim")
	}
	if len(catalog.Translations) != len(GameData.OfficialLocales()) {
		t.Fatal("locale count mismatch")
	}
	for _, locale := range GameData.OfficialLocales() {
		entry, ok := catalog.Translations[locale.Code]
		if !ok || strings.TrimSpace(entry.Text) == "" || entry.SourceSHA256 != catalog.Source.SHA256 || entry.SHA256 != hash(entry.Text) {
			t.Errorf("invalid provenance for %s", locale.Code)
		}
		if strings.Count(entry.Text, "Client/") != 1 || strings.Count(entry.Text, "Vite") != 1 {
			t.Errorf("technical literals changed in %s", locale.Code)
		}
		if locale.Code != "en" && entry.Text == missingFrontendEnglish {
			t.Errorf("English copy in %s", locale.Code)
		}
	}
	if catalog.Translations["en"].Text != missingFrontendEnglish {
		t.Fatal("English response changed")
	}
}

func TestMissingFrontendNegotiatesRequestLocale(t *testing.T) {
	tests := []struct{ name, query, header, want string }{
		{"default", "", "", "en"},
		{"explicit", "?locale=fr", "de", "fr"},
		{"invalid explicit", "?locale=../../fr", "fr", "en"},
		{"empty explicit", "?locale=", "fr", "en"},
		{"underscore alias", "?locale=ZH_tw", "", "zh-TW"},
		{"weighted", "", "fr;q=0.4,de;q=0.9", "de"},
		{"excluded", "", "fr;q=0,de;q=0.8", "de"},
		{"regional", "", "pt-BR,fr;q=0.5", "pt"},
		{"traditional script", "", "zh-Hant-HK", "zh-TW"},
		{"simplified script", "", "zh-Hans-SG", "zh-CN"},
		{"Norwegian Bokmal", "", "nb-NO", "no"},
		{"unsupported", "", "xx", "en"},
		{"malformed", "", "fr;q=broken", "en"},
		{"oversized", "", strings.Repeat("fr,", 3000), "en"},
	}
	// Exercise the actual empty-assets fallback, independent of any Client bundle.
	handler := frontendFileHandler(fstest.MapFS{})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/automation"+test.query, nil)
			request.Header.Set("Accept-Language", test.header)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if got := response.Header().Get("Content-Language"); got != test.want {
				t.Fatalf("locale %q, want %q", got, test.want)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Vary") != "Accept-Language" {
				t.Fatal("missing cache isolation")
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 3 || body["name"] != "CitadelOps" || body["version"] != App.Version || body["detail"] != bootstrapCatalog.Translations[test.want].Text {
				t.Fatalf("unexpected response: %v", body)
			}
		})
	}
}

func TestMissingFrontendConcurrentLocaleIsolation(t *testing.T) {
	handler := missingFrontendHandler()
	var workers sync.WaitGroup
	for _, locale := range GameData.OfficialLocales() {
		workers.Add(1)
		go func(code string) {
			defer workers.Done()
			for i := 0; i < 10; i++ {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?locale="+code, nil))
				if response.Header().Get("Content-Language") != code || !strings.Contains(response.Body.String(), bootstrapCatalog.Translations[code].Text) {
					t.Errorf("locale isolation failed for %s", code)
				}
			}
		}(locale.Code)
	}
	workers.Wait()
}
