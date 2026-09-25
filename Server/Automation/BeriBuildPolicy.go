package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type beriBuildSettings struct {
	Enabled          bool               `json:"enabled"`
	StableLevel      int64              `json:"stableLevel"`
	AllowPremium     bool               `json:"allowPremium"`
	AllowDemolition  bool               `json:"allowDemolition"`
	AllowTimeSkips   bool               `json:"allowTimeSkips"`
	ResourceReserves map[string]float64 `json:"resourceReserves"`
	TimeSkipReserve  map[string]int64   `json:"timeSkipReserve"`
}

type BeriBuildPolicy struct{}

func NewBeriBuildPolicy() *BeriBuildPolicy { return &BeriBuildPolicy{} }

func (*BeriBuildPolicy) ID() string         { return "autoBeriWorldBuild" }
func (*BeriBuildPolicy) EnabledKey() string { return "auto_beri_world" }
func (*BeriBuildPolicy) ActorID() string    { return "autoBeriWorld" }
func (*BeriBuildPolicy) ScheduleKey() string {
	return "autoBeriWorld"
}

func (*BeriBuildPolicy) WakeDomains() []string {
	return []string{"ruby-confirmation", "boosters", "buildings", "castles", "currencies", "events", "event-scores", "movements", "reports", "resources"}
}

func (*BeriBuildPolicy) WakeSections() []string {
	return []string{autoBeriWorldSection, Buildings.BerimondBlueprintConfigurationSection}
}

func (*BeriBuildPolicy) Evaluate(_ context.Context, snapshot Snapshot) (result Decision, resultErr error) {
	var settings beriSettings
	if !decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings) {
		return beriBuildWaiting(snapshot.Now, "Auto Beri World settings have not been saved", nil, Localization.New("server.automation.auto_beri_world_settings.a9d19bc3", "Auto Beri World settings have not been saved", nil)), nil
	}
	normalizeBeriBuildSettings(&settings.Build)
	if !settings.Build.Enabled {
		return Decision{
			Status: "disabled", Detail: "Auto Beri Builder is disabled by the user", DetailDescriptor: Localization.New("server.automation.auto_beri_builder_is.1ddcde0a", "Auto Beri Builder is disabled by the user", nil),
			EventDriven: true,
		}, nil
	}
	if decision := beriGallantryBoosterGate(snapshot, settings); decision != nil {
		return *decision, nil
	}
	if decision, locked := limitedEventGate(
		snapshot.State, snapshot.Now, []int64{GameData.BerimondEventID}, "Battle for Berimond",
	); locked {
		return decision, nil
	}
	if snapshot.GameData == nil {
		return beriBuildWaiting(snapshot.Now, "Official game data is unavailable", nil, Localization.New("server.automation.official_game_data_is.c5e55e7e", "Official game data is unavailable", nil)), nil
	}
	raw := snapshot.Configuration.Sections[Buildings.BerimondBlueprintConfigurationSection]
	document, err := Buildings.DecodeBerimondBlueprintDocument(raw, snapshot.GameData)
	if err != nil {
		return beriBuildWaiting(snapshot.Now, err.Error(), nil, Localization.FromError(err)), nil
	}
	blueprint, customTarget := document.Active()
	var target Buildings.TargetCaptureResult
	var castle State.CastleState
	if customTarget {
		target = Buildings.NormalizeTargetCapture(blueprint.Target, nil)
		if target.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
			return beriBuildWaiting(
				snapshot.Now,
				fmt.Sprintf("The active blueprint is not from Berimond kingdom %d", GameData.BerimondKingdomID),
				nil, Localization.New("server.automation.the_active_blueprint_is.d94fabdc", "The active blueprint is not from Berimond kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", GameData.BerimondKingdomID)}),
			), nil
		}
		var found bool
		castle, found = snapshot.State.Castles[target.CastleID]
		if !found {
			return beriBuildWaiting(snapshot.Now, "Waiting for the captured Berimond camp; recapture the target if the season changed", nil, Localization.New("server.automation.waiting_for_the_captured.5c83f032", "Waiting for the captured Berimond camp; recapture the target if the season changed", nil)), nil
		}
		if castle.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
			return beriBuildWaiting(snapshot.Now, "The captured target no longer resolves to an owned Berimond camp", nil, Localization.New("server.automation.the_captured_target_no.88c2ef51", "The captured target no longer resolves to an owned Berimond camp", nil)), nil
		}
	} else {
		var found bool
		castle, found = beriCastle(snapshot.State)
		if !found {
			return beriBuildWaiting(snapshot.Now, "Waiting for an owned Berimond camp", nil, Localization.New("server.automation.waiting_for_an_owned.deab064e", "Waiting for an owned Berimond camp", nil)), nil
		}
		target, err = Buildings.DefaultBerimondTarget(castle.ID, settings.Build.StableLevel, snapshot.GameData)
		if err != nil {
			return beriBuildWaiting(snapshot.Now, err.Error(), nil, Localization.FromError(err)), nil
		}
	}
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(GameData.BerimondKingdomID)]; observed &&
		!unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked", DetailDescriptor: Localization.New("server.automation.the_battle_for_berimond.a9f4b97a", "The Battle for Berimond is not currently unlocked", nil),
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, nil
	}
	effectiveStableLevel := settings.Build.StableLevel
	if customTarget {
		effectiveStableLevel = beriTargetStableLevel(target, snapshot.GameData, effectiveStableLevel)
	}
	effectiveStableLevel = preserveHigherBeriStableTarget(&target, castle, snapshot.GameData, effectiveStableLevel)
	metrics := map[string]float64{
		"castleId":                    float64(castle.ID),
		"wood":                        castle.Resources[State.ResourceID(3)].Amount,
		"stone":                       castle.Resources[State.ResourceID(4)].Amount,
		"configuredStableTargetLevel": float64(settings.Build.StableLevel),
		"stableTargetLevel":           float64(effectiveStableLevel),
	}
	if !customTarget {
		metrics["builtInTarget"] = 1
	}
	shared := defaultAutoStormSettings()
	shared.Target = &target
	shared.Harbor.Enabled = false
	shared.DecorationPresetCastleID = 0
	shared.DecorationPresetID = ""
	shared.Build = autoStormBuildSettings{
		AllowPremium: settings.Build.AllowPremium, AllowDemolition: settings.Build.AllowDemolition,
		AllowResourceTransport: false, AllowTimeSkips: settings.Build.AllowTimeSkips,
		ResourceReserves: settings.Build.ResourceReserves, SourceResourceReserves: map[string]float64{},
		TimeSkipReserve: settings.Build.TimeSkipReserve,
	}
	defer func() {
		if resultErr == nil {
			attachRubyUpgradeNotices(&result, snapshot, castle.ID, &target, shared.Build.AllowPremium)
		}
	}()
	decision, complete, detail, err := evaluateBeriEventBuild(
		snapshot,
		shared,
		castle,
		metrics,
		autoEventBuildProfile{
			KingdomID: State.KingdomID(GameData.BerimondKingdomID), FeatureLabel: "Berimond",
			AttackLootOnly: true, EventID: GameData.BerimondEventID,
			IgnoreDemolitionCandidate: isBeriStableDefinition,
		},
	)
	var detailLocalizationMessage *Localization.Message = nil
	if err != nil {
		return Decision{}, err
	}
	if decision != nil {
		return autoEventBuildContinuation(*decision, "Berimond"), nil
	}
	status := "waiting"
	if complete {
		status = "complete"
	}
	if detail == "" {
		detail = "Berimond construction is waiting for returned attack loot or a building-state change"
		detailLocalizationMessage = Localization.New("server.automation.berimond_construction_is_waiting.10e060c2", "Berimond construction is waiting for returned attack loot or a building-state change", nil)
	}
	return Decision{
		Status: status, Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), Metrics: metrics,
		NextCheckAt: snapshot.Now.Add(30 * time.Second),
	}, nil
}

