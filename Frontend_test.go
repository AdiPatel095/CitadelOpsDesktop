package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendFileHandlerDoesNotServeIndexForMissingHashedAsset(t *testing.T) {
	handler := frontendFileHandler(fstest.MapFS{
		"index.html":         {Data: []byte("<html>dashboard</html>")},
		"assets/current.js":  {Data: []byte("export const current = true")},
		"assets/current.css": {Data: []byte("body{}")},
	})

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/stale-hash.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want 404", missing.Code)
	}
	if strings.Contains(missing.Body.String(), "dashboard") {
		t.Fatalf("missing JavaScript asset received the SPA index: %q", missing.Body.String())
	}

	route := httptest.NewRecorder()
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/automation", nil))
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "dashboard") {
		t.Fatalf("SPA route status=%d body=%q", route.Code, route.Body.String())
	}
	if cacheControl := route.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("SPA index Cache-Control = %q, want no-store", cacheControl)
	}
}
