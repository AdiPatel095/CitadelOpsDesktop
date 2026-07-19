package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	baseURL  string
	http     *http.Client
	priority int
}

func main() {
	apiURL := flag.String("api", "http://127.0.0.1:8080", "CitadelOps API base URL")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	dryRun := flag.Bool("dry-run", false, "plan intents without executing them")
	priority := flag.Int("priority", 0, "outbound priority override from 1 to 100")
	expectedRevision := flag.Uint64("expected-revision", 0, "require this state revision for an intent")
	expectedConfigRevision := flag.Uint64("expected-config-revision", 0, "require this configuration revision for set-config")
	flag.Parse()
	if *priority < 0 || *priority > 100 {
		fail("priority must be 0 (automatic) or between 1 and 100")
	}
	arguments := flag.Args()
	if len(arguments) == 0 {
		usage()
	}

	api := client{
		baseURL:  strings.TrimRight(*apiURL, "/"),
		http:     &http.Client{Timeout: *timeout},
		priority: *priority,
	}
	command := strings.ToLower(arguments[0])
	var result json.RawMessage
	var err error
	switch command {
	case "health", "state", "browsers", "intents":
		result, err = api.get("/api/v2/" + command)
	case "catalogs":
		result, err = api.get("/api/v2/game-data")
	case "config":
		path := "/api/v2/config"
		if len(arguments) > 1 {
			path += "/" + url.PathEscape(arguments[1])
		}
		result, err = api.get(path)
	case "catalog":
		if len(arguments) < 2 {
			fail("catalog requires a collection name")
		}
		path := "/api/v2/game-data/" + url.PathEscape(arguments[1])
		if len(arguments) > 2 {
			path += "?id=" + url.QueryEscape(arguments[2])
		}
		result, err = api.get(path)
	case "projection":
		if len(arguments) < 2 {
			fail("projection requires a projection name")
		}
		result, err = api.get("/api/v2/projections/" + url.PathEscape(arguments[1]))
	case "history":
		if len(arguments) < 2 {
			fail("history requires player, spy, or battle")
		}
		path := map[string]string{
			"player": "/api/v2/history/player-tracker",
			"spy":    "/api/v2/history/spy-reports",
			"battle": "/api/v2/history/battle-reports",
		}[strings.ToLower(arguments[1])]
		if path == "" {
			fail("history requires player, spy, or battle")
		}
		if len(arguments) > 2 {
			parameter := "limit"
			if strings.EqualFold(arguments[1], "player") {
				parameter = "rangeSeconds"
			}
			path += "?" + parameter + "=" + url.QueryEscape(arguments[2])
		}
		result, err = api.get(path)
	case "telemetry":
		path := "/api/v2/telemetry/channels"
		if len(arguments) > 1 {
			path = "/api/v2/telemetry/" + url.PathEscape(arguments[1])
			if len(arguments) > 2 {
				path += "?limit=" + url.QueryEscape(arguments[2])
			}
		}
		result, err = api.get(path)
	case "operation":
		if len(arguments) < 2 {
			fail("operation requires an id")
		}
		result, err = api.get("/api/v2/operations/" + url.PathEscape(arguments[1]))
	case "localize":
		if len(arguments) < 2 {
			fail("localize requires at least one official language key")
		}
		result, err = api.post("/api/v2/game-data/localize", map[string]any{"keys": arguments[1:]})
	case "intent":
		if len(arguments) < 2 {
			fail("intent requires a registered intent name")
		}
		intentArguments := json.RawMessage(`{}`)
		if len(arguments) > 2 {
			intentArguments = json.RawMessage(arguments[2])
			if !json.Valid(intentArguments) {
				fail("intent arguments must be valid JSON")
			}
		}
		result, err = api.submitIntent(arguments[1], intentArguments, *dryRun, revisionPointer(*expectedRevision))
	case "start":
		result, err = api.submitIntent("session.start", json.RawMessage(`{}`), *dryRun, revisionPointer(*expectedRevision))
	case "stop":
		result, err = api.submitIntent("session.stop", json.RawMessage(`{}`), *dryRun, revisionPointer(*expectedRevision))
	case "refresh-data":
		result, err = api.submitIntent("game_data.refresh", json.RawMessage(`{}`), *dryRun, revisionPointer(*expectedRevision))
	case "select-browser":
		if len(arguments) < 2 {
			fail("select-browser requires a detected browser id or executable path")
		}
		encoded, _ := json.Marshal(map[string]string{"browser": arguments[1]})
		result, err = api.submitIntent("session.select_browser", encoded, *dryRun, revisionPointer(*expectedRevision))
	case "set-config":
		if len(arguments) < 3 {
			fail("set-config requires a section and JSON value")
		}
		value := json.RawMessage(arguments[2])
		if !json.Valid(value) {
			fail("configuration value must be valid JSON")
		}
		encoded, _ := json.Marshal(struct {
			Section          string          `json:"section"`
			Value            json.RawMessage `json:"value"`
			ExpectedRevision *uint64         `json:"expectedRevision,omitempty"`
		}{arguments[1], value, revisionPointer(*expectedConfigRevision)})
		result, err = api.submitIntent("config.update", encoded, *dryRun, revisionPointer(*expectedRevision))
	case "watch":
		err = api.watch()
	default:
		usage()
	}
	if err != nil {
		fail(err.Error())
	}
	if len(result) > 0 {
		printJSON(result)
	}
}

