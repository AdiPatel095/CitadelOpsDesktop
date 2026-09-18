package State

import (
	"testing"
	"time"
)

func TestNomadSequentialArrivalGuardUsesLiveAndCurrentOccurrenceFallback(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	gameState := NewGameState()
	gameState.Castles[99] = CastleState{ID: 99, KingdomID: 0}
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = ScalableEventScore{EventID: 80, RemainingSec: 3600, ObservedAt: now}
	commander := CommanderID(4)
	farArrival := now.Add(NomadSequentialArrivalGuardHorizon + time.Nanosecond)
	gameState.Movements[1] = MovementState{
		ID: 1, Direction: 0, SourceCastleID: 99, KingdomID: 0, TargetTypeID: 29,
		TargetX: 10, TargetY: 20, CommanderID: &commander, ArrivesAt: &farArrival,
	}
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); found {
		t.Fatalf("arrival outside horizon blocked: %#v", block)
	}

	nearArrival := now.Add(NomadSequentialArrivalGuardHorizon)
	gameState.Movements[1] = MovementState{
		ID: 1, Direction: 0, SourceCastleID: 99, KingdomID: 0, TargetTypeID: 29,
		TargetX: 10, TargetY: 20, CommanderID: &commander, ArrivesAt: &nearArrival,
	}
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); !found || !block.Live || !block.ArrivesAt.Equal(nearArrival) {
		t.Fatalf("exact horizon was not blocked across source castles: block=%#v found=%t", block, found)
	}

	delete(gameState.Movements, 1)
	if !RecordEventAttackLaunch(&gameState, 80, EventAttackRecord{
		MovementID: 1, Kind: EventActivityCamp, KingdomID: 0, TargetTypeID: 29,
		TargetX: 10, TargetY: 20, LaunchedAt: now.Add(-time.Minute), ArrivesAt: nearArrival,
	}) {
		t.Fatal("could not stage current-occurrence launch")
	}
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); !found || block.Live {
		t.Fatalf("persisted current-occurrence launch did not block: block=%#v found=%t", block, found)
	}
}

func TestNomadSequentialArrivalGuardRequiresExactClearRowAfterArrivalHorizon(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	arrival := now.Add(-10 * time.Second)
	gameState := NewGameState()
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = ScalableEventScore{EventID: 80, RemainingSec: 3600, ObservedAt: now}
	if !RecordEventAttackLaunch(&gameState, 80, EventAttackRecord{
		MovementID: 7, Kind: EventActivityCamp, KingdomID: 0, TargetTypeID: 29,
		TargetX: 10, TargetY: 20, LaunchedAt: now.Add(-time.Minute), ArrivesAt: arrival,
	}) {
		t.Fatal("could not stage pending launch")
	}
	key := "0:10:20"
	gameState.NomadCamps.Cooldowns[key] = NomadCampCooldownState{
		KingdomID: 0, X: 10, Y: 20,
		// A delayed earlier report is not per-movement settlement proof.
		LastSuccessfulBattleAt: now.Add(time.Minute),
		CooldownObservedAt:     arrival.Add(NomadSequentialArrivalGuardHorizon - time.Nanosecond),
	}
	if _, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); !found {
		t.Fatal("pre-threshold clear row or delayed report settled the pending movement")
	}
	cooldown := gameState.NomadCamps.Cooldowns[key]
	cooldown.LastSuccessfulBattleAt = time.Time{}
	cooldown.CooldownObservedAt = arrival.Add(NomadSequentialArrivalGuardHorizon)
	gameState.NomadCamps.Cooldowns[key] = cooldown
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); found {
		t.Fatalf("exact clear row after arrival horizon did not settle fallback: %#v", block)
	}

	cooldown.CooldownRemaining = 100
	gameState.NomadCamps.Cooldowns[key] = cooldown
	if _, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); !found {
		t.Fatal("positive cooldown incorrectly released the pending launch")
	}
}

func TestNomadSequentialArrivalGuardAcceptsFreshMapClearWithoutCooldownRecord(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	arrival := now.Add(-10 * time.Second)
	gameState := NewGameState()
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = ScalableEventScore{EventID: 80, RemainingSec: 3600, ObservedAt: now}
	if !RecordEventAttackLaunch(&gameState, 80, EventAttackRecord{
		MovementID: 8, Kind: EventActivityCamp, KingdomID: 0, TargetTypeID: 29,
		TargetX: 10, TargetY: 20, LaunchedAt: now.Add(-time.Minute), ArrivesAt: arrival,
	}) {
		t.Fatal("could not stage pending launch")
	}
	gameState.Map[0] = map[string]MapObservation{"10:20": {
		KingdomID: 0, TypeID: 29, X: 10, Y: 20, EventCampCooldownRemaining: 0,
		ObservedAt: arrival.Add(NomadSequentialArrivalGuardHorizon),
	}}
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, now); found {
		t.Fatalf("fresh exact clear map row without cooldown record did not settle: %#v", block)
	}
}

func TestNomadSequentialArrivalGuardAllowsWindowBetweenFourSecondArrivals(t *testing.T) {
	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	gameState := NewGameState()
	gameState.Castles[1] = CastleState{ID: 1, KingdomID: 0}
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = ScalableEventScore{EventID: 80, RemainingSec: 3600, ObservedAt: base}
	first, second := base.Add(time.Second), base.Add(5*time.Second)
	for _, record := range []EventAttackRecord{
		{MovementID: 1, Kind: EventActivityCamp, KingdomID: 0, TargetTypeID: 29, TargetX: 10, TargetY: 20, LaunchedAt: base, ArrivesAt: first},
		{MovementID: 2, Kind: EventActivityCamp, KingdomID: 0, TargetTypeID: 29, TargetX: 10, TargetY: 20, LaunchedAt: base, ArrivesAt: second},
	} {
		if !RecordEventAttackLaunch(&gameState, 80, record) {
			t.Fatalf("could not stage launch %d", record.MovementID)
		}
	}
	gameState.Movements[1] = MovementState{ID: 1, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetTypeID: 29, TargetX: 10, TargetY: 20, ArrivesAt: &first}
	gameState.Movements[2] = MovementState{ID: 2, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetTypeID: 29, TargetX: 10, TargetY: 20, ArrivesAt: &second}
	if _, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, base); !found {
		t.Fatal("first arrival inside horizon was not blocked")
	}
	activity, _ := gameState.MutableEventActivity(80)
	activity.PendingAttacks = activity.PendingAttacks[1:]
	gameState.SetEventActivity(80, activity)
	gameState.NomadCamps.Cooldowns["0:10:20"] = NomadCampCooldownState{
		KingdomID: 0, X: 10, Y: 20, LastSuccessfulBattleAt: second.Add(time.Minute),
	}
	if block, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, base.Add(2*time.Second)); found {
		t.Fatalf("exact movement reconciliation did not open the safe four-second window: %#v", block)
	}
	if _, found := NomadSequentialArrivalBlockAt(gameState, 80, 0, 29, 10, 20, base.Add(3*time.Second)); !found {
		t.Fatal("delayed generic battle time cleared the later pending movement")
	}
}
