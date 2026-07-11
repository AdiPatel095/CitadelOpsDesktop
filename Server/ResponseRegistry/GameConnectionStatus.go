package ResponseRegistry

import (
	"fmt"
	"sync"
	"time"
)

type GameConnectionState string

const (
	GameConnectionStopped        GameConnectionState = "stopped"
	GameConnectionStarting       GameConnectionState = "starting"
	GameConnectionConnecting     GameConnectionState = "connecting"
	GameConnectionAuthenticating GameConnectionState = "authenticating"
	GameConnectionConnected      GameConnectionState = "connected"
	GameConnectionCooldown       GameConnectionState = "cooldown"
	GameConnectionReconnecting   GameConnectionState = "reconnecting"
	GameConnectionDisconnected   GameConnectionState = "disconnected"
	GameConnectionError          GameConnectionState = "error"
)

// GameConnectionStatus is the authoritative game-browser/WebSocket lifecycle snapshot sent to the dashboard.
// LoggedIn and Cooldown remain for compatibility with older dashboard builds.
type GameConnectionStatus struct {
	State           GameConnectionState `json:"state"`
	LoggedIn        bool                `json:"loggedIn"`
	BrowserRunning  bool                `json:"browserRunning"`
	SocketConnected bool                `json:"socketConnected"`
	Cooldown        int                 `json:"cooldown"`
	CooldownUntil   int64               `json:"cooldownUntil"`
	RetryAt         int64               `json:"retryAt"`
	Revision        uint64              `json:"revision"`
	ChangedAt       int64               `json:"changedAt"`
	Detail          string              `json:"detail,omitempty"`
}

type trackedGameSocket struct {
	Generation uint64
	Open       bool
	URL        string
}

var gameConnection = struct {
	sync.RWMutex
	status                 GameConnectionStatus
	generation             uint64
	browserSession         uint64
	sockets                map[string]trackedGameSocket
	authenticatedRequestID string
	acceptSockets          bool
	stopInProgress         bool
	callback               func(GameConnectionStatus)
}{
	status: GameConnectionStatus{
		State:     GameConnectionStopped,
		Revision:  1,
		ChangedAt: time.Now().UnixMilli(),
	},
	generation: 1,
	sockets:    make(map[string]trackedGameSocket),
}

// LoginStatus and LoginCooldown are retained for existing automation callers. New status consumers should use
// GetGameConnectionStatus so the fields are read as one consistent snapshot.
var (
	LoginStatus   bool
	LoginCooldown int
)

func SetGameLoginStatusCallback(fn func(GameConnectionStatus)) {
	gameConnection.Lock()
	gameConnection.callback = fn
	gameConnection.Unlock()
}

func GetGameConnectionStatus() GameConnectionStatus {
	gameConnection.RLock()
	status := gameConnectionStatusSnapshotLocked(time.Now())
	gameConnection.RUnlock()
	return status
}

func gameConnectionStatusSnapshotLocked(now time.Time) GameConnectionStatus {
	status := gameConnection.status
	if status.CooldownUntil > 0 {
		remaining := time.UnixMilli(status.CooldownUntil).Sub(now)
		if now.UnixMilli() >= status.CooldownUntil {
			status.Cooldown = 0
		} else {
			status.Cooldown = int((remaining + time.Second - 1) / time.Second)
		}
	}
	return status
}

func setGameConnectionStatusLocked(
	state GameConnectionState,
	browserRunning bool,
	loggedIn bool,
	cooldownUntil int64,
	retryAt int64,
	detail string,
) {
	now := time.Now()
	gameConnection.status = GameConnectionStatus{
		State:           state,
		LoggedIn:        loggedIn,
		BrowserRunning:  browserRunning,
		SocketConnected: hasOpenGameSocketLocked(),
		CooldownUntil:   cooldownUntil,
		RetryAt:         retryAt,
		Revision:        gameConnection.status.Revision + 1,
		ChangedAt:       now.UnixMilli(),
		Detail:          detail,
	}
	status := gameConnectionStatusSnapshotLocked(now)
	gameConnection.status.Cooldown = status.Cooldown
	LoginStatus = status.LoggedIn
	LoginCooldown = status.Cooldown
	if gameConnection.callback != nil {
		// Publish while holding the state lock so concurrent transitions cannot enqueue out of order.
		gameConnection.callback(status)
	}
}

func hasOpenGameSocketLocked() bool {
	for _, socket := range gameConnection.sockets {
		if socket.Generation == gameConnection.generation && socket.Open {
			return true
		}
	}
	return false
}

func pendingGameConnectionStateLocked() GameConnectionState {
	if hasOpenGameSocketLocked() {
		return GameConnectionAuthenticating
	}
	return GameConnectionConnecting
}

