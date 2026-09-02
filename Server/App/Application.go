package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Diagnostics"
	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/RiftTemplates"
	RuntimeKernel "CitadelDesktop/Server/Runtime"
	"CitadelDesktop/Server/Scheduling"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
	"CitadelDesktop/Server/WorldIntel"
)

const GameDataRefreshInterval = 6 * time.Hour

type Config struct {
	DataDir string
	Offline bool
	// AccountKey is an optional process-local coordination identity. It is never
	// included in persistence, API responses, or frontend events.
	AccountKey string
	// GameData optionally injects an official-data manager for tests or custom
	// compositions. CitadelOpsDesktop creates and refreshes its own manager.
	GameData *GameData.Manager
	// WorldMaps optionally provides a process-local observed-world store.
	WorldMaps *State.WorldMapStore
	// IngestRegistry optionally supplies immutable opcode-to-reducer definitions.
	IngestRegistry *Ingest.Registry
	// RefreshGameData optionally coordinates an injected GameData manager.
	RefreshGameData func(context.Context) error
	// Updates optionally supplies the application update manager.
	Updates *AppUpdate.Manager
	// WorldIntelClient optionally supplies an immutable cloud client.
	WorldIntelClient *WorldIntel.CloudClient
	// ReportsCloudClient optionally supplies an immutable report client.
	ReportsCloudClient *Reports.CloudClient
	// PrivateMetricsClient is present only in explicitly configured hosted
	// compositions. It carries My Stats and Feature Stats projections; desktop
	// compositions must keep those datasets on their profile disk. The placement
	// grant remains account-scoped.
	PrivateMetricsClient    *PrivateMetrics.Client
	PrivateMetricsPlacement *PrivateMetrics.Placement
	Transport               Session.Transport
	Chromium                *Session.ChromiumConfig
	// BackgroundOnly is the hosted composition. It always constructs the
	// account-private direct game transport and never starts Chromium, even if
	// the account profile was originally created by a desktop build.
	BackgroundOnly         bool
	RuntimeContext         context.Context
	UpdateEndpoint         string
	UpdateInstallSupported bool
}

type Application struct {
	DataDir          string
	AccountKey       string
	BackgroundOnly   bool
	State            *State.Store
	GameData         *GameData.Manager
	WorldMaps        *State.WorldMapStore
	Configuration    *Configuration.Store
	History          *History.Store
	Telemetry        *Telemetry.Store
	Ingest           *Ingest.Pipeline
	Session          *Session.Controller
	Intents          *Intent.Engine
	OperationStore   *Intent.SQLiteOperationStore
	ProfileLease     *RuntimeKernel.ProfileLease
	Automation       *Automation.Coordinator
	Reports          *Reports.Manager
	BattleResearch   *Reports.BattleResearchManager
	ReportStore      *Reports.SQLiteStore
	Scheduler        *Scheduling.Scheduler
	API              *API.Server
	Updates          *AppUpdate.Manager
	Diagnostics      *Diagnostics.Monitor
	WorldIntelClient *WorldIntel.CloudClient
	WorldIntel       *WorldIntel.DesktopService
	PrivateMetrics   *PrivateMetrics.Publisher
	Checkpoints      *PrivateMetrics.CheckpointPublisher
	BackgroundLogin  *Session.BackgroundLoginStore
	StartupErr       error

	persistenceHealthMu       sync.RWMutex
	statePersistenceErr       error
	statePersistence          chan statePersistenceRequest
	statePersistenceDone      chan struct{}
	statePersistenceStarted   atomic.Bool
	controlConfigurationState atomic.Uint32
	backgroundOnly            bool
	ownsGameData              bool
	ownsUpdates               bool
	refreshGameDataAll        func(context.Context) error
	startOnce                 sync.Once
	shutdownDone              chan struct{}
}

// SetControlConfigurationReady gates hosted runtime mutations while the
// account-owned control-plane snapshot is pending. Desktop compositions never
// set required and retain their ordinary local configuration behavior.
func (application *Application) SetControlConfigurationReady(required, ready bool) {
	if application == nil {
		return
	}
	if required {
		if application.Configuration != nil {
			application.Configuration.SetExternalAuthority(
				true,
				History.PlayerSamplesConfigurationSection,
				Reports.BattleResearchConfigurationSection,
			)
		}
		if application.API != nil {
			application.API.SetExternalConfigurationAuthority(true)
		}
	}
	state := uint32(controlConfigurationUnmanaged)
	if required {
		state = controlConfigurationPending
		if ready {
			state = controlConfigurationReady
		}
	}
	application.controlConfigurationState.Store(state)
	if !required && application.API != nil {
		application.API.SetExternalConfigurationAuthority(false)
	}
	if !required && application.Configuration != nil {
		application.Configuration.SetExternalAuthority(false)
	}
}

type statePersistenceRequest struct {
	revision uint64
	result   chan error
}

