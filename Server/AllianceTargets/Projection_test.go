package AllianceTargets

import (
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Reports"
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
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	action := spyAction(gameState, spies)
	if !action.CanLaunch || action.Available != 1 || action.SourceCastleID != 100 {
		t.Fatalf("spy action = %+v", action)
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

func TestQueryTargetsFiltersSortsAndPaginates(t *testing.T) {
	rows := make([]Target, 0, 45)
	for index := 1; index <= 45; index++ {
		rows = append(rows, Target{
			PlayerID: int64(index), Name: "Player", Might: int64(index), UnderBird: index%2 == 0,
			RPTSeconds:       index * 10,
			TargetCastle:     Castle{Name: "Castle", TypeName: "Outpost", X: 100 + index, Y: 200 + index},
			ClosestOwnCastle: Castle{Name: "Main", X: 10, Y: 20}, Distance: float64(index),
		})
	}

	pageRows, total, page, pageCount := queryTargets(rows, Query{
		Status: "attackable", Sort: "might", Direction: "desc", Page: 2,
	})
	if total != 23 || page != 2 || pageCount != 2 || len(pageRows) != 3 {
		t.Fatalf("page = total %d page %d count %d rows %d", total, page, pageCount, len(pageRows))
	}
	if pageRows[0].Might != 5 || pageRows[2].Might != 1 {
		t.Fatalf("sorted rows = %+v", pageRows)
	}

	searchRows, total, page, pageCount := queryTargets(rows, Query{Search: "101:201", Page: 8})
	if total != 1 || page != 1 || pageCount != 1 || len(searchRows) != 1 || searchRows[0].Might != 1 {
		t.Fatalf("search page = total %d page %d count %d rows %+v", total, page, pageCount, searchRows)
	}
}

func TestEnrichTargetIntelligenceUsesLatestOwnedSpyReport(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 9
	targets := []Target{{
		PlayerID:     77,
		TargetCastle: Castle{CastleID: 500, X: 20, Y: 30, TypeID: 4},
	}}
	reports := []Reports.SpyReport{
		{
			MID: 1, CapturedAtUnixMillis: 1_000, Status: "partial", TotalTroops: 100,
			Source: Reports.Player{ID: 9}, Castle: Reports.Castle{ID: 500, Name: "Old name", X: 20, Y: 30},
		},
		{
			MID: 2, CapturedAtUnixMillis: 2_000, Status: "success", TotalTroops: 250,
			Source: Reports.Player{ID: 9}, Castle: Reports.Castle{ID: 500, Name: "Latest name", X: 20, Y: 30},
		},
		{
			MID: 3, CapturedAtUnixMillis: 3_000, Status: "success", TotalTroops: 999,
			Source: Reports.Player{ID: 10}, Castle: Reports.Castle{ID: 500, Name: "Other account", X: 20, Y: 30},
		},
	}

	enrichTargetIntelligence(gameState, reports, targets)
	if targets[0].TargetCastle.Name != "Latest name" || targets[0].TargetCastle.TypeName != "Outpost" {
		t.Fatalf("target name/type = %+v", targets[0].TargetCastle)
	}
	if targets[0].SpyReport == nil || targets[0].SpyReport.CapturedAtUnixMillis != 2_000 ||
		targets[0].SpyReport.TotalTroops != 250 || targets[0].SpyReport.Status != "success" {
		t.Fatalf("spy summary = %+v", targets[0].SpyReport)
	}
}

func TestEnrichTargetIntelligenceDoesNotReuseCoordinateForAnotherCastleID(t *testing.T) {
	gameState := State.NewGameState()
	targets := []Target{{TargetCastle: Castle{CastleID: 500, X: 20, Y: 30}}}
	reports := []Reports.SpyReport{{
		MID: 1, CapturedAtUnixMillis: 1_000, Status: "success", TotalTroops: 100,
		Castle: Reports.Castle{ID: 600, X: 20, Y: 30},
	}}

	enrichTargetIntelligence(gameState, reports, targets)
	if targets[0].SpyReport != nil {
		t.Fatalf("mismatched castle report was attached: %+v", targets[0].SpyReport)
	}
}
