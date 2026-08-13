package Automation

import (
	"context"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
)

const dailyAttackRefreshInterval = 15 * time.Minute

type DailyAttackRefreshPolicy struct{}

func NewDailyAttackRefreshPolicy() *DailyAttackRefreshPolicy { return &DailyAttackRefreshPolicy{} }

func (*DailyAttackRefreshPolicy) ID() string         { return "dailyAttackRefresh" }
func (*DailyAttackRefreshPolicy) EnabledKey() string { return "" }
func (*DailyAttackRefreshPolicy) CorePolicy()        {}

func (*DailyAttackRefreshPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	attacks := snapshot.State.DailyAttacks
	metrics := map[string]float64{
		"dailyAttackCount":           float64(attacks.Count),
		"serverDailyAttackThreshold": float64(attacks.ServerThreshold),
	}
	if attacks.ServerThreshold <= 0 {
		return Decision{
			Status: "waiting", Detail: "Waiting for the server daily attack threshold",
			NextCheckAt: snapshot.Now.Add(dailyAttackRefreshInterval), Metrics: metrics,
		}, nil
	}
	if attacks.Count < attacks.ServerThreshold {
		return Decision{
			Status: "idle",
			Detail: fmt.Sprintf(
				"Daily attack count is below the server maximum: %d / %d",
				attacks.Count, attacks.ServerThreshold,
			),
			NextCheckAt: snapshot.Now.Add(dailyAttackRefreshInterval), Metrics: metrics,
		}, nil
	}

	dueAt := attacks.ObservedAt.Add(dailyAttackRefreshInterval)
	if !attacks.ObservedAt.IsZero() && snapshot.Now.Before(dueAt) {
		return Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Daily attack maximum reached: %d / %d; waiting to refresh the server count",
				attacks.Count, attacks.ServerThreshold,
			),
			NextCheckAt: dueAt, Metrics: metrics,
		}, nil
	}

	return Decision{
		Status: "refreshing",
		Detail: fmt.Sprintf(
			"Refreshing daily attack count after reaching the server maximum: %d / %d",
			attacks.Count, attacks.ServerThreshold,
		),
		NextCheckAt: snapshot.Now.Add(dailyAttackRefreshInterval), Metrics: metrics,
		Request: &Intent.Request{Name: "daily_attacks.refresh"},
	}, nil
}
