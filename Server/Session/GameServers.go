package Session

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GameServer is one selectable Goodgame Empire world: the code players see
// (US1, GB1, INT1 …), the SmartFox zone the login must name, and the host the
// secure WebSocket connects to.
//
// The two are not derivable from each other in general: several worlds share
// one multi-zone host (`ep-live-mz-int1-sk1-gb1-game` serves INT1, SK1 and
// GB1, each with its own zone), and INT1's zone is the bare `EmpireEx`. Every
// host accepts `wss://<host>:443`.
type GameServer struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Zone   string `json:"zone"`
	ZoneID int    `json:"zoneId"`
	Host   string `json:"host"`
	// URL is the secure WebSocket endpoint, wss://<host>:443.
	URL           string `json:"url"`
	International bool   `json:"international"`
	// Instance is the numeric instance id in the official network config.
	Instance int `json:"instance"`
}

// GameServerCatalog is the list of worlds plus where it came from.
type GameServerCatalog struct {
	// Version is the official network config's versionNo.
	Version string `json:"version"`
	// Source is the document the catalog was read from.
	Source    string       `json:"source"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Servers   []GameServer `json:"servers"`
}

// GameServerNetworkConfigURL is the official client's server directory. The
// embedded catalog below is a snapshot of it; RefreshGameServerCatalog re-reads
// it so a newly opened world becomes selectable without a release.
const GameServerNetworkConfigURL = "https://empire-html5.goodgamestudios.com/config/network/1.xml"

// embeddedGameServerCatalogVersion is the versionNo of the snapshot below.
const embeddedGameServerCatalogVersion = "177"

var gameServerLabels = map[string]string{
	"INT1":   "International 1",
	"DE1":    "Germany",
	"FR1":    "France",
	"CZ1":    "Czechia",
	"PL1":    "Poland",
	"PT1":    "Portuguese",
	"INT2":   "International 2",
	"ES1":    "Spain 1",
	"IT1":    "Italy",
	"TR1":    "Türkiye",
	"NL1":    "Netherlands",
	"HU1":    "Hungary 1",
	"SKN1":   "Scandinavia",
	"RU1":    "Russia",
	"RO1":    "Romania",
	"BG1":    "Bulgaria",
	"HU2":    "Hungary 2",
	"SK1":    "Slovakia",
	"GB1":    "United Kingdom",
	"BR1":    "Brazil",
	"US1":    "United States",
	"AU1":    "Australia",
	"KR1":    "Korea",
	"JP1":    "Japan",
	"HIS1":   "Hispanic America",
	"IN1":    "India",
	"CN1":    "China",
	"GR1":    "Greece",
	"LT1":    "Lithuania",
	"SA1":    "Saudi Arabia",
	"AE1":    "United Arab Emirates",
	"EG1":    "Egypt",
	"ARAB1":  "Arab world",
	"ASIA1":  "Asia",
	"HANT1":  "Traditional Chinese",
	"ES2":    "Spain 2",
	"INT3":   "International 3",
	"WORLD1": "World 1",
	"WORLD2": "World 2",
}

// embeddedGameServers is the snapshot of the official network config
// (versionNo 177). Regenerate from GameServerNetworkConfigURL when GGE opens
// or retires a world; RefreshGameServerCatalog does the same at runtime.
var embeddedGameServers = []GameServer{
	{Code: "INT1", Label: "International 1", Zone: "EmpireEx", ZoneID: 121, Host: "ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com", International: true, Instance: 1},
	{Code: "DE1", Label: "Germany", Zone: "EmpireEx_2", ZoneID: 163, Host: "ep-live-de1-game.goodgamestudios.com", International: false, Instance: 2},
	{Code: "FR1", Label: "France", Zone: "EmpireEx_3", ZoneID: 164, Host: "ep-live-fr1-game.goodgamestudios.com", International: false, Instance: 3},
	{Code: "CZ1", Label: "Czechia", Zone: "EmpireEx_4", ZoneID: 165, Host: "ep-live-mz-cz1-es2-game.goodgamestudios.com", International: false, Instance: 4},
	{Code: "PL1", Label: "Poland", Zone: "EmpireEx_5", ZoneID: 166, Host: "ep-live-pl1-game.goodgamestudios.com", International: false, Instance: 5},
	{Code: "PT1", Label: "Portuguese", Zone: "EmpireEx_6", ZoneID: 185, Host: "ep-live-pt1-game.goodgamestudios.com", International: false, Instance: 6},
	{Code: "INT2", Label: "International 2", Zone: "EmpireEx_7", ZoneID: 186, Host: "ep-live-mz-int2-es1-it1-game.goodgamestudios.com", International: true, Instance: 7},
	{Code: "ES1", Label: "Spain 1", Zone: "EmpireEx_8", ZoneID: 188, Host: "ep-live-mz-int2-es1-it1-game.goodgamestudios.com", International: true, Instance: 8},
	{Code: "IT1", Label: "Italy", Zone: "EmpireEx_9", ZoneID: 189, Host: "ep-live-mz-int2-es1-it1-game.goodgamestudios.com", International: false, Instance: 9},
	{Code: "TR1", Label: "Türkiye", Zone: "EmpireEx_10", ZoneID: 190, Host: "ep-live-mz-tr1-nl1-bg1-game.goodgamestudios.com", International: false, Instance: 10},
	{Code: "NL1", Label: "Netherlands", Zone: "EmpireEx_11", ZoneID: 191, Host: "ep-live-mz-tr1-nl1-bg1-game.goodgamestudios.com", International: false, Instance: 11},
	{Code: "HU1", Label: "Hungary 1", Zone: "EmpireEx_12", ZoneID: 192, Host: "ep-live-mz-hu1-skn1-gr1-lt1-game.goodgamestudios.com", International: false, Instance: 12},
	{Code: "SKN1", Label: "Scandinavia", Zone: "EmpireEx_13", ZoneID: 193, Host: "ep-live-mz-hu1-skn1-gr1-lt1-game.goodgamestudios.com", International: false, Instance: 13},
	{Code: "RU1", Label: "Russia", Zone: "EmpireEx_14", ZoneID: 195, Host: "ep-live-ru1-game.goodgamestudios.com", International: false, Instance: 14},
	{Code: "RO1", Label: "Romania", Zone: "EmpireEx_15", ZoneID: 197, Host: "ep-live-ro1-game.goodgamestudios.com", International: true, Instance: 15},
	{Code: "BG1", Label: "Bulgaria", Zone: "EmpireEx_16", ZoneID: 198, Host: "ep-live-mz-tr1-nl1-bg1-game.goodgamestudios.com", International: true, Instance: 16},
	{Code: "HU2", Label: "Hungary 2", Zone: "EmpireEx_17", ZoneID: 199, Host: "ep-live-hu2-game.goodgamestudios.com", International: false, Instance: 17},
	{Code: "SK1", Label: "Slovakia", Zone: "EmpireEx_18", ZoneID: 200, Host: "ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com", International: false, Instance: 18},
	{Code: "GB1", Label: "United Kingdom", Zone: "EmpireEx_19", ZoneID: 201, Host: "ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com", International: false, Instance: 19},
	{Code: "BR1", Label: "Brazil", Zone: "EmpireEx_20", ZoneID: 202, Host: "ep-live-br1-game.goodgamestudios.com", International: false, Instance: 20},
	{Code: "US1", Label: "United States", Zone: "EmpireEx_21", ZoneID: 203, Host: "ep-live-us1-game.goodgamestudios.com", International: false, Instance: 21},
	{Code: "AU1", Label: "Australia", Zone: "EmpireEx_22", ZoneID: 208, Host: "ep-live-au1-game.goodgamestudios.com", International: false, Instance: 22},
	{Code: "KR1", Label: "Korea", Zone: "EmpireEx_23", ZoneID: 209, Host: "ep-live-mz-kr1-jp1-in1-cn1-game.goodgamestudios.com", International: false, Instance: 23},
	{Code: "JP1", Label: "Japan", Zone: "EmpireEx_24", ZoneID: 210, Host: "ep-live-mz-kr1-jp1-in1-cn1-game.goodgamestudios.com", International: false, Instance: 24},
	{Code: "HIS1", Label: "Hispanic America", Zone: "EmpireEx_25", ZoneID: 212, Host: "ep-live-his1-game.goodgamestudios.com", International: false, Instance: 25},
	{Code: "IN1", Label: "India", Zone: "EmpireEx_26", ZoneID: 213, Host: "ep-live-mz-kr1-jp1-in1-cn1-game.goodgamestudios.com", International: false, Instance: 26},
	{Code: "CN1", Label: "China", Zone: "EmpireEx_27", ZoneID: 216, Host: "ep-live-mz-kr1-jp1-in1-cn1-game.goodgamestudios.com", International: false, Instance: 27},
	{Code: "GR1", Label: "Greece", Zone: "EmpireEx_28", ZoneID: 255, Host: "ep-live-mz-hu1-skn1-gr1-lt1-game.goodgamestudios.com", International: false, Instance: 28},
	{Code: "LT1", Label: "Lithuania", Zone: "EmpireEx_29", ZoneID: 256, Host: "ep-live-mz-hu1-skn1-gr1-lt1-game.goodgamestudios.com", International: false, Instance: 29},
	{Code: "SA1", Label: "Saudi Arabia", Zone: "EmpireEx_32", ZoneID: 265, Host: "ep-live-mz-sa1-ae1-eg1-arab1-game.goodgamestudios.com", International: false, Instance: 32},
	{Code: "AE1", Label: "United Arab Emirates", Zone: "EmpireEx_33", ZoneID: 266, Host: "ep-live-mz-sa1-ae1-eg1-arab1-game.goodgamestudios.com", International: false, Instance: 33},
	{Code: "EG1", Label: "Egypt", Zone: "EmpireEx_34", ZoneID: 267, Host: "ep-live-mz-sa1-ae1-eg1-arab1-game.goodgamestudios.com", International: false, Instance: 34},
	{Code: "ARAB1", Label: "Arab world", Zone: "EmpireEx_35", ZoneID: 268, Host: "ep-live-mz-sa1-ae1-eg1-arab1-game.goodgamestudios.com", International: false, Instance: 35},
	{Code: "ASIA1", Label: "Asia", Zone: "EmpireEx_36", ZoneID: 459, Host: "ep-live-mz-asia1-hant1-game.goodgamestudios.com", International: false, Instance: 36},
	{Code: "HANT1", Label: "Traditional Chinese", Zone: "EmpireEx_37", ZoneID: 462, Host: "ep-live-mz-asia1-hant1-game.goodgamestudios.com", International: false, Instance: 37},
	{Code: "ES2", Label: "Spain 2", Zone: "EmpireEx_38", ZoneID: 704, Host: "ep-live-mz-cz1-es2-game.goodgamestudios.com", International: true, Instance: 38},
	{Code: "INT3", Label: "International 3", Zone: "EmpireEx_43", ZoneID: 831, Host: "ep-live-int3-game.goodgamestudios.com", International: true, Instance: 43},
	{Code: "WORLD1", Label: "World 1", Zone: "EmpireEx_46", ZoneID: 879, Host: "ep-live-world1-game.goodgamestudios.com", International: true, Instance: 46},
	{Code: "WORLD2", Label: "World 2", Zone: "EmpireEx_49", ZoneID: 926, Host: "ep-live-world2-game.goodgamestudios.com", International: false, Instance: 49},
}

var (
	gameServerCatalogMu sync.RWMutex
	gameServerCatalog   = newGameServerCatalog(embeddedGameServerCatalogVersion, "embedded", time.Time{}, embeddedGameServers)
	// gameServerZonePattern accepts every zone the official directory lists:
	// EmpireEx_<n> plus INT1's bare EmpireEx.
	gameServerZonePattern = gameNamespacePattern
)

func newGameServerCatalog(version, source string, updatedAt time.Time, servers []GameServer) GameServerCatalog {
	catalog := GameServerCatalog{Version: version, Source: source, UpdatedAt: updatedAt, Servers: make([]GameServer, 0, len(servers))}
	for _, server := range servers {
		server.Code = strings.ToUpper(strings.TrimSpace(server.Code))
		server.Host = strings.ToLower(strings.TrimSpace(server.Host))
		if server.URL == "" {
			server.URL = "wss://" + server.Host + ":443"
		}
		if server.Label == "" {
			server.Label = gameServerLabels[server.Code]
		}
		catalog.Servers = append(catalog.Servers, server)
	}
	sort.SliceStable(catalog.Servers, func(left, right int) bool {
		return catalog.Servers[left].Instance < catalog.Servers[right].Instance
	})
	return catalog
}

// GameServers returns a copy of the current catalog.
func GameServers() GameServerCatalog {
	gameServerCatalogMu.RLock()
	defer gameServerCatalogMu.RUnlock()
	copied := gameServerCatalog
	copied.Servers = append([]GameServer(nil), gameServerCatalog.Servers...)
	return copied
}

// LookupGameServer resolves a world code such as "us1" or "GB1".
func LookupGameServer(code string) (GameServer, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return GameServer{}, false
	}
	gameServerCatalogMu.RLock()
	defer gameServerCatalogMu.RUnlock()
	for _, server := range gameServerCatalog.Servers {
		if server.Code == code {
			return server, true
		}
	}
	return GameServer{}, false
}

// gameServersForHost lists the worlds served by one host. Multi-zone hosts
// return several entries; the URL alone cannot tell them apart.
func gameServersForHost(host string) []GameServer {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil
	}
	gameServerCatalogMu.RLock()
	defer gameServerCatalogMu.RUnlock()
	var matches []GameServer
	for _, server := range gameServerCatalog.Servers {
		if server.Host == host {
			matches = append(matches, server)
		}
	}
	return matches
}

// gameServerZoneForURL returns the zone for a server URL when the host serves
// exactly one world, or when preferredCode names one of the worlds it serves.
func gameServerZoneForURL(serverURL string, preferredCode string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return "", false
	}
	servers := gameServersForHost(parsed.Hostname())
	if len(servers) == 0 {
		return "", false
	}
	preferredCode = strings.ToUpper(strings.TrimSpace(preferredCode))
	for _, server := range servers {
		if server.Code == preferredCode {
			return server.Zone, true
		}
	}
	if len(servers) == 1 {
		return servers[0].Zone, true
	}
	return "", false
}

// SetGameServerCatalog replaces the catalog (tests, or a refreshed directory).
func SetGameServerCatalog(catalog GameServerCatalog) error {
	if len(catalog.Servers) == 0 {
		return fmt.Errorf("game server catalog is empty")
	}
	seen := make(map[string]struct{}, len(catalog.Servers))
	for _, server := range catalog.Servers {
		code := strings.ToUpper(strings.TrimSpace(server.Code))
		if !gameServerSelectionPattern.MatchString(code) {
			return fmt.Errorf("game server catalog has an invalid code %q", server.Code)
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("game server catalog lists %s twice", code)
		}
		seen[code] = struct{}{}
		if !gameServerHostnamePattern.MatchString(strings.ToLower(strings.TrimSpace(server.Host))) {
			return fmt.Errorf("game server %s has an unofficial host %q", code, server.Host)
		}
		if !gameServerZonePattern.MatchString(strings.TrimSpace(server.Zone)) {
			return fmt.Errorf("game server %s has an invalid zone %q", code, server.Zone)
		}
	}
	gameServerCatalogMu.Lock()
	defer gameServerCatalogMu.Unlock()
	gameServerCatalog = newGameServerCatalog(catalog.Version, catalog.Source, catalog.UpdatedAt, catalog.Servers)
	return nil
}

// ResetGameServerCatalog restores the embedded snapshot.
func ResetGameServerCatalog() {
	gameServerCatalogMu.Lock()
	defer gameServerCatalogMu.Unlock()
	gameServerCatalog = newGameServerCatalog(embeddedGameServerCatalogVersion, "embedded", time.Time{}, embeddedGameServers)
}

type networkConfigDocument struct {
	VersionNo string `xml:"versionNo,attr"`
	Instances []struct {
		Value           string `xml:"value,attr"`
		Server          string `xml:"server"`
		Zone            string `xml:"zone"`
		ZoneID          string `xml:"zoneId"`
		InstanceName    string `xml:"instanceName"`
		InstanceLocaID  string `xml:"instanceLocaId"`
		IsInternational string `xml:"isInternational"`
	} `xml:"instances>instance"`
}

// ParseGameServerNetworkConfig turns the official network config into a
// catalog. World codes follow the directory's own naming — the locale suffix
// plus the instance number (generic_country_US + 1 → US1, international + 3 →
// INT3) — which is also the token GGE embeds in each host name.
func ParseGameServerNetworkConfig(document []byte, source string, now time.Time) (GameServerCatalog, error) {
	var parsed networkConfigDocument
	if err := xml.Unmarshal(document, &parsed); err != nil {
		return GameServerCatalog{}, fmt.Errorf("decode network config: %w", err)
	}
	if len(parsed.Instances) == 0 {
		return GameServerCatalog{}, fmt.Errorf("network config lists no instances")
	}
	servers := make([]GameServer, 0, len(parsed.Instances))
	for _, instance := range parsed.Instances {
		host := strings.ToLower(strings.TrimSpace(instance.Server))
		zone := strings.TrimSpace(instance.Zone)
		if !gameServerHostnamePattern.MatchString(host) || !gameServerZonePattern.MatchString(zone) {
			continue
		}
		locale := strings.TrimSpace(instance.InstanceLocaID)
		locale = strings.TrimPrefix(locale, "generic_country_")
		locale = strings.TrimPrefix(locale, "generic_language_")
		var prefix string
		switch strings.ToLower(locale) {
		case "international":
			prefix = "INT"
		case "world":
			prefix = "WORLD"
		default:
			prefix = strings.ToUpper(locale)
		}
		instanceName := strings.TrimSpace(instance.InstanceName)
		if instanceName == "" {
			instanceName = "1"
		}
		code := prefix + instanceName
		if !gameServerSelectionPattern.MatchString(code) || !strings.Contains(host, strings.ToLower(code)) {
			// The host name always carries the world token; a row that breaks
			// that convention would produce a code we cannot vouch for.
			continue
		}
		zoneID, _ := strconv.Atoi(strings.TrimSpace(instance.ZoneID))
		instanceValue, _ := strconv.Atoi(strings.TrimSpace(instance.Value))
		servers = append(servers, GameServer{
			Code: code, Label: gameServerLabels[code], Zone: zone, ZoneID: zoneID, Host: host,
			URL: "wss://" + host + ":443", International: strings.TrimSpace(instance.IsInternational) == "1",
			Instance: instanceValue,
		})
	}
	if len(servers) == 0 {
		return GameServerCatalog{}, fmt.Errorf("network config lists no usable instances")
	}
	catalog := newGameServerCatalog(strings.TrimSpace(parsed.VersionNo), source, now.UTC(), servers)
	for index := range catalog.Servers {
		if catalog.Servers[index].Label == "" {
			catalog.Servers[index].Label = catalog.Servers[index].Code
		}
	}
	return catalog, nil
}

// RefreshGameServerCatalog re-reads the official directory and installs it.
// Failures leave the current catalog in place; the caller decides whether to
// log them.
func RefreshGameServerCatalog(ctx context.Context, client *http.Client) (GameServerCatalog, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, GameServerNetworkConfigURL, nil)
	if err != nil {
		return GameServerCatalog{}, err
	}
	request.Header.Set("Accept", "application/xml, text/xml")
	response, err := client.Do(request)
	if err != nil {
		return GameServerCatalog{}, fmt.Errorf("fetch game server directory: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return GameServerCatalog{}, fmt.Errorf("fetch game server directory: HTTP %d", response.StatusCode)
	}
	document, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return GameServerCatalog{}, fmt.Errorf("read game server directory: %w", err)
	}
	catalog, err := ParseGameServerNetworkConfig(document, GameServerNetworkConfigURL, time.Now())
	if err != nil {
		return GameServerCatalog{}, err
	}
	if err := SetGameServerCatalog(catalog); err != nil {
		return GameServerCatalog{}, err
	}
	return GameServers(), nil
}
