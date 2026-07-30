package App

import (
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
		return "", false, fmt.Errorf("dailyAttackLimit cannot be negative")
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
	if err != nil || !blocked {
		return Intent.Plan{}, false, err
	}
	return Intent.Plan{Summary: detail}, true, nil
}

func appendDailyAttackLimitGuard(steps []Intent.Step, limit int64) []Intent.Step {
	if limit <= 0 {
		return steps
	}
	arguments, _ := json.Marshal(dailyAttackLimitGuardRequest{Limit: limit})
	return append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Verify server daily attack limit", Action: "attack.daily_limit.guard", ActionArguments: arguments,
	}))
}

func (application *Application) guardDailyAttackLimit(_ context.Context, arguments json.RawMessage) error {
	var request dailyAttackLimitGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("game state is unavailable")
	}
	detail, blocked, err := dailyAttackLimitStatus(application.State.Snapshot(), request.Limit)
	if err != nil {
		return err
	}
	if blocked {
		return fmt.Errorf("%s", detail)
	}
	return nil
}
