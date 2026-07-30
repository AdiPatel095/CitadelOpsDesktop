package AllianceTargets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
)

const defaultTrackerAPI = "https://api.gge-tracker.com/api/v1"

type Service struct {
	client  *http.Client
	baseURL string
	history *History.Store

	mu               sync.Mutex
	servers          []string
	serversFetchedAt time.Time
	alliancePages    map[string]cachedAlliances
	targetDetails    map[string]cachedTargets
	spyReports       []Reports.SpyReport
	spyReportsAt     time.Time
}

type cachedAlliances struct {
	fetchedAt time.Time
	rows      []AllianceOption
}

type cachedTargets struct {
	fetchedAt   time.Time
	detail      trackerAllianceDetail
	cartography []trackerCartographyPlayer
}

type trackerInt int64

func (value *trackerInt) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	if text == "" || text == "null" {
		*value = 0
		return nil
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*value = trackerInt(parsed)
	return nil
}

type trackerAlliancePage struct {
	Alliances []struct {
		AllianceID  string     `json:"alliance_id"`
		Name        string     `json:"alliance_name"`
		PlayerCount trackerInt `json:"player_count"`
	} `json:"alliances"`
}

type trackerAllianceDetail struct {
	Name    string `json:"alliance_name"`
	Players []struct {
		PlayerID    string     `json:"player_id"`
		Name        string     `json:"player_name"`
		Level       trackerInt `json:"level"`
		LegendLevel trackerInt `json:"legendary_level"`
		Might       trackerInt `json:"might_current"`
		BirdUntil   string     `json:"peace_disabled_at"`
		UpdatedAt   string     `json:"updated_at"`
	} `json:"players"`
}

type trackerCartographyPlayer struct {
	Name    string  `json:"name"`
	Castles [][]int `json:"castles"`
}

func NewService(client *http.Client, histories ...*History.Store) *Service {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	var history *History.Store
	if len(histories) > 0 {
		history = histories[0]
	}
	return &Service{
		client: client, baseURL: defaultTrackerAPI, history: history,
		alliancePages: map[string]cachedAlliances{}, targetDetails: map[string]cachedTargets{},
	}
}

