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
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAutoBirdDirewolfProtectionUsesCurrentFortressControlAndCastleScope(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)
	configuration := autoBirdFortressConfiguration(t, json.RawMessage(`{"auto_fortress":true}`), 1)
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{ID: 10, KingdomID: 1, SlotType: 12}
	application := &Application{Configuration: configuration}

	if !application.autoBirdDirewolvesProtected(state, 10, now) {
		t.Fatal("enabled outer main castle did not reserve Direwolves")
	}
	for _, test := range []struct {
		name   string
		castle State.CastleState
	}{
		{name: "mainland", castle: State.CastleState{ID: 10, KingdomID: 0, SlotType: 12}},
		{name: "outpost", castle: State.CastleState{ID: 10, KingdomID: 1, SlotType: 1}},
		{name: "disabled kingdom", castle: State.CastleState{ID: 10, KingdomID: 2, SlotType: 12}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := state
			candidate.Castles = map[State.CastleID]State.CastleState{10: test.castle}
			if application.autoBirdDirewolvesProtected(candidate, 10, now) {
				t.Fatal("unprotected castle reserved Direwolves")
			}
		})
	}

	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{"auto_fortress":false,"auto_bird":true}`)); err != nil {
		t.Fatal(err)
	}
	if application.autoBirdDirewolvesProtected(state, 10, now) {
		t.Fatal("disabled Auto Fortress retained the automatic reserve")
	}
	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{
		"auto_fortress":{"enabled":true,"expiresAt":"2026-09-16T17:59:59Z"}
	}`)); err != nil {
		t.Fatal(err)
	}
	if application.autoBirdDirewolvesProtected(state, 10, now) {
		t.Fatal("expired Auto Fortress retained the automatic reserve")
	}
}

func TestAutoBirdManifestReservesEveryDirewolfWithoutChangingManualReserve(t *testing.T) {
	now := time.Now().UTC()
	_, gameData := autoBirdIntentTestState(t, now)
	source := State.CastleState{ID: 10, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
		GameData.DirewolfUnitID: 150,
		489:                     100,
	}}}
	manual := []stationUnitRequest{{UnitID: GameData.DirewolfUnitID, Amount: 75}, {UnitID: 489, Amount: 10}}

	protected, _, err := autoBirdStationManifest(gameData, source, manual, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := protected[GameData.DirewolfUnitID]; found || protected[489] != 90 {
		t.Fatalf("protected manifest = %#v", protected)
	}
	source.Units.Stationed[GameData.DirewolfUnitID] = 900
	fresh, _, err := autoBirdStationManifest(gameData, source, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fresh[GameData.DirewolfUnitID]; found {
		t.Fatalf("fresh Direwolf arrivals bypassed protection: %#v", fresh)
	}
	restored, _, err := autoBirdStationManifest(gameData, source, manual, false)
	if err != nil {
		t.Fatal(err)
	}
	if restored[GameData.DirewolfUnitID] != 825 || restored[489] != 90 {
		t.Fatalf("manual reserves were not restored after protection ended: %#v", restored)
	}
}

func TestAutoBirdFreshJAAPreparationAppliesDerivedDirewolfReserve(t *testing.T) {
	now := time.Now().UTC()
	gameState, _ := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 1, SlotType: 12, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 250, 489: 30}},
	}
	gameState.Alliance.Holdings[0].KingdomID = 1
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseTargetReady,
		SourceCastleID: 10, TargetCastleID: 20, DelayHours: 8, UpdatedAt: now,
	}
	configuration := autoBirdFortressConfiguration(t, json.RawMessage(`{"auto_fortress":true}`), 1)
	application := &Application{
		State: State.NewStore(gameState), Configuration: configuration,
		GameData: autoBirdFortressGameDataManager(t),
	}
	arguments, _ := json.Marshal(autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10", UnitsRefreshAt: now.Add(-time.Second),
		ExpectedTargetCastle: 20, MinimumDelayHours: 6, MaximumDelayHours: 12,
		Reserves: []stationUnitRequest{{UnitID: GameData.DirewolfUnitID, Amount: 10}},
	})
	if err := application.captureAutoBirdManifest(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	prepared := application.State.Snapshot().Stationing["autoBird:10"]
	if prepared.Phase != State.StationingPhaseDispatchReady || prepared.Units[489] != 30 {
		t.Fatalf("prepared operation = %#v", prepared)
	}
	if _, found := prepared.Units[GameData.DirewolfUnitID]; found {
		t.Fatalf("fresh JAA preparation retained Direwolves: %#v", prepared.Units)
	}
}

