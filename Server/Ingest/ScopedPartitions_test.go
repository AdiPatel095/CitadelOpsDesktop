package Ingest

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestScopedPartitionsKeepFocusedCastleDomainsIndependent(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://example.invalid"
	gameState.Player.ID = 7
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	gameState.Castles[12] = State.CastleState{ID: 12, KingdomID: 1}
	keys := scopedPartitionsForFrame(Protocol.Frame{Opcode: "jaa"}, gameState, []string{
		"castles", "resources", "building-layout", "building-construction", "construction-items",
	})
	actual := canonicalPartitionSet(keys)
	wanted := []State.PartitionKey{
		State.SessionPartition(gameState, State.CapabilitySessionContext),
		State.CastlePartition(gameState, State.CapabilityCastleDirectory, 11),
		State.CastlePartition(gameState, State.CapabilityEconomy, 11),
		State.CastlePartition(gameState, State.CapabilityBuildings, 11),
		State.CastlePartition(gameState, State.CapabilityBuildingQueue, 11),
		State.CastlePartition(gameState, State.CapabilityConstructionItems, 11),
	}
	for _, key := range wanted {
		if _, found := actual[key.Canonical()]; !found {
			t.Errorf("missing partition %s", key.Canonical())
		}
	}
	unrelated := State.CastlePartition(gameState, State.CapabilityBuildings, 12).Canonical()
	if _, found := actual[unrelated]; found {
		t.Fatalf("focused update included unrelated partition %s", unrelated)
	}
}

func TestScopedPartitionsUseMapPayloadKingdom(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{}
	gameState.Map[1] = map[string]State.MapObservation{}
	keys := scopedPartitionsForFrame(Protocol.Frame{
		Opcode: "gaa", Payload: json.RawMessage(`{"KID":0,"AI":[]}`),
	}, gameState, []string{"map", "tower-cooldowns"})
	actual := canonicalPartitionSet(keys)
	for _, capability := range []string{State.CapabilityWorldMap, State.CapabilityEvents} {
		key := State.KingdomPartition(gameState, capability, 0).Canonical()
		if _, found := actual[key]; !found {
			t.Errorf("missing kingdom-zero partition %s", key)
		}
	}
	if unrelated := State.KingdomPartition(gameState, State.CapabilityWorldMap, 1).Canonical(); actual[unrelated].Capability != "" {
		t.Fatalf("map update included unrelated kingdom partition %s", unrelated)
	}
}

func TestSuccessfulGAAIncludesSessionContextPartition(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://example.invalid"
	gameState.Player.ID = 7
	gameState.Castles[11] = State.CastleState{ID: 11, KingdomID: 1, Focused: true}
	code := 0
	keys := scopedPartitionsForFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &code,
		Payload: json.RawMessage(`{"KID":1,"AI":[]}`),
	}, gameState, []string{"map"})
	actual := canonicalPartitionSet(keys)
	contextKey := State.SessionPartition(gameState, State.CapabilitySessionContext).Canonical()
	if _, found := actual[contextKey]; !found {
		t.Fatalf("successful GAA omitted session-context partition %s", contextKey)
	}
}

func canonicalPartitionSet(keys []State.PartitionKey) map[string]State.PartitionKey {
	set := make(map[string]State.PartitionKey, len(keys))
	for _, key := range keys {
		set[key.Canonical()] = key
	}
	return set
}
