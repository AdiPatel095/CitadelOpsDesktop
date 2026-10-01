package API

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type signalRig struct {
	store         *State.Store
	configuration *Configuration.Store
	socket        *websocket.Conn
	frames        chan Envelope
}

func newSignalRig(t *testing.T, config Config) *signalRig {
	t.Helper()
	accessorState1 := State.NewGameState()
	store := State.NewStore(&accessorState1)
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"scheduler": json.RawMessage(`{"minAttackDelay":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	config.State, config.Configuration = store, configuration
	config.Intents = Intent.NewEngine(nil, store, nil, nil, nil)
	config.GameData = GameData.NewManager(GameData.UpdaterConfig{})
	server := httptest.NewServer(NewServer(config).Handler())
	t.Cleanup(server.Close)
	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v2/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	// A read that times out poisons a gorilla connection, so one goroutine owns
	// the reads and tests wait on a channel instead.
	frames := make(chan Envelope, 256)
	go func() {
		defer close(frames)
		for {
			var envelope Envelope
			if err := socket.ReadJSON(&envelope); err != nil {
				return
			}
			frames <- envelope
		}
	}()
	return &signalRig{store: store, configuration: configuration, socket: socket, frames: frames}
}

func (rig *signalRig) next(t *testing.T, kind string, within time.Duration) (Envelope, bool) {
	t.Helper()
	timer := time.NewTimer(within)
	defer timer.Stop()
	for {
		select {
		case envelope, open := <-rig.frames:
			if !open {
				return Envelope{}, false
			}
			if envelope.Type == kind {
				return envelope, true
			}
		case <-timer.C:
			return Envelope{}, false
		}
	}
}

func signalOf(t *testing.T, envelope Envelope) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestExternalConfigurationSendsOnlyTheCanonicalVersion(t *testing.T) {
	rig := newSignalRig(t, Config{BackgroundOnly: true})
	greeting, ok := rig.next(t, "config.changed", 5*time.Second)
	if !ok {
		t.Fatal("no config.changed on connect")
	}
	fields := signalOf(t, greeting)
	if len(fields) != 3 || string(fields["external"]) != "true" || string(fields["revision"]) != "0" || string(fields["digest"]) != `""` {
		t.Fatalf("greeting payload = %v, want only revision, digest and external", fields)
	}
	if _, full := fields["sections"]; full {
		t.Fatal("the greeting carried a full snapshot")
	}

	digest := strings.Repeat("a", 64)
	rig.configuration.SetAuthorityVersion(7, digest)
	changed, ok := rig.next(t, "config.changed", 5*time.Second)
	if !ok {
		t.Fatal("no config.changed after the authority applied a new version")
	}
	fields = signalOf(t, changed)
	if string(fields["revision"]) != "7" || string(fields["digest"]) != `"`+digest+`"` || changed.Gap {
		t.Fatalf("change payload = %v gap=%v", fields, changed.Gap)
	}

	// A local section change (installation-scoped settings) leaves the canonical
	// version alone, and setting the same version again is not news either.
	if _, err := rig.configuration.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}
	rig.configuration.SetAuthorityVersion(7, digest)
	if unexpected, got := rig.next(t, "config.changed", 400*time.Millisecond); got {
		t.Fatalf("unexpected config.changed: %s", unexpected.Payload)
	}

	rig.configuration.SetAuthorityVersion(8, strings.Repeat("b", 64))
	if next, got := rig.next(t, "config.changed", 5*time.Second); !got || string(signalOf(t, next)["revision"]) != "8" {
		t.Fatalf("revision 8 was not signalled: %v %v", next, got)
	}

	if err := rig.socket.WriteJSON(Envelope{Version: ContractVersion, ID: "q1", Type: "query.config"}); err != nil {
		t.Fatal(err)
	}
	reply, ok := rig.next(t, "config.changed", 5*time.Second)
	fields = signalOf(t, reply)
	if !ok || reply.ID != "q1" || string(fields["revision"]) != "8" || fields["sections"] != nil {
		t.Fatalf("query.config reply = %v %v", reply, fields)
	}
}

func TestDesktopConfigurationStillSendsFullSnapshots(t *testing.T) {
	rig := newSignalRig(t, Config{})
	greeting, ok := rig.next(t, "config.changed", 5*time.Second)
	if !ok {
		t.Fatal("no config.changed on connect")
	}
	var snapshot Configuration.Snapshot
	if err := json.Unmarshal(greeting.Payload, &snapshot); err != nil || snapshot.Sections["scheduler"] == nil {
		t.Fatalf("desktop greeting = %s, %v", greeting.Payload, err)
	}
	if fields := signalOf(t, greeting); fields["external"] != nil {
		t.Fatal("a desktop worker marked its snapshot as an external signal")
	}
	if _, err := rig.configuration.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}
	changed, ok := rig.next(t, "config.changed", 5*time.Second)
	if !ok || !strings.Contains(string(changed.Payload), `"minAttackDelay":9`) {
		t.Fatalf("desktop change = %s %v", changed.Payload, ok)
	}
}

func TestUpdateStatusIsPushedOnConnectAndOnEveryChange(t *testing.T) {
	release := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"2.1.0","downloadUrl":"https://downloads.example.test/releases/citadel-ops-2.1.0","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer release.Close()
	manager := AppUpdate.NewManager(AppUpdate.Config{
		CurrentVersion: "2.0.0", Endpoint: release.URL, DownloadBaseURL: "https://downloads.example.test/releases", Client: release.Client(),
	})
	rig := newSignalRig(t, Config{Updates: manager})
	first, ok := rig.next(t, "update.changed", 5*time.Second)
	if !ok || !strings.Contains(string(first.Payload), `"currentVersion":"2.0.0"`) || !strings.Contains(string(first.Payload), `"status":"idle"`) {
		t.Fatalf("connect status = %s %v", first.Payload, ok)
	}
	if err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	for {
		next, ok := rig.next(t, "update.changed", 5*time.Second)
		if !ok {
			t.Fatal("the availability change was not pushed")
		}
		if strings.Contains(string(next.Payload), `"status":"available"`) && strings.Contains(string(next.Payload), `"latestVersion":"2.1.0"`) {
			break
		}
	}
	// The REST route stays for older portals and for the first read.
	recorder := httptest.NewRecorder()
	NewServer(Config{Updates: manager}).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/update", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/v2/update = %d", recorder.Code)
	}
}

func TestNoUpdateManagerMeansNoUpdateEvent(t *testing.T) {
	rig := newSignalRig(t, Config{})
	if event, got := rig.next(t, "update.changed", 500*time.Millisecond); got {
		t.Fatalf("unexpected update.changed: %s", event.Payload)
	}
}