func New(ctx context.Context, config Config) (*Application, error) {
	if config.DataDir == "" {
		return nil, fmt.Errorf("application data directory is required")
	}
	if !config.BackgroundOnly &&
		((config.PrivateMetricsClient != nil && config.PrivateMetricsClient.Enabled()) || config.PrivateMetricsPlacement != nil) {
		return nil, fmt.Errorf("private metrics publishing requires hosted background mode")
	}
	profileLease, err := RuntimeKernel.AcquireProfileLease(config.DataDir)
	if err != nil {
		return nil, err
	}
	closeProfileLease := true
	defer func() {
		if closeProfileLease {
			_ = profileLease.Close()
		}
	}()
	configuration, err := Configuration.Open(config.DataDir, defaultConfiguration())
	if err != nil {
		return nil, err
	}
	history, err := History.Open(config.DataDir)
	if err != nil {
		return nil, err
	}
	var startupErr error
	gameData := config.GameData
	ownsGameData := gameData == nil
	if ownsGameData {
		gameData = GameData.NewManager(GameData.UpdaterConfig{
			CacheDir: filepath.Join(config.DataDir, "GameData", "Items"),
		})
		if config.Offline {
			startupErr = gameData.LoadCache()
		} else {
			startupErr = gameData.Initialize(ctx)
		}
	} else if _, ready := gameData.Current(); !ready {
		startupErr = errors.Join(startupErr, fmt.Errorf("shared official game data is not initialized"))
	}

	initial := State.NewGameState()
	if recovered, recoveryErr := State.LoadSnapshot(config.DataDir); recoveryErr == nil {
		initial = recovered
	} else if !os.IsNotExist(recoveryErr) {
		startupErr = errors.Join(startupErr, recoveryErr)
	}
	if current, ready := gameData.Current(); ready {
		initial.CatalogVersion = current.Metadata().ItemVersion
		initial.LanguageVersion = current.Metadata().LanguageVersion
		EquipmentDomain.HydrateState(&initial, current)
	}
	state := State.NewStoreWithWorldMap(initial, config.WorldMaps)
	if migrationErr := Reports.MigrateLegacyHistory(config.DataDir, history, initial.Player.ID); migrationErr != nil {
		startupErr = errors.Join(startupErr, migrationErr)
	}
	registry := config.IngestRegistry
	if registry == nil {
		registry = Ingest.NewRegistry()
		if err := Ingest.RegisterCoreReducers(registry); err != nil {
			return nil, fmt.Errorf("register protocol reducers: %w", err)
		}
	}
	ingest := Ingest.NewPipeline(state, gameData, registry)
	ingest.SetProfileID(profileLease.ProfileID)
	telemetry := Telemetry.NewStore(5000)
	if telemetryErr := telemetry.SetDataDir(config.DataDir); telemetryErr != nil {
		startupErr = errors.Join(startupErr, fmt.Errorf("initialize logger: %w", telemetryErr))
	}
	ingest.SetTelemetry(telemetry)
	transport := config.Transport
	if transport == nil && (config.Chromium != nil || config.BackgroundOnly) {
		mode := Session.ConnectionModeFull
		serverSelection := ""
		if raw, ok := configuration.Section("session.connection"); ok {
			var selected struct {
				Mode   string `json:"mode"`
				Server string `json:"server"`
			}
			if json.Unmarshal(raw, &selected) == nil {
				mode = Session.ParseConnectionMode(selected.Mode)
				serverSelection = strings.TrimSpace(selected.Server)
			}
		}
		if config.BackgroundOnly {
			mode = Session.ConnectionModeBackground
		}
		if mode == Session.ConnectionModeBackground {
			language := "en"
			if currentLanguage, ready := gameData.Language(); ready {
				if selectedLanguage := strings.TrimSpace(currentLanguage.Metadata().Language); selectedLanguage != "" {
					language = selectedLanguage
				}
			}
			transport = Session.NewDirectWebSocketTransport(Session.DirectWebSocketConfig{
				DataDir: config.DataDir, Server: serverSelection,
				ServerURL: strings.TrimSpace(initial.Session.ServerURL),
				Namespace: initial.Session.Namespace, Language: language,
			})
		} else {
			chromium := *config.Chromium
			chromium.DataDir = config.DataDir
			transport = Session.NewChromiumTransport(chromium)
		}
	}
	if transport == nil {
		transport = Session.NewUnavailableTransport()
	}
	runtimeContext := config.RuntimeContext
	if runtimeContext == nil {
		runtimeContext = context.Background()
	}
	session := Session.NewController(runtimeContext, transport, ingest, state)
	intentRegistry := Intent.NewRegistry()
	intentRegistry.EnforceResourceDeclarations()
	intents := Intent.NewEngine(intentRegistry, state, gameData, session, ingest)
	// Dashboard and API submissions execute under the application's runtime
	// context: closing a control panel never cancels a running operation.
	intents.SetRuntimeContext(runtimeContext)
	// Launch bursts must never re-select a commander whose movement is not
	// yet visible; the hold registry closes that window (see
	// CommanderLaunchHolds.go and the CRA 256 churn it prevents).
	intents.SetCommanderHolds(newCommanderLaunchHolds())
	operationStore, err := Intent.OpenOperationStore(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open intent operation store: %w", err)
	}
	closeOperationStore := true
	defer func() {
		if closeOperationStore {
			_ = operationStore.Close()
		}
	}()
	if err := intents.SetOperationStore(ctx, operationStore); err != nil {
		return nil, fmt.Errorf("recover intent operations: %w", err)
	}
	reportStore, err := Reports.OpenSQLiteStore(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open report analytics store: %w", err)
	}
	closeReportStore := true
	defer func() {
		if closeReportStore {
			_ = reportStore.Close()
		}
	}()
	if err := Reports.BackfillBattleHistory(ctx, history, reportStore, initial); err != nil {
		return nil, fmt.Errorf("backfill report analytics: %w", err)
	}
	if _, err := Reports.CompactBattleHistory(history); err != nil {
		return nil, fmt.Errorf("compact local battle report outbox: %w", err)
	}
	if _, err := Reports.BackfillCloudOutbox(ctx, history, reportStore, initial); err != nil {
		return nil, fmt.Errorf("backfill cloud battle report outbox: %w", err)
	}
	if err := restoreRecentAutoStormLaunchHistory(ctx, state, reportStore); err != nil {
		return nil, fmt.Errorf("restore recent Auto Storm launch history: %w", err)
	}
	worldIntelClient := config.WorldIntelClient
	if worldIntelClient == nil {
		worldIntelClient = WorldIntel.NewCloudClient(WorldIntel.ClientConfig{ClientVersion: Version})
	}
	worldIntelligence := WorldIntel.NewDesktopService(state, worldIntelClient)
	var privateMetricsPublisher *PrivateMetrics.Publisher
	var checkpointPublisher *PrivateMetrics.CheckpointPublisher
	if config.PrivateMetricsClient != nil && config.PrivateMetricsClient.Enabled() {
		privateMetricsPublisher, err = PrivateMetrics.NewPublisher(PrivateMetrics.PublisherConfig{
			RuntimeID: strings.TrimSpace(config.AccountKey), State: state, GameData: gameData,
			Reports: reportStore, Client: config.PrivateMetricsClient,
			Placement: config.PrivateMetricsPlacement,
		})
		if err != nil {
			return nil, fmt.Errorf("initialize private metrics publisher: %w", err)
		}
		if config.PrivateMetricsClient.CheckpointsEnabled() {
			checkpointPublisher, err = PrivateMetrics.NewCheckpointPublisher(PrivateMetrics.CheckpointPublisherConfig{
				RuntimeID: strings.TrimSpace(config.AccountKey), State: state, Configuration: configuration,
				Intents: intents, Client: config.PrivateMetricsClient, Placement: config.PrivateMetricsPlacement,
			})
			if err != nil {
				return nil, fmt.Errorf("initialize dashboard checkpoint publisher: %w", err)
			}
		}
	}
	updates := config.Updates
	ownsUpdates := updates == nil
	if updates == nil {
		updates = AppUpdate.NewManager(AppUpdate.Config{
			CurrentVersion: Version, Endpoint: config.UpdateEndpoint,
			InstallSupported: config.UpdateInstallSupported,
		})
	}
	application := &Application{
		DataDir: config.DataDir, AccountKey: strings.TrimSpace(config.AccountKey),
		BackgroundOnly: config.BackgroundOnly,
		State:          state, GameData: gameData, WorldMaps: config.WorldMaps, Configuration: configuration, History: history, Telemetry: telemetry,
		Ingest: ingest, Session: session, Intents: intents, OperationStore: operationStore, ReportStore: reportStore,
		ProfileLease: profileLease, StartupErr: startupErr,
		Updates:              updates,
		Diagnostics:          Diagnostics.NewMonitor(config.DataDir),
		WorldIntelClient:     worldIntelClient,
		WorldIntel:           worldIntelligence,
		PrivateMetrics:       privateMetricsPublisher,
		Checkpoints:          checkpointPublisher,
		BackgroundLogin:      Session.NewBackgroundLoginStore(config.DataDir),
		backgroundOnly:       config.BackgroundOnly,
		ownsGameData:         ownsGameData,
		ownsUpdates:          ownsUpdates,
		refreshGameDataAll:   config.RefreshGameData,
		shutdownDone:         make(chan struct{}),
		statePersistence:     make(chan statePersistenceRequest),
		statePersistenceDone: make(chan struct{}),
	}
	session.SetAttackDelayProvider(application.attackLaunchDelay)
	if relogTransport, ok := transport.(Session.RelogDelayTransport); ok {
		relogTransport.SetRelogDelayProvider(application.relogDelay)
	}
	session.SetAutomationLocked(application.automationLocked())
	intents.SetExecutionGate(application.executionGate)
	intents.SetAdmissionWeightProvider(application.attackAdmissionWeight)
	application.Scheduler = Scheduling.NewScheduler(state, intents)
	if err := application.registerCoreIntents(); err != nil {
		return nil, err
	}
	if err := application.registerGameIntents(); err != nil {
		return nil, err
	}
	if err := application.registerBuildingIntents(); err != nil {
		return nil, err
	}
	if err := application.registerShopIntents(); err != nil {
		return nil, err
	}
	if err := application.registerStormIntents(); err != nil {
		return nil, err
	}
	application.Automation = Automation.NewCoordinator(
		state, configuration, gameData, intents,
		Automation.NewSharedStormScanPolicy(application.AccountKey, config.WorldMaps),
		Automation.NewRecruitPolicy(),
		Automation.NewToolPolicy(),
		Automation.NewHospitalPolicy(),
		Automation.NewAllianceHelpPolicy(),
		Automation.NewAutoEquipmentCleanupPolicy(),
		Automation.NewDailyAttackRefreshPolicy(),
		Automation.NewConstructionPolicy(),
		Automation.NewCraftingPolicy(),
		Automation.NewCraftingLogisticsPolicy(),
		Automation.NewAutoBirdPolicy(),
		Automation.NewAutoStationPolicy(),
		Automation.NewBeriPolicy(),
		Automation.NewBeriToolPolicy(),
		Automation.NewBeriBuildPolicy(),
		Automation.NewBeriAttackPolicy(),
		Automation.NewFoodBalancePolicy(),
		Automation.NewAutoTowerPolicy(),
		Automation.NewAutoInvasionPolicy(),
		Automation.NewAutoNomadPolicy(),
		Automation.NewAutoAdvisorPolicy(),
		Automation.NewAutoBuyerPolicy(),
		Automation.NewRiftMaidenRunPolicy(),
		Automation.NewAutoKhanPolicy(),
		Automation.NewAutoKhanCooldownPolicy(),
		Automation.NewAutoKhanRagePolicy(),
		Automation.NewAutoKhanDefensePolicy(),
		Automation.NewAutoStormPolicy(),
		Automation.NewAutoStormShopPolicy(),
		Automation.NewAutoStormBuildPolicy(),
	)
	application.Automation.SetExternalConfigurationAuthority(config.BackgroundOnly)
	application.Reports = Reports.NewManagerWithCloudClient(
		state, history, intents, config.ReportsCloudClient, reportStore,
	)
	// Experimental Battle Research is intentionally no longer composed. Keep
	// the implementation and existing local trial rows intact for rollback,
	// while ensuring old saved consent cannot trigger spies or uploads.
	application.API = API.NewServer(API.Config{
		Version: Version, BuildRevision: BuildRevision, BuildID: BuildID,
		State: state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Intents: intents, ReportAnalytics: reportStore, Session: session, Updates: application.Updates, Diagnostics: application.Diagnostics,
		CloudReports: application.Reports.CloudClient(), BattleResearch: application.BattleResearch,
		BackgroundLogin: application.BackgroundLogin, BackgroundOnly: config.BackgroundOnly, Persistence: application,
		WorldIntel: application.WorldIntel,
	})
	closeOperationStore = false
	closeReportStore = false
	closeProfileLease = false
	return application, nil
}

