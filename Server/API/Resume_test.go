package API

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Diagnostics"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"github.com/gorilla/websocket"
)

type resumeRig struct {
	api           *Server
	server        *httptest.Server
	store         *State.Store
	configuration *Configuration.Store
	digest        string
}

func newResumeRig(t *testing.T) *resumeRig {
	t.Helper()
	initial := State.NewGameState()
	store := State.NewStore(&initial)
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"test": json.RawMessage(`{"enabled":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "Items-v1.json"), []byte(`{"versionInfo":{},"units":[],"buildings":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cache})
	if err := manager.LoadCache(); err != nil {
		t.Fatal(err)
	}
	api := NewServer(Config{State: store, Intents: Intent.NewEngine(nil, store, nil, nil, nil), Configuration: configuration, GameData: manager,
		Updates: AppUpdate.NewManager(AppUpdate.Config{CurrentVersion: "test"}), Diagnostics: Diagnostics.NewMonitor(t.TempDir())})
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	gameData, _ := manager.Current()
	return &resumeRig{api: api, server: server, store: store, configuration: configuration, digest: gameData.Metadata().DigestSHA256}
}

func (rig *resumeRig) cursor(since uint64) url.Values {
	return url.Values{"resume": {"1"}, "instance": {rig.store.Instance()}, "since": {strconv.FormatUint(since, 10)},
		"ops": {strconv.FormatUint(rig.api.config.Intents.EventSequence(), 10)}, "config": {strconv.FormatUint(rig.configuration.Snapshot().Revision, 10)}, "catalog": {rig.digest}}
}

// A request/reply marker delimits the entire greeting without timing-based
// absence assertions or introducing another state snapshot into the test.
func (rig *resumeRig) greeting(t *testing.T, query url.Values) []Envelope {
	t.Helper()
	suffix := ""
	if query != nil {
		suffix = "?" + query.Encode()
	}
	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(rig.server.URL, "http")+"/api/v2/events"+suffix, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := socket.WriteJSON(Envelope{Version: 2, ID: "end", Type: "greeting-marker"}); err != nil {
		t.Fatal(err)
	}
	_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	var frames []Envelope
	for {
		var frame Envelope
		if err := socket.ReadJSON(&frame); err != nil {
			t.Fatal(err)
		}
		if frame.ID == "end" {
			return frames
		}
		frames = append(frames, frame)
	}
}

func mutateResumePlayer(t *testing.T, store *State.Store, level int) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Level = level
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func frameTypes(frames []Envelope) []string {
	result := make([]string, 0, len(frames))
	for _, frame := range frames {
		result = append(result, frame.Type)
	}
	return result
}

