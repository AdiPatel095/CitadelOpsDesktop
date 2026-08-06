package Session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"github.com/gorilla/websocket"
)

const (
	directGameOrigin          = "https://empire-html5.goodgamestudios.com"
	directCacheBreakerURL     = "https://empire-html5.goodgamestudios.com/default/assets/CacheBreaker.js"
	directDefaultPingInterval = 60 * time.Second
	directMovementInterval    = 5 * time.Second
	directHandshakeTimeout    = 45 * time.Second
	directWriteTimeout        = 10 * time.Second
	directMaxMessageBytes     = 64 << 20
	directEmptyArgument       = "<RoundHouseKick>"
)

var transpilationVersionPattern = regexp.MustCompile(
	`name\s*:\s*["']TranspilationEmpire["']\s*,\s*version\s*:\s*["']([0-9]+)\.([0-9]+)\.([0-9]+)["']`,
)

type DirectWebSocketConfig struct {
	DataDir    string
	HTTPClient *http.Client

	dialer            *websocket.Dialer
	serverURLOverride string
	pingInterval      time.Duration
	movementInterval  time.Duration
	handshakeTimeout  time.Duration
	buildResolver     func(context.Context, string) (string, error)
}

type DirectWebSocketTransport struct {
	config     DirectWebSocketConfig
	credential persistedLoginCredential
	profile    gameConnectionProfile
	resolveErr error

	frames   chan RawFrame
	statuses chan Status

	mu                 sync.RWMutex
	status             Status
	cancel             context.CancelFunc
	runGeneration      uint64
	connection         *websocket.Conn
	selectedBrowser    BrowserCandidate
	relogDelayProvider func() time.Duration

	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   []directPendingResponse
}

type directPendingResponse struct {
	token     string
	opcodes   map[string]struct{}
	expiresAt time.Time
}

type directReadResult struct {
	payload string
	err     error
}

type directWireEvent struct {
	raw    string
	action string
	roomID int
}

type directWireDecoder struct {
	buffer string
}

type directLoginError struct {
	code        int
	cooldownSec int
	detail      string
	fatal       bool
}

func (err *directLoginError) Error() string { return err.detail }

func NewDirectWebSocketTransport(config DirectWebSocketConfig) *DirectWebSocketTransport {
	credential, credentialErr := loadLoginCredential(config.DataDir)
	profile, profileErr := loadGameConnectionProfile(config.DataDir)
	var resolveErr error
	switch {
	case credentialErr != nil:
		resolveErr = fmt.Errorf("Background mode needs a saved game login; start Full application mode and sign in once")
	case !credential.AutoRestore:
		resolveErr = fmt.Errorf("Background mode cannot use a saved login that has been disabled; open Settings > Game Connection to re-enable it")
	case profileErr != nil:
		resolveErr = fmt.Errorf("Background mode needs current connection details; start Full application mode and sign in once")
	}
	preference := loadBrowserPreference(config.DataDir)
	selectedBrowser, _ := ResolveChromiumBrowser(preference, "")
	state := "stopped"
	detail := ""
	if resolveErr != nil {
		state = "unavailable"
		detail = resolveErr.Error()
	}
	return &DirectWebSocketTransport{
		config: config, credential: credential, profile: profile, resolveErr: resolveErr,
		frames: make(chan RawFrame, 8192), statuses: make(chan Status, 32),
		status: Status{
			Mode: ConnectionModeBackground, State: state, Namespace: profile.Namespace,
			ServerURL: profile.ServerURL, Detail: detail, ChangedAt: time.Now().UTC(),
		},
		selectedBrowser: selectedBrowser,
	}
}

func (transport *DirectWebSocketTransport) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	transport.mu.Lock()
	if transport.resolveErr != nil {
		err := transport.resolveErr
		transport.mu.Unlock()
		return err
	}
	if transport.cancel != nil {
		transport.mu.Unlock()
		return nil
	}
	runContext, cancel := context.WithCancel(ctx)
	transport.runGeneration++
	generation := transport.runGeneration
	transport.cancel = cancel
	transport.mu.Unlock()
	if !transport.publishRunStatus(generation, Status{
		Mode: ConnectionModeBackground, State: "starting", Namespace: transport.profile.Namespace,
		ServerURL: transport.profile.ServerURL, Detail: "Starting the background game connection",
		ChangedAt: time.Now().UTC(),
	}) {
		return nil
	}
	go transport.run(runContext, generation)
	return nil
}