func (application *Application) Start(ctx context.Context) {
	if application == nil {
		return
	}
	application.startOnce.Do(func() {
		application.start(ctx)
	})
}

func (application *Application) start(ctx context.Context) {
	application.statePersistenceStarted.Store(true)
	go application.captureIntentLogs(ctx)
	go func() {
		defer close(application.shutdownDone)
		<-ctx.Done()
		<-application.statePersistenceDone
		application.Telemetry.Close()
		if application.Reports != nil {
			application.Reports.Wait()
		}
		if application.BattleResearch != nil {
			application.BattleResearch.Wait()
		}
		if application.Intents != nil {
			// Detached operations were cancelled with the runtime context; give
			// them a bounded moment to record their final receipts before the
			// operation store closes underneath them.
			drainContext, cancelDrain := context.WithTimeout(context.Background(), 5*time.Second)
			_ = application.Intents.WaitIdle(drainContext)
			cancelDrain()
		}
		if application.OperationStore != nil {
			_ = application.OperationStore.Close()
		}
		if application.ReportStore != nil {
			_ = application.ReportStore.Close()
		}
		if application.ProfileLease != nil {
			_ = application.ProfileLease.Close()
		}
	}()
	if application.ownsUpdates {
		go application.Updates.Run(ctx)
	}
	if application.ownsGameData {
		go func() {
			ticker := time.NewTicker(GameDataRefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					refreshContext, cancel := context.WithTimeout(ctx, 90*time.Second)
					_ = application.refreshGameData(refreshContext)
					cancel()
				}
			}
		}()
	}
	go application.capturePlayerHistory(ctx)
	go application.refreshAttackHistoryOnBaseline(ctx)
	if application.PrivateMetrics != nil {
		go application.PrivateMetrics.Run(ctx)
	}
	if application.Checkpoints != nil {
		go application.Checkpoints.Run(ctx)
	}
	go application.persistState(ctx)
	go application.runMovementClock(ctx)
	go application.Automation.Run(ctx)
	go application.Reports.Run(ctx)
	go application.Scheduler.Run(ctx)
}

