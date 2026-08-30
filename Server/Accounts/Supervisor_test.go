package Accounts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"github.com/gorilla/websocket"
)

func TestSupervisorCreatesIsolatedAccountsWithSharedGameData(t *testing.T) {
	supervisor := newTestSupervisor(t)
	alpha := addTestAccount(t, supervisor, "alpha")
	bravo := addTestAccount(t, supervisor, "bravo")

	if alpha.GameData != supervisor.GameData() || bravo.GameData != supervisor.GameData() || alpha.GameData != bravo.GameData {
		t.Fatal("accounts did not share the process-owned game-data manager")
	}
	if alpha.Updates == nil || alpha.Updates != bravo.Updates {
		t.Fatal("accounts did not share the process-owned application update manager")
	}
	if alpha.WorldIntelClient == nil || alpha.WorldIntelClient != bravo.WorldIntelClient {
		t.Fatal("accounts did not share the process-owned world intelligence client")
	}
	if alpha.PrivateMetrics != nil || bravo.PrivateMetrics != nil {
		t.Fatal("private metrics publishing enabled without explicit hosted configuration")
	}
	if alpha.Reports.CloudClient() == nil || alpha.Reports.CloudClient() != bravo.Reports.CloudClient() {
		t.Fatal("accounts did not share the process-owned report cloud client")
	}
	if alpha.Ingest.Registry() == nil || alpha.Ingest.Registry() != bravo.Ingest.Registry() {
		t.Fatal("accounts did not share the immutable protocol reducer registry")
	}
	if alpha.State == bravo.State || alpha.Session == bravo.Session || alpha.API == bravo.API {
		t.Fatal("account-private runtime components were shared")
	}
	if got := supervisor.AccountIDs(); len(got) != 2 || got[0] != "alpha" || got[1] != "bravo" {
		t.Fatalf("account ids = %v", got)
	}
}

func TestSupervisorSharesOnlySameWorldObjectiveMapFacts(t *testing.T) {
	supervisor := newTestSupervisor(t)
	supervisor.Start()
	alpha := addTestAccount(t, supervisor, "alpha")
	bravo := addTestAccount(t, supervisor, "bravo")
	charlie := addTestAccount(t, supervisor, "charlie")
	bindTestWorld(t, alpha.State, "world-one", 101)
	bindTestWorld(t, bravo.State, "WORLD-ONE", 202)
	bindTestWorld(t, charlie.State, "world-two", 303)
	unlockTestKingdom(t, alpha.State, 101, 4)
	unlockTestKingdom(t, bravo.State, 202, 4)
	unlockTestKingdom(t, charlie.State, 303, 4)

	shared := State.MapObservation{
		KingdomID: 0, X: 100, Y: 101, TypeID: 1, OwnerID: 500, ObjectID: 700,
		Name: "Objective castle", ObservedAt: time.Now().UTC(),
	}
	if _, err := alpha.State.ApplyComponents(State.Components(State.ComponentWorldMap), func(state *State.GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(shared), nil
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if observation, found := bravo.State.ReadOnlyView().LookupMapObservation(0, "100:101"); found && observation.OwnerID == 500 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("same-world account did not receive shared map fact")
		}
		time.Sleep(time.Millisecond)
	}
	if _, found := charlie.State.ReadOnlyView().LookupMapObservation(0, "100:101"); found {
		t.Fatal("objective map fact leaked to a different world")
	}

	private := State.MapObservation{
		KingdomID: 0, X: 200, Y: 201, TypeID: 2, OwnerID: 500,
		TowerVictoryCount: 845, ObservedAt: time.Now().UTC(),
	}
	if _, err := alpha.State.ApplyComponents(State.Components(State.ComponentWorldMap), func(state *State.GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(private), nil
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, found := bravo.State.ReadOnlyView().LookupMapObservation(0, "200:201"); found {
		t.Fatal("account-private tower progress leaked to a sibling account")
	}

	storm := State.MapObservation{
		KingdomID: 4, X: 612, Y: 667, TypeID: State.MapTypeStormFort,
		StormIsleID: 10, StormVictoryCount: 7, ObservedAt: time.Now().UTC(),
	}
	if _, err := alpha.State.ApplyComponents(State.Components(State.ComponentWorldMap), func(state *State.GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(storm), nil
	}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		if observation, found := bravo.State.ReadOnlyView().LookupStormTarget("612:667"); found && observation.StormIsleID == 10 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("same-world account did not receive shared Storm fact")
		}
		time.Sleep(time.Millisecond)
	}
	if _, found := charlie.State.ReadOnlyView().LookupStormTarget("612:667"); found {
		t.Fatal("shared Storm fact leaked to a different game world")
	}
}

func TestShardRouterRejectsCrossAccountHTTPAccess(t *testing.T) {
	supervisor := newTestSupervisor(t)
	alpha := addTestAccount(t, supervisor, "alpha")
	bravo := addTestAccount(t, supervisor, "bravo")
	setTestPlayer(t, alpha.State, 101, "Alpha")
	setTestPlayer(t, bravo.State, 202, "Bravo")

	server := httptest.NewServer(supervisor.Handler(testAuthenticator(), http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("frontend"))
	})))
	defer server.Close()

	state := getState(t, server.URL+"/accounts/alpha/api/v2/state", "alpha", http.StatusOK)
	if state.Player.ID != 101 || state.Player.Name != "Alpha" {
		t.Fatalf("alpha route returned another account: %+v", state.Player)
	}
	_ = getState(t, server.URL+"/accounts/bravo/api/v2/state", "alpha", http.StatusNotFound)
	_ = getState(t, server.URL+"/accounts/alpha/api/v2/state", "", http.StatusUnauthorized)
}