func (transport *DirectWebSocketTransport) Stop(context.Context) error {
	transport.mu.Lock()
	cancel := transport.cancel
	connection := transport.connection
	transport.cancel = nil
	transport.connection = nil
	transport.runGeneration++
	transport.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if connection != nil {
		_ = connection.Close()
	}
	transport.clearPending()
	transport.publishStatus(Status{
		Mode: ConnectionModeBackground, State: "stopped", Namespace: transport.profile.Namespace,
		ServerURL: transport.profile.ServerURL, ChangedAt: time.Now().UTC(),
	})
	return nil
}

func (transport *DirectWebSocketTransport) PrepareBackgroundMode() error {
	credential, profile, err := prepareBackgroundLogin(transport.config.DataDir)
	if err != nil {
		return err
	}
	transport.mu.Lock()
	transport.credential = credential
	transport.profile = profile
	transport.resolveErr = nil
	if transport.cancel == nil {
		status := transport.status
		status.State = "stopped"
		status.LoggedIn = false
		status.SocketReady = false
		status.Detail = ""
		status.RetryAt = nil
		status.Namespace = profile.Namespace
		status.ServerURL = profile.ServerURL
		status.ChangedAt = time.Now().UTC()
		transport.status = status
	}
	transport.mu.Unlock()
	return nil
}

