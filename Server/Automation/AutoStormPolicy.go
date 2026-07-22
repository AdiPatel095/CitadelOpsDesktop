package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoStormSection               = "automation.autoStorm"
	autoStormKingdomID             = State.KingdomID(GameData.StormKingdomID)
	autoStormIslandMapTypeID       = 24
	autoStormFortMapTypeID         = 25
	autoStormTransportDelivery     = 0.8
	autoStormMaximumTroopStacks    = 20
	autoStormMapRefreshInterval    = 6 * time.Hour
	autoStormMapRefreshSeconds     = int(autoStormMapRefreshInterval / time.Second)
	autoStormMapWindowSize         = 101
	autoStormMapMinimumWindows     = 6
	autoStormTargetVerificationAge = 30 * time.Second
	autoStormPriorityFortPrefix    = "fort:"
	autoStormPriorityIslandPrefix  = "island:"
)

type AutoStormPolicy struct{}

type autoStormSettings struct {
	Version                  int                            `json:"version"`
	Target                   *Buildings.TargetCaptureResult `json:"target,omitempty"`
	DecorationPresetCastleID State.CastleID                 `json:"decorationPresetCastleId,omitempty"`
	DecorationPresetID       string                         `json:"decorationPresetId,omitempty"`
	Build                    autoStormBuildSettings         `json:"build"`
	Harbor                   autoStormHarborSettings        `json:"harbor"`
	Forts                    autoStormFortSettings          `json:"forts"`
	Islands                  autoStormIslandSettings        `json:"islands"`
	TroopImport              autoStormTroopImportSettings   `json:"troopImport"`
	Aquamarine               autoStormAquamarineSettings    `json:"aquamarine"`
	TargetPriority           []string                       `json:"targetPriority"`
	LegacyCombatOrder        string                         `json:"combatOrder,omitempty"`
	CheckIntervalSec         int                            `json:"checkIntervalSec"`
	MapRefreshIntervalSec    int                            `json:"mapRefreshIntervalSec"`
	DailyAttackLimit         int64                          `json:"dailyAttackLimit"`
	HorseTravelBoostID       int                            `json:"horseTravelBoostId"`
}

type autoStormBuildSettings struct {
	AllowPremium           bool               `json:"allowPremium"`
	AllowDemolition        bool               `json:"allowDemolition"`
	AllowResourceTransport bool               `json:"allowResourceTransport"`
	AllowTimeSkips         bool               `json:"allowTimeSkips"`
	ResourceReserves       map[string]float64 `json:"resourceReserves"`
	TimeSkipReserve        map[string]int64   `json:"timeSkipReserve"`
}

type autoStormHarborSettings struct {
	Enabled     bool  `json:"enabled"`
	TargetLevel int64 `json:"targetLevel"`
}

type autoStormFortSettings struct {
	Enabled     bool   `json:"enabled"`
	Levels      []int  `json:"levels"`
	MinimumWins int64  `json:"minimumWins"`
	PresetID    string `json:"presetId"`
}

type autoStormDefenseUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type autoStormIslandSettings struct {
	Enabled      bool                   `json:"enabled"`
	Resources    []string               `json:"resources"`
	Sizes        []string               `json:"sizes"`
	PresetID     string                 `json:"presetId"`
	DefenseUnits []autoStormDefenseUnit `json:"defenseUnits"`
}

type autoStormTroopImportSettings struct {
	Enabled        bool             `json:"enabled"`
	DonorCastleIDs []State.CastleID `json:"donorCastleIds"`
}

type autoStormShopPurchase struct {
	PackageID       State.PackageID `json:"packageId"`
	TargetPurchases int64           `json:"targetPurchases"`
	Unlimited       bool            `json:"unlimited"`
	Priority        int             `json:"priority"`
}

type autoStormShopPurchaseLine struct {
	ProductID State.PackageID `json:"productId"`
	Amount    int64           `json:"amount"`
}

type autoStormAquamarineSettings struct {
	Reserve     int64                   `json:"reserve"`
	ShopTableID int64                   `json:"shopTableId"`
	Purchases   []autoStormShopPurchase `json:"purchases"`
}

type autoStormDecorationPreset struct {
	ID    string                    `json:"id"`
	Name  string                    `json:"name"`
	Items []autoStormDecorationItem `json:"items"`
}

type autoStormDecorationItem struct {
	WID   State.BuildingID `json:"wid"`
	X     int              `json:"x"`
	Y     int              `json:"y"`
	R     int              `json:"r"`
	Layer string           `json:"layer,omitempty"`
}

type autoStormDecorationDocument struct {
	Version int                                    `json:"version"`
	Castles map[string][]autoStormDecorationPreset `json:"castles"`
}

type autoStormCombatCandidate struct {
	Observation State.MapObservation
	Definition  GameData.StormIsleDefinition
	PresetID    string
	Defense     []autoStormDefenseUnit
}

func NewAutoStormPolicy() *AutoStormPolicy { return &AutoStormPolicy{} }

func (*AutoStormPolicy) ID() string { return "autoStorm" }

func (*AutoStormPolicy) EnabledKey() string { return "auto_storm" }

func (*AutoStormPolicy) WakeDomains() []string {
	return []string{
		"attacks", "buildings", "castles", "construction-items", "construction-offers", "inventory", "map", "movements",
		"reports", "resources", "storm", "units", "kingdom-transport",
	}
}

func (*AutoStormPolicy) WakeSections() []string {
	return []string{autoStormSection, AttackPresets.ConfigurationSection, "decorations.presets", commanderFeatureSection}
}

func (*AutoStormPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoStormSettings()
	if !decodeSection(snapshot.Configuration, autoStormSection, &settings) {
		return autoStormWaiting(snapshot.Now, "Auto Storm settings have not been saved"), nil
	}
	normalizeAutoStormSettings(&settings)
	if settings.Version != 1 {
		return autoStormWaiting(snapshot.Now, fmt.Sprintf("Unsupported Auto Storm settings version %d", settings.Version)), nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return autoStormWaiting(snapshot.Now, "Choose a supported horse travel boost"), nil
	}
	if snapshot.GameData == nil {
		return autoStormWaiting(snapshot.Now, "Official game data is unavailable"), nil
	}
	castle, found := autoStormCastle(snapshot.State, settings.Target)
	if !found {
		return autoStormWaiting(snapshot.Now, "No unlocked Storm castle is present; Auto Storm will resume when kingdom 4 becomes available"), nil
	}
	metrics := map[string]float64{
		"castleId":   float64(castle.ID),
		"aquamarine": castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount,
	}
	returnDecision, islandReturnDetail := autoStormIslandReturnDecision(snapshot, castle, metrics)
	if returnDecision != nil {
		return *returnDecision, nil
	}
	combatDetail := ""
	nextStormOpportunityAt := time.Time{}
	if settings.Forts.Enabled || settings.Islands.Enabled {
		if scanDecision := autoStormFullMapScanDecision(snapshot, castle, metrics); scanDecision != nil {
			return *scanDecision, nil
		}
		nextStormOpportunityAt = autoStormNextOpportunityAt(snapshot, settings, castle)
		if !nextStormOpportunityAt.IsZero() {
			metrics["nextStormOpportunityAtUnix"] = float64(nextStormOpportunityAt.Unix())
		}
		combatDecision, detail, err := evaluateAutoStormCombat(snapshot, settings, castle, metrics)
		if err != nil {
			return Decision{}, err
		}
		combatDetail = detail
		if combatDecision != nil {
			return *combatDecision, nil
		}
	}

	shopComplete := len(settings.Aquamarine.Purchases) == 0
	shopDetail := ""
	shopDecision, complete, detail, shopErr := evaluateAutoStormShop(snapshot, settings, castle, metrics)
	if shopErr != nil {
		return Decision{}, shopErr
	}
	shopComplete = complete
	shopDetail = detail
	if shopDecision != nil {
		return *shopDecision, nil
	}

	buildDecision, buildComplete, buildDetail, err := evaluateAutoStormBuild(snapshot, settings, castle, metrics)
	if err != nil {
		return Decision{}, err
	}
	if buildDecision != nil {
		return *buildDecision, nil
	}

	details := make([]string, 0, 4)
	if islandReturnDetail != "" {
		details = append(details, islandReturnDetail)
	}
	if buildDetail != "" {
		details = append(details, buildDetail)
	}
	if shopDetail != "" {
		details = append(details, shopDetail)
	}
	if combatDetail != "" {
		details = append(details, combatDetail)
	}
	if len(details) == 0 {
		details = append(details, "No Storm construction, combat, or Aquamarine shop goal is configured")
	}
	status := "waiting"
	if buildComplete && shopComplete && !settings.Forts.Enabled && !settings.Islands.Enabled && islandReturnDetail == "" {
		status = "complete"
	}
	nextCheckAt := snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30))
	if nextStormOpportunityAt.After(snapshot.Now) && nextStormOpportunityAt.Before(nextCheckAt) {
		nextCheckAt = nextStormOpportunityAt
	}
	return Decision{
		Status: status, Detail: strings.Join(details, " · "), Metrics: metrics,
		NextCheckAt: nextCheckAt,
	}, nil
}