func (api client) get(path string) (json.RawMessage, error) {
	return api.request(http.MethodGet, path, nil)
}

func (api client) post(path string, body any) (json.RawMessage, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return api.request(http.MethodPost, path, encoded)
}

func (api client) submitIntent(name string, arguments json.RawMessage, dryRun bool, expectedRevision *uint64) (json.RawMessage, error) {
	body := struct {
		Actor            string          `json:"actor"`
		Priority         int             `json:"priority,omitempty"`
		Arguments        json.RawMessage `json:"arguments"`
		ExpectedRevision *uint64         `json:"expectedRevision,omitempty"`
		DryRun           bool            `json:"dryRun,omitempty"`
	}{
		Actor: "cli", Priority: api.priority, Arguments: arguments, ExpectedRevision: expectedRevision, DryRun: dryRun,
	}
	return api.post("/api/v2/intents/"+url.PathEscape(name), body)
}

func (api client) request(method string, path string, body []byte) (json.RawMessage, error) {
	request, err := http.NewRequest(method, api.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := api.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 256<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(contents)))
	}
	if !json.Valid(contents) {
		return nil, fmt.Errorf("API returned invalid JSON")
	}
	return json.RawMessage(contents), nil
}

func (api client) watch() error {
	parsed, err := url.Parse(api.baseURL)
	if err != nil {
		return err
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = "/api/v2/events"
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, parsed.String(), nil)
	if err != nil {
		return err
	}
	defer connection.Close()
	go func() {
		<-ctx.Done()
		_ = connection.Close()
	}()
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		printJSON(message)
	}
}

func printJSON(raw []byte) {
	var output bytes.Buffer
	if json.Indent(&output, raw, "", "  ") == nil {
		fmt.Println(output.String())
		return
	}
	fmt.Println(string(raw))
}

func revisionPointer(value uint64) *uint64 {
	if value == 0 {
		return nil
	}
	return &value
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: CitadelOpsCLI [flags] <command> [arguments]")
	fmt.Fprintln(os.Stderr, "commands: health, state, config, set-config, catalogs, catalog, projection, history, telemetry, browsers, intents, operation, localize, intent, start, stop, refresh-data, select-browser, watch")
	fmt.Fprintln(os.Stderr, "example: CitadelOpsCLI intent game.refresh_movements '{}'")
	os.Exit(2)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "CitadelOpsCLI:", message)
	os.Exit(1)
}