func (transport *DirectWebSocketTransport) run(ctx context.Context, generation uint64) {
	for {
		err := transport.connectAndServe(ctx, generation)
		if ctx.Err() != nil || !transport.isCurrent(generation) {
			return
		}
		var loginErr *directLoginError
		if errors.As(err, &loginErr) && loginErr.fatal {
			transport.publishRunStatus(generation, Status{
				Mode: ConnectionModeBackground, State: "error", Namespace: transport.profile.Namespace,
				ServerURL: transport.profile.ServerURL, Detail: loginErr.Error(), ChangedAt: time.Now().UTC(),
			})
			return
		}
		delay := transport.relogDelay()
		now := time.Now().UTC()
		status := Status{
			Mode: ConnectionModeBackground, State: "reconnecting", Namespace: transport.profile.Namespace,
			ServerURL: transport.profile.ServerURL, LoggedIn: false, SocketReady: false,
			Detail: fmt.Sprintf("Background game connection closed: %v", err), ChangedAt: now,
		}
		if errors.As(err, &loginErr) && loginErr.code == 453 {
			cooldownUntil := now.Add(time.Duration(max(0, loginErr.cooldownSec)) * time.Second)
			retryAt := cooldownUntil.Add(delay)
			status.State = "cooldown"
			status.Detail = loginErr.Error()
			status.CooldownUntil = &cooldownUntil
			status.RetryAt = &retryAt
			delay = time.Until(retryAt)
		} else {
			retryAt := now.Add(delay)
			status.RetryAt = &retryAt
		}
		if !transport.publishRunStatus(generation, status) {
			return
		}
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (transport *DirectWebSocketTransport) connectAndServe(ctx context.Context, generation uint64) error {
	if ctx.Err() != nil || !transport.isCurrent(generation) {
		return context.Canceled
	}
	serverURL := transport.profile.ServerURL
	if override := strings.TrimSpace(transport.config.serverURLOverride); override != "" {
		serverURL = override
	}
	build, err := transport.resolveBuild(ctx)
	if err != nil {
		return err
	}
	if ctx.Err() != nil || !transport.isCurrent(generation) {
		return context.Canceled
	}
	if !transport.publishRunStatus(generation, Status{
		Mode: ConnectionModeBackground, State: "connecting", Namespace: transport.profile.Namespace,
		ServerURL: transport.profile.ServerURL, Detail: "Opening the background game WebSocket",
		ChangedAt: time.Now().UTC(),
	}) {
		return context.Canceled
	}
	dialer := transport.config.dialer
	if dialer == nil {
		copy := *websocket.DefaultDialer
		dialer = &copy
	}
	headers := http.Header{"Origin": []string{directGameOrigin}}
	connection, response, err := dialer.DialContext(ctx, serverURL, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("open background game WebSocket: %w", err)
	}
	connection.SetReadLimit(directMaxMessageBytes)
	transport.mu.Lock()
	if transport.runGeneration != generation || transport.cancel == nil {
		transport.mu.Unlock()
		_ = connection.Close()
		return context.Canceled
	}
	transport.connection = connection
	transport.mu.Unlock()
	defer func() {
		_ = connection.Close()
		transport.mu.Lock()
		if transport.runGeneration == generation && transport.connection == connection {
			transport.connection = nil
		}
		transport.mu.Unlock()
		transport.clearPending()
	}()

	connectedAt := time.Now()
	reads := make(chan directReadResult, 32)
	go readDirectWebSocket(ctx, connection, reads)
	decoder := &directWireDecoder{}
	if !transport.publishRunStatus(generation, Status{
		Mode: ConnectionModeBackground, State: "authenticating", Namespace: transport.profile.Namespace,
		ServerURL: transport.profile.ServerURL, Detail: "Game WebSocket is open; authenticating the saved login",
		ChangedAt: time.Now().UTC(),
	}) {
		return context.Canceled
	}
	roomID, roundTrip, buffered, err := transport.authenticate(
		ctx, connection, reads, decoder, build, connectedAt,
	)
	if err != nil {
		return err
	}
	transport.mu.Lock()
	if transport.runGeneration != generation || transport.cancel == nil || transport.connection != connection {
		transport.mu.Unlock()
		return context.Canceled
	}
	connectionGeneration := transport.status.ConnectionGeneration + 1
	transport.mu.Unlock()
	if !transport.publishRunStatus(generation, Status{
		Mode: ConnectionModeBackground, State: "connected", Namespace: transport.profile.Namespace,
		ServerURL: transport.profile.ServerURL, LoggedIn: true, SocketReady: true,
		ConnectionGeneration: connectionGeneration,
		Detail:               "Connected directly in background mode", ChangedAt: time.Now().UTC(),
	}) {
		return context.Canceled
	}
	for _, raw := range buffered {
		transport.deliverInbound(raw, connectionGeneration)
	}
	return transport.serveConnected(
		ctx, connection, reads, decoder, roomID, roundTrip, connectionGeneration,
	)
}

func (transport *DirectWebSocketTransport) authenticate(
	ctx context.Context,
	connection *websocket.Conn,
	reads <-chan directReadResult,
	decoder *directWireDecoder,
	build string,
	connectedAt time.Time,
) (int, time.Duration, []string, error) {
	timeout := transport.config.handshakeTimeout
	if timeout <= 0 {
		timeout = directHandshakeTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	if err := transport.writeHandshake(connection, `<msg t='sys'><body action='verChk' r='0'><ver v='166' /></body></msg>`); err != nil {
		return 0, 0, nil, err
	}
	roomID := 0
	autoJoinSent := false
	loginSent := false
	roundTripStarted := time.Time{}
	roundTrip := time.Duration(0)
	sessionID := newDirectSessionID()
	for {
		select {
		case <-ctx.Done():
			return 0, 0, nil, ctx.Err()
		case <-deadline.C:
			return 0, 0, nil, fmt.Errorf("background game authentication timed out")
		case result := <-reads:
			if result.err != nil {
				return 0, 0, nil, result.err
			}
			events := decoder.append(result.payload)
			for index, event := range events {
				switch event.action {
				case "apiOK":
					language, languageErr := gameConnectionContextString(transport.profile.LoginContext, "LANG")
					if languageErr != nil || language == "" {
						return 0, 0, nil, fmt.Errorf("background login language is unavailable")
					}
					distributor, distributorErr := gameConnectionContextString(transport.profile.LoginContext, "DID")
					if distributorErr != nil || distributor == "" {
						return 0, 0, nil, fmt.Errorf("background login distributor is unavailable")
					}
					password := build + "%" + language + "%" + distributor
					message := fmt.Sprintf(
						"<msg t='sys'><body action='login' r='0'><login z='%s'><nick><![CDATA[]]></nick><pword><![CDATA[%s]]></pword></login></body></msg>",
						transport.profile.Namespace, strings.ReplaceAll(password, "]]>", "]] ]><![CDATA[>"),
					)
					if err := transport.writeHandshake(connection, message); err != nil {
						return 0, 0, nil, err
					}
				case "joinOK":
					roomID = event.roomID
					if roomID <= 0 {
						return 0, 0, nil, fmt.Errorf("game server returned an invalid lobby room")
					}
					roundTripStarted = time.Now()
					if err := transport.writeHandshake(connection, fmt.Sprintf(
						"<msg t='sys'><body action='roundTrip' r='%d'></body></msg>", roomID,
					)); err != nil {
						return 0, 0, nil, err
					}
					versionFrame := fmt.Sprintf(
						"%%xt%%%s%%vck%%%d%%%s%%web-html5%%%s%%%s%%",
						transport.profile.Namespace, roomID, build, directEmptyArgument, sessionID,
					)
					if err := transport.writeHandshake(connection, versionFrame); err != nil {
						return 0, 0, nil, err
					}
				case "roundTripRes":
					if !roundTripStarted.IsZero() {
						roundTrip = time.Since(roundTripStarted)
					}
				}

				if event.raw == "" {
					continue
				}
				frame, decodeErr := Protocol.Decode(event.raw, Protocol.DirectionInbound, time.Now().UTC())
				if decodeErr != nil {
					continue
				}
				switch frame.Opcode {
				case "rlu":
					if !autoJoinSent {
						autoJoinSent = true
						if err := transport.writeHandshake(connection, `<msg t='sys'><body action='autoJoin' r='-1'></body></msg>`); err != nil {
							return 0, 0, nil, err
						}
					}
				case "vck":
					if frame.ResponseCode == nil || *frame.ResponseCode != 0 {
						code := -1
						if frame.ResponseCode != nil {
							code = *frame.ResponseCode
						}
						return 0, 0, nil, &directLoginError{
							code: code, fatal: true,
							detail: fmt.Sprintf("Background game client version was rejected with code %d; use Full application mode once to refresh it", code),
						}
					}
					loginFrame, loginErr := transport.loginFrame(roomID, time.Since(connectedAt), roundTrip)
					if loginErr != nil {
						return 0, 0, nil, loginErr
					}
					if err := transport.writeHandshake(connection, loginFrame); err != nil {
						return 0, 0, nil, err
					}
					loginSent = true
				case "lli":
					if !loginSent || frame.ResponseCode == nil {
						continue
					}
					if *frame.ResponseCode != 0 {
						cooldown := 0
						if *frame.ResponseCode == 453 {
							var payload struct {
								Seconds int `json:"CD"`
							}
							_ = json.Unmarshal(frame.Payload, &payload)
							cooldown = max(0, payload.Seconds)
						}
						return 0, 0, nil, &directLoginError{
							code: *frame.ResponseCode, cooldownSec: cooldown,
							fatal:  *frame.ResponseCode != 453,
							detail: fmt.Sprintf("Background game login failed with code %d", *frame.ResponseCode),
						}
					}
					buffered := []string{event.raw}
					for _, remainder := range events[index+1:] {
						if remainder.raw != "" {
							buffered = append(buffered, remainder.raw)
						}
					}
					return roomID, roundTrip, buffered, nil
				}
			}
		}
	}
}

func (transport *DirectWebSocketTransport) loginFrame(
	roomID int,
	connectionTime time.Duration,
	roundTrip time.Duration,
) (string, error) {
	payload := make(map[string]json.RawMessage, len(transport.profile.LoginContext)+6)
	for key, value := range transport.profile.LoginContext {
		payload[key] = append(json.RawMessage(nil), value...)
	}
	username, err := json.Marshal(transport.credential.Username)
	if err != nil {
		return "", err
	}
	password, err := json.Marshal(transport.credential.Password)
	if err != nil {
		return "", err
	}
	payload["NOM"] = username
	payload["PW"] = password
	payload["LT"] = json.RawMessage(`null`)
	payload["ID"] = json.RawMessage(`0`)
	payload["CONM"] = json.RawMessage(strconv.FormatInt(max(0, connectionTime.Milliseconds()), 10))
	payload["RTM"] = json.RawMessage(strconv.FormatInt(max(0, roundTrip.Milliseconds()), 10))
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode background game login: %w", err)
	}
	return fmt.Sprintf("%%xt%%%s%%lli%%%d%%%s%%", transport.profile.Namespace, roomID, encoded), nil
}

func (transport *DirectWebSocketTransport) serveConnected(
	ctx context.Context,
	connection *websocket.Conn,
	reads <-chan directReadResult,
	decoder *directWireDecoder,
	roomID int,
	_ time.Duration,
	connectionGeneration uint64,
) error {
	pingInterval := transport.config.pingInterval
	if pingInterval <= 0 {
		pingInterval = directDefaultPingInterval
	}
	movementInterval := transport.config.movementInterval
	if movementInterval <= 0 {
		movementInterval = directMovementInterval
	}
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()
	movementTicker := time.NewTicker(movementInterval)
	defer movementTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-reads:
			if result.err != nil {
				return result.err
			}
			for _, event := range decoder.append(result.payload) {
				if event.raw != "" {
					transport.deliverInbound(event.raw, connectionGeneration)
				}
			}
		case <-pingTicker.C:
			frame := fmt.Sprintf(
				"%%xt%%%s%%pin%%%d%%%s%%", transport.profile.Namespace, roomID, directEmptyArgument,
			)
			if _, err := transport.sendInternal(
				connection, frame, connectionGeneration, "session:background:heartbeat", "",
			); err != nil {
				return err
			}
		case <-movementTicker.C:
			frame := fmt.Sprintf("%%xt%%%s%%gam%%%d%%{}%%", transport.profile.Namespace, roomID)
			if _, err := transport.sendInternal(
				connection, frame, connectionGeneration, "session:background:movement-refresh", "gam",
			); err != nil {
				return err
			}
		}
	}
}