func TestTenantShardServesPersistedTablesAndSettingsWithoutGameSession(t *testing.T) {
	supervisor := newTestSupervisor(t)
	application := addTestAccount(t, supervisor, "alpha")
	session := application.Session.Status()
	if session.LoggedIn || session.SocketReady {
		t.Fatalf("test runtime unexpectedly has a game connection: %+v", session)
	}

	server := httptest.NewServer(supervisor.Handler(testAuthenticator(), nil))
	defer server.Close()
	request := func(method, path, body string) *http.Response {
		t.Helper()
		var reader *strings.Reader
		if body == "" {
			reader = strings.NewReader("")
		} else {
			reader = strings.NewReader(body)
		}
		httpRequest, err := http.NewRequest(method, server.URL+"/accounts/alpha"+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest.Header.Set("X-Test-Account", "alpha")
		response, err := http.DefaultClient.Do(httpRequest)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	for _, path := range []string{
		"/api/v2/state",
		"/api/v2/config",
		"/api/v2/history/player-tracker",
		"/api/v2/history/spy-reports",
		"/api/v2/history/battle-reports",
		"/api/v2/analytics/battle-reports",
		"/api/v2/telemetry/channels",
		"/api/v2/telemetry/attack-rates",
		"/api/v2/operations",
	} {
		response := request(http.MethodGet, path, "")
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("offline tenant table GET %s = %d, want %d", path, response.StatusCode, http.StatusOK)
		}
	}

	revision := application.Configuration.Snapshot().Revision
	body, err := json.Marshal(map[string]any{
		"value":            map[string]any{"minAttackDelay": 11},
		"expectedRevision": revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := request(http.MethodPut, "/api/v2/config/scheduler", string(body))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("offline tenant settings update = %d", response.StatusCode)
	}
	if value, ok := application.Configuration.Section("scheduler"); !ok || !strings.Contains(string(value), `"minAttackDelay":11`) {
		t.Fatalf("offline tenant settings were not persisted: %s", value)
	}
}

func TestShardWebSocketReceivesOnlyItsAccountEvents(t *testing.T) {
	supervisor := newTestSupervisor(t)
	alpha := addTestAccount(t, supervisor, "alpha")
	bravo := addTestAccount(t, supervisor, "bravo")
	setTestPlayer(t, alpha.State, 101, "Alpha")
	setTestPlayer(t, bravo.State, 202, "Bravo")

	server := httptest.NewServer(supervisor.Handler(testAuthenticator(), nil))
	defer server.Close()
	header := http.Header{"X-Test-Account": []string{"alpha"}}
	connection, response, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/accounts/alpha/api/v2/events", header,
	)
	if err != nil {
		if response != nil {
			t.Fatalf("dial account websocket: %v (status %d)", err, response.StatusCode)
		}
		t.Fatal(err)
	}
	defer connection.Close()

	var initial API.Envelope
	if err := connection.ReadJSON(&initial); err != nil {
		t.Fatal(err)
	}
	if initial.Type != "state.snapshot" {
		t.Fatalf("first websocket envelope = %q", initial.Type)
	}
	var snapshot State.GameState
	if err := json.Unmarshal(initial.Payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Player.ID != 101 {
		t.Fatalf("alpha websocket opened on player %d", snapshot.Player.ID)
	}
	// Configuration and operation snapshots complete the deterministic opening
	// sequence before live state events begin.
	for range 2 {
		var ignored API.Envelope
		if err := connection.ReadJSON(&ignored); err != nil {
			t.Fatal(err)
		}
	}

	applyDomain(t, bravo.State, "bravo-only")
	applyDomain(t, alpha.State, "alpha-only")
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	var changed API.Envelope
	if err := connection.ReadJSON(&changed); err != nil {
		t.Fatal(err)
	}
	if changed.Type != "state.changed" {
		t.Fatalf("live websocket envelope = %q", changed.Type)
	}
	var event State.Event
	if err := json.Unmarshal(changed.Payload, &event); err != nil {
		t.Fatal(err)
	}
	if len(event.Domains) != 1 || event.Domains[0] != "alpha-only" {
		t.Fatalf("alpha websocket leaked a sibling event: %+v", event)
	}
	if event.Patch == nil || event.Patch.Session == nil || event.Patch.Player != nil {
		t.Fatalf("alpha websocket did not receive a session-only patch: %+v", event.Patch)
	}
}

func TestParseAccountIDRejectsFilesystemAliases(t *testing.T) {
	for _, value := range []string{"", ".", "..", "a/b", "a%2fb", "account name"} {
		if _, err := ParseAccountID(value); err == nil {
			t.Errorf("ParseAccountID(%q) succeeded", value)
		}
	}
	if id, err := ParseAccountID("  Amos_Burton  "); err != nil || id != "amos_burton" {
		t.Fatalf("normalized id = %q, err %v", id, err)
	}
}

func TestRemoveAccountWaitsThenAllowsProfileReuse(t *testing.T) {
	supervisor := newTestSupervisor(t)
	first := addTestAccount(t, supervisor, "alpha")
	dataDir := first.DataDir

	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := supervisor.RemoveAccount(shutdown, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, exists := supervisor.Application("alpha"); exists {
		t.Fatal("removed account remained routable")
	}
	second := addTestAccount(t, supervisor, "alpha")
	if second.DataDir != dataDir {
		t.Fatalf("replacement data directory = %q, want %q", second.DataDir, dataDir)
	}
}

func TestClosedSupervisorRejectsNewAccounts(t *testing.T) {
	supervisor := newTestSupervisor(t)
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := supervisor.Close(shutdown); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.AddAccount(context.Background(), AccountConfig{
		ID: "late", Transport: Session.NewUnavailableTransport(),
	}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("late AddAccount error = %v", err)
	}
}

func TestSupervisorEnforcesProcessAccountLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: t.TempDir()})
	supervisor, err := New(ctx, Config{
		DataRoot: t.TempDir(), Offline: true, RuntimeContext: ctx,
		GameData: manager, MaxAccounts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = supervisor.Close(shutdown)
	})
	addTestAccount(t, supervisor, "alpha")
	if _, err := supervisor.AddAccount(context.Background(), AccountConfig{
		ID: "bravo", Transport: Session.NewUnavailableTransport(),
	}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("second account error = %v", err)
	}
}

func newTestSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: t.TempDir()})
	supervisor, err := New(ctx, Config{DataRoot: t.TempDir(), Offline: true, RuntimeContext: ctx, GameData: manager})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = supervisor.Close(shutdown)
		cancel()
	})
	return supervisor
}

func addTestAccount(t *testing.T, supervisor *Supervisor, id string) *App.Application {
	t.Helper()
	application, err := supervisor.AddAccount(context.Background(), AccountConfig{
		ID: id, Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func setTestPlayer(t *testing.T, store *State.Store, id State.PlayerID, name string) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Player.ID = id
		gameState.Player.Name = name
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func bindTestWorld(t *testing.T, store *State.Store, worldID string, playerID State.PlayerID) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentAccount, State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Account.WorldID = worldID
		state.Account.PlayerID = playerID
		state.Player.ID = playerID
		return []string{"account", "player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func unlockTestKingdom(t *testing.T, store *State.Store, castleID State.CastleID, kingdomID State.KingdomID) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentCastles), func(state *State.GameState) ([]string, bool, error) {
		state.SetCastle(castleID, State.CastleState{ID: castleID, KingdomID: kingdomID})
		return []string{"castles"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func applyDomain(t *testing.T, store *State.Store, domain string) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentSession), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.Generation++
		return []string{domain}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testAuthenticator() Authenticator {
	return AuthenticateFunc(func(request *http.Request) (AccountID, bool) {
		id, err := ParseAccountID(request.Header.Get("X-Test-Account"))
		return id, err == nil
	})
}

func getState(t *testing.T, target string, authenticated string, wantStatus int) State.GameState {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated != "" {
		request.Header.Set("X-Test-Account", authenticated)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", target, response.StatusCode, wantStatus)
	}
	var state State.GameState
	if wantStatus == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
	}
	return state
}
