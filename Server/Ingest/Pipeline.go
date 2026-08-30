package Ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

var (
	ErrObservationSessionChanged = errors.New("observation session context changed before commit")
	ErrObservationAccountChanged = errors.New("observation account context changed before commit")
	ErrObservationFocusChanged   = errors.New("observation focus context changed before commit")
)

const observationDecoderVersion = "xt-v1"

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type Pipeline struct {
	state    *State.Store
	gameData GameDataProvider
	registry *Registry

	watchMu      sync.RWMutex
	watchers     map[uint64]frameWatcher
	wireWatchers map[uint64]wireFrameWatcher
	subscribers  map[uint64]chan Protocol.CommittedFrame
	nextWatch    atomic.Uint64
	nextIngress  atomic.Uint64

	commitMu    sync.Mutex
	wireCommits map[uint64]*wireCommit
	telemetry   frameTelemetry
	profileID   string
}

type frameTelemetry interface {
	Record(frame Protocol.CommittedFrame, reduceErr error)
}

type rawFrameTelemetry interface {
	RecordRaw(raw string, direction Protocol.Direction, observedAt time.Time, decodeErr error)
}

type appCommandTelemetry interface {
	RecordAppOutbound(payload string, actor string)
}

type webSocketSessionTelemetry interface {
	BeginWebSocketGameSession()
}

type frameWatcher struct {
	opcode        string
	responseToken string
	afterRevision uint64
	afterIngress  uint64
	channel       chan Protocol.CommittedFrame
}

type wireFrameWatcher struct {
	opcode        string
	responseToken string
	channel       chan Protocol.CommittedFrame
}

type wireCommit struct {
	done      chan struct{}
	frame     Protocol.CommittedFrame
	err       error
	completed bool
}

// ObservedFrame is a decoded transport frame that has already been exposed to
// wire-response watchers but has not necessarily been reduced into GameState.
type ObservedFrame struct {
	Frame                Protocol.Frame
	IngressID            uint64
	ProfileID            string
	WorldID              string
	PlayerID             State.PlayerID
	SessionGeneration    uint64
	ConnectionGeneration uint64
	FocusEpoch           uint64
	FocusedCastleID      State.CastleID
	CatalogVersion       string
	DecoderVersion       string
	CausationOperationID string
}

func NewPipeline(state *State.Store, gameData GameDataProvider, registry *Registry) *Pipeline {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Pipeline{
		state: state, gameData: gameData, registry: registry,
		watchers: map[uint64]frameWatcher{}, wireWatchers: map[uint64]wireFrameWatcher{},
		subscribers: map[uint64]chan Protocol.CommittedFrame{},
		wireCommits: map[uint64]*wireCommit{},
	}
}

func (pipeline *Pipeline) Registry() *Registry {
	return pipeline.registry
}

func (pipeline *Pipeline) SetTelemetry(telemetry frameTelemetry) {
	pipeline.telemetry = telemetry
}

func (pipeline *Pipeline) SetProfileID(profileID string) {
	pipeline.profileID = strings.TrimSpace(profileID)
}

func (pipeline *Pipeline) HandleRaw(ctx context.Context, raw string, direction Protocol.Direction) (Protocol.CommittedFrame, error) {
	return pipeline.HandleRawAt(ctx, raw, direction, time.Now().UTC())
}

func (pipeline *Pipeline) HandleRawAt(ctx context.Context, raw string, direction Protocol.Direction, observedAt time.Time) (Protocol.CommittedFrame, error) {
	observed, err := pipeline.DecodeRawAt(raw, direction, observedAt)
	if err != nil {
		return Protocol.CommittedFrame{}, err
	}
	return pipeline.CommitFrame(ctx, observed)
}

// DecodeRawAt performs the cheap, ordered wire-observation phase. Callers that
// need a non-blocking transport loop can enqueue the result for CommitFrame.
func (pipeline *Pipeline) DecodeRawAt(raw string, direction Protocol.Direction, observedAt time.Time) (ObservedFrame, error) {
	return pipeline.DecodeTransportFrameAt(raw, direction, observedAt, "", "")
}

