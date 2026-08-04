package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type stormMapBurstTestObserver struct {
	mu        sync.Mutex
	watchers  map[string]chan Protocol.CommittedFrame
	committed map[uint64]Protocol.CommittedFrame
	waited    []uint64
}

func newStormMapBurstTestObserver() *stormMapBurstTestObserver {
	return &stormMapBurstTestObserver{
		watchers:  map[string]chan Protocol.CommittedFrame{},
		committed: map[uint64]Protocol.CommittedFrame{},
	}
}

func (observer *stormMapBurstTestObserver) WatchWireResponse(
	_ string,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	channel := make(chan Protocol.CommittedFrame, 1)
	observer.mu.Lock()
	observer.watchers[responseToken] = channel
	observer.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			observer.mu.Lock()
			delete(observer.watchers, responseToken)
			observer.mu.Unlock()
		})
	}
}

func (observer *stormMapBurstTestObserver) WaitCommitted(
	_ context.Context,
	ingressID uint64,
) (Protocol.CommittedFrame, error) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	frame, exists := observer.committed[ingressID]
	if !exists {
		return Protocol.CommittedFrame{}, errors.New("commit is unavailable")
	}
	delete(observer.committed, ingressID)
	observer.waited = append(observer.waited, ingressID)
	return frame, nil
}

func (observer *stormMapBurstTestObserver) ForgetCommitted(ingressID uint64) {
	observer.mu.Lock()
	delete(observer.committed, ingressID)
	observer.mu.Unlock()
}

func (observer *stormMapBurstTestObserver) watcherCount() int {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return len(observer.watchers)
}

func (observer *stormMapBurstTestObserver) deliver(responseToken string, ingressID uint64) {
	code := 0
	frame := Protocol.CommittedFrame{
		Frame: Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &code,
			ResponseToken: responseToken, ReceivedAt: time.Now().UTC(),
		},
		IngressID: ingressID,
		Revision:  ingressID,
	}
	observer.mu.Lock()
	channel := observer.watchers[responseToken]
	observer.committed[ingressID] = frame
	observer.mu.Unlock()
	channel <- frame
}

type stormMapBurstTestSender struct {
	observer            *stormMapBurstTestObserver
	expectedSends       int
	watchersAtFirstSend int
	metadata            []Outbound.Metadata
	frames              []Protocol.Frame
}

func (*stormMapBurstTestSender) CorrelatesResponses() bool { return true }
func (*stormMapBurstTestSender) Namespace() string         { return "EmpireEx_21" }
func (*stormMapBurstTestSender) WaitForAutomationUnlocked(context.Context) error {
	return nil
}

func (sender *stormMapBurstTestSender) Send(ctx context.Context, payload []byte) error {
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(sender.frames) == 0 {
		sender.watchersAtFirstSend = sender.observer.watcherCount()
	}
	sender.frames = append(sender.frames, frame)
	sender.metadata = append(sender.metadata, Outbound.MetadataFromContext(ctx))
	if len(sender.frames) == sender.expectedSends {
		for index := len(sender.metadata) - 1; index >= 0; index-- {
			sender.observer.deliver(sender.metadata[index].ResponseToken, uint64(index+1))
		}
	}
	return nil
}

