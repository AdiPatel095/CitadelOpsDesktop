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
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"github.com/gorilla/websocket"
)

const (
	directGameOrigin          = "https://empire-html5.goodgamestudios.com"
	directGameIndexURL        = "https://empire-html5.goodgamestudios.com/default/index.html"
	directDefaultPingInterval = 60 * time.Second
	directMovementInterval    = 5 * time.Second
	// directSubscriptionRefreshInterval re-pulls subscription packages so a
	// mid-session lapse or purchase is noticed without a relog.
	directSubscriptionRefreshInterval = 6 * time.Hour
	directHandshakeTimeout            = 45 * time.Second
	directWriteTimeout                = 10 * time.Second
	directMaxMessageBytes             = 64 << 20
	directMaxFrontendBytes            = 2 << 20
	directEmptyArgument               = "<RoundHouseKick>"
)

var (
	cacheBreakerBundlePattern = regexp.MustCompile(
		`(?i)(?:src|href)\s*=\s*["']([^"']*CacheBreaker\.bundle\.[a-f0-9]+\.js)["']`,
	)
	transpilationVersionPattern = regexp.MustCompile(
		`name\s*:\s*["']TranspilationEmpire["']\s*,\s*version\s*:\s*["']([0-9]+)\.([0-9]+)\.([0-9]+)["']`,
	)
)

type DirectWebSocketConfig struct {
	DataDir    string
	HTTPClient *http.Client
	Server     string
	ServerURL  string
	Namespace  string
	Language   string

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
	// parkedFailure records why the run loop stopped on its own (a login
	// failure only the user or the account holder can resolve, or a released
	// session waiting for parkedRetryAt). Start refuses to spend the same saved
	// login again until it changes, the wait elapses, or a reconnect is forced.
	parkedFailure      *State.LoginFailure
	parkedCredentialAt time.Time
	parkedRetryAt      time.Time
	forceNextStart     bool
	reconnectPolicy    ReconnectPolicy

	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   []directPendingResponse
	// allianceHelpPlayerID and allianceHelpCastleID are refreshed from the
	// authoritative JAA castle-focus reply. Recruitment AHR does not carry its
	// target in the outbound payload, so each pending request snapshots this
	// identity before it is written to the socket.
	allianceHelpPlayerID  int64
	allianceHelpCastleID  int64
	ahrRejectionAmbiguous bool
}

