package Automation

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func stormArrivalFixture(t *testing.T) (State.GameState, *GameData.Store, State.CastleState, State.MapObservation, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("testdata/storm_arrival_kingdoms_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "fixture-786.03"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Player.ID = 71001
	state.Session.ConnectionGeneration = 7
	source := State.CastleState{ID: 81001, KingdomID: 4, X: 100, Y: 100}
	state.Castles[source.ID] = source
	fundAutoStormArrivalForTest(&state, source, now)
	state.Storm.TravelObservations = nil
	target := State.MapObservation{KingdomID: 4, TypeID: 25, X: 110, Y: 100}
	return state, data, source, target, now
}

func TestAutoStormArrivalBlockedPriorityStillAttacksCloser(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	source := autoStormTestCastle(81001, 4, "Synthetic Storm castle")
	source.X, source.Y = 100, 100
	source.Units.Stationed[10] = 20
	state.Castles[source.ID] = source
	state.Commanders[11] = State.CommanderState{ID: 11, Available: true}
	fundAutoStormArrivalForTest(&state, source, now)
	row := state.KingdomTransport.Unlocks[4]
	row.EventEndsAt = now.Add(90 * time.Second)
	state.KingdomTransport.Unlocks[4] = row
	for key, observation := range state.Storm.TravelObservations {
		observation.EventEndsAt = row.EventEndsAt
		state.Storm.TravelObservations[key] = observation
	}
	far := State.MapObservation{KingdomID: 4, TypeID: 25, X: 140, Y: 100, Level: 80, StormIsleID: 10, ObservedAt: now}
	near := State.MapObservation{KingdomID: 4, TypeID: 25, X: 110, Y: 100, Level: 40, StormIsleID: 7, ObservedAt: now}
	state.Storm.Map = State.StormMapState{SourceCastleID: source.ID, LastAttemptAt: now, LastCompletedAt: now,
		Targets: map[string]State.MapObservation{"140:100": far, "110:100": near}}
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled, settings.Forts.PresetID = true, "synthetic-preset"
	settings.Forts.Levels = []int{40, 80}
	settings.TargetPriority = []string{"fort:80", "fort:40"}
	unit := int64(10)
	presets, _ := json.Marshal(AttackPresets.Document{Version: 1, Presets: []AttackPresets.Preset{{
		ID: "synthetic-preset", Name: "Synthetic preset", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unit, Quantity: 1}}},
		}},
	}}})
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{AttackPresets.ConfigurationSection: presets}}}
	candidates := autoStormCombatCandidates(snapshot, settings, source, now)
	if len(candidates) != 2 || candidates[0].Observation.X != far.X {
		t.Fatalf("priority fixture: %+v", candidates)
	}
	decision, detail, err := evaluateAutoStormCombat(snapshot, settings, source, map[string]float64{})
	if err != nil || decision == nil || decision.Request == nil || decision.Request.Name != "storm.attack" {
		t.Fatalf("closer attack: %+v detail=%s err=%v", decision, detail, err)
	}
	var arguments struct {
		TargetX int `json:"targetX"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.TargetX != near.X {
		t.Fatal("late priority target starved the closer target")
	}
}

func fundAutoStormArrivalForTest(state *State.GameState, source State.CastleState, now time.Time) {
	state.Session.ChangedAt = now.Add(-time.Hour)
	end := now.Add(24 * time.Hour)
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{
		KingdomID: 4, Unlocked: true, EventEndsAt: end,
		EventEndObservedAt: now.Add(-time.Minute), EventObservedFrom: now.Add(-time.Hour),
		EventEndConnectionGeneration: state.Session.ConnectionGeneration,
	}
	option := int64(-1)
	state.Storm.TravelObservations = map[string]State.StormTravelObservation{"fixture": {
		SourceCastleID: source.ID, SourceX: source.X, SourceY: source.Y,
		TargetX: source.X + 1, TargetY: source.Y, HorseBoosterWID: &option,
		MovementID: 91001, TravelSeconds: 1, StartedAt: now.Add(-time.Minute), EventEndsAt: end, SlowestSecondsPerTile: 1,
	}}
}

func addStormArrivalReport(t *testing.T, state *State.GameState, source State.CastleState, x, seconds int, id State.MovementID, started, observed time.Time, option *int64) {
	t.Helper()
	if !state.ObserveStormTravel(State.MovementState{
		ID: id, OwnerPlayerID: state.Player.ID, SourceCastleID: source.ID, KingdomID: 4,
		SourceX: source.X, SourceY: source.Y, TargetX: x, TargetY: source.Y, TargetTypeID: 25,
		TravelSeconds: seconds, StartedAt: started, ObservedAt: observed, HorseBoosterWID: option,
	}) {
		t.Fatal("own Storm TT report was not retained")
	}
}

func TestStormArrivalStrictBoundaries(t *testing.T) {
	for _, kind := range []string{"CRA", "KUT"} {
		for _, margin := range []time.Duration{time.Second, 0, -time.Second} {
			t.Run(kind+margin.String(), func(t *testing.T) {
				state, data, source, target, now := stormArrivalFixture(t)
				seconds := 180
				if kind == "KUT" {
					seconds = 7200
				}
				row := state.KingdomTransport.Unlocks[4]
				row.EventEndsAt = now.Add(time.Duration(seconds)*time.Second + stormDispatchAllowance + margin)
				state.KingdomTransport.Unlocks[4] = row
				option := int64(-1)
				addStormArrivalReport(t, &state, source, target.X, seconds, 91002, now.Add(-30*time.Second), now, &option)
				block := StormAttackArrivalBlock(&state, data, source, target, -1, now)
				if kind == "KUT" {
					block = StormKingdomArrivalBlock(&state, data, 4, now)
				}
				if (block == nil) != (margin > 0) {
					t.Fatalf("arrival margin %s: block=%+v", margin, block)
				}
			})
		}
	}
}

func TestStormArrivalUsesNewestMatchingTTOrSlowestSourceRate(t *testing.T) {
	state, _, source, target, now := stormArrivalFixture(t)
	option := int64(-1)
	end, _ := state.StormEventEndAt(now)
	addStormArrivalReport(t, &state, source, target.X, 100, 91004, now.Add(-20*time.Second), now, &option)
	addStormArrivalReport(t, &state, source, target.X, 900, 91003, now.Add(-40*time.Second), now, &option)
	addStormArrivalReport(t, &state, source, 120, 400, 91005, now.Add(-10*time.Second), now, nil)
	seconds, found := stormObservedTravelSeconds(&state, source, target, -1, end)
	if !found || seconds != 100 {
		t.Fatalf("newest matching TT=%v known=%t", seconds, found)
	}
	target.X = 130
	seconds, found = stormObservedTravelSeconds(&state, source, target, -1, end)
	if !found || seconds != 2700 {
		t.Fatalf("slowest retained rate scaled to distance=%v known=%t", seconds, found)
	}
	target.X = 110
	seconds, found = stormObservedTravelSeconds(&state, source, target, 1007, end)
	if !found || seconds != 900 {
		t.Fatalf("different travel option must use source rate: %v %t", seconds, found)
	}
}

func TestStormArrivalUnknownStaleAndOtherKingdoms(t *testing.T) {
	for _, scenario := range []string{"missing", "expired", "future", "reconnected", "pre-session", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			state, data, source, target, now := stormArrivalFixture(t)
			row := state.KingdomTransport.Unlocks[4]
			switch scenario {
			case "missing":
				row.EventEndsAt = time.Time{}
			case "expired":
				row.EventEndsAt = now
			case "future":
				row.EventEndObservedAt = now.Add(time.Second)
			case "reconnected":
				state.Session.ConnectionGeneration++
			case "pre-session":
				state.Session.ChangedAt = now
			case "closed":
				row.Unlocked = false
			}
			state.KingdomTransport.Unlocks[4] = row
			if StormAttackArrivalBlock(&state, data, source, target, -1, now) == nil || StormKingdomArrivalBlock(&state, data, 4, now) == nil {
				t.Fatal("unknown/stale event authorized a move")
			}
			if StormKingdomArrivalBlock(&state, nil, 2, now) != nil {
				t.Fatal("other kingdom transfer regressed")
			}
		})
	}
}

func TestStormArrivalCloserTargetStillFitsAndDispatchTimeChangesBound(t *testing.T) {
	state, data, source, target, now := stormArrivalFixture(t)
	row := state.KingdomTransport.Unlocks[4]
	row.EventEndsAt = now.Add(300 * time.Second)
	state.KingdomTransport.Unlocks[4] = row
	addStormArrivalReport(t, &state, source, 110, 100, 91006, now.Add(-10*time.Second), now, nil)
	target.X = 140
	if StormAttackArrivalBlock(&state, data, source, target, -1, now) == nil {
		t.Fatal("far target authorized")
	}
	target.X = 110
	if StormAttackArrivalBlock(&state, data, source, target, -1, now) != nil {
		t.Fatal("closer target blocked")
	}
	if StormAttackArrivalBlock(&state, data, source, target, -1, now.Add(140*time.Second)) == nil {
		t.Fatal("dispatch-time boundary did not block")
	}
}
