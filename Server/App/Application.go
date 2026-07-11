package App

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

const GameDataRefreshInterval = 6 * time.Hour

type Config struct {
	DataDir        string
	Offline        bool
	Transport      Session.Transport
	RuntimeContext context.Context
}

type Application struct {
	State      *State.Store
	GameData   *GameData.Manager
	Ingest     *Ingest.Pipeline
	Session    *Session.Controller
	Intents    *Intent.Engine
	API        *API.Server
	StartupErr error
}

func New(ctx context.Context, config Config) (*Application, error) {
	if config.DataDir == "" {
		return nil, fmt.Errorf("application data directory is required")
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
		State: state, GameData: gameData, Ingest: ingest, Session: session,
		Intents: intents, StartupErr: startupErr,
	}
	if err := application.registerCoreIntents(); err != nil {
		return nil, err
	}
	if err := application.registerGameIntents(); err != nil {
		return nil, err
	}
	application.API = API.NewServer(API.Config{
		Version: Version, State: state, GameData: gameData, Intents: intents, Session: session,
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