func (pipeline *Pipeline) DecodeTransportFrameAt(
	raw string,
	direction Protocol.Direction,
	observedAt time.Time,
	responseToken string,
	causationOperationID string,
) (ObservedFrame, error) {
	frame, err := Protocol.Decode(raw, direction, observedAt)
	if err != nil {
		if telemetry, ok := pipeline.telemetry.(rawFrameTelemetry); ok {
			telemetry.RecordRaw(raw, direction, observedAt, err)
		}
		return ObservedFrame{}, err
	}
	frame.ResponseToken = strings.TrimSpace(responseToken)
	frame.CausationOperationID = strings.TrimSpace(causationOperationID)
	return pipeline.observeFrame(frame, frame.CausationOperationID), nil
}

// RecordAppOutbound marks a dispatched Citadel command so the dashboard can separate it
// from traffic initiated directly by the game client.
func (pipeline *Pipeline) RecordAppOutbound(payload string, actor string) {
	if telemetry, ok := pipeline.telemetry.(appCommandTelemetry); ok {
		telemetry.RecordAppOutbound(payload, actor)
	}
}

// BeginWebSocketGameSession starts a fresh durable websocket capture when the session starts.
func (pipeline *Pipeline) BeginWebSocketGameSession() {
	if telemetry, ok := pipeline.telemetry.(webSocketSessionTelemetry); ok {
		telemetry.BeginWebSocketGameSession()
	}
}

func (pipeline *Pipeline) HandleFrame(ctx context.Context, frame Protocol.Frame) (Protocol.CommittedFrame, error) {
	return pipeline.CommitFrame(ctx, pipeline.ObserveFrame(frame))
}

func (pipeline *Pipeline) ObserveFrame(frame Protocol.Frame) ObservedFrame {
	return pipeline.observeFrame(frame, frame.CausationOperationID)
}

func (pipeline *Pipeline) observeFrame(frame Protocol.Frame, causationOperationID string) ObservedFrame {
	observed := ObservedFrame{
		Frame: frame, IngressID: pipeline.nextIngress.Add(1), ProfileID: pipeline.profileID,
		DecoderVersion: observationDecoderVersion, CausationOperationID: strings.TrimSpace(causationOperationID),
	}
	if pipeline.state != nil {
		view := pipeline.state.IngestObservationView()
		observed.WorldID = view.ServerURL
		observed.PlayerID = view.PlayerID
		observed.SessionGeneration = view.SessionGeneration
		observed.ConnectionGeneration = view.ConnectionGeneration
		observed.FocusEpoch = view.FocusEpoch
		observed.FocusedCastleID = view.FocusedCastleID
		observed.CatalogVersion = view.CatalogVersion
	}
	pipeline.publishWire(Protocol.CommittedFrame{Frame: frame, IngressID: observed.IngressID})
	return observed
}

func (pipeline *Pipeline) CommitFrame(ctx context.Context, observed ObservedFrame) (Protocol.CommittedFrame, error) {
	return pipeline.CommitFrameGuarded(ctx, observed, nil)
}