func defaultAutoStormSettings() autoStormSettings {
	return autoStormSettings{
		Version: 1,
		Build:   autoStormBuildSettings{ResourceReserves: map[string]float64{}, TimeSkipReserve: map[string]int64{}},
		Forts:   autoStormFortSettings{Levels: []int{40, 50, 60, 70, 80}},
		Islands: autoStormIslandSettings{
			Resources: []string{"wood", "stone", "aquamarine"}, Sizes: []string{"large", "small"},
			DefenseUnits: []autoStormDefenseUnit{},
		},
		TroopImport:    autoStormTroopImportSettings{DonorCastleIDs: []State.CastleID{}},
		Aquamarine:     autoStormAquamarineSettings{Purchases: []autoStormShopPurchase{}},
		TargetPriority: nil, CheckIntervalSec: 30, MapRefreshIntervalSec: autoStormMapRefreshSeconds,
		HorseTravelBoostID: -1,
	}
}

func normalizeAutoStormSettings(settings *autoStormSettings) {
	if settings.Build.ResourceReserves == nil {
		settings.Build.ResourceReserves = map[string]float64{}
	}
	if settings.Build.TimeSkipReserve == nil {
		settings.Build.TimeSkipReserve = map[string]int64{}
	}
	if settings.Forts.Levels == nil {
		settings.Forts.Levels = []int{}
	}
	if settings.Islands.Resources == nil {
		settings.Islands.Resources = []string{}
	}
	if settings.Islands.Sizes == nil {
		settings.Islands.Sizes = []string{}
	}
	if settings.Islands.DefenseUnits == nil {
		settings.Islands.DefenseUnits = []autoStormDefenseUnit{}
	}
	if settings.TroopImport.DonorCastleIDs == nil {
		settings.TroopImport.DonorCastleIDs = []State.CastleID{}
	}
	donors := make([]State.CastleID, 0, len(settings.TroopImport.DonorCastleIDs))
	seenDonors := map[State.CastleID]struct{}{}
	for _, castleID := range settings.TroopImport.DonorCastleIDs {
		if castleID <= 0 {
			continue
		}
		if _, duplicate := seenDonors[castleID]; duplicate {
			continue
		}
		seenDonors[castleID] = struct{}{}
		donors = append(donors, castleID)
	}
	settings.TroopImport.DonorCastleIDs = donors
	settings.Forts.MinimumWins = max(int64(0), settings.Forts.MinimumWins)
	if settings.Aquamarine.Purchases == nil {
		settings.Aquamarine.Purchases = []autoStormShopPurchase{}
	}
	settings.DecorationPresetID = strings.TrimSpace(settings.DecorationPresetID)
	settings.Forts.PresetID = strings.TrimSpace(settings.Forts.PresetID)
	settings.Islands.PresetID = strings.TrimSpace(settings.Islands.PresetID)
	settings.TargetPriority = normalizeAutoStormTargetPriority(settings.TargetPriority, settings.LegacyCombatOrder)
	if settings.CheckIntervalSec < 30 || settings.CheckIntervalSec > 3600 {
		settings.CheckIntervalSec = 30
	}
	settings.MapRefreshIntervalSec = autoStormMapRefreshSeconds
}

func autoStormCastle(state State.GameState, target *Buildings.TargetCaptureResult) (State.CastleState, bool) {
	if target != nil {
		if castle, found := state.Castles[target.CastleID]; found && castle.KingdomID == autoStormKingdomID {
			return castle, true
		}
	}
	ids := make([]State.CastleID, 0)
	for id, castle := range state.Castles {
		if castle.KingdomID == autoStormKingdomID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	if len(ids) == 0 {
		return State.CastleState{}, false
	}
	return state.Castles[ids[0]], true
}

func autoStormMapStateMatches(state State.GameState, castle State.CastleState) bool {
	mapState := state.Storm.Map
	return mapState.ServerURL == state.Session.ServerURL && mapState.PlayerID == state.Player.ID &&
		mapState.SourceCastleID == castle.ID
}

func autoStormMapScanBounds(state State.GameState, castle State.CastleState) State.StormMapBounds {
	maximumX := max(0, castle.X)
	maximumY := max(0, castle.Y)
	bounds := State.StormMapBounds{
		X1: 0,
		Y1: 0,
		X2: autoStormRoundedMapMaximum(maximumX),
		Y2: autoStormRoundedMapMaximum(maximumY),
	}
	if autoStormMapStateMatches(state, castle) && state.Storm.Map.NextBounds.IsValid() &&
		(state.Storm.Map.NextBounds.X2 > 0 || state.Storm.Map.NextBounds.Y2 > 0) {
		bounds.X2 = max(bounds.X2, state.Storm.Map.NextBounds.X2)
		bounds.Y2 = max(bounds.Y2, state.Storm.Map.NextBounds.Y2)
	}
	return bounds
}

func autoStormRoundedMapMaximum(observedMaximum int) int {
	minimumMaximum := autoStormMapMinimumWindows*autoStormMapWindowSize - 1
	requiredMaximum := max(minimumMaximum, observedMaximum+autoStormMapWindowSize)
	windowCount := (requiredMaximum + 1 + autoStormMapWindowSize - 1) / autoStormMapWindowSize
	return windowCount*autoStormMapWindowSize - 1
}

func autoStormFullMapScanDecision(snapshot Snapshot, castle State.CastleState, metrics map[string]float64) *Decision {
	mapStateCurrent := autoStormMapStateMatches(snapshot.State, castle)
	mapState := snapshot.State.Storm.Map
	lastAttemptAt := time.Time{}
	if mapStateCurrent {
		lastAttemptAt = mapState.LastAttemptAt
		metrics["stormMapTargets"] = float64(len(mapState.Targets))
		metrics["stormMapWindows"] = float64(mapState.WindowCount)
		metrics["stormMapMaximumX"] = float64(mapState.CoveredBounds.X2)
		metrics["stormMapMaximumY"] = float64(mapState.CoveredBounds.Y2)
	} else if mapState.SourceCastleID == 0 {
		// Honor a recent pre-full-map Storm scan during migration so upgrading
		// cannot immediately add another scan burst.
		lastAttemptAt = snapshot.State.Storm.LastScannedAt[castle.ID]
	}
	if !lastAttemptAt.IsZero() && snapshot.Now.Before(lastAttemptAt.Add(autoStormMapRefreshInterval)) {
		return nil
	}
	bounds := autoStormMapScanBounds(snapshot.State, castle)
	windowColumns := (bounds.X2 - bounds.X1 + autoStormMapWindowSize) / autoStormMapWindowSize
	windowRows := (bounds.Y2 - bounds.Y1 + autoStormMapWindowSize) / autoStormMapWindowSize
	metrics["stormMapPlannedWindows"] = float64(windowColumns * windowRows)
	decision := autoStormIntentDecision(
		snapshot.Now,
		metrics,
		fmt.Sprintf("Refresh the complete Storm map for %s", autoStormCastleName(castle)),
		"storm.map.scan",
		map[string]any{
			"sourceCastleId": castle.ID,
			"fullMap":        true,
			"bounds":         bounds,
			"scanStartedAt":  snapshot.Now,
		},
	)
	decision.NextCheckAt = snapshot.Now.Add(autoStormMapRefreshInterval)
	return decision
}

func evaluateAutoStormBuild(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, bool, string, error) {
	if settings.Target == nil && !settings.Harbor.Enabled {
		return nil, true, "", nil
	}
	if settings.Target != nil && settings.Target.KingdomID != autoStormKingdomID {
		return nil, false, "Captured target is not a Storm castle state", nil
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() || castle.BuildingQueue.ObservedAt.IsZero() {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh the Storm castle building state", "building.refresh", map[string]any{
			"castleId": castle.ID,
		}), false, "", nil
	}
	catalog, err := snapshot.GameData.BuildingCatalog()
	if err != nil {
		return nil, false, "", err
	}

	if giftID, found := autoStormExpansionGift(castle, catalog); found {
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Collect expansion gift %d before reconciling the layout", giftID), "building.collect_expansion_gift", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": giftID,
		}), false, "", nil
	}

	queueDecision, queueBlocked := autoStormQueueDecision(snapshot, settings, castle, catalog, metrics)
	if queueDecision != nil {
		return queueDecision, false, "", nil
	}
	buildWaiting := ""
	if queueBlocked {
		buildWaiting = "The Storm construction queue is occupied"
	}

	if settings.Target != nil {
		missingGround := autoStormMissingGround(castle, settings.Target.Ground)
		metrics["targetGroundRemaining"] = float64(len(missingGround))
		if len(missingGround) > 0 && !queueBlocked {
			decision, detail, expansionErr := autoStormExpansionDecision(snapshot, settings, castle, missingGround, metrics)
			if expansionErr != nil {
				return nil, false, "", expansionErr
			}
			if decision != nil {
				return decision, false, "", nil
			}
			buildWaiting = detail
		}
	}

	targetBuildings, exact, ignoreDecorations, targetErr := autoStormTargetBuildings(settings, catalog)
	if targetErr != nil {
		return nil, false, targetErr.Error(), nil
	}
	if len(targetBuildings) == 0 {
		if settings.Target != nil && len(autoStormMissingGround(castle, settings.Target.Ground)) > 0 {
			return nil, false, buildWaiting, nil
		}
		return autoStormDecorationDecision(snapshot, settings, castle, catalog, metrics)
	}
	diff, err := Buildings.CompileTargetDiff(snapshot.State, snapshot.GameData, Buildings.TargetDiffRequest{
		CastleID: castle.ID, Exact: exact,
		Policy: Buildings.TargetDiffPolicy{
			AllowPremium: settings.Build.AllowPremium, IgnoreDecorations: ignoreDecorations,
			ResourceReserves: settings.Build.ResourceReserves,
		},
		Buildings: targetBuildings,
	})
	if err != nil {
		return nil, false, "", err
	}
	metrics["targetBuildingsSatisfied"] = float64(diff.Summary.SatisfiedCount)
	metrics["targetBuildingsTotal"] = float64(diff.Summary.TargetCount)
	metrics["targetActionsRemaining"] = float64(diff.Summary.ActionCount)
	metrics["unmanagedBuildings"] = float64(diff.Summary.UnmanagedCount)

	if remediation := autoStormDiffRemediation(snapshot, settings, castle, catalog, diff, metrics); remediation != nil {
		return remediation, false, "", nil
	}
	if !queueBlocked {
		storageDefinitionIDs := make([]State.BuildingID, 0, len(diff.Actions))
		for _, pendingAction := range diff.Actions {
			if pendingAction.Intent == "building.construct" || pendingAction.Intent == "building.upgrade" {
				storageDefinitionIDs = append(storageDefinitionIDs, pendingAction.Definition.ID)
			}
		}
		for _, action := range diff.Actions {
			if len(action.DependsOn) > 0 {
				continue
			}
			if issue, blocked := autoStormTargetActionIssue(diff, action); blocked {
				if buildWaiting == "" {
					buildWaiting = issue.Message
				}
				continue
			}
			storage, storageErr := Buildings.PreviewStorageDependency(snapshot.State, snapshot.GameData, Buildings.StorageDependencyRequest{
				CastleID: castle.ID, Costs: action.Costs,
				ResourceReserves: settings.Build.ResourceReserves,
				AllowPremium:     settings.Build.AllowPremium, AllowResourceTransport: settings.Build.AllowResourceTransport,
				AllowTimeSkips: settings.Build.AllowTimeSkips, AllowedBuildingDefinitionIDs: storageDefinitionIDs,
			})
			if storageErr != nil {
				return nil, false, "", storageErr
			}
			if storage.Required {
				metrics["targetStorageDependencies"] = float64(len(storage.CapacityNeeds))
				if storage.RecommendedAction != nil {
					dependencyAction := *storage.RecommendedAction
					if autoStormExpansionActionAllowed(dependencyAction, settings) {
						autoStormApplyTimeSkipReserve(dependencyAction.Arguments, dependencyAction.Intent, settings.Build.TimeSkipReserve)
						return autoStormIntentDecision(snapshot.Now, metrics, dependencyAction.Reason, dependencyAction.Intent, dependencyAction.Arguments), false, "", nil
					}
					buildWaiting = dependencyAction.Reason
				} else if len(storage.Blockers) > 0 {
					buildWaiting = storage.Blockers[0].Message
				} else {
					buildWaiting = "The next captured target action requires more storage capacity"
				}
				continue
			}
			if action.AffordableNow {
				if decision := autoStormTargetActionDecision(snapshot.Now, settings, castle, action, metrics); decision != nil {
					return decision, false, "", nil
				}
			}
			if settings.Build.AllowResourceTransport {
				decision, detail := autoStormTargetTransportDecision(snapshot, settings, castle, action, metrics)
				if decision != nil {
					return decision, false, "", nil
				}
				if detail != "" {
					buildWaiting = detail
				}
			}
			if buildWaiting == "" {
				buildWaiting = fmt.Sprintf("Waiting for resources to %s %s", action.Kind, action.Definition.DisplayName)
			}
		}
	}
	if !diff.Satisfied {
		if buildWaiting == "" && len(diff.Issues) > 0 {
			buildWaiting = diff.Issues[0].Message
		}
		if buildWaiting == "" {
			buildWaiting = "The captured Storm building state is not yet satisfied"
		}
		return nil, false, buildWaiting, nil
	}
	if settings.Target != nil && len(autoStormMissingGround(castle, settings.Target.Ground)) > 0 {
		return nil, false, buildWaiting, nil
	}
	return autoStormDecorationDecision(snapshot, settings, castle, catalog, metrics)
}

