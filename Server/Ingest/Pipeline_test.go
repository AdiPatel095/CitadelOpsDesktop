package Ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestFrameSubscriberObservesCommittedOutboundFrames(t *testing.T) {
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, NewRegistry())
	frames, unsubscribe := pipeline.SubscribeFrames(2)
	defer unsubscribe()
	committed, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "cra", Payload: json.RawMessage(`{"LID":7,"A":[{}]}`),
		ReceivedAt: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-frames:
		if observed.IngressID != committed.IngressID || observed.Frame.Direction != Protocol.DirectionOutbound || observed.Frame.Opcode != "cra" {
			t.Fatalf("observed = %#v, committed = %#v", observed, committed)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the committed outbound frame")
	}
}

func TestWireObservationPrecedesCommittedStateBarrier(t *testing.T) {
	registry := NewRegistry()
	reducerStarted := make(chan struct{})
	releaseReducer := make(chan struct{})
	if err := registry.Register("bup", func(
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
	pipeline := NewPipeline(store, nil, registry)
	wire, cancelWire := pipeline.WatchWire("bup")
	defer cancelWire()
	committed, cancelCommitted := pipeline.Watch("bup", store.Revision())
	defer cancelCommitted()
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bup", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
	})
	commitResult := make(chan error, 1)
	go func() {
		_, err := pipeline.CommitFrame(t.Context(), observed)
		commitResult <- err
	}()

	var wireFrame Protocol.CommittedFrame
	select {
	case wireFrame = <-wire:
		if wireFrame.IngressID == 0 || wireFrame.Revision != 0 {
			t.Fatalf("wire frame = %#v", wireFrame)
		}
	case <-time.After(time.Second):
		t.Fatal("wire response was not observed")
	}
	select {
	case <-reducerStarted:
	case <-time.After(time.Second):
		t.Fatal("reducer did not start")
	}
	select {
	case frame := <-committed:
		t.Fatalf("committed response arrived while reducer was blocked: %#v", frame)
	default:
	}

	close(releaseReducer)
	select {
	case err := <-commitResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("frame did not commit")
	}
	select {
	case frame := <-committed:
		if frame.IngressID != wireFrame.IngressID || frame.Revision == 0 {
			t.Fatalf("committed frame = %#v, wire = %#v", frame, wireFrame)
		}
	case <-time.After(time.Second):
		t.Fatal("committed watcher did not receive the response")
	}
	if _, err := pipeline.WaitCommitted(t.Context(), wireFrame.IngressID); err != nil {
		t.Fatalf("exact wire commit wait failed: %v", err)
	}
}

func TestProtocolObservationRetainsSuccessfulInboundAcrossOutbound(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	inboundAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	recordObservation(&gameState, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ggm", ResponseCode: &code, ReceivedAt: inboundAt,
	}, "")
	recordObservation(&gameState, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "ggm", ReceivedAt: inboundAt.Add(time.Second),
	}, "")

	observation := gameState.Observations["ggm"]
	if observation.LastDirection != string(Protocol.DirectionOutbound) {
		t.Fatalf("last direction = %q, want outbound", observation.LastDirection)
	}
	if got := observation.SuccessfulInboundAt(); !got.Equal(inboundAt) {
		t.Fatalf("successful inbound = %s, want %s", got, inboundAt)
	}
}

func TestCommitFrameRejectsStaleConnectionInsideStateMutation(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 2
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, nil, NewRegistry())
	wire, cancelWire := pipeline.WatchWire("bup")
	defer cancelWire()
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bup", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
	})
	observed.ConnectionGeneration = 1
	wireFrame := <-wire
	if _, err := pipeline.CommitFrame(t.Context(), observed); !errors.Is(err, Outbound.ErrConnectionChanged) {
		t.Fatalf("stale commit error = %v", err)
	}
	if _, err := pipeline.WaitCommitted(t.Context(), wireFrame.IngressID); !errors.Is(err, Outbound.ErrConnectionChanged) {
		t.Fatalf("stale exact-commit error = %v", err)
	}
	if revision := store.Revision(); revision != 0 {
		t.Fatalf("stale frame advanced state to revision %d", revision)
	}
}

