package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type dailyAttackLimitGuardRequest struct {
	Limit int64 `json:"limit"`
}

func dailyAttackLimitStatus(gameState State.GameState, limit int64) (string, bool, error) {
	if limit == 0 {
		return "", false, nil
	}
	if limit < 0 {
		return "", false, Localization.WithError(fmt.Errorf("dailyAttackLimit cannot be negative"), Localization.New("server.app.dailyattacklimit_cannot_be_negative.c9a956ed", "dailyAttackLimit cannot be negative", nil))
	}
	attacks := gameState.DailyAttacks
	if attacks.ObservedAt.IsZero() {
		return "Waiting for the server daily attack count before queuing another attack", true, nil
	}
	if attacks.Count >= limit {
		return fmt.Sprintf(
			"Daily attack limit reached: %d / %d; normal attacks resume when the server count resets",
			attacks.Count, limit,
		), true, nil
	}
	return "", false, nil
}

func dailyAttackLimitPlan(gameState State.GameState, limit int64) (Intent.Plan, bool, error) {
	detail, blocked, err := dailyAttackLimitStatus(gameState, limit)
	var detailLocalizationMessage *Localization.Message = nil
	if err != nil || !blocked {
		return Intent.Plan{}, false, err
	}
	return Intent.Plan{Summary: detail, SummaryDescriptor: Localization.Clone(detailLocalizationMessage)}, true, nil
}

func guardDailyAttackLimitAtDispatch(gameState State.GameState, limit int64) error {
	detail, blocked, err := dailyAttackLimitStatus(gameState, limit)
	if err != nil {
		return err
	}
	if blocked {
		return Localization.WithError(fmt.Errorf("%w: %s", Intent.ErrPlanStale, detail), Localization.New("server.app.intent_plan_became_stale.9e9732a4", "intent plan became stale before dispatch: {p1}", Localization.Params{"p1": fmt.Sprintf("%s", detail)}))
	}
	return nil
}

func appendDailyAttackLimitGuard(steps []Intent.Step, limit int64) []Intent.Step {
	if limit <= 0 {
		return steps
	}
	arguments, _ := json.Marshal(dailyAttackLimitGuardRequest{Limit: limit})
	return append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Verify server daily attack limit", NameDescriptor: Localization.New("server.app.verify_server_daily_attack.1eef8374", "Verify server daily attack limit", nil), Action: "attack.daily_limit.guard", ActionArguments: arguments,
	}))
}

func (application *Application) guardDailyAttackLimit(_ context.Context, arguments json.RawMessage) error {
	var request dailyAttackLimitGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("game state is unavailable"), Localization.New("server.app.game_state_is_unavailable.cfae30c6", "game state is unavailable", nil))
	}
	detail, blocked, err := dailyAttackLimitStatus(application.State.ReadOnlyView(), request.Limit)
	if err != nil {
		return err
	}
	if blocked {
		return Localization.WithError(fmt.Errorf("%s", detail), Localization.New("server.app.p.8af35f19", "{p0}", Localization.Params{"p0": fmt.Sprintf("%s", detail)}))
	}
	return nil
}