func TestEventsResumeInsideWindowAndAtHead(t *testing.T) {
	rig := newResumeRig(t)
	mutateResumePlayer(t, rig.store, 10)
	mutateResumePlayer(t, rig.store, 11)
	frames := rig.greeting(t, rig.cursor(0))
	if got := frameTypes(frames); !reflect.DeepEqual(got, []string{"state.resumed", "state.changed", "update.changed"}) {
		t.Fatalf("greeting = %v", got)
	}
	var resumed struct {
		Instance            string
		From, Revision, Ops uint64
	}
	if err := json.Unmarshal(frames[0].Payload, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Instance != rig.store.Instance() || resumed.From != 0 || resumed.Revision != 2 || resumed.Ops != 0 {
		t.Fatalf("resume = %+v", resumed)
	}
	delta := frames[1]
	var event State.ClientStateEvent
	if err := json.Unmarshal(delta.Payload, &event); err != nil {
		t.Fatal(err)
	}
	if !delta.Gap || delta.BaseRevision == nil || *delta.BaseRevision != 0 || delta.Revision != 2 || event.Patch.Player.Level != 11 {
		t.Fatalf("delta = %+v", delta)
	}
	if got := frameTypes(rig.greeting(t, rig.cursor(2))); !reflect.DeepEqual(got, []string{"state.resumed", "update.changed"}) {
		t.Fatalf("at-head = %v", got)
	}
	recorder := httptest.NewRecorder()
	rig.api.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/diagnostics", nil))
	var counters struct{ EventsGreetings, EventsResumes uint64 }
	if err := json.Unmarshal(recorder.Body.Bytes(), &counters); err != nil {
		t.Fatal(err)
	}
	if counters.EventsGreetings != 2 || counters.EventsResumes != 2 {
		t.Fatalf("counters = %+v", counters)
	}
}

func TestEventsResumeFallbacksAndLegacyGreetingGolden(t *testing.T) {
	rig := newResumeRig(t)
	for level := 1; level <= 514; level++ {
		mutateResumePlayer(t, rig.store, level)
	}
	for _, test := range []string{"no parameters", "below floor", "above head", "instance changed", "missing instance", "malformed", "incomplete"} {
		t.Run(test, func(t *testing.T) {
			query := rig.cursor(514)
			switch test {
			case "no parameters":
				query = nil
			case "below floor":
				query.Set("since", "0")
			case "above head":
				query.Set("since", "515")
			case "instance changed":
				query.Set("instance", "another-process")
			case "missing instance":
				query.Del("instance")
			case "malformed":
				query.Set("ops", "NaN")
			case "incomplete":
				query.Del("config")
			}
			frames := rig.greeting(t, query)
			if got := frameTypes(frames); !reflect.DeepEqual(got, []string{"state.snapshot", "config.changed", "update.changed", "operations.snapshot", "catalog.changed"}) {
				t.Fatalf("fallback = %v", got)
			}
			if frames[0].Instance != rig.store.Instance() {
				t.Fatal("snapshot does not identify store")
			}
			// Today's envelope and payload bytes are preserved, apart from the
			// plan's additive instance and operation-sequence fields.
			view := rig.store.ReadOnlyView()
			gameData, _ := rig.api.config.GameData.Current()
			want := []Envelope{
				streamEnvelope("", "state.snapshot", view.Revision, view.Revision, false, State.NewClientStateSnapshot(&view)),
				streamEnvelope("", "config.changed", view.Revision, rig.configuration.Snapshot().Revision, false, rig.configuration.Snapshot()),
				newEnvelope("", "update.changed", view.Revision, rig.api.config.Updates.Snapshot()),
				newEnvelope("", "operations.snapshot", view.Revision, []Intent.Receipt{}),
				newEnvelope("", "catalog.changed", view.Revision, map[string]any{"metadata": gameData.Metadata(), "catalogs": gameData.Summaries()}),
			}
			frames[0].Instance = ""
			frames[3].Sequence = 0
			gotRaw, _ := json.Marshal(frames)
			wantRaw, _ := json.Marshal(want)
			if !bytes.Equal(gotRaw, wantRaw) {
				t.Fatal("legacy greeting changed beyond additive fields")
			}
		})
	}
}

func TestEventsResumeConditionalGreetingParts(t *testing.T) {
	for _, field := range []string{"ops", "config", "catalog"} {
		t.Run(field, func(t *testing.T) {
			rig := newResumeRig(t)
			query := rig.cursor(0)
			query.Set(field, "999")
			frames := rig.greeting(t, query)
			want := []string{"state.resumed"}
			if field == "config" {
				want = append(want, "config.changed")
			}
			want = append(want, "update.changed")
			if field == "ops" {
				want = append(want, "operations.snapshot")
			}
			if field == "catalog" {
				want = append(want, "catalog.changed")
			}
			if got := frameTypes(frames); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s mismatch = %v want %v", field, got, want)
			}
		})
	}
	// The hosted signal uses the canonical authority revision, not local revision.
	rig := newResumeRig(t)
	rig.api.SetExternalConfigurationAuthority(true)
	query := rig.cursor(0)
	query.Set("config", strconv.FormatUint(rig.api.configurationSignal().Revision, 10))
	if got := frameTypes(rig.greeting(t, query)); !reflect.DeepEqual(got, []string{"state.resumed", "update.changed"}) {
		t.Fatalf("canonical match = %v", got)
	}
}

func TestEventsResumeWhilePublishing(t *testing.T) {
	rig := newResumeRig(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for level := 1; level <= 100; level++ {
			_, err := rig.store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
				state.Player.Level = level
				return []string{"player"}, true, nil
			})
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for range 12 {
		frames := rig.greeting(t, rig.cursor(0))
		if frames[0].Type != "state.resumed" {
			t.Fatal("concurrent resume unexpectedly fell back")
		}
		for _, frame := range frames {
			if frame.Type == "state.changed" {
				var event State.ClientStateEvent
				if err := json.Unmarshal(frame.Payload, &event); err != nil {
					t.Fatal(err)
				}
				if event.Patch.Player.Level != int(frame.Revision) {
					t.Fatalf("patch/head mismatch: %s", fmt.Sprint(frame.Revision))
				}
			}
		}
	}
	<-done
}

func TestEventsOperationSnapshotUsesCapturedHead(t *testing.T) {
	rig := newResumeRig(t)
	rig.api.config.Intents.Submit(t.Context(), Intent.Request{ID: "op-before", Name: "missing.action"})
	sequence := rig.api.config.Intents.EventSequence()
	if sequence == 0 {
		t.Fatal("fixture did not publish operation history")
	}
	frames := rig.greeting(t, nil)
	for _, frame := range frames {
		if frame.Type == "operations.snapshot" {
			if frame.Sequence != sequence {
				t.Fatalf("snapshot label %d != head %d", frame.Sequence, sequence)
			}
			var receipts []Intent.Receipt
			if err := json.Unmarshal(frame.Payload, &receipts); err != nil {
				t.Fatal(err)
			}
			if len(receipts) != 1 || receipts[0].ID != "op-before" {
				t.Fatal("label does not cover history")
			}
		}
	}
	query := rig.cursor(0)
	if got := frameTypes(rig.greeting(t, query)); !reflect.DeepEqual(got, []string{"state.resumed", "update.changed"}) {
		t.Fatalf("matched operation head did not skip snapshot: %v", got)
	}
	rig.api.config.Intents.Submit(t.Context(), Intent.Request{ID: "op-after", Name: "missing.action"})
	frames = rig.greeting(t, query)
	if got := frameTypes(frames); !reflect.DeepEqual(got, []string{"state.resumed", "update.changed", "operations.snapshot"}) {
		t.Fatalf("missed operation did not restore snapshot: %v", got)
	}
	if frames[2].Sequence != rig.api.config.Intents.EventSequence() {
		t.Fatal("new snapshot has wrong label")
	}
}
