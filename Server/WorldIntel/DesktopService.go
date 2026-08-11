package WorldIntel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	uploadPoll       = 30 * time.Second
	scannerPoll      = 20 * time.Second
	scannerRetryWait = 5 * time.Minute
)

type Settings struct {
	CollectorPlayerID int64 `json:"collectorPlayerId,omitempty"`
	CollectorSlot     int   `json:"collectorSlot,omitempty"`
	CollectorSlots    int   `json:"collectorSlots,omitempty"`
}

type DesktopService struct {
	state          *State.Store
	gameData       *GameData.Manager
	refreshCatalog func(context.Context) error
	configuration  *Configuration.Store
	store          *DesktopStore
	client         *CloudClient
	wake           chan struct{}
	done           chan struct{}

	collectionMu          sync.RWMutex
	collectionInProgress  bool
	collectedDatasets     int
	nextCollectionRetryAt time.Time
	lastCatalogRefresh    time.Time

	runOnce   sync.Once
	closeOnce sync.Once
}

func NewDesktopService(
	dataDir string,
	state *State.Store,
	gameData *GameData.Manager,
	refreshCatalog func(context.Context) error,
	configuration *Configuration.Store,
	client *CloudClient,
) (*DesktopService, error) {
	store, err := OpenDesktopStore(dataDir)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = NewCloudClient(ClientConfig{})
	}
	return &DesktopService{
		state: state, gameData: gameData, refreshCatalog: refreshCatalog, configuration: configuration, store: store, client: client,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
		lastCatalogRefresh: time.Now().UTC(),
	}, nil
}

func (service *DesktopService) Run(ctx context.Context) {
	if service == nil {
		return
	}
	service.runOnce.Do(func() {
		defer close(service.done)
		if service.store == nil {
			return
		}
		var wait sync.WaitGroup
		wait.Add(2)
		go func() { defer wait.Done(); service.runCatalogCollector(ctx) }()
		go func() {
			defer wait.Done()
			service.runUploader(ctx)
		}()
		wait.Wait()
	})
}

func (service *DesktopService) Wait() {
	if service == nil || service.done == nil {
		return
	}
	<-service.done
}

func (service *DesktopService) Status(ctx context.Context) DesktopStatus {
	settings := service.Settings()
	storeStatus := StoreStatus{}
	if service != nil && service.store != nil {
		storeStatus = service.store.Status(ctx)
	}
	status := DesktopStatus{
		Enabled: true, CollectionMode: "official-catalog",
		PendingBatches: storeStatus.Pending, LastCapturedAt: storeStatus.LastCapturedAt,
		PendingCatalogs: storeStatus.PendingCatalogs,
		LastUploadAt:    storeStatus.LastUploadAt, LastUploadError: storeStatus.LastError,
		CatalogVersion: storeStatus.LastCatalogVersion, CatalogDatasets: storeStatus.CatalogDatasets,
		LastCatalogAt: storeStatus.LastCatalogAt, LastCatalogError: storeStatus.LastCatalogError,
		LastScanAt: storeStatus.LastScanAt, LastScanError: storeStatus.LastScanError,
		PublicFieldsOnly: true, OfficialSourceOnly: true,
	}
	if service != nil && service.client != nil {
		status.Endpoint = service.client.Endpoint()
	}
	if service != nil && service.state != nil {
		snapshot := service.state.Snapshot()
		status.WorldID = NormalizeWorldID(snapshot.Account.WorldID)
		if status.WorldID == "" {
			status.WorldID = NormalizeWorldID(snapshot.Session.ServerURL)
		}
	}
	status.Collector = settings.collectsCatalog()
	status.Contributing = status.Collector
	if status.Collector {
		status.CollectorPlayerID = settings.CollectorPlayerID
		status.CollectorSlot = settings.CollectorSlot
		status.CollectorSlots = settings.CollectorSlots
		next := nextCollectorSlot(time.Now().UTC(), settings.CollectorSlot, settings.CollectorSlots)
		status.NextCatalogAt = &next
	}
	if service != nil && service.gameData != nil {
		if current, found := service.gameData.Current(); found && status.CatalogVersion == "" {
			status.CatalogVersion = current.Metadata().ItemVersion
		}
	}
	service.collectionMu.RLock()
	status.CatalogCollectionInProgress = service.collectionInProgress
	if service.collectedDatasets > 0 {
		status.CatalogDatasets = service.collectedDatasets
	}
	service.collectionMu.RUnlock()
	return status
}