func (transport *DirectWebSocketTransport) Send(ctx context.Context, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	metadata := Outbound.MetadataFromContext(ctx)
	transport.mu.RLock()
	connection := transport.connection
	status := transport.status
	transport.mu.RUnlock()
	if metadata.ConnectionGeneration > 0 && metadata.ConnectionGeneration != status.ConnectionGeneration {
		return Outbound.ErrConnectionChanged
	}
	if connection == nil || !status.LoggedIn || !status.SocketReady {
		return fmt.Errorf("background game websocket is not ready")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	transport.writeMu.Lock()
	pending := transport.registerPending(metadata)
	err := transport.writeApplicationFrameLocked(ctx, connection, payload)
	transport.writeMu.Unlock()
	if err != nil {
		transport.removePending(pending)
		_ = connection.Close()
		return Outbound.MarkIndeterminate(err)
	}
	transport.frames <- RawFrame{
		Payload: string(payload), Direction: Protocol.DirectionOutbound, ObservedAt: time.Now().UTC(),
		ConnectionGeneration: status.ConnectionGeneration, CausationOperationID: metadata.OperationID,
	}
	return nil
}

func (transport *DirectWebSocketTransport) sendInternal(
	connection *websocket.Conn,
	payload string,
	connectionGeneration uint64,
	causation string,
	skipForPendingOpcode string,
) (bool, error) {
	transport.writeMu.Lock()
	if skipForPendingOpcode != "" && transport.hasPendingOpcode(skipForPendingOpcode) {
		transport.writeMu.Unlock()
		return false, nil
	}
	err := transport.writeApplicationFrameLocked(context.Background(), connection, []byte(payload))
	transport.writeMu.Unlock()
	if err != nil {
		_ = connection.Close()
		return false, err
	}
	transport.frames <- RawFrame{
		Payload: payload, Direction: Protocol.DirectionOutbound, ObservedAt: time.Now().UTC(),
		ConnectionGeneration: connectionGeneration, CausationOperationID: causation,
	}
	return true, nil
}

func (transport *DirectWebSocketTransport) writeApplicationFrameLocked(
	ctx context.Context,
	connection *websocket.Conn,
	payload []byte,
) error {
	deadline := time.Now().Add(directWriteTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	_ = connection.SetWriteDeadline(deadline)
	return connection.WriteMessage(websocket.TextMessage, payload)
}

func (transport *DirectWebSocketTransport) writeHandshake(connection *websocket.Conn, payload string) error {
	transport.writeMu.Lock()
	defer transport.writeMu.Unlock()
	_ = connection.SetWriteDeadline(time.Now().Add(directWriteTimeout))
	if err := connection.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		return fmt.Errorf("send background game handshake: %w", err)
	}
	return nil
}

func (transport *DirectWebSocketTransport) deliverInbound(payload string, generation uint64) {
	frame, err := Protocol.Decode(payload, Protocol.DirectionInbound, time.Now().UTC())
	if err != nil {
		return
	}
	transport.frames <- RawFrame{
		Payload: payload, Direction: Protocol.DirectionInbound, ObservedAt: frame.ReceivedAt,
		ConnectionGeneration: generation, ResponseToken: transport.matchResponseToken(frame.Opcode),
	}
}

func (transport *DirectWebSocketTransport) registerPending(metadata Outbound.Metadata) *directPendingResponse {
	if strings.TrimSpace(metadata.ResponseToken) == "" || len(metadata.ResponseOpcodes) == 0 {
		return nil
	}
	expiresAt := time.Now().Add(time.Duration(metadata.ResponseTimeoutMillis) * time.Millisecond)
	if metadata.ResponseTimeoutMillis <= 0 {
		expiresAt = time.Now().Add(30 * time.Second)
	}
	pending := &directPendingResponse{
		token: metadata.ResponseToken, opcodes: make(map[string]struct{}, len(metadata.ResponseOpcodes)),
		expiresAt: expiresAt,
	}
	for _, opcode := range metadata.ResponseOpcodes {
		if opcode = strings.ToLower(strings.TrimSpace(opcode)); opcode != "" {
			pending.opcodes[opcode] = struct{}{}
		}
	}
	if len(pending.opcodes) == 0 {
		return nil
	}
	transport.pendingMu.Lock()
	transport.purgePendingLocked(time.Now())
	transport.pending = append(transport.pending, *pending)
	transport.pendingMu.Unlock()
	return pending
}

func (transport *DirectWebSocketTransport) removePending(target *directPendingResponse) {
	if target == nil {
		return
	}
	transport.pendingMu.Lock()
	defer transport.pendingMu.Unlock()
	for index := range transport.pending {
		if transport.pending[index].token == target.token {
			transport.pending = append(transport.pending[:index], transport.pending[index+1:]...)
			return
		}
	}
}

func (transport *DirectWebSocketTransport) matchResponseToken(opcode string) string {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	transport.pendingMu.Lock()
	defer transport.pendingMu.Unlock()
	transport.purgePendingLocked(time.Now())
	for index, pending := range transport.pending {
		if _, expected := pending.opcodes[opcode]; !expected {
			continue
		}
		transport.pending = append(transport.pending[:index], transport.pending[index+1:]...)
		return pending.token
	}
	return ""
}

func (transport *DirectWebSocketTransport) hasPendingOpcode(opcode string) bool {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	transport.pendingMu.Lock()
	defer transport.pendingMu.Unlock()
	transport.purgePendingLocked(time.Now())
	for _, pending := range transport.pending {
		if _, expected := pending.opcodes[opcode]; expected {
			return true
		}
	}
	return false
}

func (transport *DirectWebSocketTransport) purgePendingLocked(now time.Time) {
	kept := transport.pending[:0]
	for _, pending := range transport.pending {
		if pending.expiresAt.After(now) {
			kept = append(kept, pending)
		}
	}
	transport.pending = kept
}

func (transport *DirectWebSocketTransport) clearPending() {
	transport.pendingMu.Lock()
	transport.pending = nil
	transport.pendingMu.Unlock()
}

func (transport *DirectWebSocketTransport) resolveBuild(ctx context.Context) (string, error) {
	fallback := transport.profile.ClientBuild
	if transport.config.buildResolver != nil {
		return transport.config.buildResolver(ctx, fallback)
	}
	client := transport.config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, directCacheBreakerURL, nil)
	if err != nil {
		return fallback, nil
	}
	response, err := client.Do(request)
	if err != nil {
		return fallback, nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fallback, nil
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return fallback, nil
	}
	match := transpilationVersionPattern.FindSubmatch(contents)
	if len(match) != 4 {
		return fallback, nil
	}
	major, majorErr := strconv.ParseUint(string(match[1]), 10, 32)
	minor, minorErr := strconv.ParseUint(string(match[2]), 10, 32)
	patch, patchErr := strconv.ParseUint(string(match[3]), 10, 32)
	if majorErr != nil || minorErr != nil || patchErr != nil || minor > 999 || patch > 999 {
		return fallback, nil
	}
	build := strconv.FormatUint(major*1_000_000+minor*1_000+patch, 10)
	if !gameBuildPattern.MatchString(build) {
		return fallback, nil
	}
	return build, nil
}

