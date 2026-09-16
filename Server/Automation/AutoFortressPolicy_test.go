package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoFortressPurchaseUsesCheapestDirewolfTierBeforeNextTier(t *testing.T) {
	gameData := autoFortressTestGameData(t)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	main := State.CastleState{ID: 1, KingdomID: 0, SlotType: 1}
	gameState.Castles[1] = main
	gameState.Player.Currencies[37] = 1_000_000
	gameState.EventScores.ShopByPackage[3857] = State.EventShopRoute{EventID: 72, RemainingSec: 3600, ObservedAt: now}
	gameState.EventScores.ShopByPackage[3858] = State.EventShopRoute{EventID: 72, RemainingSec: 3600, ObservedAt: now}
	gameState.Inventory.ConstructionOffersCastleID = 1
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	gameState.Inventory.ConstructionOffers = map[State.PackageID]int64{}
	settings := defaultAutoFortressSettings()
	settings.DirewolfPurchaseLimit = 6000
	snapshot := Snapshot{State: gameState, GameData: gameData, Now: now}

	decision, detail := evaluateAutoFortressPurchase(snapshot, settings, main, map[string]float64{})
	if detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "autoBuyer.package.purchase" {
		t.Fatalf("first Direwolf purchase = %#v detail=%q", decision, detail)
	}
	var first struct {
		PackageID int64 `json:"packageId"`
		Amount    int64 `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &first); err != nil {
		t.Fatal(err)
	}
	if first.PackageID != 3857 || first.Amount != 50 {
		t.Fatalf("first Direwolf tier request = %#v", first)
	}

	gameState.Inventory.ConstructionOffers[3857] = 50
	snapshot.State = gameState
	decision, detail = evaluateAutoFortressPurchase(snapshot, settings, main, map[string]float64{})
	if detail != "" || decision == nil || decision.Request == nil {
		t.Fatalf("second Direwolf purchase = %#v detail=%q", decision, detail)
	}
	var second struct {
		PackageID int64 `json:"packageId"`
		Amount    int64 `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &second); err != nil {
		t.Fatal(err)
	}
	if second.PackageID != 3858 || second.Amount != 10 {
		t.Fatalf("second Direwolf tier request = %#v", second)
	}
}

func TestAutoFortressPolicyLaunchesOnlyWithMaxedRelicTwoCommander(t *testing.T) {
	gameData := autoFortressTestGameData(t)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 1, SlotType: 12, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 10_000}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 5001}}
	gameState.Inventory.Equipment[5001] = State.EquipmentInstance{
		ID: 5001, Slot: 1, RarityID: 5, Effects: State.EquipmentEffects{
			{DefinitionID: 2106, Values: []float64{100}},
			{DefinitionID: 1, Values: []float64{1}},
			{DefinitionID: 2, Values: []float64{1}},
			{DefinitionID: 3, Values: []float64{1}},
		},
	}
	gameState.Map[1] = map[string]State.MapObservation{
		"101:100": {KingdomID: 1, X: 101, Y: 100, TypeID: State.MapTypeKingdomFortress, Level: 45, ObservedAt: now},
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoFortressSection: json.RawMessage(`{
			"version":1,"checkIntervalSec":5,"mapRefreshIntervalSec":1800,
			"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100,"direwolfPurchaseLimit":0,
			"kingdoms":{"1":{"enabled":true}}
		}`),
	}}
	decision, err := NewAutoFortressPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Configuration: configuration, GameData: gameData, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "fortress.attack" {
		t.Fatalf("fortress launch decision = %#v err=%v", decision, err)
	}

	item := gameState.Inventory.Equipment[5001]
	item.RarityID = 4
	gameState.Inventory.Equipment[5001] = item
	decision, err = NewAutoFortressPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Configuration: configuration, GameData: gameData, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("non-Relic-2.0 commander was accepted: %#v err=%v", decision, err)
	}
}

