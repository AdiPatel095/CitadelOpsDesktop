package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	"CitadelDesktop/Server/Reports"
	RuntimeKernel "CitadelDesktop/Server/Runtime"
	"CitadelDesktop/Server/Scheduling"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

const GameDataRefreshInterval = 6 * time.Hour

type Config struct {
	DataDir                string
	Offline                bool
	Transport              Session.Transport
	RuntimeContext         context.Context
	UpdateEndpoint         string
	UpdateInstallSupported bool
}

type Application struct {
	DataDir        string
	State          *State.Store
	GameData       *GameData.Manager
	Configuration  *Configuration.Store
	History        *History.Store
	Telemetry      *Telemetry.Store
	Ingest         *Ingest.Pipeline
	Session        *Session.Controller
	Intents        *Intent.Engine
	OperationStore *Intent.SQLiteOperationStore
	ProfileLease   *RuntimeKernel.ProfileLease
	Automation     *Automation.Coordinator
	Reports        *Reports.Manager
	ReportStore    *Reports.SQLiteStore
	Scheduler      *Scheduling.Scheduler
	API            *API.Server
	Updates        *AppUpdate.Manager
	Diagnostics    *Diagnostics.Monitor
	StartupErr     error

	statePersistenceMu  sync.Mutex
	persistenceHealthMu sync.RWMutex
	statePersistenceErr error
	startOnce           sync.Once
}

