package AllianceTargets

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/WorldIntel"
)

type IntelligenceProvider interface {
	Rankings(context.Context, string, string, string, int) (WorldIntel.RankingResponse, error)
	Alliance(context.Context, string, int64, int) (WorldIntel.AllianceProfile, error)
}

type Service struct {
	provider IntelligenceProvider
	history  *History.Store

	mu            sync.Mutex
	alliancePages map[string]cachedAlliances
	targetDetails map[string]cachedProfile
	spyReports    []Reports.SpyReport
	spyReportsAt  time.Time
}

type cachedAlliances struct {
	fetchedAt time.Time
	rows      []AllianceOption
}

type cachedProfile struct {
	fetchedAt time.Time
	profile   WorldIntel.AllianceProfile
}

func NewService(provider IntelligenceProvider, histories ...*History.Store) *Service {
	var history *History.Store
	if len(histories) > 0 {
		history = histories[0]
	}
	return &Service{
		provider: provider, history: history,
		alliancePages: map[string]cachedAlliances{}, targetDetails: map[string]cachedProfile{},
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
	server := resolveWorldID(gameState, serverOverride)
	if server == "" {
		return View{}, fmt.Errorf("could not identify the active GGE world")
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
		return View{}, fmt.Errorf("selected alliance is not in the current World Intelligence ranking")
	}
	selected := alliances[selectedIndex]
	profile, err := service.loadProfile(ctx, server, selected.AllianceID, forceRefresh)
	if err != nil {
		return View{}, err
	}
	if profile.Current.Name != "" {
		selected.Name = profile.Current.Name
	}
	view.SelectedAlliance = &SelectedAllianceView{
		ExternalID: selected.ExternalID, AllianceID: selected.AllianceID, Name: selected.Name,
	}
	var targets []Target
	if live, found := gameState.Alliances[State.AllianceID(selected.AllianceID)]; found && live.ID > 0 {
		targets = buildLiveTargets(gameState, live, profile.Members)
	} else {
		targets = buildCloudTargets(gameState, profile)
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

func (service *Service) loadTopAlliances(ctx context.Context, worldID string, forceRefresh bool) ([]AllianceOption, error) {
	if service == nil || service.provider == nil {
		return nil, fmt.Errorf("World Intelligence is unavailable")
	}
	service.mu.Lock()
	if cached := service.alliancePages[worldID]; !forceRefresh && len(cached.rows) > 0 && time.Since(cached.fetchedAt) < 5*time.Minute {
		rows := append([]AllianceOption(nil), cached.rows...)
		service.mu.Unlock()
		return rows, nil
	}
	service.mu.Unlock()
	ranking, err := service.provider.Rankings(ctx, worldID, "alliances", "might", 50)
	if err != nil {
		return nil, fmt.Errorf("load World Intelligence alliance rankings: %w", err)
	}
	rows := make([]AllianceOption, 0, len(ranking.Entries))
	for _, entry := range ranking.Entries {
		if entry.ID <= 0 || strings.TrimSpace(entry.Name) == "" {
			continue
		}
		rows = append(rows, AllianceOption{
			ExternalID: strconv.FormatInt(entry.ID, 10), AllianceID: entry.ID,
			Name: entry.Name, Rank: entry.Rank, PlayerCount: entry.MemberCount,
		})
	}
	service.mu.Lock()
	service.alliancePages[worldID] = cachedAlliances{fetchedAt: time.Now(), rows: append([]AllianceOption(nil), rows...)}
	service.mu.Unlock()
	return rows, nil
}

func (service *Service) loadProfile(
	ctx context.Context,
	worldID string,
	allianceID int64,
	forceRefresh bool,
) (WorldIntel.AllianceProfile, error) {
	cacheKey := worldID + ":" + strconv.FormatInt(allianceID, 10)
	service.mu.Lock()
	if cached, found := service.targetDetails[cacheKey]; found && !forceRefresh && time.Since(cached.fetchedAt) < 2*time.Minute {
		service.mu.Unlock()
		return cached.profile, nil
	}
	service.mu.Unlock()
	profile, err := service.provider.Alliance(ctx, worldID, allianceID, 365)
	if err != nil {
		return WorldIntel.AllianceProfile{}, fmt.Errorf("load World Intelligence alliance profile: %w", err)
	}
	service.mu.Lock()
	service.targetDetails[cacheKey] = cachedProfile{fetchedAt: time.Now(), profile: profile}
	service.mu.Unlock()
	return profile, nil
}

func resolveWorldID(gameState State.GameState, override string) string {
	if worldID := WorldIntel.NormalizeWorldID(override); worldID != "" {
		return worldID
	}
	if worldID := WorldIntel.NormalizeWorldID(gameState.Account.WorldID); worldID != "" {
		return worldID
	}
	return WorldIntel.NormalizeWorldID(gameState.Session.ServerURL)
}
