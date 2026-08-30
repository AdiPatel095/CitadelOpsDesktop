package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestKhanPresetShortageCancelsTheCRAAsStale(t *testing.T) {
	itemID := int64(215)
	preset := AttackPresets.Preset{
		CourtyardSupport: AttackPresets.CourtyardSupport{
			Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 60}},
		},
		Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{
				Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 40}},
			},
		}},
	}
	source := State.CastleState{
		ID:    2,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 50}},
	}
	err := khanAttackPresetAvailability(preset, source, nil)
	if !errors.Is(err, Intent.ErrPlanStale) ||
		!strings.Contains(err.Error(), "CRA launch cursor paused") {
		t.Fatalf("Khan preset shortage error = %v", err)
	}
}

func TestKhanAttackContextRechecksRageChainCapBeforeDeferredCRA(t *testing.T) {
	now := time.Date(2026, 8, 29, 16, 30, 0, 0, time.UTC)
	eventEndsAt := now.Add(72 * time.Hour)
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, SlotType: 1,
		Defense: State.CastleDefenseState{ObservedAt: now, InventoryObservedAt: now},
	}
	gameState.Castles[2] = State.CastleState{ID: 2, KingdomID: 0, SlotType: 2}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"210:942": {KingdomID: 0, TypeID: khanCampTypeID, X: 210, Y: 942, ObservedAt: now},
	}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: int64((72 * time.Hour) / time.Second), ObservedAt: now,
	}
	gameState.EventScores.ActivityByEvent[khanEventID] = State.EventActivityState{
		EventID: khanEventID, OccurrenceEndsAt: eventEndsAt, ObservedFrom: now,
		KhanDefense: State.EventCombatTotals{Launches: 3},
	}
	request := khanAttackRequest{
		RunID: "run", EventEndsAt: eventEndsAt, SourceCastleID: 2, MainCastleID: 1,
		KingdomID: 0, TargetX: 210, TargetY: 942, CommanderID: 1,
		HorseTravelBoostID: -1, MaxRageChain: 3,
	}
	arguments, _ := json.Marshal(request)
	_, _, _, err := khanAttackContext(Intent.PlanningContext{
		State: gameState, GameData: &GameData.Store{},
	}, arguments, now, false)
	if err == nil || errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "3 / 3 taunts") {
		t.Fatalf("rage-chain execution guard error = %v", err)
	}

	request.MaxRageChain = 4
	arguments, _ = json.Marshal(request)
	if _, _, _, err = khanAttackContext(Intent.PlanningContext{
		State: gameState, GameData: &GameData.Store{},
	}, arguments, now, false); err != nil {
		t.Fatalf("below-cap attack context was blocked: %v", err)
	}
}

