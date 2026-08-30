package Automation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriAttackPolicyUsesSharedFeatureControls(t *testing.T) {
	policy := NewBeriAttackPolicy()
	if policy.ActorID() != "autoBeriWorld" || policy.ScheduleKey() != "autoBeriWorld" ||
		policy.EnabledKey() != "auto_beri_world" {
		t.Fatalf("unexpected feature controls: actor=%q schedule=%q enabled=%q", policy.ActorID(), policy.ScheduleKey(), policy.EnabledKey())
	}
	if got := policy.UrgentWakeDomains(); !reflect.DeepEqual(got, []string{"commanders", "movements", "units"}) {
		t.Fatalf("urgent wake domains = %v", got)
	}
	if !containsString(policy.WakeDomains(), "attacks") {
		t.Fatalf("daily attack-count changes do not wake the attack lane: %v", policy.WakeDomains())
	}
}

func TestBeriAttackPolicyHonorsDailyAttackLimitAndResumesAfterReset(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	snapshot := beriAttackTestSnapshot(t, now)
	snapshot.Configuration.Sections[autoBeriWorldSection] = json.RawMessage(
		`{"presetId":"beri","attackCheckIntervalSec":30,"dailyAttackLimit":1000,"horseTravelBoostId":-1}`,
	)
	snapshot.State.KingdomTransport.ObservedAt = now
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{KingdomID: 10, Unlocked: true}
	snapshot.State.DailyAttacks = State.DailyAttackState{Count: 1000, ServerThreshold: 3500, ObservedAt: now}

	decision, err := NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "1000 / 1000") ||
		decision.Metrics["dailyAttackLimit"] != 1000 || decision.Metrics["dailyAttackCount"] != 1000 {
		t.Fatalf("reached daily cap decision = %#v err=%v", decision, err)
	}

	snapshot.State.DailyAttacks = State.DailyAttackState{Count: 0, ServerThreshold: 3500, ObservedAt: now.Add(time.Minute)}
	decision, err = NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.camp.open" {
		t.Fatalf("post-reset decision = %#v err=%v", decision, err)
	}
}

func TestBeriAttackPolicyOpensCheapestNonPremiumCamp(t *testing.T) {
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	snapshot := beriAttackTestSnapshot(t, now)
	snapshot.State.KingdomTransport.ObservedAt = now
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{KingdomID: 10, Unlocked: true}

	decision, err := NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.camp.open" {
		t.Fatalf("camp decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CampID int64 `json:"campId"`
	}
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil || arguments.CampID != 1 {
		t.Fatalf("camp arguments = %s", decision.Request.Arguments)
	}
}

func TestBeriAttackPolicyFindsThenAttacksTower(t *testing.T) {
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	snapshot := beriAttackTestSnapshot(t, now)
	snapshot.Configuration.Sections[autoBeriWorldSection] = json.RawMessage(
		`{"presetId":"beri","attackCheckIntervalSec":30,"dailyAttackLimit":1000,"horseTravelBoostId":-1}`,
	)
	snapshot.State.DailyAttacks = State.DailyAttackState{Count: 25, ServerThreshold: 3500, ObservedAt: now}
	snapshot.State.Castles[900] = State.CastleState{
		ID: 900, KingdomID: 10, X: 300, Y: 600,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 400}},
	}
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{KingdomID: 10, Unlocked: true, Created: true}
	policy := NewBeriAttackPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.target.find" {
		t.Fatalf("find decision: %#v err=%v", decision, err)
	}
	if decision.NextCheckAt != now.Add(beriLaunchRetryInterval) {
		t.Fatalf("find retry = %s, want %s", decision.NextCheckAt, now.Add(beriLaunchRetryInterval))
	}

	observedAt := now.Add(time.Second)
	snapshot.Now = observedAt
	snapshot.State.Beri.TargetX = 321
	snapshot.State.Beri.TargetY = 654
	snapshot.State.Beri.TargetTypeID = 17
	snapshot.State.Beri.TargetObservedAt = observedAt
	snapshot.State.Map[10] = map[string]State.MapObservation{
		"321:654": {KingdomID: 10, X: 321, Y: 654, TypeID: 17, Level: 55, ObservedAt: observedAt},
	}
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tower.attack" {
		t.Fatalf("attack decision: %#v err=%v", decision, err)
	}

	arrivesAt := observedAt.Add(20 * time.Second)
	firstCommanderID := State.CommanderID(0)
	snapshot.Now = observedAt.Add(2 * time.Second)
	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, SourceCastleID: 900, TargetTypeID: 17, KingdomID: 10,
		TargetX: 321, TargetY: 654, CommanderID: &firstCommanderID, ArrivesAt: &arrivesAt,
	}
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tower.attack" {
		t.Fatalf("cached target attack decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CommanderID      State.CommanderID `json:"commanderId"`
		DailyAttackLimit int64             `json:"dailyAttackLimit"`
	}
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil ||
		arguments.CommanderID != 1 || arguments.DailyAttackLimit != 1000 {
		t.Fatalf("cached target attack arguments = %s", decision.Request.Arguments)
	}

	snapshot.State.Beri.TargetInvalidatedAt = snapshot.Now.Add(time.Millisecond)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.target.find" {
		t.Fatalf("invalidated target decision: %#v err=%v", decision, err)
	}
}