func autoStormTargetActionIssue(diff Buildings.TargetDiffResult, action Buildings.TargetAction) (Buildings.TargetIssue, bool) {
	for _, target := range diff.Targets {
		if target.TargetID != action.TargetID {
			continue
		}
		for _, issue := range target.Issues {
			if issue.Code != "resources_pending" {
				return issue, true
			}
		}
		break
	}
	return Buildings.TargetIssue{}, false
}

func autoStormExpansionGift(castle State.CastleState, catalog *GameData.BuildingCatalog) (State.BuildingInstanceID, bool) {
	ids := sortedAutoStormBuildingIDs(castle.Layout.Objects)
	for _, id := range ids {
		building := castle.Layout.Objects[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if found && building.Placed && strings.EqualFold(definition.InternalName, "TreasureChest") {
			return id, true
		}
	}
	return 0, false
}

func autoStormQueueDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	metrics map[string]float64,
) (*Decision, bool) {
	occupied := make([]State.BuildingInstanceID, 0)
	available := false
	for _, slot := range castle.BuildingQueue.Slots {
		if slot.Status == State.BuildingQueueSlotAvailable {
			available = true
		}
		if slot.Status == State.BuildingQueueSlotOccupied && slot.BuildingID > 0 {
			occupied = append(occupied, slot.BuildingID)
		}
	}
	sort.Slice(occupied, func(left, right int) bool { return occupied[left] < occupied[right] })
	metrics["constructionQueueOccupied"] = float64(len(occupied))
	for _, buildingID := range occupied {
		building, found := autoStormBuilding(castle, buildingID)
		if !found {
			continue
		}
		remaining, known := autoStormBuildingRemaining(castle, building, catalog, snapshot.Now)
		if !known {
			continue
		}
		if remaining <= 60 {
			return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Finish Storm building %d through the free path", buildingID), "building.finish_free", map[string]any{
				"castleId": castle.ID, "buildingInstanceId": buildingID,
			}), false
		}
		if settings.Build.AllowTimeSkips {
			if minutes, reserve, found := autoStormBuildingTimeSkip(snapshot.State, settings.Build.TimeSkipReserve, remaining); found {
				return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Apply a %d-minute skip to Storm building %d", minutes, buildingID), "building.skip_time", map[string]any{
					"castleId": castle.ID, "buildingInstanceId": buildingID, "minutes": minutes, "minimumRemaining": reserve,
				}), false
			}
		}
	}
	return nil, len(occupied) > 0 && !available
}

func autoStormBuildingRemaining(
	castle State.CastleState,
	building State.Building,
	catalog *GameData.BuildingCatalog,
	now time.Time,
) (int64, bool) {
	current, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return 0, false
	}
	target := current
	inProgress := false
	switch building.ConstructionState {
	case State.BuildingStateBuildStopped:
	case State.BuildingStateBuildInProgress:
		inProgress = true
	case State.BuildingStateUpgradeStopped, State.BuildingStateUpgradeInProgress:
		if current.UpgradeDefinitionID <= 0 {
			return 0, false
		}
		target, found = catalog.Definition(current.UpgradeDefinitionID)
		if !found {
			return 0, false
		}
		inProgress = building.ConstructionState == State.BuildingStateUpgradeInProgress
	default:
		return 0, false
	}
	if target.DurationSec <= 0 {
		return 0, false
	}
	progress := max(int64(0), building.ProgressSec)
	if inProgress && !castle.Layout.ObservedAt.IsZero() && now.After(castle.Layout.ObservedAt) {
		progress += int64(now.Sub(castle.Layout.ObservedAt) / time.Second)
	}
	return max(int64(0), target.DurationSec-progress), true
}

func autoStormBuildingTimeSkip(state State.GameState, reserves map[string]int64, remainingSec int64) (int, int64, bool) {
	options := []struct {
		Minutes  int
		Currency State.CurrencyID
		Key      string
	}{
		{1440, 1007, "MS7"}, {300, 1006, "MS6"}, {60, 1005, "MS5"}, {30, 1004, "MS4"},
		{10, 1003, "MS3"}, {5, 1002, "MS2"}, {1, 1001, "MS1"},
	}
	for _, option := range options {
		if option.Minutes*60 > int(remainingSec) && option.Minutes != 1 {
			continue
		}
		reserve := autoStormTimeSkipReserve(reserves, option.Currency, option.Key)
		if state.Player.Currencies[option.Currency]-1 >= float64(reserve) {
			return option.Minutes, reserve, true
		}
	}
	return 0, 0, false
}

func autoStormMissingGround(castle State.CastleState, target []Buildings.TargetGround) []Buildings.TargetGround {
	result := make([]Buildings.TargetGround, 0)
	for _, desired := range target {
		found := false
		for _, current := range castle.Layout.Ground {
			if current.Placed && current.DefinitionID == desired.DefinitionID && current.GridX == desired.GridX &&
				current.GridY == desired.GridY && current.Rotation == desired.Direction {
				found = true
				break
			}
		}
		if !found {
			result = append(result, desired)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].GridY != result[right].GridY {
			return result[left].GridY < result[right].GridY
		}
		if result[left].GridX != result[right].GridX {
			return result[left].GridX < result[right].GridX
		}
		return result[left].Direction < result[right].Direction
	})
	return result
}

func autoStormExpansionDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	missing []Buildings.TargetGround,
	metrics map[string]float64,
) (*Decision, string, error) {
	baseRequest := Buildings.ExpansionPreviewRequest{
		CastleID: castle.ID, Payment: Buildings.ExpansionPaymentResources,
		ResourceReserves: settings.Build.ResourceReserves, AllowPremium: settings.Build.AllowPremium,
		AllowTimeSkips: settings.Build.AllowTimeSkips,
	}
	base, err := Buildings.PreviewExpansion(snapshot.State, snapshot.GameData, baseRequest)
	if err != nil {
		return nil, "", err
	}
	if base.NextExpansion == nil {
		return nil, "The captured target requires ground that cannot be expanded further", nil
	}
	ground, found := autoStormGroundForExpansion(missing, base.NextExpansion.SpaceIDs)
	if !found {
		return nil, fmt.Sprintf("Captured ground does not contain the next official expansion level %d", base.NextExpansion.Level), nil
	}
	x, y, direction := ground.GridX, ground.GridY, ground.Direction
	baseRequest.X, baseRequest.Y, baseRequest.Direction = &x, &y, &direction
	preview, err := Buildings.PreviewExpansion(snapshot.State, snapshot.GameData, baseRequest)
	if err != nil {
		return nil, "", err
	}
	if preview.RecommendedAction == nil && settings.Build.AllowPremium && autoStormExpansionPaymentUnavailable(preview) {
		baseRequest.Payment = Buildings.ExpansionPaymentPremium
		preview, err = Buildings.PreviewExpansion(snapshot.State, snapshot.GameData, baseRequest)
		if err != nil {
			return nil, "", err
		}
	}
	if preview.RecommendedAction != nil {
		action := *preview.RecommendedAction
		if !autoStormExpansionActionAllowed(action, settings) {
			return nil, action.Reason, nil
		}
		autoStormApplyTimeSkipReserve(action.Arguments, action.Intent, settings.Build.TimeSkipReserve)
		return autoStormIntentDecision(snapshot.Now, metrics, action.Reason, action.Intent, action.Arguments), "", nil
	}
	if len(preview.Blockers) > 0 {
		return nil, preview.Blockers[0].Message, nil
	}
	return nil, "The next captured Storm expansion is not currently actionable", nil
}

func autoStormGroundForExpansion(missing []Buildings.TargetGround, spaceIDs []int64) (Buildings.TargetGround, bool) {
	spaces := map[State.BuildingID]struct{}{}
	for _, id := range spaceIDs {
		spaces[State.BuildingID(id)] = struct{}{}
	}
	for _, ground := range missing {
		if _, found := spaces[ground.DefinitionID]; found || len(spaces) == 0 {
			return ground, true
		}
	}
	return Buildings.TargetGround{}, false
}

func autoStormExpansionPaymentUnavailable(preview Buildings.ExpansionPreviewResult) bool {
	for _, blocker := range preview.Blockers {
		if blocker.Code == "payment_unavailable" {
			return true
		}
	}
	return false
}

func autoStormExpansionActionAllowed(action Buildings.ExpansionAction, settings autoStormSettings) bool {
	switch action.Intent {
	case "resource.logistics.refresh", "resource.ship", "resource.kingdom.ship":
		return settings.Build.AllowResourceTransport
	case "resource.kingdom.skip":
		return settings.Build.AllowResourceTransport && settings.Build.AllowTimeSkips
	case "building.skip_time":
		return settings.Build.AllowTimeSkips
	default:
		return true
	}
}

func autoStormTargetBuildings(
	settings autoStormSettings,
	catalog *GameData.BuildingCatalog,
) ([]Buildings.TargetBuilding, bool, bool, error) {
	result := make([]Buildings.TargetBuilding, 0)
	exact := false
	ignoreDecorations := false
	if settings.Target != nil {
		result = append(result, settings.Target.Buildings...)
		exact = settings.Target.Exact
		ignoreDecorations = settings.Target.Mode == Buildings.TargetCaptureModeBuildings
	}
	if !settings.Harbor.Enabled {
		return result, exact, ignoreDecorations, nil
	}
	if settings.Harbor.TargetLevel < 1 || settings.Harbor.TargetLevel > 3 {
		return nil, false, false, fmt.Errorf("Harbor target level must be between 1 and 3")
	}
	filtered := result[:0]
	for _, target := range result {
		definition, found := catalog.Definition(int64(target.DefinitionID))
		if found && strings.EqualFold(definition.InternalName, "Harbor") {
			continue
		}
		filtered = append(filtered, target)
	}
	result = filtered
	harbor, found := autoStormHarborDefinition(catalog, settings.Harbor.TargetLevel)
	if !found {
		return nil, false, false, fmt.Errorf("Official data has no Storm Harbor level %d", settings.Harbor.TargetLevel)
	}
	result = append(result, Buildings.TargetBuilding{TargetID: "storm-harbor", DefinitionID: State.BuildingID(harbor.ID)})
	return result, exact, ignoreDecorations, nil
}

func autoStormHarborDefinition(catalog *GameData.BuildingCatalog, level int64) (GameData.BuildingDefinition, bool) {
	for _, definition := range catalog.Definitions() {
		if strings.EqualFold(definition.InternalName, "Harbor") && definition.Level == level {
			return definition, true
		}
	}
	return GameData.BuildingDefinition{}, false
}

func autoStormDiffRemediation(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	diff Buildings.TargetDiffResult,
	metrics map[string]float64,
) *Decision {
	for _, issue := range diff.Issues {
		for _, buildingID := range issue.BuildingIDs {
			if decision := autoStormBuildingRemovalDecision(snapshot.Now, settings, castle, catalog, buildingID, metrics); decision != nil {
				return decision
			}
		}
	}
	if diff.Exact && len(diff.Actions) == 0 {
		for _, unmanaged := range diff.Unmanaged {
			if decision := autoStormBuildingRemovalDecision(snapshot.Now, settings, castle, catalog, unmanaged.BuildingInstanceID, metrics); decision != nil {
				return decision
			}
		}
	}
	return nil
}

func autoStormBuildingRemovalDecision(
	now time.Time,
	settings autoStormSettings,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	buildingID State.BuildingInstanceID,
	metrics map[string]float64,
) *Decision {
	building, exists := castle.Layout.Objects[buildingID]
	if !exists || !building.Placed || autoStormBuildingQueued(castle, buildingID) {
		return nil
	}
	definition, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return nil
	}
	if definition.Storeable != nil && *definition.Storeable {
		return autoStormIntentDecision(now, metrics, fmt.Sprintf("Store layout blocker %s", definition.DisplayName), "building.store", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": buildingID,
		})
	}
	if definition.Movable == nil || *definition.Movable {
		if placement, found := Buildings.FindPlacement(castle, definition, catalog, buildingID); found &&
			(placement.GridX != building.GridX || placement.GridY != building.GridY || placement.Rotation != building.Rotation) {
			return autoStormIntentDecision(now, metrics, fmt.Sprintf("Stage layout blocker %s in free space", definition.DisplayName), "building.move", map[string]any{
				"castleId": castle.ID, "buildingInstanceId": buildingID,
				"x": placement.GridX, "y": placement.GridY, "rotation": placement.Rotation,
			})
		}
	}
	if settings.Build.AllowDemolition && definition.Destructable != nil && *definition.Destructable {
		return autoStormIntentDecision(now, metrics, fmt.Sprintf("Demolish unmanaged %s", definition.DisplayName), "building.demolish", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": buildingID,
		})
	}
	return nil
}

func autoStormTargetActionDecision(
	now time.Time,
	settings autoStormSettings,
	castle State.CastleState,
	action Buildings.TargetAction,
	metrics map[string]float64,
) *Decision {
	arguments := map[string]any{"castleId": castle.ID}
	switch action.Intent {
	case "building.construct", "building.place":
		if action.Placement == nil {
			return nil
		}
		arguments["definitionId"] = action.Definition.ID
		arguments["x"], arguments["y"], arguments["rotation"] = action.Placement.GridX, action.Placement.GridY, action.Placement.Rotation
		arguments["resourceReserves"], arguments["allowPremium"] = settings.Build.ResourceReserves, settings.Build.AllowPremium
	case "building.move":
		if action.Placement == nil || action.BuildingInstanceID <= 0 {
			return nil
		}
		arguments["buildingInstanceId"] = action.BuildingInstanceID
		arguments["x"], arguments["y"], arguments["rotation"] = action.Placement.GridX, action.Placement.GridY, action.Placement.Rotation
	case "building.upgrade":
		if action.BuildingInstanceID <= 0 {
			return nil
		}
		arguments["buildingInstanceId"] = action.BuildingInstanceID
		arguments["resourceReserves"], arguments["allowPremium"] = settings.Build.ResourceReserves, settings.Build.AllowPremium
	default:
		return nil
	}
	actionLabel := action.Kind
	if actionLabel != "" {
		actionLabel = strings.ToUpper(actionLabel[:1]) + actionLabel[1:]
	}
	return autoStormIntentDecision(now, metrics, fmt.Sprintf("%s %s toward the captured Storm target", actionLabel, action.Definition.DisplayName), action.Intent, arguments)
}

func autoStormTargetTransportDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	action Buildings.TargetAction,
	metrics map[string]float64,
) (*Decision, string) {
	if snapshot.State.KingdomTransport.ObservedAt.IsZero() {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh resource logistics for the Storm target", "resource.logistics.refresh", map[string]any{}), ""
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[castle.KingdomID]
	if !observed || !unlock.Unlocked {
		return nil, "Kingdom resource transport to Storm is not unlocked"
	}
	if pending, found := pendingKingdomResourceTransport(snapshot.State, castle.KingdomID); found {
		if pending.RemainingSec > 0 && settings.Build.AllowTimeSkips {
			if key, _, reserve, found := autoStormTransportTimeSkip(snapshot.State, settings.Build.TimeSkipReserve, pending.RemainingSec); found {
				return autoStormIntentDecision(snapshot.Now, metrics, "Advance the pending Storm resource shipment", "resource.kingdom.skip", map[string]any{
					"targetKingdomId": castle.KingdomID, "timeSkipId": key, "minimumRemaining": reserve,
				}), ""
			}
		}
		if pending.RemainingSec <= 0 {
			return nil, "Waiting for the pending Storm resource shipment to settle"
		}
		return nil, fmt.Sprintf("Waiting for the pending Storm resource shipment (%d seconds)", pending.RemainingSec)
	}
	needs := make([]Buildings.CostStatus, 0)
	for _, cost := range action.Costs {
		if cost.Scope == GameData.BuildingCostCastleResource && cost.DefinitionID > 0 && cost.Shortfall > 0 {
			needs = append(needs, cost)
		}
	}
	if len(needs) == 0 {
		return nil, "The next target action is waiting for a non-transportable resource"
	}
	sources := make([]State.CastleState, 0)
	for _, source := range snapshot.State.Castles {
		if source.ID != castle.ID && source.KingdomID != castle.KingdomID {
			sources = append(sources, source)
		}
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left].ID < sources[right].ID })
	for _, source := range sources {
		goods := make([]map[string]any, 0, len(needs))
		for _, need := range needs {
			reserve := need.Reserve
			available := int64(math.Floor(max(float64(0), source.Resources[State.ResourceID(need.DefinitionID)].Amount-reserve)))
			shipment := int64(math.Ceil(need.Shortfall / autoStormTransportDelivery))
			amount := min(available, shipment)
			if amount > 0 {
				goods = append(goods, map[string]any{"resourceId": need.DefinitionID, "amount": amount})
			}
		}
		if len(goods) == 0 {
			continue
		}
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Transport resources from %s toward the Storm target", autoStormCastleName(source)), "resource.ship", map[string]any{
			"sourceCastleId": source.ID, "targetCastleId": castle.ID, "goods": goods,
		}), ""
	}
	return nil, "No owned castle can currently supply the missing Storm building resources"
}

func autoStormTransportTimeSkip(
	state State.GameState,
	reserves map[string]int64,
	remainingSec int,
) (string, State.CurrencyID, int64, bool) {
	options := []struct {
		Key      string
		Minutes  int
		Currency State.CurrencyID
	}{
		{"MS7", 1440, 1007}, {"MS6", 300, 1006}, {"MS5", 60, 1005}, {"MS4", 30, 1004},
		{"MS3", 10, 1003}, {"MS2", 5, 1002}, {"MS1", 1, 1001},
	}
	for _, option := range options {
		if option.Minutes*60 > remainingSec && option.Minutes != 1 {
			continue
		}
		reserve := autoStormTimeSkipReserve(reserves, option.Currency, option.Key)
		if state.Player.Currencies[option.Currency]-1 >= float64(reserve) {
			return option.Key, option.Currency, reserve, true
		}
	}
	return "", 0, 0, false
}

func autoStormTimeSkipReserve(reserves map[string]int64, currencyID State.CurrencyID, key string) int64 {
	reserve := reserves[strconv.FormatInt(int64(currencyID), 10)]
	if keyed := reserves[strings.ToUpper(key)]; keyed > reserve {
		reserve = keyed
	}
	return max(int64(0), reserve)
}

func autoStormApplyTimeSkipReserve(arguments map[string]any, intent string, reserves map[string]int64) {
	if intent == "building.skip_time" {
		minutes, _ := arguments["minutes"].(int)
		currency := map[int]State.CurrencyID{1: 1001, 5: 1002, 10: 1003, 30: 1004, 60: 1005, 300: 1006, 1440: 1007}[minutes]
		arguments["minimumRemaining"] = autoStormTimeSkipReserve(reserves, currency, fmt.Sprintf("MS%d", map[int]int{1: 1, 5: 2, 10: 3, 30: 4, 60: 5, 300: 6, 1440: 7}[minutes]))
	}
	if intent == "resource.kingdom.skip" {
		key, _ := arguments["timeSkipId"].(string)
		currency := map[string]State.CurrencyID{"MS1": 1001, "MS2": 1002, "MS3": 1003, "MS4": 1004, "MS5": 1005, "MS6": 1006, "MS7": 1007}[strings.ToUpper(key)]
		arguments["minimumRemaining"] = autoStormTimeSkipReserve(reserves, currency, key)
	}
}

func autoStormDecorationDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	metrics map[string]float64,
) (*Decision, bool, string, error) {
	if settings.Target == nil || settings.Target.Mode != Buildings.TargetCaptureModeBuildings || settings.DecorationPresetID == "" {
		return nil, true, "Captured Storm target state satisfied", nil
	}
	preset, found := autoStormDecorationPresetFromConfiguration(snapshot.Configuration.Sections["decorations.presets"], settings.DecorationPresetCastleID, settings.DecorationPresetID)
	if !found {
		return nil, false, "The selected decoration preset no longer exists", nil
	}
	if autoStormDecorationPresetSatisfied(castle, catalog, preset) {
		return nil, true, "Captured Storm building state and decoration preset satisfied", nil
	}
	items := make([]map[string]any, 0, len(preset.Items))
	for _, item := range preset.Items {
		items = append(items, map[string]any{"wid": item.WID, "x": item.X, "y": item.Y, "r": item.R, "layer": item.Layer})
	}
	return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Apply decoration preset %s to the completed Storm layout", preset.Name), "decoration.apply_preset", map[string]any{
		"castleId": castle.ID, "kingdomId": castle.KingdomID, "presetId": preset.ID, "items": items,
	}), false, "", nil
}

func autoStormDecorationPresetFromConfiguration(
	raw json.RawMessage,
	castleID State.CastleID,
	presetID string,
) (autoStormDecorationPreset, bool) {
	document := autoStormDecorationDocument{}
	if len(raw) == 0 || json.Unmarshal(raw, &document) != nil {
		return autoStormDecorationPreset{}, false
	}
	for _, preset := range document.Castles[strconv.FormatInt(int64(castleID), 10)] {
		if preset.ID == presetID {
			return preset, true
		}
	}
	return autoStormDecorationPreset{}, false
}

func autoStormDecorationPresetSatisfied(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	preset autoStormDecorationPreset,
) bool {
	matched := make([]bool, len(preset.Items))
	decorationCount := 0
	for _, building := range castle.Layout.Objects {
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || !building.Placed || !autoStormDecorationDefinition(definition) {
			continue
		}
		decorationCount++
		match := -1
		for index, item := range preset.Items {
			if !matched[index] && item.WID == building.DefinitionID && item.X == building.GridX && item.Y == building.GridY && item.R == building.Rotation {
				match = index
				break
			}
		}
		if match < 0 {
			return false
		}
		matched[match] = true
	}
	return decorationCount == len(preset.Items)
}

func autoStormDecorationDefinition(definition GameData.BuildingDefinition) bool {
	return strings.EqualFold(definition.GroundType, "DECO") || strings.EqualFold(definition.ShopCategory, "DECO") ||
		strings.EqualFold(definition.InternalName, "Deco")
}

func evaluateAutoStormShop(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, bool, string, error) {
	if len(settings.Aquamarine.Purchases) == 0 {
		return nil, true, "", nil
	}
	if settings.Aquamarine.Reserve < 0 {
		return nil, false, "Aquamarine reserve cannot be negative", nil
	}
	if snapshot.State.Inventory.ConstructionOffersCastleID != castle.ID ||
		snapshot.State.Inventory.ConstructionOffersKingdomID != castle.KingdomID ||
		snapshot.State.Inventory.ConstructionOffersObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionOffersObservedAt) >= 5*time.Minute {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh Luna package purchase counters", "shop.package.history", map[string]any{
			"castleId": castle.ID, "kingdomId": castle.KingdomID,
		}), false, "", nil
	}
	rules := append([]autoStormShopPurchase(nil), settings.Aquamarine.Purchases...)
	sort.SliceStable(rules, func(left, right int) bool {
		leftPriority, rightPriority := rules[left].Priority, rules[right].Priority
		if leftPriority <= 0 {
			leftPriority = int(^uint(0) >> 1)
		}
		if rightPriority <= 0 {
			rightPriority = int(^uint(0) >> 1)
		}
		return leftPriority < rightPriority
	})
	aquamarine := int64(math.Floor(castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount))
	spendable := aquamarine - settings.Aquamarine.Reserve
	purchases := make([]autoStormShopPurchaseLine, 0, len(rules))
	purchaseLabels := make([]string, 0, len(rules))
	totalCost := int64(0)
	includesUnlimited := false
	seenPackages := map[State.PackageID]struct{}{}
	for _, rule := range rules {
		if rule.PackageID <= 0 || (!rule.Unlimited && rule.TargetPurchases <= 0) {
			continue
		}
		if _, duplicate := seenPackages[rule.PackageID]; duplicate {
			continue
		}
		seenPackages[rule.PackageID] = struct{}{}
		item, found := snapshot.GameData.StormShopPackage(int64(rule.PackageID))
		if !found {
			return nil, false, fmt.Sprintf("Configured package %d is not sold by Luna", rule.PackageID), nil
		}
		purchased := snapshot.State.Inventory.ConstructionOffers[rule.PackageID]
		if item.Stock > 0 && purchased >= item.Stock {
			continue
		}
		remaining := int64(0)
		if !rule.Unlimited {
			if purchased >= rule.TargetPurchases {
				continue
			}
			remaining = rule.TargetPurchases - purchased
			metrics["shopPurchasesRemaining"] = float64(remaining)
		} else {
			metrics["shopPurchaseUnlimited"] = 1
			if item.Stock > 0 {
				remaining = item.Stock - purchased
				metrics["shopPurchasesRemaining"] = float64(remaining)
			}
		}
		affordable := spendable / item.AquamarinePrice
		if affordable <= 0 {
			if len(purchases) == 0 {
				return nil, false, fmt.Sprintf("Waiting for %d Aquamarine above reserve to buy %s", item.AquamarinePrice, item.Name), nil
			}
			break
		}
		desired := remaining
		if item.Stock > 0 {
			stockRemaining := item.Stock - purchased
			if desired == 0 || desired > stockRemaining {
				desired = stockRemaining
			}
		}
		amount := affordable
		if desired > 0 && amount > desired {
			amount = desired
		}
		if amount <= 0 {
			continue
		}
		cost := amount * item.AquamarinePrice
		purchases = append(purchases, autoStormShopPurchaseLine{ProductID: rule.PackageID, Amount: amount})
		purchaseLabels = append(purchaseLabels, fmt.Sprintf("%d x %s", amount, item.Name))
		totalCost += cost
		spendable -= cost
		if rule.Unlimited {
			includesUnlimited = true
		}
		if (desired > 0 && amount < desired) || (rule.Unlimited && item.Stock <= 0) {
			break
		}
	}
	if len(purchases) == 0 {
		return nil, true, "Aquamarine shop goals complete", nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"castleId": castle.ID, "purchases": purchases, "aquamarineReserve": settings.Aquamarine.Reserve,
	})
	followUp, _ := json.Marshal(map[string]any{"castleId": castle.ID, "kingdomId": castle.KingdomID})
	detail := fmt.Sprintf("Buy %s from Luna for %d Aquamarine", autoStormShopFriendlyList(purchaseLabels), totalCost)
	if includesUnlimited {
		detail += " (unlimited goal)"
	}
	return &Decision{
		Status: "ready", Detail: detail,
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request:  &Intent.Request{Name: "storm.shop.purchase", Arguments: arguments},
		FollowUp: &Intent.Request{Name: "shop.package.history", Arguments: followUp}, ReevaluateOnSuccess: true,
	}, false, "", nil
}

