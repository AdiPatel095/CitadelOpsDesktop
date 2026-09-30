package API

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func jsonBody(size int) []byte {
	items := make([]string, 0, size/20+1)
	for index := 0; len(strings.Join(items, ","))+2 < size; index++ {
		items = append(items, `{"id":`+strings.Repeat("7", 1+index%5)+`,"n":"castle"}`)
	}
	return []byte("[" + strings.Join(items, ",") + "]")
}

func serve(t *testing.T, handler http.Handler, method string, headers map[string]string) *http.Response {
	t.Helper()
	server := httptest.NewServer(compressResponses(handler))
	t.Cleanup(server.Close)
	request, err := http.NewRequest(method, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	// Disable the transport's transparent gzip so the raw wire encoding is visible.
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func jsonHandler(body []byte, mutate func(http.ResponseWriter)) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		if mutate != nil {
			mutate(writer)
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	})
}

func TestJSONResponsesOfOneKiBOrMoreAreGzipped(t *testing.T) {
	body := jsonBody(64 << 10)
	response := serve(t, jsonHandler(body, func(w http.ResponseWriter) { w.Header().Set("Content-Length", "65536") }), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if response.Header.Get("Content-Encoding") != "gzip" || response.Header.Get("Content-Length") != "" {
		t.Fatalf("headers = %v, want gzip and no stale Content-Length", response.Header)
	}
	if !strings.Contains(response.Header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q", response.Header.Get("Vary"))
	}
	compressed, _ := io.ReadAll(response.Body)
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := io.ReadAll(reader)
	if !bytes.Equal(decoded, body) {
		t.Fatal("the gzip body does not decode to the original")
	}
	if len(compressed)*4 > len(body) {
		t.Fatalf("compressed %d of %d bytes: JSON state should shrink much more than that", len(compressed), len(body))
	}
}

func TestResponsesUnderTheThresholdStayPlainButVary(t *testing.T) {
	body := jsonBody(700)
	response := serve(t, jsonHandler(body, nil), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	got, _ := io.ReadAll(response.Body)
	if response.Header.Get("Content-Encoding") != "" || !bytes.Equal(got, body) {
		t.Fatalf("a %d-byte response was encoded %q", len(body), response.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(response.Header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q", response.Header.Get("Vary"))
	}
	// Exactly at the threshold compresses.
	edge := serve(t, jsonHandler(bytes.Repeat([]byte("a"), compressionThreshold), nil), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if edge.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("a %d-byte response was not compressed", compressionThreshold)
	}
	// A declared short Content-Length passes through without buffering.
	declared := serve(t, jsonHandler(body, func(w http.ResponseWriter) { w.Header().Set("Content-Length", "700") }), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if declared.Header.Get("Content-Encoding") != "" || declared.Header.Get("Content-Length") != "700" {
		t.Fatalf("declared short response headers = %v", declared.Header)
	}
}

func TestResponsesAreNotCompressedWhenTheClientDoesNotAskOrTheResponseCannotBe(t *testing.T) {
	body := jsonBody(8 << 10)
	for name, test := range map[string]struct {
		method  string
		headers map[string]string
		handler http.Handler
	}{
		"no Accept-Encoding": {http.MethodGet, nil, jsonHandler(body, nil)},
		"identity only":      {http.MethodGet, map[string]string{"Accept-Encoding": "identity"}, jsonHandler(body, nil)},
		"gzip refused":       {http.MethodGet, map[string]string{"Accept-Encoding": "gzip;q=0, br"}, jsonHandler(body, nil)},
		"range request":      {http.MethodGet, map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-99"}, jsonHandler(body, nil)},
		"already encoded":    {http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}, jsonHandler(body, func(w http.ResponseWriter) { w.Header().Set("Content-Encoding", "br") })},
		"no-transform":       {http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}, jsonHandler(body, func(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-cache, no-transform") })},
		"binary content": {http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(body)
		})},
		"no content type": {http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header()["Content-Type"] = nil
			_, _ = w.Write(body)
		})},
	} {
		response := serve(t, test.handler, test.method, test.headers)
		if encoding := response.Header.Get("Content-Encoding"); encoding == "gzip" {
			t.Errorf("%s: response was gzipped", name)
		}
		if got, _ := io.ReadAll(response.Body); name != "range request" && !bytes.Equal(got, body) {
			t.Errorf("%s: body altered", name)
		}
	}
	head := serve(t, jsonHandler(body, nil), http.MethodHead, map[string]string{"Accept-Encoding": "gzip"})
	if head.Header.Get("Content-Encoding") != "" {
		t.Error("a HEAD response was encoded")
	}
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		response := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
		}), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
		if response.StatusCode != status || response.Header.Get("Content-Encoding") != "" {
			t.Errorf("status %d: got %d encoding %q", status, response.StatusCode, response.Header.Get("Content-Encoding"))
		}
	}
	errorResponse := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadRequest, "bad", strings.Repeat("x", 2000), nil)
	}), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if errorResponse.StatusCode != http.StatusBadRequest || errorResponse.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("a large error body: status %d encoding %q, want 400 gzip", errorResponse.StatusCode, errorResponse.Header.Get("Content-Encoding"))
	}
}

