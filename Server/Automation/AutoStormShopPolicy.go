package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"fmt"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const autoStormShopWatchdogInterval = 5 * time.Minute

type AutoStormShopPolicy struct{}

func NewAutoStormShopPolicy() *AutoStormShopPolicy { return &AutoStormShopPolicy{} }

func (*AutoStormShopPolicy) ID() string         { return "autoStormShop" }
func (*AutoStormShopPolicy) EnabledKey() string { return "auto_storm" }
func (*AutoStormShopPolicy) ActorID() string    { return "autoStorm" }
func (*AutoStormShopPolicy) ScheduleKey() string {
	return "autoStorm"
}

func (*AutoStormShopPolicy) WakeDomains() []string {
	// A Storm response or a returning movement can change Aquamarine or shop
	// stock immediately. The five-minute watchdog covers any game response that
	// did not carry a focused domain without returning to the old 30-second poll.
	return []string{"construction-offers", "movements", "storm"}
}

func (*AutoStormShopPolicy) WakeSections() []string {
	return []string{autoStormSection, Buildings.StormBlueprintConfigurationSection}
}

func (*AutoStormShopPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoStormSettings()
	if !decodeSection(snapshot.Configuration, autoStormSection, &settings) {
		return autoStormWaiting(snapshot.Now, "Auto Storm settings have not been saved", Localization.New("server.automation.auto_storm_settings_have.7747d389", "Auto Storm settings have not been saved", nil)), nil
	}
	normalizeAutoStormSettings(&settings)
	if settings.Version != 1 {
		return autoStormWaiting(snapshot.Now, fmt.Sprintf("Unsupported Auto Storm settings version %d", settings.Version), Localization.New("server.automation.unsupported_auto_storm_settings.354f0544", "Unsupported Auto Storm settings version {p0, number}", Localization.Params{"p0": settings.Version})), nil
	}
	if snapshot.GameData == nil {
		return autoStormWaiting(snapshot.Now, "Official game data is unavailable", Localization.New("server.automation.official_game_data_is.c5e55e7e", "Official game data is unavailable", nil)), nil
	}
	if err := autoStormApplyActiveBlueprint(snapshot, &settings); err != nil {
		return autoStormWaiting(snapshot.Now, err.Error(), Localization.FromError(err)), nil
	}
	castle, found := autoStormCastle(snapshot.State, settings.Target)
	if !found {
		return autoStormWaiting(snapshot.Now, "Waiting for the combat lane to unlock or reconcile the Storm castle", Localization.New("server.automation.waiting_for_the_combat.7933a7b9", "Waiting for the combat lane to unlock or reconcile the Storm castle", nil)), nil
	}
	metrics := map[string]float64{
		"castleId":   float64(castle.ID),
		"aquamarine": castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount,
	}
	decision, complete, detail, err := evaluateAutoStormShop(snapshot, settings, castle, metrics)
	var detailLocalizationMessage *Localization.Message = nil
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
		detailLocalizationMessage = Localization.New("server.automation.no_aquamarine_shop_goal.d89c4457", "No Aquamarine shop goal is configured", nil)
	}
	result := Decision{
		Status: status, Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), Metrics: metrics,
		NextCheckAt: snapshot.Now.Add(autoStormShopWatchdogInterval),
	}
	if len(settings.Aquamarine.Purchases) == 0 {
		result.NextCheckAt = time.Time{}
		result.EventDriven = true
	}
	return result, nil
}