func beginGameConnectionAttempt(state GameConnectionState, browserRunning bool) uint64 {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	gameConnection.generation++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = true
	gameConnection.stopInProgress = false
	setGameConnectionStatusLocked(state, browserRunning, false, 0, 0, "")
	return gameConnection.generation
}

func beginGameBrowserStart() (generation uint64, browserSession uint64, started bool) {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	if gameConnection.status.State == GameConnectionStarting {
		return 0, 0, false
	}
	gameConnection.generation++
	gameConnection.browserSession++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = true
	gameConnection.stopInProgress = false
	setGameConnectionStatusLocked(GameConnectionStarting, true, false, 0, 0, "")
	return gameConnection.generation, gameConnection.browserSession, true
}

func isCurrentGameBrowserStart(generation uint64) bool {
	gameConnection.RLock()
	current := gameConnection.generation == generation && gameConnection.status.State == GameConnectionStarting
	gameConnection.RUnlock()
	return current
}

func markGameConnectionStopped(browserRunning bool) {
	gameConnection.Lock()
	gameConnection.generation++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = false
	gameConnection.stopInProgress = false
	setGameConnectionStatusLocked(GameConnectionStopped, browserRunning, false, 0, 0, "")
	gameConnection.Unlock()
}

func beginGameConnectionStop() uint64 {
	gameConnection.Lock()
	gameConnection.stopInProgress = true
	generation := gameConnection.generation
	gameConnection.Unlock()
	return generation
}

func finishGameConnectionStop(generation uint64, browserRunning bool, succeeded bool) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	if gameConnection.generation != generation {
		return false
	}
	gameConnection.stopInProgress = false
	if !succeeded {
		return false
	}
	gameConnection.generation++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = false
	setGameConnectionStatusLocked(GameConnectionStopped, browserRunning, false, 0, 0, "")
	return true
}

func markGameConnectionError(generation uint64, browserRunning bool, detail string) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	if gameConnection.generation != generation {
		return false
	}
	switch gameConnection.status.State {
	case GameConnectionStarting, GameConnectionReconnecting, GameConnectionConnecting, GameConnectionAuthenticating:
	default:
		return false
	}
	gameConnection.generation++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = false
	gameConnection.stopInProgress = false
	setGameConnectionStatusLocked(GameConnectionError, browserRunning, false, 0, 0, detail)
	return true
}

func markGameContextEnded(browserSession uint64) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	if gameConnection.browserSession != browserSession {
		return false
	}
	if gameConnection.status.State == GameConnectionStopped {
		if gameConnection.status.BrowserRunning {
			setGameConnectionStatusLocked(GameConnectionStopped, false, false, 0, 0, "")
		}
		return false
	}
	if gameConnection.status.State == GameConnectionError {
		if gameConnection.status.BrowserRunning {
			setGameConnectionStatusLocked(GameConnectionError, false, false, 0, 0, gameConnection.status.Detail)
		}
		return false
	}
	gameConnection.generation++
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = false
	setGameConnectionStatusLocked(GameConnectionDisconnected, false, false, 0, 0, "Game browser closed")
	return true
}

func trackGameRequestID(browserSession uint64, id string) bool {
	return trackGameRequestIDURL(browserSession, id, "")
}

func trackGameRequestIDURL(browserSession uint64, id, socketURL string) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	if gameConnection.browserSession != browserSession || !gameConnection.acceptSockets {
		return false
	}
	gameConnection.sockets[id] = trackedGameSocket{Generation: gameConnection.generation, URL: socketURL}
	if gameConnection.status.State != GameConnectionConnected {
		gameConnection.authenticatedRequestID = ""
		setGameConnectionStatusLocked(GameConnectionConnecting, true, false, 0, 0, "")
	}
	return true
}

// CurrentGameServerURL returns the authenticated game socket URL, or the newest tracked game socket URL.
func CurrentGameServerURL() string {
	gameConnection.RLock()
	defer gameConnection.RUnlock()
	if socket, ok := gameConnection.sockets[gameConnection.authenticatedRequestID]; ok {
		return socket.URL
	}
	for _, socket := range gameConnection.sockets {
		if socket.Generation == gameConnection.generation && socket.URL != "" {
			return socket.URL
		}
	}
	return ""
}

func markGameSocketOpen(browserSession uint64, id string) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	socket, ok := gameConnection.sockets[id]
	if gameConnection.browserSession != browserSession ||
		!ok || socket.Generation != gameConnection.generation || !gameConnection.acceptSockets {
		return false
	}
	if socket.Open {
		return true
	}
	socket.Open = true
	gameConnection.sockets[id] = socket
	if gameConnection.status.State != GameConnectionConnected {
		setGameConnectionStatusLocked(GameConnectionAuthenticating, true, false, 0, 0, "")
	}
	return true
}

func isTrackedGameRequestID(browserSession uint64, id string) bool {
	gameConnection.RLock()
	socket, ok := gameConnection.sockets[id]
	tracked := gameConnection.browserSession == browserSession &&
		ok && socket.Generation == gameConnection.generation && gameConnection.acceptSockets
	gameConnection.RUnlock()
	return tracked
}