func TestAutoBirdFinalDispatchRebuildsLateDirewolfBatchAndAllowsOrdinaryBatch(t *testing.T) {
	for _, test := range []struct {
		name       string
		stationed  map[State.UnitID]int64
		wantSends  int
		wantReplan bool
	}{
		{name: "Direwolf batch", stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 40, 489: 20}, wantSends: 2, wantReplan: true},
		{name: "ordinary batch", stationed: map[State.UnitID]int64{489: 20}, wantSends: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			gameState, gameData := autoBirdIntentTestState(t, now)
			gameState.Castles[10] = State.CastleState{
				ID: 10, KingdomID: 1, SlotType: 12, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
				Units: State.CastleUnits{Stationed: test.stationed},
			}
			gameState.Alliance.Holdings[0].KingdomID = 1
			gameState.Stationing["autoBird:10"] = State.StationingOperation{
				ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
				SourceCastleID: 10, TargetCastleID: 20, DelayHours: 8,
				UnitsObservedAt: now, UpdatedAt: now,
			}
			configuration := autoBirdFortressConfiguration(t, json.RawMessage(`{"auto_fortress":false,"auto_bird":true}`), 1)
			state := State.NewStore(gameState)
			application := &Application{State: state, Configuration: configuration}
			request, _ := json.Marshal(autoBirdCycleRequest{
				SourceCastleID: 10, TrackingID: "autoBird:10", DispatchStartedAt: now.Add(-time.Second),
				ExpectedTargetCastle: 20, MinimumDelayHours: 6, MaximumDelayHours: 12,
			})
			sender := &autoBirdFinalGuardSender{configuration: configuration}
			registry := Intent.NewRegistry()
			if err := registry.Register(Intent.Definition{
				Name: "test.auto_bird.final", Effect: Intent.EffectLaunch,
				Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
					return Intent.Plan{Summary: "exercise Auto Bird final guard", Steps: []Intent.Step{{
						Name: "Resolve Auto Bird batch", Resolver: "auto_bird.dispatch.build",
						ResolverArguments: arguments, AwaitOpcode: "cds",
					}}}, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			engine := Intent.NewEngine(registry, state, autoBirdStaticGameData{store: gameData}, sender, autoBirdNoResponseObserver{})
			if err := engine.RegisterStepResolver("auto_bird.dispatch.build", application.resolveAutoBirdDispatchStep); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterAction("auto_bird.batch.guard", application.guardAutoBirdBatch); err != nil {
				t.Fatal(err)
			}

			receipt := engine.Submit(t.Context(), Intent.Request{
				ID: "late-fortress-toggle-" + strings.ReplaceAll(test.name, " ", "-"), Name: "test.auto_bird.final",
				Actor: "automation:autoBird", AutomationLane: "autoBird", Arguments: request,
			})
			if !strings.Contains(receipt.DiagnosticError(), errAutoBirdFinalGuardSentinel.Error()) {
				t.Fatalf("receipt error = %v", receipt.DiagnosticError())
			}
			if len(sender.attempts) != test.wantSends || len(sender.crossed) != 1 {
				t.Fatalf("attempts=%v crossed=%v", sender.attempts, sender.crossed)
			}
			if test.wantReplan {
				if !payloadContainsUnit(sender.attempts[0], GameData.DirewolfUnitID) || payloadContainsUnit(sender.crossed[0], GameData.DirewolfUnitID) {
					t.Fatalf("late enable did not rebuild before transport: attempts=%v crossed=%v", sender.attempts, sender.crossed)
				}
			} else if payloadContainsUnit(sender.crossed[0], GameData.DirewolfUnitID) {
				t.Fatalf("ordinary batch unexpectedly contains Direwolves: %s", sender.crossed[0])
			}
		})
	}
}

func autoBirdFortressConfiguration(t *testing.T, enabled json.RawMessage, enabledKingdom State.KingdomID) *Configuration.Store {
	t.Helper()
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled":      enabled,
		"automation.autoFortress": json.RawMessage(fmt.Sprintf(`{"kingdoms":{"%d":{"enabled":true}}}`, enabledKingdom)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return configuration
}

func autoBirdFortressGameDataManager(t *testing.T) *GameData.Manager {
	t.Helper()
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vtest.json"), []byte(`{
		"versionInfo":{"version":{"@value":"test"}},"buildings":[],
		"units":[{"wodID":277},{"wodID":489}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cacheDir, VersionURL: "offline://items-version"})
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return manager
}

type autoBirdStaticGameData struct{ store *GameData.Store }

func (provider autoBirdStaticGameData) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

type autoBirdNoResponseObserver struct{}

func (autoBirdNoResponseObserver) Watch(string, uint64) (<-chan Protocol.CommittedFrame, func()) {
	return make(chan Protocol.CommittedFrame), func() {}
}

var errAutoBirdFinalGuardSentinel = errors.New("stop after Auto Bird final guard")

type autoBirdFinalGuardSender struct {
	configuration *Configuration.Store
	once          sync.Once
	attempts      []json.RawMessage
	crossed       []json.RawMessage
}

func (*autoBirdFinalGuardSender) Ready() bool       { return true }
func (*autoBirdFinalGuardSender) Namespace() string { return "EmpireEx_21" }
func (sender *autoBirdFinalGuardSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.attempts = append(sender.attempts, append(json.RawMessage(nil), command.Payload...))
	sender.once.Do(func() {
		_, err = sender.configuration.Update("automation.enabled", json.RawMessage(`{"auto_fortress":true,"auto_bird":true}`))
	})
	if err != nil {
		return err
	}
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	sender.crossed = append(sender.crossed, append(json.RawMessage(nil), command.Payload...))
	return errAutoBirdFinalGuardSentinel
}

func payloadContainsUnit(payload json.RawMessage, unitID State.UnitID) bool {
	var command struct {
		Units [][2]int64 `json:"A"`
	}
	if json.Unmarshal(payload, &command) != nil {
		return false
	}
	for _, unit := range command.Units {
		if State.UnitID(unit[0]) == unitID {
			return true
		}
	}
	return false
}
