package API

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestStreamEnvelopeRawKeepsZeroBaseRevisionDistinctFromAbsent(t *testing.T) {
	raw, err := json.Marshal(streamEnvelopeRaw("", "state.changed", 1, 1, true, 0, json.RawMessage(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"baseRevision":0`) {
		t.Fatalf("state.changed with base revision 0 = %s, want an explicit baseRevision", raw)
	}
	other, err := json.Marshal(newEnvelope("", "config.changed", 1, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(other), "baseRevision") {
		t.Fatalf("non-state envelope = %s, must not declare baseRevision", other)
	}
}

func TestEventsSocketDeclaresBaseRevisionAndAnswersQueryState(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(nil, store, nil, nil, nil)
	server := httptest.NewServer(NewServer(Config{
		State: store, Intents: engine, GameData: GameData.NewManager(GameData.UpdaterConfig{}),
	}).Handler())
	defer server.Close()

	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v2/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()

	read := func() Envelope {
		t.Helper()
		_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
		var envelope Envelope
		if err := socket.ReadJSON(&envelope); err != nil {
			t.Fatal(err)
		}
		return envelope
	}
	// The snapshot is the first frame; later handshake frames (operations, ...) are skipped.
	if first := read(); first.Type != "state.snapshot" || first.Revision != 0 {
		t.Fatalf("first frame = %s revision %d, want state.snapshot at 0", first.Type, first.Revision)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.Apply(func(*State.GameState) ([]string, bool, error) {
			return []string{"units"}, true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for want := uint64(1); want <= 2; {
		envelope := read()
		if envelope.Type != "state.changed" {
			continue
		}
		if envelope.Revision != want || envelope.Gap || envelope.BaseRevision == nil || *envelope.BaseRevision != want-1 {
			t.Fatalf("state.changed = revision %d gap %t base %v, want revision %d base %d",
				envelope.Revision, envelope.Gap, envelope.BaseRevision, want, want-1)
		}
		want++
	}

	if err := socket.WriteJSON(Envelope{Version: ContractVersion, ID: "resync-1", Type: "query.state"}); err != nil {
		t.Fatal(err)
	}
	for {
		envelope := read()
		if envelope.Type != "state.snapshot" {
			continue
		}
		if envelope.ID != "resync-1" || envelope.Revision != 2 {
			t.Fatalf("query.state reply = id %q revision %d, want resync-1 at 2", envelope.ID, envelope.Revision)
		}
		if envelope.BaseRevision != nil {
			t.Fatalf("snapshot must not declare a base revision, got %d", *envelope.BaseRevision)
		}
		return
	}
}