func autoStormShopFriendlyList(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return strings.Join(values[:len(values)-1], ", ") + " and " + values[len(values)-1]
}

func autoStormIslandReturnDecision(
	snapshot Snapshot,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, string) {
	readyCount := 0
	waitingCount := 0
	selectedKey := ""
	selected := State.StormIslandReturnState{}
	for key, operation := range snapshot.State.Storm.IslandReturns {
		if operation.SourceCastleID != castle.ID || operation.KingdomID != autoStormKingdomID {
			continue
		}
		switch operation.Status {
		case State.StormIslandReturnAwaitingReport:
			waitingCount++
		case State.StormIslandReturnReady:
			readyCount++
			selectedAt := selected.ReportedAt
			if selectedAt.IsZero() {
				selectedAt = selected.LaunchedAt
			}
			operationAt := operation.ReportedAt
			if operationAt.IsZero() {
				operationAt = operation.LaunchedAt
			}
			if selectedKey == "" || operationAt.Before(selectedAt) || operationAt.Equal(selectedAt) && key < selectedKey {
				selectedKey = key
				selected = operation
			}
		}
	}
	metrics["islandReturnsAwaitingReport"] = float64(waitingCount)
	metrics["islandReturnsReady"] = float64(readyCount)
	if selectedKey == "" {
		if waitingCount == 0 {
			return nil, ""
		}
		return nil, fmt.Sprintf("%d Storm island attack(s) awaiting a battle report", waitingCount)
	}
	returnAmounts := selected.UnitsToReturn()
	units := make([]map[string]any, 0, len(returnAmounts))
	var survivorTotal int64
	var returnTotal int64
	for _, amount := range selected.Survivors {
		if amount > 0 {
			survivorTotal += amount
		}
	}
	for _, unitID := range sortedAutoStormUnitIDs(returnAmounts) {
		amount := returnAmounts[unitID]
		if amount <= 0 {
			continue
		}
		units = append(units, map[string]any{"unitId": unitID, "amount": amount})
		returnTotal += amount
	}
	metrics["islandReportSurvivors"] = float64(survivorTotal)
	metrics["islandTroopsReturning"] = float64(returnTotal)
	metrics["islandTroopsLeftBehind"] = float64(selected.LeaveBehind)
	if len(units) == 0 {
		return nil, fmt.Sprintf("Storm island %d:%d has no report-confirmed surplus troop to return", selected.TargetX, selected.TargetY)
	}
	detail := fmt.Sprintf("Return %d report-confirmed surviving troops from Storm island %d:%d", returnTotal, selected.TargetX, selected.TargetY)
	if selected.LeaveBehind > 0 {
		detail += fmt.Sprintf(", leaving %d occupier", selected.LeaveBehind)
	}
	return autoStormIntentDecision(snapshot.Now, metrics, detail, "storm.island.return", map[string]any{
		"sourceCastleId": selected.SourceCastleID,
		"kingdomId":      selected.KingdomID,
		"islandX":        selected.TargetX,
		"islandY":        selected.TargetY,
		"islandObjectId": selected.IslandObjectID,
		"reportId":       selected.ReportID,
		"units":          units,
	}), ""
}

func evaluateAutoStormCombat(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, string, error) {
	if !settings.Forts.Enabled && !settings.Islands.Enabled {
		return nil, "", nil
	}
	if decision, pending := autoStormPendingTroopTransportDecision(snapshot, settings, castle, metrics); pending {
		if decision != nil {
			return decision, "", nil
		}
		return nil, "Waiting for the pending Storm troop transfer", nil
	}
	mapState := snapshot.State.Storm.Map
	if decision := autoStormFullMapScanDecision(snapshot, castle, metrics); decision != nil {
		return decision, "", nil
	}
	if mapState.LastCompletedAt.IsZero() || mapState.LastCompletedAt.Before(mapState.LastAttemptAt) {
		return nil, "The latest full Storm map sweep did not complete; the six-hour scan safety interval is still active", nil
	}
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return nil, "", err
	}
	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoStorm")
	if restricted && len(commanderIDs) == 0 {
		return nil, "No commanders are assigned to Auto Storm", nil
	}
	if !hasAvailableFeatureCommander(snapshot.State, commanderIDs, restricted, snapshot.Now) {
		return nil, "No assigned Auto Storm commander is currently available", nil
	}
	candidates := autoStormCombatCandidates(snapshot, settings, castle, mapState.LastCompletedAt)
	metrics["eligibleStormTargets"] = float64(len(candidates))
	metrics["minimumFortAttacksRemaining"] = float64(settings.Forts.MinimumWins)
	if len(candidates) == 0 {
		if nextOpportunityAt := autoStormNextOpportunityAt(snapshot, settings, castle); !nextOpportunityAt.IsZero() {
			return nil, fmt.Sprintf("Next learned Storm opportunity is ready at %s", nextOpportunityAt.Format(time.RFC3339)), nil
		}
		return nil, "No eligible Storm fort or resource island is available in the latest map scan", nil
	}
	if _, blocked := dailyAttackLimitAllowance(
		snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics,
	); blocked != nil {
		return nil, blocked.Detail, nil
	}
	waitingDetail := ""
	for _, candidate := range candidates {
		preset, found := AttackPresets.Find(document, candidate.PresetID)
		if !found {
			continue
		}
		defense := candidate.Defense
		required, valid := autoStormPresetRequirements(preset, defense)
		if !valid {
			continue
		}
		if autoStormTargetNeedsVerification(candidate.Observation, snapshot.Now) {
			metrics["stormTargetVerification"] = 1
			return autoStormIntentDecision(
				snapshot.Now,
				metrics,
				fmt.Sprintf("Refresh Storm target %d:%d before attack", candidate.Observation.X, candidate.Observation.Y),
				"storm.map.scan",
				map[string]any{
					"sourceCastleId": castle.ID,
					"targeted":       true,
					"bounds": State.StormMapBounds{
						X1: candidate.Observation.X, Y1: candidate.Observation.Y,
						X2: candidate.Observation.X, Y2: candidate.Observation.Y,
					},
				},
			), "", nil
		}
		shortages := autoStormUnitShortages(required, castle)
		if len(shortages) > 0 {
			decision, detail := autoStormTroopImportDecision(snapshot, settings, castle, shortages, metrics)
			if decision != nil {
				return decision, "", nil
			}
			if detail != "" {
				waitingDetail = detail
			}
			continue
		}
		arguments := map[string]any{
			"sourceCastleId": castle.ID, "kingdomId": castle.KingdomID,
			"targetTypeId": candidate.Observation.TypeID, "targetX": candidate.Observation.X, "targetY": candidate.Observation.Y,
			"stormIsleId": candidate.Observation.StormIsleID, "victoryCount": candidate.Observation.StormVictoryCount,
			"preset":             preset,
			"horseTravelBoostId": settings.HorseTravelBoostID,
			"dailyAttackLimit":   settings.DailyAttackLimit,
		}
		if candidate.Definition.Kind == GameData.StormIsleKindFort {
			arguments["minimumVictoryCount"] = settings.Forts.MinimumWins
		}
		if restricted {
			arguments["commanderIds"] = commanderIDs
		}
		if len(defense) > 0 {
			arguments["defenseUnits"] = defense
		}
		payload, _ := json.Marshal(arguments)
		label := fmt.Sprintf("level %d Storm fort", candidate.Observation.Level)
		if candidate.Definition.Kind == GameData.StormIsleKindIsland {
			label = fmt.Sprintf("%s %s island", candidate.Definition.Size, candidate.Definition.Resource)
		}
		detail := fmt.Sprintf("Attack %s at %d:%d with %s", label, candidate.Observation.X, candidate.Observation.Y, preset.Name)
		if candidate.Definition.Kind == GameData.StormIsleKindIsland {
			if len(defense) == 0 {
				detail += "; after the victory report, return the surviving army except one occupier"
			} else {
				detail += "; after the victory report, return the surviving attack army"
			}
		}
		return &Decision{
			Status: "ready", Detail: detail,
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
			Request: &Intent.Request{Name: "storm.attack", Arguments: payload}, ReevaluateOnSuccess: true,
		}, "", nil
	}
	if waitingDetail != "" {
		return nil, waitingDetail, nil
	}
	return nil, "Eligible Storm targets are waiting for a valid preset and stationed inventory", nil
}