func closeTrackedGameSocket(browserSession uint64, id string) (tracked bool, connectionLost bool) {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	socket, ok := gameConnection.sockets[id]
	if gameConnection.browserSession != browserSession || !ok || socket.Generation != gameConnection.generation {
		return false, false
	}
	delete(gameConnection.sockets, id)
	wasAuthenticated := gameConnection.authenticatedRequestID == id
	if wasAuthenticated {
		gameConnection.authenticatedRequestID = ""
	}

	if !gameConnection.acceptSockets {
		return true, false
	}
	if !wasAuthenticated && gameConnection.status.LoggedIn {
		return true, false
	}
	if len(gameConnection.sockets) > 0 {
		setGameConnectionStatusLocked(pendingGameConnectionStateLocked(), true, false, 0, 0, "")
		return true, true
	}
	if gameConnection.status.State == GameConnectionCooldown && gameConnection.status.RetryAt > 0 {
		setGameConnectionStatusLocked(
			GameConnectionCooldown,
			true,
			false,
			gameConnection.status.CooldownUntil,
			gameConnection.status.RetryAt,
			gameConnection.status.Detail,
		)
		return true, true
	}
	setGameConnectionStatusLocked(GameConnectionDisconnected, true, false, 0, 0, "Game WebSocket closed")
	return true, true
}

func markGameSocketFailure(browserSession uint64, id string, detail string) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	socket, ok := gameConnection.sockets[id]
	if gameConnection.browserSession != browserSession || !ok || socket.Generation != gameConnection.generation {
		return false
	}
	delete(gameConnection.sockets, id)
	wasAuthenticated := gameConnection.authenticatedRequestID == id
	if !wasAuthenticated && gameConnection.status.LoggedIn {
		return false
	}
	if wasAuthenticated {
		gameConnection.authenticatedRequestID = ""
	}
	if len(gameConnection.sockets) > 0 {
		setGameConnectionStatusLocked(pendingGameConnectionStateLocked(), true, false, 0, 0, detail)
		return true
	}
	if gameConnection.status.State == GameConnectionCooldown && gameConnection.status.RetryAt > 0 {
		setGameConnectionStatusLocked(
			GameConnectionCooldown,
			true,
			false,
			gameConnection.status.CooldownUntil,
			gameConnection.status.RetryAt,
			gameConnection.status.Detail,
		)
		return true
	}
	if detail == "" {
		detail = "Game WebSocket error"
	}
	setGameConnectionStatusLocked(GameConnectionError, true, false, 0, 0, detail)
	return true
}

func markGameLoginSucceeded(browserSession uint64, id string) bool {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	socket, ok := gameConnection.sockets[id]
	if gameConnection.browserSession != browserSession ||
		!ok || socket.Generation != gameConnection.generation || !socket.Open || !gameConnection.acceptSockets {
		return false
	}
	gameConnection.authenticatedRequestID = id
	setGameConnectionStatusLocked(GameConnectionConnected, true, true, 0, 0, "")
	return true
}

func markGameLoginCooldown(browserSession uint64, id string, cooldown int) (uint64, int64, bool) {
	gameConnection.Lock()
	defer gameConnection.Unlock()
	socket, ok := gameConnection.sockets[id]
	if gameConnection.browserSession != browserSession ||
		!ok || socket.Generation != gameConnection.generation || !gameConnection.acceptSockets {
		return 0, 0, false
	}
	if gameConnection.status.LoggedIn && gameConnection.authenticatedRequestID != id {
		return 0, 0, false
	}
	if cooldown < 0 {
		cooldown = 0
	}
	now := time.Now()
	cooldownUntil := now.Add(time.Duration(cooldown) * time.Second).UnixMilli()
	retryAt := now.Add(time.Duration(cooldown+5) * time.Second).UnixMilli()
	gameConnection.authenticatedRequestID = ""
	setGameConnectionStatusLocked(
		GameConnectionCooldown,
		true,
		false,
		cooldownUntil,
		retryAt,
		fmt.Sprintf("Login retry scheduled in %d seconds", cooldown+5),
	)
	return gameConnection.generation, retryAt, true
}

func isCurrentGameLoginRetry(generation uint64, retryAt int64) bool {
	gameConnection.RLock()
	current := gameConnection.generation == generation &&
		gameConnection.status.State == GameConnectionCooldown &&
		gameConnection.status.RetryAt == retryAt &&
		!gameConnection.status.LoggedIn &&
		!gameConnection.stopInProgress
	gameConnection.RUnlock()
	return current
}

func hasTrackedOpenGameSocket() bool {
	gameConnection.RLock()
	ready := hasOpenGameSocketLocked()
	gameConnection.RUnlock()
	return ready
}
