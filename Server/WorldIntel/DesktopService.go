package WorldIntel

import (
	"context"
	"fmt"

	"CitadelDesktop/Server/State"
)

// DesktopService is intentionally a read-only facade over the shared World
// Intelligence backend. Collection and upload belong to standalone collector
// processes, never to the interactive desktop runtime.
type DesktopService struct {
	state  *State.Store
	client *CloudClient
}

func NewDesktopService(state *State.Store, client *CloudClient) *DesktopService {
	if client == nil {
		client = NewCloudClient(ClientConfig{})
	}
	return &DesktopService{state: state, client: client}
}

func (service *DesktopService) Status(_ context.Context) DesktopStatus {
	status := DesktopStatus{
		Enabled: true, CollectionMode: "shared-data-reader",
		PublicFieldsOnly: true, OfficialSourceOnly: true,
	}
	if service != nil && service.client != nil {
		status.Endpoint = service.client.Endpoint()
	}
	status.WorldID = service.CurrentWorldID()
	return status
}

func (service *DesktopService) CurrentWorldID() string {
	if service == nil || service.state == nil {
		return ""
	}
	snapshot := service.state.Snapshot()
	worldID := NormalizeWorldID(snapshot.Account.WorldID)
	if worldID == "" {
		worldID = NormalizeWorldID(snapshot.Session.ServerURL)
	}
	return worldID
}

func (service *DesktopService) Search(
	ctx context.Context,
	worldID string,
	query string,
	entityType string,
	limit int,
) (SearchResponse, error) {
	if err := service.queryReady(); err != nil {
		return SearchResponse{}, err
	}
	return service.client.Search(ctx, service.resolveWorld(worldID), query, entityType, limit)
}

func (service *DesktopService) Player(ctx context.Context, worldID string, playerID int64, limit int) (PlayerProfile, error) {
	if err := service.queryReady(); err != nil {
		return PlayerProfile{}, err
	}
	return service.client.Player(ctx, service.resolveWorld(worldID), playerID, limit)
}

func (service *DesktopService) Alliance(ctx context.Context, worldID string, allianceID int64, limit int) (AllianceProfile, error) {
	if err := service.queryReady(); err != nil {
		return AllianceProfile{}, err
	}
	return service.client.Alliance(ctx, service.resolveWorld(worldID), allianceID, limit)
}

func (service *DesktopService) EventRuns(
	ctx context.Context,
	worldID string,
	eventKey string,
	limit int,
) (EventRunListResponse, error) {
	if err := service.queryReady(); err != nil {
		return EventRunListResponse{}, err
	}
	return service.client.EventRuns(ctx, service.resolveWorld(worldID), eventKey, limit)
}

func (service *DesktopService) EventRunRankings(
	ctx context.Context,
	worldID string,
	occurrenceID string,
	listType int64,
	leagueID int64,
	limit int,
) (EventRunRankingResponse, error) {
	if err := service.queryReady(); err != nil {
		return EventRunRankingResponse{}, err
	}
	return service.client.EventRunRankings(
		ctx, service.resolveWorld(worldID), occurrenceID, listType, leagueID, limit,
	)
}

func (service *DesktopService) PlayerEventScores(
	ctx context.Context,
	worldID string,
	playerID int64,
	eventKey string,
	occurrenceID string,
	limit int,
) (PlayerEventScoreResponse, error) {
	if err := service.queryReady(); err != nil {
		return PlayerEventScoreResponse{}, err
	}
	return service.client.PlayerEventScores(
		ctx, service.resolveWorld(worldID), playerID, eventKey, occurrenceID, limit,
	)
}

func (service *DesktopService) Rankings(
	ctx context.Context,
	worldID string,
	entityType string,
	metric string,
	limit int,
) (RankingResponse, error) {
	if err := service.queryReady(); err != nil {
		return RankingResponse{}, err
	}
	return service.client.Rankings(ctx, service.resolveWorld(worldID), entityType, metric, limit)
}

func (service *DesktopService) RankingMetrics(
	ctx context.Context,
	worldID string,
	entityType string,
) (RankingMetricCatalogResponse, error) {
	if err := service.queryReady(); err != nil {
		return RankingMetricCatalogResponse{}, err
	}
	return service.client.RankingMetrics(ctx, service.resolveWorld(worldID), entityType)
}

func (service *DesktopService) Coverage(ctx context.Context, worldID string) (CoverageResponse, error) {
	if err := service.queryReady(); err != nil {
		return CoverageResponse{}, err
	}
	return service.client.Coverage(ctx, service.resolveWorld(worldID))
}

func (service *DesktopService) CatalogDatasets(ctx context.Context) (CatalogDatasetCatalogResponse, error) {
	if err := service.queryReady(); err != nil {
		return CatalogDatasetCatalogResponse{}, err
	}
	return service.client.CatalogDatasets(ctx)
}

func (service *DesktopService) CatalogDataset(ctx context.Context, datasetKey string, historyLimit int) (CatalogDatasetResponse, error) {
	if err := service.queryReady(); err != nil {
		return CatalogDatasetResponse{}, err
	}
	return service.client.CatalogDataset(ctx, datasetKey, historyLimit)
}

func (service *DesktopService) resolveWorld(worldID string) string {
	if normalized := NormalizeWorldID(worldID); normalized != "" {
		return normalized
	}
	return service.CurrentWorldID()
}

func (service *DesktopService) queryReady() error {
	if service == nil || service.client == nil {
		return fmt.Errorf("world intelligence service is unavailable")
	}
	return nil
}
