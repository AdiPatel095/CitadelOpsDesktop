package State

import (
	"CitadelDesktop/Server/Localization"
	"testing"
	"time"
)

func TestArrivalAndProtectionDescriptorsAreSnapshotIsolated(t *testing.T) {
	state := NewGameState()
	arrival := time.Date(2026, 9, 20, 1, 2, 3, 456, time.UTC)
	state.Khan.SafetyErrorDescriptor = ArrivalOrderDescriptor(2, arrival, 1, arrival.Add(time.Second))
	state.Khan.Protection.ReasonDescriptor = Localization.New("reason", "Threshold {threshold}", Localization.Params{"threshold": 1000})
	state.NomadCamps.RBCTest = &NomadRBCTestState{SafetyErrorDescriptor: ArrivalOrderDescriptor(4, arrival, 3, arrival.Add(time.Second))}
	store := NewStore(state)
	snapshot := store.Snapshot()
	snapshot.Khan.SafetyErrorDescriptor.Params["commander"] = "changed"
	snapshot.Khan.Protection.ReasonDescriptor.Params["threshold"] = 0
	snapshot.NomadCamps.RBCTest.SafetyErrorDescriptor.Params["commander"] = "changed"
	current := store.ReadOnlyView()
	if current.Khan.SafetyErrorDescriptor.Params["commander"] != "2" || current.Khan.Protection.ReasonDescriptor.Params["threshold"] != 1000 || current.NomadCamps.RBCTest.SafetyErrorDescriptor.Params["commander"] != "4" {
		t.Fatal("snapshot mutates presentation source")
	}
	if current.Khan.SafetyErrorDescriptor.Params["arrival"] != arrival.Format(time.RFC3339Nano) {
		t.Fatal("timestamp precision changed")
	}
}
