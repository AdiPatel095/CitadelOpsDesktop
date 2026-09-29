package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

// CIT-13 ABI/95: the rejected fortress is excluded (not merely sorted later),
// the other ready fortress is used, and the rejected one returns after Until.
func TestAutoFortressExcludesRejectedFortressUntilDeferralEnds(t *testing.T) {
	snapshot := fortressCooldownSnapshot(t)
	source := snapshot.State.Castles[10]
	rejection, recorded := State.RecordAttackTargetRejection(&snapshot.State, State.AttackTargetRejection{
		KingdomID: 3, TargetTypeID: State.MapTypeKingdomFortress, X: 100, Y: 100, Opcode: "abi", Code: 95,
		ObservedAt: snapshot.Now,
	})
	if !recorded {
		t.Fatal("rejection not recorded")
	}
	candidates, next, stats := autoFortressTargets(snapshot, []State.CastleState{source})
	if len(candidates) != 1 || candidates[0].Target.X != 101 || !next.Equal(rejection.Until) || len(stats[3].Deferred) != 1 {
		t.Fatalf("rejected fortress was selectable: candidates=%#v next=%v stats=%#v", candidates, next, stats[3])
	}
	decision, err := NewAutoFortressPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || !strings.Contains(decision.Details["rejection:"+rejection.Key()], "rejected by ABI 95 until") {
		t.Fatalf("deferral not explained: %#v err=%v", decision.Details, err)
	}

	// A fresh map zero for the rejected fortress does not release it.
	target := snapshot.State.Map[3]["100:100"]
	target.TowerCooldownRemaining = 0
	target.ObservedAt = snapshot.Now.Add(time.Minute)
	snapshot.State.Map[3]["100:100"] = target
	snapshot.Now = snapshot.Now.Add(time.Minute)
	if candidates, _, _ = autoFortressTargets(snapshot, []State.CastleState{source}); len(candidates) != 1 || candidates[0].Target.X != 101 {
		t.Fatalf("fresh map zero cleared the rejection: %#v", candidates)
	}

	snapshot.Now = rejection.Until
	if candidates, _, _ = autoFortressTargets(snapshot, []State.CastleState{source}); len(candidates) != 2 {
		t.Fatalf("rejected fortress did not return after its deferral: %#v", candidates)
	}
}

// H1: a fortress this player defeated without an observed victory report may
// still be in the personal lockout; it is tried after other ready fortresses.
func TestAutoFortressDeprioritizesSuspectedPersonalLockout(t *testing.T) {
	snapshot := fortressCooldownSnapshot(t)
	snapshot.State.Player.ID = 42
	near := snapshot.State.Map[3]["100:100"]
	near.FortressDefeaterPlayerID = 42
	snapshot.State.Map[3]["100:100"] = near
	source := snapshot.State.Castles[10]
	source.X, source.Y = 99, 100
	candidates, _, _ := autoFortressTargets(snapshot, []State.CastleState{source})
	if len(candidates) != 2 || candidates[0].Target.X != 101 || candidates[1].Target.X != 100 {
		t.Fatalf("suspected personal lockout was not tried last: %#v", candidates)
	}
}

// CIT-13 CRA/95: the rejected Storm fort is skipped, another fort stays
// eligible, and the rejected fort returns after its deferral.
func TestAutoStormSkipsRejectedTargetButKeepsOthers(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 100, 100
	state.Castles[storm.ID] = storm
	targets := map[string]State.MapObservation{
		"101:101": {KingdomID: 4, X: 101, Y: 101, TypeID: autoStormFortMapTypeID, StormIsleID: 7, StormVictoryCount: 1, ObservedAt: now},
		"105:105": {KingdomID: 4, X: 105, Y: 105, TypeID: autoStormFortMapTypeID, StormIsleID: 7, StormVictoryCount: 1, ObservedAt: now},
	}
	state.Map[4] = map[string]State.MapObservation{}
	for key, target := range targets {
		state.Map[4][key] = target
	}
	state.Storm.Map = State.StormMapState{SourceCastleID: storm.ID, LastAttemptAt: now, LastCompletedAt: now, Targets: targets}
	rejection, _ := State.RecordAttackTargetRejection(&state, State.AttackTargetRejection{
		KingdomID: 4, TargetTypeID: autoStormFortMapTypeID, X: 101, Y: 101, Opcode: "cra", Code: 95, ObservedAt: now,
	})
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}
	candidates, next := autoStormCombatOpportunities(snapshot, settings, storm)
	if len(candidates) != 1 || candidates[0].Observation.X != 105 || !next.Equal(rejection.Until) {
		t.Fatalf("Storm candidates = %#v next=%v", candidates, next)
	}
	snapshot.Now = rejection.Until
	if candidates, _ = autoStormCombatOpportunities(snapshot, settings, storm); len(candidates) != 2 {
		t.Fatalf("rejected Storm fort did not return: %#v", candidates)
	}
}

// CIT-13 SBP: counters read before the latest dispatched purchase (timeout,
// no reply) force a counter refresh; fresh counters that show the purchase
// advanced do not plan a duplicate.
func TestAutoFortressPurchaseRereadsCountersAfterUnresolvedDispatch(t *testing.T) {
	gameData := autoFortressTestGameData(t)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	main := State.CastleState{ID: 1, KingdomID: 0, SlotType: 1}
	gameState.Castles[1] = main
	gameState.Player.Currencies[37] = 1_000_000
	gameState.EventScores.ShopByPackage[3857] = State.EventShopRoute{EventID: 72, RemainingSec: 3600, ObservedAt: now}
	gameState.EventScores.ShopByPackage[3858] = State.EventShopRoute{EventID: 72, RemainingSec: 3600, ObservedAt: now}
	gameState.Inventory.ConstructionOffersCastleID = 1
	gameState.Inventory.ConstructionOffersObservedAt = now.Add(-10 * time.Second)
	gameState.Inventory.ConstructionOffers = map[State.PackageID]int64{}
	gameState.Inventory.LastPackagePurchaseDispatch = State.PackagePurchaseDispatch{PackageID: 3857, Amount: 50, SentAt: now.Add(-5 * time.Second)}
	settings := defaultAutoFortressSettings()
	settings.DirewolfPurchaseLimit = 5000
	snapshot := Snapshot{State: gameState, GameData: gameData, Now: now}

	decision, detail := evaluateAutoFortressPurchase(snapshot, settings, main, map[string]float64{})
	if detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "autoBuyer.package.history" {
		t.Fatalf("unresolved purchase did not force a counter refresh: %#v detail=%q", decision, detail)
	}

	gameState.Inventory.ConstructionOffersObservedAt = now
	gameState.Inventory.ConstructionOffers[3857] = 50 // the uncertain purchase did land
	snapshot.State = gameState
	decision, detail = evaluateAutoFortressPurchase(snapshot, settings, main, map[string]float64{})
	if decision != nil && decision.Request != nil && decision.Request.Name == "autoBuyer.package.purchase" {
		var arguments struct {
			PackageID int64 `json:"packageId"`
		}
		_ = json.Unmarshal(decision.Request.Arguments, &arguments)
		if arguments.PackageID == 3857 {
			t.Fatalf("landed purchase was planned again: %#v", decision.Request)
		}
	}
	if decision == nil && !strings.Contains(detail, "satisfied") {
		t.Fatalf("post-refresh decision detail=%q", detail)
	}
}
