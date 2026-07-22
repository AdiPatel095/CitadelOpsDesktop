package Buildings

import (
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestExpansionPreviewRecommendsStorehouseBeforeCapacityBoundExpansion(t *testing.T) {
	gameData := expansionTestGameData(t)
	state := expansionTestState(8, 6800)
	result, err := PreviewExpansion(state, gameData, ExpansionPreviewRequest{CastleID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || result.NextExpansion == nil || result.NextExpansion.Level != 8 {
		t.Fatalf("expansion readiness = %#v", result)
	}
	if result.RecommendedAction == nil || result.RecommendedAction.Intent != "building.upgrade" ||
		result.RecommendedAction.Arguments["buildingInstanceId"] != State.BuildingInstanceID(28) {
		t.Fatalf("recommended action = %#v", result.RecommendedAction)
	}
	if len(result.StorageBuildingCandidates) == 0 || result.StorageBuildingCandidates[0].Definition.ID != 134 {
		t.Fatalf("storage candidates = %#v", result.StorageBuildingCandidates)
	}
	for _, cost := range result.Costs {
		if (cost.Key == "W" || cost.Key == "S") && cost.CapacityShortfall != 383 {
			t.Fatalf("capacity status = %#v", cost)
		}
	}
}

func TestExpansionPreviewBuildsMultiGoodKingdomTransportAction(t *testing.T) {
	gameData := expansionTestGameData(t)
	state := expansionTestState(7, 1000)
	donor := State.CastleState{
		ID: 20, KingdomID: 0, Name: "Donor",
		Resources: map[State.ResourceID]State.ResourceBalance{3: {Amount: 20_000}, 4: {Amount: 20_000}},
		Buildings: map[State.BuildingInstanceID]State.Building{},
	}
	state.Castles[donor.ID] = donor
	richDonor := donor
	richDonor.ID = 21
	richDonor.Name = "Rich Donor"
	richDonor.Resources = map[State.ResourceID]State.ResourceBalance{3: {Amount: 100_000}, 4: {Amount: 100_000}}
	state.Castles[richDonor.ID] = richDonor
	state.KingdomTransport.ObservedAt = time.Now().UTC()
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	result, err := PreviewExpansion(state, gameData, ExpansionPreviewRequest{CastleID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedAction == nil || result.RecommendedAction.Intent != "resource.ship" {
		t.Fatalf("recommended action = %#v", result.RecommendedAction)
	}
	if result.RecommendedAction.Arguments["sourceCastleId"] != State.CastleID(21) {
		t.Fatalf("transport donor = %#v", result.RecommendedAction.Arguments["sourceCastleId"])
	}
	goods, ok := result.RecommendedAction.Arguments["goods"].([]map[string]any)
	if !ok || len(goods) != 2 || goods[0]["resourceId"] != State.ResourceID(3) || goods[1]["resourceId"] != State.ResourceID(4) {
		t.Fatalf("transport goods = %#v", result.RecommendedAction.Arguments["goods"])
	}
	if goods[0]["amount"] != int64(6143) || goods[1]["amount"] != int64(6143) {
		t.Fatalf("transport goods do not include the kingdom delivery loss = %#v", goods)
	}
}

func TestExpansionPreviewFundsStorageUpgradeBeforeCapacityBoundExpansion(t *testing.T) {
	gameData := expansionTestGameData(t)
	state := expansionTestState(8, 0)
	donor := State.CastleState{
		ID: 20, KingdomID: 0, Name: "Donor",
		Resources: map[State.ResourceID]State.ResourceBalance{3: {Amount: 20_000}, 4: {Amount: 20_000}},
		Buildings: map[State.BuildingInstanceID]State.Building{},
	}
	state.Castles[donor.ID] = donor
	state.KingdomTransport.ObservedAt = time.Now().UTC()
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	result, err := PreviewExpansion(state, gameData, ExpansionPreviewRequest{CastleID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.RecommendedAction == nil || result.RecommendedAction.Intent != "resource.ship" {
		t.Fatalf("recommended action = %#v", result.RecommendedAction)
	}
	goods, ok := result.RecommendedAction.Arguments["goods"].([]map[string]any)
	if !ok || len(goods) != 2 || goods[0]["amount"] != int64(1745) || goods[1]["amount"] != int64(1745) {
		t.Fatalf("storage funding goods = %#v", result.RecommendedAction.Arguments["goods"])
	}
}

func TestExpansionPreviewUsesApprovedTimeSkipForQueuedStorageUpgrade(t *testing.T) {
	gameData := expansionTestGameData(t)
	state := expansionTestState(8, 6800)
	castle := state.Castles[10]
	storehouse := castle.Layout.Objects[28]
	storehouse.ConstructionState = State.BuildingStateUpgradeInProgress
	storehouse.ProgressSec = 1000
	castle.Layout.Objects[28] = storehouse
	castle.Buildings[28] = storehouse
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: 28, Status: State.BuildingQueueSlotOccupied, BuildingID: 28,
	}
	state.Castles[10] = castle
	state.Player.Currencies[1003] = 2

	result, err := PreviewExpansion(state, gameData, ExpansionPreviewRequest{CastleID: 10, AllowTimeSkips: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.PendingStorageBuild == nil || result.PendingStorageBuild.TargetDefinitionID != 134 {
		t.Fatalf("pending storage build = %#v", result.PendingStorageBuild)
	}
	if result.RecommendedAction == nil || result.RecommendedAction.Intent != "building.skip_time" ||
		result.RecommendedAction.Arguments["minutes"] != 10 {
		t.Fatalf("storage time-skip action = %#v", result.RecommendedAction)
	}
}

func expansionTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],
		"resources":[
			{"resourceID":"2","JSONKey":"C2","name":"currency2"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"}
		],
		"currencies":[{"currencyID":"1003","JSONKey":"MS3","Name":"10MinSkip"}],
		"currencyMinutesSkipValues":[{"currencyID":"1003","MinutesSkipValue":"10"}],
		"constructionItems":[
			{"constructionItemID":"265","constructionItemGroupID":"23","name":"WoodStorageCapacity","level":"1","woodStorage":"1000"},
			{"constructionItemID":"275","constructionItemGroupID":"24","name":"StoneStorageCapacity","level":"1","stoneStorage":"1000"}
		],
		"expansions":[
			{"expansionID":"112","spaceIDs":"4","expansionLevel":"7","costWood":"5914","costStone":"5914","costC2":"1400"},
			{"expansionID":"113","spaceIDs":"4","expansionLevel":"8","costWood":"7183","costStone":"7183","costC2":"1600"}
		],
		"buildings":[
			{"wodID":"200","name":"Ground","group":"Ground","width":"40","height":"40"},
			{"wodID":"171","name":"Keep","group":"Building","level":"1","width":"5","height":"5","woodStorage":"800","stoneStorage":"800"},
			{"wodID":"133","name":"Storehouse","group":"Building","level":"2","width":"5","height":"5","upgradeWodID":"134","woodStorage":"6000","stoneStorage":"6000"},
			{"wodID":"134","name":"Storehouse","group":"Building","level":"3","width":"5","height":"5","downgradeWodID":"133","woodStorage":"12500","stoneStorage":"12500","costWood":"1396","costStone":"1396","buildDuration":"33960"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func expansionTestState(groundCount int, resourceAmount float64) State.GameState {
	state := State.NewGameState()
	state.Player.Level = 70
	observedAt := time.Now().UTC()
	ground := map[State.BuildingInstanceID]State.Building{}
	buildings := map[State.BuildingInstanceID]State.Building{}
	for index := 0; index < groundCount; index++ {
		id := State.BuildingInstanceID(index + 1)
		tile := State.Building{InstanceID: id, DefinitionID: 200, GridX: index * 40, GridY: 0, Layer: State.BuildingLayerBG, Placed: true}
		ground[id] = tile
		buildings[id] = tile
	}
	keep := State.Building{InstanceID: 16, DefinitionID: 171, GridX: 1, GridY: 1, Layer: State.BuildingLayerBD, Placed: true}
	storehouse := State.Building{InstanceID: 28, DefinitionID: 133, GridX: 8, GridY: 1, Layer: State.BuildingLayerBD, Placed: true}
	buildings[16] = keep
	buildings[28] = storehouse
	state.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 4, Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{
			3: {Amount: resourceAmount, Capacity: float64Pointer(6800)},
			4: {Amount: resourceAmount, Capacity: float64Pointer(6800)},
		},
		Buildings: buildings,
		Layout: State.CastleLayout{
			Ground: ground, Objects: map[State.BuildingInstanceID]State.Building{16: keep, 28: storehouse},
			Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: observedAt,
		},
		BuildingQueue: State.BuildingConstructionQueue{
			SlotCount: 1, ObservedAt: observedAt,
			Slots: []State.BuildingConstructionQueueSlot{{Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable}},
		},
		ConstructionSlots:           map[State.BuildingInstanceID][]State.ConstructionSlot{},
		ConstructionSlotsObservedAt: observedAt,
	}
	state.Inventory.ConstructionItemsObservedAt = observedAt
	return state
}

func float64Pointer(value float64) *float64 { return &value }
