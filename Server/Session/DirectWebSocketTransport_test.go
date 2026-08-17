package Session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
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

// fakeGameServer answers the Background handshake and replies to each login
// frame with loginReply(attempt), so tests can observe how the transport
// classifies a failed login and when it tries again, without a real game
// server. It returns the server and a counter of login attempts seen.
func fakeGameServer(t *testing.T, loginReply string) *httptest.Server {
	t.Helper()
	server, _ := fakeGameServerWithReplies(t, func(int) string { return loginReply })
	return server
}

func fakeGameServerWithReplies(t *testing.T, loginReply func(attempt int) string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var attempts atomic.Int32
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
				reply = loginReply(int(attempts.Add(1)))
				if reply == "CLOSE" {
					// Simulate a server-side drop right after the login frame.
					_ = connection.Close()
					return
				}
				if reply == "OKTHENDROP" {
					// Log the session in, then kill the socket the way the
					// live game does on displacement: abruptly, no close frame.
					if writeErr := connection.WriteMessage(websocket.TextMessage, []byte(`%xt%lli%1%0%{}%`)); writeErr != nil {
						return
					}
					time.Sleep(50 * time.Millisecond)
					_ = connection.Close()
					return
				}
				if reply == "KICK" {
					// Simulate the game displacing this session because the
					// player logged in: a deliberate close frame, then close.
					_ = connection.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, "kicked"),
						time.Now().Add(time.Second),
					)
					_ = connection.Close()
					return
				}
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
	t.Cleanup(server.Close)
	return server, &attempts
}

func startFakeGameTransport(t *testing.T, server *httptest.Server) *DirectWebSocketTransport {
	t.Helper()
	transport := newFakeGameTransport(t, server, t.TempDir())
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return transport
}

func newFakeGameTransport(t *testing.T, server *httptest.Server, dataDir string) *DirectWebSocketTransport {
	t.Helper()
	return newFakeGameTransportWithConfig(t, server, dataDir, nil)
}

func newFakeGameTransportWithConfig(t *testing.T, server *httptest.Server, dataDir string, adjust func(*DirectWebSocketConfig)) *DirectWebSocketTransport {
	t.Helper()
	if err := saveBackgroundLoginCredential(dataDir, persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion, CapturedAt: time.Now().UTC(), AutoRestore: true,
		Username: "test-player", Password: "test-password",
		ServerURL: "wss://ep-live-us1-game.goodgamestudios.com:443", Language: "en",
	}); err != nil {
		t.Fatal(err)
	}
	config := DirectWebSocketConfig{
		DataDir:           dataDir,
		serverURLOverride: "ws" + strings.TrimPrefix(server.URL, "http"),
		pingInterval:      25 * time.Millisecond,
		movementInterval:  15 * time.Millisecond,
		handshakeTimeout:  2 * time.Second,
		buildResolver:     func(context.Context, string) (string, error) { return "1165009", nil },
	}
	if adjust != nil {
		adjust(&config)
	}
	transport := NewDirectWebSocketTransport(config)
	transport.SetRelogDelayProvider(func() time.Duration { return 30 * time.Millisecond })
	t.Cleanup(func() { _ = transport.Stop(context.Background()) })
	return transport
}

func awaitTransportState(t *testing.T, transport *DirectWebSocketTransport, states ...string) Status {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("transport never reported %v: %+v", states, transport.Status())
		case status := <-transport.StatusChanges():
			for _, state := range states {
				if status.State == state {
					return status
				}
			}
		}
	}
}

func awaitLoginFailureStatus(t *testing.T, transport *DirectWebSocketTransport, states ...string) Status {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("transport never reported %v: %+v", states, transport.Status())
		case status := <-transport.StatusChanges():
			for _, state := range states {
				if status.State == state {
					return status
				}
			}
			if status.State == "connected" {
				t.Fatalf("transport connected despite the failed login: %+v", status)
			}
		}
	}
}