func TestPlanStormCastleUnlockUsesCapturedOfficialWireShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"prebuiltcastles":[
			{"preBuiltCastleID":"16","comment2":"CheapCamp","spaceIDs":"4","minLevel":35,"costWood":10000,"costStone":10000,"costFood":2500,"costC1":5000},
			{"preBuiltCastleID":"18","comment2":"C2Camp","spaceIDs":"4","minLevel":35,"costC2":59000}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Player.ID = 42
	state.Player.Level = 70
	state.KingdomTransport.ObservedAt = time.Now().UTC()
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4}
	input := Intent.PlanningContext{State: state, GameData: gameData}

	plan, err := planStormCastleUnlock(t.Context(), input, json.RawMessage(`{"prebuiltCastleId":16}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "kpi" ||
		plan.Steps[1].Action != "storm.castle.unlock.verify" || plan.Steps[2].Opcode != "ksc" ||
		plan.Steps[3].Opcode != "gcl" || plan.Steps[4].Opcode != "kpi" {
		t.Fatalf("Storm castle unlock steps = %#v", plan.Steps)
	}
	if got := string(plan.Steps[2].Command.Payload); got != `{"ID":16,"D":0,"PWR":0,"OC2":0,"SID":4}` {
		t.Fatalf("non-premium Storm unlock payload = %s", got)
	}
	if got := string(plan.Steps[3].Command.Payload); got != `{"PID":42}` {
		t.Fatalf("Storm castle refresh payload = %s", got)
	}

	premiumPlan, err := planStormCastleUnlock(t.Context(), input, json.RawMessage(`{"prebuiltCastleId":18}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(premiumPlan.Steps[2].Command.Payload); got != `{"ID":18,"D":0,"PWR":0,"OC2":1,"SID":4}` {
		t.Fatalf("premium Storm unlock payload = %s", got)
	}
}

func TestPlanStormCastleRefreshRequestsCurrentPlayerDirectory(t *testing.T) {
	state := State.NewGameState()
	state.Player.ID = 42
	plan, err := planStormCastleRefresh(t.Context(), Intent.PlanningContext{State: state}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "gcl" ||
		string(plan.Steps[0].Command.Payload) != `{"PID":42}` ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("Storm castle refresh plan = %#v", plan)
	}
}

func TestStormAttackReplansWhenCommanderAvailabilityChanges(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 100, Y: 100, Focused: true}
	state.Commanders[43] = State.CommanderState{ID: 43, Available: false}
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: stormIntentKingdomID, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 7, StormVictoryCount: 5, ObservedAt: now,
		},
	}
	arguments := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"commanderIds":[43],
		"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)

	noOp, err := planStormAttack(t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(noOp.Steps) != 0 || !strings.Contains(noOp.Summary, "Skip Storm attack") {
		t.Fatalf("busy-commander Storm plan = %#v", noOp)
	}

	commander := state.Commanders[43]
	commander.Available = true
	state.Commanders[43] = commander
	plan, err := planStormAttack(t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var resolverArguments json.RawMessage
	for _, step := range plan.Steps {
		if step.Resolver == "storm.attack.build" {
			resolverArguments = step.ResolverArguments
			break
		}
	}
	if len(resolverArguments) == 0 {
		t.Fatalf("Storm attack has no deferred launch step: %#v", plan.Steps)
	}
	commander.Available = false
	state.Commanders[43] = commander
	if _, err := (&Application{}).resolveStormAttackStep(
		t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, resolverArguments,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("busy commander should make the Storm plan stale: %v", err)
	}
}

func TestStormMapScanWindowsCoverSixHundredBySixHundredInThirtySixRequests(t *testing.T) {
	windows := stormMapScanWindows(State.StormMapBounds{X1: 0, Y1: 0, X2: 605, Y2: 605})
	if len(windows) != 36 {
		t.Fatalf("window count = %d, want 36", len(windows))
	}
	if first := windows[0]; first != (towerMapWindow{X1: 0, Y1: 0, X2: 100, Y2: 100}) {
		t.Fatalf("first window = %#v", first)
	}
	if last := windows[len(windows)-1]; last != (towerMapWindow{X1: 505, Y1: 505, X2: 605, Y2: 605}) {
		t.Fatalf("last window = %#v", last)
	}
	for _, window := range windows {
		if width, height := window.X2-window.X1+1, window.Y2-window.Y1+1; width > 101 || height > 101 {
			t.Fatalf("oversized GAA window = %#v", window)
		}
	}
}

func TestStormMapConcentricRingsExpandOutwardFromSixFifty(t *testing.T) {
	ringZero := stormMapConcentricRingWindows(0)
	if len(ringZero) != 1 || ringZero[0] != (towerMapWindow{X1: 600, Y1: 600, X2: 700, Y2: 700}) {
		t.Fatalf("center ring = %#v", ringZero)
	}
	ringOne := stormMapConcentricRingWindows(1)
	if len(ringOne) != 8 {
		t.Fatalf("first outer ring has %d windows, want 8", len(ringOne))
	}
	ringTwo := stormMapConcentricRingWindows(2)
	if len(ringTwo) != 16 {
		t.Fatalf("second outer ring has %d windows, want 16", len(ringTwo))
	}
	if bounds := stormMapConcentricBounds(1); bounds != (State.StormMapBounds{X1: 500, Y1: 500, X2: 800, Y2: 800}) {
		t.Fatalf("first expanded bounds = %#v", bounds)
	}
	if bounds := stormMapConcentricBounds(2); bounds != (State.StormMapBounds{X1: 400, Y1: 400, X2: 900, Y2: 900}) {
		t.Fatalf("second expanded bounds = %#v", bounds)
	}
	windows := append(append(ringZero, ringOne...), ringTwo...)
	totalArea := 0
	for index, window := range windows {
		totalArea += (window.X2 - window.X1 + 1) * (window.Y2 - window.Y1 + 1)
		for priorIndex := 0; priorIndex < index; priorIndex++ {
			prior := windows[priorIndex]
			if window.X1 <= prior.X2 && window.X2 >= prior.X1 &&
				window.Y1 <= prior.Y2 && window.Y2 >= prior.Y1 {
				t.Fatalf("concentric windows overlap: %#v and %#v", prior, window)
			}
		}
	}
	if want := 501 * 501; totalArea != want {
		t.Fatalf("concentric window area = %d, want %d", totalArea, want)
	}
}

func TestStormMapSweepExpandsOnlyForTargetsInsideEdgeBuffer(t *testing.T) {
	startedAt := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"500:650": {
			KingdomID: stormIntentKingdomID, X: 500, Y: 650, TypeID: 12,
			ObservedAt: startedAt.Add(time.Second),
		},
	}
	state.Storm.Map.Targets = map[string]State.MapObservation{
		"526:650": {
			KingdomID: stormIntentKingdomID, X: 526, Y: 650, TypeID: stormIntentFortMapTypeID,
		},
	}
	bounds := State.StormMapBounds{X1: 500, Y1: 500, X2: 800, Y2: 800}
	if stormMapSweepNeedsExpansion(state, bounds, startedAt) {
		t.Fatal("target 26 coordinates inside the edge unexpectedly expanded the sweep")
	}

	target := state.Storm.Map.Targets["526:650"]
	target.X = 525
	state.Storm.Map.Targets = map[string]State.MapObservation{"525:650": target}
	if !stormMapSweepNeedsExpansion(state, bounds, startedAt) {
		t.Fatal("target on the 25-coordinate lower margin did not expand the sweep")
	}
	target.X = 775
	state.Storm.Map.Targets = map[string]State.MapObservation{"775:650": target}
	if !stormMapSweepNeedsExpansion(state, bounds, startedAt) {
		t.Fatal("target on the 25-coordinate upper margin did not expand the sweep")
	}

	target.X = 520
	state.Storm.Map.Targets = map[string]State.MapObservation{"520:650": target}
	if stormMapSweepNeedsExpansion(
		state, State.StormMapBounds{X1: 400, Y1: 400, X2: 900, Y2: 900}, startedAt,
	) {
		t.Fatal("target 120 coordinates inside the expanded edge unexpectedly enlarged the sweep")
	}
}

