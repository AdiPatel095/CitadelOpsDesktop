package Automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestMapFeatureWakesAreTargetedAndStormProgressIsSilent(t *testing.T) {
	worlds := State.NewWorldMapStore()
	policies := []Policy{
		NewAutoStormPolicy(), NewAutoTowerPolicy(), NewAutoNomadPolicy(), NewAutoInvasionPolicy(),
		NewBeriAttackPolicy(), NewRiftMaidenRunPolicy(), NewSharedStormScanPolicy("alpha", worlds),
	}
	indexed := indexPolicyWakeDomains(policies)
	if consumers := indexed["storm-scan-progress"]; len(consumers) != 0 {
		t.Fatalf("intermediate Storm tiles wake policies: %v", consumers)
	}
	if meaningfulStateEvent(State.Event{Revision: 1, Domains: []string{"storm-scan-progress"}}) {
		t.Fatal("intermediate Storm tile reached coordinator state handling")
	}
	for domain, expected := range map[string][]string{
		"map-storm":    {"autoStorm"},
		"map-tower":    {"autoTowers"},
		"map-invasion": {"autoInvasion"},
		"map-berimond": {"autoBeriWorldAttack"},
		"map-rift":     {"riftMaidenRun"},
		"storm-scan":   {"autoStorm", "sharedStormScan"},
	} {
		actual := append([]string(nil), indexed[domain]...)
		slices.Sort(actual)
		slices.Sort(expected)
		if !slices.Equal(actual, expected) {
			t.Fatalf("%s consumers = %v, want %v", domain, actual, expected)
		}
	}
}

func TestSharedStormScanPolicySplitsCoverageAcrossCapableAccounts(t *testing.T) {
	worlds := State.NewWorldMapStore()
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	type participant struct {
		policy *SharedStormScanPolicy
		state  State.GameState
	}
	participants := make([]participant, 0, 4)
	for index := 0; index < 4; index++ {
		state := State.NewGameState()
		state.Account.WorldID = "world-one"
		castleID := State.CastleID(index + 1)
		state.Castles[castleID] = State.CastleState{ID: castleID, KingdomID: autoStormKingdomID, Focused: true}
		store := State.NewStoreWithWorldMap(state, worlds)
		participants = append(participants, participant{
			policy: NewSharedStormScanPolicy(fmt.Sprintf("account-%d", index), worlds),
			state:  store.ReadOnlyView(),
		})
	}
	for _, item := range participants {
		decision, err := item.policy.Evaluate(nil, Snapshot{State: item.state, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Request != nil {
			t.Fatal("participant received work before the roster settled")
		}
	}

	seen := map[string]struct{}{}
	workCounts := make([]int, 0, len(participants))
	for _, item := range participants {
		decision, err := item.policy.Evaluate(nil, Snapshot{
			State: item.state, Now: now.Add(3 * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Request == nil || decision.Request.Name != "storm.map.scan" {
			t.Fatalf("shared scan decision = %+v", decision)
		}
		var request struct {
			Cooperative bool                   `json:"cooperative"`
			LeaseID     string                 `json:"leaseId"`
			Windows     []State.StormMapBounds `json:"windows"`
		}
		if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
			t.Fatal(err)
		}
		if !request.Cooperative || request.LeaseID == "" || len(request.Windows) == 0 {
			t.Fatalf("cooperative request = %+v", request)
		}
		workCounts = append(workCounts, len(request.Windows))
		for _, window := range request.Windows {
			key := fmt.Sprintf("%d:%d:%d:%d", window.X1, window.Y1, window.X2, window.Y2)
			if _, duplicate := seen[key]; duplicate {
				t.Fatalf("window %s was assigned twice", key)
			}
			seen[key] = struct{}{}
		}
	}
	if len(seen) != 25 {
		t.Fatalf("four-account coverage = %d unique windows, want 25", len(seen))
	}
	for _, count := range workCounts {
		if count < 6 || count > 7 {
			t.Fatalf("unfair four-account work counts = %v", workCounts)
		}
	}
}

func TestSharedStormScanPolicyIsCoreAndRequiresPrivateKingdomCapability(t *testing.T) {
	worlds := State.NewWorldMapStore()
	policy := NewSharedStormScanPolicy("alpha", worlds)
	if _, ok := any(policy).(CorePolicy); !ok {
		t.Fatal("shared scanner is not a core read-only policy")
	}
	state := State.NewGameState()
	state.Account.WorldID = "world-one"
	decision, err := policy.Evaluate(nil, Snapshot{State: state, Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("locked account scan decision = %+v", decision)
	}
}

func TestSharedStormScanWindowGeometryIgnoresPrivateSuppression(t *testing.T) {
	worlds := State.NewWorldMapStore()
	state := State.NewGameState()
	state.Account.WorldID = "world-one"
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: autoStormKingdomID, Focused: true}
	store := State.NewStoreWithWorldMap(state, worlds)
	if _, err := store.ApplyComponents(State.Components(State.ComponentWorldMap), func(state *State.GameState) ([]string, bool, error) {
		changed := state.SetMapObservation(State.MapObservation{
			KingdomID: autoStormKingdomID, X: 400, Y: 650, TypeID: State.MapTypeStormFort,
			StormIsleID: 7, ObservedAt: time.Now().UTC(),
		})
		return []string{"map"}, changed, nil
	}); err != nil {
		t.Fatal(err)
	}
	before := store.ReadOnlyView().SharedStormScanCoverage(autoStormKingdomID, time.Now().UTC())
	if before.WindowCount != 30 {
		t.Fatalf("one-sided edge observation expanded to %d windows, want 30", before.WindowCount)
	}
	if _, err := store.ApplyComponents(State.Components(State.ComponentStorm), func(state *State.GameState) ([]string, bool, error) {
		return []string{"storm"}, state.DeleteStormTarget("400:650"), nil
	}); err != nil {
		t.Fatal(err)
	}
	after := store.ReadOnlyView().SharedStormScanCoverage(autoStormKingdomID, time.Now().UTC())
	if before.Bounds != after.Bounds || before.WindowCount != after.WindowCount {
		t.Fatalf("private suppression changed shared scan geometry: before=%v after=%v", before, after)
	}
}