func (service *Service) View(
	ctx context.Context,
	gameState State.GameState,
	gameData *GameData.Store,
	serverOverride string,
	selectedExternalID string,
	forceRefresh bool,
	query Query,
) (View, error) {
	server, err := service.resolveServer(ctx, gameState.Session.ServerURL, serverOverride)
	if err != nil {
		return View{}, err
	}
	alliances, err := service.loadTopAlliances(ctx, server, forceRefresh)
	if err != nil {
		return View{}, err
	}
	availability := spyAvailability(gameState, gameData)
	responseAlliances := []AllianceOptionView{}
	if query.IncludeAlliances {
		responseAlliances = allianceOptionViews(alliances)
	}
	view := View{
		Server: server, Alliances: responseAlliances, Targets: []TargetView{},
		Page: 1, PageSize: TargetPageSize, PageCount: 1,
		CanInspect: gameState.Session.LoggedIn && gameState.Session.SocketReady,
		Spies:      spyAction(gameState, availability),
	}
	if selectedExternalID == "" {
		for _, alliance := range alliances {
			if alliance.AllianceID > 0 && State.AllianceID(alliance.AllianceID) != gameState.Player.AllianceID {
				selectedExternalID = alliance.ExternalID
				break
			}
		}
	}
	if selectedExternalID == "" {
		return view, nil
	}
	selectedIndex := -1
	for index := range alliances {
		if alliances[index].ExternalID == selectedExternalID {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 {
		return View{}, fmt.Errorf("selected alliance is not in the current top 50")
	}
	selected := alliances[selectedIndex]
	detail, cartography, err := service.loadTargets(ctx, server, selected.ExternalID, forceRefresh)
	if err != nil {
		return View{}, err
	}
	if detail.Name != "" {
		selected.Name = detail.Name
	}
	view.SelectedAlliance = &SelectedAllianceView{
		ExternalID: selected.ExternalID, AllianceID: selected.AllianceID, Name: selected.Name,
	}
	var targets []Target
	if live, found := gameState.Alliances[State.AllianceID(selected.AllianceID)]; found && live.ID > 0 {
		targets = buildLiveTargets(gameState, live, detail)
	} else {
		targets = buildTrackerTargets(gameState, detail, cartography)
	}
	reports, _ := service.recentSpyReports()
	enrichTargetIntelligence(gameState, reports, targets)
	view.Targets, view.TotalTargets, view.Page, view.PageCount = queryTargets(targets, query)
	return view, nil
}

func (service *Service) recentSpyReports() ([]Reports.SpyReport, error) {
	service.mu.Lock()
	if service.history == nil {
		service.mu.Unlock()
		return []Reports.SpyReport{}, nil
	}
	if time.Since(service.spyReportsAt) < 2*time.Second {
		reports := append([]Reports.SpyReport(nil), service.spyReports...)
		service.mu.Unlock()
		return reports, nil
	}
	history := service.history
	service.mu.Unlock()

	rows, err := history.Read(History.CollectionSpyReports, time.Time{}, 10_000)
	if err != nil {
		return nil, err
	}
	reports := make([]Reports.SpyReport, 0, len(rows))
	for _, row := range rows {
		var report Reports.SpyReport
		if json.Unmarshal(row, &report) == nil && report.CapturedAtUnixMillis > 0 {
			reports = append(reports, report)
		}
	}
	service.mu.Lock()
	service.spyReports = append([]Reports.SpyReport(nil), reports...)
	service.spyReportsAt = time.Now()
	service.mu.Unlock()
	return reports, nil
}

func (service *Service) resolveServer(ctx context.Context, socketURL string, override string) (string, error) {
	servers, err := service.supportedServers(ctx)
	if err != nil {
		return "", err
	}
	if override = strings.TrimSpace(override); override != "" {
		for _, server := range servers {
			if strings.EqualFold(server, override) {
				return server, nil
			}
		}
		return "", fmt.Errorf("GGE Tracker does not support server %q", override)
	}
	candidates := serverCandidates(socketURL, servers)
	if len(candidates) == 0 {
		return "", fmt.Errorf("could not identify the active GGE server")
	}
	return candidates[0], nil
}

func (service *Service) supportedServers(ctx context.Context) ([]string, error) {
	service.mu.Lock()
	if len(service.servers) > 0 && time.Since(service.serversFetchedAt) < 6*time.Hour {
		servers := append([]string(nil), service.servers...)
		service.mu.Unlock()
		return servers, nil
	}
	service.mu.Unlock()
	var servers []string
	if err := service.getJSON(ctx, "", "/servers", &servers); err != nil {
		return nil, err
	}
	service.mu.Lock()
	service.servers = append([]string(nil), servers...)
	service.serversFetchedAt = time.Now()
	service.mu.Unlock()
	return servers, nil
}

func (service *Service) loadTopAlliances(ctx context.Context, server string, forceRefresh bool) ([]AllianceOption, error) {
	service.mu.Lock()
	if cached := service.alliancePages[server]; !forceRefresh && len(cached.rows) == 50 && time.Since(cached.fetchedAt) < 5*time.Minute {
		rows := append([]AllianceOption(nil), cached.rows...)
		service.mu.Unlock()
		return rows, nil
	}
	service.mu.Unlock()

	type pageResult struct {
		page int
		data trackerAlliancePage
		err  error
	}
	results := make(chan pageResult, 4)
	for page := 1; page <= 4; page++ {
		go func(page int) {
			var data trackerAlliancePage
			path := fmt.Sprintf("/alliances?page=%d&orderBy=might_current&orderType=DESC", page)
			err := service.getJSON(ctx, server, path, &data)
			results <- pageResult{page: page, data: data, err: err}
		}(page)
	}
	pages := make([]trackerAlliancePage, 4)
	for range 4 {
		result := <-results
		if result.err != nil {
			return nil, result.err
		}
		pages[result.page-1] = result.data
	}
	rows := make([]AllianceOption, 0, 50)
	for _, page := range pages {
		for _, alliance := range page.Alliances {
			rows = append(rows, AllianceOption{
				ExternalID: alliance.AllianceID, AllianceID: trackerGameID(alliance.AllianceID),
				Name: alliance.Name, Rank: len(rows) + 1, PlayerCount: int(alliance.PlayerCount),
			})
			if len(rows) == 50 {
				break
			}
		}
		if len(rows) == 50 {
			break
		}
	}
	service.mu.Lock()
	service.alliancePages[server] = cachedAlliances{fetchedAt: time.Now(), rows: append([]AllianceOption(nil), rows...)}
	service.mu.Unlock()
	return rows, nil
}

func (service *Service) loadTargets(ctx context.Context, server string, externalID string, forceRefresh bool) (trackerAllianceDetail, []trackerCartographyPlayer, error) {
	cacheKey := server + ":" + externalID
	service.mu.Lock()
	if cached, found := service.targetDetails[cacheKey]; !forceRefresh && found && time.Since(cached.fetchedAt) < 2*time.Minute {
		service.mu.Unlock()
		return cached.detail, append([]trackerCartographyPlayer(nil), cached.cartography...), nil
	}
	service.mu.Unlock()
	var detail trackerAllianceDetail
	var cartography []trackerCartographyPlayer
	var detailErr, cartographyErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		detailErr = service.getJSON(ctx, server, "/alliances/id/"+url.PathEscape(externalID), &detail)
	}()
	go func() {
		defer wait.Done()
		cartographyErr = service.getJSON(ctx, server, "/cartography/id/"+url.PathEscape(externalID), &cartography)
	}()
	wait.Wait()
	if detailErr != nil {
		return detail, nil, detailErr
	}
	if cartographyErr != nil {
		return detail, nil, cartographyErr
	}
	service.mu.Lock()
	service.targetDetails[cacheKey] = cachedTargets{
		fetchedAt: time.Now(), detail: detail, cartography: append([]trackerCartographyPlayer(nil), cartography...),
	}
	service.mu.Unlock()
	return detail, cartography, nil
}

func (service *Service) getJSON(ctx context.Context, server string, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CitadelOpsDesktop/2.0")
	if server != "" {
		request.Header.Set("gge-server", server)
	}
	response, err := service.client.Do(request)
	if err != nil {
		return fmt.Errorf("GGE Tracker request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GGE Tracker returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("GGE Tracker response decode failed: %w", err)
	}
	return nil
}

func serverCandidates(socketURL string, supported []string) []string {
	parsed, err := url.Parse(socketURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(parsed.Hostname())
	cluster := strings.TrimPrefix(strings.Split(host, ".")[0], "ep-live-")
	cluster = strings.TrimSuffix(cluster, "-game")
	cluster = strings.TrimPrefix(cluster, "mz-")
	seen := map[string]struct{}{}
	candidates := []string{}
	for _, token := range strings.Split(cluster, "-") {
		for _, server := range supported {
			serverToken := strings.ToLower(server)
			if token != serverToken && token != serverToken+"1" {
				continue
			}
			if _, found := seen[server]; found {
				continue
			}
			seen[server] = struct{}{}
			candidates = append(candidates, server)
		}
	}
	sort.Strings(candidates)
	return candidates
}

func trackerGameID(externalID string) int64 {
	value, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value / 1000
}