func TestPlanFullStormMapScanUsesOneLeaseHeldBurstStep(t *testing.T) {
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{
		ID: 40, KingdomID: stormIntentKingdomID, X: 100, Y: 50, Focused: true,
	}
	request := json.RawMessage(`{
		"sourceCastleId":40,"fullMap":true,
		"bounds":{"x1":600,"y1":600,"x2":700,"y2":700}
	}`)
	plan, err := planStormMapScan(t.Context(), Intent.PlanningContext{State: state}, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("full Storm map steps = %#v", plan.Steps)
	}
	for index, action := range []string{"storm.scan.begin", "storm.scan.burst"} {
		if plan.Steps[index].Action != action || plan.Steps[index].Opcode != "" {
			t.Fatalf("full Storm map step %d = %#v, want action %q", index, plan.Steps[index], action)
		}
	}
}

func TestStormMapGAABurstRegistersIndependentSlotsBeforeSending(t *testing.T) {
	windows := []towerMapWindow{
		{X1: 0, Y1: 0, X2: 100, Y2: 100},
		{X1: 101, Y1: 0, X2: 201, Y2: 100},
		{X1: 202, Y1: 0, X2: 302, Y2: 100},
	}
	observer := newStormMapBurstTestObserver()
	sender := &stormMapBurstTestSender{
		observer: observer, expectedSends: len(windows),
	}
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{
		OperationID: "storm-burst-test", Actor: "automation:autoStorm",
	})
	if err := runStormMapGAABurst(
		ctx, sender, observer, stormIntentKingdomID, windows, stormMapBurstResponseTimeout,
	); err != nil {
		t.Fatal(err)
	}
	if sender.watchersAtFirstSend != len(windows) {
		t.Fatalf("watchers at first GAA send = %d, want %d", sender.watchersAtFirstSend, len(windows))
	}
	if observer.watcherCount() != 0 {
		t.Fatalf("response watchers after burst = %d, want 0", observer.watcherCount())
	}
	if len(sender.frames) != len(windows) || len(sender.metadata) != len(windows) {
		t.Fatalf("burst sends = %d frames and %d metadata entries", len(sender.frames), len(sender.metadata))
	}
	tokens := map[string]struct{}{}
	for index := range sender.frames {
		if sender.frames[index].Opcode != "gaa" {
			t.Fatalf("burst opcode %d = %q", index, sender.frames[index].Opcode)
		}
		metadata := sender.metadata[index]
		if metadata.ResponseTimeoutMillis != 15_000 || len(metadata.ResponseOpcodes) != 1 ||
			metadata.ResponseOpcodes[0] != "gaa" || metadata.ResponseToken == "" {
			t.Fatalf("burst response metadata %d = %#v", index, metadata)
		}
		if _, duplicate := tokens[metadata.ResponseToken]; duplicate {
			t.Fatalf("duplicate GAA response token %q", metadata.ResponseToken)
		}
		tokens[metadata.ResponseToken] = struct{}{}
	}
	observer.mu.Lock()
	waited := append([]uint64(nil), observer.waited...)
	observer.mu.Unlock()
	if len(waited) != 3 || waited[0] != 1 || waited[1] != 2 || waited[2] != 3 {
		t.Fatalf("committed response slots = %v, want [1 2 3]", waited)
	}
}