func TestReducerFailurePropagatesThroughExactCommit(t *testing.T) {
	registry := NewRegistry()
	wanted := errors.New("broken reducer")
	if err := registry.Register("bup", func(
		context.Context,
		Protocol.Frame,
		*State.GameState,
		*GameData.Store,
	) ([]string, bool, error) {
		return nil, false, wanted
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, registry)
	wire, cancelWire := pipeline.WatchWire("bup")
	defer cancelWire()
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bup", ReceivedAt: time.Now().UTC(),
	})
	wireFrame := <-wire
	if _, err := pipeline.CommitFrame(t.Context(), observed); !errors.Is(err, wanted) {
		t.Fatalf("commit error = %v", err)
	}
	committed, err := pipeline.WaitCommitted(t.Context(), wireFrame.IngressID)
	if !errors.Is(err, wanted) || committed.ReduceError == "" {
		t.Fatalf("exact commit = %#v, error = %v", committed, err)
	}
}

func TestWireWatcherCancellationCleansDeliveredAndUndeliveredCommits(t *testing.T) {
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, NewRegistry())
	_, cancel := pipeline.WatchWire("bup")
	pipeline.ObserveFrame(Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "bup"})
	pipeline.ObserveFrame(Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "bup"})
	cancel()
	pipeline.commitMu.Lock()
	tracked := len(pipeline.wireCommits)
	pipeline.commitMu.Unlock()
	if tracked != 0 {
		t.Fatalf("wire commit trackers after cancellation = %d", tracked)
	}
}

func TestCorrelatedWireWatcherIgnoresManualSameOpcodeResponse(t *testing.T) {
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, NewRegistry())
	wire, cancel := pipeline.WatchWireResponse("bup", "operation/1")
	defer cancel()
	pipeline.ObserveFrame(Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "bup"})
	select {
	case frame := <-wire:
		t.Fatalf("manual response satisfied correlated watcher: %#v", frame)
	default:
	}
	correlated := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bup", ResponseToken: "operation/1",
	}
	pipeline.ObserveFrame(correlated)
	select {
	case frame := <-wire:
		if frame.Frame.ResponseToken != "operation/1" {
			t.Fatalf("correlated response = %#v", frame)
		}
		pipeline.ForgetCommitted(frame.IngressID)
	case <-time.After(time.Second):
		t.Fatal("correlated response was not delivered")
	}
}

