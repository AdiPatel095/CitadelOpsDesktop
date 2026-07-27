package Automation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoStormMapScanBoundsStartAtSixFiftyCenter(t *testing.T) {
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 679, 596
	state.Castles[storm.ID] = storm

	bounds := autoStormMapScanBounds(state, storm)
	if bounds != (State.StormMapBounds{X1: 600, Y1: 600, X2: 700, Y2: 700}) {
		t.Fatalf("initial bounds = %#v", bounds)
	}
	state.Storm.Map = State.StormMapState{
		SourceCastleID: storm.ID,
		NextBounds:     State.StormMapBounds{X1: 0, Y1: 0, X2: 908, Y2: 807},
		Targets:        map[string]State.MapObservation{},
	}
	if next := autoStormMapScanBounds(state, storm); next != bounds {
		t.Fatalf("next scan bounds = %#v, want center %#v", next, bounds)
	}
}

func TestNormalizeAutoStormSettingsFixesMapRefreshAtTwoHours(t *testing.T) {
	settings := defaultAutoStormSettings()
	settings.MapRefreshIntervalSec = 300
	normalizeAutoStormSettings(&settings)
	if settings.MapRefreshIntervalSec != 7_200 {
		t.Fatalf("map refresh = %d, want 7200", settings.MapRefreshIntervalSec)
	}
}

func TestAutoStormExpansionUsesCapturedCoordinatesInsteadOfKingdomSpaceIDs(t *testing.T) {
	missing := []Buildings.TargetGround{{DefinitionID: 201, GridX: 220, GridY: 190, Direction: 3}}
	selected, found := autoStormGroundForExpansion(missing, []int64{4})
	if !found || selected != missing[0] {
		t.Fatalf("selected expansion = %#v, found=%t", selected, found)
	}
}

func TestAutoStormTargetTransportBundlesGoodsCapacityAndImmediateSkip(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	woodCapacity := float64(200)
	stoneCapacity := float64(500)
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources[3] = State.ResourceBalance{Amount: 100, Capacity: &woodCapacity}
	storm.Resources[4] = State.ResourceBalance{Amount: 50, Capacity: &stoneCapacity}
	donor := autoStormTestCastle(10, 0, "Donor")
	donor.Resources[3] = State.ResourceBalance{Amount: 1_000_000}
	donor.Resources[4] = State.ResourceBalance{Amount: 1_000_000}
	state.Castles[storm.ID] = storm
	state.Castles[donor.ID] = donor
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}
	state.Player.Currencies[1005] = 1
	settings := defaultAutoStormSettings()
	settings.Build.AllowResourceTransport = true
	settings.Build.AllowTimeSkips = true
	settings.Build.SourceResourceReserves = map[string]float64{"3": 100_000, "4": 200_000}
	action := Buildings.TargetAction{Costs: []Buildings.CostStatus{
		{Scope: GameData.BuildingCostCastleResource, DefinitionID: 3, Shortfall: 1_000},
		{Scope: GameData.BuildingCostCastleResource, DefinitionID: 4, Shortfall: 1_000},
	}}

	decision, detail := autoStormTargetTransportDecision(
		Snapshot{State: state, Now: now}, settings, storm, action, map[string]float64{},
	)
	if decision == nil || decision.Request == nil || decision.Request.Name != "resource.ship" || detail != "" {
		t.Fatalf("transport decision = %#v detail=%q", decision, detail)
	}
	var arguments struct {
		Goods []struct {
			ResourceID State.ResourceID `json:"resourceId"`
			Amount     int64            `json:"amount"`
		} `json:"goods"`
		WorkflowOwner    string `json:"workflowOwner"`
		TimeSkipID       string `json:"timeSkipId"`
		MinimumRemaining int64  `json:"minimumRemaining"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if len(arguments.Goods) != 2 || arguments.Goods[0].Amount != 111 || arguments.Goods[1].Amount != 500 {
		t.Fatalf("capacity-bounded multi-resource goods = %#v", arguments.Goods)
	}
	if arguments.WorkflowOwner != autoStormTransportOwner || arguments.TimeSkipID != "MS5" || arguments.MinimumRemaining != 0 {
		t.Fatalf("shipment workflow and immediate skip = %#v", arguments)
	}
}

func TestAutoStormFullMapAttemptUsesTwoHourSafetyInterval(t *testing.T) {
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 679, 596
	state.Castles[storm.ID] = storm
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true

	decision, detail, err := evaluateAutoStormCombat(Snapshot{State: state, Now: now}, settings, storm, map[string]float64{})
	if err != nil || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "storm.map.scan" {
		t.Fatalf("map scan decision = %#v detail=%q err=%v", decision, detail, err)
	}
	if want := now.Add(2 * time.Hour); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("failed-scan retry = %s, want %s", decision.NextCheckAt, want)
	}
	var request struct {
		FullMap bool                 `json:"fullMap"`
		Bounds  State.StormMapBounds `json:"bounds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if !request.FullMap || request.Bounds != (State.StormMapBounds{X1: 600, Y1: 600, X2: 700, Y2: 700}) {
		t.Fatalf("map scan request = %#v", request)
	}

	state.Storm.Map = State.StormMapState{
		SourceCastleID: storm.ID,
		LastAttemptAt:  now.Add(-time.Hour),
		Targets:        map[string]State.MapObservation{},
	}
	decision, detail, err = evaluateAutoStormCombat(Snapshot{State: state, Now: now}, settings, storm, map[string]float64{})
	if err != nil || decision != nil || !strings.Contains(detail, "two-hour scan safety interval") {
		t.Fatalf("incomplete-scan decision = %#v detail=%q err=%v", decision, detail, err)
	}
}

