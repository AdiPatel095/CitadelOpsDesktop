package AllianceTargets

import (
	"context"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/WorldIntel"
)

type fakeIntelligenceProvider struct {
	ranking WorldIntel.RankingResponse
	profile WorldIntel.AllianceProfile
}

func (provider fakeIntelligenceProvider) Rankings(
	context.Context, string, string, string, int,
) (WorldIntel.RankingResponse, error) {
	return provider.ranking, nil
}

func (provider fakeIntelligenceProvider) Alliance(
	context.Context, string, int64, int,
) (WorldIntel.AllianceProfile, error) {
	return provider.profile, nil
}

func TestServiceLoadsWorldIntelligenceAllianceTargets(t *testing.T) {
	now := time.Now().UTC()
	provider := fakeIntelligenceProvider{
		ranking: WorldIntel.RankingResponse{Entries: make([]WorldIntel.RankingEntry, 50)},
		profile: WorldIntel.AllianceProfile{
			Current:  WorldIntel.AllianceObservation{AllianceID: 101, Name: "Alliance 101", ObservedAt: now},
			Members:  []WorldIntel.PlayerObservation{{PlayerID: 7, Name: "Target", Might: 9000, ObservedAt: now}},
			Holdings: []WorldIntel.HoldingObservation{{AllianceID: 101, PlayerID: 7, CastleID: 70, KingdomID: 0, X: 13, Y: 14, SlotType: 1, ObservedAt: now}},
		},
	}
	for index := range provider.ranking.Entries {
		id := int64(index + 101)
		provider.ranking.Entries[index] = WorldIntel.RankingEntry{
			Rank: index + 1, ID: id, Name: fmt.Sprintf("Alliance %d", id), MemberCount: 10,
		}
	}
	service := NewService(provider)
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "wss://ep-live-us1-game.example.test/socket"
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 10}

	view, err := service.View(t.Context(), gameState, nil, "", "101", false, Query{IncludeAlliances: true})
	if err != nil {
		t.Fatal(err)
	}
	if view.Server != "ep-live-us1-game.example.test" || len(view.Alliances) != 50 ||
		view.SelectedAlliance == nil || view.SelectedAlliance.AllianceID != 101 {
		t.Fatalf("alliance view = %+v", view)
	}
	if len(view.Targets) != 1 || view.TotalTargets != 1 || view.Page != 1 || view.PageCount != 1 ||
		view.Targets[0].Name != "Target" || view.Targets[0].Distance != 5 {
		t.Fatalf("target view = %+v", view.Targets)
	}
}

func TestResolveWorldIDUsesOverrideThenBoundAccount(t *testing.T) {
	state := State.NewGameState()
	state.Account.WorldID = "wss://world-a.example/socket"
	if world := resolveWorldID(state, "https://WORLD-B.EXAMPLE/path"); world != "world-b.example" {
		t.Fatalf("override world = %q", world)
	}
	if world := resolveWorldID(state, ""); world != "world-a.example" {
		t.Fatalf("bound world = %q", world)
	}
}