// SetSessionReconnectPolicy tells the game transport whether to hold the
// session loop across disconnects or to release it for the control plane.
func (application *Application) SetSessionReconnectPolicy(policy Session.ReconnectPolicy) {
	if application == nil || application.Session == nil {
		return
	}
	application.Session.SetReconnectPolicy(policy)
}

// SetPrivateMetricsPlacement rotates only this account runtime's outbound
// metrics grant. It never changes dashboard authentication or another runtime.
func (application *Application) SetPrivateMetricsPlacement(placement *PrivateMetrics.Placement) error {
	if application == nil || application.PrivateMetrics == nil {
		if placement == nil {
			return nil
		}
		return fmt.Errorf("private metrics publisher is unavailable")
	}
	if err := application.PrivateMetrics.SetPlacement(placement); err != nil {
		return err
	}
	if application.Checkpoints != nil {
		return application.Checkpoints.SetPlacement(placement)
	}
	return nil
}

// Checkpoint publishes the dashboard read model now (for example the final
// checkpoint before the runtime is drained). It is a no-op without a
// checkpoint publisher or without a current placement.
func (application *Application) Checkpoint(ctx context.Context, reason PrivateMetrics.CheckpointReason) error {
	if application == nil || application.Checkpoints == nil {
		return nil
	}
	return application.Checkpoints.Checkpoint(ctx, reason)
}

// Wait blocks until every application worker has stopped and its durable
// stores and profile lease are closed.
func (application *Application) Wait(ctx context.Context) error {
	if application == nil || application.shutdownDone == nil {
		return nil
	}
	select {
	case <-application.shutdownDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (application *Application) captureIntentLogs(ctx context.Context) {
	if application == nil || application.Intents == nil || application.Telemetry == nil {
		return
	}
	events, unsubscribe := application.Intents.Subscribe(512)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case receipt := <-events:
			application.recordIntentLog(receipt)
		}
	}
}

func (application *Application) recordIntentLog(receipt Intent.Receipt) {
	if application == nil || application.Telemetry == nil {
		return
	}
	for _, activity := range featureActivities(receipt) {
		application.Telemetry.RecordFeatureActivity(
			receipt.Actor, receipt.Intent, activity.severity, activity.event, activity.detail,
		)
	}
}

