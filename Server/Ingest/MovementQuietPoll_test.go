package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// pollPayload is what a 5-second gam poll looks like for a busy account when
// nothing changes between polls: only the elapsed-travel counters (PT) advance.
// Movements: an owned attack with a commander, an incoming foreign attack, an
// owned outbound station with its total wait, and an advisor attack in flight.
// movementTestStart is the frame time these tests build on. It is taken from the
// clock, not pinned to a date: the reducers decide commander availability from
// the real clock (commanderAvailable), so a fixed date makes the tests depend on
// when they run. Frames are start plus an offset, so every relation between a
// frame and "now" is the same at any time.
func movementTestStart() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func pollPayload(elapsed int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"M":[
		{"M":{"MID":50,"PT":%d,"TT":900,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"A":[[6,40],[7,10]],"UM":{"L":{"ID":7}}},
		{"M":{"MID":51,"PT":%d,"TT":1200,"D":0,"T":0,"KID":0,"OID":2,"TID":1,"SA":[0,60,61,700,2],"TA":[0,10,11,100,1]},"A":[[6,500]],"UM":{"L":{"ID":-1}}},
		{"M":{"MID":52,"PT":%d,"TT":896,"D":0,"T":1,"KID":0,"OID":1,"TID":98,"SA":[0,10,11,100,1],"TA":[0,30,31,400,98]},"A":[[6,80]],"UM":{"TWD":21600,"PWD":%d,"L":{"ID":8}}},
		{"M":{"MID":53,"PT":%d,"TT":700,"D":0,"T":0,"KID":0,"OID":1,"TID":97,"SA":[0,10,11,100,1],"TA":[27,50,51,-1,-1]},"A":[[6,30]],"UM":{"AAT":1,"AAN":1,"AAC":2,"AAL":0,"L":{"ID":9}}}
	],"O":[]}`, 100+elapsed, 200+elapsed, 300+elapsed, 267+elapsed, 50+elapsed))
}

func quietPollFixture(t *testing.T) (*Pipeline, *State.Store) {
	t.Helper()
	initial := State.NewGameState()
	initial.Player.ID = 1
	initial.Session = State.SessionState{
		LoggedIn: true, SocketReady: true, Generation: 1, ConnectionGeneration: 1, BaselineGeneration: 1,
	}
	initial.Castles[100] = newCastleState(100)
	for _, id := range []State.CommanderID{7, 8, 9, 10} {
		initial.Commanders[id] = State.CommanderState{ID: id, Available: true}
	}
	store := State.NewStore(initial)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	return NewPipeline(store, nil, registry), store
}

func sendPoll(t *testing.T, pipeline *Pipeline, payload json.RawMessage, at time.Time) Protocol.CommittedFrame {
	t.Helper()
	code := 0
	committed, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at, Payload: payload,
	})
	if err != nil {
		t.Fatalf("gam at %s: %v", at.Format(time.RFC3339), err)
	}
	return committed
}

// The acceptance criterion of CIT-30: an identical gam reply leaves the store
// revision unchanged, emits no state event (so nothing wakes, persists or reaches
// the dashboard), and still advances the freshness barrier.
func TestIdenticalMovementPollsCreateNoRevisionOrEventButKeepTheBarrierFresh(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	start := movementTestStart()
	first := sendPoll(t, pipeline, pollPayload(0), start)
	if first.Revision == 0 || len(first.Domains) == 0 {
		t.Fatalf("the first poll must commit: %+v", first)
	}
	baseline := store.ReadOnlyView()
	if baseline.MovementCount() != 4 || baseline.Commanders[7].Available {
		t.Fatalf("baseline state: %d movements, commander 7 available %t", baseline.MovementCount(), baseline.Commanders[7].Available)
	}
	events, unsubscribe := store.Subscribe(64)
	defer unsubscribe()

	// A minute of polls, each 5 s later with the elapsed counters advanced, plus reply-arrival jitter.
	jitter := []time.Duration{0, 120 * time.Millisecond, -60 * time.Millisecond, 400 * time.Millisecond, 30 * time.Millisecond, 250 * time.Millisecond}
	for poll := 1; poll <= 12; poll++ {
		at := start.Add(time.Duration(poll)*5*time.Second + jitter[poll%len(jitter)])
		committed := sendPoll(t, pipeline, pollPayload(poll*5), at)
		if committed.Revision != first.Revision || len(committed.Domains) != 0 {
			t.Fatalf("poll %d committed revision %d domains %v; want the unchanged revision %d and no domains", poll, committed.Revision, committed.Domains, first.Revision)
		}
	}
	select {
	case event := <-events:
		t.Fatalf("an unchanged poll emitted a state event: revision %d domains %v", event.Revision, event.Domains)
	default:
	}
	after := store.ReadOnlyView()
	if after.Revision != first.Revision {
		t.Fatalf("revision %d after 12 unchanged polls, want %d", after.Revision, first.Revision)
	}
	if !reflect.DeepEqual(after.MovementViewMap(), baseline.MovementViewMap()) {
		t.Fatal("unchanged polls rewrote the stored movements")
	}
	lastAt := start.Add(60*time.Second + jitter[0])
	if !after.MovementSnapshot.ObservedAt.Equal(lastAt) || after.MovementSnapshot.Version <= baseline.MovementSnapshot.Version ||
		after.MovementSnapshot.ConnectionGeneration != 1 {
		t.Fatalf("freshness barrier = %+v, want observed at %s with a later version", after.MovementSnapshot, lastAt)
	}
	// A guard that needs a snapshot observed after its planning time passes without a new revision.
	plannedAt := start.Add(50 * time.Second)
	if after.MovementSnapshot.ObservedAt.Before(plannedAt) {
		t.Fatal("a guard planned during the quiet polls would wrongly find the snapshot stale")
	}
}

func TestMovementChangesStillCommitWithTheSameDomainsAndTiming(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	start := movementTestStart()
	first := sendPoll(t, pipeline, pollPayload(0), start)
	events, unsubscribe := store.Subscribe(64)
	defer unsubscribe()

	assertCommit := func(name string, payload json.RawMessage, at time.Time) {
		t.Helper()
		before := store.Revision()
		committed := sendPoll(t, pipeline, payload, at)
		if committed.Revision != before+1 {
			t.Fatalf("%s: revision %d, want %d", name, committed.Revision, before+1)
		}
		event := <-events
		for _, domain := range []string{"movements", "commanders", "movement-snapshot"} {
			if !slices.Contains(event.Domains, domain) {
				t.Fatalf("%s: event domains %v miss %s", name, event.Domains, domain)
			}
		}
	}

	// A new incoming attack appears.
	extra := string(pollPayload(5))
	withNew := json.RawMessage(extra[:len(extra)-len(`],"O":[]}`)] + `,
		{"M":{"MID":60,"PT":3,"TT":600,"D":0,"T":0,"KID":0,"OID":3,"TID":1,"SA":[0,80,81,900,3],"TA":[0,10,11,100,1]},"A":[[6,900]],"UM":{"L":{"ID":-1}}}
	],"O":[]}`)
	assertCommit("new incoming attack", withNew, start.Add(5*time.Second))
	// The travel time of an owned movement changes (a speed-up): a real change even though the same id.
	sped := string(pollPayload(10))
	spedPayload := json.RawMessage(replaceOnce(sped, `"TT":900`, `"TT":650`))
	assertCommit("speed-up changes travel time", spedPayload, start.Add(10*time.Second))
	// A movement disappears from the authoritative snapshot (the incoming attack landed).
	removed := string(spedPayload)
	begin := strings.Index(removed, `{"M":{"MID":51`)
	end := strings.Index(removed, `{"M":{"MID":52`)
	assertCommit("movement removed", json.RawMessage(removed[:begin]+removed[end:]), start.Add(15*time.Second))
	// After all that the state moves on from the first poll's revision.
	if store.Revision() <= first.Revision+2 {
		t.Fatalf("revision %d did not advance", store.Revision())
	}
}

func TestOwnedCommanderAvailabilityStillFollowsARemovedMovement(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	start := movementTestStart()
	sendPoll(t, pipeline, pollPayload(0), start)
	if store.ReadOnlyView().Commanders[7].Available {
		t.Fatal("commander 7 should be busy while its attack is in flight")
	}
	before := store.Revision()
	// The whole reply is now empty and long after every movement's completion time.
	code := 0
	if _, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: start.Add(3 * time.Hour), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}); err != nil {
		t.Fatal(err)
	}
	after := store.ReadOnlyView()
	// The long station wait (6 h) is retained across a scoped empty reply; the attack's commander is released.
	if store.Revision() != before+1 || after.MovementCount() != 1 || !after.Commanders[7].Available || after.Commanders[8].Available {
		t.Fatalf("revision %d (want %d), %d movements, commander 7 available %t, commander 8 available %t",
			store.Revision(), before+1, after.MovementCount(), after.Commanders[7].Available, after.Commanders[8].Available)
	}
}

func replaceOnce(text string, old string, replacement string) string {
	if !strings.Contains(text, old) {
		panic("replaceOnce: " + old + " not found")
	}
	return strings.Replace(text, old, replacement, 1)
}

func TestMovementsEquivalentIgnoresOnlyReceiveTimeDerivedFields(t *testing.T) {
	base := movementTestStart()
	arrives := base.Add(900 * time.Second)
	commander := State.CommanderID(7)
	original := State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, TargetPlayerID: 99, TravelSeconds: 900, ProgressSeconds: 100,
		ObservedAt: base, StartedAt: base.Add(-100 * time.Second), ArrivesAt: &arrives, CommanderID: &commander,
		Units: map[State.UnitID]int64{6: 40}, TargetX: 20, TargetY: 21, KingdomID: 0,
	}
	shifted := func(mutate func(*State.MovementState)) State.MovementState {
		next := original
		next.Units = map[State.UnitID]int64{6: 40}
		later := base.Add(5*time.Second + 300*time.Millisecond)
		arrivalJitter := arrives.Add(900 * time.Millisecond)
		next.ObservedAt, next.ProgressSeconds = later, 105
		next.StartedAt = later.Add(-105 * time.Second)
		next.ArrivesAt = &arrivalJitter
		if mutate != nil {
			mutate(&next)
		}
		return next
	}
	if !movementsEquivalent(original, shifted(nil)) {
		t.Fatal("the same movement seen 5 s later with sub-second jitter must be equivalent")
	}
	for name, mutate := range map[string]func(*State.MovementState){
		"travel time":            func(m *State.MovementState) { m.TravelSeconds = 650 },
		"units":                  func(m *State.MovementState) { m.Units[6] = 39 },
		"direction":              func(m *State.MovementState) { m.Direction = 1 },
		"target":                 func(m *State.MovementState) { m.TargetX = 21 },
		"owner":                  func(m *State.MovementState) { m.OwnerPlayerID = 2 },
		"wait":                   func(m *State.MovementState) { m.WaitSeconds = 3600 },
		"start beyond tolerance": func(m *State.MovementState) { m.StartedAt = m.StartedAt.Add(3 * time.Second) },
		"arrival beyond tolerance": func(m *State.MovementState) {
			later := arrives.Add(5 * time.Second)
			m.ArrivesAt = &later
		},
		"arrival vanished": func(m *State.MovementState) { m.ArrivesAt = nil },
		"return appeared": func(m *State.MovementState) {
			returns := arrives
			m.ReturnsAt = &returns
		},
		"commander": func(m *State.MovementState) { other := State.CommanderID(8); m.CommanderID = &other },
	} {
		if movementsEquivalent(original, shifted(mutate)) {
			t.Errorf("a changed %s must not be equivalent", name)
		}
	}
}

// Khan retaliation movements and an unmatched invasion reservation are the other
// gam consumers that keep their own time bookkeeping; identical polls must stay quiet for them too.
func TestIdenticalPollsStayQuietWithKhanTauntAndPendingInvasionReservation(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	if _, err := store.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Castles[100] = func() State.CastleState {
			castle := newCastleState(100)
			castle.KingdomID, castle.SlotType, castle.X, castle.Y = 0, 1, 10, 11
			return castle
		}()
		gameState.Khan.TargetX, gameState.Khan.TargetY = 900, 901
		gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
			KingdomID: 0, EventID: 71, OccurrenceEndsAt: movementTestStart().Add(12 * time.Hour),
			TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
			SourceCastleID: 100, SourceX: 10, SourceY: 11, SourceKnown: true,
			CommanderID: 9, CommanderKnown: true, OperationID: "pending-op",
			ReservedAt: movementTestStart().Add(-time.Minute),
		})
		return []string{"invasion"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	payload := func(elapsed int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"M":[
			{"M":{"MID":70,"PT":%d,"TT":300,"D":0,"T":%d,"KID":0,"OID":-9,"TID":1,"SA":[%d,900,901,-5,-9],"TA":[1,10,11,100,1]},"A":[[6,100]],"UM":{"L":{"WID":4242}}}
		],"O":[]}`, 20+elapsed, khanTauntMovementTypeID, khanCampMapTypeID))
	}
	start := movementTestStart()
	first := sendPoll(t, pipeline, payload(0), start)
	if first.Revision == 0 {
		t.Fatalf("first poll did not commit: %+v", first)
	}
	if len(store.ReadOnlyView().Khan.Taunts) != 1 {
		t.Fatal("the Khan retaliation movement was not tracked as a taunt")
	}
	events, unsubscribe := store.Subscribe(16)
	defer unsubscribe()
	for poll := 1; poll <= 10; poll++ {
		committed := sendPoll(t, pipeline, payload(poll*5), start.Add(time.Duration(poll)*5*time.Second+time.Duration(poll%3)*100*time.Millisecond))
		if committed.Revision != first.Revision {
			t.Fatalf("poll %d committed revision %d (domains %v), want the quiet revision %d", poll, committed.Revision, committed.Domains, first.Revision)
		}
	}
	select {
	case event := <-events:
		t.Fatalf("event emitted for an unchanged poll: %v", event.Domains)
	default:
	}
}