func New(ctx context.Context, config Config) (*Application, error) {
	if config.DataDir == "" {
		return nil, fmt.Errorf("application data directory is required")
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
	gameData := GameData.NewManager(GameData.UpdaterConfig{
		CacheDir: filepath.Join(config.DataDir, "GameData", "Items"),
	})
	var startupErr error
	if config.Offline {
		startupErr = gameData.LoadCache()
	} else {
		startupErr = gameData.Initialize(ctx)
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
	state := State.NewStore(initial)
	if migrationErr := Reports.MigrateLegacyHistory(config.DataDir, history, initial.Player.ID); migrationErr != nil {
		startupErr = errors.Join(startupErr, migrationErr)
	}
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		return nil, fmt.Errorf("register protocol reducers: %w", err)
	}
	ingest := Ingest.NewPipeline(state, gameData, registry)
	ingest.SetProfileID(profileLease.ProfileID)
	telemetry := Telemetry.NewStore(5000)
	if telemetryErr := telemetry.SetDataDir(config.DataDir); telemetryErr != nil {
		startupErr = errors.Join(startupErr, fmt.Errorf("initialize logger: %w", telemetryErr))
	}
	ingest.SetTelemetry(telemetry)
	transport := config.Transport
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
	application := &Application{
		DataDir: config.DataDir,
		State:   state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Ingest: ingest, Session: session, Intents: intents, OperationStore: operationStore, ReportStore: reportStore,
		ProfileLease: profileLease, StartupErr: startupErr,
		Updates: AppUpdate.NewManager(AppUpdate.Config{
			CurrentVersion: Version, Endpoint: config.UpdateEndpoint,
			InstallSupported: config.UpdateInstallSupported,
		}),
		Diagnostics: Diagnostics.NewMonitor(config.DataDir),
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
		Automation.NewRecruitPolicy(),
		Automation.NewToolPolicy(),
		Automation.NewHospitalPolicy(),
		Automation.NewConstructionPolicy(),
		Automation.NewCraftingPolicy(),
		Automation.NewCraftingLogisticsPolicy(),
		Automation.NewAutoBirdPolicy(),
		Automation.NewAutoStationPolicy(),
		Automation.NewBeriPolicy(),
		Automation.NewFoodBalancePolicy(),
		Automation.NewAutoTowerPolicy(),
		Automation.NewAutoInvasionPolicy(),
		Automation.NewAutoNomadPolicy(),
		Automation.NewAutoAdvisorPolicy(),
		Automation.NewAutoKhanPolicy(),
		Automation.NewAutoKhanCooldownPolicy(),
		Automation.NewAutoKhanRagePolicy(),
		Automation.NewAutoKhanDefensePolicy(),
		Automation.NewAutoStormPolicy(),
		Automation.NewAutoStormBuildPolicy(),
	)
	application.Reports = Reports.NewManager(state, history, intents, reportStore)
	application.API = API.NewServer(API.Config{
		Version: Version, State: state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Intents: intents, ReportAnalytics: reportStore, Session: session, Updates: application.Updates, Diagnostics: application.Diagnostics,
		CloudReports: application.Reports.CloudClient(), Persistence: application,
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
	go application.captureIntentLogs(ctx)
	go func() {
		<-ctx.Done()
		application.Telemetry.Close()
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
	go application.Updates.Run(ctx)
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
	go application.capturePlayerHistory(ctx)
	go application.persistState(ctx)
	go application.runMovementClock(ctx)
	go application.Automation.Run(ctx)
	go application.Reports.Run(ctx)
	go application.Scheduler.Run(ctx)
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
	events, unsubscribe := application.State.Subscribe(128)
	defer unsubscribe()
	var timer *time.Timer
	var timerChannel <-chan time.Time
	flush := func() bool {
		return application.saveStateSnapshot() == nil
	}
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			flush()
			return
		case <-events:
			if timer == nil {
				timer = time.NewTimer(2 * time.Second)
				timerChannel = timer.C
			} else if timer.Stop() {
				timer.Reset(2 * time.Second)
			}
		case <-timerChannel:
			if flush() {
				timer = nil
				timerChannel = nil
			} else {
				timer = time.NewTimer(2 * time.Second)
				timerChannel = timer.C
			}
		}
	}
}

func (application *Application) saveStateSnapshot() error {
	if application == nil || application.State == nil || strings.TrimSpace(application.DataDir) == "" {
		return nil
	}
	application.statePersistenceMu.Lock()
	defer application.statePersistenceMu.Unlock()
	err := State.SaveSnapshot(application.DataDir, application.State.ReadOnlyView())
	application.persistenceHealthMu.Lock()
	application.statePersistenceErr = err
	application.persistenceHealthMu.Unlock()
	return err
}

func (application *Application) PersistenceError() error {
	if application == nil {
		return nil
	}
	application.persistenceHealthMu.RLock()
	stateErr := application.statePersistenceErr
	application.persistenceHealthMu.RUnlock()
	var operationErr error
	var reportErr error
	if application.Intents != nil {
		operationErr = application.Intents.PersistenceError()
	}
	if application.ReportStore != nil {
		reportErr = application.ReportStore.LastError()
	}
	if stateErr != nil {
		stateErr = fmt.Errorf("state snapshot persistence: %w", stateErr)
	}
	if operationErr != nil {
		operationErr = fmt.Errorf("operation journal persistence: %w", operationErr)
	}
	if reportErr != nil {
		reportErr = fmt.Errorf("report analytics persistence: %w", reportErr)
	}
	return errors.Join(stateErr, operationErr, reportErr)
}

func (application *Application) capturePlayerHistory(ctx context.Context) {
	events, unsubscribe := application.State.Subscribe(32)
	defer unsubscribe()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	lastCaptured := time.Time{}
	capture := func() {
		snapshot := application.State.Snapshot()
		if snapshot.Player.ID == 0 || time.Since(lastCaptured) < 55*time.Second {
			return
		}
		if application.History.Append(History.CollectionPlayerSamples, History.NewPlayerSample(snapshot, application.GameData)) == nil {
			lastCaptured = time.Now()
		}
	}
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
		case <-ticker.C:
			capture()
		}
	}
}

