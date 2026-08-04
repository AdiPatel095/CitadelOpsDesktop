package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

type metadataSender struct {
	metadata   chan Outbound.Metadata
	generation uint64
}

func TestValidateExpectedResponsePayloadRequiresExactSemanticMatch(t *testing.T) {
	expected := json.RawMessage(`{"OID":32,"PWR":0,"PO":-1,"CC2T":12300}`)
	actual := json.RawMessage(`{"CC2T":12300,"PO":-1,"PWR":0,"OID":32}`)
	if err := validateExpectedResponsePayload(expected, actual); err != nil {
		t.Fatalf("semantically equal response rejected: %v", err)
	}
	if err := validateExpectedResponsePayload(expected, json.RawMessage(`{"OID":32,"PWR":0,"PO":-1,"CC2T":12400}`)); err == nil {
		t.Fatal("changed premium quote was accepted")
	}
}

func (*metadataSender) Ready() bool                         { return true }
func (*metadataSender) Namespace() string                   { return "EmpireEx_21" }
func (sender *metadataSender) ConnectionGeneration() uint64 { return sender.generation }
func (sender *metadataSender) Send(ctx context.Context, _ []byte) error {
	sender.metadata <- Outbound.MetadataFromContext(ctx)
	return nil
}

type connectionGenerationSender struct {
	generation atomic.Uint64
	sent       chan struct{}
}

type readinessSender struct {
	ready atomic.Bool
	sent  chan struct{}
}

func (sender *readinessSender) Ready() bool { return sender.ready.Load() }
func (*readinessSender) Namespace() string  { return "EmpireEx_21" }
func (sender *readinessSender) Send(context.Context, []byte) error {
	sender.sent <- struct{}{}
	return nil
}

func (*connectionGenerationSender) Ready() bool       { return true }
func (*connectionGenerationSender) Namespace() string { return "EmpireEx_21" }
func (sender *connectionGenerationSender) ConnectionGeneration() uint64 {
	return sender.generation.Load()
}
func (sender *connectionGenerationSender) Send(context.Context, []byte) error {
	sender.sent <- struct{}{}
	return nil
}

type admissionTestSender struct {
	nextAllowed time.Time
}

type indeterminateSender struct{}

func (*indeterminateSender) Ready() bool       { return true }
func (*indeterminateSender) Namespace() string { return "EmpireEx_21" }
func (*indeterminateSender) Send(context.Context, []byte) error {
	return Outbound.MarkIndeterminate(context.DeadlineExceeded)
}

func (*admissionTestSender) Ready() bool                        { return true }
func (*admissionTestSender) Namespace() string                  { return "EmpireEx_21" }
func (*admissionTestSender) Send(context.Context, []byte) error { return nil }
func (sender *admissionTestSender) NextAllowed(Outbound.Lane) time.Time {
	return sender.nextAllowed
}

type payloadSender struct {
	payload chan Protocol.Frame
}

type rapidChainTransport struct {
	mu       sync.Mutex
	sends    []time.Time
	frames   chan Session.RawFrame
	statuses chan Session.Status
	status   Session.Status
}

type pipelineResponseSender struct {
	pipeline     *Ingest.Pipeline
	responseCode int
}

type correlatingPipelineResponseSender struct {
	*pipelineResponseSender
	metadata chan Outbound.Metadata
}

type countingStateReader struct {
	store     *State.Store
	snapshots atomic.Int32
}

type dispatchMutationStateReader struct {
	store *State.Store
	calls atomic.Int32
}

type rotatingGameDataProvider struct {
	first  *GameData.Store
	second *GameData.Store
	calls  atomic.Int32
}

type localizedGameDataProvider struct {
	store    *GameData.Store
	language *GameData.LanguageStore
}

func (provider localizedGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

func (provider localizedGameDataProvider) Language() (*GameData.LanguageStore, bool) {
	return provider.language, provider.language != nil
}

func (provider *rotatingGameDataProvider) Current() (*GameData.Store, bool) {
	if provider.calls.Add(1) == 1 {
		return provider.first, provider.first != nil
	}
	return provider.second, provider.second != nil
}

func TestPlanningContextIncludesOfficialLanguage(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"unit_name":"Official Unit"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(nil, nil, localizedGameDataProvider{store: gameData, language: language}, nil, nil)
	input := engine.planningContext()
	if input.GameData != gameData || input.Language != language {
		t.Fatalf("planning context game data = %p language = %p", input.GameData, input.Language)
	}
}

func (reader *dispatchMutationStateReader) Snapshot() State.GameState { return reader.store.Snapshot() }
func (reader *dispatchMutationStateReader) Revision() uint64          { return reader.store.Revision() }
func (reader *dispatchMutationStateReader) Session() State.SessionState {
	return reader.store.Session()
}
func (reader *dispatchMutationStateReader) PlanningView() State.PlanningView {
	if reader.calls.Add(1) == 2 {
		_, _ = reader.store.Apply(func(state *State.GameState) ([]string, bool, error) {
			state.Player.Level = 9
			return []string{"player"}, true, nil
		})
	}
	return reader.store.PlanningView()
}

func (reader *countingStateReader) Snapshot() State.GameState {
	reader.snapshots.Add(1)
	return reader.store.Snapshot()
}

func (reader *countingStateReader) Revision() uint64 {
	return reader.store.Revision()
}

func (reader *countingStateReader) Session() State.SessionState {
	return reader.store.Session()
}

func (*pipelineResponseSender) Ready() bool       { return true }
func (*pipelineResponseSender) Namespace() string { return "EmpireEx_21" }
func (sender *pipelineResponseSender) Send(_ context.Context, payload []byte) error {
	request, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	code := sender.responseCode
	observed := sender.pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Namespace: request.Namespace,
		Opcode: request.Opcode, ResponseCode: &code, ReceivedAt: time.Now().UTC(),
	})
	go func() { _, _ = sender.pipeline.CommitFrame(context.Background(), observed) }()
	return nil
}