func TestStrongETagBecomesWeakWhenGzipped(t *testing.T) {
	response := serve(t, jsonHandler(jsonBody(4<<10), func(w http.ResponseWriter) { w.Header().Set("ETag", `"abc"`) }), http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if response.Header.Get("Etag") != `W/"abc"` {
		t.Fatalf("ETag = %q", response.Header.Get("Etag"))
	}
}

// Server-sent events keep flushing each event as it happens: the client reads the
// first event while the handler is still running and has not written the second.
func TestServerSentEventsAreDeliveredIncrementallyAndNeverCompressed(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		flusher := writer.(http.Flusher)
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache, no-transform")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("event: first\ndata: 1\n\n"))
		flusher.Flush()
		<-release
		_, _ = writer.Write([]byte("event: second\ndata: 2\n\n"))
		flusher.Flush()
	})
	response := serve(t, handler, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("an event stream was encoded %q", response.Header.Get("Content-Encoding"))
	}
	reader := bufio.NewReader(response.Body)
	lines := make(chan string, 8)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- line
		}
	}()
	select {
	case line := <-lines:
		if line != "event: first\n" {
			t.Fatalf("first line = %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the first event was buffered until the response ended")
	}
	closeRelease()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("stream ended without the second event")
			}
			if line == "event: second\n" {
				return
			}
		case <-deadline:
			t.Fatal("second event never arrived")
		}
	}
}