func (pipeline *Pipeline) CommitFrameGuarded(
	ctx context.Context,
	observed ObservedFrame,
	guard func() error,
) (Protocol.CommittedFrame, error) {
	if pipeline.state == nil {
		err := fmt.Errorf("ingest state store is unavailable")
		pipeline.completeWireCommit(observed.IngressID, Protocol.CommittedFrame{}, err)
		return Protocol.CommittedFrame{}, err
	}
	frame := observed.Frame
	focusSubcontext := protocolFocusSubcontextForFrame(frame)
	authoritativePlayerID, accountAuthoritative := observationAuthoritativePlayerID(frame)
	validateObserved := func(
		sessionGeneration uint64,
		connectionGeneration uint64,
		serverURL string,
		playerID State.PlayerID,
		focusEpoch uint64,
		focusedCastleID State.CastleID,
		afterReducer bool,
	) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if observed.ConnectionGeneration > 0 &&
			connectionGeneration != observed.ConnectionGeneration {
			return Outbound.ErrConnectionChanged
		}
		if observed.SessionGeneration > 0 && sessionGeneration != observed.SessionGeneration {
			return ErrObservationSessionChanged
		}
		if observed.WorldID != "" && serverURL != "" &&
			!strings.EqualFold(strings.TrimSpace(observed.WorldID), strings.TrimSpace(serverURL)) {
			return ErrObservationAccountChanged
		}
		if accountAuthoritative && afterReducer {
			if authoritativePlayerID > 0 && playerID > 0 && authoritativePlayerID != playerID {
				return ErrObservationAccountChanged
			}
		} else if observed.PlayerID > 0 && playerID > 0 && observed.PlayerID != playerID {
			return ErrObservationAccountChanged
		} else if accountAuthoritative && observed.PlayerID == 0 && playerID > 0 &&
			authoritativePlayerID > 0 && authoritativePlayerID != playerID {
			// Two account-authoritative baselines may have been decoded before
			// either committed. Do not let a later, stale bootstrap frame replace
			// the identity established by the first committed baseline.
			return ErrObservationAccountChanged
		}
		if observationRequiresCapturedFocus(frame) && observed.FocusEpoch > 0 {
			if focusEpoch != observed.FocusEpoch || focusedCastleID != observed.FocusedCastleID {
				return ErrObservationFocusChanged
			}
		}
		if guard != nil {
			return guard()
		}
		return nil
	}
	validateCommit := func(gameState *State.GameState, afterReducer bool) error {
		current := pipeline.state.ProtocolContext()
		return validateObserved(
			gameState.Session.Generation, gameState.Session.ConnectionGeneration,
			gameState.Session.ServerURL, gameState.Player.ID,
			current.FocusEpoch, current.FocusedCastleID, afterReducer,
		)
	}
	var currentData *GameData.Store
	if pipeline.gameData != nil {
		currentData, _ = pipeline.gameData.Current()
	}
	registration := pipeline.registry.registered(frame.Opcode, frame.Direction)
	reducer := registration.reducer
	retainsObservation := State.RetainProtocolObservation(frame.Opcode) &&
		(frame.Direction == Protocol.DirectionInbound || State.RetainOutboundProtocolObservation(frame.Opcode))
	writes := State.ComponentSet(0)
	if retainsObservation {
		// recordObservation can also reconcile a baseline decoded before the
		// session-ready transition. Session is the conservative COW boundary;
		// the emitted dirty set includes it only when reconciliation occurs.
		writes = writes.Union(State.Components(State.ComponentObservations, State.ComponentSession))
	}
	if reducer != nil {
		writes = writes.Union(registration.writes)
	}
	if writes == 0 {
		view := pipeline.state.IngestObservationView()
		if validateErr := validateObserved(
			view.SessionGeneration, view.ConnectionGeneration, view.ServerURL, view.PlayerID,
			view.FocusEpoch, view.FocusedCastleID, false,
		); validateErr != nil {
			pipeline.completeWireCommit(observed.IngressID, Protocol.CommittedFrame{}, validateErr)
			return Protocol.CommittedFrame{}, validateErr
		}
		pipeline.state.ObserveProtocolFocus(focusSubcontext, frame.ReceivedAt)
		committed := Protocol.CommittedFrame{
			Frame: frame, IngressID: observed.IngressID, Revision: pipeline.state.Revision(),
		}
		pipeline.publish(committed)
		pipeline.completeWireCommit(observed.IngressID, committed, nil)
		if pipeline.telemetry != nil {
			pipeline.telemetry.Record(committed, nil)
		}
		return committed, nil
	}
	event, err := pipeline.state.ApplyScopedComponents(writes, func(gameState *State.GameState) (State.ScopedChange, error) {
		if validateErr := validateCommit(gameState, false); validateErr != nil {
			return State.ScopedChange{}, validateErr
		}
		baselineChanged := false
		domains := []string{}
		dirtyComponents := State.ComponentSet(0)
		if retainsObservation {
			baselineChanged = recordObservation(gameState, frame, "")
			domains = append(domains, "protocol")
			dirtyComponents = dirtyComponents.Union(State.Components(State.ComponentObservations))
		}
		reducerChanged := false
		if baselineChanged {
			domains = append(domains, "session")
			dirtyComponents = dirtyComponents.Union(State.Components(State.ComponentSession))
		}
		for _, step := range registration.steps {
			reducerDomains, changed, reduceErr := step.reducer(ctx, frame, gameState, currentData)
			if reduceErr != nil {
				return State.ScopedChange{}, fmt.Errorf("reduce %s: %w", frame.Opcode, reduceErr)
			}
			if changed {
				reducerChanged = true
				dirtyComponents = dirtyComponents.Union(step.writes)
				domains = append(domains, reducerDomains...)
			}
		}
		if validateErr := validateCommit(gameState, true); validateErr != nil {
			return State.ScopedChange{}, validateErr
		}
		return State.ScopedChange{
			Domains: domains, Partitions: scopedPartitionsForFrame(frame, *gameState, domains),
			FocusSubcontext: focusSubcontext, Changed: retainsObservation || baselineChanged || reducerChanged,
			DirtyComponents: dirtyComponents, DirtyComponentsSet: true,
		}, nil
	})
	if err != nil {
		if errors.Is(err, Outbound.ErrConnectionChanged) || errors.Is(err, ErrObservationSessionChanged) ||
			errors.Is(err, ErrObservationAccountChanged) || errors.Is(err, ErrObservationFocusChanged) ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			pipeline.completeWireCommit(observed.IngressID, Protocol.CommittedFrame{}, err)
			return Protocol.CommittedFrame{}, err
		}
		reduceErr := err
		if !retainsObservation {
			committed := Protocol.CommittedFrame{
				Frame: frame, IngressID: observed.IngressID, Revision: pipeline.state.Revision(), ReduceError: reduceErr.Error(),
			}
			pipeline.publish(committed)
			pipeline.completeWireCommit(observed.IngressID, committed, reduceErr)
			if pipeline.telemetry != nil {
				pipeline.telemetry.Record(committed, reduceErr)
			}
			return committed, reduceErr
		}
		event, err = pipeline.state.ApplyScopedComponents(
			State.Components(State.ComponentObservations, State.ComponentSession),
			func(gameState *State.GameState) (State.ScopedChange, error) {
				if validateErr := validateCommit(gameState, false); validateErr != nil {
					return State.ScopedChange{}, validateErr
				}
				baselineChanged := recordObservation(gameState, frame, reduceErr.Error())
				domains := []string{"protocol"}
				if baselineChanged {
					domains = append(domains, "session")
				}
				dirtyComponents := State.Components(State.ComponentObservations)
				if baselineChanged {
					dirtyComponents = dirtyComponents.Union(State.Components(State.ComponentSession))
				}
				return State.ScopedChange{
					Domains: domains, Partitions: scopedPartitionsForFrame(frame, *gameState, domains),
					FocusSubcontext: focusSubcontext, Changed: true,
					DirtyComponents: dirtyComponents, DirtyComponentsSet: true,
				}, nil
			},
		)
		if err != nil {
			err = fmt.Errorf("record failed %s frame: %w", frame.Opcode, err)
			pipeline.completeWireCommit(observed.IngressID, Protocol.CommittedFrame{}, err)
			return Protocol.CommittedFrame{}, err
		}
		committed := Protocol.CommittedFrame{
			Frame: frame, IngressID: observed.IngressID, Revision: event.Revision,
			Domains: event.Domains, ReduceError: reduceErr.Error(),
		}
		pipeline.publish(committed)
		pipeline.completeWireCommit(observed.IngressID, committed, reduceErr)
		if pipeline.telemetry != nil {
			pipeline.telemetry.Record(committed, reduceErr)
		}
		return committed, reduceErr
	}
	committed := Protocol.CommittedFrame{Frame: frame, IngressID: observed.IngressID, Revision: event.Revision, Domains: event.Domains}
	pipeline.publish(committed)
	pipeline.completeWireCommit(observed.IngressID, committed, nil)
	if pipeline.telemetry != nil {
		pipeline.telemetry.Record(committed, nil)
	}
	return committed, nil
}