func (application *Application) persistState(ctx context.Context) {
	defer close(application.statePersistenceDone)
	events, unsubscribe := application.State.Subscribe(128)
	defer unsubscribe()
	writer := State.NewComponentSnapshotWriter(application.DataDir)
	var timer *time.Timer
	var timerChannel <-chan time.Time
	var pending State.PersistenceBatch
	var persistedRevision uint64
	accumulate := func(event State.Event) {
		if !pending.Accumulate(event) {
			return
		}
		if timer == nil {
			timer = time.NewTimer(2 * time.Second)
			timerChannel = timer.C
		}
	}
	flush := func() error {
		if pending.Revision() == 0 {
			return nil
		}
		revision, err := pending.FlushWithWriter(writer)
		application.persistenceHealthMu.Lock()
		application.statePersistenceErr = err
		application.persistenceHealthMu.Unlock()
		if err == nil {
			persistedRevision = revision
			if timer != nil {
				timer.Stop()
				timer = nil
				timerChannel = nil
			}
		}
		return err
	}
	force := func(request statePersistenceRequest) {
		for pending.Revision() < request.revision && persistedRevision < request.revision {
			select {
			case event := <-events:
				accumulate(event)
			case <-ctx.Done():
				request.result <- ctx.Err()
				return
			}
		}
		request.result <- flush()
	}
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			_ = flush()
			return
		case event := <-events:
			accumulate(event)
		case request := <-application.statePersistence:
			force(request)
		case <-timerChannel:
			if flush() != nil {
				timer = time.NewTimer(2 * time.Second)
				timerChannel = timer.C
			}
		}
	}
}

func (application *Application) saveStateEvent(ctx context.Context, event State.Event) error {
	if application == nil || application.State == nil || event.Patch == nil || strings.TrimSpace(application.DataDir) == "" {
		return nil
	}
	if !application.statePersistenceStarted.Load() {
		err := State.SaveComponentSnapshot(application.DataDir, event, State.Components(event.Components...))
		application.persistenceHealthMu.Lock()
		application.statePersistenceErr = err
		application.persistenceHealthMu.Unlock()
		return err
	}
	request := statePersistenceRequest{revision: event.Revision, result: make(chan error, 1)}
	select {
	case application.statePersistence <- request:
	case <-application.statePersistenceDone:
		return fmt.Errorf("state persistence worker is stopped")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.result:
		return err
	case <-application.statePersistenceDone:
		return fmt.Errorf("state persistence worker stopped before revision %d was durable", event.Revision)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (application *Application) PersistenceError() error {
	actionErr := application.actionPersistenceError()
	if application == nil {
		return actionErr
	}
	var reportErr error
	if application.ReportStore != nil {
		reportErr = application.ReportStore.LastError()
	}
	if reportErr != nil {
		reportErr = fmt.Errorf("report analytics persistence: %w", reportErr)
	}
	return errors.Join(actionErr, reportErr)
}

func (application *Application) actionPersistenceError() error {
	if application == nil {
		return nil
	}
	application.persistenceHealthMu.RLock()
	stateErr := application.statePersistenceErr
	application.persistenceHealthMu.RUnlock()
	var worldMapErr error
	if application.WorldMaps != nil {
		worldMapErr = application.WorldMaps.PersistenceError()
	}
	var operationErr error
	if application.Intents != nil {
		operationErr = application.Intents.PersistenceError()
	}
	if stateErr != nil {
		stateErr = fmt.Errorf("state snapshot persistence: %w", stateErr)
	}
	if operationErr != nil {
		operationErr = fmt.Errorf("operation journal persistence: %w", operationErr)
	}
	return errors.Join(stateErr, worldMapErr, operationErr)
}

func (application *Application) capturePlayerHistory(ctx context.Context) {
	events, unsubscribe := application.State.Subscribe(32)
	defer unsubscribe()
	configurationEvents, unsubscribeConfiguration := application.Configuration.Subscribe(8)
	defer unsubscribeConfiguration()
	initialPolicy := application.playerSamplesRetentionPolicy()
	ticker := time.NewTicker(History.PlayerSamplesRecordingIntervalDuration(initialPolicy.RecordingIntervalSeconds))
	defer ticker.Stop()
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	lastCaptured := time.Time{}
	resolvePolicy := func() History.PlayerSamplesStoragePolicy {
		return History.PlayerSamplesStoragePolicyFromRetentionPolicy(application.playerSamplesRetentionPolicy())
	}
	compact := func(force bool) {
		if force {
			if _, err := application.History.CompactPlayerSamplesWithResolvedPolicy(time.Now().UTC(), resolvePolicy); err != nil {
				log.Printf("player history retention: %v", err)
			}
			return
		}
		if _, ran, err := application.History.CompactPlayerSamplesIfDueResolvedPolicy(time.Now().UTC(), resolvePolicy); ran && err != nil {
			log.Printf("player history retention: %v", err)
		}
	}
	capture := func() {
		policy := application.playerSamplesRetentionPolicy()
		if policy.Effective == History.PlayerSamplesRetentionNone {
			lastCaptured = time.Now().UTC()
			return
		}
		snapshot := application.State.ReadOnlyView()
		if snapshot.Player.ID == 0 {
			return
		}
		observedAt := time.Now().UTC()
		_, _, err := application.History.CapturePlayerSampleWithPolicy(
			History.NewPlayerSampleAt(snapshot, application.GameData, observedAt),
			observedAt,
			History.PlayerSamplesStoragePolicyFromRetentionPolicy(policy),
		)
		if err != nil {
			log.Printf("player history capture: %v", err)
			return
		}
		// A restart in an already-recorded bucket is still a completed attempt;
		// ticker-driven capture owns subsequent buckets without a state-event
		// retry storm.
		lastCaptured = observedAt
	}
	// Apply a saved reduction even while the game is logged out and no player
	// state is available. In particular, "none" clears the collection without
	// waiting for a future capture.
	compact(true)
	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case <-events:
			if lastCaptured.IsZero() {
				if debounce == nil {
					debounce = time.NewTimer(250 * time.Millisecond)
					debounceChannel = debounce.C
				} else if debounce.Stop() {
					debounce.Reset(250 * time.Millisecond)
				}
			}
		case <-debounceChannel:
			capture()
			debounce = nil
			debounceChannel = nil
		case event := <-configurationEvents:
			if event.Gap || event.Section == History.PlayerSamplesConfigurationSection {
				policy := application.playerSamplesRetentionPolicy()
				ticker.Reset(History.PlayerSamplesRecordingIntervalDuration(policy.RecordingIntervalSeconds))
				compact(false)
				capture()
			}
		case <-ticker.C:
			compact(false)
			capture()
		}
	}
}