func TestAutoStormTroopImportUsesSelectedDonorsInOrder(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources[12] = State.ResourceBalance{Amount: GameData.StormTroopSupportMead}
	storm.FoodStateObservedAt = now
	first := autoStormTestCastle(10, 0, "First donor")
	second := autoStormTestCastle(20, 0, "Second donor")
	first.Units.Stationed[10] = 3
	second.Units.Stationed[10] = 20
	state.Castles[storm.ID] = storm
	state.Castles[first.ID] = first
	state.Castles[second.ID] = second
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}
	settings := defaultAutoStormSettings()
	settings.TroopImport = autoStormTroopImportSettings{Enabled: true, DonorCastleIDs: []State.CastleID{first.ID, second.ID}}

	decision, detail := autoStormTroopImportDecision(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[State.UnitID]int64{10: 8}, map[string]float64{})
	if decision == nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.ship" || detail != "" {
		t.Fatalf("troop import decision = %#v detail=%q", decision, detail)
	}
	var arguments struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
		Units          []struct {
			UnitID State.UnitID `json:"unitId"`
			Amount int64        `json:"amount"`
		} `json:"units"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.SourceCastleID != first.ID || len(arguments.Units) != 1 || arguments.Units[0].UnitID != 10 || arguments.Units[0].Amount != 3 {
		t.Fatalf("troop import arguments = %#v", arguments)
	}
}

func TestAutoStormTroopImportWaitsForFiftyThousandMead(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources[12] = State.ResourceBalance{Amount: GameData.StormTroopSupportMead - 1}
	storm.FoodStateObservedAt = now
	donor := autoStormTestCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	state.Castles[storm.ID] = storm
	state.Castles[donor.ID] = donor
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}
	settings := defaultAutoStormSettings()
	settings.TroopImport = autoStormTroopImportSettings{Enabled: true, DonorCastleIDs: []State.CastleID{donor.ID}}
	metrics := map[string]float64{}

	decision, detail := autoStormTroopImportDecision(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[State.UnitID]int64{10: 8}, metrics)
	if decision != nil || !strings.Contains(detail, "waiting for Auto Food to reach 50000") {
		t.Fatalf("troop import below Mead floor = %#v detail=%q", decision, detail)
	}
	if metrics["stormMead"] != GameData.StormTroopSupportMead-1 {
		t.Fatalf("Storm Mead metric = %v", metrics["stormMead"])
	}
}

func TestAutoStormIslandLaunchWaitsForReportBeforeChoosingOccupier(t *testing.T) {
	now := time.Date(2026, time.July, 15, 18, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 100, 100
	storm.Units.Stationed[10] = 5
	storm.Units.Stationed[11] = 100
	storm.Units.Stationed[12] = 2
	state.Castles[storm.ID] = storm
	state.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	target := State.MapObservation{
		KingdomID: 4, X: 101, Y: 101, TypeID: autoStormIslandMapTypeID, OwnerID: -403,
		ObjectID: 777, StormIsleID: 4, StormReadyAt: now, StormExpiresAt: now.Add(time.Hour), ObservedAt: now,
	}
	state.Storm.Map = State.StormMapState{
		SourceCastleID: storm.ID, LastAttemptAt: now, LastCompletedAt: now,
		Targets: map[string]State.MapObservation{"101:101": target},
	}
	settings := defaultAutoStormSettings()
	settings.Islands.Enabled = true
	settings.Islands.PresetID = "island"
	presets := json.RawMessage(`{"version":1,"presets":[{"id":"island","name":"Island","waves":[{"L":{"troops":[],"tools":[]},"M":{"troops":[{"itemId":10,"quantity":5}],"tools":[]},"R":{"troops":[],"tools":[]}}]}]}`)

	decision, detail, err := evaluateAutoStormCombat(Snapshot{
		State: state,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			AttackPresets.ConfigurationSection: presets,
		}},
		GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[string]float64{})
	if err != nil || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "storm.attack" {
		t.Fatalf("island attack decision = %#v detail=%q err=%v", decision, detail, err)
	}
	var arguments struct {
		DefenseUnits []autoStormDefenseUnit `json:"defenseUnits"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if len(arguments.DefenseUnits) != 0 {
		t.Fatalf("launch-time occupation defense = %#v, want report-gated selection", arguments.DefenseUnits)
	}
	if !strings.Contains(decision.Detail, "victory report") {
		t.Fatalf("island decision detail = %q", decision.Detail)
	}
}

