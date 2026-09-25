package Automation

import (
	"CitadelDesktop/Server/Localization"
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
			Status: "waiting", Detail: "Waiting for the server daily attack threshold", DetailDescriptor: Localization.New("server.automation.waiting_for_the_server.abedd1c3", "Waiting for the server daily attack threshold", nil),
			NextCheckAt: snapshot.Now.Add(dailyAttackRefreshInterval), Metrics: metrics,
		}, nil
	}
	if attacks.Count < attacks.ServerThreshold {
		return Decision{
			Status: "idle",
			Detail: fmt.Sprintf(
				"Daily attack count is below the server maximum: %d / %d",
				attacks.Count, attacks.ServerThreshold,
			), DetailDescriptor: Localization.New("server.automation.daily_attack_count_is.4d83c848", "Daily attack count is below the server maximum: {p0} / {p1}", Localization.Params{"p0": attacks.Count, "p1": attacks.ServerThreshold}),
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
			), DetailDescriptor: Localization.New("server.automation.daily_attack_maximum_reached.4583fede", "Daily attack maximum reached: {p0} / {p1}; waiting to refresh the server count", Localization.Params{"p0": attacks.Count, "p1": attacks.ServerThreshold}),
			NextCheckAt: dueAt, Metrics: metrics,
		}, nil
	}

	return Decision{
		Status: "refreshing",
		Detail: fmt.Sprintf(
			"Refreshing daily attack count after reaching the server maximum: %d / %d",
			attacks.Count, attacks.ServerThreshold,
		), DetailDescriptor: Localization.New("server.automation.refreshing_daily_attack_count.58b506ff", "Refreshing daily attack count after reaching the server maximum: {p0} / {p1}", Localization.Params{"p0": attacks.Count, "p1": attacks.ServerThreshold}),
		NextCheckAt: snapshot.Now.Add(dailyAttackRefreshInterval), Metrics: metrics,
		Request: &Intent.Request{Name: "daily_attacks.refresh"},
	}, nil
}