// The client treats the movement snapshot as ready only when its connection
// generation matches the session's, so the first snapshot of a connection is a
// real change even when the movements themselves did not change.
func TestFirstSnapshotOfANewConnectionCommitsEvenWhenMovementsAreUnchanged(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	start := movementTestStart()
	code := 0
	empty := json.RawMessage(`{"M":[],"O":[]}`)
	send := func(at time.Time) Protocol.CommittedFrame {
		committed, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
			Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at, Payload: empty,
		})
		if err != nil {
			t.Fatal(err)
		}
		return committed
	}
	first := send(start)
	if first.Revision == 0 || store.ReadOnlyView().MovementSnapshot.Version == 0 {
		t.Fatalf("the very first snapshot must commit: %+v", first)
	}
	if again := send(start.Add(5 * time.Second)); again.Revision != first.Revision {
		t.Fatalf("an unchanged poll on the same connection committed revision %d", again.Revision)
	}
	// A new game connection: the session generation moves on, movements are still empty.
	if _, err := store.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.ConnectionGeneration = 2
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	before := store.Revision()
	committed := send(start.Add(10 * time.Second))
	snapshot := store.ReadOnlyView().MovementSnapshot
	if committed.Revision != before+1 || snapshot.ConnectionGeneration != 2 {
		t.Fatalf("first snapshot of connection 2: revision %d (want %d), snapshot %+v", committed.Revision, before+1, snapshot)
	}
	// And the next poll on connection 2 is quiet again.
	if again := send(start.Add(15 * time.Second)); again.Revision != committed.Revision {
		t.Fatalf("an unchanged poll on connection 2 committed revision %d", again.Revision)
	}
}