func TestObservationEnvelopeCapturesAccountSessionFocusAndCausation(t *testing.T) {
	gameState := State.NewGameState()
	gameState.CatalogVersion = "catalog-7"
	gameState.Session.Generation = 3
	gameState.Session.ConnectionGeneration = 4
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 42
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	pipeline := NewPipeline(State.NewStore(gameState), nil, NewRegistry())
	pipeline.SetProfileID("profile-1")
	observed, err := pipeline.DecodeTransportFrameAt(
		`%xt%EmpireEx_21%bup%1%0%{}%`, Protocol.DirectionInbound, time.Now().UTC(),
		"response-1", "operation-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if observed.ProfileID != "profile-1" || observed.WorldID != "https://world.example" ||
		observed.PlayerID != 42 || observed.SessionGeneration != 3 || observed.ConnectionGeneration != 4 ||
		observed.FocusEpoch != 1 || observed.FocusedCastleID != 11 || observed.CatalogVersion != "catalog-7" ||
		observed.DecoderVersion != observationDecoderVersion || observed.CausationOperationID != "operation-1" {
		t.Fatalf("observation envelope = %#v", observed)
	}
	if observed.Frame.ResponseToken != "response-1" || observed.Frame.CausationOperationID != "operation-1" {
		t.Fatalf("decoded frame causation = %#v", observed.Frame)
	}
}

func TestFocusedObservationRejectsFocusAwayAndBackBeforeCommit(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 1
	gameState.Session.ConnectionGeneration = 1
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	gameState.Castles[12] = State.CastleState{ID: 12, KingdomID: 1}
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := registry.Register("bup", func(
		_ context.Context,
		_ Protocol.Frame,
		state *State.GameState,
		_ *GameData.Store,
	) ([]string, bool, error) {
		state.Player.Level++
		return []string{"production"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	observed := pipeline.ObserveFrame(Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "bup"})
	setFocus := func(castleID State.CastleID) {
		if _, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
			for id, castle := range state.Castles {
				castle.Focused = id == castleID
				state.Castles[id] = castle
			}
			return []string{"session-context"}, true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	setFocus(12)
	setFocus(11)
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 || context.FocusEpoch != 3 {
		t.Fatalf("current focus context = %#v", context)
	}
	if _, err := pipeline.CommitFrame(t.Context(), observed); !errors.Is(err, ErrObservationFocusChanged) {
		t.Fatalf("stale focus error = %v", err)
	}
	if level := store.Snapshot().Player.Level; level != 0 {
		t.Fatalf("stale focused frame mutated player level to %d", level)
	}
}

func TestCommittedGAAAndJAATrackFocusedCastleSubcontext(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 1
	gameState.Session.ConnectionGeneration = 1
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, nil, NewRegistry())
	zero := 0
	invalidFocus := 53

	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &zero,
	}); err != nil {
		t.Fatal(err)
	}
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 ||
		context.FocusSubcontext != State.FocusSubcontextMap || context.FocusEpoch != 2 {
		t.Fatalf("GAA protocol context = %#v", context)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &invalidFocus,
	}); err != nil {
		t.Fatal(err)
	}
	if context := store.ProtocolContext(); context.FocusSubcontext != State.FocusSubcontextMap || context.FocusEpoch != 2 {
		t.Fatalf("failed GAA changed protocol context = %#v", context)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "jaa", ResponseCode: &zero,
	}); err != nil {
		t.Fatal(err)
	}
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 ||
		context.FocusSubcontext != State.FocusSubcontextCastle || context.FocusEpoch != 3 {
		t.Fatalf("JAA protocol context = %#v", context)
	}
}

