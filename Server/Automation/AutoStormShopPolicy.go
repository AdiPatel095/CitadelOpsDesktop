package Automation

import (
	"context"
	"fmt"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type AutoStormShopPolicy struct{}

func NewAutoStormShopPolicy() *AutoStormShopPolicy { return &AutoStormShopPolicy{} }

func (*AutoStormShopPolicy) ID() string         { return "autoStormShop" }
func (*AutoStormShopPolicy) EnabledKey() string { return "auto_storm" }
func (*AutoStormShopPolicy) ActorID() string    { return "autoStorm" }
func (*AutoStormShopPolicy) ScheduleKey() string {
	return "autoStorm"
}

func (*AutoStormShopPolicy) WakeDomains() []string {
	return []string{"castles", "construction-offers", "inventory", "resources", "storm"}
}

func (*AutoStormShopPolicy) WakeSections() []string {
	return []string{autoStormSection, Buildings.StormBlueprintConfigurationSection}
}

func (*AutoStormShopPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoStormSettings()
	if !decodeSection(snapshot.Configuration, autoStormSection, &settings) {
		return autoStormWaiting(snapshot.Now, "Auto Storm settings have not been saved"), nil
	}
	normalizeAutoStormSettings(&settings)
	if settings.Version != 1 {
		return autoStormWaiting(snapshot.Now, fmt.Sprintf("Unsupported Auto Storm settings version %d", settings.Version)), nil
	}
	if snapshot.GameData == nil {
		return autoStormWaiting(snapshot.Now, "Official game data is unavailable"), nil
	}
	if err := autoStormApplyActiveBlueprint(snapshot, &settings); err != nil {
		return autoStormWaiting(snapshot.Now, err.Error()), nil
	}
	castle, found := autoStormCastle(snapshot.State, settings.Target)
	if !found {
		return autoStormWaiting(snapshot.Now, "Waiting for the combat lane to unlock or reconcile the Storm castle"), nil
	}
	metrics := map[string]float64{
		"castleId":   float64(castle.ID),
		"aquamarine": castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount,
	}
	decision, complete, detail, err := evaluateAutoStormShop(snapshot, settings, castle, metrics)
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
		detail = "No Aquamarine shop goal is configured"
	}
	return Decision{
		Status: status, Detail: detail, Metrics: metrics,
		NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
	}, nil
}
