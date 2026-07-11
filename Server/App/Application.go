package App

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

const GameDataRefreshInterval = 6 * time.Hour

type Config struct {
	DataDir        string
	Offline        bool
	Transport      Session.Transport
	RuntimeContext context.Context
}

type Application struct {
	State         *State.Store
	GameData      *GameData.Manager
	Configuration *Configuration.Store
	History       *History.Store
	Telemetry     *Telemetry.Store
	Ingest        *Ingest.Pipeline
	Session       *Session.Controller
	Intents       *Intent.Engine
	API           *API.Server
	StartupErr    error
}

func New(ctx context.Context, config Config) (*Application, error) {
	if config.DataDir == "" {
		return nil, fmt.Errorf("application data directory is required")
	}
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
	if current, ready := gameData.Current(); ready {
		initial.CatalogVersion = current.Metadata().ItemVersion
		initial.LanguageVersion = current.Metadata().LanguageVersion
	}
	state := State.NewStore(initial)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		return nil, fmt.Errorf("register protocol reducers: %w", err)
	}
	ingest := Ingest.NewPipeline(state, gameData, registry)
	telemetry := Telemetry.NewStore(5000)
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
	intents := Intent.NewEngine(intentRegistry, state, gameData, session, ingest)
	application := &Application{
		State: state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Ingest: ingest, Session: session, Intents: intents, StartupErr: startupErr,
	}
	if err := application.registerCoreIntents(); err != nil {
		return nil, err
	}
	if err := application.registerGameIntents(); err != nil {
		return nil, err
	}
	application.API = API.NewServer(API.Config{
		Version: Version, State: state, GameData: gameData, Configuration: configuration, History: history, Telemetry: telemetry,
		Intents: intents, Session: session,
	})
	return application, nil
}

func (application *Application) Start(ctx context.Context) {
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
			_, err = application.Configuration.UpdateExpected(update.Section, update.Value, update.ExpectedRevision)
			return err
		},
		"game_data.refresh": ignoreArguments(application.refreshGameData),
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
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
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
		if gameState.CatalogVersion == version && gameState.LanguageVersion == languageVersion {
			return nil, false, nil
		}
		gameState.CatalogVersion = version
		gameState.LanguageVersion = languageVersion
		return []string{"game-data"}, true, nil
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
	Section          string          `json:"section"`
	Value            json.RawMessage `json:"value"`
	ExpectedRevision *uint64         `json:"expectedRevision,omitempty"`
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
	return input, nil
}

func defaultConfiguration() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"scheduler":          json.RawMessage(`{"minAttackDelay":4,"maxAttackDelay":6,"upgradeEreDelayMs":50,"upgradeCoinThreshold":0,"manualFocusIdleSec":30,"tabPriorities":{},"featureSchedules":{}}`),
		"automation.enabled": json.RawMessage(`{}`),
	}
}