func TestFocusAuthoritativeSnapshotCanEstablishNewFocus(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 1
	gameState.Session.ConnectionGeneration = 1
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	gameState.Castles[12] = State.CastleState{ID: 12, KingdomID: 1}
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := registry.Register("jaa", func(
		_ context.Context,
		_ Protocol.Frame,
		state *State.GameState,
		_ *GameData.Store,
	) ([]string, bool, error) {
		state.Player.Level++
		return []string{"session-context"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	observed := pipeline.ObserveFrame(Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "jaa"})
	if _, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
		first := state.Castles[11]
		first.Focused = false
		state.Castles[11] = first
		second := state.Castles[12]
		second.Focused = true
		state.Castles[12] = second
		return []string{"session-context"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.CommitFrame(t.Context(), observed); err != nil {
		t.Fatalf("focus-authoritative commit: %v", err)
	}
	if level := store.Snapshot().Player.Level; level != 1 {
		t.Fatalf("focus-authoritative reducer level = %d", level)
	}
}

func TestAccountAuthoritativeBaselineClearsPriorAccountState(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Revision = 7
	gameState.CatalogVersion = "catalog-7"
	gameState.LanguageVersion = "language-7"
	gameState.Session.Generation = 3
	gameState.Session.ConnectionGeneration = 4
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 41
	gameState.Player.Name = "Previous"
	oldCastle := newCastleState(11)
	oldCastle.KingdomID = 1
	oldCastle.Focused = true
	gameState.Castles[11] = oldCastle
	gameState.Movements[91] = State.MovementState{ID: 91, SourceCastleID: 11}
	gameState.Scheduled["old-schedule"] = State.ScheduledOperation{ID: "old-schedule", Intent: "old.intent", Status: "scheduled"}
	gameState.Inventory.Items["units"] = map[int64]int64{5: 100}
	gameState.AttackPresets = []State.AttackPreset{{Slot: 1, Name: "Old account"}}
	gameState.Observations["old"] = State.ProtocolObservation{Opcode: "old", Count: 10}

	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gbd", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{
			"gpi":{"PID":42,"PN":"Current"},
			"gcl":{"C":[{"KID":9,"AI":[{"AI":[0,12,13,22,0,0,0,0,0,0,"Current Castle"]}]}]}
		}`),
	})
	committed, err := pipeline.CommitFrame(t.Context(), observed)
	if err != nil {
		t.Fatalf("commit account baseline: %v", err)
	}
	next := store.Snapshot()
	if next.Player.ID != 42 || next.Player.Name != "Current" {
		t.Fatalf("current player = %+v", next.Player)
	}
	if next.Account.PlayerID != 42 || next.Account.WorldID != "https://world.example" {
		t.Fatalf("current account binding = %+v", next.Account)
	}
	if _, found := next.Castles[22]; !found || len(next.Castles) != 1 {
		t.Fatalf("current castles = %+v", next.Castles)
	}
	if len(next.Movements) != 0 || len(next.Scheduled) != 0 || len(next.Inventory.Items) != 0 || len(next.AttackPresets) != 0 {
		t.Fatalf("prior account state survived: movements=%d scheduled=%d inventory=%d presets=%d",
			len(next.Movements), len(next.Scheduled), len(next.Inventory.Items), len(next.AttackPresets))
	}
	if next.Session.Generation != 3 || next.Session.ConnectionGeneration != 4 ||
		next.Session.ServerURL != "https://world.example" || next.CatalogVersion != "catalog-7" ||
		next.LanguageVersion != "language-7" {
		t.Fatalf("runtime context was not preserved: session=%+v catalog=%q language=%q",
			next.Session, next.CatalogVersion, next.LanguageVersion)
	}
	if next.Revision != 8 || committed.Revision != 8 {
		t.Fatalf("revision regressed across account reset: state=%d committed=%d", next.Revision, committed.Revision)
	}
	if _, found := next.Observations["old"]; found || next.Observations["gbd"].Count != 1 {
		t.Fatalf("protocol observations crossed accounts: %+v", next.Observations)
	}
}

func TestAccountAuthoritativeBaselineSeparatesSamePlayerIDOnDifferentWorld(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://world-a.example"
	gameState.Player.ID = 42
	gameState.Player.Name = "World A"
	oldCastle := newCastleState(11)
	oldCastle.Focused = true
	gameState.Castles[11] = oldCastle
	store := State.NewStore(gameState)
	if bound := store.Snapshot().Account; bound.WorldID != "https://world-a.example" || bound.PlayerID != 42 {
		t.Fatalf("recovered account binding = %+v", bound)
	}
	if _, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
		state.Session.ServerURL = "https://world-b.example"
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gbd", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{
			"gpi":{"PID":42,"PN":"World B"},
			"gcl":{"C":[{"KID":2,"AI":[{"AI":[0,3,4,22,0,0,0,0,0,0,"World B Castle"]}]}]}
		}`),
	})
	if _, err := pipeline.CommitFrame(t.Context(), observed); err != nil {
		t.Fatalf("commit new-world baseline: %v", err)
	}
	next := store.Snapshot()
	if next.Account.WorldID != "https://world-b.example" || next.Account.PlayerID != 42 ||
		next.Player.Name != "World B" || len(next.Castles) != 1 {
		t.Fatalf("new-world state = account:%+v player:%+v castles:%+v", next.Account, next.Player, next.Castles)
	}
	if _, oldFound := next.Castles[11]; oldFound {
		t.Fatalf("world A castle survived world switch: %+v", next.Castles)
	}
}

func TestAccountAuthoritativeBaselineRejectsAccountChangedAfterObservation(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 41
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gbd", ResponseCode: &code,
		Payload: json.RawMessage(`{"gpi":{"PID":42,"PN":"Stale"}}`),
	})
	if _, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
		state.Player.ID = 43
		state.Player.Name = "Newer"
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.CommitFrame(t.Context(), observed); !errors.Is(err, ErrObservationAccountChanged) {
		t.Fatalf("stale account baseline error = %v", err)
	}
	current := store.Snapshot()
	if current.Player.ID != 43 || current.Player.Name != "Newer" {
		t.Fatalf("stale baseline replaced current account: %+v", current.Player)
	}
}
