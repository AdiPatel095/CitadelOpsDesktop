package App

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func hospitalHealTestContext(t *testing.T) Intent.PlanningContext {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
        "versionInfo":[],"buildings":[],"units":[{"wodID":489}],
        "effects":[{"effectID":103,"effectTypeID":106,"name":"hospitalSlotBonus"}],
        "researches":[
            {"researchID":271,"effects":"103&1"},{"researchID":272,"effects":"103&1"},
            {"researchID":273,"effects":"103&1"},{"researchID":274,"effects":"103&1"},
            {"researchID":275,"effects":"103&1"}],
        "subscriptionsBuffs":[{"subscriptionTypeID":1,"effects":"103&5,189&40"}]
    }`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.Generation = 1
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.Research = State.ResearchState{CompletedIDs: map[int64]bool{}, ObservedAt: now.Add(time.Second), Generation: 1}
	state.SubscriptionsObservedAt = now.Add(2 * time.Second)
	state.SubscriptionsGeneration = 1
	state.Castles[77] = State.CastleState{
		ID: 77, Focused: true, ContextSnapshotObservedAt: now, UnitsObservedAt: now,
		Units:      State.CastleUnits{Hospital: map[State.UnitID]int64{489: 20}},
		Production: map[int]State.ProductionQueue{2: {LineID: 2, Capacity: 5, ObservedAt: now}},
	}
	return Intent.PlanningContext{State: state, GameData: store}
}

func hospitalResolvedAmount(t *testing.T, input Intent.PlanningContext, requested int64) (int64, error) {
	t.Helper()
	arguments, _ := json.Marshal(map[string]any{"castleId": 77, "unitId": 489, "amount": requested})
	step, err := resolveHospitalHealStep(t.Context(), input, arguments)
	if err != nil {
		return 0, err
	}
	if step.Opcode != "hru" {
		t.Fatalf("opcode %s, want hru", step.Opcode)
	}
	var payload struct {
		Amount int64 `json:"A"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Amount > requested || payload.Amount > 15 {
		t.Fatalf("over-cap HRU amount %d, requested %d", payload.Amount, requested)
	}
	return payload.Amount, nil
}

func TestHospitalHealResolvedBonusesAndUpperBounds(t *testing.T) {
	cases := []struct {
		name                     string
		research                 []int64
		subscription             State.SubscriptionState
		subscriptionObserved     bool
		requested, wounded, want int64
	}{
		{name: "no bonuses", requested: 15, wounded: 20, want: 5},
		{name: "partial research", research: []int64{271, 272}, requested: 15, wounded: 20, want: 7},
		{name: "full research", research: []int64{271, 272, 273, 274, 275}, requested: 15, wounded: 20, want: 10},
		{name: "active subscription", subscription: State.SubscriptionState{TypeID: 1, RemainingSec: 60}, subscriptionObserved: true, requested: 15, wounded: 20, want: 10},
		{name: "expired subscription", subscription: State.SubscriptionState{TypeID: 1, RemainingSec: 0}, subscriptionObserved: true, requested: 15, wounded: 20, want: 5},
		{name: "research and subscription", research: []int64{271, 272, 273, 274, 275}, subscription: State.SubscriptionState{TypeID: 1, RemainingSec: 60}, subscriptionObserved: true, requested: 20, wounded: 20, want: 15},
		{name: "manual request upper bound", research: []int64{271, 272, 273, 274, 275}, subscription: State.SubscriptionState{TypeID: 1, RemainingSec: 60}, subscriptionObserved: true, requested: 4, wounded: 20, want: 4},
		{name: "wounded count upper bound", research: []int64{271, 272, 273, 274, 275}, subscription: State.SubscriptionState{TypeID: 1, RemainingSec: 60}, subscriptionObserved: true, requested: 15, wounded: 3, want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := hospitalHealTestContext(t)
			for _, id := range tc.research {
				input.State.Research.CompletedIDs[id] = true
			}
			if tc.subscriptionObserved {
				input.State.Subscriptions[1] = tc.subscription
			}
			castle := input.State.Castles[77]
			castle.Units.Hospital[489] = tc.wounded
			input.State.Castles[77] = castle
			got, err := hospitalResolvedAmount(t, input, tc.requested)
			if err != nil || got != tc.want {
				t.Fatalf("amount=%d err=%v, want %d", got, err, tc.want)
			}
		})
	}
}

func TestHospitalHealMissingOrStaleEvidenceFallsBackToBase(t *testing.T) {
	input := hospitalHealTestContext(t)
	input.State.Research.CompletedIDs[271] = true
	input.State.Subscriptions[1] = State.SubscriptionState{TypeID: 1, RemainingSec: 60}
	input.State.Research.ObservedAt = time.Time{}
	input.State.SubscriptionsObservedAt = time.Time{}
	got, err := hospitalResolvedAmount(t, input, 15)
	if err != nil || got != 5 {
		t.Fatalf("amount=%d err=%v, want base 5", got, err)
	}
}

func TestHospitalHealSubscriptionExpiryAfterSnapshotFallsBackToBase(t *testing.T) {
	input := hospitalHealTestContext(t)
	castle := input.State.Castles[77]
	castle.ContextSnapshotObservedAt = time.Now().UTC().Add(-3 * time.Second)
	castle.UnitsObservedAt = castle.ContextSnapshotObservedAt
	queue := castle.Production[2]
	queue.ObservedAt = castle.ContextSnapshotObservedAt
	castle.Production[2] = queue
	input.State.Castles[77] = castle
	input.State.SubscriptionsObservedAt = time.Now().UTC().Add(-2 * time.Second)
	input.State.Subscriptions[1] = State.SubscriptionState{TypeID: 1, RemainingSec: 1}
	got, err := hospitalResolvedAmount(t, input, 15)
	if err != nil || got != 5 {
		t.Fatalf("expired subscription amount=%d err=%v, want 5", got, err)
	}
}

func TestHospitalHealRejectsStaleWoundsOrFullQueueAfterJAA(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*State.CastleState)
	}{
		{"stale wounds", func(c *State.CastleState) { c.UnitsObservedAt = c.ContextSnapshotObservedAt.Add(-time.Second) }},
		{"stale queue", func(c *State.CastleState) {
			q := c.Production[2]
			q.ObservedAt = c.ContextSnapshotObservedAt.Add(-time.Second)
			c.Production[2] = q
		}},
		{"full queue", func(c *State.CastleState) {
			q := c.Production[2]
			q.Capacity = 1
			q.Active = &State.QueueItem{}
			c.Production[2] = q
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := hospitalHealTestContext(t)
			castle := input.State.Castles[77]
			tc.mutate(&castle)
			input.State.Castles[77] = castle
			if _, err := hospitalResolvedAmount(t, input, 15); !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatalf("err=%v, want stale", err)
			}
		})
	}
}