func (application *Application) playerSamplesRetentionPolicy() History.PlayerSamplesRetentionPolicy {
	if application == nil {
		return History.ResolvePlayerSamplesRetention(nil, false)
	}
	var raw json.RawMessage
	if application.Configuration != nil {
		raw, _ = application.Configuration.Section(History.PlayerSamplesConfigurationSection)
	}
	return History.ResolvePlayerSamplesRetention(raw, application.BackgroundOnly)
}

func (application *Application) registerCoreIntents() error {
	for name, action := range map[string]Intent.Action{
		"session.start":     ignoreArguments(application.Session.Start),
		"session.stop":      ignoreArguments(application.Session.Stop),
		"session.reconnect": ignoreArguments(application.Session.Reconnect),
		"session.background.prepare": ignoreArguments(func(context.Context) error {
			return application.Session.PrepareBackgroundMode(application.DataDir)
		}),
		"game.ui.close": ignoreArguments(application.Session.CloseGameUI),
		"session.select_browser": func(_ context.Context, arguments json.RawMessage) error {
			preference, err := browserPreference(arguments)
			if err != nil {
				return err
			}
			return application.Session.SelectBrowser(preference)
		},
		"config.update": func(_ context.Context, arguments json.RawMessage) error {
			update, err := decodeConfigurationUpdate(arguments)
			if err != nil {
				return err
			}
			_, err = application.Configuration.UpdateConditional(
				update.Section, update.Value, update.ExpectedRevision, update.ExpectedValue,
			)
			if err == nil && update.Section == "scheduler" {
				application.Session.SetAutomationLocked(application.automationLocked())
			}
			return err
		},
		"game_data.refresh":  ignoreArguments(application.refreshGameData),
		"app.update.check":   ignoreArguments(application.Updates.Check),
		"app.update.install": ignoreArguments(application.Updates.Install),
		"operation.schedule": application.scheduleOperation,
		"operation.cancel":   application.cancelOperation,
	} {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}

	definitions := []Intent.Definition{
		{
			Name: "session.start", Description: "Start the configured game session adapter", Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.start", "session", "Start the game session"),
		},
		{
			Name: "session.reconnect", Description: "Reconnect the game session now, bypassing a scheduled retry, cooldown wait, or login park", Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.reconnect", "session", "Reconnect the game session now"),
		},
		{
			Name: "session.stop", Description: "Stop the active game session", Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.stop", "session", "Stop the game session"),
		},
		{
			Name: "session.background.prepare", Description: "Validate and authorize the protected saved login for Background mode", Effect: Intent.EffectWrite,
			Planner: actionPlanner("session.background.prepare", "session", "Prepare the saved login for Background mode"),
		},
		{
			Name: "session.select_browser", Description: "Select the CDP-capable Chromium browser used for game sessions", Effect: Intent.EffectExternal,
			Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
				preference, err := browserPreference(arguments)
				if err != nil {
					return Intent.Plan{}, err
				}
				candidate, err := Session.ResolveChromiumBrowser(preference, "")
				if err != nil {
					return Intent.Plan{}, err
				}
				selected := candidate.ID
				if strings.HasPrefix(candidate.ID, "custom-") {
					selected = candidate.ExecutablePath
				}
				canonical, _ := json.Marshal(map[string]string{"browser": selected})
				return Intent.Plan{
					Claims: []string{"session"}, Summary: fmt.Sprintf("Use %s for game sessions", candidate.Name),
					Steps: []Intent.Step{{
						Name: "Select browser", Action: "session.select_browser", ActionArguments: canonical,
					}},
				}, nil
			},
		},
		{
			Name: "game.ui.close", Description: "Close dismissible dialogs, panels, attack panels, and contextual menus in the live game", Effect: Intent.EffectExternal,
			Planner: actionPlanner("game.ui.close", "game-ui", "Close the active game UI"),
		},
		{
			Name: "config.update", Description: "Atomically update one versioned user-configuration section", Effect: Intent.EffectWrite,
			Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
				update, err := decodeConfigurationUpdate(arguments)
				if err != nil {
					return Intent.Plan{}, err
				}
				canonical, _ := json.Marshal(update)
				return Intent.Plan{
					Claims:  []string{"configuration:" + update.Section},
					Summary: fmt.Sprintf("Update %s configuration", update.Section),
					Steps: []Intent.Step{{
						Name: "Save configuration", Action: "config.update", ActionArguments: canonical,
					}},
				}, nil
			},
		},
		{
			Name: "game_data.refresh", Description: "Refresh the official versioned game-data snapshot", Effect: Intent.EffectExternal,
			Planner: actionPlanner("game_data.refresh", "game-data", "Refresh official game data"),
		},
		{
			Name: "app.update.check", Description: "Check the trusted CitadelOps release endpoint for a newer application version", Effect: Intent.EffectRead,
			Planner: actionPlanner("app.update.check", "application-update", "Check for a CitadelOps update"),
		},
		{
			Name: "app.update.install", Description: "Download and atomically install the checked platform-specific CitadelOps release", Effect: Intent.EffectExternal,
			Planner: actionPlanner("app.update.install", "application-update", "Install the checked CitadelOps update"),
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func (application *Application) scheduleOperation(_ context.Context, arguments json.RawMessage) error {
	var request Scheduling.Request
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return fmt.Errorf("decode scheduled operation: %w", err)
	}
	return application.Scheduler.Schedule(request)
}

