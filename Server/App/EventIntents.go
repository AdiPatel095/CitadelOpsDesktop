package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const allianceNomadRankingListType = 47

type eventRankingRequest struct {
	EventID int64 `json:"eventId"`
}

func planEventRankingRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request eventRankingRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	score, found := input.State.LookupScalableEventScore(request.EventID)
	leagueID, listType, leaderboardFound := publicEventRanking(score)
	if !found || request.EventID <= 0 || !leaderboardFound {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d does not expose a supported public GGE leaderboard", request.EventID), Localization.New("server.app.event_p_does_not.5369b5fc", "event {p0} does not expose a supported public GGE leaderboard", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	payload, _ := json.Marshal(struct {
		LeagueID    int64  `json:"LID"`
		ListType    int64  `json:"LT"`
		SearchValue string `json:"SV"`
	}{leagueID, listType, "-1"})
	return Intent.Plan{
		Claims: []string{"event:" + strconv.FormatInt(request.EventID, 10)}, Summary: fmt.Sprintf("Refresh event %d GGE alliance leaderboard", request.EventID), SummaryDescriptor: Localization.New("server.app.refresh_event_p_gge.4695bf60", "Refresh event {p0} GGE alliance leaderboard", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}),
		Steps: []Intent.Step{
			{Name: "Clear stale event ranking rows", NameDescriptor: Localization.New("server.app.clear_stale_event_ranking.fd7c82ed", "Clear stale event ranking rows", nil), Action: "event.ranking.begin", ActionArguments: arguments},
			commandStep("Request live GGE event alliance ranking", "hgh", payload, "hgh", Localization.New("server.app.request_live_gge_event.0eb078bf", "Request live GGE event alliance ranking", nil)),
		},
	}, nil
}

func (application *Application) beginEventRankingRefresh(_ context.Context, arguments json.RawMessage) error {
	var request eventRankingRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentEventScores), func(gameState *State.GameState) ([]string, bool, error) {
		score, found := gameState.LookupScalableEventScore(request.EventID)
		leagueID, listType, rankingFound := publicEventRanking(score)
		if !found || !rankingFound {
			return nil, false, Localization.WithError(fmt.Errorf("event %d leaderboard is unavailable", request.EventID), Localization.New("server.app.event_p_leaderboard_is.9f7bb908", "event {p0} leaderboard is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
		}
		gameState.SetEventRanking(request.EventID, State.EventRankingState{
			EventID: request.EventID, Scope: "alliance", LeagueID: leagueID, ListType: listType,
			OwnAllianceID: gameState.Player.AllianceID,
			Entries:       []State.EventRankingEntry{}, Pending: true, ObservedAt: time.Now().UTC(),
		})
		return []string{"event-scores"}, true, nil
	})
	return err
}

func publicEventRanking(score State.ScalableEventScore) (int64, int64, bool) {
	if score.EventID != 72 || score.AllianceLeagueID <= 0 {
		return 0, 0, false
	}
	return score.AllianceLeagueID, allianceNomadRankingListType, true
}