func TestDirectTransportClassifiesFailedLogins(t *testing.T) {
	t.Run("temporary lockout is a non-fatal cooldown", func(t *testing.T) {
		transport := startFakeGameTransport(t, fakeGameServer(t, `%xt%lli%1%453%{"CD":7}%`))
		status := awaitLoginFailureStatus(t, transport, "cooldown")
		if status.LoginFailure == nil || status.LoginFailure.Code != 453 ||
			status.LoginFailure.Class != State.LoginFailureCooldown || status.LoginFailure.Fatal ||
			status.CooldownUntil == nil || status.RetryAt == nil || status.LoggedIn {
			t.Fatalf("cooldown status = %+v failure = %+v", status, status.LoginFailure)
		}
		if strings.Contains(status.Detail, "test-password") {
			t.Fatal("status detail exposed the password")
		}
	})
	t.Run("IS_BANNED with a remaining time suspends and resumes automatically", func(t *testing.T) {
		before := time.Now().UTC()
		transport := startFakeGameTransport(t, fakeGameServer(t, `%xt%lli%1%27%{"RS":86400}%`))
		status := awaitLoginFailureStatus(t, transport, "suspended")
		failure := status.LoginFailure
		if failure == nil || failure.Code != 27 || failure.Class != State.LoginFailureSuspended || !failure.Fatal ||
			failure.SuspendedUntil == nil || failure.SuspendedUntil.Before(before.Add(23*time.Hour)) ||
			failure.SuspendedUntil.After(before.Add(25*time.Hour)) ||
			status.RetryAt == nil || status.RetryAt.Before(*failure.SuspendedUntil) {
			t.Fatalf("suspension status = %+v failure = %+v", status, failure)
		}
		if !strings.Contains(status.Detail, "IS_BANNED") {
			t.Fatalf("detail did not name the official code: %q", status.Detail)
		}
		if !transport.Running() {
			t.Fatal("a temporary suspension parked the transport instead of scheduling the resume")
		}
	})
	t.Run("a short suspension resumes and connects without intervention", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(attempt int) string {
			if attempt == 1 {
				return `%xt%lli%1%27%{"RS":1}%`
			}
			return `%xt%lli%1%0%{}%`
		})
		transport := startFakeGameTransport(t, server)
		awaitTransportState(t, transport, "suspended")
		status := awaitTransportState(t, transport, "connected")
		if !status.LoggedIn || attempts.Load() < 2 {
			t.Fatalf("transport did not resume after the suspension: %+v attempts=%d", status, attempts.Load())
		}
	})
	t.Run("permanent suspension parks until the saved login changes", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(int) string { return `%xt%lli%1%27%{}%` })
		transport := startFakeGameTransport(t, server)
		status := awaitLoginFailureStatus(t, transport, "error")
		if status.RetryAt != nil || status.LoginFailure == nil || status.LoginFailure.SuspendedUntil != nil {
			t.Fatalf("permanent suspension status = %+v", status)
		}
		waitUntilNotRunning(t, transport)
		if err := transport.Start(context.Background()); !errors.Is(err, ErrLoginParked) {
			t.Fatalf("Start on a parked login = %v", err)
		}
		time.Sleep(60 * time.Millisecond)
		if attempts.Load() != 1 {
			t.Fatalf("parked transport spent the login again: %d attempts", attempts.Load())
		}
	})
	t.Run("IS_BANNED with the GDPR flag is a deactivated account", func(t *testing.T) {
		transport := startFakeGameTransport(t, fakeGameServer(t, `%xt%lli%1%27%{"GDPR":true}%`))
		status := awaitLoginFailureStatus(t, transport, "error")
		if status.LoginFailure == nil || status.LoginFailure.Class != State.LoginFailureAccountDeleted ||
			!status.LoginFailure.Fatal || status.LoginFailure.SuspendedUntil != nil {
			t.Fatalf("deleted-account failure = %+v", status.LoginFailure)
		}
	})
	t.Run("INVALID_PASSWORD parks and a changed login restarts", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(attempt int) string {
			if attempt == 1 {
				return `%xt%lli%1%20%{}%`
			}
			return `%xt%lli%1%0%{}%`
		})
		dataDir := t.TempDir()
		transport := newFakeGameTransport(t, server, dataDir)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		status := awaitLoginFailureStatus(t, transport, "error")
		if status.LoginFailure == nil || status.LoginFailure.Class != State.LoginFailureInvalidCredentials ||
			!status.LoginFailure.Fatal || status.RetryAt != nil {
			t.Fatalf("invalid password status = %+v failure = %+v", status, status.LoginFailure)
		}
		waitUntilNotRunning(t, transport)
		if err := transport.Start(context.Background()); !errors.Is(err, ErrLoginParked) {
			t.Fatalf("Start with the same rejected login = %v", err)
		}
		if err := saveBackgroundLoginCredential(dataDir, persistedLoginCredential{
			SchemaVersion: loginCredentialSchemaVersion, CapturedAt: time.Now().UTC().Add(time.Second), AutoRestore: true,
			Username: "test-player", Password: "corrected-password",
			ServerURL: "wss://ep-live-us1-game.goodgamestudios.com:443", Language: "en",
		}); err != nil {
			t.Fatal(err)
		}
		if err := transport.Start(context.Background()); err != nil {
			t.Fatalf("Start with a changed login = %v", err)
		}
		if status := awaitTransportState(t, transport, "connected"); !status.LoggedIn || attempts.Load() != 2 {
			t.Fatalf("changed login did not connect: %+v attempts=%d", status, attempts.Load())
		}
	})
	t.Run("PLAYER_NOT_FOUND is the wrong-server class", func(t *testing.T) {
		transport := startFakeGameTransport(t, fakeGameServer(t, `%xt%lli%1%21%{}%`))
		if failure := awaitLoginFailureStatus(t, transport, "error").LoginFailure; failure == nil ||
			failure.Class != State.LoginFailureWrongServer || !failure.Fatal {
			t.Fatalf("player not found failure = %+v", failure)
		}
	})
	t.Run("unknown code keeps retrying with a doubling relog delay", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(int) string { return `%xt%lli%1%777%{}%` })
		transport := startFakeGameTransport(t, server)
		status := awaitLoginFailureStatus(t, transport, "error")
		if status.LoginFailure == nil || status.LoginFailure.Code != 777 ||
			status.LoginFailure.Class != State.LoginFailureUnknown || !status.LoginFailure.Fatal ||
			status.LoginFailure.ObservedAt.IsZero() || status.RetryAt == nil {
			t.Fatalf("fatal status = %+v failure = %+v", status, status.LoginFailure)
		}
		deadline := time.Now().Add(3 * time.Second)
		for attempts.Load() < 3 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if attempts.Load() < 3 || !transport.Running() {
			t.Fatalf("unknown code stopped retrying: attempts=%d running=%t", attempts.Load(), transport.Running())
		}
	})
}