func frameMutatesWorldMap(frame Protocol.Frame) bool {
	if frame.Direction != Protocol.DirectionInbound {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(frame.Opcode)) {
	case "gaa", "fnm", "fnt", "ssi", "adi":
		return true
	default:
		return false
	}
}

func observationAuthoritativePlayerID(frame Protocol.Frame) (State.PlayerID, bool) {
	if frame.Direction != Protocol.DirectionInbound || !strings.EqualFold(strings.TrimSpace(frame.Opcode), "gbd") {
		return 0, false
	}
	var root map[string]json.RawMessage
	if len(frame.Payload) == 0 || json.Unmarshal(frame.Payload, &root) != nil {
		return 0, false
	}
	raw := root["gpi"]
	if len(raw) == 0 {
		return 0, false
	}
	player, err := decodePlayerInfo(raw)
	if err != nil || player.ID <= 0 {
		return 0, false
	}
	return State.PlayerID(player.ID), true
}

func observationRequiresCapturedFocus(frame Protocol.Frame) bool {
	if frame.Direction != Protocol.DirectionInbound && frame.Direction != Protocol.DirectionOutbound {
		return false
	}
	opcode := strings.ToLower(strings.TrimSpace(frame.Opcode))
	if opcode == "jaa" || opcode == "jca" {
		return false
	}
	_, focused := focusedCastleOpcodes[opcode]
	return focused
}

