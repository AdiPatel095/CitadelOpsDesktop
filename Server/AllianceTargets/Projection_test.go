package AllianceTargets

import (
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestSpyAvailabilityUsesOfficialBuildingCapacity(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[{"wodID":145,"level":"1","spySize":"2"}],"units":[]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 20,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 145},
		},
	}
	gameState.Movements[50] = State.MovementState{
		ID: 50, TypeID: 3, SourceCastleID: 100, SpyCount: 1, Units: map[State.UnitID]int64{},
	}

	spies := spyAvailability(gameState, gameData)
	if spies.Capacity != 2 || spies.Active != 1 || spies.Available != 1 {
		t.Fatalf("spy availability = %+v", spies)
	}
	if len(spies.Taverns) != 1 || spies.Taverns[0].Level != 1 || spies.Taverns[0].Capacity != 2 {
		t.Fatalf("taverns = %+v", spies.Taverns)
	}
}

func TestBuildLiveTargetsUsesDirectoryBirdAndHoldingState(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 10}
	alliance := State.AllianceState{
		ID: 9, Name: "Targets", ObservedAt: time.Now().Add(-time.Second),
		Members:  []State.AllianceMember{{PlayerID: 7, Name: "Player", Might: 123, ReturnProtectionSec: 120}},
		Holdings: []State.AllianceHolding{{CastleID: 77, PlayerID: 7, KingdomID: 0, X: 13, Y: 14, SlotType: 1}},
	}

	rows := buildLiveTargets(gameState, alliance, trackerAllianceDetail{})
	if len(rows) != 1 {
		t.Fatalf("targets = %+v", rows)
	}
	if !rows[0].UnderBird || rows[0].RPTSeconds < 118 || rows[0].RPTSeconds > 120 {
		t.Fatalf("bird state = %+v", rows[0])
	}
	if rows[0].Distance != 5 || rows[0].TargetCastle.CastleID != 77 || rows[0].ClosestOwnCastle.CastleID != 100 {
		t.Fatalf("target projection = %+v", rows[0])
	}
}