func waitUntilNotRunning(t *testing.T, transport *DirectWebSocketTransport) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for transport.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if transport.Running() {
		t.Fatal("transport did not park after a fatal login failure")
	}
}

func TestClassifyLoginFailure(t *testing.T) {
	tests := []struct {
		code  int
		class State.LoginFailureClass
		fatal bool
		name  string
	}{
		{453, State.LoginFailureCooldown, false, "LOGIN_COOLDOWN_ACTIVE"},
		{27, State.LoginFailureSuspended, true, "IS_BANNED"},
		{20, State.LoginFailureInvalidCredentials, true, "INVALID_PASSWORD"},
		{409, State.LoginFailureInvalidCredentials, true, "INVALID_LOGIN_TOKEN"},
		{423, State.LoginFailureInvalidCredentials, true, "INVALID_GLOBALSERVER_LOGIN_TOKEN"},
		{21, State.LoginFailureWrongServer, true, "PLAYER_NOT_FOUND"},
		{26, State.LoginFailureWrongServer, true, "NO_AVATAR_CREATED"},
		{368, State.LoginFailureWrongServer, true, "EXISTING_MAPPING_WRONG_SERVER"},
		{369, State.LoginFailureUnknown, true, "UNEXPECTED_FACEBOOK_ERROR"},
		{999, State.LoginFailureUnknown, true, ""},
	}
	for _, test := range tests {
		class, fatal := State.ClassifyLoginFailure(test.code, false)
		if class != test.class || fatal != test.fatal || State.LoginFailureCodeName(test.code) != test.name {
			t.Fatalf("code %d = %s fatal %t name %q, want %s %t %q", test.code, class, fatal, State.LoginFailureCodeName(test.code), test.class, test.fatal, test.name)
		}
	}
	if class, fatal := State.ClassifyLoginFailure(-1, true); class != State.LoginFailureClientVersionRejected || fatal {
		t.Fatalf("client version = %s fatal %t", class, fatal)
	}
}

