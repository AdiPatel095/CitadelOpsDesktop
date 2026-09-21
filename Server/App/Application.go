package App

import (
	"CitadelDesktop/Server/Localization"
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
	coinGate         *coinDispatchGate

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
	event  State.Event
	result chan error
}

func New(ctx context.Context, config Config) (*Application, error) {
	if config.DataDir == "" {
		return nil, Localization.WithError(fmt.Errorf("application data directory is required"), Localization.New("server.app.application_data_directory_is.85ca97b1", "application data directory is required", nil))
	}
	if !config.BackgroundOnly &&
		((config.PrivateMetricsClient != nil && config.PrivateMetricsClient.Enabled()) || config.PrivateMetricsPlacement != nil) {
		return nil, Localization.WithError(fmt.Errorf("private metrics publishing requires hosted background mode"), Localization.New("server.app.private_metrics_publishing_requires.de36212f", "private metrics publishing requires hosted background mode", nil))
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
	if err := removeRetiredBattleResearchConfiguration(configuration); err != nil {
		return nil, Localization.WithError(fmt.Errorf("remove retired Experimental Battle Research settings: %w", err), Localization.ErrorContext(Localization.New("server.app.remove_retired_experimental_battle.47320b39", "remove retired Experimental Battle Research settings", nil), err))
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
		startupErr = errors.Join(startupErr, Localization.WithError(fmt.Errorf("shared official game data is not initialized"), Localization.New("server.app.shared_official_game_data.0942a417", "shared official game data is not initialized", nil)))
	}

	initial := State.NewGameState()
	if recovered, recoveryErr := State.LoadSnapshot(config.DataDir); recoveryErr == nil {
		initial = recovered
	} else if !os.IsNotExist(recoveryErr) {
		// An unreadable profile may contain an active safety lock. Never start
		// automation from empty state and overwrite that evidence.
		return nil, Localization.WithError(fmt.Errorf("recover durable account state: %w", recoveryErr), Localization.ErrorContext(Localization.New("server.app.recover_durable_account_state.1eb09389", "recover durable account state", nil), recoveryErr))
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
			return nil, Localization.WithError(fmt.Errorf("register protocol reducers: %w", err), Localization.ErrorContext(Localization.New("server.app.register_protocol_reducers.803a416c", "register protocol reducers", nil), err))
		}
	}
	ingest := Ingest.NewPipeline(state, gameData, registry)
	ingest.SetProfileID(profileLease.ProfileID)
	telemetry := Telemetry.NewStore(5000)
	if telemetryErr := telemetry.SetDataDir(config.DataDir); telemetryErr != nil {
		startupErr = errors.Join(startupErr, Localization.WithError(fmt.Errorf("initialize logger: %w", telemetryErr), Localization.ErrorContext(Localization.New("server.app.initialize_logger.453e61ce", "initialize logger", nil), telemetryErr)))
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
		return nil, Localization.WithError(fmt.Errorf("open intent operation store: %w", err), Localization.ErrorContext(Localization.New("server.app.open_intent_operation_store.1034b32d", "open intent operation store", nil), err))
	}
	closeOperationStore := true
	defer func() {
		if closeOperationStore {
			_ = operationStore.Close()
		}
	}()
	if err := intents.SetOperationStore(ctx, operationStore); err != nil {
		return nil, Localization.WithError(fmt.Errorf("recover intent operations: %w", err), Localization.ErrorContext(Localization.New("server.app.recover_intent_operations.9d357f70", "recover intent operations", nil), err))
	}
	reportStore, err := Reports.OpenSQLiteStore(config.DataDir)
	if err != nil {
		return nil, Localization.WithError(fmt.Errorf("open report analytics store: %w", err), Localization.ErrorContext(Localization.New("server.app.open_report_analytics_store.d17d5d65", "open report analytics store", nil), err))
	}
	closeReportStore := true
	defer func() {
		if closeReportStore {
			_ = reportStore.Close()
		}
	}()
	if err := Reports.BackfillBattleHistory(ctx, history, reportStore, initial); err != nil {
		return nil, Localization.WithError(fmt.Errorf("backfill report analytics: %w", err), Localization.ErrorContext(Localization.New("server.app.backfill_report_analytics.5f079728", "backfill report analytics", nil), err))
	}
	if _, err := Reports.CompactBattleHistory(history); err != nil {
		return nil, Localization.WithError(fmt.Errorf("compact local battle report outbox: %w", err), Localization.ErrorContext(Localization.New("server.app.compact_local_battle_report.d6c15e1c", "compact local battle report outbox", nil), err))
	}
	if _, err := Reports.BackfillCloudOutbox(ctx, history, reportStore, initial); err != nil {
		return nil, Localization.WithError(fmt.Errorf("backfill cloud battle report outbox: %w", err), Localization.ErrorContext(Localization.New("server.app.backfill_cloud_battle_report.38509104", "backfill cloud battle report outbox", nil), err))
	}
	if err := restoreRecentAutoStormLaunchHistory(ctx, state, reportStore); err != nil {
		return nil, Localization.WithError(fmt.Errorf("restore recent Auto Storm launch history: %w", err), Localization.ErrorContext(Localization.New("server.app.restore_recent_auto_storm.4c619011", "restore recent Auto Storm launch history", nil), err))
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
			return nil, Localization.WithError(fmt.Errorf("initialize private metrics publisher: %w", err), Localization.ErrorContext(Localization.New("server.app.initialize_private_metrics_publisher.b0796171", "initialize private metrics publisher", nil), err))
		}
		if config.PrivateMetricsClient.CheckpointsEnabled() {
			checkpointPublisher, err = PrivateMetrics.NewCheckpointPublisher(PrivateMetrics.CheckpointPublisherConfig{
				RuntimeID: strings.TrimSpace(config.AccountKey), State: state, Configuration: configuration,
				Intents: intents, Client: config.PrivateMetricsClient, Placement: config.PrivateMetricsPlacement,
			})
			if err != nil {
				return nil, Localization.WithError(fmt.Errorf("initialize dashboard checkpoint publisher: %w", err), Localization.ErrorContext(Localization.New("server.app.initialize_dashboard_checkpoint_publisher.db9f6578", "initialize dashboard checkpoint publisher", nil), err))
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
		coinGate:             newCoinDispatchGate(),
	}
	ingest.SetDurabilityFence(application.saveStateEvent)
	session.SetAttackDelayProvider(application.attackLaunchDelay)
	if relogTransport, ok := transport.(Session.RelogDelayTransport); ok {
		relogTransport.SetRelogDelayProvider(application.relogDelay)
	}
	session.SetAutomationLocked(application.automationLocked())
	intents.SetExecutionGate(application.executionGate)
	intents.SetAdmissionWeightProvider(application.attackAdmissionWeight)
	intents.SetFinalDispatchProvider(application.coinGate)
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
	application.Intents.SetLaneSafetyPersistence(application.saveStateEvent)
	if err := application.Intents.RefreshAutomationLaneLocks(); err != nil {
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
		Automation.NewInvasionRecoveryPolicy(),
		Automation.NewAutoFortressPolicy(),
		Automation.NewAutoInvasionPolicy(),
		Automation.NewAutoNomadPolicy(),
		Automation.NewAutoAdvisorPolicy(),
		Automation.NewAutoBoosterPolicy(),
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
	application.Automation.SetTelemetry(telemetry)
	application.Automation.SetExternalConfigurationAuthority(config.BackgroundOnly)
	application.Reports = Reports.NewManagerWithCloudClient(
		state, history, intents, config.ReportsCloudClient, reportStore,
	)
	// Historical Experimental Battle Research trial rows remain in report
	// storage for compatibility, but no runtime or status API is composed.
	application.API = API.NewServer(API.Config{
		Version: Version, BuildRevision: BuildRevision, BuildID: BuildID,
		State: state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Intents: intents, ReportAnalytics: reportStore, Session: session, Updates: application.Updates, Diagnostics: application.Diagnostics,
		CloudReports:    application.Reports.CloudClient(),
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
	configurationEvents, unsubscribeConfiguration := application.Configuration.Subscribe(8)
	application.Session.SetAutomationLocked(application.automationLocked())
	go application.syncAutomationLock(ctx, configurationEvents, unsubscribeConfiguration)
	persistenceReady := make(chan struct{})
	go application.persistState(ctx, persistenceReady)
	<-persistenceReady
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
	go application.runMovementClock(ctx)
	go application.Automation.Run(ctx)
	go application.Reports.Run(ctx)
	go application.Scheduler.Run(ctx)
}

func (application *Application) syncAutomationLock(ctx context.Context, events <-chan Configuration.Event, unsubscribe func()) {
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.Gap || event.Section == "scheduler" || event.Section == "*" {
				application.Session.SetAutomationLocked(application.automationLocked())
			}
		}
	}
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
		return Localization.WithError(fmt.Errorf("private metrics publisher is unavailable"), Localization.New("server.app.private_metrics_publisher_is.ccdc1d0d", "private metrics publisher is unavailable", nil))
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
		application.Telemetry.RecordFeatureActivityMessage(
			receipt.Actor, receipt.Intent, activity.severity, activity.event, activity.detail, activity.descriptor,
		)
	}
}

func (application *Application) persistState(ctx context.Context, ready chan<- struct{}) {
	defer close(application.statePersistenceDone)
	events, unsubscribe := application.State.Subscribe(128)
	defer unsubscribe()
	subscriptionBaseline := application.State.Revision()
	close(ready)
	writer := State.NewComponentSnapshotWriter(application.DataDir)
	var timer *time.Timer
	var timerChannel <-chan time.Time
	var pending State.PersistenceBatch
	coveredRevision := subscriptionBaseline
	var persistedCoverage uint64
	accumulate := func(event State.Event) bool {
		if !pending.Accumulate(event) {
			return false
		}
		if timer == nil {
			timer = time.NewTimer(2 * time.Second)
			timerChannel = timer.C
		}
		return true
	}
	observe := func(event State.Event) {
		accumulate(event)
		if event.Revision > coveredRevision {
			coveredRevision = event.Revision
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
			if coverage := min(coveredRevision, revision); coverage > persistedCoverage {
				persistedCoverage = coverage
			}
			if timer != nil {
				timer.Stop()
				timer = nil
				timerChannel = nil
			}
		}
		return err
	}
	force := func(request statePersistenceRequest) {
		if request.event.Revision == 0 {
			request.result <- nil
			return
		}
		if request.event.Revision <= subscriptionBaseline {
			// Defensive fallback for an event produced before this subscriber was
			// installed. Production keeps request-mode persistence disabled until
			// the readiness handshake, but persisting the event's cumulative
			// generation as a full snapshot keeps this path lossless as well.
			full := request.event
			full.Components = State.AllComponents.List()
			if !accumulate(full) {
				request.result <- nil
				return
			}
		} else {
			// The State subscription is ordered and coalescing preserves every
			// dirty component/key. Drain through the requested revision before
			// acknowledging it; a later sparse event is not by itself proof that an
			// earlier invasion patch reached disk.
			for coveredRevision < request.event.Revision {
				select {
				case event := <-events:
					observe(event)
				case <-ctx.Done():
					request.result <- ctx.Err()
					return
				}
			}
		}
		if persistedCoverage >= request.event.Revision {
			request.result <- nil
			return
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
			observe(event)
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
	request := statePersistenceRequest{event: event, result: make(chan error, 1)}
	select {
	case application.statePersistence <- request:
	case <-application.statePersistenceDone:
		return Localization.WithError(fmt.Errorf("state persistence worker is stopped"), Localization.New("server.app.state_persistence_worker_is.68887613", "state persistence worker is stopped", nil))
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.result:
		return err
	case <-application.statePersistenceDone:
		return Localization.WithError(fmt.Errorf("state persistence worker stopped before revision %d was durable", event.Revision), Localization.New("server.app.state_persistence_worker_stopped.4e353ae5", "state persistence worker stopped before revision {p0} was durable", Localization.Params{"p0": event.Revision}))
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
		reportErr = Localization.WithError(fmt.Errorf("report analytics persistence: %w", reportErr), Localization.ErrorContext(Localization.New("server.app.report_analytics_persistence.bc9d3420", "report analytics persistence", nil), reportErr))
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
		stateErr = Localization.WithError(fmt.Errorf("state snapshot persistence: %w", stateErr), Localization.ErrorContext(Localization.New("server.app.state_snapshot_persistence.d210ebcb", "state snapshot persistence", nil), stateErr))
	}
	if operationErr != nil {
		operationErr = Localization.WithError(fmt.Errorf("operation journal persistence: %w", operationErr), Localization.ErrorContext(Localization.New("server.app.operation_journal_persistence.a22b1ecd", "operation journal persistence", nil), operationErr))
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
		"automation.safety.clear": application.clearAutomationSafetyLock,
		"support.batch.guard":     application.guardSupportBatch,
		"session.start":           ignoreArguments(application.Session.Start),
		"session.stop":            ignoreArguments(application.Session.Stop),
		"session.reconnect":       ignoreArguments(application.Session.Reconnect),
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
			Name: "automation.safety.clear", Description: "Clear a reviewed lane safety incident", DescriptionDescriptor: Localization.New("server.intent.description.fd4caae5", "Clear a reviewed lane safety incident", nil), Effect: Intent.EffectWrite,
			Planner: actionPlanner("automation.safety.clear", "automation-safety", "Clear reviewed lane safety lock", Localization.New("server.app.clear_reviewed_lane_safety.ab5b3bd7", "Clear reviewed lane safety lock", nil)),
		},
		{
			Name: "session.start", Description: "Start the configured game session adapter", DescriptionDescriptor: Localization.New("server.intent.description.cc692397", "Start the configured game session adapter", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.start", "session", "Start the game session", Localization.New("server.app.start_the_game_session.29f0589e", "Start the game session", nil)),
		},
		{
			Name: "session.reconnect", Description: "Reconnect the game session now, bypassing a scheduled retry, cooldown wait, or login park", DescriptionDescriptor: Localization.New("server.intent.description.18923c33", "Reconnect the game session now, bypassing a scheduled retry, cooldown wait, or login park", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.reconnect", "session", "Reconnect the game session now", Localization.New("server.app.reconnect_the_game_session.19b7c5b5", "Reconnect the game session now", nil)),
		},
		{
			Name: "session.stop", Description: "Stop the active game session", DescriptionDescriptor: Localization.New("server.intent.description.6532e53e", "Stop the active game session", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.stop", "session", "Stop the game session", Localization.New("server.app.stop_the_game_session.e5c7c3b2", "Stop the game session", nil)),
		},
		{
			Name: "session.background.prepare", Description: "Validate and authorize the protected saved login for Background mode", DescriptionDescriptor: Localization.New("server.intent.description.21a5d0c3", "Validate and authorize the protected saved login for Background mode", nil), Effect: Intent.EffectWrite,
			Planner: actionPlanner("session.background.prepare", "session", "Prepare the saved login for Background mode", Localization.New("server.app.prepare_the_saved_login.110e36e6", "Prepare the saved login for Background mode", nil)),
		},
		{
			Name: "session.select_browser", Description: "Select the CDP-capable Chromium browser used for game sessions", DescriptionDescriptor: Localization.New("server.intent.description.1d805c26", "Select the CDP-capable Chromium browser used for game sessions", nil), Effect: Intent.EffectExternal,
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
					Claims: []string{"session"}, Summary: fmt.Sprintf("Use %s for game sessions", candidate.Name), SummaryDescriptor: Localization.New("server.app.use_p_for_game.fc0d9888", "Use {p0} for game sessions", Localization.Params{"p0": fmt.Sprintf("%s", candidate.Name)}),
					Steps: []Intent.Step{{
						Name: "Select browser", Action: "session.select_browser", ActionArguments: canonical,
					}},
				}, nil
			},
		},
		{
			Name: "game.ui.close", Description: "Close dismissible dialogs, panels, attack panels, and contextual menus in the live game", DescriptionDescriptor: Localization.New("server.intent.description.09094736", "Close dismissible dialogs, panels, attack panels, and contextual menus in the live game", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("game.ui.close", "game-ui", "Close the active game UI", Localization.New("server.app.close_the_active_game.09e0984e", "Close the active game UI", nil)),
		},
		{
			Name: "config.update", Description: "Atomically update one versioned user-configuration section", DescriptionDescriptor: Localization.New("server.intent.description.e0f16148", "Atomically update one versioned user-configuration section", nil), Effect: Intent.EffectWrite,
			Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
				update, err := decodeConfigurationUpdate(arguments)
				if err != nil {
					return Intent.Plan{}, err
				}
				canonical, _ := json.Marshal(update)
				return Intent.Plan{
					Claims:  []string{"configuration:" + update.Section},
					Summary: fmt.Sprintf("Update %s configuration", update.Section), SummaryDescriptor: Localization.New("server.app.update_p_configuration.94613bbf", "Update {p0} configuration", Localization.Params{"p0": fmt.Sprintf("%s", update.Section)}),
					Steps: []Intent.Step{{
						Name: "Save configuration", Action: "config.update", ActionArguments: canonical,
					}},
				}, nil
			},
		},
		{
			Name: "game_data.refresh", Description: "Refresh the official versioned game-data snapshot", DescriptionDescriptor: Localization.New("server.intent.description.cec773aa", "Refresh the official versioned game-data snapshot", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("game_data.refresh", "game-data", "Refresh official game data", Localization.New("server.app.refresh_official_game_data.08d8de5a", "Refresh official game data", nil)),
		},
		{
			Name: "app.update.check", Description: "Check the trusted CitadelOps release endpoint for a newer application version", DescriptionDescriptor: Localization.New("server.intent.description.53237709", "Check the trusted CitadelOps release endpoint for a newer application version", nil), Effect: Intent.EffectRead,
			Planner: actionPlanner("app.update.check", "application-update", "Check for a CitadelOps update", Localization.New("server.app.check_for_a_citadelops.0e65e959", "Check for a CitadelOps update", nil)),
		},
		{
			Name: "app.update.install", Description: "Download and atomically install the checked platform-specific CitadelOps release", DescriptionDescriptor: Localization.New("server.intent.description.d8af18aa", "Download and atomically install the checked platform-specific CitadelOps release", nil), Effect: Intent.EffectExternal,
			Planner: actionPlanner("app.update.install", "application-update", "Install the checked CitadelOps update", Localization.New("server.app.install_the_checked_citadelops.91ec953f", "Install the checked CitadelOps update", nil)),
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
		return Localization.WithError(fmt.Errorf("decode scheduled operation: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_scheduled_operation.5b452ece", "decode scheduled operation", nil), err))
	}
	return application.Scheduler.Schedule(request)
}

func (application *Application) cancelOperation(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Localization.WithError(fmt.Errorf("decode scheduled-operation cancellation: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_scheduled_operation_cancellation.f2f63ac8", "decode scheduled-operation cancellation", nil), err))
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
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
		return Localization.WithError(fmt.Errorf("application is unavailable"), Localization.New("server.app.application_is_unavailable.eed006de", "application is unavailable", nil))
	}
	return synchronizeGameDataStore(application.State, application.GameData)
}

func synchronizeGameDataStore(state *State.Store, gameData *GameData.Manager) error {
	if state == nil || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	current, ok := gameData.Current()
	if !ok {
		return Localization.WithError(fmt.Errorf("official game data did not produce a snapshot"), Localization.New("server.app.official_game_data_did.bc1853fe", "official game data did not produce a snapshot", nil))
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

func actionPlanner(action string, claim string, summary string, descriptors ...*Localization.Message) Intent.Planner {
	return func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{
			Claims: []string{claim}, Summary: summary, SummaryDescriptor: Localization.First(descriptors),
			Steps: []Intent.Step{{
				Name: summary, Action: action,
				ActionArguments: append(json.RawMessage(nil), arguments...),
			}},
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
		return "", Localization.WithError(fmt.Errorf("decode browser selection: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_browser_selection.5cd1812d", "decode browser selection", nil), err))
	}
	input.Browser = strings.TrimSpace(input.Browser)
	if input.Browser == "" {
		return "", Localization.WithError(fmt.Errorf("browser is required"), Localization.New("server.app.browser_is_required.24d5c322", "browser is required", nil))
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
		return input, Localization.WithError(fmt.Errorf("decode configuration update: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_configuration_update.5b6a4490", "decode configuration update", nil), err))
	}
	input.Section = strings.TrimSpace(input.Section)
	if input.Section == Reports.BattleResearchConfigurationSection {
		return input, Localization.WithError(fmt.Errorf("Experimental Battle Research settings have been removed"), Localization.New("server.app.experimental_battle_research_settings.878d9340", "Experimental Battle Research settings have been removed", nil))
	}
	if err := Configuration.Validate(input.Section, input.Value); err != nil {
		return input, err
	}
	if input.ExpectedValue != nil {
		if err := Configuration.Validate(input.Section, *input.ExpectedValue); err != nil {
			return input, Localization.WithError(fmt.Errorf("expected configuration value: %w", err), Localization.ErrorContext(Localization.New("server.app.expected_configuration_value.c71676bc", "expected configuration value", nil), err))
		}
	}
	return input, nil
}

func removeRetiredBattleResearchConfiguration(configuration *Configuration.Store) error {
	if configuration == nil {
		return nil
	}
	snapshot := configuration.Snapshot()
	if _, exists := snapshot.Sections[Reports.BattleResearchConfigurationSection]; !exists {
		return nil
	}
	delete(snapshot.Sections, Reports.BattleResearchConfigurationSection)
	_, _, err := configuration.ReplaceAllAuthoritative(snapshot.Sections)
	return err
}

func defaultConfiguration() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"scheduler":          json.RawMessage(`{"minAttackDelay":4,"maxAttackDelay":6,"upgradeEreDelayMs":50,"upgradeCoinThreshold":0,"botLocked":false,"attackPriorities":{"autoFortress":60,"autoTowers":50,"autoAdvisor":50,"autoBeriWorld":50,"autoStorm":50,"riftMaiden":50,"riftReplay":50},"featureSchedules":{}}`),
		"session.connection": json.RawMessage(`{"mode":"full"}`),
		"session.reconnect":  json.RawMessage(`{"relogDelaySec":300}`),
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
		"automation.enabled":                      json.RawMessage(`{}`),
		"automation.autoEquipmentCleanup":         json.RawMessage(`{"version":1,"checkIntervalSec":60}`),
		"automation.recruitTroops":                json.RawMessage(`{"version":1,"mode":"global","checkIntervalSec":300,"recruitLevel10OnTitleLoss":false,"globalItems":[],"castles":{}}`),
		"automation.autoBeriWorld":                json.RawMessage(`{"minTroopsToTransfer":1,"beriCastleId":0,"transferTroopId":0,"sourceCastleId":0,"wireCastleId":-1,"troopSpaceCheckIntervalSec":30,"presetId":"","attackCheckIntervalSec":30,"dailyAttackLimit":0,"horseTravelBoostId":-1,"toolMinimums":{"611":0,"614":0,"620":0},"build":{"enabled":false,"stableLevel":5,"allowPremium":false,"allowDemolition":false,"allowTimeSkips":false,"resourceReserves":{},"timeSkipReserve":{}},"requireActiveGallantryBooster":false,"useTroopTransportTimeSkips":false,"troopTransportTimeSkipId":"MS5"}`),
		"automation.autoBeriWorldBlueprints":      json.RawMessage(`{"version":1,"blueprints":{}}`),
		"automation.commanderFeatures":            json.RawMessage(`{"version":2,"assignments":{},"requirements":{}}`),
		"automation.autoFoodBalance":              json.RawMessage(`{"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1000,"minimumStormShipmentSize":10000,"minimumSourceReserve":1000,"minimumCoinReserve":0,"autoKingdomTransport":true,"useKingdomTimeSkips":false,"allowedTimeSkips":[],"timeSkipReserve":{},"horseTravelBoostId":-1}`),
		"automation.autoBird":                     json.RawMessage(`{"version":2,"activePresetId":null,"ignoreSettings":{"settings":{},"minDelay":6,"maxDelay":12,"minSend":0,"minRPTDays":3},"presets":{"version":1,"lastSelectedPresetId":null,"presets":[]}}`),
		"automation.autoTowers":                   json.RawMessage(`{"version":4,"checkIntervalSec":30,"mapRefreshIntervalSec":1800,"dailyAttackLimit":0,"horseTravelBoostId":-1,"useAdvisor":false,"autoActivateAdvisor":false,"maximumDailyTimeSkips":0,"castles":{}}`),
		"automation.autoFortress":                 json.RawMessage(`{"version":1,"checkIntervalSec":5,"mapRefreshIntervalSec":1800,"dailyAttackLimit":0,"horseTravelBoostId":1009,"minimumCommanderSpeedBonus":100,"direwolfPurchaseLimit":0,"minimumTabletReserve":0,"useTimeSkips":false,"timeSkipReserve":{},"kingdoms":{"1":{"enabled":false},"2":{"enabled":false},"3":{"enabled":false}}}`),
		"automation.autoInvasion":                 json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","foreignLordsDifficultyId":0,"bloodcrowDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"fortifyCurrency":"","horseTravelBoostId":-1}`),
		"automation.autoNomad":                    json.RawMessage(`{"version":5,"sourceCastleId":0,"nomadPresetId":"","samuraiPresetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"skipCooldowns":false,"timeSkipReserve":{},"rbcTest":{"enabled":false,"runId":"","targetX":0,"targetY":0},"horseTravelBoostId":-1}`),
		"automation.autoAdvisor":                  json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"maxAttackCount":9999,"minimumRemainingSec":1800,"coinCostPerAttack":500,"minimumCoinReserve":0,"rubyCostPerAttack":0,"minimumRubyReserve":0,"minimumFeatherReserve":0,"timeSkipReserve":{},"checkIntervalSec":30,"mapRefreshIntervalSec":300,"horseTravelBoostId":-1}`),
		"automation.autoBooster":                  json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
		"automation.autoBuyer":                    json.RawMessage(`{"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":0,"minimumRubyReserve":0,"allowRubyPackages":false,"packages":[],"specialists":[],"feast":{"enabled":false,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":0,"minimumFoodReserve":0,"allowRubies":false,"maximumRubyCostPerPurchase":0}}`),
		"automation.autoKhan":                     json.RawMessage(`{"version":1,"sourceCastleId":0,"attackPresetId":"","defensePresetId":"","minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,"mapRefreshIntervalSec":30,"dailyAttackLimit":0,"attackLaunchesEnabled":true,"triggerRage":true,"skipCooldowns":true,"timeSkipReserve":{},"openGateProtection":true,"offensiveUnitThreshold":1000,"horseTravelBoostId":-1,"nomadPointThreshold":0,"replenishDefenseTools":false,"maxRageChain":0,"requireActiveRageBooster":false}`),
		"attacks.presets":                         json.RawMessage(`{"version":1,"presets":[]}`),
		"defense.presets":                         json.RawMessage(`{"version":1,"presets":[]}`),
		"automation.autoStorm":                    json.RawMessage(`{"version":1,"unlock":{"enabled":false,"prebuiltCastleId":0},"decorationPresetCastleId":0,"decorationPresetId":"","build":{"allowPremium":false,"allowDemolition":false,"allowResourceTransport":true,"allowTimeSkips":false,"resourceReserves":{},"sourceResourceReserves":{},"timeSkipReserve":{}},"harbor":{"enabled":false,"targetLevel":1},"forts":{"enabled":false,"levels":[40,50,60,70,80],"minimumWins":0,"presetId":""},"islands":{"enabled":false,"resources":["wood","stone","aquamarine"],"sizes":["large","small"],"presetId":"","defenseUnits":[]},"troopImport":{"enabled":false,"donorCastleIds":[],"minimumTroops":0,"historyHours":24},"aquamarine":{"reserve":0,"shopTableId":0,"purchases":[]},"targetPriority":["fort:80","fort:70","fort:60","fort:50","fort:40","island:large","island:small"],"checkIntervalSec":30,"mapRefreshIntervalSec":7200,"dailyAttackLimit":0,"horseTravelBoostId":-1}`),
		"automation.autoStormBlueprints":          json.RawMessage(`{"version":1,"blueprints":{}}`),
		"rift.attackPreferences":                  json.RawMessage(`{"version":1,"replayHorseTravelBoostId":-1,"maidenHorseTravelBoostId":-1}`),
		RiftTemplates.ConfigurationSection:        json.RawMessage(`{"version":1,"launches":{},"deletedLaunchIds":{}}`),
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
