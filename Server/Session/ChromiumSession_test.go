package Session

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseDevToolsActivePort(t *testing.T) {
	port, websocketPath, err := parseDevToolsActivePort(
		[]byte("43123\n/devtools/browser/session-id\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if port != 43123 || websocketPath != "/devtools/browser/session-id" {
		t.Fatalf("unexpected endpoint: port=%d path=%q", port, websocketPath)
	}
	for _, contents := range []string{
		"",
		"43123\n",
		"invalid\n/devtools/browser/session-id\n",
		"43123\n/devtools/page/session-id\n",
	} {
		if _, _, err := parseDevToolsActivePort([]byte(contents)); err == nil {
			t.Fatalf("accepted invalid DevToolsActivePort %q", contents)
		}
	}
}

func TestLiveChromiumEndpointUsesProfilePortFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/json/version" {
			http.NotFound(writer, request)
			return
		}
		port := request.Context().Value(http.LocalAddrContextKey).(net.Addr).(*net.TCPAddr).Port
		_, _ = fmt.Fprintf(
			writer,
			`{"webSocketDebuggerUrl":"ws://127.0.0.1:%d/devtools/browser/live"}`,
			port,
		)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	profileDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(profileDir, "DevToolsActivePort"),
		[]byte(fmt.Sprintf("%d\n/devtools/browser/live\n", port)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	endpoint, err := liveChromiumEndpoint(profileDir)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/live", port)
	if endpoint != want {
		t.Fatalf("endpoint = %q, want %q", endpoint, want)
	}
}

func TestChromiumLaunchArgumentsKeepReusableProfileEndpoint(t *testing.T) {
	arguments := chromiumLaunchArguments("/tmp/Citadel Profile", "http://127.0.0.1:8080/", false)
	joined := strings.Join(arguments, "\n")
	for _, required := range []string{
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"--user-data-dir=/tmp/Citadel Profile",
		"--disable-site-isolation-trials",
		"--disable-features=IsolateOrigins,site-per-process",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("launch arguments are missing %q", required)
		}
	}
	if arguments[len(arguments)-1] != "http://127.0.0.1:8080/" {
		t.Fatalf("launch URL = %q", arguments[len(arguments)-1])
	}
}

func TestGamePageURLMatchesOnlyGameOrigins(t *testing.T) {
	for _, value := range []string{
		"https://empire.goodgamestudios.com/",
		"https://empire-html5.goodgamestudios.com/default/",
	} {
		if !isGamePageURL(value) {
			t.Errorf("game page URL did not match %q", value)
		}
	}
	for _, value := range []string{
		"http://127.0.0.1:8080/",
		"https://example.com/empire.goodgamestudios.com/",
		"about:blank",
	} {
		if isGamePageURL(value) {
			t.Errorf("non-game page URL matched %q", value)
		}
	}
}
