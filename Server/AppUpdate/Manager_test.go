package AppUpdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckFindsNewerBackwardCompatibleRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://storage.googleapis.com/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := manager.Snapshot()
	if !snapshot.Available || snapshot.LatestVersion != "2.1.0" || snapshot.Status != "available" || snapshot.DownloadURL == "" {
		t.Fatalf("unexpected update snapshot: %+v", snapshot)
	}
}

func TestSemanticVersionComparison(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"2.0.0", "1.3.8", 1},
		{"v2.0", "2.0.0", 0},
		{"2.0.0", "2.0.0-beta.1", 1},
		{"2.0.0-beta.1", "2.0.0", -1},
	} {
		got := compareVersions(test.left, test.right)
		if got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestSemanticVersionValidationRejectsArbitraryText(t *testing.T) {
	for _, version := range []string{"", "latest", "2.x.0", "2.0.0/../../file", "2.0.0@bad"} {
		if isSemanticVersion(version) {
			t.Fatalf("expected %q to be rejected", version)
		}
	}
}

func TestCheckRejectsUntrustedArtifactHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://example.invalid/update"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://storage.googleapis.com/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected an untrusted download host to be rejected")
	}
}

func TestDownloadURLMatchesReleaseArtifactNames(t *testing.T) {
	if got := buildDownloadURL("https://example.test/releases/", "2.0.0", "windows", "amd64"); got != "https://example.test/releases/windows-citadel-ops-desktop-2.0.0.exe" {
		t.Fatalf("unexpected Windows artifact URL: %s", got)
	}
	if got := buildDownloadURL("https://example.test/releases", "2.0.0", "darwin", "arm64"); got != "https://example.test/releases/macos-arm64-citadel-ops-desktop-2.0.0" {
		t.Fatalf("unexpected macOS artifact URL: %s", got)
	}
}
