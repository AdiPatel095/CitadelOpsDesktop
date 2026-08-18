package Session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEmbeddedGameServerCatalogMatchesOfficialDirectory(t *testing.T) {
	document, err := os.ReadFile("testdata/network-config-177.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := ParseGameServerNetworkConfig(document, "fixture", time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	embedded := GameServers()
	if len(parsed.Servers) != len(embedded.Servers) {
		t.Fatalf("embedded catalog has %d worlds, directory %d", len(embedded.Servers), len(parsed.Servers))
	}
	byCode := map[string]GameServer{}
	for _, server := range embedded.Servers {
		byCode[server.Code] = server
	}
	for _, server := range parsed.Servers {
		match, exists := byCode[server.Code]
		if !exists {
			t.Fatalf("embedded catalog is missing %s", server.Code)
		}
		if match.Zone != server.Zone || match.Host != server.Host || match.URL != server.URL {
			t.Fatalf("%s: embedded %+v != directory %+v", server.Code, match, server)
		}
	}
}

func TestLookupGameServerResolvesMultiZoneHosts(t *testing.T) {
	cases := map[string]struct{ zone, host string }{
		"US1":  {"EmpireEx_21", "ep-live-us1-game.goodgamestudios.com"},
		"GB1":  {"EmpireEx_19", "ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com"},
		"INT1": {"EmpireEx", "ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com"},
		"ES2":  {"EmpireEx_38", "ep-live-mz-cz1-es2-game.goodgamestudios.com"},
	}
	for code, expected := range cases {
		server, ok := LookupGameServer(code)
		if !ok || server.Zone != expected.zone || server.Host != expected.host {
			t.Fatalf("%s resolved to %+v (ok=%v)", code, server, ok)
		}
		if server.URL != "wss://"+expected.host+":443" {
			t.Fatalf("%s url = %s", code, server.URL)
		}
	}
	if _, ok := LookupGameServer("XX9"); ok {
		t.Fatal("unknown world resolved")
	}
	if lower, ok := LookupGameServer(" gb1 "); !ok || lower.Code != "GB1" {
		t.Fatalf("case-insensitive lookup = %+v (ok=%v)", lower, ok)
	}
}

func TestGameServerZoneForURL(t *testing.T) {
	if zone, ok := gameServerZoneForURL("wss://ep-live-us1-game.goodgamestudios.com:443", ""); !ok || zone != "EmpireEx_21" {
		t.Fatalf("single-world host zone = %q (ok=%v)", zone, ok)
	}
	if zone, ok := gameServerZoneForURL("wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443", "GB1"); !ok || zone != "EmpireEx_19" {
		t.Fatalf("multi-zone host with code = %q (ok=%v)", zone, ok)
	}
	if _, ok := gameServerZoneForURL("wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443", ""); ok {
		t.Fatal("multi-zone host without a code must not guess")
	}
	if zone, ok := gameServerZoneForURL("wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443", "INT1"); !ok || zone != "EmpireEx" {
		t.Fatalf("INT1 zone = %q (ok=%v)", zone, ok)
	}
}

func TestBackgroundGameServerURLUsesCatalog(t *testing.T) {
	url, code, err := backgroundGameServerURL("gb1")
	if err != nil || code != "GB1" || url != "wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443" {
		t.Fatalf("GB1 = %s %s %v", url, code, err)
	}
	// A world the catalog does not know falls back to the single-world
	// convention so a freshly opened world is still reachable.
	url, code, err = backgroundGameServerURL("zz9")
	if err != nil || code != "ZZ9" || url != "wss://ep-live-zz9-game.goodgamestudios.com:443" {
		t.Fatalf("ZZ9 = %s %s %v", url, code, err)
	}
	if _, _, err = backgroundGameServerURL("not a code!"); err == nil {
		t.Fatal("invalid code accepted")
	}
}

func TestRefreshGameServerCatalogInstallsDirectory(t *testing.T) {
	document, err := os.ReadFile("testdata/network-config-177.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(document)
	}))
	defer upstream.Close()
	t.Cleanup(ResetGameServerCatalog)
	catalog, err := ParseGameServerNetworkConfig(document, upstream.URL, time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := SetGameServerCatalog(catalog); err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed := GameServers(); installed.Source != upstream.URL || len(installed.Servers) != 39 {
		t.Fatalf("installed = %s with %d worlds", installed.Source, len(installed.Servers))
	}
	if err := SetGameServerCatalog(GameServerCatalog{}); err == nil {
		t.Fatal("empty catalog accepted")
	}
	if err := SetGameServerCatalog(GameServerCatalog{Servers: []GameServer{{Code: "US1", Host: "evil.example.com", Zone: "EmpireEx_21"}}}); err == nil {
		t.Fatal("unofficial host accepted")
	}
}

func TestConfigurePinsExplicitEndpointAndZone(t *testing.T) {
	store := NewBackgroundLoginStore(t.TempDir())
	status, err := store.Configure(BackgroundLoginInput{
		Username: "keeper", Password: "hunter2!!", Server: "NEW9",
		ServerURL: "wss://ep-live-mz-new9-other1-game.goodgamestudios.com:443", Zone: "EmpireEx_77",
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if status.Server != "NEW9" || status.ServerURL != "wss://ep-live-mz-new9-other1-game.goodgamestudios.com:443" {
		t.Fatalf("status = %+v", status)
	}
	credential, err := loadBackgroundLoginCredential(store.dataDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if credential.Zone != "EmpireEx_77" || credential.Server != "NEW9" {
		t.Fatalf("credential = %+v", credential)
	}
	// The pinned zone reaches the connection profile even though the catalog
	// knows nothing about this world.
	profile, err := resolveDirectGameProfile(DirectWebSocketConfig{DataDir: store.dataDir}, credential, gameConnectionProfile{}, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if profile.Namespace != "EmpireEx_77" || profile.ServerURL != credential.ServerURL {
		t.Fatalf("profile = %+v", profile)
	}
	if _, err := store.Configure(BackgroundLoginInput{Username: "keeper", Password: "pw", Server: "US1", ServerURL: "wss://evil.example.com"}); err == nil {
		t.Fatal("unofficial explicit URL accepted")
	}
	if _, err := store.Configure(BackgroundLoginInput{Username: "keeper", Password: "pw", Server: "US1", Zone: "NotAZone"}); err == nil {
		t.Fatal("invalid zone accepted")
	}
}

func TestConfigureResolvesMultiZoneWorldFromCatalog(t *testing.T) {
	store := NewBackgroundLoginStore(t.TempDir())
	if _, err := store.Configure(BackgroundLoginInput{Username: "keeper", Password: "pw", Server: "gb1"}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	credential, err := loadBackgroundLoginCredential(store.dataDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if credential.ServerURL != "wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443" {
		t.Fatalf("credential url = %s", credential.ServerURL)
	}
	profile, err := resolveDirectGameProfile(DirectWebSocketConfig{DataDir: store.dataDir}, credential, gameConnectionProfile{}, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if profile.Namespace != "EmpireEx_19" {
		t.Fatalf("GB1 namespace = %s", profile.Namespace)
	}
}
