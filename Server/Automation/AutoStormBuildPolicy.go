package Automation

import (
	"context"
	"fmt"
	"time"

	"CitadelDesktop/Server/Buildings"
)

const autoStormTransportOwner = "autoStorm"

type AutoStormBuildPolicy struct{}

func NewAutoStormBuildPolicy() *AutoStormBuildPolicy { return &AutoStormBuildPolicy{} }

func (*AutoStormBuildPolicy) ID() string         { return "autoStormBuild" }
func (*AutoStormBuildPolicy) EnabledKey() string { return "auto_storm" }
func (*AutoStormBuildPolicy) ActorID() string    { return "autoStorm" }
func (*AutoStormBuildPolicy) ScheduleKey() string {
	return "autoStorm"
}

func (*AutoStormBuildPolicy) WakeDomains() []string {
	return []string{"buildings", "castles", "currencies", "inventory", "kingdom-transport", "resources"}
}

func (*AutoStormBuildPolicy) WakeSections() []string {
	return []string{autoStormSection, Buildings.StormBlueprintConfigurationSection, "decorations.presets"}
}

func (*AutoStormBuildPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoStormSettings()
	if !decodeSection(snapshot.Configuration, autoStormSection, &settings) {
		return autoStormBuildWaiting(snapshot.Now, "Auto Storm settings have not been saved", nil), nil
	}
	normalizeAutoStormSettings(&settings)
	if settings.Version != 1 {
		return autoStormBuildWaiting(snapshot.Now, fmt.Sprintf("Unsupported Auto Storm settings version %d", settings.Version), nil), nil
	}
	if snapshot.GameData == nil {
		return autoStormBuildWaiting(snapshot.Now, "Official game data is unavailable", nil), nil
	}
	if err := autoStormApplyActiveBlueprint(snapshot, &settings); err != nil {
		return autoStormBuildWaiting(snapshot.Now, err.Error(), nil), nil
	}
	if settings.Target == nil && !settings.Harbor.Enabled {
		return Decision{
			Status: "complete", Detail: "No Storm castle blueprint or Harbor goal is active",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
		}, nil
	}
	castle, found := autoStormCastle(snapshot.State, settings.Target)
	if !found {
		return autoStormBuildWaiting(snapshot.Now, "No unlocked Storm castle is present", nil), nil
	}
	metrics := map[string]float64{"castleId": float64(castle.ID)}
	if decision, actionable := ownedKingdomTransportDecision(
		autoStormTransportOwner,
		"Auto Storm build lane",
		settings.Build.AllowTimeSkips,
		autoStormAllowedTimeSkips(settings),
		settings.Build.TimeSkipReserve,
		snapshot,
	); actionable {
		decision.Metrics = metrics
		return decision, nil
	}
	decision, complete, detail, err := evaluateAutoStormBuild(snapshot, settings, castle, metrics)
	if err != nil {
		return Decision{}, err
	}
	if decision != nil {
		return *decision, nil
	}
	status := "waiting"
	if complete {
		status = "complete"
	}
	if detail == "" {
		detail = "Storm build lane is waiting for its next state change"
	}
	return Decision{
		Status: status, Detail: detail, Metrics: metrics,
		NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
	}, nil
}

func autoStormApplyActiveBlueprint(snapshot Snapshot, settings *autoStormSettings) error {
	if settings.Target != nil {
		normalized := Buildings.NormalizeTargetCapture(*settings.Target, nil)
		settings.Target = &normalized
	}
	raw := snapshot.Configuration.Sections[Buildings.StormBlueprintConfigurationSection]
	document, err := Buildings.DecodeStormBlueprintDocument(raw, snapshot.GameData)
	if err != nil {
		return err
	}
	if active, found := document.Active(); found {
		target := active.Target
		settings.Target = &target
	}
	return nil
}

func autoStormAllowedTimeSkips(settings autoStormSettings) []string {
	if !settings.Build.AllowTimeSkips {
		return nil
	}
	return []string{"MS1", "MS2", "MS3", "MS4", "MS5", "MS6", "MS7"}
}

func autoStormBuildWaiting(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{Status: "waiting", Detail: detail, Metrics: metrics, NextCheckAt: now.Add(30 * time.Second)}
}