// Even an event stream mislabelled as JSON keeps flowing: a Flush commits the
// response to streaming gzip and each flushed chunk reaches the client.
func TestFlushedCompressibleStreamsAreDeliveredIncrementally(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = writer.Write([]byte(`{"a":1}` + "\n"))
		writer.(http.Flusher).Flush()
		<-release
		_, _ = writer.Write([]byte(`{"a":2}` + "\n"))
	})
	response := serve(t, handler, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("flushed compressible stream encoding = %q", response.Header.Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	line := make(chan string, 1)
	go func() {
		text, _ := bufio.NewReader(reader).ReadString('\n')
		line <- text
	}()
	select {
	case text := <-line:
		if text != `{"a":1}`+"\n" {
			t.Fatalf("first chunk = %q", text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a flushed chunk was held back")
	}
	closeRelease()
}

func TestWebSocketUpgradesPassThroughTheCompressionMiddleware(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(compressResponses(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.WriteMessage(websocket.TextMessage, jsonBody(4<<10))
	})))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Accept-Encoding": {"gzip"}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, message, err := connection.ReadMessage()
	if err != nil || len(message) < 4<<10 {
		t.Fatalf("websocket message %d bytes, %v", len(message), err)
	}
}

// capturingConn records every byte the server sends so the test can inspect the
// websocket frame headers (RSV1 marks a compressed message).
type capturingConn struct {
	net.Conn
	mu       sync.Mutex
	received bytes.Buffer
}

func (conn *capturingConn) Read(buffer []byte) (int, error) {
	count, err := conn.Conn.Read(buffer)
	conn.mu.Lock()
	conn.received.Write(buffer[:count])
	conn.mu.Unlock()
	return count, err
}

// messageFrames returns, per websocket message, whether its first frame had RSV1 set.
func (conn *capturingConn) messageFrames(t *testing.T) []bool {
	t.Helper()
	conn.mu.Lock()
	data := append([]byte(nil), conn.received.Bytes()...)
	conn.mu.Unlock()
	end := bytes.Index(data, []byte("\r\n\r\n"))
	if end < 0 {
		t.Fatal("no HTTP upgrade response captured")
	}
	data = data[end+4:]
	var compressed []bool
	startOfMessage := true
	for len(data) >= 2 {
		first, second := data[0], data[1]
		length, header := int(second&0x7f), 2
		switch length {
		case 126:
			if len(data) < 4 {
				return compressed
			}
			length, header = int(data[2])<<8|int(data[3]), 4
		case 127:
			if len(data) < 10 {
				return compressed
			}
			length, header = 0, 10
			for _, b := range data[2:10] {
				length = length<<8 | int(b)
			}
		}
		if len(data) < header+length {
			return compressed
		}
		if startOfMessage && first&0x0f != 0x0 { // data frame, not continuation
			compressed = append(compressed, first&0x40 != 0)
		} else if startOfMessage && first&0x0f == 0x0 {
			compressed = append(compressed, first&0x40 != 0)
		}
		startOfMessage = first&0x80 != 0
		data = data[header+length:]
	}
	return compressed
}

func newSocketServer(t *testing.T, hosted bool) (*State.Store, string) {
	t.Helper()
	store := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(nil, store, nil, nil, nil)
	server := httptest.NewServer(NewServer(Config{
		State: store, Intents: engine, GameData: GameData.NewManager(GameData.UpdaterConfig{}), BackgroundOnly: hosted,
	}).Handler())
	t.Cleanup(server.Close)
	return store, "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v2/events"
}

func dialCapturing(t *testing.T, url string, offerCompression bool) (*websocket.Conn, *capturingConn, *http.Response) {
	t.Helper()
	capture := &capturingConn{}
	dialer := websocket.Dialer{
		EnableCompression: offerCompression,
		NetDial: func(network string, address string) (net.Conn, error) {
			connection, err := net.Dial(network, address)
			capture.Conn = connection
			return capture, err
		},
	}
	connection, response, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return connection, capture, response
}

func TestHostedSocketCompressesOnlyMessagesOfOneKiBOrMore(t *testing.T) {
	store, url := newSocketServer(t, true)
	connection, capture, response := dialCapturing(t, url, true)
	if !strings.Contains(response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate") {
		t.Fatalf("permessage-deflate not negotiated: %q", response.Header.Get("Sec-WebSocket-Extensions"))
	}
	// Make one large and one small state event: many domains, then one tiny mutation.
	if _, err := store.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		for index := 0; index < 400; index++ {
			gameState.Player.Name = strings.Repeat("commander-", 1) + string(rune('a'+index%26))
		}
		gameState.Player.Level = 42
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	var sizes []int
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	for len(sizes) < 3 {
		_, message, err := connection.ReadMessage()
		if err != nil {
			break
		}
		sizes = append(sizes, len(message))
	}
	if len(sizes) < 2 {
		t.Fatalf("read only %d messages", len(sizes))
	}
	flags := capture.messageFrames(t)
	if len(flags) < len(sizes) {
		t.Fatalf("captured %d frames for %d messages", len(flags), len(sizes))
	}
	sawLarge, sawSmall := false, false
	for index, size := range sizes {
		want := size >= compressionThreshold
		sawLarge, sawSmall = sawLarge || want, sawSmall || !want
		if flags[index] != want {
			t.Errorf("message %d (%d bytes): compressed=%t, want %t", index, size, flags[index], want)
		}
	}
	if !sawLarge || !sawSmall {
		t.Fatalf("the test needs both a message of at least %d bytes and a smaller one; sizes %v", compressionThreshold, sizes)
	}
}

func TestDesktopSocketStaysUncompressed(t *testing.T) {
	_, url := newSocketServer(t, false)
	connection, capture, response := dialCapturing(t, url, true)
	if extensions := response.Header.Get("Sec-WebSocket-Extensions"); extensions != "" {
		t.Fatalf("the desktop server negotiated %q", extensions)
	}
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := connection.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	for _, compressed := range capture.messageFrames(t) {
		if compressed {
			t.Fatal("desktop mode sent a compressed frame")
		}
	}
}

func TestHostedAPIResponsesAreCompressedAndDesktopOnesAreNot(t *testing.T) {
	for _, hosted := range []bool{true, false} {
		store := State.NewStore(State.NewGameState())
		engine := Intent.NewEngine(nil, store, nil, nil, nil)
		server := httptest.NewServer(NewServer(Config{State: store, Intents: engine, GameData: GameData.NewManager(GameData.UpdaterConfig{}), BackgroundOnly: hosted}).Handler())
		request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v2/state", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		response, err := (&http.Client{Transport: &http.Transport{DisableCompression: true}}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		server.Close()
		if hosted {
			reader, err := gzip.NewReader(bytes.NewReader(body))
			if response.Header.Get("Content-Encoding") != "gzip" || err != nil {
				t.Fatalf("hosted /state: encoding %q, err %v", response.Header.Get("Content-Encoding"), err)
			}
			decoded, _ := io.ReadAll(reader)
			var state map[string]any
			if json.Unmarshal(decoded, &state) != nil || len(decoded) < len(body) {
				t.Fatal("hosted /state did not decode to JSON")
			}
		} else if response.Header.Get("Content-Encoding") != "" {
			t.Fatalf("desktop /state was encoded %q", response.Header.Get("Content-Encoding"))
		}
	}
}

// benchmarkClientState is a synthetic busy account: thousands of equipment items and
// 52 movements, so the JSON is in the hundreds of KB like production /state.
func benchmarkClientState(tb testing.TB) []byte {
	tb.Helper()
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	for index := 0; index < 4000; index++ {
		id := State.EquipmentInstanceID(100000 + index)
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, DefinitionID: State.EquipmentID(2000 + index%300), Slot: 1 + index%5, TypeID: 2, RarityID: 1 + index%6,
			Level: index % 60, WearerID: int64(index % 17), Effects: State.EquipmentEffects{
				{DefinitionID: 9001, Values: []float64{float64(10 + index%90)}},
				{DefinitionID: 9002, Values: []float64{float64(index % 45), 2, 3}},
			},
		}
	}
	for index := 0; index < 52; index++ {
		id := State.MovementID(5000 + index)
		arrives := time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC)
		gameState.SetMovement(id, State.MovementState{
			ID: id, Direction: 0, OwnerPlayerID: 1, TargetPlayerID: 99, TravelSeconds: 900, ObservedAt: arrives, StartedAt: arrives,
			ArrivesAt: &arrives, Units: map[State.UnitID]int64{6: 40, 7: 10}, TargetX: 20 + index, TargetY: 21,
		})
	}
	raw, err := json.Marshal(State.NewClientStateSnapshot(gameState))
	if err != nil {
		tb.Fatal(err)
	}
	return raw
}

func BenchmarkGzipClientState(b *testing.B) {
	raw := benchmarkClientState(b)
	var compressedSize int
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		var out bytes.Buffer
		writer := gzipWriters.Get().(*gzip.Writer)
		writer.Reset(&out)
		_, _ = writer.Write(raw)
		_ = writer.Close()
		gzipWriters.Put(writer)
		compressedSize = out.Len()
	}
	b.ReportMetric(float64(len(raw)), "raw-bytes")
	b.ReportMetric(float64(compressedSize), "gzip-bytes")
	b.ReportMetric(float64(len(raw))/float64(compressedSize), "ratio")
}