func TestAutoStormIslandReturnUsesReportConfirmedSurvivors(t *testing.T) {
	now := time.Date(2026, time.July, 15, 18, 5, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	state.Castles[storm.ID] = storm
	key := State.StormIslandReturnKey(4, 101, 102)
	state.Storm.IslandReturns[key] = State.StormIslandReturnState{
		KingdomID: 4, SourceCastleID: storm.ID, TargetX: 101, TargetY: 102,
		IslandObjectID: 777, ReportID: 202, Status: State.StormIslandReturnReady, LeaveBehind: 1,
		Survivors: map[State.UnitID]int64{10: 4, 12: 5}, LaunchedAt: now.Add(-time.Minute), ReportedAt: now,
	}
	metrics := map[string]float64{}
	decision, detail := autoStormIslandReturnDecision(Snapshot{State: state, Now: now}, storm, metrics)
	if detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "storm.island.return" {
		t.Fatalf("island return decision = %#v detail=%q", decision, detail)
	}
	var arguments struct {
		Units []autoStormDefenseUnit `json:"units"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if len(arguments.Units) != 2 || arguments.Units[0] != (autoStormDefenseUnit{UnitID: 10, Amount: 4}) ||
		arguments.Units[1] != (autoStormDefenseUnit{UnitID: 12, Amount: 4}) {
		t.Fatalf("report-confirmed return units = %#v", arguments.Units)
	}
	if metrics["islandReportSurvivors"] != 9 || metrics["islandTroopsReturning"] != 8 || metrics["islandTroopsLeftBehind"] != 1 {
		t.Fatalf("island return metrics = %#v", metrics)
	}
}

func TestAutoStormFortCandidatesEnforceMinimumAttacksRemaining(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 100, 100
	state.Castles[storm.ID] = storm
	target := State.MapObservation{
		KingdomID: 4, X: 101, Y: 101, TypeID: autoStormFortMapTypeID,
		StormIsleID: 7, StormVictoryCount: 7, ObservedAt: now,
	}
	state.Map[4] = map[string]State.MapObservation{"101:101": target}
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true
	settings.Forts.MinimumWins = 4
	state.Storm.Map = State.StormMapState{
		SourceCastleID: storm.ID, LastAttemptAt: now, LastCompletedAt: now,
		Targets: map[string]State.MapObservation{"101:101": target},
	}
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}

	if candidates := autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 0 {
		t.Fatalf("candidates below minimum attacks remaining = %#v", candidates)
	}
	target.StormVictoryCount = 6
	state.Map[4]["101:101"] = target
	state.Storm.Map.Targets["101:101"] = target
	snapshot.State = state
	if candidates := autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 1 {
		t.Fatalf("candidates at minimum attacks remaining = %#v", candidates)
	}
}

func TestAutoStormCandidatesFollowConfiguredTargetPriority(t *testing.T) {
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 100, 100
	state.Castles[storm.ID] = storm
	state.Storm.Map.Targets = map[string]State.MapObservation{
		"200:200": {KingdomID: 4, X: 200, Y: 200, TypeID: autoStormFortMapTypeID, StormIsleID: 10, ObservedAt: now},
		"101:101": {KingdomID: 4, X: 101, Y: 101, TypeID: autoStormFortMapTypeID, StormIsleID: 9, ObservedAt: now},
		"102:102": {KingdomID: 4, X: 102, Y: 102, TypeID: autoStormFortMapTypeID, StormIsleID: 8, ObservedAt: now},
		"103:103": {KingdomID: 4, X: 103, Y: 103, TypeID: autoStormIslandMapTypeID, OwnerID: -403, StormIsleID: 1, ObservedAt: now},
		"104:104": {KingdomID: 4, X: 104, Y: 104, TypeID: autoStormIslandMapTypeID, OwnerID: -403, StormIsleID: 4, ObservedAt: now},
	}
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true
	settings.Forts.Levels = []int{60, 70, 80}
	settings.Islands.Enabled = true
	settings.Islands.Resources = []string{"wood"}
	settings.Islands.Sizes = []string{"large", "small"}
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}

	assertStormCandidateOrder(t, autoStormCombatCandidates(snapshot, settings, storm, now), []int64{10, 9, 8, 1, 4})

	settings.TargetPriority = []string{
		"island:small", "fort:60", "island:large", "fort:80", "fort:70", "fort:50", "fort:40",
	}
	assertStormCandidateOrder(t, autoStormCombatCandidates(snapshot, settings, storm, now), []int64{4, 8, 1, 10, 9})
}

func TestAutoStormCandidatesUseNewerLiveCooldown(t *testing.T) {
	now := time.Date(2026, time.July, 21, 19, 18, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	state.Castles[storm.ID] = storm
	state.Storm.Map.Targets["612:667"] = State.MapObservation{
		KingdomID: 4, X: 612, Y: 667, TypeID: autoStormFortMapTypeID, StormIsleID: 10,
		ObservedAt: now.Add(-2 * time.Hour), StormReadyAt: now.Add(-time.Hour),
	}
	readyAt := now.Add(10 * time.Hour)
	state.Map[4] = map[string]State.MapObservation{
		"612:667": {
			KingdomID: 4, X: 612, Y: 667, TypeID: autoStormFortMapTypeID, StormIsleID: 8,
			StormCooldownRemaining: 36_000, StormReadyAt: readyAt, ObservedAt: now,
		},
	}
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true
	settings.Forts.Levels = []int{60, 80}
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}

	if candidates := autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 0 {
		t.Fatalf("newer live cooldown produced candidates: %#v", candidates)
	}
	if next := autoStormNextOpportunityAt(snapshot, settings, storm); !next.Equal(readyAt) {
		t.Fatalf("next opportunity = %s, want newer live readyAt %s", next, readyAt)
	}
}

func TestNormalizeAutoStormTargetPriorityMigratesLegacyOrder(t *testing.T) {
	got := normalizeAutoStormTargetPriority(nil, "islands_first")
	want := "island:large,island:small,fort:80,fort:70,fort:60,fort:50,fort:40"
	if strings.Join(got, ",") != want {
		t.Fatalf("legacy islands-first priority = %q, want %q", strings.Join(got, ","), want)
	}

	got = normalizeAutoStormTargetPriority([]string{"FORT:60", "fort:60", "unknown"}, "")
	want = "fort:60,fort:80,fort:70,fort:50,fort:40,island:large,island:small"
	if strings.Join(got, ",") != want {
		t.Fatalf("normalized priority = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestAutoStormCandidatesUseReadyAtAndIslandExpiryLabels(t *testing.T) {
	now := time.Date(2026, time.July, 15, 16, 20, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.X, storm.Y = 100, 100
	state.Castles[storm.ID] = storm
	unoccupied := State.MapObservation{
		KingdomID: 4, X: 101, Y: 101, TypeID: autoStormIslandMapTypeID, OwnerID: -403,
		StormIsleID: 4, StormCooldownRemaining: 3_600, StormReadyAt: now, StormExpiresAt: now.Add(time.Hour), ObservedAt: now,
	}
	occupied := State.MapObservation{
		KingdomID: 4, X: 102, Y: 102, TypeID: autoStormIslandMapTypeID, OwnerID: 99,
		StormIsleID: 4, StormCooldownRemaining: 120, StormReadyAt: now.Add(120 * time.Second),
		StormExpiresAt: now.Add((120 + 115_200) * time.Second), ObservedAt: now,
	}
	state.Storm.Map.Targets = map[string]State.MapObservation{"101:101": unoccupied, "102:102": occupied}
	settings := defaultAutoStormSettings()
	settings.Islands.Enabled = true
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}

	candidates := autoStormCombatCandidates(snapshot, settings, storm, now)
	if len(candidates) != 1 || candidates[0].Observation.X != unoccupied.X {
		t.Fatalf("ready island candidates = %#v", candidates)
	}
	if next := autoStormNextOpportunityAt(snapshot, settings, storm); !next.Equal(occupied.StormReadyAt) {
		t.Fatalf("next Storm opportunity = %s, want %s", next, occupied.StormReadyAt)
	}

	delete(state.Storm.Map.Targets, "101:101")
	snapshot.State = state
	snapshot.Now = occupied.StormReadyAt.Add(time.Second)
	candidates = autoStormCombatCandidates(snapshot, settings, storm, now)
	if len(candidates) != 1 || candidates[0].Observation.X != occupied.X {
		t.Fatalf("released island candidates = %#v", candidates)
	}

	occupied.OwnerID = -403
	occupied.StormReadyAt = now
	occupied.StormExpiresAt = now
	state.Storm.Map.Targets["102:102"] = occupied
	snapshot.State = state
	snapshot.Now = now.Add(time.Second)
	if candidates = autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 0 {
		t.Fatalf("expired island candidates = %#v", candidates)
	}
}

func TestAutoStormFortReadyAtTriggersAuthoritativeVictoryRefresh(t *testing.T) {
	now := time.Date(2026, time.July, 15, 16, 20, 0, 0, time.UTC)
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	state.Castles[storm.ID] = storm
	target := State.MapObservation{
		KingdomID: 4, X: 101, Y: 101, TypeID: autoStormFortMapTypeID, StormIsleID: 7,
		StormVictoryCount: 0, StormCooldownRemaining: 60, StormReadyAt: now.Add(time.Minute), ObservedAt: now,
	}
	state.Storm.Map.Targets = map[string]State.MapObservation{"101:101": target}
	settings := defaultAutoStormSettings()
	settings.Forts.Enabled = true
	settings.Forts.MinimumWins = 5
	snapshot := Snapshot{State: state, GameData: autoStormTestGameData(t), Now: now}

	if candidates := autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 0 {
		t.Fatalf("cooling fort candidates = %#v", candidates)
	}
	if next := autoStormNextOpportunityAt(snapshot, settings, storm); !next.Equal(target.StormReadyAt) {
		t.Fatalf("fort next readyAt = %s, want %s", next, target.StormReadyAt)
	}
	snapshot.Now = target.StormReadyAt
	if candidates := autoStormCombatCandidates(snapshot, settings, storm, now); len(candidates) != 1 {
		t.Fatalf("fort readyAt verification candidates = %#v", candidates)
	}
}

func TestAutoStormShopUnlimitedKeepsBuying(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources = map[State.ResourceID]State.ResourceBalance{
		GameData.StormAquamarineID: {Amount: 100_000},
	}
	state.Castles[storm.ID] = storm
	state.Inventory.ConstructionOffersCastleID = storm.ID
	state.Inventory.ConstructionOffersKingdomID = storm.KingdomID
	state.Inventory.ConstructionOffersObservedAt = now
	state.Inventory.ConstructionOffers[245] = 12
	settings := defaultAutoStormSettings()
	settings.Aquamarine.Purchases = []autoStormShopPurchase{{
		PackageID: 245, TargetPurchases: 1, Unlimited: true, Priority: 1,
	}}

	decision, complete, detail, err := evaluateAutoStormShop(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil {
		t.Fatalf("unlimited Luna decision = %#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	if decision.Request.Name != "storm.shop.purchase" || decision.Detail != "Buy 33 x War horn from Luna for 97680 Aquamarine (unlimited goal)" {
		t.Fatalf("unlimited Luna request = %#v detail=%q", decision.Request, decision.Detail)
	}
	var request struct {
		Purchases []autoStormShopPurchaseLine `json:"purchases"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Purchases) != 1 || request.Purchases[0].Amount != 33 {
		t.Fatalf("unlimited Luna purchases = %#v, want every affordable pack in one purchase", request.Purchases)
	}
}

func TestAutoStormShopBatchesRemainingTargetPurchases(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources = map[State.ResourceID]State.ResourceBalance{
		GameData.StormAquamarineID: {Amount: 100_000},
	}
	state.Castles[storm.ID] = storm
	state.Inventory.ConstructionOffersCastleID = storm.ID
	state.Inventory.ConstructionOffersKingdomID = storm.KingdomID
	state.Inventory.ConstructionOffersObservedAt = now
	state.Inventory.ConstructionOffers[245] = 2
	settings := defaultAutoStormSettings()
	settings.Aquamarine.Purchases = []autoStormShopPurchase{{
		PackageID: 245, TargetPurchases: 5, Priority: 1,
	}}

	decision, complete, detail, err := evaluateAutoStormShop(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil {
		t.Fatalf("batched Luna decision = %#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var request struct {
		Purchases []autoStormShopPurchaseLine `json:"purchases"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Purchases) != 1 || request.Purchases[0].Amount != 3 || decision.Detail != "Buy 3 x War horn from Luna for 8880 Aquamarine" {
		t.Fatalf("batched Luna request = %#v detail=%q", request, decision.Detail)
	}
}

func TestAutoStormShopGroupsDifferentProductsIntoOnePass(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources = map[State.ResourceID]State.ResourceBalance{
		GameData.StormAquamarineID: {Amount: 100_000},
	}
	state.Castles[storm.ID] = storm
	state.Inventory.ConstructionOffersCastleID = storm.ID
	state.Inventory.ConstructionOffersKingdomID = storm.KingdomID
	state.Inventory.ConstructionOffersObservedAt = now
	settings := defaultAutoStormSettings()
	settings.Aquamarine.Purchases = []autoStormShopPurchase{
		{PackageID: 245, TargetPurchases: 3, Priority: 1},
		{PackageID: 3119, TargetPurchases: 2, Priority: 2},
	}

	decision, complete, detail, err := evaluateAutoStormShop(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil {
		t.Fatalf("multi-product Luna decision = %#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var request struct {
		Purchases []autoStormShopPurchaseLine `json:"purchases"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	want := []autoStormShopPurchaseLine{{ProductID: 245, Amount: 3}, {ProductID: 3119, Amount: 2}}
	if !reflect.DeepEqual(request.Purchases, want) {
		t.Fatalf("multi-product Luna purchases = %#v, want %#v", request.Purchases, want)
	}
	if decision.Detail != "Buy 3 x War horn and 2 x Silver Coins from Luna for 28880 Aquamarine" {
		t.Fatalf("multi-product Luna detail = %q", decision.Detail)
	}
}

func TestAutoStormShopRunsBeforeCastleMaintenance(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources = map[State.ResourceID]State.ResourceBalance{
		GameData.StormAquamarineID: {Amount: 100_000},
	}
	state.Castles[storm.ID] = storm
	state.Inventory.ConstructionOffersCastleID = storm.ID
	state.Inventory.ConstructionOffersKingdomID = storm.KingdomID
	state.Inventory.ConstructionOffersObservedAt = now
	settings := defaultAutoStormSettings()
	settings.Harbor.Enabled = true
	settings.Aquamarine.Purchases = []autoStormShopPurchase{{
		PackageID: 245, TargetPurchases: 1, Priority: 1,
	}}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewAutoStormPolicy().Evaluate(t.Context(), Snapshot{
		State: state,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoStormSection: rawSettings,
		}},
		GameData: autoStormTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "storm.shop.purchase" {
		t.Fatalf("Auto Storm decision = %#v, want Luna purchase before building refresh", decision)
	}
}

func TestAutoStormShopUnlimitedStopsAtPackageStock(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	storm := autoStormTestCastle(40, 4, "Storm")
	storm.Resources = map[State.ResourceID]State.ResourceBalance{
		GameData.StormAquamarineID: {Amount: 100_000},
	}
	state.Castles[storm.ID] = storm
	state.Inventory.ConstructionOffersCastleID = storm.ID
	state.Inventory.ConstructionOffersKingdomID = storm.KingdomID
	state.Inventory.ConstructionOffersObservedAt = now
	state.Inventory.ConstructionOffers[3119] = 3
	settings := defaultAutoStormSettings()
	settings.Aquamarine.Purchases = []autoStormShopPurchase{{
		PackageID: 3119, TargetPurchases: 1, Unlimited: true, Priority: 1,
	}}

	decision, complete, detail, err := evaluateAutoStormShop(Snapshot{
		State: state, GameData: autoStormTestGameData(t), Now: now,
	}, settings, storm, map[string]float64{})
	if err != nil || !complete || decision != nil || detail != "Aquamarine shop goals complete" {
		t.Fatalf("stock-limited Luna decision = %#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
}

func autoStormTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10},{"wodID":11,"slotTypes":"tool"},{"wodID":12}],
		"resources":[{"resourceID":12,"JSONKey":"MEAD"}],
		"isles":[
			{"IsleID":1,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400},
			{"IsleID":4,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400},
			{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"},
			{"IsleID":8,"type":"DUNGEON","dungeonlevel":60,"maxCountVictories":10,"countVictories":"10#11#12#13#14#15#16#17#18#19"},
			{"IsleID":9,"type":"DUNGEON","dungeonlevel":70,"maxCountVictories":10,"countVictories":"20#21#22#23#24#25#26#27#28#29"},
			{"IsleID":10,"type":"DUNGEON","dungeonlevel":80,"maxCountVictories":10,"countVictories":"30#31#32#33#34#35#36#37#38#39"}
		],
		"packages":[
			{"packageID":245,"comment1":"War horn","comment2":"Luna's trade boat","packageType":"tool","packagePriceAquamarine":2960},
			{"packageID":3119,"comment1":"Silver Coins","comment2":"Luna's trade boat","packageType":"currency","packagePriceAquamarine":10000,"stock":3}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertStormCandidateOrder(t *testing.T, candidates []autoStormCombatCandidate, want []int64) {
	t.Helper()
	if len(candidates) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %#v", len(candidates), len(want), candidates)
	}
	for index, candidate := range candidates {
		if candidate.Observation.StormIsleID != want[index] {
			t.Fatalf("candidate %d = isle %d, want isle %d: %#v", index, candidate.Observation.StormIsleID, want[index], candidates)
		}
	}
}

func autoStormTestCastle(id State.CastleID, kingdom State.KingdomID, name string) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, Name: name,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{}, Traveling: map[State.UnitID]int64{},
			Hospital: map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{}, Total: map[State.UnitID]int64{},
		},
	}
}
