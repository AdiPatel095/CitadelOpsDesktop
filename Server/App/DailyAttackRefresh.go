package App

import (
	"context"
	"encoding/json"

	"CitadelDesktop/Server/Intent"
)

func planDailyAttackRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	step := commandStep("Refresh daily attack count", "gai", json.RawMessage(`{}`), "gai")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"game:daily-attacks"},
		Summary: "Refresh daily attack count",
		Steps:   []Intent.Step{step},
	}, nil
}