func (application *Application) cancelOperation(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return fmt.Errorf("decode scheduled-operation cancellation: %w", err)
	}
	return application.Scheduler.Cancel(request.ID)
}

func (application *Application) refreshGameData(ctx context.Context) error {
	if application.refreshGameDataAll != nil {
		return application.refreshGameDataAll(ctx)
	}
	return refreshGameDataStore(ctx, application.State, application.GameData)
}

func refreshGameDataStore(ctx context.Context, state *State.Store, gameData *GameData.Manager) error {
	if state == nil || gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	if err := gameData.Refresh(ctx); err != nil {
		return err
	}
	return synchronizeGameDataStore(state, gameData)
}

// SynchronizeGameData applies the current process-owned catalog generation to
// this account without downloading or decoding the catalog again.
func (application *Application) SynchronizeGameData() error {
	if application == nil {
		return fmt.Errorf("application is unavailable")
	}
	return synchronizeGameDataStore(application.State, application.GameData)
}

func synchronizeGameDataStore(state *State.Store, gameData *GameData.Manager) error {
	if state == nil || gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	current, ok := gameData.Current()
	if !ok {
		return fmt.Errorf("official game data did not produce a snapshot")
	}
	version := current.Metadata().ItemVersion
	languageVersion := current.Metadata().LanguageVersion
	_, err := state.ApplyComponents(State.Components(
		State.ComponentCatalog, State.ComponentInventory, State.ComponentCommanders, State.ComponentCastellans,
	), func(gameState *State.GameState) ([]string, bool, error) {
		changed := EquipmentDomain.HydrateState(gameState, current)
		if gameState.CatalogVersion != version || gameState.LanguageVersion != languageVersion {
			gameState.CatalogVersion = version
			gameState.LanguageVersion = languageVersion
			changed = true
		}
		if !changed {
			return nil, false, nil
		}
		return []string{"game-data", "equipment", "gems"}, true, nil
	})
	return err
}

func actionPlanner(action string, claim string, summary string) Intent.Planner {
	return func(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{
			Claims: []string{claim}, Summary: summary,
			Steps: []Intent.Step{{Name: summary, Action: action}},
		}, nil
	}
}

func ignoreArguments(action func(context.Context) error) Intent.Action {
	return func(ctx context.Context, _ json.RawMessage) error {
		return action(ctx)
	}
}