func autoStormCombatCandidates(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	_ time.Time,
) []autoStormCombatCandidate {
	active := autoStormActiveTargets(snapshot.State, castle.ID, snapshot.Now)
	result := make([]autoStormCombatCandidate, 0)
	for _, scannedTarget := range snapshot.State.Storm.Map.Targets {
		target := autoStormLatestTargetObservation(snapshot.State, scannedTarget)
		if _, busy := active[fmt.Sprintf("%d:%d", target.X, target.Y)]; busy {
			continue
		}
		definition, found := snapshot.GameData.StormIsle(target.StormIsleID)
		if !found {
			continue
		}
		candidate, eligible := autoStormCandidateForTarget(settings, target, definition)
		if !eligible || autoStormTargetExpired(target, definition, snapshot.Now) || autoStormTargetReadyAt(target).After(snapshot.Now) {
			continue
		}
		result = append(result, candidate)
	}
	sort.Slice(result, func(left, right int) bool {
		leftRank := autoStormTargetPriorityRank(settings.TargetPriority, result[left].Definition)
		rightRank := autoStormTargetPriorityRank(settings.TargetPriority, result[right].Definition)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if result[left].Definition.Kind == GameData.StormIsleKindIsland && result[right].Definition.Kind == GameData.StormIsleKindIsland {
			leftExpiresAt := autoStormTargetExpiresAt(result[left].Observation, result[left].Definition)
			rightExpiresAt := autoStormTargetExpiresAt(result[right].Observation, result[right].Definition)
			if leftExpiresAt.IsZero() != rightExpiresAt.IsZero() {
				return !leftExpiresAt.IsZero()
			}
			if !leftExpiresAt.Equal(rightExpiresAt) {
				return leftExpiresAt.Before(rightExpiresAt)
			}
		}
		leftDistance, rightDistance := autoStormDistanceSquared(castle, result[left].Observation), autoStormDistanceSquared(castle, result[right].Observation)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if result[left].Observation.Y != result[right].Observation.Y {
			return result[left].Observation.Y < result[right].Observation.Y
		}
		return result[left].Observation.X < result[right].Observation.X
	})
	return result
}

func autoStormNextOpportunityAt(snapshot Snapshot, settings autoStormSettings, castle State.CastleState) time.Time {
	active := autoStormActiveTargets(snapshot.State, castle.ID, snapshot.Now)
	next := time.Time{}
	for _, scannedTarget := range snapshot.State.Storm.Map.Targets {
		target := autoStormLatestTargetObservation(snapshot.State, scannedTarget)
		if _, busy := active[fmt.Sprintf("%d:%d", target.X, target.Y)]; busy {
			continue
		}
		definition, found := snapshot.GameData.StormIsle(target.StormIsleID)
		if !found {
			continue
		}
		if _, eligible := autoStormCandidateForTarget(settings, target, definition); !eligible ||
			autoStormTargetExpired(target, definition, snapshot.Now) {
			continue
		}
		readyAt := autoStormTargetReadyAt(target)
		if !readyAt.After(snapshot.Now) {
			continue
		}
		if next.IsZero() || readyAt.Before(next) {
			next = readyAt
		}
	}
	return next
}

func autoStormCandidateForTarget(
	settings autoStormSettings,
	target State.MapObservation,
	definition GameData.StormIsleDefinition,
) (autoStormCombatCandidate, bool) {
	switch target.TypeID {
	case autoStormFortMapTypeID:
		if !settings.Forts.Enabled || definition.Kind != GameData.StormIsleKindFort ||
			!autoStormIntSelected(settings.Forts.Levels, definition.Level) {
			return autoStormCombatCandidate{}, false
		}
		if settings.Forts.MinimumWins > 0 && target.StormCooldownRemaining <= 0 {
			remaining, known := GameData.StormFortAttacksRemaining(definition, target.StormVictoryCount)
			if !known || remaining < settings.Forts.MinimumWins {
				return autoStormCombatCandidate{}, false
			}
		}
		return autoStormCombatCandidate{Observation: target, Definition: definition, PresetID: settings.Forts.PresetID}, true
	case autoStormIslandMapTypeID:
		if !settings.Islands.Enabled || definition.Kind != GameData.StormIsleKindIsland ||
			!autoStormStringSelected(settings.Islands.Resources, definition.Resource) ||
			!autoStormStringSelected(settings.Islands.Sizes, definition.Size) {
			return autoStormCombatCandidate{}, false
		}
		return autoStormCombatCandidate{
			Observation: target, Definition: definition, PresetID: settings.Islands.PresetID,
			Defense: append([]autoStormDefenseUnit(nil), settings.Islands.DefenseUnits...),
		}, true
	default:
		return autoStormCombatCandidate{}, false
	}
}

func autoStormIntSelected(values []int, candidate int) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func autoStormStringSelected(values []string, candidate string) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == candidate {
			return true
		}
	}
	return false
}

func defaultAutoStormTargetPriority() []string {
	return []string{
		"fort:80", "fort:70", "fort:60", "fort:50", "fort:40",
		"island:large", "island:small",
	}
}

func normalizeAutoStormTargetPriority(priority []string, legacyOrder string) []string {
	fallback := defaultAutoStormTargetPriority()
	if len(priority) == 0 && strings.EqualFold(strings.TrimSpace(legacyOrder), "islands_first") {
		fallback = []string{
			"island:large", "island:small",
			"fort:80", "fort:70", "fort:60", "fort:50", "fort:40",
		}
	}
	allowed := make(map[string]struct{}, len(fallback))
	for _, id := range defaultAutoStormTargetPriority() {
		allowed[id] = struct{}{}
	}
	result := make([]string, 0, len(allowed))
	seen := make(map[string]struct{}, len(allowed))
	appendID := func(raw string) {
		id := strings.ToLower(strings.TrimSpace(raw))
		if _, valid := allowed[id]; !valid {
			return
		}
		if _, duplicate := seen[id]; duplicate {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	for _, id := range priority {
		appendID(id)
	}
	for _, id := range fallback {
		appendID(id)
	}
	return result
}

func autoStormTargetPriorityRank(priority []string, definition GameData.StormIsleDefinition) int {
	if len(priority) == 0 {
		priority = defaultAutoStormTargetPriority()
	}
	id := ""
	switch definition.Kind {
	case GameData.StormIsleKindFort:
		id = autoStormPriorityFortPrefix + strconv.Itoa(definition.Level)
	case GameData.StormIsleKindIsland:
		id = autoStormPriorityIslandPrefix + strings.ToLower(strings.TrimSpace(definition.Size))
	}
	for rank, candidate := range priority {
		if candidate == id {
			return rank
		}
	}
	return len(priority)
}

func autoStormLatestTargetObservation(state State.GameState, scanned State.MapObservation) State.MapObservation {
	key := fmt.Sprintf("%d:%d", scanned.X, scanned.Y)
	if current, found := state.Map[scanned.KingdomID][key]; found && current.ObservedAt.After(scanned.ObservedAt) {
		return current
	}
	return scanned
}

func autoStormTargetNeedsVerification(target State.MapObservation, now time.Time) bool {
	return target.ObservedAt.IsZero() || now.After(target.ObservedAt.Add(autoStormTargetVerificationAge))
}

func autoStormPresetRequirements(preset AttackPresets.Preset, defense []autoStormDefenseUnit) (map[State.UnitID]int64, bool) {
	requested := map[State.UnitID]int64{}
	for _, wave := range preset.Waves {
		for _, lane := range []AttackPresets.Lane{wave.Left, wave.Middle, wave.Right} {
			for _, slots := range [][]AttackPresets.Slot{lane.Troops, lane.Tools} {
				for _, slot := range slots {
					if slot.ItemID != nil && *slot.ItemID > 0 && slot.Quantity > 0 {
						requested[State.UnitID(*slot.ItemID)] += slot.Quantity
					}
				}
			}
		}
	}
	for _, unit := range defense {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return nil, false
		}
		requested[unit.UnitID] += unit.Amount
	}
	if len(requested) == 0 {
		return nil, false
	}
	return requested, true
}

func autoStormUnitShortages(required map[State.UnitID]int64, castle State.CastleState) map[State.UnitID]int64 {
	shortages := map[State.UnitID]int64{}
	for unitID, amount := range required {
		if missing := amount - castle.Units.Stationed[unitID]; missing > 0 {
			shortages[unitID] = missing
		}
	}
	return shortages
}

func autoStormPendingTroopTransportDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, bool) {
	for _, pending := range snapshot.State.KingdomTransport.PendingUnits {
		if pending.KingdomID != castle.KingdomID {
			continue
		}
		remaining := pending.RemainingSec
		if !snapshot.State.KingdomTransport.ObservedAt.IsZero() && snapshot.Now.After(snapshot.State.KingdomTransport.ObservedAt) {
			remaining = max(0, remaining-int(snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt)/time.Second))
		}
		metrics["troopTransferRemainingSec"] = float64(remaining)
		metrics["troopTransferStacks"] = float64(len(pending.Units))
		focusArguments, _ := json.Marshal(map[string]any{"castleId": castle.ID})
		if remaining <= 0 {
			refreshArguments, _ := json.Marshal(map[string]any{})
			return &Decision{
				Status: "ready", Detail: "Confirm the arriving Storm troop transfer and refresh the castle inventory",
				NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
				Request:  &Intent.Request{Name: "troops.kingdom.refresh", Arguments: refreshArguments},
				FollowUp: &Intent.Request{Name: "game.focus_castle", Arguments: focusArguments}, ReevaluateOnSuccess: true,
			}, true
		}
		if settings.TroopImport.Enabled && settings.Build.AllowTimeSkips {
			if key, _, reserve, found := autoStormTransportTimeSkip(snapshot.State, settings.Build.TimeSkipReserve, remaining); found {
				skipArguments, _ := json.Marshal(map[string]any{
					"targetKingdomId": castle.KingdomID, "timeSkipId": key, "minimumRemaining": reserve,
				})
				return &Decision{
					Status: "ready", Detail: fmt.Sprintf("Apply %s to the pending Storm troop transfer", key),
					NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
					Request:  &Intent.Request{Name: "troops.kingdom.skip", Arguments: skipArguments},
					FollowUp: &Intent.Request{Name: "game.focus_castle", Arguments: focusArguments}, ReevaluateOnSuccess: true,
				}, true
			}
		}
		wait := time.Duration(remaining) * time.Second
		interval := policyInterval(settings.CheckIntervalSec, 30)
		if wait > interval {
			wait = interval
		}
		return &Decision{
			Status: "waiting", Detail: fmt.Sprintf("Waiting for the Storm troop transfer (%d seconds)", remaining),
			NextCheckAt: snapshot.Now.Add(wait), Metrics: metrics,
		}, true
	}
	return nil, false
}