func TestExecuteStepClassifiesDeclaredResponseCodeAsStale(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21",
	}
	store := State.NewStore(gameState)
	pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
	engine := NewEngine(nil, store, nil, &pipelineResponseSender{pipeline: pipeline, responseCode: 147}, pipeline)

	_, err := engine.executeStep(t.Context(), store.Revision(), Step{
		Opcode: "msb", AwaitOpcode: "msb", TimeoutMillis: 1_000,
		SuccessCodes: []int{0}, StaleCodes: []int{147},
		Command: Protocol.Command{Opcode: "msb", Payload: json.RawMessage(`{"OID":17,"MST":"MS3"}`)},
	})
	if !errors.Is(err, ErrPlanStale) || !strings.Contains(err.Error(), "response code 147") {
		t.Fatalf("declared stale response error = %v", err)
	}
}

func (*correlatingPipelineResponseSender) CorrelatesResponses() bool { return true }
func (sender *correlatingPipelineResponseSender) Send(ctx context.Context, payload []byte) error {
	sender.metadata <- Outbound.MetadataFromContext(ctx)
	return sender.pipelineResponseSender.Send(ctx, payload)
}

func newRapidChainTransport() *rapidChainTransport {
	return &rapidChainTransport{
		frames: make(chan Session.RawFrame, 32), statuses: make(chan Session.Status, 4),
		status: Session.Status{
			State: "connected", LoggedIn: true, SocketReady: true,
			ConnectionGeneration: 1, Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC(),
		},
	}
}

func (*rapidChainTransport) Start(context.Context) error { return nil }
func (*rapidChainTransport) Stop(context.Context) error  { return nil }
func (transport *rapidChainTransport) Frames() <-chan Session.RawFrame {
	return transport.frames
}
func (transport *rapidChainTransport) StatusChanges() <-chan Session.Status {
	return transport.statuses
}
func (transport *rapidChainTransport) Status() Session.Status { return transport.status }
func (*rapidChainTransport) CorrelatesResponses() bool        { return true }
func (transport *rapidChainTransport) Send(ctx context.Context, payload []byte) error {
	transport.mu.Lock()
	transport.sends = append(transport.sends, time.Now())
	transport.mu.Unlock()
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	response, err := Protocol.Encode(Protocol.Command{
		Namespace: frame.Namespace, Opcode: frame.Opcode, Sequence: frame.ResponseText,
		Payload: frame.Payload,
	})
	if err != nil {
		return err
	}
	// Encode produces an outbound command shape; responses carry the status in
	// the fifth field instead of the command sequence.
	parts := strings.SplitN(string(response), "%", 6)
	parts[4] = "0"
	transport.frames <- Session.RawFrame{
		Payload: strings.Join(parts, "%"), Direction: Protocol.DirectionInbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1,
		ResponseToken: Outbound.MetadataFromContext(ctx).ResponseToken,
	}
	return nil
}

func (*payloadSender) Ready() bool       { return true }
func (*payloadSender) Namespace() string { return "EmpireEx_21" }
func (sender *payloadSender) Send(_ context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.payload <- frame
	return nil
}

type blockingObserver struct {
	once    sync.Once
	started chan struct{}
	frames  chan Protocol.CommittedFrame
}

type wireCleanupObserver struct {
	frames chan Protocol.CommittedFrame
	mu     sync.Mutex
	forgot map[uint64]bool
}

func (observer *blockingObserver) Watch(
	string,
	uint64,
) (<-chan Protocol.CommittedFrame, func()) {
	observer.once.Do(func() { close(observer.started) })
	return observer.frames, func() {}
}

func (observer *wireCleanupObserver) Watch(string, uint64) (<-chan Protocol.CommittedFrame, func()) {
	return observer.frames, func() {}
}

func (observer *wireCleanupObserver) WatchWire(string) (<-chan Protocol.CommittedFrame, func()) {
	return observer.frames, func() {}
}

func (*wireCleanupObserver) WaitCommitted(context.Context, uint64) (Protocol.CommittedFrame, error) {
	return Protocol.CommittedFrame{}, nil
}

func (observer *wireCleanupObserver) ForgetCommitted(ingressID uint64) {
	observer.mu.Lock()
	observer.forgot[ingressID] = true
	observer.mu.Unlock()
}

func (observer *wireCleanupObserver) forgotCommit(ingressID uint64) bool {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.forgot[ingressID]
}

type yieldingStepSender struct {
	mu       sync.Mutex
	yieldID  string
	yielded  bool
	attempts map[string]int
}

func (*yieldingStepSender) Ready() bool       { return true }
func (*yieldingStepSender) Namespace() string { return "EmpireEx_21" }

func (sender *yieldingStepSender) Send(_ context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now())
	if err != nil {
		return err
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame.Payload, &body); err != nil {
		return err
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.attempts[body.ID]++
	if body.ID == sender.yieldID && !sender.yielded {
		sender.yielded = true
		return Outbound.ErrAutomationLocked
	}
	return nil
}

func (sender *yieldingStepSender) hasYielded() bool {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.yielded
}

func (sender *yieldingStepSender) count(id string) int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.attempts[id]
}

func TestWatchAnyWireCancellationForgetsBufferedExactCommit(t *testing.T) {
	observer := &wireCleanupObserver{
		frames: make(chan Protocol.CommittedFrame, 1), forgot: map[uint64]bool{},
	}
	engine := &Engine{observer: observer}
	merged, cancel := engine.watchAnyWire(t.Context(), observer, []string{"bup"}, "")
	observer.frames <- Protocol.CommittedFrame{IngressID: 42}
	deadline := time.Now().Add(time.Second)
	for len(merged) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(merged) == 0 {
		t.Fatal("wire watcher did not buffer the observed frame")
	}
	cancel()
	if !observer.forgotCommit(42) {
		t.Fatal("cancellation retained the buffered exact-commit tracker")
	}
}

