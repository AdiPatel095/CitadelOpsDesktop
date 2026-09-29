package State

import (
	"testing"
	"time"
)

func TestAttackTargetRejectionDeferralDoublesCapsAndRestarts(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	record := func(at time.Time, personal bool, operationID string) AttackTargetRejection {
		t.Helper()
		rejection, _ := RecordAttackTargetRejection(&state, AttackTargetRejection{
			KingdomID: 1, TargetTypeID: MapTypeKingdomFortress, X: 10, Y: 20, Opcode: "ABI", Code: 95,
			OperationID: operationID, ObservedAt: at, Personal: personal,
		})
		return rejection
	}
	want := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 16 * time.Hour, 24 * time.Hour, 24 * time.Hour}
	at := now
	for index, deferral := range want {
		rejection := record(at, false, "op-"+string(rune('a'+index)))
		if rejection.Count != index+1 || rejection.Until.Sub(rejection.ObservedAt) != deferral || rejection.Opcode != "abi" {
			t.Fatalf("rejection %d = %#v, want deferral %s", index+1, rejection, deferral)
		}
		at = rejection.Until
	}
	if duplicate, recorded := RecordAttackTargetRejection(&state, AttackTargetRejection{
		KingdomID: 1, TargetTypeID: MapTypeKingdomFortress, X: 10, Y: 20, Opcode: "abi", Code: 95,
		OperationID: "op-g", ObservedAt: at,
	}); recorded || duplicate.Count != len(want) {
		t.Fatalf("same operation counted twice: %#v recorded=%t", duplicate, recorded)
	}
	if !ClearAttackTargetRejection(&state, 1, MapTypeKingdomFortress, 10, 20) {
		t.Fatal("clear failed")
	}
	if restarted := record(at, true, "op-personal"); restarted.Count != 1 || restarted.Until.Sub(restarted.ObservedAt) != AttackTargetRejectionPersonalBase {
		t.Fatalf("cleared target did not restart with the personal base: %#v", restarted)
	}
	later := at.Add(AttackTargetRejectionPersonalBase + AttackTargetRejectionPersonalCap + time.Minute)
	if restarted := record(later, true, "op-late"); restarted.Count != 1 {
		t.Fatalf("rejection after retention counted as consecutive: %#v", restarted)
	}
	personal := at
	state = NewGameState()
	for index := 0; index < 6; index++ {
		rejection := record(personal, true, "p"+string(rune('a'+index)))
		personal = rejection.Until
		if index == 5 && rejection.Until.Sub(rejection.ObservedAt) != AttackTargetRejectionPersonalCap {
			t.Fatalf("personal deferral not capped: %#v", rejection)
		}
	}
}

func TestAttackTargetRejectionsAreBoundedAndOnlyActiveBeforeUntil(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	for index := 0; index < AttackTargetRejectionLimit+20; index++ {
		RecordAttackTargetRejection(&state, AttackTargetRejection{
			KingdomID: 4, TargetTypeID: MapTypeStormFort, X: index, Y: index, Opcode: "cra", Code: 95,
			ObservedAt: now.Add(time.Duration(index) * time.Second),
		})
	}
	if len(state.AttackAnalytics.RejectedTargets) != AttackTargetRejectionLimit {
		t.Fatalf("registry size = %d", len(state.AttackAnalytics.RejectedTargets))
	}
	if _, found := AttackTargetRejectedAt(state, 4, MapTypeStormFort, 0, 0, now); found {
		t.Fatal("oldest rejection survived the bound")
	}
	last := AttackTargetRejectionLimit + 19
	rejection, found := AttackTargetRejectedAt(state, 4, MapTypeStormFort, last, last, now.Add(time.Hour))
	if !found {
		t.Fatal("newest rejection missing")
	}
	if _, found := AttackTargetRejectedAt(state, 4, MapTypeStormFort, last, last, rejection.Until); found {
		t.Fatal("rejection active at its Until")
	}
	if _, found := AttackTargetRejectedAt(state, 4, MapTypeStormIsland, last, last, now.Add(time.Hour)); found {
		t.Fatal("rejection matched a different target type")
	}
}

func TestRejectedTargetsAndPackageDispatchPersist(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := NewStore(NewGameState())
	event, err := store.ApplyComponents(Components(ComponentAttackAnalytics, ComponentInventory), func(state *GameState) ([]string, bool, error) {
		RecordAttackTargetRejection(state, AttackTargetRejection{
			KingdomID: 1, TargetTypeID: MapTypeKingdomFortress, X: 10, Y: 20, Opcode: "abi", Code: 95, ObservedAt: now,
		})
		state.MarkInventoryPackagePurchaseDispatched(PackagePurchaseDispatch{PackageID: 3857, Amount: 50, SentAt: now})
		return []string{"attack-analytics", "inventory"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, event, Components(event.Components...)); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AttackAnalytics.RejectedTargets) != 1 || loaded.Inventory.LastPackagePurchaseDispatch.PackageID != 3857 {
		t.Fatalf("persisted rejections=%#v dispatch=%#v", loaded.AttackAnalytics.RejectedTargets, loaded.Inventory.LastPackagePurchaseDispatch)
	}
	projected := store.ReadOnlyView().clientStateProjection()
	if len(projected.AttackAnalytics.RejectedTargets) != 0 {
		t.Fatal("rejected-target registry leaked into the client projection")
	}
}
