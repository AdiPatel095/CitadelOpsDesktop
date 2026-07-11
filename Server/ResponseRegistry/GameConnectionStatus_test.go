package ResponseRegistry

import (
	"testing"
	"time"
)

const testBrowserSession uint64 = 1

func resetGameConnectionStatusForTest() {
	gameConnection.Lock()
	gameConnection.status = GameConnectionStatus{
		State:     GameConnectionStopped,
		Revision:  1,
		ChangedAt: time.Now().UnixMilli(),
	}
	gameConnection.generation++
	gameConnection.browserSession = testBrowserSession
	gameConnection.sockets = make(map[string]trackedGameSocket)
	gameConnection.authenticatedRequestID = ""
	gameConnection.acceptSockets = false
	gameConnection.stopInProgress = false
	gameConnection.callback = nil
	LoginStatus = false
	LoginCooldown = 0
	gameConnection.Unlock()
}

func TestReloadInvalidatesAuthenticatedSocket(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	if !trackGameRequestID(testBrowserSession, "old") ||
		!markGameSocketOpen(testBrowserSession, "old") ||
		!markGameLoginSucceeded(testBrowserSession, "old") {
		t.Fatal("could not establish the test game connection")
	}

	beginGameConnectionAttempt(GameConnectionReconnecting, true)
	status := GetGameConnectionStatus()
	if status.State != GameConnectionReconnecting || status.LoggedIn || status.SocketConnected {
		t.Fatalf("reload status = %#v", status)
	}
	if markGameLoginSucceeded(testBrowserSession, "old") {
		t.Fatal("stale socket restored login after reload")
	}
}

func TestClosingPendingSocketKeepsAuthenticatedSocketConnected(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	trackGameRequestID(testBrowserSession, "authenticated")
	markGameSocketOpen(testBrowserSession, "authenticated")
	markGameLoginSucceeded(testBrowserSession, "authenticated")
	trackGameRequestID(testBrowserSession, "pending")
	markGameSocketOpen(testBrowserSession, "pending")

	tracked, connectionLost := closeTrackedGameSocket(testBrowserSession, "pending")
	if !tracked || connectionLost {
		t.Fatalf("pending close = tracked %v, connectionLost %v", tracked, connectionLost)
	}
	status := GetGameConnectionStatus()
	if status.State != GameConnectionConnected || !status.LoggedIn || !status.SocketConnected {
		t.Fatalf("authenticated connection was demoted: %#v", status)
	}
}

func TestClosingAuthenticatedSocketFallsBackToPendingSocket(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	trackGameRequestID(testBrowserSession, "authenticated")
	markGameSocketOpen(testBrowserSession, "authenticated")
	markGameLoginSucceeded(testBrowserSession, "authenticated")
	trackGameRequestID(testBrowserSession, "pending")
	markGameSocketOpen(testBrowserSession, "pending")

	tracked, connectionLost := closeTrackedGameSocket(testBrowserSession, "authenticated")
	if !tracked || !connectionLost {
		t.Fatalf("authenticated close = tracked %v, connectionLost %v", tracked, connectionLost)
	}
	status := GetGameConnectionStatus()
	if status.State != GameConnectionAuthenticating || status.LoggedIn || !status.SocketConnected {
		t.Fatalf("pending connection status = %#v", status)
	}
}

func TestCooldownRetryBelongsToCurrentGeneration(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	trackGameRequestID(testBrowserSession, "cooldown")
	markGameSocketOpen(testBrowserSession, "cooldown")

	generation, retryAt, ok := markGameLoginCooldown(testBrowserSession, "cooldown", 30)
	if !ok {
		t.Fatal("cooldown transition was rejected")
	}
	status := GetGameConnectionStatus()
	if status.State != GameConnectionCooldown || status.LoggedIn || status.Cooldown <= 0 || status.RetryAt <= status.CooldownUntil {
		t.Fatalf("cooldown status = %#v", status)
	}
	if !isCurrentGameLoginRetry(generation, retryAt) {
		t.Fatal("current cooldown retry was not recognized")
	}

	beginGameConnectionAttempt(GameConnectionReconnecting, true)
	if isCurrentGameLoginRetry(generation, retryAt) {
		t.Fatal("superseded cooldown retry remained current")
	}
}

func TestPendingSocketCooldownDoesNotDemoteAuthenticatedSocket(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	trackGameRequestID(testBrowserSession, "authenticated")
	markGameSocketOpen(testBrowserSession, "authenticated")
	markGameLoginSucceeded(testBrowserSession, "authenticated")
	trackGameRequestID(testBrowserSession, "pending")
	markGameSocketOpen(testBrowserSession, "pending")

	if _, _, ok := markGameLoginCooldown(testBrowserSession, "pending", 30); ok {
		t.Fatal("pending socket cooldown replaced the authenticated session")
	}
	status := GetGameConnectionStatus()
	if status.State != GameConnectionConnected || !status.LoggedIn || !status.SocketConnected {
		t.Fatalf("authenticated connection was demoted: %#v", status)
	}
}

func TestOldBrowserSessionEventsAreIgnored(t *testing.T) {
	resetGameConnectionStatusForTest()
	beginGameConnectionAttempt(GameConnectionStarting, true)
	if trackGameRequestID(testBrowserSession-1, "old-session") {
		t.Fatal("old browser session socket was tracked")
	}
	if status := GetGameConnectionStatus(); status.State != GameConnectionStarting || status.SocketConnected {
		t.Fatalf("old browser event changed status: %#v", status)
	}
}
