package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// wakeCountingPolicy is an event-driven policy on the two domains every movement
// poll used to wake; it counts how often the coordinator evaluates it.
type wakeCountingPolicy struct {
	evaluations atomic.Int32
}

// movementTestStart is taken from the clock rather than pinned to a date so the
// test does not depend on when it runs (see the Ingest movement tests).
func movementTestStart() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func (*wakeCountingPolicy) ID() string         { return "movement-watcher" }
func (*wakeCountingPolicy) EnabledKey() string { return "movement-watcher" }
func (*wakeCountingPolicy) WakeDomains() []string {
	return []string{"movements", "commanders", "movement-snapshot"}
}
func (policy *wakeCountingPolicy) Evaluate(context.Context, Snapshot) (Decision, error) {
	policy.evaluations.Add(1)
	return Decision{Status: "armed", EventDriven: true}, nil
}

func movementPollForCoordinator(elapsed int, extra bool) json.RawMessage {
	incoming := ""
	if extra {
		incoming = `,{"M":{"MID":60,"PT":3,"TT":600,"D":0,"T":0,"KID":0,"OID":3,"TID":1,"SA":[0,80,81,900,3],"TA":[0,10,11,100,1]},"A":[[6,900]],"UM":{"L":{"ID":-1}}}`
	}
	return json.RawMessage(fmt.Sprintf(`{"M":[
		{"M":{"MID":50,"PT":%d,"TT":900,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"A":[[6,40]],"UM":{"L":{"ID":7}}}%s
	],"O":[]}`, 100+elapsed, incoming))
}

// CIT-30 acceptance: an unchanged poll wakes no policy; a real movement change
// still wakes the policies that depend on movements.
func TestUnchangedMovementPollsWakeNoPolicyButRealChangesStillDo(t *testing.T) {
	initial := coordinatorReadyState()
	initial.Player.ID = 1
	initial.Session.ConnectionGeneration = 1
	initial.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	store := State.NewStore(initial)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	policy := &wakeCountingPolicy{}
	configuration := openCoordinatorTestConfiguration(t, policy.ID())
	coordinator := NewCoordinator(store, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}, policy)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go coordinator.Run(ctx)

	waitFor := func(description string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s (evaluations %d)", description, policy.evaluations.Load())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	poll := func(payload json.RawMessage, at time.Time) {
		t.Helper()
		code := 0
		if _, err := pipeline.HandleFrame(ctx, Protocol.Frame{
			Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at, Payload: payload,
		}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor("the initial evaluation", func() bool { return policy.evaluations.Load() == 1 })

	start := movementTestStart()
	poll(movementPollForCoordinator(0, false), start)
	waitFor("the first poll's targeted wake", func() bool { return policy.evaluations.Load() == 2 })
	revision := store.Revision()

	for index := 1; index <= 12; index++ {
		poll(movementPollForCoordinator(index*5, false), start.Add(time.Duration(index)*5*time.Second))
	}
	// Longer than the state-change debounce: a wake would have evaluated by now.
	time.Sleep(600 * time.Millisecond)
	if got := policy.evaluations.Load(); got != 2 || store.Revision() != revision {
		t.Fatalf("12 unchanged polls: %d evaluations (want 2), revision %d (want %d)", got, store.Revision(), revision)
	}

	poll(movementPollForCoordinator(65, true), start.Add(65*time.Second))
	waitFor("the wake for a real movement change", func() bool { return policy.evaluations.Load() == 3 })
}

// These are the policies a movement poll used to wake every 5 s per runtime
// whether or not anything moved. CIT-30 wakes them only for real changes; this
// list is pinned so anyone adding a movement-domain subscriber sees the trade-off.
func TestPoliciesSubscribedToMovementDomains(t *testing.T) {
	all := []Policy{
		NewRecruitPolicy(), NewToolPolicy(), NewHospitalPolicy(), NewAllianceHelpPolicy(), NewAutoEquipmentCleanupPolicy(),
		NewDailyAttackRefreshPolicy(), NewConstructionPolicy(), NewCraftingPolicy(), NewCraftingLogisticsPolicy(),
		NewAutoBirdPolicy(), NewAutoStationPolicy(), NewBeriPolicy(), NewBeriToolPolicy(), NewBeriBuildPolicy(),
		NewBeriAttackPolicy(), NewFoodBalancePolicy(), NewAutoTowerPolicy(), NewInvasionRecoveryPolicy(),
		NewAutoFortressPolicy(), NewAutoInvasionPolicy(), NewAutoNomadPolicy(), NewAutoAdvisorPolicy(),
		NewAutoBoosterPolicy(), NewAutoBuyerPolicy(), NewRiftMaidenRunPolicy(), NewAutoKhanPolicy(),
		NewAutoKhanCooldownPolicy(), NewAutoKhanRagePolicy(), NewAutoKhanDefensePolicy(), NewAutoStormPolicy(),
		NewAutoStormShopPolicy(), NewAutoStormBuildPolicy(),
	}
	got := []string{}
	for _, policy := range all {
		waker, ok := policy.(interface{ WakeDomains() []string })
		if !ok {
			continue
		}
		for _, domain := range waker.WakeDomains() {
			if domain == "movements" || domain == "commanders" || domain == "movement-snapshot" {
				got = append(got, policy.ID())
				break
			}
		}
	}
	want := []string{
		"autoSceatResLogistics", "autoBird", "autoStation", "autoBeriWorldBuild", "autoBeriWorldAttack", "autoTowers",
		"autoInvasionRecovery", "autoFortress", "autoInvasion", "autoNomad", "autoAdvisor", "riftMaidenRun",
		"autoKhan", "autoKhan:cooldown", "autoKhan:rage", "autoKhan:defense", "autoStorm", "autoStormShop",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("policies woken by movement domains changed:\n got %v\nwant %v\nUpdate CIT-30's wake-timing note and this list together.", got, want)
	}
}