func TestDirectTransportReleasePolicyHandsTheWaitToTheControlPlane(t *testing.T) {
	t.Run("cooldown releases the session and refuses an unforced early start", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(attempt int) string {
			if attempt == 1 {
				return `%xt%lli%1%453%{"CD":3600}%`
			}
			return `%xt%lli%1%0%{}%`
		})
		transport := newFakeGameTransport(t, server, t.TempDir())
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := time.Now().UTC()
		status := awaitTransportState(t, transport, "released")
		if status.CooldownUntil == nil || status.RetryAt == nil || status.RetryAt.Before(before.Add(59*time.Minute)) ||
			status.LoginFailure == nil || status.LoginFailure.Class != State.LoginFailureCooldown ||
			!strings.HasPrefix(status.Detail, "Session released") {
			t.Fatalf("released status = %+v failure = %+v", status, status.LoginFailure)
		}
		waitUntilNotRunning(t, transport)
		if err := transport.Start(context.Background()); !errors.Is(err, ErrReconnectScheduled) {
			t.Fatalf("unforced Start during a released cooldown = %v", err)
		}
		if attempts.Load() != 1 {
			t.Fatalf("released transport spent the login again: %d attempts", attempts.Load())
		}
		// The user or control plane forces a reconnect: the hold is bypassed once.
		transport.ClearReconnectHold()
		if err := transport.Start(context.Background()); err != nil {
			t.Fatalf("forced Start = %v", err)
		}
		if status := awaitTransportState(t, transport, "connected"); !status.LoggedIn || attempts.Load() != 2 {
			t.Fatalf("forced reconnect did not connect: %+v attempts=%d", status, attempts.Load())
		}
	})
	t.Run("temporary suspension releases with the suspension end as retry time", func(t *testing.T) {
		server, _ := fakeGameServerWithReplies(t, func(int) string { return `%xt%lli%1%27%{"RS":7200}%` })
		transport := newFakeGameTransport(t, server, t.TempDir())
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := time.Now().UTC()
		status := awaitTransportState(t, transport, "released")
		if status.RetryAt == nil || status.RetryAt.Before(before.Add(119*time.Minute)) ||
			status.LoginFailure == nil || status.LoginFailure.Class != State.LoginFailureSuspended {
			t.Fatalf("released suspension status = %+v", status)
		}
		waitUntilNotRunning(t, transport)
	})
	t.Run("invalid password releases without a retry time", func(t *testing.T) {
		server, _ := fakeGameServerWithReplies(t, func(int) string { return `%xt%lli%1%20%{}%` })
		transport := newFakeGameTransport(t, server, t.TempDir())
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		status := awaitTransportState(t, transport, "released")
		if status.RetryAt != nil || status.LoginFailure == nil || status.LoginFailure.Class != State.LoginFailureInvalidCredentials {
			t.Fatalf("released invalid-password status = %+v", status)
		}
		waitUntilNotRunning(t, transport)
		if err := transport.Start(context.Background()); !errors.Is(err, ErrLoginParked) {
			t.Fatalf("Start with the same rejected login = %v", err)
		}
	})
	t.Run("plain drops get a short immediate retry window before release", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(int) string { return "CLOSE" })
		transport := newFakeGameTransportWithConfig(t, server, t.TempDir(), func(config *DirectWebSocketConfig) {
			config.releaseRetryDelay = 10 * time.Millisecond
			config.releaseRetryAttempts = 2
		})
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		status := awaitTransportState(t, transport, "released")
		if status.RetryAt == nil || status.LoginFailure != nil {
			t.Fatalf("released drop status = %+v", status)
		}
		waitUntilNotRunning(t, transport)
		if got := attempts.Load(); got != 3 {
			t.Fatalf("login attempts before release = %d, want 1 + 2 immediate retries", got)
		}
	})
	t.Run("a deliberate server close skips the immediate retry window", func(t *testing.T) {
		// The player logging in displaces the runtime's session with a close
		// frame. Reconnecting immediately would kick the player right back
		// out, so the release must happen at once with the full relog wait.
		server, attempts := fakeGameServerWithReplies(t, func(int) string { return "KICK" })
		transport := newFakeGameTransportWithConfig(t, server, t.TempDir(), func(config *DirectWebSocketConfig) {
			config.releaseRetryDelay = 10 * time.Millisecond
			config.releaseRetryAttempts = 2
		})
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		status := awaitTransportState(t, transport, "released")
		if status.RetryAt == nil || status.LoginFailure != nil {
			t.Fatalf("displaced status = %+v", status)
		}
		if !strings.Contains(status.Detail, "taken over") {
			t.Fatalf("displaced detail = %q", status.Detail)
		}
		waitUntilNotRunning(t, transport)
		if got := attempts.Load(); got != 1 {
			t.Fatalf("login attempts after displacement = %d, want exactly 1 (no immediate retries)", got)
		}
	})
	t.Run("a session that dies young releases with the relog wait", func(t *testing.T) {
		// The live game kicks the runtime with an abrupt drop (no close
		// frame) when the player logs in. A session that connected and then
		// died inside the stability window must not burn immediate retries —
		// that is the relog ping-pong that kicks the player straight back out.
		server, attempts := fakeGameServerWithReplies(t, func(int) string { return "OKTHENDROP" })
		transport := newFakeGameTransportWithConfig(t, server, t.TempDir(), func(config *DirectWebSocketConfig) {
			config.releaseRetryDelay = 10 * time.Millisecond
			config.releaseRetryAttempts = 2
		})
		transport.SetReconnectPolicy(ReconnectPolicyRelease)
		if err := transport.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		status := awaitTransportState(t, transport, "released")
		if status.RetryAt == nil || status.LoginFailure != nil {
			t.Fatalf("young-session release status = %+v", status)
		}
		if !strings.Contains(status.Detail, "relog delay") {
			t.Fatalf("young-session release detail = %q", status.Detail)
		}
		waitUntilNotRunning(t, transport)
		if got := attempts.Load(); got != 1 {
			t.Fatalf("login attempts after young-session drop = %d, want exactly 1", got)
		}
	})
	t.Run("hold policy keeps reconnecting after drops", func(t *testing.T) {
		server, attempts := fakeGameServerWithReplies(t, func(attempt int) string {
			if attempt <= 2 {
				return "CLOSE"
			}
			return `%xt%lli%1%0%{}%`
		})
		transport := startFakeGameTransport(t, server)
		if status := awaitTransportState(t, transport, "connected"); !status.LoggedIn || attempts.Load() < 3 {
			t.Fatalf("hold policy did not reconnect through drops: %+v attempts=%d", status, attempts.Load())
		}
	})
}