func TestEnginePlansOnceWhenClaimsAreImmediateAndRevisionIsUnchanged(t *testing.T) {
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.fast-plan", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			return Plan{
				Claims: []string{"fast-plan"},
				Steps:  []Step{{Name: "Run", Action: "test.fast-plan.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	stateReader := &countingStateReader{store: State.NewStore(State.NewGameState())}
	engine := NewEngine(registry, stateReader, nil, nil, nil)
	if err := engine.RegisterAction("test.fast-plan.run", func(context.Context, json.RawMessage) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Request{Name: "test.fast-plan"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("receipt = %#v", receipt)
	}
	if plannerCalls.Load() != 1 {
		t.Fatalf("planner calls = %d, want 1", plannerCalls.Load())
	}
	if stateReader.snapshots.Load() != 1 {
		t.Fatalf("state snapshots = %d, want 1", stateReader.snapshots.Load())
	}
}

func TestEngineReplansAfterWaitingForClaims(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.claim-holder", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{
				Claims: []string{"contended-plan"},
				Steps:  []Step{{Name: "Hold", Action: "test.claim-holder.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.contended-plan", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			return Plan{
				Claims: []string{"contended-plan"},
				Steps:  []Step{{Name: "Run", Action: "test.contended-plan.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	holderStarted := make(chan struct{})
	releaseHolder := make(chan struct{})
	if err := engine.RegisterAction("test.claim-holder.run", func(context.Context, json.RawMessage) error {
		close(holderStarted)
		<-releaseHolder
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterAction("test.contended-plan.run", func(context.Context, json.RawMessage) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	holderResult := make(chan Receipt, 1)
	go func() {
		holderResult <- engine.Submit(context.Background(), Request{Name: "test.claim-holder"})
	}()
	select {
	case <-holderStarted:
	case <-time.After(time.Second):
		t.Fatal("claim holder did not start")
	}
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{Name: "test.contended-plan"})
	}()
	waitForClaimWaiters(t, engine.claims, 1)
	close(releaseHolder)
	if receipt := <-holderResult; receipt.Status != StatusSucceeded {
		t.Fatalf("holder receipt = %#v", receipt)
	}
	select {
	case receipt := <-result:
		if receipt.Status != StatusSucceeded {
			t.Fatalf("contended receipt = %#v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("contended operation did not finish")
	}
	if plannerCalls.Load() != 2 {
		t.Fatalf("planner calls = %d, want initial plan plus claim revalidation", plannerCalls.Load())
	}
}

func TestEngineReplansWhenStateChangesAtFirstDispatchPermit(t *testing.T) {
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.dispatch-replan", Effect: EffectWrite,
		Planner: func(_ context.Context, input PlanningContext, _ json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			payload, _ := json.Marshal(map[string]int{"level": input.State.Player.Level})
			return Plan{
				Claims: []string{"dispatch-state"},
				Steps:  []Step{{Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: payload}}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	reader := &dispatchMutationStateReader{store: State.NewStore(State.NewGameState())}
	sender := &payloadSender{payload: make(chan Protocol.Frame, 1)}
	engine := NewEngine(registry, reader, nil, sender, nil)
	receipt := engine.Submit(t.Context(), Request{Name: "test.dispatch-replan"})
	if receipt.Status != StatusSucceeded || receipt.Attempt != 1 {
		t.Fatalf("dispatch-replanned receipt = %#v", receipt)
	}
	if plannerCalls.Load() != 2 {
		t.Fatalf("planner calls = %d, want 2", plannerCalls.Load())
	}
	frame := <-sender.payload
	var payload map[string]int
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["level"] != 9 {
		t.Fatalf("dispatched stale payload = %#v", payload)
	}
}

func TestEngineReplansWhenCatalogChangesAtFirstDispatchPermit(t *testing.T) {
	oldCatalog, err := GameData.DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "old"})
	if err != nil {
		t.Fatal(err)
	}
	newCatalog, err := GameData.DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "new"})
	if err != nil {
		t.Fatal(err)
	}
	provider := &rotatingGameDataProvider{first: oldCatalog, second: newCatalog}
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.catalog-replan", Effect: EffectWrite,
		Planner: func(_ context.Context, input PlanningContext, _ json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			payload, _ := json.Marshal(map[string]string{"catalog": currentCatalogVersion(input)})
			return Plan{
				Claims: []string{"catalog-plan"},
				Steps:  []Step{{Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: payload}}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &payloadSender{payload: make(chan Protocol.Frame, 1)}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), provider, sender, nil)
	receipt := engine.Submit(t.Context(), Request{Name: "test.catalog-replan"})
	if receipt.Status != StatusSucceeded || receipt.Attempt != 1 || receipt.Plan == nil || receipt.Plan.CatalogVersion != "new" {
		t.Fatalf("catalog-replanned receipt = %#v", receipt)
	}
	if plannerCalls.Load() != 2 {
		t.Fatalf("planner calls = %d, want 2", plannerCalls.Load())
	}
	frame := <-sender.payload
	var payload map[string]string
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["catalog"] != "new" {
		t.Fatalf("dispatched stale catalog payload = %#v", payload)
	}
}

func TestEngineReplansWhenStateChangesBeforeImmediateClaimAcquisition(t *testing.T) {
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	var plannedRevisions []uint64
	if err := registry.Register(Definition{
		Name: "test.changed-plan", Effect: EffectWrite,
		Planner: func(_ context.Context, input PlanningContext, _ json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			plannedRevisions = append(plannedRevisions, input.State.Revision)
			return Plan{
				Claims: []string{"changed-plan"},
				Steps:  []Step{{Name: "Run", Action: "test.changed-plan.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(State.NewGameState())
	stateReader := &countingStateReader{store: store}
	engine := NewEngine(registry, stateReader, nil, nil, nil)
	if err := engine.RegisterAction("test.changed-plan.run", func(context.Context, json.RawMessage) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	engine.SetExecutionGate(func(_ context.Context, _ Request, _ Plan, point ExecutionPoint) error {
		if point != ExecutionBeforeClaims || changed.Swap(true) {
			return nil
		}
		_, err := store.Apply(func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Player.Level++
			return []string{"player"}, true, nil
		})
		return err
	})
	receipt := engine.Submit(t.Context(), Request{Name: "test.changed-plan"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("receipt = %#v", receipt)
	}
	if plannerCalls.Load() != 2 {
		t.Fatalf("planner calls = %d, want 2", plannerCalls.Load())
	}
	if stateReader.snapshots.Load() != 2 {
		t.Fatalf("state snapshots = %d, want 2", stateReader.snapshots.Load())
	}
	if len(plannedRevisions) != 2 || plannedRevisions[0] != 0 || plannedRevisions[1] != 1 {
		t.Fatalf("planned revisions = %#v, want [0 1]", plannedRevisions)
	}
}

func TestEngineScopedDependenciesIgnoreUnrelatedStateChanges(t *testing.T) {
	tests := []struct {
		name          string
		changedCastle State.CastleID
		plannerCalls  int32
	}{
		{name: "unrelated castle", changedCastle: 12, plannerCalls: 1},
		{name: "read castle", changedCastle: 11, plannerCalls: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initial := State.NewGameState()
			initial.Session.ServerURL = "https://example.invalid"
			initial.Player.ID = 7
			initial.Castles[11] = State.CastleState{ID: 11, KingdomID: 1}
			initial.Castles[12] = State.CastleState{ID: 12, KingdomID: 1}
			store := State.NewStore(initial)
			registry := NewRegistry()
			var plannerCalls atomic.Int32
			if err := registry.Register(Definition{
				Name: "test.scoped-plan", Effect: EffectWrite,
				ReadSet: func(input PlanningContext, _ json.RawMessage, _ Plan) ([]State.PartitionKey, error) {
					return []State.PartitionKey{
						State.CastlePartition(input.State, State.CapabilityConstruction, 11),
					}, nil
				},
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					plannerCalls.Add(1)
					return Plan{Claims: []string{"scoped-plan"}, Steps: []Step{{Action: "test.scoped-plan.run"}}}, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, nil, nil)
			if err := engine.RegisterAction("test.scoped-plan.run", func(context.Context, json.RawMessage) error {
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var changed atomic.Bool
			engine.SetExecutionGate(func(_ context.Context, _ Request, _ Plan, point ExecutionPoint) error {
				if point != ExecutionBeforeClaims || changed.Swap(true) {
					return nil
				}
				_, err := store.ApplyScoped(func(state *State.GameState) (State.ScopedChange, error) {
					state.Player.Level++
					return State.ScopedChange{
						Partitions: []State.PartitionKey{
							State.CastlePartition(*state, State.CapabilityConstruction, test.changedCastle),
						},
						Changed: true,
					}, nil
				})
				return err
			})

			receipt := engine.Submit(t.Context(), Request{Name: "test.scoped-plan"})
			if receipt.Status != StatusSucceeded {
				t.Fatalf("receipt = %#v", receipt)
			}
			if got := plannerCalls.Load(); got != test.plannerCalls {
				t.Fatalf("planner calls = %d, want %d", got, test.plannerCalls)
			}
		})
	}
}

func TestEngineReplansAfterAdmissionTransition(t *testing.T) {
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.admitted-plan", Effect: EffectLaunch,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			plannerCalls.Add(1)
			return Plan{
				Admission: &Admission{Class: AdmissionAttackLaunch, Module: "test"},
				Claims:    []string{"admitted-plan"},
				Steps:     []Step{{Name: "Run", Action: "test.admitted-plan.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	stateReader := &countingStateReader{store: State.NewStore(State.NewGameState())}
	engine := NewEngine(registry, stateReader, nil, &admissionTestSender{}, nil)
	if err := engine.RegisterAction("test.admitted-plan.run", func(context.Context, json.RawMessage) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Request{Name: "test.admitted-plan"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("receipt = %#v", receipt)
	}
	if plannerCalls.Load() != 2 {
		t.Fatalf("planner calls = %d, want initial plan plus admission revalidation", plannerCalls.Load())
	}
	if stateReader.snapshots.Load() != 2 {
		t.Fatalf("state snapshots = %d, want 2", stateReader.snapshots.Load())
	}
}

func TestEngineCancelsRunningOperationByID(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.block", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"test"}, Steps: []Step{{Name: "Block", Action: "test.block"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	started := make(chan struct{})
	if err := engine.RegisterAction("test.block", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{ID: "cancel-me", Name: "test.block"})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}
	if !engine.Cancel("cancel-me") {
		t.Fatal("running operation was not cancelled")
	}
	select {
	case receipt := <-result:
		if receipt.Status != StatusCancelled {
			t.Fatalf("cancelled operation status = %q, error = %q", receipt.Status, receipt.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled operation did not stop")
	}
	if engine.Cancel("cancel-me") {
		t.Fatal("completed operation remained cancellable")
	}
}

func TestEngineStopsAwaitingResponseWhenConnectionGenerationChanges(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true,
	}
	store := State.NewStore(gameState)
	sender := &connectionGenerationSender{sent: make(chan struct{}, 1)}
	sender.generation.Store(1)
	observer := &blockingObserver{
		started: make(chan struct{}), frames: make(chan Protocol.CommittedFrame),
	}
	engine := NewEngine(nil, store, nil, sender, observer)
	result := make(chan error, 1)
	go func() {
		_, err := engine.executeStep(t.Context(), gameState.Revision, Step{
			Name: "Await command", Opcode: "ain", AwaitOpcode: "ain", TimeoutMillis: 1_000,
			Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
		})
		result <- err
	}()
	select {
	case <-sender.sent:
	case <-time.After(time.Second):
		t.Fatal("command was not sent")
	}
	sender.generation.Store(2)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "game session changed") {
			t.Fatalf("session replacement wait error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("response wait survived a session replacement")
	}
}

func TestEngineRejectsCommandBeforeReplacementSessionBaselineIsCurrent(t *testing.T) {
	for _, test := range []struct {
		name     string
		session  State.SessionState
		provider uint64
	}{
		{
			name: "controller has not applied replacement",
			session: State.SessionState{
				Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
				Status: "connected", LoggedIn: true, SocketReady: true,
			},
			provider: 2,
		},
		{
			name: "replacement baseline is pending",
			session: State.SessionState{
				Generation: 2, BaselineGeneration: 0, ConnectionGeneration: 2,
				Status: "connected", LoggedIn: true, SocketReady: true,
			},
			provider: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Session = test.session
			sender := &connectionGenerationSender{sent: make(chan struct{}, 1)}
			sender.generation.Store(test.provider)
			engine := NewEngine(nil, State.NewStore(gameState), nil, sender, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			_, err := engine.executeStep(ctx, gameState.Revision, Step{
				Name: "Unsafe command", Opcode: "ain",
				Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
			})
			if err == nil {
				t.Fatal("command was accepted without a synchronized baseline")
			}
			select {
			case <-sender.sent:
				t.Fatal("command reached the physical sender")
			default:
			}
		})
	}
}

func TestEngineWaitsForCurrentSessionBaselineBeforeReplanning(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 2, BaselineGeneration: 0, ConnectionGeneration: 2,
		Status: "connected", LoggedIn: true, SocketReady: true,
	}
	state := State.NewStore(gameState)
	sender := &connectionGenerationSender{sent: make(chan struct{}, 1)}
	sender.generation.Store(2)
	engine := NewEngine(nil, state, nil, sender, nil)
	result := make(chan error, 1)
	go func() {
		_, err := engine.executeStep(t.Context(), gameState.Revision, Step{
			Name: "Wait for baseline", Opcode: "ain",
			Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
		})
		result <- err
	}()
	time.Sleep(30 * time.Millisecond)
	_, err := state.ApplyWithoutMapMutation(func(current *State.GameState) ([]string, bool, error) {
		current.Session.BaselineGeneration = current.Session.Generation
		return []string{"session"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrPlanStale) {
			t.Fatalf("baseline readiness error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not resume after the current baseline arrived")
	}
	select {
	case <-sender.sent:
		t.Fatal("stale pre-baseline command reached the physical sender")
	default:
	}
}

func TestEngineWaitsForGameSocketBeforeReplanning(t *testing.T) {
	gameState := State.NewGameState()
	sender := &readinessSender{sent: make(chan struct{}, 1)}
	engine := NewEngine(nil, State.NewStore(gameState), nil, sender, nil)
	result := make(chan error, 1)
	go func() {
		_, err := engine.executeStep(t.Context(), gameState.Revision, Step{
			Name: "Wait for websocket", Opcode: "ain",
			Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
		})
		result <- err
	}()
	time.Sleep(30 * time.Millisecond)
	sender.ready.Store(true)
	select {
	case err := <-result:
		if !errors.Is(err, ErrPlanStale) {
			t.Fatalf("websocket readiness error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not resume after the websocket became ready")
	}
	select {
	case <-sender.sent:
		t.Fatal("pre-socket command reached the physical sender")
	default:
	}
}

func TestNormalizePlanUsesWireBarriersOnlyForStaticCommandRuns(t *testing.T) {
	command := func(name string) Step {
		return Step{
			Name: name, Opcode: "bup", AwaitOpcode: "bup",
			Command: Protocol.Command{Opcode: "bup", Payload: json.RawMessage(`{}`)},
		}
	}
	plan := normalizePlan(Definition{}, 0, Plan{Steps: []Step{
		command("one"), command("two"), command("three"),
		{Resolver: "state-dependent", AwaitOpcode: "ain"},
		command("single"),
	}})
	if plan.Steps[0].ResponseBarrier != ResponseBarrierWire ||
		plan.Steps[1].ResponseBarrier != ResponseBarrierWire ||
		plan.Steps[2].ResponseBarrier != ResponseBarrierWireThenCommitted {
		t.Fatalf("static run barriers = %q, %q, %q", plan.Steps[0].ResponseBarrier, plan.Steps[1].ResponseBarrier, plan.Steps[2].ResponseBarrier)
	}
	if plan.Steps[3].ResponseBarrier != "" || plan.Steps[4].ResponseBarrier != "" {
		t.Fatalf("state boundary barriers = %q, %q", plan.Steps[3].ResponseBarrier, plan.Steps[4].ResponseBarrier)
	}
}

func TestEngineDispatchesStaticResponseChainBeforeSlowStateCommits(t *testing.T) {
	transport := newRapidChainTransport()
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true,
		Namespace: "EmpireEx_21", ChangedAt: transport.status.ChangedAt,
	}
	store := State.NewStore(gameState)
	ingestRegistry := Ingest.NewRegistry()
	if err := ingestRegistry.Register("bup", func(
		context.Context,
		Protocol.Frame,
		*State.GameState,
		*GameData.Store,
	) ([]string, bool, error) {
		time.Sleep(60 * time.Millisecond)
		return []string{"production"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, ingestRegistry)
	root, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	controller := Session.NewController(root, transport, pipeline, store)
	if err := controller.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Stop(context.Background()) }()

	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.rapid-chain", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			steps := make([]Step, 6)
			for index := range steps {
				payload, _ := json.Marshal(map[string]int{"slot": index})
				steps[index] = Step{
					Name: "enqueue", Opcode: "bup", AwaitOpcode: "bup",
					TimeoutMillis: 2_000, SuccessCodes: []int{0},
					Command: Protocol.Command{Opcode: "bup", Payload: payload},
				}
			}
			return Plan{Claims: []string{"production"}, Steps: steps}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, controller, pipeline)
	started := time.Now()
	receipt := engine.Submit(t.Context(), Request{Name: "test.rapid-chain", Actor: "ui"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("rapid-chain receipt = %#v", receipt)
	}
	transport.mu.Lock()
	sends := append([]time.Time(nil), transport.sends...)
	transport.mu.Unlock()
	if len(sends) != 6 {
		t.Fatalf("physical sends = %d, want 6", len(sends))
	}
	span := sends[len(sends)-1].Sub(sends[0])
	if span >= 300*time.Millisecond {
		t.Fatalf("six-command chain dispatch span = %s, want under 300ms", span)
	}
	if time.Since(started) < 300*time.Millisecond {
		t.Fatalf("operation returned before the slow final state barrier committed")
	}
}

func TestEngineAcceptsEquivalentUntaggedRepliesOnlyForReadIntents(t *testing.T) {
	for _, test := range []struct {
		name       string
		effect     Effect
		wantStatus Status
		wantToken  bool
	}{
		{name: "read", effect: EffectRead, wantStatus: StatusSucceeded},
		{name: "write", effect: EffectWrite, wantStatus: StatusIndeterminate, wantToken: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := State.NewStore(State.NewGameState())
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &correlatingPipelineResponseSender{
				pipelineResponseSender: &pipelineResponseSender{pipeline: pipeline},
				metadata:               make(chan Outbound.Metadata, 1),
			}
			registry := NewRegistry()
			if err := registry.Register(Definition{
				Name: "test.untagged-" + test.name, Effect: test.effect,
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					return Plan{Steps: []Step{{
						Name: "Refresh", Opcode: "gam", AwaitOpcode: "gam", TimeoutMillis: 100,
						SuccessCodes: []int{0}, Command: Protocol.Command{Opcode: "gam", Payload: json.RawMessage(`{}`)},
					}}}, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, sender, pipeline)
			receipt := engine.Submit(t.Context(), Request{Name: "test.untagged-" + test.name})
			if receipt.Status != test.wantStatus {
				t.Fatalf("untagged %s receipt = %#v", test.name, receipt)
			}
			metadata := <-sender.metadata
			if got := metadata.ResponseToken != ""; got != test.wantToken {
				t.Fatalf("untagged %s response token present = %t, want %t", test.name, got, test.wantToken)
			}
		})
	}
}

func TestEngineCommitsAcknowledgedWireStepBeforeCancellationReleasesClaims(t *testing.T) {
	reducerStarted := make(chan struct{})
	releaseReducer := make(chan struct{})
	ingestRegistry := Ingest.NewRegistry()
	if err := ingestRegistry.Register("bup", func(
		context.Context,
		Protocol.Frame,
		*State.GameState,
		*GameData.Store,
	) ([]string, bool, error) {
		close(reducerStarted)
		<-releaseReducer
		return []string{"production"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(State.NewGameState())
	pipeline := Ingest.NewPipeline(store, nil, ingestRegistry)
	sender := &pipelineResponseSender{pipeline: pipeline}
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.cancel-wire", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			step := func() Step {
				return Step{
					Opcode: "bup", AwaitOpcode: "bup", TimeoutMillis: 1_000,
					SuccessCodes: []int{0}, Command: Protocol.Command{Opcode: "bup", Payload: json.RawMessage(`{}`)},
				}
			}
			return Plan{Claims: []string{"production"}, Steps: []Step{step(), step()}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, sender, pipeline)
	stepGates := 0
	engine.SetExecutionGate(func(_ context.Context, _ Request, _ Plan, point ExecutionPoint) error {
		if point != ExecutionBeforeStep {
			return nil
		}
		stepGates++
		if stepGates == 2 {
			return context.Canceled
		}
		return nil
	})
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{Name: "test.cancel-wire"})
	}()
	<-reducerStarted
	select {
	case receipt := <-result:
		t.Fatalf("operation released before acknowledged state committed: %#v", receipt)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseReducer)
	select {
	case receipt := <-result:
		if receipt.Status != StatusPartiallySucceeded {
			t.Fatalf("partially completed wire receipt = %#v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("operation did not finish after acknowledged state committed")
	}
}

func TestEnginePreservesIndeterminateWriteOutcome(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.indeterminate", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{
				Name: "Possibly sent", Opcode: "ain",
				Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
			}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, &indeterminateSender{}, nil)
	receipt := engine.Submit(t.Context(), Request{Name: "test.indeterminate"})
	if receipt.Status != StatusIndeterminate {
		t.Fatalf("indeterminate receipt = %#v", receipt)
	}
	if !strings.Contains(receipt.Error, Outbound.ErrIndeterminate.Error()) {
		t.Fatalf("indeterminate error = %q", receipt.Error)
	}
}

func TestEnginePropagatesResolvedPriorityToOutboundSend(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.send", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{
				Name: "Send", Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
			}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &metadataSender{metadata: make(chan Outbound.Metadata, 1), generation: 7}
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 7,
		Status: "connected", LoggedIn: true, SocketReady: true,
	}
	engine := NewEngine(registry, State.NewStore(gameState), nil, sender, nil)
	receipt := engine.Submit(context.Background(), Request{Name: "test.send", Actor: "automation:autoBird"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("receipt = %#v", receipt)
	}
	if receipt.Priority != Outbound.PriorityAutoBird {
		t.Fatalf("receipt priority = %d", receipt.Priority)
	}
	metadata := <-sender.metadata
	if metadata.Priority != Outbound.PriorityAutoBird || metadata.Actor != "automation:autoBird" || metadata.Intent != "test.send" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.ConnectionGeneration != 7 {
		t.Fatalf("connection generation = %d, want 7", metadata.ConnectionGeneration)
	}
	if !metadata.SubmittedAt.Equal(receipt.SubmittedAt) {
		t.Fatalf("metadata submission time = %s, want %s", metadata.SubmittedAt, receipt.SubmittedAt)
	}
	sender.metadata = make(chan Outbound.Metadata, 1)
	receipt = engine.Submit(context.Background(), Request{
		Name: "test.send", Actor: "api", Priority: 77,
	})
	if receipt.Status != StatusSucceeded || receipt.Priority != 77 {
		t.Fatalf("explicit-priority receipt = %#v", receipt)
	}
	if metadata := <-sender.metadata; metadata.Priority != 77 {
		t.Fatalf("explicit metadata priority = %d", metadata.Priority)
	}
}

func TestEngineRejectsOutOfRangePriority(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test", Effect: EffectRead,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	receipt := engine.Submit(context.Background(), Request{Name: "test", Priority: 101})
	if receipt.Status != StatusFailed || receipt.Error != "priority must be between 1 and 100" {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func TestEngineQueuesAttackAdmissionBeforeAcquiringClaims(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.attack", Effect: EffectLaunch,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{
				Admission: &Admission{Class: AdmissionAttackLaunch, Module: "testAttack"},
				Claims:    []string{"shared-focus"}, Steps: []Step{{Name: "Run attack", Action: "test.attack.run"}},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Definition{
		Name: "test.focus", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"shared-focus"}, Steps: []Step{{Name: "Use focus", Action: "test.focus.run"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &admissionTestSender{nextAllowed: time.Now().UTC().Add(70 * time.Millisecond)}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, sender, nil)
	attackStarted := make(chan struct{})
	if err := engine.RegisterAction("test.attack.run", func(context.Context, json.RawMessage) error {
		close(attackStarted)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterAction("test.focus.run", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	updates, unsubscribe := engine.Subscribe(16)
	defer unsubscribe()
	attackResult := make(chan Receipt, 1)
	go func() {
		attackResult <- engine.Submit(t.Context(), Request{ID: "queued-attack", Name: "test.attack", Actor: "automation:test"})
	}()
	waitForIntentStatus(t, updates, StatusQueued)
	interactive := engine.Submit(t.Context(), Request{ID: "focus", Name: "test.focus", Actor: "ui"})
	if interactive.Status != StatusSucceeded {
		t.Fatalf("interactive operation = %#v", interactive)
	}
	select {
	case <-attackStarted:
	case <-time.After(time.Second):
		t.Fatal("queued attack was not admitted")
	}
	if receipt := <-attackResult; receipt.Status != StatusSucceeded {
		t.Fatalf("attack receipt = %#v", receipt)
	}
}

func TestEngineStepResolverReadsStateCommittedByEarlierSteps(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.resolve", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{
				{Name: "Update state", Action: "test.state.update"},
				{Name: "Resolve command", Resolver: "test.command.resolve"},
			}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(State.NewGameState())
	sender := &payloadSender{payload: make(chan Protocol.Frame, 1)}
	engine := NewEngine(registry, store, nil, sender, nil)
	if err := engine.RegisterAction("test.state.update", func(context.Context, json.RawMessage) error {
		_, err := store.Apply(func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Player.Level = 9
			return []string{"player"}, true, nil
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterStepResolver("test.command.resolve", func(_ context.Context, input PlanningContext, _ json.RawMessage) (Step, error) {
		payload, _ := json.Marshal(map[string]int{"level": input.State.Player.Level})
		return Step{Name: "Resolved command", Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: payload}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Request{Name: "test.resolve"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("resolver receipt = %#v", receipt)
	}
	frame := <-sender.payload
	var payload map[string]int
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["level"] != 9 {
		t.Fatalf("resolved payload = %#v", payload)
	}
}

func TestNormalizePlanClaimsAwaitedResponseOpcodes(t *testing.T) {
	plan := normalizePlan(Definition{Name: "test", Effect: EffectRead}, 1, Plan{
		Claims: []string{"state"},
		Steps:  []Step{{AwaitOpcode: "CRA"}, {AwaitOpcodes: []string{"ain", "CRA"}}},
	})
	wanted := map[string]bool{"state": true, "attack-context": true, "response:cra": true, "response:ain": true}
	if len(plan.Claims) != len(wanted) {
		t.Fatalf("claims = %#v", plan.Claims)
	}
	for _, claim := range plan.Claims {
		if !wanted[claim] {
			t.Fatalf("unexpected claim %q in %#v", claim, plan.Claims)
		}
	}
	if plan.Admission == nil || plan.Admission.Class != AdmissionAttackLaunch || plan.Admission.Module != "test" {
		t.Fatalf("attack admission = %#v", plan.Admission)
	}
}

func TestEngineRejectsExecutableProductionPlanWithoutEffectResource(t *testing.T) {
	registry := NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Definition{
		Name: "test.missing-resource", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{Action: "test.noop"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	if err := engine.RegisterAction("test.noop", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Request{Name: "test.missing-resource"})
	if receipt.Status != StatusFailed || !strings.Contains(receipt.Error, "declares no effect resource") {
		t.Fatalf("missing-resource receipt = %#v", receipt)
	}
}

func TestEngineRejectsExecutableProductionPlanWithUnmappedLegacyClaim(t *testing.T) {
	registry := NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Definition{
		Name: "test.unmapped-resource", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"misspelled-resource"}, Steps: []Step{{Action: "test.noop"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	receipt := engine.Submit(t.Context(), Request{Name: "test.unmapped-resource"})
	if receipt.Status != StatusFailed || !strings.Contains(receipt.Error, "unmapped legacy claim") {
		t.Fatalf("unmapped-resource receipt = %#v", receipt)
	}
}

func TestEngineBoundsDurableInMemoryOperationHistory(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, nil)
	engine.mu.Lock()
	engine.operationStore = &SQLiteOperationStore{}
	for index := 0; index < operationHistoryLimit+5; index++ {
		id := fmt.Sprintf("operation-%d", index)
		engine.cacheOperationLocked(Receipt{ID: id, Status: StatusSucceeded}, "hash-"+id)
	}
	operationCount := len(engine.operations)
	hashCount := len(engine.requestHashes)
	orderCount := len(engine.operationOrder)
	_, oldestPresent := engine.operations["operation-0"]
	_, newestPresent := engine.operations[fmt.Sprintf("operation-%d", operationHistoryLimit+4)]
	engine.mu.Unlock()
	if operationCount != operationHistoryLimit || hashCount != operationHistoryLimit || orderCount != operationHistoryLimit {
		t.Fatalf("bounded history sizes = operations:%d hashes:%d order:%d", operationCount, hashCount, orderCount)
	}
	if oldestPresent || !newestPresent {
		t.Fatalf("bounded history retained wrong entries: oldest=%t newest=%t", oldestPresent, newestPresent)
	}
}

func TestEngineResumesCheckpointAndRebuildsOnlyContextSteps(t *testing.T) {
	registry := NewRegistry()
	var plannerCalls atomic.Int32
	if err := registry.Register(Definition{
		Name: "test.resumable", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			if plannerCalls.Add(1) > 1 {
				return Plan{}, errors.New("partially executed plan was rebuilt")
			}
			return Plan{Claims: []string{"shared-context"}, Steps: []Step{
				RebuildOnResume(testCommandStep("setup")),
				testCommandStep("effect-1"),
				testCommandStep("effect-2"),
			}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Definition{
		Name: "test.interactive", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"shared-context"}, Steps: []Step{testCommandStep("interactive")}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &yieldingStepSender{yieldID: "effect-2", attempts: map[string]int{}}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, sender, nil)
	resume := make(chan struct{})
	engine.SetExecutionGate(func(ctx context.Context, request Request, _ Plan, point ExecutionPoint) error {
		if request.Actor != "automation:autoBird" || point != ExecutionBeforeClaims || !sender.hasYielded() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resume:
			return nil
		}
	})
	updates, unsubscribe := engine.Subscribe(16)
	defer unsubscribe()
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{
			ID: "resumable", Name: "test.resumable", Actor: "automation:autoBird",
		})
	}()
	waitForIntentStatus(t, updates, StatusPaused)
	if sender.count("setup") != 1 || sender.count("effect-1") != 1 || sender.count("effect-2") != 1 {
		t.Fatalf(
			"attempts before resume: setup=%d effect-1=%d effect-2=%d",
			sender.count("setup"), sender.count("effect-1"), sender.count("effect-2"),
		)
	}
	interactive := engine.Submit(context.Background(), Request{
		ID: "interactive", Name: "test.interactive", Actor: "ui",
	})
	if interactive.Status != StatusSucceeded {
		t.Fatalf("interactive receipt = %#v", interactive)
	}
	close(resume)
	select {
	case receipt := <-result:
		if receipt.Status != StatusSucceeded {
			t.Fatalf("resumed receipt = %#v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("paused operation did not resume")
	}
	if sender.count("setup") != 2 {
		t.Fatalf("setup attempts = %d, want 2", sender.count("setup"))
	}
	if sender.count("effect-1") != 1 {
		t.Fatalf("completed effect attempts = %d, want 1", sender.count("effect-1"))
	}
	if sender.count("effect-2") != 2 {
		t.Fatalf("yielded effect attempts = %d, want 2", sender.count("effect-2"))
	}
	if plannerCalls.Load() != 1 {
		t.Fatalf("planner calls = %d, want only the initial plan", plannerCalls.Load())
	}
}

func TestEngineCanCancelWhilePaused(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.cancel-paused", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"cancel-context"}, Steps: []Step{testCommandStep("yield")}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &yieldingStepSender{yieldID: "yield", attempts: map[string]int{}}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, sender, nil)
	engine.SetExecutionGate(func(ctx context.Context, _ Request, _ Plan, point ExecutionPoint) error {
		if point != ExecutionBeforeClaims || !sender.hasYielded() {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	})
	updates, unsubscribe := engine.Subscribe(8)
	defer unsubscribe()
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{ID: "cancel-paused", Name: "test.cancel-paused"})
	}()
	waitForIntentStatus(t, updates, StatusPaused)
	if !engine.Cancel("cancel-paused") {
		t.Fatal("paused operation was not cancellable")
	}
	select {
	case receipt := <-result:
		if receipt.Status != StatusCancelled {
			t.Fatalf("cancelled paused receipt = %#v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled paused operation did not stop")
	}
}

func TestIntentSubscriberMarksGapAndDeliversLatestReceipt(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, nil)
	updates, unsubscribe := engine.Subscribe(1)
	defer unsubscribe()
	engine.publish(Receipt{ID: "first", Status: StatusRunning})
	engine.publish(Receipt{ID: "second", Status: StatusSucceeded})

	receipt := <-updates
	if receipt.ID != "second" || receipt.StreamSequence != 2 || !receipt.StreamGap {
		t.Fatalf("coalesced receipt = %#v", receipt)
	}
}

func testCommandStep(id string) Step {
	payload, _ := json.Marshal(map[string]string{"id": id})
	return Step{Name: id, Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: payload}}
}

type dependencyOrderSender struct {
	events *[]string
}

func (sender *dependencyOrderSender) Ready() bool       { return true }
func (sender *dependencyOrderSender) Namespace() string { return "EmpireEx_21" }
func (sender *dependencyOrderSender) Send(_ context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	*sender.events = append(*sender.events, "send:"+frame.Opcode)
	return nil
}

func TestDeferredCommandRunsRegisteredDependenciesBeforeBuildAndSend(t *testing.T) {
	events := []string{}
	store := State.NewStore(State.NewGameState())
	engine := NewEngine(nil, store, nil, &dependencyOrderSender{events: &events}, nil)
	if err := engine.RegisterAction("test.cra.guard", func(context.Context, json.RawMessage) error {
		events = append(events, "guard")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterCommandDependencies("cra", func(
		context.Context, PlanningContext, Step,
	) (CommandDependencyPlan, error) {
		return CommandDependencyPlan{Key: "route", Steps: []Step{{Action: "test.cra.guard"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterStepResolver("test.cra.build", func(
		context.Context, PlanningContext, json.RawMessage,
	) (Step, error) {
		events = append(events, "build")
		return Step{Opcode: "cra", Command: Protocol.Command{Opcode: "cra", Payload: json.RawMessage(`{"route":"same"}`)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := engine.executeStep(t.Context(), store.Revision(), Step{
		Resolver:            "test.cra.build",
		CommandDependencies: &CommandDependencyRequest{Opcode: "cra", Payload: json.RawMessage(`{"route":"same"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(events, ","), "guard,build,send:cra"; got != want {
		t.Fatalf("execution order = %q, want %q", got, want)
	}
}

func waitForIntentStatus(t *testing.T, updates <-chan Receipt, wanted Status) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case receipt := <-updates:
			if receipt.Status == wanted {
				return
			}
		case <-timer.C:
			t.Fatalf("intent status did not reach %q", wanted)
		}
	}
}