func (application *Application) registerCoreIntents() error {
	for name, action := range map[string]Intent.Action{
		"session.start": ignoreArguments(application.Session.Start),
		"session.stop":  ignoreArguments(application.Session.Stop),
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
			Name: "session.stop", Description: "Stop the active game session", Effect: Intent.EffectExternal,
			Planner: actionPlanner("session.stop", "session", "Stop the game session"),
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
	if err := application.GameData.Refresh(ctx); err != nil {
		return err
	}
	current, ok := application.GameData.Current()
	if !ok {
		return fmt.Errorf("official game data did not produce a snapshot")
	}
	version := current.Metadata().ItemVersion
	languageVersion := current.Metadata().LanguageVersion
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
		"scheduler":                      json.RawMessage(`{"minAttackDelay":4,"maxAttackDelay":6,"upgradeEreDelayMs":50,"upgradeCoinThreshold":0,"botLocked":false,"attackPriorities":{"autoTowers":50,"autoAdvisor":50,"autoStorm":50,"riftMaiden":50,"riftReplay":50},"featureSchedules":{}}`),
		"session.reconnect":              json.RawMessage(`{"relogDelaySec":300}`),
		"automation.enabled":             json.RawMessage(`{}`),
		"automation.autoBeriWorld":       json.RawMessage(`{"minTroopsToTransfer":1,"beriCastleId":0,"transferTroopId":0,"sourceCastleId":0,"wireCastleId":-1,"troopSpaceCheckIntervalSec":30}`),
		"automation.commanderFeatures":   json.RawMessage(`{"version":1,"assignments":{}}`),
		"automation.autoFoodBalance":     json.RawMessage(`{"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1000,"minimumSourceReserve":1000,"minimumCoinReserve":0,"autoKingdomTransport":true,"useKingdomTimeSkips":false,"allowedTimeSkips":[],"timeSkipReserve":{}}`),
		"automation.autoTowers":          json.RawMessage(`{"version":2,"checkIntervalSec":30,"mapRefreshIntervalSec":1800,"dailyAttackLimit":0,"horseTravelBoostId":-1,"castles":{}}`),
		"automation.autoInvasion":        json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","foreignLordsDifficultyId":0,"bloodcrowDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"fortifyCurrency":"","horseTravelBoostId":-1}`),
		"automation.autoNomad":           json.RawMessage(`{"version":5,"sourceCastleId":0,"nomadPresetId":"","samuraiPresetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"scoreTarget":0,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"dailyAttackLimit":0,"skipCooldowns":false,"timeSkipReserve":{},"rbcTest":{"enabled":false,"runId":"","targetX":0,"targetY":0},"horseTravelBoostId":-1}`),
		"automation.autoAdvisor":         json.RawMessage(`{"version":1,"sourceCastleId":0,"presetId":"","nomadDifficultyId":0,"samuraiDifficultyId":0,"maxAttackCount":9999,"minimumRemainingSec":1800,"coinCostPerAttack":500,"minimumCoinReserve":0,"rubyCostPerAttack":0,"minimumRubyReserve":0,"minimumFeatherReserve":0,"timeSkipReserve":{},"checkIntervalSec":30,"mapRefreshIntervalSec":300,"horseTravelBoostId":-1}`),
		"automation.autoKhan":            json.RawMessage(`{"version":1,"sourceCastleId":0,"attackPresetId":"","defensePresetId":"","minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,"mapRefreshIntervalSec":30,"dailyAttackLimit":0,"skipCooldowns":true,"timeSkipReserve":{},"openGateProtection":true,"offensiveUnitThreshold":1000,"horseTravelBoostId":-1,"nomadPointThreshold":0,"replenishDefenseTools":false}`),
		"automation.autoStorm":           json.RawMessage(`{"version":1,"decorationPresetCastleId":0,"decorationPresetId":"","build":{"allowPremium":false,"allowDemolition":false,"allowResourceTransport":true,"allowTimeSkips":false,"resourceReserves":{},"sourceResourceReserves":{},"timeSkipReserve":{}},"harbor":{"enabled":false,"targetLevel":1},"forts":{"enabled":false,"levels":[40,50,60,70,80],"minimumWins":0,"presetId":""},"islands":{"enabled":false,"resources":["wood","stone","aquamarine"],"sizes":["large","small"],"presetId":"","defenseUnits":[]},"troopImport":{"enabled":false,"donorCastleIds":[]},"aquamarine":{"reserve":0,"shopTableId":0,"purchases":[]},"targetPriority":["fort:80","fort:70","fort:60","fort:50","fort:40","island:large","island:small"],"checkIntervalSec":30,"mapRefreshIntervalSec":7200,"dailyAttackLimit":0,"horseTravelBoostId":-1}`),
		"automation.autoStormBlueprints": json.RawMessage(`{"version":1,"blueprints":{}}`),
		"rift.attackPreferences":         json.RawMessage(`{"version":1,"replayHorseTravelBoostId":-1,"maidenHorseTravelBoostId":-1}`),
	}
}