func (pipeline *Pipeline) DiscardObserved(observed ObservedFrame, err error) {
	if err == nil {
		err = fmt.Errorf("observed frame was not committed")
	}
	pipeline.completeWireCommit(observed.IngressID, Protocol.CommittedFrame{}, err)
}

func (pipeline *Pipeline) WaitCommitted(ctx context.Context, ingressID uint64) (Protocol.CommittedFrame, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pipeline.commitMu.Lock()
	commit := pipeline.wireCommits[ingressID]
	pipeline.commitMu.Unlock()
	if commit == nil {
		return Protocol.CommittedFrame{}, fmt.Errorf("wire frame %d is not tracked", ingressID)
	}
	defer func() {
		pipeline.commitMu.Lock()
		delete(pipeline.wireCommits, ingressID)
		pipeline.commitMu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return Protocol.CommittedFrame{}, ctx.Err()
	case <-commit.done:
		return commit.frame, commit.err
	}
}

func (pipeline *Pipeline) ForgetCommitted(ingressID uint64) {
	pipeline.commitMu.Lock()
	delete(pipeline.wireCommits, ingressID)
	pipeline.commitMu.Unlock()
}

func (pipeline *Pipeline) Watch(opcode string, afterRevision uint64) (<-chan Protocol.CommittedFrame, func()) {
	return pipeline.watch(opcode, afterRevision, "")
}

func (pipeline *Pipeline) WatchResponse(
	opcode string,
	afterRevision uint64,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	return pipeline.watch(opcode, afterRevision, responseToken)
}

func (pipeline *Pipeline) watch(
	opcode string,
	afterRevision uint64,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	id := pipeline.nextWatch.Add(1)
	channel := make(chan Protocol.CommittedFrame, 1)
	pipeline.watchMu.Lock()
	pipeline.watchers[id] = frameWatcher{
		opcode: strings.ToLower(strings.TrimSpace(opcode)), responseToken: strings.TrimSpace(responseToken),
		afterRevision: afterRevision, afterIngress: pipeline.nextIngress.Load(), channel: channel,
	}
	pipeline.watchMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			pipeline.watchMu.Lock()
			delete(pipeline.watchers, id)
			pipeline.watchMu.Unlock()
		})
	}
	return channel, cancel
}

// WatchWire observes an inbound frame immediately after protocol decoding and
// before state reduction. The watcher must be registered before the command is
// dispatched; unlike Watch, it intentionally has no revision look-behind.
func (pipeline *Pipeline) WatchWire(opcode string) (<-chan Protocol.CommittedFrame, func()) {
	return pipeline.watchWire(opcode, "")
}

// SubscribeFrames observes every successfully committed inbound and outbound
// frame. Delivery is best-effort and non-blocking so a diagnostic or research
// consumer can never delay the ordered game-state reducer.
func (pipeline *Pipeline) SubscribeFrames(buffer int) (<-chan Protocol.CommittedFrame, func()) {
	if buffer < 1 {
		buffer = 1
	}
	id := pipeline.nextWatch.Add(1)
	channel := make(chan Protocol.CommittedFrame, buffer)
	pipeline.watchMu.Lock()
	pipeline.subscribers[id] = channel
	pipeline.watchMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			pipeline.watchMu.Lock()
			delete(pipeline.subscribers, id)
			pipeline.watchMu.Unlock()
		})
	}
	return channel, cancel
}

func (pipeline *Pipeline) WatchWireResponse(
	opcode string,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	return pipeline.watchWire(opcode, responseToken)
}

func (pipeline *Pipeline) watchWire(
	opcode string,
	responseToken string,
) (<-chan Protocol.CommittedFrame, func()) {
	id := pipeline.nextWatch.Add(1)
	channel := make(chan Protocol.CommittedFrame, 1)
	pipeline.watchMu.Lock()
	pipeline.wireWatchers[id] = wireFrameWatcher{
		opcode: strings.ToLower(strings.TrimSpace(opcode)), responseToken: strings.TrimSpace(responseToken),
		channel: channel,
	}
	pipeline.watchMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			pipeline.watchMu.Lock()
			delete(pipeline.wireWatchers, id)
			pipeline.watchMu.Unlock()
			select {
			case frame := <-channel:
				pipeline.ForgetCommitted(frame.IngressID)
			default:
			}
		})
	}
	return channel, cancel
}