func TestPlanKhanMapJumpUsesCapturedFNMCommand(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	plan, err := planKhanMapJump(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Claims) != 2 || plan.Claims[0] != "castle-focus" || plan.Claims[1] != "map:0" ||
		len(plan.Steps) != 1 || plan.Steps[0].Opcode != "fnm" || plan.Steps[0].AwaitOpcode != "fnm" {
		t.Fatalf("Khan map jump plan = %#v", plan)
	}
	var payload struct {
		TargetTypeID int             `json:"T"`
		KingdomID    State.KingdomID `json:"KID"`
		MinimumLevel int             `json:"LMIN"`
		MaximumLevel int             `json:"LMAX"`
		NPCID        int             `json:"NID"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TargetTypeID != 35 || payload.KingdomID != 0 ||
		payload.MinimumLevel != -1 || payload.MaximumLevel != -1 || payload.NPCID != -801 {
		t.Fatalf("Khan map jump payload = %#v", payload)
	}
}

func TestPlanKhanTauntUsesLTAAndAllowsParallelRetaliations(t *testing.T) {
	now := time.Now().UTC()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":"1147","eventID":"72","difficultyID":"310",
			"areaType":"35","camplevel":"107","playerRageCap":"1740"
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1, X: 212, Y: 941}
	gameState.Map[0] = map[string]State.MapObservation{
		"939:1123": {KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123, EventCampID: 1147},
	}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	eventEndsAt := now.Add(7_200 * time.Second)
	gameState.EventScores.ActivityByEvent[khanEventID] = State.EventActivityState{
		EventID: khanEventID, OccurrenceEndsAt: eventEndsAt, ObservedFrom: now.Add(-time.Hour),
	}
	gameState.Khan = State.KhanState{
		RageCampID: 1147, PlayerRage: 1740, PlayerRageCap: 1740, PlayerTotalRage: 52140,
		RageObservedAt: now, TauntsTriggered: 1, LastTauntTriggeredAt: now.Add(-time.Minute),
		LastTauntTriggeredRage: 50400,
		Taunts: map[State.MovementID]State.KhanTauntState{
			90: {MovementID: 90, ObservedAt: now.Add(-time.Minute), ImpactAt: now.Add(time.Minute)},
		},
		Launches: []State.KhanLaunchState{},
	}
	arguments, _ := json.Marshal(khanTauntRequest{
		EventID: khanEventID, EventEndsAt: eventEndsAt, MainCastleID: 1, TargetX: 939, TargetY: 1123,
		RageCampID: 1147, PlayerTotalRage: 52140, RageObservedAt: now,
		KhanGuard: khanLaneGuardRequest{MainCastleID: 1},
	})
	plan, err := planKhanTaunt(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Resolver != "khan.taunt.build" ||
		plan.Steps[0].AwaitOpcode != "gam" ||
		plan.Steps[0].TimeoutMillis != khanTauntResponseTimeoutMillis ||
		len(plan.Steps[0].SuccessCodes) != 1 || plan.Steps[0].SuccessCodes[0] != 0 ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[0].ResponseIdentity.PlayerID != 158 || plan.Steps[0].ResponseIdentity.CastleID != 1 ||
		plan.Steps[1].Action != "khan.taunt.accepted" {
		t.Fatalf("Khan LTA plan = %#v", plan)
	}
	resolved, err := resolveKhanTauntStep(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, plan.Steps[0].ResolverArguments)
	if err != nil || resolved.Opcode != "lta" || resolved.AwaitOpcode != "gam" ||
		resolved.TimeoutMillis != khanTauntResponseTimeoutMillis ||
		len(resolved.SuccessCodes) != 1 || resolved.SuccessCodes[0] != 0 ||
		resolved.ResponseBarrier != Intent.ResponseBarrierCommitted ||
		resolved.ResponseIdentity.PlayerID != 158 || resolved.ResponseIdentity.CastleID != 1 ||
		string(resolved.Command.Payload) != `{"AV":0,"EID":72}` {
		t.Fatalf("resolved Khan LTA step = %#v, err=%v", resolved, err)
	}

	gameState.Khan.PlayerRage = 0
	if _, err := resolveKhanTauntStep(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, plan.Steps[0].ResolverArguments); err == nil || !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("send-time Khan rage change did not stale the LTA: %v", err)
	}
	gameState.Khan.PlayerRage = gameState.Khan.PlayerRageCap

	gameState.Khan.LastTauntTriggeredRage = gameState.Khan.PlayerTotalRage
	_, err = planKhanTaunt(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "rage bar is no longer ready") {
		t.Fatalf("same rage fill plan error = %v", err)
	}

	gameState.Khan.LastTauntTriggeredRage = 682_930
	gameState.Khan.LastTauntTriggeredAt = now.Add(-7 * 24 * time.Hour)
	if _, err = planKhanTaunt(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments); err != nil {
		t.Fatalf("new event rage reset did not reopen the taunt cursor: %v", err)
	}
}

func TestRecordAcceptedKhanTauntReplacesPriorEventRageCursor(t *testing.T) {
	observedAt := time.Now().UTC()
	eventEndsAt := observedAt.Add(72 * time.Hour)
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: int64((72 * time.Hour) / time.Second), ObservedAt: observedAt,
	}
	gameState.EventScores.ActivityByEvent[khanEventID] = State.EventActivityState{
		EventID: khanEventID, OccurrenceEndsAt: eventEndsAt, ObservedFrom: observedAt.Add(-time.Hour),
	}
	gameState.Khan.LastTauntTriggeredRage = 682_930
	gameState.Khan.LastTauntTriggeredAt = observedAt.Add(-7 * 24 * time.Hour)
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanTauntRequest{
		EventEndsAt: eventEndsAt, PlayerTotalRage: 10_080, RageObservedAt: observedAt,
	})
	if err := application.recordKhanTauntAcceptance(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	khan := application.State.Snapshot().Khan
	if khan.LastTauntTriggeredRage != 10_080 ||
		!State.SameEventOccurrence(khan.LastTauntTriggeredEventEndsAt, eventEndsAt) || khan.TauntsTriggered != 1 ||
		!khan.LastTauntTriggeredAt.After(observedAt) {
		t.Fatalf("new event rage cursor = %#v", khan)
	}
}

func TestLegacyUnconfirmedKhanTauntDispatchFailsClosed(t *testing.T) {
	gameState := State.NewGameState()
	application := &Application{State: State.NewStore(gameState)}
	if err := application.rejectUnconfirmedKhanTauntDispatch(t.Context(), json.RawMessage(`{}`)); err == nil || !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("legacy unconfirmed dispatch error = %v", err)
	}
	if khan := application.State.ReadOnlyView().Khan; khan.TauntsTriggered != 0 ||
		!khan.LastTauntTriggeredAt.IsZero() || khan.LastTauntTriggeredRage != 0 {
		t.Fatalf("legacy unconfirmed dispatch consumed the rage cursor: %#v", khan)
	}
}

type khanTauntResponseObserver struct {
	mu       sync.Mutex
	watchers map[string]chan Protocol.CommittedFrame
	ingress  uint64
}

func newKhanTauntResponseObserver() *khanTauntResponseObserver {
	return &khanTauntResponseObserver{watchers: map[string]chan Protocol.CommittedFrame{}}
}

func (observer *khanTauntResponseObserver) Watch(
	opcode string,
	_ uint64,
) (<-chan Protocol.CommittedFrame, func()) {
	return observer.watch(opcode)
}

func (observer *khanTauntResponseObserver) WatchResponse(
	_ string,
	_ uint64,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	return observer.watch(responseToken)
}

func (observer *khanTauntResponseObserver) watch(key string) (<-chan Protocol.CommittedFrame, func()) {
	channel := make(chan Protocol.CommittedFrame, 1)
	observer.mu.Lock()
	observer.watchers[key] = channel
	observer.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			observer.mu.Lock()
			if observer.watchers[key] == channel {
				delete(observer.watchers, key)
			}
			observer.mu.Unlock()
		})
	}
}

func (observer *khanTauntResponseObserver) deliver(responseToken string, code int) bool {
	observer.mu.Lock()
	channel := observer.watchers[responseToken]
	observer.ingress++
	ingress := observer.ingress
	observer.mu.Unlock()
	if channel == nil {
		return false
	}
	channel <- Protocol.CommittedFrame{
		Frame: Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "gam", ResponseCode: &code,
			ResponseToken: responseToken, ReceivedAt: time.Now().UTC(),
		},
		IngressID: ingress, Revision: ingress,
	}
	return true
}

type khanTauntResponseSender struct {
	observer *khanTauntResponseObserver
	mu       sync.Mutex
	autoCode *int
	metadata []Outbound.Metadata
	sent     chan Outbound.Metadata
}

func (*khanTauntResponseSender) Ready() bool                  { return true }
func (*khanTauntResponseSender) Namespace() string            { return "EmpireEx_21" }
func (*khanTauntResponseSender) CorrelatesResponses() bool    { return true }
func (*khanTauntResponseSender) ConnectionGeneration() uint64 { return 0 }

func (sender *khanTauntResponseSender) Send(ctx context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if frame.Opcode != "lta" {
		return errors.New("unexpected non-LTA command")
	}
	metadata := Outbound.MetadataFromContext(ctx)
	sender.mu.Lock()
	sender.metadata = append(sender.metadata, metadata)
	autoCode := sender.autoCode
	sender.mu.Unlock()
	select {
	case sender.sent <- metadata:
	default:
	}
	if autoCode != nil && !sender.observer.deliver(metadata.ResponseToken, *autoCode) {
		return errors.New("GAM response watcher was not installed before LTA")
	}
	return nil
}

func (sender *khanTauntResponseSender) setAutoCode(code *int) {
	sender.mu.Lock()
	sender.autoCode = code
	sender.mu.Unlock()
}

func (sender *khanTauntResponseSender) sendCount() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.metadata)
}

type khanTauntGameDataProvider struct{ store *GameData.Store }

func (provider khanTauntGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

func newKhanTauntExecutionHarness(
	t *testing.T,
) (*Intent.Engine, *Application, *khanTauntResponseSender, json.RawMessage) {
	t.Helper()
	now := time.Now().UTC()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],"currencies":[],"effects":[],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":"1147","eventID":"72","difficultyID":"310",
			"areaType":"35","camplevel":"107","playerRageCap":"1740"
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1, X: 212, Y: 941}
	gameState.Map[0] = map[string]State.MapObservation{
		"939:1123": {KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123, EventCampID: 1147},
	}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	eventEndsAt := now.Add(7_200 * time.Second)
	gameState.EventScores.ActivityByEvent[khanEventID] = State.EventActivityState{
		EventID: khanEventID, OccurrenceEndsAt: eventEndsAt, ObservedFrom: now.Add(-time.Hour),
	}
	gameState.Khan = State.KhanState{
		RageCampID: 1147, PlayerRage: 1740, PlayerRageCap: 1740,
		PlayerTotalRage: 52_140, RageObservedAt: now,
		Taunts: map[State.MovementID]State.KhanTauntState{},
	}
	arguments, _ := json.Marshal(khanTauntRequest{
		EventID: khanEventID, EventEndsAt: eventEndsAt, MainCastleID: 1, TargetX: 939, TargetY: 1123,
		RageCampID: 1147, PlayerTotalRage: 52_140, RageObservedAt: now,
		KhanGuard: khanLaneGuardRequest{MainCastleID: 1},
	})
	stateStore := State.NewStore(gameState)
	application := &Application{State: stateStore}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "khan.taunt", Effect: Intent.EffectWrite, Planner: planKhanTaunt,
	}); err != nil {
		t.Fatal(err)
	}
	observer := newKhanTauntResponseObserver()
	sender := &khanTauntResponseSender{observer: observer, sent: make(chan Outbound.Metadata, 8)}
	engine := Intent.NewEngine(
		registry, stateStore, khanTauntGameDataProvider{store: gameData}, sender, observer,
	)
	if err := engine.RegisterStepResolver("khan.taunt.build", resolveKhanTauntStep); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterAction("khan.taunt.accepted", application.recordKhanTauntAcceptance); err != nil {
		t.Fatal(err)
	}
	return engine, application, sender, arguments
}

func TestKhanTauntRejectedOrDroppedGAMDoesNotConsumeCursorAndCanRetry(t *testing.T) {
	for _, test := range []struct {
		name string
		code *int
	}{
		{name: "rejected", code: func() *int { value := 273; return &value }()},
		{name: "dropped"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, application, sender, arguments := newKhanTauntExecutionHarness(t)
			sender.setAutoCode(test.code)
			ctx := t.Context()
			cancel := func() {}
			if test.code == nil {
				ctx, cancel = context.WithTimeout(ctx, 40*time.Millisecond)
			}
			receipt := engine.Submit(ctx, Intent.Request{Name: "khan.taunt", Arguments: arguments})
			cancel()
			if receipt.Status == Intent.StatusSucceeded {
				t.Fatalf("unaccepted GAM receipt = %#v", receipt)
			}
			khan := application.State.ReadOnlyView().Khan
			if khan.TauntsTriggered != 0 || !khan.LastTauntTriggeredAt.IsZero() ||
				khan.LastTauntTriggeredRage != 0 {
				t.Fatalf("unaccepted GAM consumed rage cursor: %#v", khan)
			}

			accepted := 0
			sender.setAutoCode(&accepted)
			retry := engine.Submit(t.Context(), Intent.Request{Name: "khan.taunt", Arguments: arguments})
			if retry.Status != Intent.StatusSucceeded {
				t.Fatalf("accepted GAM retry receipt = %#v", retry)
			}
			khan = application.State.ReadOnlyView().Khan
			if khan.TauntsTriggered != 1 || khan.LastTauntTriggeredRage != 52_140 ||
				khan.LastTauntTriggeredAt.IsZero() {
				t.Fatalf("accepted GAM did not advance rage cursor: %#v", khan)
			}
			if sender.sendCount() != 2 {
				t.Fatalf("LTA sends = %d, want one failed attempt plus one retry", sender.sendCount())
			}
		})
	}
}

func TestKhanTauntClaimPreventsDuplicateWhileGAMIsPending(t *testing.T) {
	engine, application, sender, arguments := newKhanTauntExecutionHarness(t)
	firstResult := make(chan Intent.Receipt, 1)
	go func() {
		firstResult <- engine.Submit(context.Background(), Intent.Request{Name: "khan.taunt", Arguments: arguments})
	}()
	var first Outbound.Metadata
	select {
	case first = <-sender.sent:
	case <-time.After(time.Second):
		t.Fatal("first LTA was not dispatched")
	}
	if first.ResponseToken == "" {
		t.Fatal("pending LTA omitted its correlated GAM response token")
	}

	secondContext, cancelSecond := context.WithTimeout(t.Context(), 40*time.Millisecond)
	second := engine.Submit(secondContext, Intent.Request{Name: "khan.taunt", Arguments: arguments})
	cancelSecond()
	if second.Status == Intent.StatusSucceeded {
		t.Fatalf("duplicate pending LTA unexpectedly succeeded: %#v", second)
	}
	if sender.sendCount() != 1 {
		t.Fatalf("pending GAM allowed %d LTA sends, want exactly one", sender.sendCount())
	}
	if !sender.observer.deliver(first.ResponseToken, 0) {
		t.Fatal("pending correlated GAM watcher disappeared")
	}
	select {
	case receipt := <-firstResult:
		if receipt.Status != Intent.StatusSucceeded {
			t.Fatalf("first accepted LTA receipt = %#v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("first LTA did not finish after accepted GAM")
	}
	if khan := application.State.ReadOnlyView().Khan; khan.TauntsTriggered != 1 ||
		khan.LastTauntTriggeredRage != 52_140 {
		t.Fatalf("accepted pending LTA cursor = %#v", khan)
	}
}

func TestKhanDefenseToolPurchasePassesProductionResourceAdmission(t *testing.T) {
	now := time.Now().UTC()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":731}],"currencies":[],
		"packages":[{
			"packageID":10,"packageType":"tool","unitID":731,"unitAmount":1,"packagePriceC1":25
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var preset KhanDomain.DefensePreset
	if err := json.Unmarshal([]byte(`{
		"id":"defense","name":"Defense",
		"wall":{
			"left":{"toolSlots":[{"definitionId":731,"amount":1}]},
			"middle":{"toolSlots":[]},"right":{"toolSlots":[]}
		},
		"moat":{"leftToolSlots":[],"middleToolSlots":[],"rightToolSlots":[]}
	}`), &preset); err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.Resources[1] = 100
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, SlotType: 1,
		Defense: State.CastleDefenseState{Inventory: map[State.UnitID]int64{}},
	}
	gameState.EventScores.ShopByPackage[10] = State.EventShopRoute{
		EventID: 72, RemainingSec: 3_600, ObservedAt: now,
	}
	registry := Intent.NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Intent.Definition{
		Name: "khan.defense_tools.replenish", Effect: Intent.EffectWrite,
		Planner: planKhanDefenseToolReplenish,
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(
		registry, State.NewStore(gameState), beriIntentGameDataProvider{store: gameData}, nil, nil,
	)
	arguments, _ := json.Marshal(khanDefenseToolPurchaseRequest{
		CastleID: 1, PackageID: 10, ToolID: 731, Amount: 1, ShopTableID: 72, DefensePreset: preset,
	})
	receipt := engine.Submit(t.Context(), Intent.Request{
		Name: "khan.defense_tools.replenish", DryRun: true, Arguments: arguments,
	})
	if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
		t.Fatalf("Khan defense-tool resource admission failed: %#v", receipt)
	}
	foundToolResource := false
	for _, resource := range receipt.Plan.Resources {
		if resource.Capability == "legacy" {
			t.Fatalf("Khan defense-tool purchase retained a legacy resource: %#v", receipt.Plan.Resources)
		}
		if resource.Scope == Intent.ResourceScopeCastle && resource.CastleID == 1 &&
			resource.Capability == "garrison" && resource.ResourceKind == "unit" && resource.ResourceID == "731" {
			foundToolResource = true
		}
	}
	if !foundToolResource {
		t.Fatalf("Khan defense-tool purchase omitted the castle tool resource: %#v", receipt.Plan.Resources)
	}
}

func TestCaptureKhanLaunchAcceptsSubsecondArrivalProjectionJitter(t *testing.T) {
	now := time.Date(2026, 8, 29, 11, 45, 46, 0, time.UTC)
	previousArrival := time.Date(2026, 8, 29, 11, 45, 47, 740198696, time.UTC)
	currentArrival := time.Date(2026, 8, 29, 11, 45, 47, 143253357, time.UTC)
	commanderID := State.CommanderID(2)
	gameState := State.NewGameState()
	gameState.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1,
		Launches:        []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: previousArrival}},
		Taunts:          map[State.MovementID]State.KhanTauntState{},
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 2, KingdomID: 0, TargetX: 210, TargetY: 942,
		CommanderID: &commanderID, ArrivesAt: &currentArrival, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanLaunchCapture{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0,
		TargetX: 210, TargetY: 942, CommanderID: commanderID,
	})
	if err := application.captureKhanLaunch(t.Context(), arguments); err != nil {
		t.Fatalf("subsecond projection jitter was rejected: %v", err)
	}
	khan := application.State.Snapshot().Khan
	if khan.SafetyError != "" || khan.AttacksLaunched != 2 || len(khan.Launches) != 2 {
		t.Fatalf("captured Khan state = %#v", khan)
	}
}

func TestCaptureKhanLaunchRecordsAndBlocksMaterialOvertake(t *testing.T) {
	now := time.Date(2026, 8, 29, 11, 45, 46, 0, time.UTC)
	previousArrival := now.Add(3 * time.Second)
	currentArrival := now.Add(time.Second)
	commanderID := State.CommanderID(2)
	gameState := State.NewGameState()
	gameState.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1,
		Launches:        []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: previousArrival}},
		Taunts:          map[State.MovementID]State.KhanTauntState{},
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 2, KingdomID: 0, TargetX: 210, TargetY: 942,
		CommanderID: &commanderID, ArrivesAt: &currentArrival, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanLaunchCapture{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0,
		TargetX: 210, TargetY: 942, CommanderID: commanderID,
	})
	err := application.captureKhanLaunch(t.Context(), arguments)
	if err == nil || !strings.Contains(err.Error(), "unsafe Khan chain arrival order") {
		t.Fatalf("capture error = %v", err)
	}
	khan := application.State.Snapshot().Khan
	if khan.SafetyError == "" || khan.AttacksLaunched != 2 || len(khan.Launches) != 2 {
		t.Fatalf("captured Khan state = %#v", khan)
	}
}

func TestCaptureKhanLaunchClearsFinishedArrivalErrorAfterOrderedLaunch(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	previousArrival := now.Add(-time.Minute)
	currentArrival := now.Add(time.Minute)
	commanderID := State.CommanderID(2)
	gameState := State.NewGameState()
	gameState.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1,
		Launches:        []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: previousArrival}},
		Taunts:          map[State.MovementID]State.KhanTauntState{},
		SafetyError:     "finished historical inversion",
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 2, KingdomID: 0, TargetX: 210, TargetY: 942,
		CommanderID: &commanderID, ArrivesAt: &currentArrival, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanLaunchCapture{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0,
		TargetX: 210, TargetY: 942, CommanderID: commanderID,
	})
	if err := application.captureKhanLaunch(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	if khan := application.State.Snapshot().Khan; khan.SafetyError != "" || khan.AttacksLaunched != 2 {
		t.Fatalf("recovered Khan state = %#v", khan)
	}
}

func TestPlanKhanPointLimitProtectionRecallsKhanLaunchAndOpensGates(t *testing.T) {
	now := time.Now().UTC()
	arrival := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 158, SourceCastleID: 1, ArrivesAt: &arrival,
	}
	gameState.Khan.Launches = []State.KhanLaunchState{{MovementID: 50}}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{EventID: khanEventID, PlayerScore: 10_000}
	arguments := json.RawMessage(`{"castleId":1,"pointThreshold":10000}`)
	plan, err := planKhanPointLimitProtection(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "mcm" || plan.Steps[1].Opcode != "mos" {
		t.Fatalf("point-limit steps = %#v", plan.Steps)
	}
}
