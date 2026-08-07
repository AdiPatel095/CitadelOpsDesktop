package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
)

type worldIntelLeaderboardPageRequest struct {
	ListType      int64 `json:"listType"`
	LevelCategory int64 `json:"levelCategory"`
	SearchValue   int64 `json:"searchValue"`
}

func planWorldIntelLeaderboardPage(
	_ context.Context,
	_ Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request worldIntelLeaderboardPageRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.SearchValue < 1 || request.SearchValue > 250_000 {
		return Intent.Plan{}, fmt.Errorf("leaderboard searchValue must be between 1 and 250000")
	}
	switch request.ListType {
	case 2:
		if request.LevelCategory != 1 {
			return Intent.Plan{}, fmt.Errorf("weekly-loot leaderboard only supports level category 1")
		}
	case 6:
		if request.LevelCategory < 1 || request.LevelCategory > 6 {
			return Intent.Plan{}, fmt.Errorf("might leaderboard level category must be between 1 and 6")
		}
	default:
		return Intent.Plan{}, fmt.Errorf("World Intelligence only supports GGE leaderboard types 2 and 6")
	}
	payload, _ := json.Marshal(struct {
		ListType    int64  `json:"LT"`
		LeagueID    int64  `json:"LID"`
		SearchValue string `json:"SV"`
	}{request.ListType, request.LevelCategory, strconv.FormatInt(request.SearchValue, 10)})
	step := commandStep("Read one public GGE leaderboard page", "hgh", payload, "hgh")
	step.CaptureResponse = true
	step.TimeoutMillis = 20_000
	return Intent.Plan{
		Resources: []Intent.ResourceKey{{
			Scope: Intent.ResourceScopeSession, Capability: "protocol", ResourceKind: "leaderboard", ResourceID: "public-player",
		}},
		Summary: fmt.Sprintf("Read public GGE leaderboard %d category %d around rank %d", request.ListType, request.LevelCategory, request.SearchValue),
		Steps:   []Intent.Step{step},
	}, nil
}