func recordObservation(gameState *State.GameState, frame Protocol.Frame, lastError string) bool {
	if gameState == nil || !State.RetainProtocolObservation(frame.Opcode) ||
		frame.Direction == Protocol.DirectionOutbound && !State.RetainOutboundProtocolObservation(frame.Opcode) {
		return false
	}
	observation := gameState.Observations[frame.Opcode]
	observation.Opcode = frame.Opcode
	observation.Count++
	if frame.Direction == Protocol.DirectionInbound {
		observation.InboundCount++
	} else if frame.Direction == Protocol.DirectionOutbound {
		observation.OutboundCount++
	}
	observation.LastDirection = string(frame.Direction)
	observation.LastCode = frame.ResponseCode
	observation.LastError = lastError
	observation.LastSeenAt = frame.ReceivedAt
	observation.LastRevision = gameState.Revision + 1
	if frame.Direction == Protocol.DirectionInbound && frame.ResponseCode != nil && *frame.ResponseCode == 0 && lastError == "" {
		observation.LastSuccessfulInboundAt = frame.ReceivedAt
		observation.LastSuccessfulInboundRevision = gameState.Revision + 1
	}
	gameState.Observations[frame.Opcode] = observation
	return reconcileSessionBaseline(gameState)
}

func reconcileSessionBaseline(gameState *State.GameState) bool {
	session := &gameState.Session
	if !session.LoggedIn || !session.SocketReady || session.Generation == 0 ||
		session.BaselineGeneration == session.Generation {
		return false
	}
	baselineAt := gameState.Observations["gbd"].SuccessfulInboundAt()
	if baselineAt.IsZero() || baselineAt.Before(session.ChangedAt) {
		return false
	}
	session.BaselineGeneration = session.Generation
	return true
}

func (pipeline *Pipeline) publish(frame Protocol.CommittedFrame) {
	pipeline.watchMu.RLock()
	defer pipeline.watchMu.RUnlock()
	for _, subscriber := range pipeline.subscribers {
		select {
		case subscriber <- frame:
		default:
		}
	}
	for _, watcher := range pipeline.watchers {
		if frame.Frame.Direction != Protocol.DirectionInbound {
			continue
		}
		if watcher.opcode != "" && watcher.opcode != frame.Frame.Opcode {
			continue
		}
		if watcher.responseToken != "" && watcher.responseToken != frame.Frame.ResponseToken {
			continue
		}
		// A committed response need not create durable state. The ingress floor
		// proves it was observed after this live watcher was registered; the
		// revision floor only rejects a genuinely older state generation.
		if frame.IngressID <= watcher.afterIngress || frame.Revision < watcher.afterRevision {
			continue
		}
		select {
		case watcher.channel <- frame:
		default:
		}
	}
}

func (pipeline *Pipeline) publishWire(frame Protocol.CommittedFrame) {
	if frame.Frame.Direction != Protocol.DirectionInbound {
		return
	}
	pipeline.watchMu.RLock()
	defer pipeline.watchMu.RUnlock()
	tracked := false
	delivered := false
	for _, watcher := range pipeline.wireWatchers {
		if watcher.opcode != "" && watcher.opcode != frame.Frame.Opcode {
			continue
		}
		if watcher.responseToken != "" && watcher.responseToken != frame.Frame.ResponseToken {
			continue
		}
		if !tracked {
			pipeline.trackWireCommit(frame.IngressID)
			tracked = true
		}
		select {
		case watcher.channel <- frame:
			delivered = true
		default:
		}
	}
	if tracked && !delivered {
		pipeline.ForgetCommitted(frame.IngressID)
	}
}

func (pipeline *Pipeline) trackWireCommit(ingressID uint64) {
	pipeline.commitMu.Lock()
	if pipeline.wireCommits[ingressID] == nil {
		pipeline.wireCommits[ingressID] = &wireCommit{done: make(chan struct{})}
	}
	pipeline.commitMu.Unlock()
}

func (pipeline *Pipeline) completeWireCommit(
	ingressID uint64,
	frame Protocol.CommittedFrame,
	err error,
) {
	pipeline.commitMu.Lock()
	commit := pipeline.wireCommits[ingressID]
	if commit != nil && !commit.completed {
		commit.frame = frame
		commit.err = err
		commit.completed = true
		close(commit.done)
	}
	pipeline.commitMu.Unlock()
}
