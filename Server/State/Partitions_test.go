package State

import (
	"testing"
	"time"
)

func TestPartitionVersionsUseImmutableCommonCapabilitySlots(t *testing.T) {
	state := NewGameState()
	state.Account = AccountBindingState{WorldID: "world-1", PlayerID: 42}
	key := AccountPartition(state, CapabilityMovement)
	first, changed := advancePartitionVersions(nil, []PartitionKey{key}, 1, time.Unix(1, 0).UTC())
	if len(changed) != 1 || changed[0].Version != 1 {
		t.Fatalf("first change = %#v", changed)
	}
	second, changed := advancePartitionVersions(first, []PartitionKey{key}, 2, time.Unix(2, 0).UTC())
	if len(changed) != 1 || changed[0].Version != 2 {
		t.Fatalf("second change = %#v", changed)
	}
	if got := (PartitionVersions{snapshot: first}).Version(key); got != 1 {
		t.Fatalf("immutable first version = %d, want 1", got)
	}
	if got := (PartitionVersions{snapshot: second}).Version(key); got != 2 {
		t.Fatalf("second version = %d, want 2", got)
	}
}

func TestPartitionVersionsRetainIndependentCastleFallbackKeys(t *testing.T) {
	state := NewGameState()
	state.Account = AccountBindingState{WorldID: "world-1", PlayerID: 42}
	left := CastlePartition(state, CapabilityConstruction, 10)
	right := CastlePartition(state, CapabilityConstruction, 20)
	first, _ := advancePartitionVersions(nil, []PartitionKey{left, right}, 1, time.Unix(1, 0).UTC())
	second, _ := advancePartitionVersions(first, []PartitionKey{left}, 2, time.Unix(2, 0).UTC())
	versions := PartitionVersions{snapshot: second}
	if got := versions.Version(left); got != 2 {
		t.Fatalf("left version = %d, want 2", got)
	}
	if got := versions.Version(right); got != 1 {
		t.Fatalf("right version = %d, want 1", got)
	}
	if got := (PartitionVersions{snapshot: first}).Version(left); got != 1 {
		t.Fatalf("immutable fallback version = %d, want 1", got)
	}
	if listed := versions.List(); len(listed) != 2 {
		t.Fatalf("listed fallback partitions = %d, want 2", len(listed))
	}
}

func TestSessionPartitionIdentityDoesNotGrowWithGeneration(t *testing.T) {
	state := NewGameState()
	state.Session.Generation = 1
	firstKey := SessionPartition(state, CapabilitySession)
	first, _ := advancePartitionVersions(nil, []PartitionKey{firstKey}, 1, time.Unix(1, 0).UTC())
	state.Session.Generation = 99
	state.Session.ConnectionGeneration = 12
	secondKey := SessionPartition(state, CapabilitySession)
	second, _ := advancePartitionVersions(first, []PartitionKey{secondKey}, 2, time.Unix(2, 0).UTC())
	versions := PartitionVersions{snapshot: second}
	if got := versions.Version(firstKey); got != 2 {
		t.Fatalf("original session key version = %d, want 2", got)
	}
	if got := versions.Version(secondKey); got != 2 {
		t.Fatalf("new session key version = %d, want 2", got)
	}
	if listed := versions.List(); len(listed) != 1 {
		t.Fatalf("session partition count = %d, want 1", len(listed))
	}
}