func TestPlanStormMapScanRejectsSecondAttemptInsideTwoHours(t *testing.T) {
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 300, Y: 300, Focused: true}
	state.Storm.Map = State.StormMapState{
		SourceCastleID: 40,
		LastAttemptAt:  time.Now().UTC(),
		Targets:        map[string]State.MapObservation{},
	}
	request := json.RawMessage(`{
		"sourceCastleId":40,"fullMap":true,
		"bounds":{"x1":600,"y1":600,"x2":700,"y2":700}
	}`)
	if _, err := planStormMapScan(context.Background(), Intent.PlanningContext{State: state}, request); err == nil ||
		!strings.Contains(err.Error(), "one attempt every two hours") {
		t.Fatalf("second full-map attempt error = %v", err)
	}
}

func TestCaptureStormScanBuildsAuthoritativeMapState(t *testing.T) {
	startedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ServerURL = "storm-test.example"
	state.Player.ID = 99
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 300, Y: 300, Focused: true}
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"420:420": {
			KingdomID: stormIntentKingdomID, X: 420, Y: 420, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 7, ObservedAt: startedAt.Add(-time.Second),
		},
		"650:650": {
			KingdomID: stormIntentKingdomID, X: 650, Y: 650, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 8, ObservedAt: startedAt.Add(time.Second),
		},
		"899:899": {
			KingdomID: stormIntentKingdomID, X: 899, Y: 899, TypeID: 12,
			ObservedAt: startedAt.Add(2 * time.Second),
		},
	}
	application := &Application{State: State.NewStore(state)}
	scanRequest := stormMapScanRequest{
		SourceCastleID: 40,
		FullMap:        true,
		Bounds:         State.StormMapBounds{X1: 600, Y1: 600, X2: 700, Y2: 700},
		ScanStartedAt:  startedAt,
	}
	request, err := json.Marshal(scanRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.beginStormScan(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		castle := gameState.Castles[40]
		castle.Focused = false
		gameState.Castles[40] = castle
		return []string{"castles"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	scanRequest.Bounds = State.StormMapBounds{X1: 400, Y1: 400, X2: 900, Y2: 900}
	if err := application.captureStormScanRequest(scanRequest); err != nil {
		t.Fatalf("capture after focus changed: %v", err)
	}

	snapshot := application.State.Snapshot()
	if _, stale := snapshot.Map[stormIntentKingdomID]["420:420"]; stale {
		t.Fatal("stale Storm fort survived authoritative sweep")
	}
	if len(snapshot.Storm.Map.Targets) != 1 || snapshot.Storm.Map.Targets["650:650"].StormIsleID != 8 {
		t.Fatalf("authoritative targets = %#v", snapshot.Storm.Map.Targets)
	}
	if snapshot.Storm.Map.WindowCount != 25 ||
		snapshot.Storm.Map.CoveredBounds != (State.StormMapBounds{X1: 400, Y1: 400, X2: 900, Y2: 900}) ||
		snapshot.Storm.Map.NextBounds != snapshot.Storm.Map.CoveredBounds {
		t.Fatalf("Storm map coverage = %#v", snapshot.Storm.Map)
	}
	if !snapshot.Storm.Map.LastAttemptAt.Equal(startedAt) || snapshot.Storm.Map.LastCompletedAt.IsZero() {
		t.Fatalf("Storm scan timing = %#v", snapshot.Storm.Map)
	}
}

func TestCaptureTargetedStormScanRefreshesTrackedCooldown(t *testing.T) {
	startedAt := time.Date(2026, time.July, 21, 19, 18, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, Focused: true}
	state.Storm.Map = State.StormMapState{
		SourceCastleID: 40,
		Targets: map[string]State.MapObservation{
			"612:667": {
				KingdomID: 4, X: 612, Y: 667, TypeID: stormIntentFortMapTypeID,
				StormIsleID: 9, ObservedAt: startedAt.Add(-time.Hour),
			},
		},
	}
	readyAt := startedAt.Add(10 * time.Hour)
	state.Map[4] = map[string]State.MapObservation{
		"612:667": {
			KingdomID: 4, X: 612, Y: 667, TypeID: stormIntentFortMapTypeID, StormIsleID: 7,
			StormCooldownRemaining: 36_000, StormReadyAt: readyAt, ObservedAt: startedAt.Add(time.Second),
		},
	}
	application := &Application{State: State.NewStore(state)}
	request, err := json.Marshal(stormMapScanRequest{
		SourceCastleID: 40, Targeted: true,
		Bounds: State.StormMapBounds{X1: 612, Y1: 667, X2: 612, Y2: 667}, ScanStartedAt: startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := application.captureStormScan(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	tracked := application.State.Snapshot().Storm.Map.Targets["612:667"]
	if tracked.StormIsleID != 7 || tracked.StormCooldownRemaining != 36_000 || !tracked.StormReadyAt.Equal(readyAt) {
		t.Fatalf("targeted cooldown refresh = %#v", tracked)
	}
}

func TestStormAttackContextEnforcesMinimumFortAttacksRemaining(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	state.Map[4] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 7, StormVictoryCount: 7, ObservedAt: time.Now().UTC(),
		},
	}
	input := Intent.PlanningContext{State: state, GameData: gameData}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "attacks remaining") {
		t.Fatalf("minimum-attacks-remaining guard error = %v", err)
	}
	target := state.Map[4]["101:102"]
	target.StormVictoryCount = 6
	state.Map[4]["101:102"] = target
	input.State = state
	request = json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("fort at minimum attacks remaining rejected: %v", err)
	}
	readyAt := time.Now().UTC().Add(-time.Minute)
	target = state.Map[4]["101:102"]
	target.StormVictoryCount = 0
	target.StormCooldownRemaining = 60
	target.StormReadyAt = readyAt
	target.ObservedAt = readyAt.Add(-time.Minute)
	state.Map[4]["101:102"] = target
	input.State = state
	request = json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":5,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("fort readyAt was not allowed to refresh authoritative wins: %v", err)
	}
}