func normalizeBeriBuildSettings(settings *beriBuildSettings) {
	if settings.StableLevel == 0 {
		settings.StableLevel = Buildings.DefaultBerimondStableTargetLevel
	}
	if settings.ResourceReserves == nil {
		settings.ResourceReserves = map[string]float64{}
	}
	if settings.TimeSkipReserve == nil {
		settings.TimeSkipReserve = map[string]int64{}
	}
}

func preserveHigherBeriStableTarget(
	target *Buildings.TargetCaptureResult,
	castle State.CastleState,
	gameData *GameData.Store,
	configuredLevel int64,
) int64 {
	if gameData == nil {
		return configuredLevel
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return configuredLevel
	}
	current := GameData.BuildingDefinition{}
	for _, building := range castle.Layout.Objects {
		if !building.Placed {
			continue
		}
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if found && isBeriStableDefinition(definition) && definition.Level > current.Level {
			current = definition
		}
	}
	if current.Level <= configuredLevel {
		return configuredLevel
	}
	for index := range target.Buildings {
		definition, found := catalog.DefinitionView(int64(target.Buildings[index].DefinitionID))
		if found && isBeriStableDefinition(definition) {
			target.Buildings[index].DefinitionID = State.BuildingID(current.ID)
			return current.Level
		}
	}
	return configuredLevel
}

func beriTargetStableLevel(target Buildings.TargetCaptureResult, gameData *GameData.Store, fallback int64) int64 {
	if gameData == nil {
		return fallback
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return fallback
	}
	for _, building := range target.Buildings {
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if found && isBeriStableDefinition(definition) {
			return definition.Level
		}
	}
	return fallback
}

func isBeriStableDefinition(definition GameData.BuildingDefinition) bool {
	name := strings.TrimSpace(definition.InternalName)
	return strings.EqualFold(name, "FactionStable") || strings.EqualFold(name, "Stable")
}

func beriBuildWaiting(now time.Time, detail string, metrics map[string]float64, descriptors ...*Localization.Message) Decision {
	return Decision{
		Status: "waiting", Detail: detail, DetailDescriptor: Localization.First(descriptors), Metrics: metrics,
		NextCheckAt: now.Add(30 * time.Second),
	}
}