func TestAutoFortressTracksFarTargetsAndSchedulesTheirExpectedReadyTime(t *testing.T) {
	now := time.Date(2026, time.September, 2, 16, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{ID: 10, KingdomID: 1, SlotType: 12, X: 100, Y: 100}
	gameState.Map[1] = map[string]State.MapObservation{
		"1000:1000": {
			KingdomID: 1, X: 1000, Y: 1000, TypeID: State.MapTypeKingdomFortress,
			TowerCooldownRemaining: 600, ObservedAt: now,
		},
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoFortressSection: json.RawMessage(`{
			"version":1,"checkIntervalSec":5,"mapRefreshIntervalSec":3600,
			"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100,"direwolfPurchaseLimit":0,
			"kingdoms":{"1":{"enabled":true}}
		}`),
	}}
	policy := NewAutoFortressPolicy()
	policy.markFullScanRequested(1, now)
	decision, err := policy.Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantReadyAt := now.Add(10 * time.Minute)
	if decision.Request != nil || decision.Status != "idle" || !decision.NextCheckAt.Equal(wantReadyAt) ||
		decision.Metrics["knownFortressesKingdom1"] != 1 ||
		int64(decision.Metrics["nextReadyAtKingdom1Unix"]) != wantReadyAt.Unix() ||
		!strings.Contains(decision.Detail, "targeted cooldown check") {
		t.Fatalf("far Fortress ready-time decision = %#v", decision)
	}
}

