package Automation

import (
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
	return []string{"boosters", "buildings", "castles", "currencies", "events", "event-scores", "movements", "reports", "resources"}
}

func (*BeriBuildPolicy) WakeSections() []string {
	return []string{autoBeriWorldSection, Buildings.BerimondBlueprintConfigurationSection}
}

func (*BeriBuildPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	var settings beriSettings
	if !decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings) {
		return beriBuildWaiting(snapshot.Now, "Auto Beri World settings have not been saved", nil), nil
	}
	if decision := beriGallantryBoosterGate(snapshot, settings); decision != nil {
		return *decision, nil
	}
	normalizeBeriBuildSettings(&settings.Build)
	if !settings.Build.Enabled {
		return Decision{
			Status: "complete", Detail: "Automatic Berimond construction is disabled",
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, nil
	}
	if decision, locked := limitedEventGate(
		snapshot.State, snapshot.Now, []int64{GameData.BerimondEventID}, "Battle for Berimond",
	); locked {
		return decision, nil
	}
	if snapshot.GameData == nil {
		return beriBuildWaiting(snapshot.Now, "Official game data is unavailable", nil), nil
	}
	raw := snapshot.Configuration.Sections[Buildings.BerimondBlueprintConfigurationSection]
	document, err := Buildings.DecodeBerimondBlueprintDocument(raw, snapshot.GameData)
	if err != nil {
		return beriBuildWaiting(snapshot.Now, err.Error(), nil), nil
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
				nil,
			), nil
		}
		var found bool
		castle, found = snapshot.State.Castles[target.CastleID]
		if !found {
			return beriBuildWaiting(snapshot.Now, "Waiting for the captured Berimond camp; recapture the target if the season changed", nil), nil
		}
		if castle.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
			return beriBuildWaiting(snapshot.Now, "The captured target no longer resolves to an owned Berimond camp", nil), nil
		}
	} else {
		var found bool
		castle, found = beriToolCastle(snapshot.State, settings.BeriCastleID)
		if !found {
			return beriBuildWaiting(snapshot.Now, "Waiting for an owned Berimond camp", nil), nil
		}
		target, err = Buildings.DefaultBerimondTarget(castle.ID, settings.Build.StableLevel, snapshot.GameData)
		if err != nil {
			return beriBuildWaiting(snapshot.Now, err.Error(), nil), nil
		}
	}
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(GameData.BerimondKingdomID)]; observed &&
		!unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked",
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, nil
	}
	effectiveStableLevel := settings.Build.StableLevel
	if !customTarget {
		effectiveStableLevel = preserveHigherBeriStableTarget(&target, castle, snapshot.GameData, effectiveStableLevel)
	}
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
	decision, complete, detail, err := evaluateAutoEventBuild(
		snapshot,
		shared,
		castle,
		metrics,
		autoEventBuildProfile{
			KingdomID: State.KingdomID(GameData.BerimondKingdomID), FeatureLabel: "Berimond",
			AttackLootOnly: true, EventID: GameData.BerimondEventID,
		},
	)
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
	}
	return Decision{
		Status: status, Detail: detail, Metrics: metrics,
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
		if found && strings.EqualFold(definition.InternalName, "FactionStable") && definition.Level > current.Level {
			current = definition
		}
	}
	if current.Level <= configuredLevel {
		return configuredLevel
	}
	for index := range target.Buildings {
		definition, found := catalog.DefinitionView(int64(target.Buildings[index].DefinitionID))
		if found && strings.EqualFold(definition.InternalName, "FactionStable") {
			target.Buildings[index].DefinitionID = State.BuildingID(current.ID)
			return current.Level
		}
	}
	return configuredLevel
}

func beriBuildWaiting(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{
		Status: "waiting", Detail: detail, Metrics: metrics,
		NextCheckAt: now.Add(30 * time.Second),
	}
}