type directPendingResponse struct {
	token         string
	opcodes       map[string]struct{}
	expiresAt     time.Time
	requestOpcode string
	requestID     int64
	requestType   int
	requestPlayer int64
	requestCastle int64
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

// ErrLoginParked is returned by Start while the transport is parked on a login
// failure that retrying cannot fix. Saving a different login or server
// selection clears the park.
var ErrLoginParked = errors.New("game login is parked until the saved login or server selection changes")

// maximumUnknownLoginRetryDelay caps the doubling retry spacing used for login
// codes without an established meaning.
const maximumUnknownLoginRetryDelay = time.Hour

const (
	defaultReleaseRetryDelay    = 5 * time.Second
	defaultReleaseRetryAttempts = 3
)

// ErrReconnectScheduled is returned by Start while a released or cooling-down
// session is waiting for its retry time; ClearReconnectHold or a changed login
// bypasses it.
var ErrReconnectScheduled = errors.New("game reconnect is scheduled; the retry time has not elapsed")

// loginParkNeedsUserAction reports whether a fatal login class can only be
// resolved by a credential, server, or account change, so the runtime parks
// instead of retrying.
func loginParkNeedsUserAction(class State.LoginFailureClass) bool {
	switch class {
	case State.LoginFailureInvalidCredentials, State.LoginFailureWrongServer,
		State.LoginFailureAccountDeleted, State.LoginFailureSuspended:
		return true
	default:
		return false
	}
}

type directLoginError struct {
	code                  int
	cooldownSec           int
	suspendedSec          int
	accountDeleted        bool
	detail                string
	fatal                 bool
	clientVersionRejected bool
}

func (err *directLoginError) Error() string { return err.detail }

func (err *directLoginError) failure(observedAt time.Time) *State.LoginFailure {
	if err == nil {
		return nil
	}
	class, _ := State.ClassifyLoginFailure(err.code, err.clientVersionRejected)
	if class == State.LoginFailureSuspended && err.accountDeleted {
		class = State.LoginFailureAccountDeleted
	}
	failure := &State.LoginFailure{Code: err.code, Class: class, Fatal: err.fatal, ObservedAt: observedAt.UTC()}
	if class == State.LoginFailureSuspended && err.suspendedSec > 0 {
		until := observedAt.UTC().Add(time.Duration(err.suspendedSec) * time.Second)
		failure.SuspendedUntil = &until
	}
	return failure
}

func NewDirectWebSocketTransport(config DirectWebSocketConfig) *DirectWebSocketTransport {
	credential, profile, resolveErr := loadDirectWebSocketBootstrap(config)
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

func loadDirectWebSocketBootstrap(
	config DirectWebSocketConfig,
) (persistedLoginCredential, gameConnectionProfile, error) {
	credential, credentialErr := loadBackgroundLoginCredential(config.DataDir)
	if errors.Is(credentialErr, os.ErrNotExist) {
		credential, credentialErr = loadLoginCredential(config.DataDir)
	}
	profile, profileErr := loadGameConnectionProfile(config.DataDir)
	var resolveErr error
	switch {
	case errors.Is(credentialErr, os.ErrNotExist):
		resolveErr = fmt.Errorf("Background mode needs a saved game login; enter username, password, and server in Settings or sign in once in Full application mode")
	case credentialErr != nil:
		resolveErr = fmt.Errorf("Background mode rejected the saved game login: %w", credentialErr)
	case !credential.AutoRestore:
		resolveErr = fmt.Errorf("Background mode cannot use a saved login that has been disabled")
	case profileErr != nil && !errors.Is(profileErr, os.ErrNotExist) && strings.TrimSpace(credential.ServerURL) == "":
		resolveErr = fmt.Errorf("Background mode rejected the saved game connection details: %w", profileErr)
	default:
		profile, resolveErr = resolveDirectGameProfile(config, credential, profile, profileErr == nil)
	}
	return credential, profile, resolveErr
}

func resolveDirectGameProfile(
	config DirectWebSocketConfig,
	credential persistedLoginCredential,
	savedProfile gameConnectionProfile,
	hasSavedProfile bool,
) (gameConnectionProfile, error) {
	selectedServerURL := strings.TrimSpace(credential.ServerURL)
	if selectedServerURL == "" {
		if configuredServerURL, _, err := backgroundGameServerURL(config.Server); err == nil {
			selectedServerURL = configuredServerURL
		}
	}
	if selectedServerURL == "" {
		selectedServerURL = strings.TrimSpace(config.ServerURL)
		if validateDirectGameServerURL(selectedServerURL) != nil {
			selectedServerURL = ""
		}
	}
	if selectedServerURL == "" && hasSavedProfile {
		selectedServerURL = savedProfile.ServerURL
	}
	if selectedServerURL == "" {
		return gameConnectionProfile{}, fmt.Errorf(
			"Background mode needs a saved game server selection; enter the server in Settings",
		)
	}
	// The zone belongs to the selected world, not to the process: an
	// explicitly pinned zone (installed with the login by the hosted control
	// plane) wins; otherwise several worlds share one multi-zone host, so the
	// saved world code decides, then a single-world host, and only then
	// whatever the state last saw.
	selectedCode := strings.TrimSpace(credential.Server)
	if selectedCode == "" {
		selectedCode = strings.TrimSpace(config.Server)
	}
	catalogZone, zoneKnown := gameServerZoneForURL(selectedServerURL, selectedCode)
	if pinnedZone := strings.TrimSpace(credential.Zone); pinnedZone != "" && gameNamespacePattern.MatchString(pinnedZone) {
		catalogZone, zoneKnown = pinnedZone, true
	}
	if hasSavedProfile && selectedServerURL == savedProfile.ServerURL {
		if zoneKnown && (credential.Server != "" || credential.Zone != "") && savedProfile.Namespace != catalogZone {
			// A profile derived before the world code was recorded carries the
			// default zone; re-derive it for the world the login is saved for.
			return derivedGameConnectionProfile(selectedServerURL, catalogZone, resolvedLoginLanguage(config, credential))
		}
		return savedProfile, nil
	}

	namespace := strings.TrimSpace(config.Namespace)
	if zoneKnown {
		namespace = catalogZone
	}
	if !gameNamespacePattern.MatchString(namespace) {
		namespace = defaultGameNamespace
	}
	profile, err := derivedGameConnectionProfile(selectedServerURL, namespace, resolvedLoginLanguage(config, credential))
	if err != nil {
		return gameConnectionProfile{}, fmt.Errorf("Background mode cannot use the saved game server selection: %w", err)
	}
	return profile, nil
}

func resolvedLoginLanguage(config DirectWebSocketConfig, credential persistedLoginCredential) string {
	language := strings.TrimSpace(credential.Language)
	if !loginLanguagePattern.MatchString(language) {
		language = strings.TrimSpace(config.Language)
	}
	if !loginLanguagePattern.MatchString(language) {
		language = defaultGameLanguage
	}
	return language
}

func (transport *DirectWebSocketTransport) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	transport.mu.Lock()
	if transport.cancel != nil {
		transport.mu.Unlock()
		return nil
	}
	credential, profile, resolveErr := loadDirectWebSocketBootstrap(transport.config)
	force := transport.forceNextStart
	transport.forceNextStart = false
	if resolveErr == nil && !force && credential.CapturedAt.Equal(transport.parkedCredentialAt) {
		if parked := transport.parkedFailure; parked != nil && loginParkNeedsUserAction(parked.Class) {
			// The same saved login already failed for a reason retrying cannot
			// fix (wrong password, wrong server, suspended, deactivated).
			// Spending it again would only earn a login cooldown; wait for a
			// changed login or an explicit reconnect.
			transport.mu.Unlock()
			return fmt.Errorf("%w: %s (code %d)", ErrLoginParked, parked.Class, parked.Code)
		}
		if !transport.parkedRetryAt.IsZero() && time.Now().Before(transport.parkedRetryAt) {
			// A released session (or a suspension) is waiting for its retry
			// time; an unforced Start must not shorten the game's cooldown.
			retryAt := transport.parkedRetryAt
			transport.mu.Unlock()
			return fmt.Errorf("%w: retry at %s", ErrReconnectScheduled, retryAt.Format(time.RFC3339))
		}
	}
	transport.parkedFailure = nil
	transport.parkedCredentialAt = time.Time{}
	transport.parkedRetryAt = time.Time{}
	transport.credential = credential
	transport.profile = profile
	transport.resolveErr = resolveErr
	if resolveErr != nil {
		status := Status{
			Mode: ConnectionModeBackground, State: "unavailable", Namespace: profile.Namespace,
			ServerURL: profile.ServerURL, Detail: resolveErr.Error(), ChangedAt: time.Now().UTC(),
		}
		transport.status = status
		transport.mu.Unlock()
		transport.enqueueStatus(status)
		return resolveErr
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
	credential, err := loadBackgroundLoginCredential(transport.config.DataDir)
	backgroundCredential := err == nil
	if errors.Is(err, os.ErrNotExist) {
		credential, err = loadLoginCredential(transport.config.DataDir)
	}
	if err != nil {
		return fmt.Errorf("Background mode needs a valid saved game login; enter it in Settings or sign in once in Full application mode")
	}
	if !credential.AutoRestore {
		credential.AutoRestore = true
		credential.CapturedAt = time.Now().UTC()
		if backgroundCredential {
			err = saveBackgroundLoginCredential(transport.config.DataDir, credential)
		} else {
			err = saveLoginCredential(transport.config.DataDir, credential)
		}
		if err != nil {
			return fmt.Errorf("authorize the saved game login for Background mode: %w", err)
		}
	}
	credential, profile, err := loadDirectWebSocketBootstrap(transport.config)
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

// run keeps the account connected for as long as the transport is started.
// Every interruption is followed by a reconnect after the user-configured
// relog delay, except the login outcomes that retrying cannot fix:
//
//   - transient drops and rejected client builds: relog delay;
//   - LOGIN_COOLDOWN_ACTIVE (453): the game's cooldown, then the relog delay;
//   - a temporary suspension (27 with a remaining time): resume automatically
//     when the suspension ends, plus the relog delay;
//   - a code without an established meaning: retry with the relog delay
//     doubling per repeat, capped at one hour;
//   - invalid credentials, wrong server, permanent suspension, or a deactivated
//     account: park until the saved login or server selection changes.
func (transport *DirectWebSocketTransport) run(ctx context.Context, generation uint64) {
	unknownFailures := 0
	for {
		err := transport.connectAndServe(ctx, generation)
		if ctx.Err() != nil || !transport.isCurrent(generation) {
			return
		}
		var loginErr *directLoginError
		isLoginErr := errors.As(err, &loginErr)
		if !isLoginErr {
			unknownFailures = 0
		}
		displaced := !isLoginErr && isDisplacementClose(err)
		now := time.Now().UTC()
		delay := transport.relogDelay()
		status := Status{
			Mode: ConnectionModeBackground, State: "reconnecting", Namespace: transport.profile.Namespace,
			ServerURL: transport.profile.ServerURL, LoggedIn: false, SocketReady: false,
			Detail: fmt.Sprintf("Background game connection closed: %v", err), ChangedAt: now,
		}
		if isLoginErr {
			status.LoginFailure = loginErr.failure(now)
			status.Detail = loginErr.Error()
		}
		if displaced {
			status.Detail = fmt.Sprintf(
				"The game session was taken over by another login; waiting the configured relog delay (%s) before reconnecting",
				delay.Round(time.Second),
			)
		} else if !isLoginErr {
			status.Detail = fmt.Sprintf(
				"Game connection closed; waiting the configured relog delay (%s) before reconnecting", delay.Round(time.Second),
			)
		}
		if transport.currentReconnectPolicy() == ReconnectPolicyRelease {
			// Under the release policy the runtime's lifetime follows the game
			// connection, and EVERY disconnect is treated as a displacement:
			// the session is released at once and the configured relog delay
			// is the wait before the control plane brings a runtime back.
			// There is deliberately no immediate-retry window — the game kicks
			// this session with a plain socket drop when the player logs in,
			// and an instant relog would kick the player straight back out.
			transport.release(generation, status, loginErr, isLoginErr, delay, now)
			return
		}
		switch {
		case isLoginErr && loginErr.code == 453:
			cooldownUntil := now.Add(time.Duration(max(0, loginErr.cooldownSec)) * time.Second)
			retryAt := cooldownUntil.Add(delay)
			status.State = "cooldown"
			status.CooldownUntil = &cooldownUntil
			status.RetryAt = &retryAt
			delay = time.Until(retryAt)
		case isLoginErr && loginErr.fatal && status.LoginFailure.Class == State.LoginFailureSuspended &&
			status.LoginFailure.SuspendedUntil != nil:
			retryAt := status.LoginFailure.SuspendedUntil.Add(delay)
			status.State = "suspended"
			status.RetryAt = &retryAt
			delay = time.Until(retryAt)
		case isLoginErr && loginErr.fatal && status.LoginFailure.Class == State.LoginFailureUnknown:
			unknownFailures++
			for index := 1; index < unknownFailures && delay < maximumUnknownLoginRetryDelay; index++ {
				delay *= 2
			}
			if delay > maximumUnknownLoginRetryDelay {
				delay = maximumUnknownLoginRetryDelay
			}
			retryAt := now.Add(delay)
			status.State = "error"
			status.RetryAt = &retryAt
		case isLoginErr && loginErr.fatal:
			// Only a changed login, server selection, or account can help. Park:
			// publish the obstacle, release this run, and let Start refuse the
			// unchanged login until it is replaced.
			status.State = "error"
			transport.publishRunStatus(generation, status)
			transport.mu.Lock()
			var release context.CancelFunc
			if transport.runGeneration == generation {
				transport.parkedFailure = status.LoginFailure
				transport.parkedCredentialAt = transport.credential.CapturedAt
				release = transport.cancel
				transport.cancel = nil
			}
			transport.mu.Unlock()
			if release != nil {
				release()
			}
			return
		default:
			retryAt := now.Add(delay)
			status.RetryAt = &retryAt
		}
		if !transport.publishRunStatus(generation, status) {
			return
		}
		if !transport.sleep(ctx, delay) {
			return
		}
	}
}

// release publishes the "released" state for the given interruption and parks
// the run so Start refuses to reconnect before the retry time (unless the login
// changes or a reconnect is forced). RetryAt is the earliest sensible retry:
// the game's cooldown or suspension end, never sooner than the relog delay.
func (transport *DirectWebSocketTransport) release(
	generation uint64,
	status Status,
	loginErr *directLoginError,
	isLoginErr bool,
	relogDelay time.Duration,
	now time.Time,
) {
	status.State = "released"
	retryAt := now.Add(relogDelay)
	switch {
	case isLoginErr && loginErr.code == 453:
		cooldownUntil := now.Add(time.Duration(max(0, loginErr.cooldownSec)) * time.Second)
		status.CooldownUntil = &cooldownUntil
		if cooldownUntil.After(retryAt) {
			retryAt = cooldownUntil
		}
	case isLoginErr && loginErr.fatal && status.LoginFailure != nil && status.LoginFailure.SuspendedUntil != nil:
		if status.LoginFailure.SuspendedUntil.After(retryAt) {
			retryAt = *status.LoginFailure.SuspendedUntil
		}
	case isLoginErr && loginErr.fatal && status.LoginFailure != nil && loginParkNeedsUserAction(status.LoginFailure.Class):
		// Only a changed login or account can help; no retry time.
		retryAt = time.Time{}
	}
	if !retryAt.IsZero() {
		status.RetryAt = &retryAt
	}
	status.Detail = "Session released: " + status.Detail
	transport.publishRunStatus(generation, status)
	transport.mu.Lock()
	var releaseRun context.CancelFunc
	if transport.runGeneration == generation {
		transport.parkedFailure = status.LoginFailure
		transport.parkedCredentialAt = transport.credential.CapturedAt
		transport.parkedRetryAt = retryAt
		releaseRun = transport.cancel
		transport.cancel = nil
	}
	transport.mu.Unlock()
	if releaseRun != nil {
		releaseRun()
	}
}

// sleep waits for delay or the context; it reports false when the run should
// stop.
func (transport *DirectWebSocketTransport) sleep(ctx context.Context, delay time.Duration) bool {
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false
	case <-timer.C:
		return true
	}
}

func (transport *DirectWebSocketTransport) currentReconnectPolicy() ReconnectPolicy {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	if transport.reconnectPolicy == "" {
		return ReconnectPolicyHold
	}
	return transport.reconnectPolicy
}

// SetReconnectPolicy switches between holding and releasing the session after
// a disconnect. It applies to the next interruption.
func (transport *DirectWebSocketTransport) SetReconnectPolicy(policy ReconnectPolicy) {
	transport.mu.Lock()
	transport.reconnectPolicy = policy
	transport.mu.Unlock()
}

// ClearReconnectHold lets the next Start bypass a park or scheduled retry: an
// explicit reconnect requested by the user or the control plane.
func (transport *DirectWebSocketTransport) ClearReconnectHold() {
	transport.mu.Lock()
	transport.forceNextStart = true
	transport.mu.Unlock()
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
	transport.rememberSuccessfulDirectLogin(build, time.Now().UTC())
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

func (transport *DirectWebSocketTransport) rememberSuccessfulDirectLogin(build string, observedAt time.Time) {
	transport.mu.RLock()
	profile := transport.profile
	transport.mu.RUnlock()
	profile.SchemaVersion = gameConnectionProfileSchemaVersion
	profile.CapturedAt = observedAt
	profile.ClientBuild = build
	if validateGameConnectionProfile(profile) == nil {
		_ = saveGameConnectionProfile(transport.config.DataDir, profile)
	}
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
							code: code, clientVersionRejected: true,
							detail: fmt.Sprintf("Background game client version was rejected with code %d; refreshing it from the official frontend before retrying", code),
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
						loginErr := &directLoginError{code: *frame.ResponseCode}
						switch *frame.ResponseCode {
						case 453:
							// LOGIN_COOLDOWN_ACTIVE carries the remaining lockout.
							var payload struct {
								Seconds int `json:"CD"`
							}
							_ = json.Unmarshal(frame.Payload, &payload)
							loginErr.cooldownSec = max(0, payload.Seconds)
						case 27:
							// IS_BANNED carries the remaining suspension and, for a
							// deactivated account, the GDPR deletion flag.
							var payload struct {
								Seconds int  `json:"RS"`
								Deleted bool `json:"GDPR"`
							}
							_ = json.Unmarshal(frame.Payload, &payload)
							loginErr.suspendedSec = max(0, payload.Seconds)
							loginErr.accountDeleted = payload.Deleted
						}
						_, loginErr.fatal = State.ClassifyLoginFailure(*frame.ResponseCode, false)
						loginErr.detail = fmt.Sprintf("Background game login failed with code %d", *frame.ResponseCode)
						if name := State.LoginFailureCodeName(*frame.ResponseCode); name != "" {
							loginErr.detail += " (" + name + ")"
						}
						return 0, 0, nil, loginErr
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
	subscriptionTicker := time.NewTicker(directSubscriptionRefreshInterval)
	defer subscriptionTicker.Stop()
	// The official client PULLS its subscription packages (C2S "sie") at
	// startup — the server never volunteers them on login or in the gbd
	// baseline. In browser mode the embedded official client makes that
	// request itself and the sniffer ingests the answer, but in background
	// mode nobody asked, so Subscriptions state stayed empty and every
	// subscription-aware computation (e.g. the +40-per-slot recruitment
	// bonus) silently lost its input. Ask once per connection, then refresh
	// periodically so a lapse or purchase is noticed without a relog.
	// The same holds for the general roster with active skills (C2S "gie"):
	// the official client asks at login (CastleLoginEvent) and gbd does not
	// carry it, so without this pull every commander with a general assigned
	// is "skills not observed" until an attack plan happens to ask — which
	// capacity-gated lanes never do (they need the skills first).
	requestSubscriptions := func() error {
		for _, request := range []struct{ opcode, causation string }{
			{"sie", "session:background:subscription-refresh"},
			{"gie", "session:background:general-skills-refresh"},
		} {
			frame := fmt.Sprintf("%%xt%%%s%%%s%%%d%%{}%%", transport.profile.Namespace, request.opcode, roomID)
			if _, err := transport.sendInternal(
				connection, frame, connectionGeneration, request.causation, request.opcode,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if err := requestSubscriptions(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-reads:
			if result.err != nil {
				return result.err
			}
			for _, event := range decoder.append(result.payload) {
				if event.raw == "" {
					continue
				}
				transport.deliverInbound(event.raw, connectionGeneration)
				// The authenticated baseline (gbd) may follow a login-time
				// state reset that discards anything reduced before it —
				// including the subscription packages pulled right after
				// login. Re-pull after every baseline so subscription-aware
				// automation math always has its input; hasPendingOpcode
				// dedupes overlapping requests.
				if strings.Contains(event.raw, "%xt%gbd%") {
					if err := requestSubscriptions(); err != nil {
						return err
					}
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
		case <-subscriptionTicker.C:
			if err := requestSubscriptions(); err != nil {
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
	transport.invalidateAllianceHelpContextForOutbound(string(payload))
	pending, registerErr := transport.registerPending(metadata, string(payload))
	if registerErr != nil {
		transport.writeMu.Unlock()
		return registerErr
	}
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
	transport.invalidateAllianceHelpContextForOutbound(payload)
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
		ConnectionGeneration: generation, ResponseToken: transport.matchResponseToken(frame),
	}
}

func (transport *DirectWebSocketTransport) registerPending(
	metadata Outbound.Metadata,
	requestPayload string,
) (*directPendingResponse, error) {
	expiresAt := time.Now().Add(time.Duration(metadata.ResponseTimeoutMillis) * time.Millisecond)
	if metadata.ResponseTimeoutMillis <= 0 {
		expiresAt = time.Now().Add(30 * time.Second)
	}
	pending := &directPendingResponse{
		token: metadata.ResponseToken, opcodes: make(map[string]struct{}, len(metadata.ResponseOpcodes)),
		expiresAt: expiresAt, requestPlayer: metadata.ResponseIdentity.PlayerID,
		requestCastle: metadata.ResponseIdentity.CastleID,
	}
	if request, err := Protocol.Decode(requestPayload, Protocol.DirectionOutbound, time.Now().UTC()); err == nil {
		pending.requestOpcode = request.Opcode
		if request.Opcode == "ahr" && len(request.Payload) > 0 {
			var allianceHelpRequest struct {
				ID   int64 `json:"ID"`
				Type int   `json:"T"`
			}
			if json.Unmarshal(request.Payload, &allianceHelpRequest) == nil {
				pending.requestID = allianceHelpRequest.ID
				pending.requestType = allianceHelpRequest.Type
			}
		}
	}
	if pending.requestOpcode == "ahr" &&
		(strings.TrimSpace(metadata.ResponseToken) == "" || len(metadata.ResponseOpcodes) == 0) {
		return nil, fmt.Errorf("AHR dispatch requires correlated response metadata")
	}
	if strings.TrimSpace(metadata.ResponseToken) == "" || len(metadata.ResponseOpcodes) == 0 {
		return nil, nil
	}
	for _, opcode := range metadata.ResponseOpcodes {
		if opcode = strings.ToLower(strings.TrimSpace(opcode)); opcode != "" {
			pending.opcodes[opcode] = struct{}{}
		}
	}
	if len(pending.opcodes) == 0 {
		return nil, nil
	}
	transport.pendingMu.Lock()
	defer transport.pendingMu.Unlock()
	transport.purgePendingLocked(time.Now())
	if pending.requestOpcode == "ahr" {
		for _, active := range transport.pending {
			if active.requestOpcode == "ahr" {
				return nil, fmt.Errorf("AHR dispatch is already awaiting an authoritative response")
			}
		}
		if pending.requestType == 6 {
			if pending.requestPlayer <= 0 || pending.requestCastle <= 0 {
				return nil, fmt.Errorf("recruitment AHR requires an expected player and castle identity")
			}
			if transport.allianceHelpPlayerID <= 0 || transport.allianceHelpCastleID <= 0 {
				return nil, fmt.Errorf("recruitment AHR requires a current authoritative castle focus")
			}
			if pending.requestPlayer != transport.allianceHelpPlayerID ||
				pending.requestCastle != transport.allianceHelpCastleID {
				return nil, fmt.Errorf(
					"recruitment AHR focus changed before dispatch: expected player %d castle %d, observed player %d castle %d",
					pending.requestPlayer, pending.requestCastle,
					transport.allianceHelpPlayerID, transport.allianceHelpCastleID,
				)
			}
		}
	}
	transport.pending = append(transport.pending, *pending)
	return pending, nil
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

func (transport *DirectWebSocketTransport) matchResponseToken(frame Protocol.Frame) string {
	opcode := strings.ToLower(strings.TrimSpace(frame.Opcode))
	transport.pendingMu.Lock()
	defer transport.pendingMu.Unlock()
	transport.observeAllianceHelpContextLocked(frame)
	transport.purgePendingLocked(time.Now())
	if opcode == "ahr" {
		if !directPayloadlessAHRRejection(frame) {
			return ""
		}
		if transport.ahrRejectionAmbiguous {
			return ""
		}
		matchedIndex := -1
		for index, pending := range transport.pending {
			if pending.requestOpcode != "ahr" {
				continue
			}
			if _, expected := pending.opcodes[opcode]; !expected {
				continue
			}
			if matchedIndex >= 0 {
				// Native AHR rejections carry no request identity. Never let an
				// ambiguous rejection complete an arbitrary pending command.
				return ""
			}
			matchedIndex = index
		}
		if matchedIndex < 0 {
			return ""
		}
		pending := transport.pending[matchedIndex]
		transport.pending = append(transport.pending[:matchedIndex], transport.pending[matchedIndex+1:]...)
		return pending.token
	}
	for index, pending := range transport.pending {
		if _, expected := pending.opcodes[opcode]; !expected {
			continue
		}
		if !directResponseMatchesRequest(pending, frame) {
			continue
		}
		transport.pending = append(transport.pending[:index], transport.pending[index+1:]...)
		return pending.token
	}
	return ""
}

func directPayloadlessAHRRejection(frame Protocol.Frame) bool {
	if strings.ToLower(strings.TrimSpace(frame.Opcode)) != "ahr" ||
		frame.ResponseCode == nil || *frame.ResponseCode == 0 ||
		strings.TrimSpace(frame.PayloadText) != "" {
		return false
	}
	payload := strings.TrimSpace(string(frame.Payload))
	if payload == "" || payload == "null" {
		return true
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(frame.Payload, &object) == nil && object != nil && len(object) == 0
}

func (transport *DirectWebSocketTransport) invalidateAllianceHelpContextForOutbound(payload string) {
	frame, err := Protocol.Decode(payload, Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil || !allianceHelpContextTransitionOpcode(frame.Opcode) {
		return
	}
	transport.pendingMu.Lock()
	transport.allianceHelpPlayerID = 0
	transport.allianceHelpCastleID = 0
	transport.pendingMu.Unlock()
}

func allianceHelpContextTransitionOpcode(opcode string) bool {
	switch strings.ToLower(strings.TrimSpace(opcode)) {
	case "gaa", "jaa", "jca":
		return true
	default:
		return false
	}
}

func directResponseMatchesRequest(pending directPendingResponse, frame Protocol.Frame) bool {
	if pending.requestOpcode == "lta" && frame.Opcode == "gam" {
		return directKhanTauntResponseMatches(pending, frame)
	}
	if pending.requestOpcode != "ahr" || frame.Opcode != "ahh" {
		return true
	}
	if frame.ResponseCode == nil || *frame.ResponseCode != 0 {
		return false
	}
	var response struct {
		ListID           int64  `json:"LID"`
		PlayerID         int64  `json:"PID"`
		Type             int    `json:"TID"`
		Progress         *int64 `json:"P"`
		AlreadyConfirmed *int64 `json:"AC"`
		Optional         *struct {
			CastleID      int64 `json:"AID"`
			RecruitmentID int64 `json:"RID"`
			LineID        *int  `json:"RLID"`
		} `json:"OP"`
	}
	if len(frame.Payload) == 0 || json.Unmarshal(frame.Payload, &response) != nil ||
		response.Type != pending.requestType {
		return false
	}
	switch pending.requestType {
	case 2:
		return response.Optional != nil && response.Optional.RecruitmentID == pending.requestID
	case 6:
		return pending.requestPlayer > 0 && pending.requestCastle > 0 &&
			response.ListID > 0 && response.Progress != nil && *response.Progress == 0 &&
			response.AlreadyConfirmed != nil && *response.AlreadyConfirmed == 0 &&
			response.PlayerID == pending.requestPlayer &&
			response.Optional != nil && response.Optional.CastleID == pending.requestCastle &&
			response.Optional.LineID != nil && *response.Optional.LineID == 0
	default:
		return true
	}
}

// directKhanTauntResponseMatches prevents a periodic or manually requested GAM
// snapshot from acknowledging LTA. A rejection is still correlated so the
// intent can surface the game code; success requires the retaliation movement
// shape published by the game for an accepted Khan taunt.
func directKhanTauntResponseMatches(pending directPendingResponse, frame Protocol.Frame) bool {
	if frame.ResponseCode == nil {
		return false
	}
	if *frame.ResponseCode != 0 {
		return true
	}
	var response struct {
		Movements []json.RawMessage `json:"M"`
	}
	if len(frame.Payload) == 0 || json.Unmarshal(frame.Payload, &response) != nil {
		return false
	}
	for _, raw := range response.Movements {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		var movement struct {
			ID        json.RawMessage   `json:"MID"`
			Direction json.RawMessage   `json:"D"`
			TypeID    json.RawMessage   `json:"T"`
			KingdomID json.RawMessage   `json:"KID"`
			TargetID  json.RawMessage   `json:"TID"`
			Source    []json.RawMessage `json:"SA"`
			Target    []json.RawMessage `json:"TA"`
		}
		if json.Unmarshal(item["M"], &movement) != nil {
			continue
		}
		movementID, movementIDFound := directResponseInt64(movement.ID)
		direction, directionFound := directResponseInt64(movement.Direction)
		typeID, typeFound := directResponseInt64(movement.TypeID)
		kingdomID, kingdomFound := directResponseInt64(movement.KingdomID)
		sourceTypeID, sourceFound := directResponseRowInt64(movement.Source, 0)
		if !movementIDFound || movementID <= 0 || !directionFound || direction != 0 && direction != 1 ||
			!typeFound || typeID != 20 || !kingdomFound || kingdomID != 0 ||
			!sourceFound || sourceTypeID != 35 || directResponseHasCommander(item["UM"]) {
			continue
		}
		if pending.requestCastle > 0 {
			targetCastleID, found := directResponseRowInt64(movement.Target, 3)
			if !found || targetCastleID != pending.requestCastle {
				continue
			}
		}
		if pending.requestPlayer > 0 {
			targetPlayerID, _ := directResponseInt64(movement.TargetID)
			if addressPlayerID, found := directResponseRowInt64(movement.Target, 4); found && addressPlayerID != 0 {
				targetPlayerID = addressPlayerID
			}
			if targetPlayerID != pending.requestPlayer {
				continue
			}
		}
		return true
	}
	return false
}

func directResponseHasCommander(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var unitMovement struct {
		Leader struct {
			ID json.RawMessage `json:"ID"`
		} `json:"L"`
	}
	if json.Unmarshal(raw, &unitMovement) != nil {
		return true
	}
	id, found := directResponseInt64(unitMovement.Leader.ID)
	return found && id >= 0
}

func directResponseRowInt64(row []json.RawMessage, index int) (int64, bool) {
	if index < 0 || index >= len(row) {
		return 0, false
	}
	return directResponseInt64(row[index])
}

func directResponseInt64(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var value int64
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	return value, err == nil
}

func (transport *DirectWebSocketTransport) observeAllianceHelpContextLocked(frame Protocol.Frame) {
	if frame.Opcode == "gaa" && frame.ResponseCode != nil && *frame.ResponseCode == 0 {
		transport.allianceHelpPlayerID = 0
		transport.allianceHelpCastleID = 0
		return
	}
	playerID, castleID, ok := allianceHelpFocusIdentity(frame)
	if ok {
		transport.allianceHelpPlayerID = playerID
		transport.allianceHelpCastleID = castleID
	}
}

func allianceHelpFocusIdentity(frame Protocol.Frame) (int64, int64, bool) {
	if frame.Opcode != "jaa" || len(frame.Payload) == 0 ||
		frame.ResponseCode == nil || *frame.ResponseCode != 0 {
		return 0, 0, false
	}
	var response struct {
		Castle struct {
			Owner struct {
				PlayerID int64 `json:"OID"`
			} `json:"O"`
			Resources struct {
				CastleID int64 `json:"AID"`
			} `json:"grc"`
			Address []json.RawMessage `json:"A"`
		} `json:"gca"`
	}
	if json.Unmarshal(frame.Payload, &response) != nil {
		return 0, 0, false
	}
	playerID := response.Castle.Owner.PlayerID
	castleID := response.Castle.Resources.CastleID
	if playerID <= 0 && len(response.Castle.Address) > 4 {
		_ = json.Unmarshal(response.Castle.Address[4], &playerID)
	}
	if castleID <= 0 && len(response.Castle.Address) > 3 {
		_ = json.Unmarshal(response.Castle.Address[3], &castleID)
	}
	if playerID > 0 && castleID > 0 {
		return playerID, castleID, true
	}
	return 0, 0, false
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
		} else if pending.requestOpcode == "ahr" {
			// A native AHR rejection has no request identity. Once an AHR wait
			// expires, a later rejection could belong to it or to any newer AHR.
			// Keep that ambiguity latched until this socket is replaced.
			transport.ahrRejectionAmbiguous = true
		}
	}
	transport.pending = kept
}

func (transport *DirectWebSocketTransport) clearPending() {
	transport.pendingMu.Lock()
	transport.pending = nil
	transport.allianceHelpPlayerID = 0
	transport.allianceHelpCastleID = 0
	transport.ahrRejectionAmbiguous = false
	transport.pendingMu.Unlock()
}

func (transport *DirectWebSocketTransport) resolveBuild(ctx context.Context) (string, error) {
	fallback := strings.TrimSpace(transport.profile.ClientBuild)
	if transport.config.buildResolver != nil {
		build, err := transport.config.buildResolver(ctx, fallback)
		if err != nil {
			return "", err
		}
		if !gameBuildPattern.MatchString(strings.TrimSpace(build)) {
			return "", fmt.Errorf("background game client build resolver returned an invalid value")
		}
		return strings.TrimSpace(build), nil
	}
	client := transport.config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	build, err := resolveCurrentGameBuild(ctx, client)
	if err == nil {
		return build, nil
	}
	if gameBuildPattern.MatchString(fallback) {
		return fallback, nil
	}
	return "", fmt.Errorf("resolve current background game client build: %w", err)
}

func resolveCurrentGameBuild(ctx context.Context, client *http.Client) (string, error) {
	index, err := fetchDirectFrontendDocument(ctx, client, directGameIndexURL)
	if err != nil {
		return "", fmt.Errorf("load game frontend index: %w", err)
	}
	bundleURL, err := resolveCacheBreakerBundleURL(index)
	if err != nil {
		return "", err
	}
	bundle, err := fetchDirectFrontendDocument(ctx, client, bundleURL)
	if err != nil {
		return "", fmt.Errorf("load game frontend version bundle: %w", err)
	}
	return transpilationBuild(bundle)
}

func fetchDirectFrontendDocument(ctx context.Context, client *http.Client, sourceURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, directMaxFrontendBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > directMaxFrontendBytes {
		return nil, fmt.Errorf("game frontend document is too large")
	}
	return contents, nil
}

func resolveCacheBreakerBundleURL(index []byte) (string, error) {
	match := cacheBreakerBundlePattern.FindSubmatch(index)
	if len(match) != 2 {
		return "", fmt.Errorf("game frontend index did not identify its version bundle")
	}
	base, _ := url.Parse(directGameIndexURL)
	reference, err := url.Parse(string(match[1]))
	if err != nil {
		return "", fmt.Errorf("game frontend version bundle URL is invalid")
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "https" || resolved.User != nil || resolved.Fragment != "" ||
		!strings.EqualFold(resolved.Hostname(), base.Hostname()) || resolved.Port() != "" ||
		!strings.HasPrefix(resolved.EscapedPath(), "/default/CacheBreaker.bundle.") ||
		!strings.HasSuffix(resolved.EscapedPath(), ".js") {
		return "", fmt.Errorf("game frontend version bundle is not an approved official asset")
	}
	return resolved.String(), nil
}

func transpilationBuild(contents []byte) (string, error) {
	match := transpilationVersionPattern.FindSubmatch(contents)
	if len(match) != 4 {
		return "", fmt.Errorf("game frontend version bundle did not contain the client build")
	}
	major, majorErr := strconv.ParseUint(string(match[1]), 10, 32)
	minor, minorErr := strconv.ParseUint(string(match[2]), 10, 32)
	patch, patchErr := strconv.ParseUint(string(match[3]), 10, 32)
	if majorErr != nil || minorErr != nil || patchErr != nil || minor > 999 || patch > 999 {
		return "", fmt.Errorf("game frontend client build is invalid")
	}
	build := strconv.FormatUint(major*1_000_000+minor*1_000+patch, 10)
	if !gameBuildPattern.MatchString(build) {
		return "", fmt.Errorf("game frontend client build is invalid")
	}
	return build, nil
}

// isDisplacementClose reports whether the connection ended with a close frame
// the server sent on purpose. The game closes an account's socket cleanly when
// a second login takes the session over (the player signing in); network
// faults surface as read errors without a close frame. Server restarts also
// close cleanly — waiting the relog delay is the right behaviour there too.
func isDisplacementClose(err error) bool {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return false
	}
	// 1006 is synthesized locally when the connection dies WITHOUT a close
	// frame (network fault) — that is a plain drop, not a displacement.
	return closeErr.Code != websocket.CloseAbnormalClosure
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

// Running reports whether the connection loop is active. A transport that
// parked itself on a login failure is not running and may be started again once
// its saved login changes.
func (transport *DirectWebSocketTransport) Running() bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.cancel != nil
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