func browserPreference(arguments json.RawMessage) (string, error) {
	var input struct {
		Browser string `json:"browser"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("decode browser selection: %w", err)
	}
	input.Browser = strings.TrimSpace(input.Browser)
	if input.Browser == "" {
		return "", fmt.Errorf("browser is required")
	}
	return input.Browser, nil
}

type configurationUpdate struct {
	Section          string           `json:"section"`
	Value            json.RawMessage  `json:"value"`
	ExpectedRevision *uint64          `json:"expectedRevision,omitempty"`
	ExpectedValue    *json.RawMessage `json:"expectedValue,omitempty"`
}

func decodeConfigurationUpdate(arguments json.RawMessage) (configurationUpdate, error) {
	var input configurationUpdate
	if err := json.Unmarshal(arguments, &input); err != nil {
		return input, fmt.Errorf("decode configuration update: %w", err)
	}
	input.Section = strings.TrimSpace(input.Section)
	if err := Configuration.Validate(input.Section, input.Value); err != nil {
		return input, err
	}
	if input.ExpectedValue != nil {
		if err := Configuration.Validate(input.Section, *input.ExpectedValue); err != nil {
			return input, fmt.Errorf("expected configuration value: %w", err)
		}
	}
	return input, nil
}

func defaultConfiguration() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"scheduler":          json.RawMessage(`{"minAttackDelay":4,"maxAttackDelay":6,"upgradeEreDelayMs":50,"upgradeCoinThreshold":0,"botLocked":false,"attackPriorities":{"autoTowers":50,"autoAdvisor":50,"autoBeriWorld":50,"autoStorm":50,"riftMaiden":50,"riftReplay":50},"featureSchedules":{}}`),
		"session.connection": json.RawMessage(`{"mode":"full"}`),
		"session.reconnect":  json.RawMessage(`{"relogDelaySec":300}`),
		History.PlayerSamplesConfigurationSection:  json.RawMessage(`{"version":1,"retention":"30d"}`),
		Reports.BattleResearchConfigurationSection: json.RawMessage(`{"enabled":false,"consentVersion":0,"spyCount":1}`),
		"automation.enabled":                       json.RawMessage(`{}`),
		"automation.autoEquipmentCleanup":          json.RawMessage(`{"version":1,"checkIntervalSec":60}`),
		"automation.recruitTroops":                 json.RawMessage(`{"version":1,"mode":"global","checkIntervalSec":300,"recruitLevel10OnTitleLoss":false,"globalItems":[],"castles":{}}`),
		"automation.autoBeriWorld":                 json.RawMessage(`{"minTroopsToTransfer":1,"beriCastleId":0,"transferTroopId":0,"sourceCastleId":0,"wireCastleId":-1,"troopSpaceCheckIntervalSec":30,"presetId":"","attackCheckIntervalSec":30,"dailyAttackLimit":0,"horseTravelBoostId":-1,"toolMinimums":{"611":0,"614":0,"620":0},"build":{"enabled":false,"stableLevel":5,"allowPremium":false,"allowDemolition":false,"allowTimeSkips":false,"resourceReserves":{},"timeSkipReserve":{}},"requireActiveGallantryBooster":false,"useTroopTransportTimeSkips":false,"troopTransportTimeSkipId":"MS5"}`),
		"automation.autoBeriWorldBlueprints":       json.RawMessage(`{"version":1,"blueprints":{}}`),
		"automation.commanderFeatures":             json.RawMessage(`{"version":2,"assignments":{},"requirements":{}}`),
		"automation.autoFoodBalance":               json.RawMessage(`{"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1000,"minimumStormShipmentSize":10000,"minimumSourceReserve":1000,"minimumCoinReserve":0,"autoKingdomTransport":true,"useKingdomTimeSkips":false,"allowedTimeSkips":[],"timeSkipReserve":{},"horseTravelBoostId":-1}`),
		"automation.autoTowers":                    json.RawMessage(`{"version":2,"checkIntervalSec":30,"mapRefreshIntervalSec":1800,"dailyAttackLimit":0,"horseTravelBoostId":-1,"castles":{}}`),
		"automation.autoInvasion":                  json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","foreignLordsDifficultyId":0,"bloodcrowDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"fortifyCurrency":"","horseTravelBoostId":-1}`),
		"automation.autoNomad":                     json.RawMessage(`{"version":5,"sourceCastleId":0,"nomadPresetId":"","samuraiPresetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"skipCooldowns":false,"timeSkipReserve":{},"rbcTest":{"enabled":false,"runId":"","targetX":0,"targetY":0},"horseTravelBoostId":-1}`),
		"automation.autoAdvisor":                   json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"maxAttackCount":9999,"minimumRemainingSec":1800,"coinCostPerAttack":500,"minimumCoinReserve":0,"rubyCostPerAttack":0,"minimumRubyReserve":0,"minimumFeatherReserve":0,"timeSkipReserve":{},"checkIntervalSec":30,"mapRefreshIntervalSec":300,"horseTravelBoostId":-1}`),
		"automation.autoBuyer":                     json.RawMessage(`{"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":0,"minimumRubyReserve":0,"allowRubyPackages":false,"packages":[],"specialists":[],"feast":{"enabled":false,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":0,"minimumFoodReserve":0,"allowRubies":false,"maximumRubyCostPerPurchase":0}}`),
		"automation.autoKhan":                      json.RawMessage(`{"version":1,"sourceCastleId":0,"attackPresetId":"","defensePresetId":"","minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,"mapRefreshIntervalSec":30,"dailyAttackLimit":0,"attackLaunchesEnabled":true,"triggerRage":true,"skipCooldowns":true,"timeSkipReserve":{},"openGateProtection":true,"offensiveUnitThreshold":1000,"horseTravelBoostId":-1,"nomadPointThreshold":0,"replenishDefenseTools":false,"maxRageChain":0,"requireActiveRageBooster":false}`),
		"attacks.presets":                          json.RawMessage(`{"version":1,"presets":[]}`),
		"defense.presets":                          json.RawMessage(`{"version":1,"presets":[]}`),
		"automation.autoStorm":                     json.RawMessage(`{"version":1,"unlock":{"enabled":false,"prebuiltCastleId":0},"decorationPresetCastleId":0,"decorationPresetId":"","build":{"allowPremium":false,"allowDemolition":false,"allowResourceTransport":true,"allowTimeSkips":false,"resourceReserves":{},"sourceResourceReserves":{},"timeSkipReserve":{}},"harbor":{"enabled":false,"targetLevel":1},"forts":{"enabled":false,"levels":[40,50,60,70,80],"minimumWins":0,"presetId":""},"islands":{"enabled":false,"resources":["wood","stone","aquamarine"],"sizes":["large","small"],"presetId":"","defenseUnits":[]},"troopImport":{"enabled":false,"donorCastleIds":[],"minimumTroops":0,"historyHours":24},"aquamarine":{"reserve":0,"shopTableId":0,"purchases":[]},"targetPriority":["fort:80","fort:70","fort:60","fort:50","fort:40","island:large","island:small"],"checkIntervalSec":30,"mapRefreshIntervalSec":7200,"dailyAttackLimit":0,"horseTravelBoostId":-1}`),
		"automation.autoStormBlueprints":           json.RawMessage(`{"version":1,"blueprints":{}}`),
		"rift.attackPreferences":                   json.RawMessage(`{"version":1,"replayHorseTravelBoostId":-1,"maidenHorseTravelBoostId":-1}`),
		RiftTemplates.ConfigurationSection:         json.RawMessage(`{"version":1,"launches":{},"deletedLaunchIds":{}}`),
	}
}

// DefaultConfigurationSections returns an isolated copy of the runtime
// defaults. Hosted orchestration uses it as the base for sparse account-owned
// overrides so omitted sections reset deterministically instead of inheriting
// values from an older local profile.
func DefaultConfigurationSections() map[string]json.RawMessage {
	defaults := defaultConfiguration()
	cloned := make(map[string]json.RawMessage, len(defaults))
	for section, value := range defaults {
		cloned[section] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
