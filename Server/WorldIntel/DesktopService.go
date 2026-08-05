package WorldIntel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

const (
	captureDebounce = 2 * time.Second
	uploadPoll      = 30 * time.Second
)

type Settings struct {
	Enabled                      bool `json:"enabled"`
	ContributePublicObservations bool `json:"contributePublicObservations"`
}

type DesktopService struct {
	state         *State.Store
	configuration *Configuration.Store
	store         *DesktopStore
	client        *CloudClient
	wake          chan struct{}
	done          chan struct{}

	runOnce   sync.Once
	closeOnce sync.Once
}

func NewDesktopService(
	dataDir string,
	state *State.Store,
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
		state: state, configuration: configuration, store: store, client: client,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
	}, nil
}

func (service *DesktopService) Run(ctx context.Context) {
	if service == nil {
		return
	}
	service.runOnce.Do(func() {
		defer close(service.done)
		if service.store == nil || service.state == nil {
			return
		}
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			service.runCollector(ctx)
		}()
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
		Enabled: settings.Enabled, Contributing: settings.Enabled && settings.ContributePublicObservations,
		PendingBatches: storeStatus.Pending, LastCapturedAt: storeStatus.LastCapturedAt,
		LastUploadAt: storeStatus.LastUploadAt, LastUploadError: storeStatus.LastError,
		PublicFieldsOnly: true,
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

func (service *DesktopService) Coverage(ctx context.Context, worldID string) (CoverageResponse, error) {
	if err := service.queryReady(); err != nil {
		return CoverageResponse{}, err
	}
	return service.client.Coverage(ctx, service.resolveWorld(worldID))
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

func (service *DesktopService) runCollector(ctx context.Context) {
	events, unsubscribe := service.state.Subscribe(32)
	defer unsubscribe()
	refresh := time.NewTicker(captureBucket)
	defer refresh.Stop()
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	capture := func() {
		settings := service.Settings()
		if !settings.Enabled || !settings.ContributePublicObservations {
			return
		}
		batch, available, err := BuildObservationBatch(service.state.Snapshot(), time.Now())
		if err != nil || !available {
			return
		}
		inserted, err := service.store.Enqueue(ctx, batch)
		if err == nil && inserted {
			service.signal()
		}
	}
	capture()
	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case <-events:
			if debounce == nil {
				debounce = time.NewTimer(captureDebounce)
				debounceChannel = debounce.C
			} else if debounce.Stop() {
				debounce.Reset(captureDebounce)
			}
		case <-debounceChannel:
			capture()
			debounce = nil
			debounceChannel = nil
		case <-refresh.C:
			capture()
		}
	}
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
		settings := service.Settings()
		if !settings.Enabled || !settings.ContributePublicObservations {
			continue
		}
		service.uploadAvailable(ctx)
	}
}

func (service *DesktopService) uploadAvailable(ctx context.Context) {
	pending, err := service.store.Pending(ctx, time.Now(), desktopBatchLimit)
	if err != nil || len(pending) == 0 {
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
	if !service.Settings().Enabled {
		return fmt.Errorf("world intelligence is disabled")
	}
	return nil
}

func (service *DesktopService) signal() {
	select {
	case service.wake <- struct{}{}:
	default:
	}
}