// A movement the game still lists at or after its nominal end pushes the
// commander's release to that sighting plus the grace. Keeping the held record
// would freeze ObservedAt and free the commander while the movement is listed.
func TestLingeringMovementKeepsTheCommanderBusyUntilTheGameDropsIt(t *testing.T) {
	pipeline, store := quietPollFixture(t)
	start := movementTestStart()
	sendPoll(t, pipeline, pollPayload(0), start)
	first, _ := store.ReadOnlyView().LookupMovement(50)
	nominal := *State.CommanderMovementReleaseAt(first) // release = nominal end + grace
	nominalEnd := nominal.Add(-State.CommanderMovementReturnGrace)

	// Well before the end: identical polls stay quiet (the optimisation still works).
	events, unsubscribe := store.Subscribe(16)
	defer unsubscribe()
	before := store.Revision()
	sendPoll(t, pipeline, pollPayload(60), start.Add(60*time.Second))
	if store.Revision() != before {
		t.Fatal("an early identical poll committed")
	}

	// Ten seconds past the nominal end (arrival plus the return trip) the game still lists it.
	late := nominalEnd.Add(10 * time.Second)
	lateElapsed := int(late.Sub(start) / time.Second)
	before = store.Revision()
	committed := sendPoll(t, pipeline, pollPayload(lateElapsed), late)
	if committed.Revision != before+1 {
		t.Fatalf("a movement still listed past its nominal end must commit: revision %d, want %d", committed.Revision, before+1)
	}
	<-events
	view := store.ReadOnlyView()
	lingering, _ := view.LookupMovement(50)
	if !lingering.ObservedAt.Equal(late) {
		t.Fatalf("stored ObservedAt = %s, want the late sighting %s (a frozen ObservedAt frees the commander early)", lingering.ObservedAt, late)
	}
	if release := *State.CommanderMovementReleaseAt(lingering); !release.Equal(late.Add(State.CommanderMovementReturnGrace)) {
		t.Fatalf("release = %s, want sighting + grace = %s", release, late.Add(State.CommanderMovementReturnGrace))
	}
	if view.Commanders[7].Available {
		t.Fatal("commander 7 was freed while the game still lists his movement")
	}

	// The next reply without the movement releases him.
	payload := string(pollPayload(lateElapsed + 5))
	begin := strings.Index(payload, `{"M":{"MID":50`)
	end := strings.Index(payload, `{"M":{"MID":51`)
	sendPoll(t, pipeline, json.RawMessage(payload[:begin]+payload[end:]), late.Add(5*time.Second))
	if !store.ReadOnlyView().Commanders[7].Available {
		t.Fatal("commander 7 stayed busy after the game dropped the movement")
	}
}