func autoStormTroopImportDecision(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	shortages map[State.UnitID]int64,
	metrics map[string]float64,
) (*Decision, string) {
	missingTotal := int64(0)
	for _, amount := range shortages {
		missingTotal += amount
	}
	metrics["presetUnitsMissing"] = float64(missingTotal)
	metrics["presetUnitStacksMissing"] = float64(len(shortages))
	if !settings.TroopImport.Enabled {
		return nil, fmt.Sprintf("Storm is missing %d preset or defense units and troop import is disabled", missingTotal)
	}
	if len(settings.TroopImport.DonorCastleIDs) == 0 {
		return nil, "Storm troop import is enabled, but no donor castles are selected"
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[castle.KingdomID]
	if snapshot.State.KingdomTransport.ObservedAt.IsZero() || !observed {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh kingdom troop-transfer availability", "troops.kingdom.refresh", map[string]any{}), ""
	}
	if !unlock.Unlocked {
		return nil, "Kingdom troop transfer to Storm is not unlocked"
	}
	transferable := map[State.UnitID]int64{}
	missingTools := 0
	for _, unitID := range sortedAutoStormUnitIDs(shortages) {
		isTool, found := autoStormUnitIsTool(snapshot.GameData, unitID)
		if !found {
			return nil, fmt.Sprintf("Official unit definition %d is unavailable", unitID)
		}
		if isTool {
			missingTools++
			continue
		}
		transferable[unitID] = shortages[unitID]
	}
	metrics["presetToolStacksMissing"] = float64(missingTools)
	if len(transferable) == 0 {
		return nil, "Storm has the required troops but is missing preset tools, which kingdom troop transport cannot import"
	}
	for _, donorID := range settings.TroopImport.DonorCastleIDs {
		donor, found := snapshot.State.Castles[donorID]
		if !found || donor.ID == castle.ID || donor.KingdomID == castle.KingdomID {
			continue
		}
		units := make([]map[string]any, 0, len(transferable))
		transferTotal := int64(0)
		for _, unitID := range sortedAutoStormUnitIDs(transferable) {
			if len(units) >= autoStormMaximumTroopStacks {
				break
			}
			amount := min(transferable[unitID], donor.Units.Stationed[unitID])
			if amount <= 0 {
				continue
			}
			units = append(units, map[string]any{"unitId": unitID, "amount": amount})
			transferTotal += amount
		}
		if len(units) == 0 {
			continue
		}
		metrics["troopsSelectedForImport"] = float64(transferTotal)
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Import %d required troops from %s", transferTotal, autoStormCastleName(donor)), "troops.kingdom.ship", map[string]any{
			"sourceCastleId": donor.ID, "targetCastleId": castle.ID, "targetKingdomId": castle.KingdomID, "units": units,
		}), ""
	}
	if missingTools > 0 {
		return nil, fmt.Sprintf("Selected donor castles cannot supply the missing Storm troops; %d preset tool stack(s) are also missing", missingTools)
	}
	return nil, "Selected donor castles cannot currently supply the missing Storm troops"
}

func autoStormUnitIsTool(gameData *GameData.Store, unitID State.UnitID) (bool, bool) {
	if gameData == nil || unitID <= 0 {
		return false, false
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return false, false
	}
	raw, found := catalog.Find(strconv.FormatInt(int64(unitID), 10))
	if !found {
		return false, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return false, false
	}
	return GameData.IsToolRecord(record), true
}

func sortedAutoStormUnitIDs(values map[State.UnitID]int64) []State.UnitID {
	result := make([]State.UnitID, 0, len(values))
	for unitID := range values {
		result = append(result, unitID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func autoStormActiveTargets(state State.GameState, castleID State.CastleID, now time.Time) map[string]struct{} {
	result := map[string]struct{}{}
	for _, movement := range state.Movements {
		if movement.Direction != 0 || movement.SourceCastleID != castleID ||
			(movement.TargetTypeID != autoStormIslandMapTypeID && movement.TargetTypeID != autoStormFortMapTypeID) {
			continue
		}
		if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() && !movement.ReturnsAt.After(now) {
			continue
		}
		result[fmt.Sprintf("%d:%d", movement.TargetX, movement.TargetY)] = struct{}{}
	}
	return result
}

func autoStormTargetReadyAt(target State.MapObservation) time.Time {
	if !target.StormReadyAt.IsZero() {
		return target.StormReadyAt
	}
	if target.ObservedAt.IsZero() {
		return time.Time{}
	}
	readyAt := target.ObservedAt
	if target.StormCooldownRemaining > 0 &&
		(target.TypeID == autoStormFortMapTypeID || target.TypeID == autoStormIslandMapTypeID && target.OwnerID > 0) {
		readyAt = readyAt.Add(time.Duration(target.StormCooldownRemaining) * time.Second)
	}
	return readyAt
}

func autoStormTargetExpiresAt(target State.MapObservation, definition GameData.StormIsleDefinition) time.Time {
	if !target.StormExpiresAt.IsZero() {
		return target.StormExpiresAt
	}
	if target.TypeID != autoStormIslandMapTypeID || target.ObservedAt.IsZero() {
		return time.Time{}
	}
	if target.OwnerID <= 0 && target.StormCooldownRemaining > 0 {
		return target.ObservedAt.Add(time.Duration(target.StormCooldownRemaining) * time.Second)
	}
	readyAt := autoStormTargetReadyAt(target)
	if target.OwnerID > 0 && !readyAt.IsZero() && definition.GlobalCooldownSec > 0 {
		return readyAt.Add(time.Duration(definition.GlobalCooldownSec) * time.Second)
	}
	return time.Time{}
}

func autoStormTargetExpired(target State.MapObservation, definition GameData.StormIsleDefinition, now time.Time) bool {
	expiresAt := autoStormTargetExpiresAt(target, definition)
	return !expiresAt.IsZero() && !expiresAt.After(now)
}

func autoStormDistanceSquared(castle State.CastleState, target State.MapObservation) int {
	x, y := target.X-castle.X, target.Y-castle.Y
	return x*x + y*y
}

func autoStormBuilding(castle State.CastleState, id State.BuildingInstanceID) (State.Building, bool) {
	if building, found := castle.Layout.Objects[id]; found {
		return building, true
	}
	if building, found := castle.Layout.Fixed[id]; found {
		return building, true
	}
	building, found := castle.Buildings[id]
	return building, found
}

func autoStormBuildingQueued(castle State.CastleState, id State.BuildingInstanceID) bool {
	for _, slot := range castle.BuildingQueue.Slots {
		if slot.Status == State.BuildingQueueSlotOccupied && slot.BuildingID == id {
			return true
		}
	}
	return false
}

func sortedAutoStormBuildingIDs(buildings map[State.BuildingInstanceID]State.Building) []State.BuildingInstanceID {
	result := make([]State.BuildingInstanceID, 0, len(buildings))
	for id := range buildings {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func autoStormIntentDecision(
	now time.Time,
	metrics map[string]float64,
	detail string,
	intentName string,
	argumentsValue map[string]any,
) *Decision {
	arguments, _ := json.Marshal(argumentsValue)
	return &Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: intentName, Arguments: arguments}, ReevaluateOnSuccess: true,
	}
}

func autoStormWaiting(now time.Time, detail string) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(30 * time.Second)}
}

func autoStormCastleName(castle State.CastleState) string {
	if name := strings.TrimSpace(castle.Name); name != "" {
		return name
	}
	return fmt.Sprintf("castle %d", castle.ID)
}