func TestStormAttackContextUsesIslandReadyAndExpiryLabels(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":4,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	target := State.MapObservation{
		KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentIslandMapTypeID, OwnerID: -403,
		ObjectID: 777, StormIsleID: 4, StormCooldownRemaining: 3_600, StormReadyAt: now.Add(-time.Minute),
		StormExpiresAt: now.Add(time.Hour), ObservedAt: now.Add(-time.Minute),
	}
	state.Map[4] = map[string]State.MapObservation{"101:102": target}
	input := Intent.PlanningContext{State: state, GameData: gameData}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":24,"targetX":101,"targetY":102,
		"stormIsleId":4,"preset":{"id":"island","name":"Island","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("unoccupied island timer was treated as a cooldown: %v", err)
	}

	target.OwnerID = 99
	target.StormReadyAt = now.Add(time.Minute)
	state.Map[4]["101:102"] = target
	input.State = state
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("occupied island readyAt guard error = %v", err)
	}

	target.OwnerID = -403
	target.StormReadyAt = now.Add(-time.Minute)
	target.StormExpiresAt = now.Add(-time.Second)
	state.Map[4]["101:102"] = target
	input.State = state
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired island guard error = %v", err)
	}
}

func TestConsumeStormIslandTargetRecordsReportGatedReturn(t *testing.T) {
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	state.Map[4] = map[string]State.MapObservation{
		"101:102": {KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentIslandMapTypeID, ObjectID: 777},
	}
	state.Storm.Map.Targets["101:102"] = state.Map[4]["101:102"]
	application := &Application{State: State.NewStore(state)}
	arguments, err := json.Marshal(stormTargetConsumeRequest{
		SourceCastleID: 40, KingdomID: 4, TargetTypeID: stormIntentIslandMapTypeID,
		TargetX: 101, TargetY: 102, IslandObjectID: 777, LeaveBehind: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.consumeStormTarget(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	snapshot := application.State.Snapshot()
	operation := snapshot.Storm.IslandReturns[State.StormIslandReturnKey(4, 101, 102)]
	if operation.Status != State.StormIslandReturnAwaitingReport || operation.SourceCastleID != 40 ||
		operation.IslandObjectID != 777 || operation.LeaveBehind != 1 || operation.LaunchedAt.IsZero() {
		t.Fatalf("pending island return = %#v", operation)
	}
	if _, exists := snapshot.Map[4]["101:102"]; exists {
		t.Fatal("consumed island remained in the live map")
	}
}

func TestPlanStormIslandReturnUsesIslandAsSourceAndStormCastleAsDestination(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10},{"wodID":12}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4, X: 200, Y: 300, Focused: true}
	key := State.StormIslandReturnKey(4, 101, 102)
	state.Storm.IslandReturns[key] = State.StormIslandReturnState{
		KingdomID: 4, SourceCastleID: 40, TargetX: 101, TargetY: 102,
		IslandObjectID: 777, ReportID: 202, Status: State.StormIslandReturnReady, LeaveBehind: 1,
		Survivors: map[State.UnitID]int64{10: 4, 12: 5}, ReportedAt: time.Now().UTC(),
	}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"islandX":101,"islandY":102,
		"islandObjectId":777,"reportId":202,"units":[{"unitId":10,"amount":4},{"unitId":12,"amount":4}]
	}`)
	plan, err := planStormIslandReturn(context.Background(), Intent.PlanningContext{State: state, GameData: gameData}, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "sdi" ||
		plan.Steps[1].Action != "storm.island.return.guard" || plan.Steps[2].Opcode != "cds" ||
		plan.Steps[3].Action != "storm.island.return.complete" {
		t.Fatalf("island return steps = %#v", plan.Steps)
	}
	var route struct {
		TargetX int `json:"TX"`
		TargetY int `json:"TY"`
		SourceX int `json:"SX"`
		SourceY int `json:"SY"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &route); err != nil {
		t.Fatal(err)
	}
	if route.TargetX != 200 || route.TargetY != 300 || route.SourceX != 101 || route.SourceY != 102 {
		t.Fatalf("island return route = %#v", route)
	}
	var dispatch struct {
		SourceID int64      `json:"SID"`
		TargetX  int        `json:"TX"`
		TargetY  int        `json:"TY"`
		Wait     int        `json:"WT"`
		Units    [][2]int64 `json:"A"`
	}
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &dispatch); err != nil {
		t.Fatal(err)
	}
	if dispatch.SourceID != 777 || dispatch.TargetX != 200 || dispatch.TargetY != 300 || dispatch.Wait != 0 ||
		len(dispatch.Units) != 2 || dispatch.Units[0] != [2]int64{10, 4} || dispatch.Units[1] != [2]int64{12, 4} {
		t.Fatalf("island return dispatch = %#v", dispatch)
	}
}

