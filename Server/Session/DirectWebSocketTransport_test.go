package Session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"github.com/gorilla/websocket"
)

func TestDirectWebSocketTransportAuthenticatesAndKeepsGameSessionAlive(t *testing.T) {
	if directDefaultPingInterval != 60*time.Second {
		t.Fatalf("background ping interval = %s, want 60s", directDefaultPingInterval)
	}
	received := make(chan string, 128)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		for {
			_, payload, readErr := connection.ReadMessage()
			if readErr != nil {
				return
			}
			message := string(payload)
			received <- message
			var reply string
			switch {
			case strings.Contains(message, "action='verChk'"):
				reply = `<msg t='sys'><body action='apiOK' r='0'></body></msg>`
			case strings.Contains(message, "action='login'"):
				reply = `%xt%rlu%-1%0%{}%`
			case strings.Contains(message, "action='autoJoin'"):
				reply = `<msg t='sys'><body action='joinOK' r='1'><pid id='0'/></body></msg>`
			case strings.Contains(message, "action='roundTrip'"):
				reply = `<msg t='sys'><body action='roundTripRes' r='1'></body></msg>`
			case strings.Contains(message, "%vck%"):
				reply = `%xt%vck%1%0%{}%`
			case strings.Contains(message, "%lli%"):
				reply = `%xt%lli%1%0%{}%`
			case strings.Contains(message, "%abc%"):
				reply = `%xt%abc%1%0%{"ok":true}%`
			}
			if reply != "" {
				if writeErr := connection.WriteMessage(websocket.TextMessage, []byte(reply)); writeErr != nil {
					return
				}
			}
		}
	}))
	defer server.Close()

	dataDir := t.TempDir()
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		CapturedAt:    time.Now().UTC(), AutoRestore: true,
		Username: "test-player", Password: "test-password",
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveGameConnectionProfile(dataDir, gameConnectionProfile{
		SchemaVersion: gameConnectionProfileSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		ServerURL:     "wss://ep-live-us1-game.goodgamestudios.com:443",
		Namespace:     "EmpireEx_21", ClientBuild: "1165009", Platform: "web-html5",
		LoginContext: map[string]json.RawMessage{
			"LANG": json.RawMessage(`"en"`),
			"DID":  json.RawMessage(`0`),
			"AID":  json.RawMessage(`""`),
			"PL":   json.RawMessage(`1`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	transport := NewDirectWebSocketTransport(DirectWebSocketConfig{
		DataDir:           dataDir,
		serverURLOverride: "ws" + strings.TrimPrefix(server.URL, "http"),
		pingInterval:      25 * time.Millisecond,
		movementInterval:  15 * time.Millisecond,
		handshakeTimeout:  2 * time.Second,
		buildResolver:     func(context.Context, string) (string, error) { return "1165009", nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer transport.Stop(context.Background())

	for {
		select {
		case <-ctx.Done():
			t.Fatal("background transport did not authenticate")
		case status := <-transport.StatusChanges():
			if status.State == "connected" {
				if status.Mode != ConnectionModeBackground || !status.LoggedIn || !status.SocketReady || status.ConnectionGeneration == 0 {
					t.Fatalf("unexpected connected status: %+v", status)
				}
				goto connected
			}
			if status.State == "error" {
				t.Fatalf("background transport authentication failed: %+v", status)
			}
		}
	}

connected:
	status := transport.Status()
	metadata := Outbound.Metadata{
		OperationID: "test-operation", ConnectionGeneration: status.ConnectionGeneration,
		ResponseToken: "test-response", ResponseOpcodes: []string{"abc"}, ResponseTimeoutMillis: 1000,
	}
	if err := transport.Send(
		Outbound.WithMetadata(ctx, metadata),
		[]byte(`%xt%EmpireEx_21%abc%1%{}%`),
	); err != nil {
		t.Fatal(err)
	}

	wantPing := `%xt%EmpireEx_21%pin%1%<RoundHouseKick>%`
	wantMovement := `%xt%EmpireEx_21%gam%1%{}%`
	seenPing := false
	seenMovement := false
	seenFreshSessionID := false
	seenCorrelatedResponse := false
	for !seenPing || !seenMovement || !seenCorrelatedResponse {
		select {
		case <-ctx.Done():
			t.Fatalf(
				"missing background traffic: ping=%v movement=%v response=%v",
				seenPing, seenMovement, seenCorrelatedResponse,
			)
		case message := <-received:
			seenPing = seenPing || message == wantPing
			seenMovement = seenMovement || message == wantMovement
			if strings.Contains(message, "%vck%") {
				parts := strings.Split(message, "%")
				seenFreshSessionID = len(parts) >= 10 && parts[7] == directEmptyArgument && parts[8] != directEmptyArgument && parts[8] != ""
			}
		case frame := <-transport.Frames():
			if strings.Contains(frame.Payload, "%abc%") && frame.Direction == "inbound" && frame.ResponseToken == "test-response" {
				seenCorrelatedResponse = true
			}
		}
	}
	if !seenFreshSessionID {
		t.Fatal("version check did not include a fresh non-persisted session id")
	}
}

func TestDirectWebSocketTransportRequiresProtectedFullModeBootstrap(t *testing.T) {
	transport := NewDirectWebSocketTransport(DirectWebSocketConfig{DataDir: t.TempDir()})
	if transport.Status().Mode != ConnectionModeBackground || transport.Status().State != "unavailable" {
		t.Fatalf("unexpected unavailable background status: %+v", transport.Status())
	}
	if err := transport.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "Full application mode") {
		t.Fatalf("unexpected missing bootstrap error: %v", err)
	}
}

func TestGameConnectionProfileRejectsCredentialExfiltrationTargets(t *testing.T) {
	profile := gameConnectionProfile{
		Namespace: "EmpireEx_21", ClientBuild: "1165009", Platform: "web-html5",
		ServerURL: "wss://attacker.example/ep-live-us1-game.goodgamestudios.com",
		LoginContext: map[string]json.RawMessage{
			"LANG": json.RawMessage(`"en"`), "DID": json.RawMessage(`0`),
		},
	}
	if err := validateGameConnectionProfile(profile); err == nil {
		t.Fatal("unofficial game server was accepted")
	}
	profile.ServerURL = "wss://ep-live-us1-game.goodgamestudios.com:443"
	profile.LoginContext["LT"] = json.RawMessage(`"rotating-token"`)
	if err := validateGameConnectionProfile(profile); err == nil {
		t.Fatal("rotating login token was accepted in the connection profile")
	}
}

func TestSanitizedGameLoginContextDropsSecretsAndCaptchaTokens(t *testing.T) {
	context := sanitizedGameLoginContext(json.RawMessage(`{
		"LANG":"en","DID":0,"AID":"account","NOM":"player","PW":"password",
		"LT":"login-token","RCT":"captcha-token","FTK":"social-token"
	}`))
	for _, key := range []string{"NOM", "PW", "LT", "RCT", "FTK"} {
		if _, exists := context[key]; exists {
			t.Fatalf("sensitive login key %s was retained", key)
		}
	}
	if _, ok := context["LANG"]; !ok {
		t.Fatal("safe language context was dropped")
	}
	if _, ok := context["DID"]; !ok {
		t.Fatal("safe distributor context was dropped")
	}
}

func TestDirectWireDecoderBuffersFragmentedSmartFoxMessages(t *testing.T) {
	decoder := &directWireDecoder{}
	if events := decoder.append(`<msg t='sys'><body action='api`); len(events) != 0 {
		t.Fatalf("partial XML produced events: %+v", events)
	}
	events := decoder.append(`OK' r='0'></body></msg>%xt%lli%1%0%{"ok":`)
	if len(events) != 1 || events[0].action != "apiOK" {
		t.Fatalf("completed XML events = %+v", events)
	}
	events = decoder.append(`true}%%xt%gbd%1%0%{}%`)
	if len(events) != 2 || !strings.Contains(events[0].raw, `%lli%`) || !strings.Contains(events[1].raw, `%gbd%`) {
		t.Fatalf("fragmented XT events = %+v", events)
	}
}
