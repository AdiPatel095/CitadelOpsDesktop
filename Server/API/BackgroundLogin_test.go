package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"CitadelDesktop/Server/Session"
)

func TestBackgroundLoginAPIStoresCredentialsWithoutReturningThem(t *testing.T) {
	loginStore := Session.NewBackgroundLoginStore(t.TempDir())
	handler := NewServer(Config{BackgroundLogin: loginStore}).Handler()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/session/background-login", strings.NewReader(`{
		"username":"private-player","password":"private-password","server":"us1"
	}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure background login returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); strings.Contains(body, "private-player") || strings.Contains(body, "private-password") {
		t.Fatalf("background login response exposed credentials: %s", body)
	}
	var status Session.BackgroundLoginStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Server != "US1" ||
		status.ServerURL != "wss://ep-live-us1-game.goodgamestudios.com:443" {
		t.Fatalf("unexpected configured status: %+v", status)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("background login cache control = %q", cacheControl)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v2/session/background-login", nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("read background login returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); strings.Contains(body, "private-player") || strings.Contains(body, "private-password") {
		t.Fatalf("background login status exposed credentials: %s", body)
	}
}

func TestBackgroundLoginAPIRejectsUnknownOrInvalidInput(t *testing.T) {
	handler := NewServer(Config{BackgroundLogin: Session.NewBackgroundLoginStore(t.TempDir())}).Handler()
	for _, body := range []string{
		`{"username":"player","password":"password","server":"US1","token":"secret"}`,
		`{"username":"player","password":"password","server":"https://example.com"}`,
		`{"username":"player","password":"password","server":"US1"}{}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v2/session/background-login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(recorder, request)
		if recorder.Code < 400 {
			t.Fatalf("invalid background login returned HTTP %d: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestBackgroundLoginAPIRejectsCrossOriginWrites(t *testing.T) {
	handler := NewServer(Config{BackgroundLogin: Session.NewBackgroundLoginStore(t.TempDir())}).Handler()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/session/background-login",
		strings.NewReader(`{"username":"player","password":"password","server":"US1"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin background login returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
}