func TestAutoFortressTransportAllocatesAllAvailableDirewolvesToOneDestination(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	main := State.CastleState{
		ID: 1, KingdomID: 0, SlotType: 1, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 900}},
	}
	target := State.CastleState{
		ID: 10, KingdomID: 1, SlotType: 12, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 100}},
	}
	gameState.Castles[main.ID] = main
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}

	settings := defaultAutoFortressSettings()
	settings.Kingdoms["1"] = autoFortressKingdom{Enabled: true}
	decision := NewAutoFortressPolicy().autoFortressSupplyDecision(
		Snapshot{State: gameState, GameData: autoFortressTestGameData(t), Now: now},
		settings, []State.CastleState{target}, main, true, map[string]float64{}, map[string]string{},
	)
	if decision == nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.ship" {
		t.Fatalf("fortress troop transport = %#v", decision)
	}
	var request struct {
		SourceCastleID  State.CastleID  `json:"sourceCastleId"`
		TargetCastleID  State.CastleID  `json:"targetCastleId"`
		TargetKingdomID State.KingdomID `json:"targetKingdomId"`
		Units           []struct {
			UnitID int64 `json:"unitId"`
			Amount int64 `json:"amount"`
		} `json:"units"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != main.ID || request.TargetCastleID != target.ID ||
		request.TargetKingdomID != target.KingdomID || len(request.Units) != 1 ||
		request.Units[0].UnitID != GameData.DirewolfUnitID || request.Units[0].Amount != 900 {
		t.Fatalf("fortress troop transport request = %#v", request)
	}
}

func TestAutoFortressBalanceCountsInboundAndUsesStableKingdomTieBreak(t *testing.T) {
	destinations := []autoFortressSupplyDestination{
		{castle: State.CastleState{KingdomID: 2}, stationed: 1, committed: 1},
		{castle: State.CastleState{KingdomID: 1}, inbound: 0, committed: 0},
	}
	autoFortressBalanceAllocations(destinations, 4)
	allocations := map[State.KingdomID]int64{}
	for _, destination := range destinations {
		allocations[destination.castle.KingdomID] = destination.allocation
	}
	if allocations[1] != 3 || allocations[2] != 1 {
		t.Fatalf("stable four-unit allocation = %#v", allocations)
	}

	destinations = []autoFortressSupplyDestination{
		{castle: State.CastleState{KingdomID: 1}, stationed: 100, inbound: 200, committed: 300},
		{castle: State.CastleState{KingdomID: 2}, stationed: 200, inbound: 0, committed: 200},
	}
	autoFortressBalanceAllocations(destinations, 500)
	allocations = map[State.KingdomID]int64{}
	for _, destination := range destinations {
		allocations[destination.castle.KingdomID] = destination.allocation
	}
	if allocations[1] != 200 || allocations[2] != 300 {
		t.Fatalf("inbound-aware allocation = %#v", allocations)
	}
}

func TestAutoFortressTimeSkipUsesOfficialDurationsAndReserves(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 7
	state.Player.Currencies[1004] = 1
	state.Player.Currencies[1005] = 2
	state.Player.CurrencyObservations[1004] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	state.Player.CurrencyObservations[1005] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	snapshot := Snapshot{State: state, GameData: autoFortressTestGameData(t), Now: now}
	option := autoFortressTimeSkipOption(snapshot, map[string]int64{"MS5": 2}, 1_900)
	if option.WireKey != "MS4" || option.Seconds != 1_800 {
		t.Fatalf("official minimal-waste option = %#v", option)
	}
	state.Player.Currencies[1004] = 0
	snapshot.State = state
	if option = autoFortressTimeSkipOption(snapshot, map[string]int64{"MS5": 2}, 1_900); option.WireKey != "" {
		t.Fatalf("depleted/reserved skips should wait naturally: %#v", option)
	}
}

func TestAutoFortressDisabledDestinationOnlyReconcilesOwnedTransfer(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	target := State.CastleState{ID: 22, KingdomID: 2, SlotType: 12, UnitsObservedAt: now}
	state.Castles[target.ID] = target
	state.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: autoFortressTransportOwner, Status: "pending", KingdomID: 2,
		SourceCastleID: 1, TargetCastleID: target.ID,
		Units: []State.KingdomTransportUnit{{UnitID: GameData.DirewolfUnitID, Amount: 100}},
	}
	state.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 2, RemainingSec: 120, Units: []State.KingdomTransportUnit{{UnitID: GameData.DirewolfUnitID, Amount: 100}},
	}}
	settings := defaultAutoFortressSettings()
	settings.UseTimeSkips = true
	settings.Kingdoms["2"] = autoFortressKingdom{Enabled: false}
	details := map[string]string{}
	decision := NewAutoFortressPolicy().autoFortressSupplyDecision(
		Snapshot{State: state, GameData: autoFortressTestGameData(t), Now: now},
		settings, nil, State.CastleState{}, false, map[string]float64{}, details,
	)
	if decision != nil {
		t.Fatalf("disabled destination issued an action: %#v", decision)
	}
	if got := details["supplyKingdom2"]; !strings.Contains(got, "disabled") {
		t.Fatalf("disabled destination reason=%q", got)
	}

	workflow := state.KingdomTransport.TroopWorkflows[2]
	workflow.Status = "awaiting_destination_refresh"
	workflow.TransportObservedAt = now
	state.KingdomTransport.TroopWorkflows[2] = workflow
	details = map[string]string{}
	decision = NewAutoFortressPolicy().autoFortressSupplyDecision(
		Snapshot{State: state, GameData: autoFortressTestGameData(t), Now: now},
		settings, nil, State.CastleState{}, false, map[string]float64{}, details,
	)
	if decision == nil || decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("disabled completed destination was not refreshed: %#v", decision)
	}
}

func autoFortressTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":277,"type":"Elitetinoswolves","name":"Eventunit","comment1":"Nomad Shop"}],
		"bossdungeons":[
			{"kID":1,"dungeonlevel":45,"cooldownDelay":86400,"playerCooldownDelay":432000},
			{"kID":2,"dungeonlevel":21,"cooldownDelay":86400,"playerCooldownDelay":432000},
			{"kID":3,"dungeonlevel":55,"cooldownDelay":86400,"playerCooldownDelay":432000}
		],
		"effects":[
			{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006},
			{"effectID":426,"name":"speedBonus","effectTypeID":15,"capID":99}
		],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}],
		"globalEffects":[{"ID":10,"globalEffectID":2,"name":"SpeedBoost","effects":"426&60","boostValue":60}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}],
		"currencies":[
			{"currencyID":37,"JSONKey":"KT","Name":"KhanTablet"},
			{"currencyID":1004,"JSONKey":"MS4","Name":"30MinSkip"},
			{"currencyID":1005,"JSONKey":"MS5","Name":"1HourSkip"}
		],
		"currencyMinutesSkipValues":[
			{"currencyID":"1004","MinutesSkipValue":"30"},
			{"currencyID":"1005","MinutesSkipValue":"60"}
		],
		"packages":[
			{"packageID":3858,"packageType":"soldier","unitID":277,"unitAmount":100,"stock":50,"sortOrder":34,"costKhanTablet":4680,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"},
			{"packageID":3857,"packageType":"soldier","unitID":277,"unitAmount":100,"stock":50,"sortOrder":33,"costKhanTablet":2340,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"}
		],"feasts":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
