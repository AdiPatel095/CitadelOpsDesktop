package AppUpdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicSourceDoesNotEmbedDistributionOrigin(t *testing.T) {
	if strings.TrimSpace(DefaultDownloadBase) != "" {
		t.Fatalf("public source embeds a distribution origin: %s", DefaultDownloadBase)
	}
}

func TestCheckFindsNewerBackwardCompatibleRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://downloads.example.test/releases/citadel-ops-2.1.0","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
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
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://example.invalid/update","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected an untrusted download host to be rejected")
	}
}

func TestCheckRejectsSameHostOutsideDownloadBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://downloads.example.test/other-root/update","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected an artifact outside the configured download path to be rejected")
	}
}

func TestCheckRejectsMalformedChecksum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://downloads.example.test/releases/update","sha256":"not-a-sha256"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected a malformed artifact checksum to be rejected")
	}
}

func TestCheckRejectsPartialVersionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://downloads.example.test/releases/update"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected partial artifact metadata to be rejected")
	}
}

func TestCheckRejectsVersionOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		CurrentVersion: "2.0.0", Endpoint: server.URL,
		DownloadBaseURL: "https://downloads.example.test/releases", Client: server.Client(),
	})
	if err := manager.Check(context.Background()); err == nil {
		t.Fatal("expected version-only update metadata to be rejected")
	}
}

func TestDownloadURLRequiresExactTrustedBase(t *testing.T) {
	manager := NewManager(Config{DownloadBaseURL: "https://downloads.example.test/trusted-root"})
	if err := manager.validateDownloadURL("https://downloads.example.test/trusted-root/update.bin"); err != nil {
		t.Fatalf("trusted artifact was rejected: %v", err)
	}
	for _, candidate := range []string{
		"https://downloads.example.test/other-root/update.bin",
		"https://downloads.example.test/trusted-root-sibling/update.bin",
		"https://downloads.example.test/trusted-root/../other/update.bin",
		"https://downloads.example.test/trusted-root/%2e%2e/other.bin",
		"https://downloads.example.test/trusted-root/update.bin?generation=1",
		"https://downloads.example.test/trusted-root/update.bin#fragment",
		"https://user@downloads.example.test/trusted-root/update.bin",
		"https://downloads.example.test:444/trusted-root/update.bin",
	} {
		if err := manager.validateDownloadURL(candidate); err == nil {
			t.Fatalf("untrusted artifact URL was accepted: %s", candidate)
		}
	}
}

func TestInstallerRejectsMissingChecksumBeforeNetwork(t *testing.T) {
	called := false
	manager := NewManager(Config{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, fmt.Errorf("unexpected request")
	})}})
	if err := manager.downloadAndInstall(context.Background(), DefaultDownloadBase+"/update", ""); err == nil {
		t.Fatal("expected an absent checksum to be rejected")
	}
	if called {
		t.Fatal("installer made a network request before validating the checksum")
	}
}

func TestInstallerRejectsUntrustedURLBeforeNetwork(t *testing.T) {
	called := false
	manager := NewManager(Config{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, fmt.Errorf("unexpected request")
	})}})
	if err := manager.downloadAndInstall(context.Background(), "https://downloads.example.test/other-root/update", strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected an untrusted URL to be rejected")
	}
	if called {
		t.Fatal("installer made a network request before validating the URL")
	}
}

func TestInstallerRedirectOutsideTrustedBaseIsRejectedBeforeRequest(t *testing.T) {
	outsideRequested := false
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/downloads/update.bin":
			http.Redirect(writer, request, server.URL+"/outside", http.StatusFound)
		case "/outside":
			outsideRequested = true
			_, _ = writer.Write([]byte("unexpected"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	manager := NewManager(Config{
		DownloadBaseURL: server.URL + "/downloads", Client: server.Client(),
	})
	if err := manager.downloadAndInstall(context.Background(), server.URL+"/downloads/update.bin", strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected an off-base artifact redirect to be rejected")
	}
	if outsideRequested {
		t.Fatal("client requested the off-base artifact redirect target")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