func (service *DesktopService) Settings() Settings {
	settings := Settings{}
	if service == nil || service.configuration == nil {
		return settings
	}
	raw, found := service.configuration.Section("world-intelligence")
	if !found || json.Unmarshal(raw, &settings) != nil {
		return Settings{}
	}
	return settings
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

func (service *DesktopService) PersistenceError() error {
	if service == nil || service.store == nil {
		return nil
	}
	return service.store.PersistenceError()
}

func (service *DesktopService) Close() error {
	if service == nil {
		return nil
	}
	var err error
	service.closeOnce.Do(func() {
		if service.store != nil {
			err = service.store.Close()
		}
	})
	return err
}

func (service *DesktopService) runUploader(ctx context.Context) {
	poll := time.NewTicker(uploadPoll)
	defer poll.Stop()
	service.signal()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
		case <-service.wake:
		}
		service.uploadAvailable(ctx)
	}
}

func (service *DesktopService) uploadAvailable(ctx context.Context) {
	pendingCatalogs, catalogErr := service.store.PendingCatalog(ctx, time.Now(), desktopBatchLimit)
	pending, observationErr := service.store.Pending(ctx, time.Now(), desktopBatchLimit)
	if catalogErr != nil || observationErr != nil || len(pendingCatalogs)+len(pending) == 0 {
		return
	}
	credentials, err := service.store.Credentials(ctx)
	if err != nil {
		return
	}
	if err := service.client.Register(ctx, credentials); err != nil {
		service.store.RecordUploadError(ctx, err.Error())
		return
	}
	for {
		for _, queued := range pendingCatalogs {
			uploadedAt := time.Now().UTC()
			response, err := service.client.UploadCatalog(ctx, credentials, queued.Snapshot)
			if err != nil {
				_ = service.store.FailCatalog(ctx, queued.Snapshot.SnapshotID, queued.Attempts, err.Error(), uploadedAt)
				return
			}
			if response.SnapshotID != "" && response.SnapshotID != queued.Snapshot.SnapshotID {
				_ = service.store.FailCatalog(ctx, queued.Snapshot.SnapshotID, queued.Attempts, "Cloud acknowledged a different catalog snapshot.", uploadedAt)
				return
			}
			if err := service.store.ConfirmCatalog(ctx, queued.Snapshot.SnapshotID, uploadedAt); err != nil {
				return
			}
		}
		if len(pendingCatalogs) < desktopBatchLimit {
			break
		}
		pendingCatalogs, catalogErr = service.store.PendingCatalog(ctx, time.Now(), desktopBatchLimit)
		if catalogErr != nil || len(pendingCatalogs) == 0 {
			break
		}
	}
	for {
		for _, queued := range pending {
			uploadedAt := time.Now().UTC()
			response, err := service.client.Upload(ctx, credentials, queued.Batch)
			if err != nil {
				_ = service.store.Fail(ctx, queued.Batch.BatchID, queued.Attempts, err.Error(), uploadedAt)
				return
			}
			if response.BatchID != "" && response.BatchID != queued.Batch.BatchID {
				_ = service.store.Fail(ctx, queued.Batch.BatchID, queued.Attempts, "Cloud acknowledged a different batch.", uploadedAt)
				return
			}
			if err := service.store.Confirm(ctx, queued.Batch.BatchID, uploadedAt); err != nil {
				return
			}
		}
		if len(pending) < desktopBatchLimit {
			return
		}
		pending, err = service.store.Pending(ctx, time.Now(), desktopBatchLimit)
		if err != nil || len(pending) == 0 {
			return
		}
	}
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

func (settings Settings) collectsCatalog() bool {
	return settings.CollectorPlayerID > 0 && settings.CollectorSlots >= 1 && settings.CollectorSlots <= 8 &&
		settings.CollectorSlot >= 0 && settings.CollectorSlot < settings.CollectorSlots
}

func (service *DesktopService) signal() {
	select {
	case service.wake <- struct{}{}:
	default:
	}
}