func TestPlanStormShopPurchaseUsesLunaStorefrontWireShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"packages":[
			{"packageID":245,"comment1":"War horn","comment2":"Luna's trade boat","packageType":"tool","packagePriceAquamarine":2960},
			{"packageID":3119,"comment1":"Silver Coins","comment2":"Luna's trade boat","packageType":"currency","packagePriceAquamarine":10000,"stock":3}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{
		ID: 40, KingdomID: 4,
		Resources: map[State.ResourceID]State.ResourceBalance{GameData.StormAquamarineID: {Amount: 100_000}},
	}
	state.Inventory.ConstructionOffersCastleID = 40
	state.Inventory.ConstructionOffersKingdomID = 4
	state.Inventory.ConstructionOffersObservedAt = now
	plan, err := planStormShopPurchase(context.Background(), Intent.PlanningContext{State: state, GameData: gameData}, json.RawMessage(`{
		"castleId":40,"productId":245,"amount":2,"aquamarineReserve":50000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "jaa" || plan.Steps[1].Opcode != "gbc" ||
		plan.Steps[2].Action != "storm.shop.guard" || plan.Steps[3].Opcode != "sbp" {
		t.Fatalf("Storm shop steps = %#v", plan.Steps)
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"CID":40,"KID":4}` {
		t.Fatalf("Storm shop history payload = %s", got)
	}
	if got := string(plan.Steps[3].Command.Payload); got != `{"PID":245,"BT":3,"TID":-1,"AMT":2,"KID":4,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}` {
		t.Fatalf("Storm shop payload = %s", got)
	}
	if plan.Summary != "Buy 2 x War horn from Luna for 5920 Aquamarine at castle 40" {
		t.Fatalf("Storm shop summary = %q", plan.Summary)
	}

	batch, err := planStormShopPurchase(context.Background(), Intent.PlanningContext{State: state, GameData: gameData}, json.RawMessage(`{
		"castleId":40,"purchases":[{"productId":245,"amount":2},{"productId":3119,"amount":3}],"aquamarineReserve":50000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Steps) != 5 || batch.Steps[3].Opcode != "sbp" || batch.Steps[4].Opcode != "sbp" {
		t.Fatalf("batched Storm shop steps = %#v", batch.Steps)
	}
	if got := string(batch.Steps[3].Command.Payload); !strings.Contains(got, `"PID":245`) || !strings.Contains(got, `"AMT":2`) {
		t.Fatalf("first batched Storm shop payload = %s", got)
	}
	if got := string(batch.Steps[4].Command.Payload); !strings.Contains(got, `"PID":3119`) || !strings.Contains(got, `"AMT":3`) {
		t.Fatalf("second batched Storm shop payload = %s", got)
	}
	if batch.Summary != "Buy 2 x War horn and 3 x Silver Coins from Luna for 35920 Aquamarine at castle 40" {
		t.Fatalf("batched Storm shop summary = %q", batch.Summary)
	}
}