func readDirectWebSocket(ctx context.Context, connection *websocket.Conn, output chan<- directReadResult) {
	for {
		messageType, payload, err := connection.ReadMessage()
		if err != nil {
			select {
			case output <- directReadResult{err: fmt.Errorf("read background game WebSocket: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		select {
		case output <- directReadResult{payload: string(payload)}:
		case <-ctx.Done():
			return
		}
	}
}

func splitDirectWireEvents(payload string) []directWireEvent {
	decoder := &directWireDecoder{}
	return decoder.append(payload)
}

func (decoder *directWireDecoder) append(payload string) []directWireEvent {
	decoder.buffer += payload
	var events []directWireEvent
	for decoder.buffer != "" {
		xmlIndex := strings.Index(decoder.buffer, "<msg")
		xtIndex := strings.Index(decoder.buffer, "%xt%")
		switch {
		case xmlIndex >= 0 && (xtIndex < 0 || xmlIndex < xtIndex):
			decoder.buffer = decoder.buffer[xmlIndex:]
			end := strings.Index(decoder.buffer, "</msg>")
			if end < 0 {
				return events
			}
			message := decoder.buffer[:end+len("</msg>")]
			events = append(events, parseDirectSystemEvent(message))
			decoder.buffer = decoder.buffer[end+len("</msg>"):]
		case xtIndex >= 0:
			decoder.buffer = decoder.buffer[xtIndex:]
			nextXT := strings.Index(decoder.buffer[len("%xt%"):], "%xt%")
			nextXML := strings.Index(decoder.buffer[len("%xt%"):], "<msg")
			end := len(decoder.buffer)
			if nextXT >= 0 {
				end = len("%xt%") + nextXT
			}
			if nextXML >= 0 && len("%xt%")+nextXML < end {
				end = len("%xt%") + nextXML
			}
			frame := strings.TrimSpace(decoder.buffer[:end])
			if end == len(decoder.buffer) && !strings.HasSuffix(frame, "%") {
				return events
			}
			if strings.HasSuffix(frame, "%") {
				events = append(events, directWireEvent{raw: frame})
			}
			decoder.buffer = decoder.buffer[end:]
		default:
			if len(decoder.buffer) > directMaxMessageBytes {
				decoder.buffer = ""
			}
			return events
		}
	}
	return events
}

func parseDirectSystemEvent(message string) directWireEvent {
	action := attributeValue(message, "action")
	roomID := 0
	if action == "joinOK" {
		roomID, _ = strconv.Atoi(attributeValue(message, "r"))
	}
	return directWireEvent{action: action, roomID: roomID}
}

func attributeValue(message string, name string) string {
	for _, quote := range []string{"'", `"`} {
		needle := name + "=" + quote
		start := strings.Index(message, needle)
		if start < 0 {
			continue
		}
		start += len(needle)
		end := strings.Index(message[start:], quote)
		if end >= 0 {
			return message[start : start+end]
		}
	}
	return ""
}

func newDirectSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		value := new(big.Int).SetBytes(bytes)
		if value.Sign() > 0 {
			return value.String()
		}
	}
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func (transport *DirectWebSocketTransport) Status() Status {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.status
}

func (transport *DirectWebSocketTransport) StatusChanges() <-chan Status { return transport.statuses }
func (transport *DirectWebSocketTransport) Frames() <-chan RawFrame      { return transport.frames }
func (*DirectWebSocketTransport) CorrelatesResponses() bool              { return true }
func (*DirectWebSocketTransport) ReportsOutboundCausation() bool         { return true }
func (*DirectWebSocketTransport) CloseGameUI(context.Context) error      { return nil }

func (transport *DirectWebSocketTransport) publishStatus(status Status) {
	status = transport.prepareStatus(status)
	transport.mu.Lock()
	if status.ConnectionGeneration == 0 {
		status.ConnectionGeneration = transport.status.ConnectionGeneration
	}
	transport.status = status
	transport.mu.Unlock()
	transport.enqueueStatus(status)
}

func (transport *DirectWebSocketTransport) publishRunStatus(generation uint64, status Status) bool {
	status = transport.prepareStatus(status)
	transport.mu.Lock()
	if transport.runGeneration != generation || transport.cancel == nil {
		transport.mu.Unlock()
		return false
	}
	if status.ConnectionGeneration == 0 {
		status.ConnectionGeneration = transport.status.ConnectionGeneration
	}
	transport.status = status
	transport.mu.Unlock()
	transport.enqueueStatus(status)
	return true
}

func (transport *DirectWebSocketTransport) prepareStatus(status Status) Status {
	if status.ChangedAt.IsZero() {
		status.ChangedAt = time.Now().UTC()
	}
	status.Mode = ConnectionModeBackground
	if status.Namespace == "" {
		status.Namespace = transport.profile.Namespace
	}
	if status.ServerURL == "" {
		status.ServerURL = transport.profile.ServerURL
	}
	return status
}

func (transport *DirectWebSocketTransport) enqueueStatus(status Status) {
	select {
	case transport.statuses <- status:
	default:
		select {
		case <-transport.statuses:
		default:
		}
		select {
		case transport.statuses <- status:
		default:
		}
	}
}

func (transport *DirectWebSocketTransport) isCurrent(generation uint64) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.runGeneration == generation && transport.cancel != nil
}

func (transport *DirectWebSocketTransport) SetRelogDelayProvider(provider func() time.Duration) {
	transport.mu.Lock()
	transport.relogDelayProvider = provider
	transport.mu.Unlock()
}

func (transport *DirectWebSocketTransport) relogDelay() time.Duration {
	transport.mu.RLock()
	provider := transport.relogDelayProvider
	transport.mu.RUnlock()
	if provider == nil {
		return defaultRelogDelay
	}
	delay := provider()
	if delay < 0 {
		return defaultRelogDelay
	}
	return delay
}

func (transport *DirectWebSocketTransport) SelectBrowser(preference string) error {
	candidate, err := ResolveChromiumBrowser(preference, "")
	if err != nil {
		return err
	}
	if err := saveBrowserPreference(transport.config.DataDir, candidate); err != nil {
		return err
	}
	transport.mu.Lock()
	transport.selectedBrowser = candidate
	transport.mu.Unlock()
	return nil
}

func (transport *DirectWebSocketTransport) BrowserInventory() BrowserInventory {
	transport.mu.RLock()
	selected := transport.selectedBrowser
	transport.mu.RUnlock()
	inventory := browserInventory(BrowserCandidate{}, selected, DiscoverChromiumBrowsers())
	// Background mode has no running browser to replace. This selection will be
	// used if the user later restarts in Full application mode.
	inventory.RestartRequired = false
	return inventory
}