func TestBeriAttackPolicyRefillsReturnedCommanderWhileLaterMovementIsActive(t *testing.T) {
	now := time.Date(2026, 8, 4, 22, 15, 0, 0, time.UTC)
	snapshot := beriAttackTestSnapshot(t, now)
	snapshot.State.Castles[900] = State.CastleState{
		ID: 900, KingdomID: 10, X: 300, Y: 600,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 400}},
	}
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: true,
	}
	snapshot.State.Beri.TargetX = 321
	snapshot.State.Beri.TargetY = 654
	snapshot.State.Beri.TargetTypeID = 17
	snapshot.State.Beri.TargetObservedAt = now.Add(-time.Minute)
	snapshot.State.Map[10] = map[string]State.MapObservation{
		"321:654": {
			KingdomID: 10, X: 321, Y: 654, TypeID: 17, Level: 55,
			ObservedAt: snapshot.State.Beri.TargetObservedAt,
		},
	}
	returnedCommander := State.CommanderID(0)
	laterCommander := State.CommanderID(1)
	returnedAt := now.Add(-State.CommanderMovementReturnGrace - time.Second)
	laterReturn := now.Add(time.Minute)
	snapshot.State.Movements[10] = State.MovementState{
		ID: 10, Direction: 1, SourceCastleID: 900, KingdomID: 10,
		CommanderID: &returnedCommander, ReturnsAt: &returnedAt,
	}
	snapshot.State.Movements[11] = State.MovementState{
		ID: 11, Direction: 1, SourceCastleID: 900, KingdomID: 10,
		CommanderID: &laterCommander, ReturnsAt: &laterReturn,
	}

	decision, err := NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tower.attack" {
		t.Fatalf("rolling refill decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CommanderID State.CommanderID `json:"commanderId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.CommanderID != returnedCommander {
		t.Fatalf("rolling refill commander = %d, want returned commander %d", arguments.CommanderID, returnedCommander)
	}
}

func TestBeriAttackPolicyKeepsCachedTargetAndReservesUnreflectedInventory(t *testing.T) {
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	snapshot := beriAttackTestSnapshot(t, now)
	observedAt := now.Add(-10 * time.Minute)
	snapshot.State.Castles[900] = State.CastleState{
		ID: 900, KingdomID: 10, X: 300, Y: 600, UnitsObservedAt: now.Add(-time.Minute),
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 200}},
	}
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: true,
	}
	snapshot.State.Beri.TargetX = 321
	snapshot.State.Beri.TargetY = 654
	snapshot.State.Beri.TargetTypeID = 17
	snapshot.State.Beri.TargetObservedAt = observedAt
	snapshot.State.Map[10] = map[string]State.MapObservation{
		"321:654": {KingdomID: 10, X: 321, Y: 654, TypeID: 17, Level: 55, ObservedAt: observedAt},
	}

	decision, err := NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tower.attack" {
		t.Fatalf("aged cached target decision: %#v err=%v", decision, err)
	}

	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(time.Minute)
	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, SourceCastleID: 900, TargetTypeID: 17, KingdomID: 10,
		TargetX: 321, TargetY: 654, CommanderID: &commanderID,
		StartedAt: now.Add(-30 * time.Second), ArrivesAt: &arrivesAt,
	}
	decision, err = NewBeriAttackPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "inventory to reflect 1 launched attack") {
		t.Fatalf("unreflected inventory decision: %#v err=%v", decision, err)
	}
}

func beriAttackTestSnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":"10","unitID":"10","unitType":"melee"}],
		"prebuiltcastles":[
			{"preBuiltCastleID":"3","spaceIDs":"10","minLevel":15,"costC2":"49000"},
			{"preBuiltCastleID":"2","spaceIDs":"10","minLevel":15,"costWood":"9000","costStone":"9000"},
			{"preBuiltCastleID":"1","spaceIDs":"10","minLevel":15,"costWood":"900","costStone":"900"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID := int64(10)
	presetDocument, _ := json.Marshal(AttackPresets.Document{
		Version: 1,
		Presets: []AttackPresets.Preset{{
			ID: "beri", Name: "Berimond",
			Waves: []AttackPresets.Wave{{
				Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 300}}},
			}},
		}},
	})
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoBeriWorldSection:               json.RawMessage(`{"presetId":"beri","attackCheckIntervalSec":30,"horseTravelBoostId":-1}`),
			AttackPresets.ConfigurationSection: presetDocument,
			commanderFeatureSection:            json.RawMessage(`{"version":1,"assignments":{"autoBeriWorld":[0,1]}}`),
		}},
	}
}
