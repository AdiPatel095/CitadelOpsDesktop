package Buildings

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBerimondPreviewAdvancesMoraleMinimumBeforeCampCapacity(t *testing.T) {
	store := testBuildingStore(t)
	state := State.NewGameState()
	state.Revision = 7
	state.Player.Level = 70
	state.Player.Resources[1] = 1000
	castle := testBuildingCastle()
	state.Castles[castle.ID] = castle
	includeUpgrades := false
	result, err := Preview(state, store, PreviewRequest{
		ExpectedRevision:       uint64Pointer(7),
		CastleID:               castle.ID,
		Profile:                "berimond",
		CandidateDefinitionIDs: []int64{242, 339},
		IncludeUpgrades:        &includeUpgrades,
		IncludeBlocked:         true,
		Constraints: Constraints{
			MinimumValues: map[string]float64{"layoutMoralePercent": 110},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recommended == nil || result.Recommended.Definition.ID != 339 {
		t.Fatalf("recommended candidate = %#v, want morale decoration 339", result.Recommended)
	}
	var tent *Candidate
	for index := range result.Candidates {
		if result.Candidates[index].Definition.ID == 242 {
			tent = &result.Candidates[index]
		}
	}
	if tent == nil || tent.Eligible || !hasBlocker(*tent, "minimum_value_progress") {
		t.Fatalf("camp candidate did not preserve the morale target: %#v", tent)
	}
	if result.CurrentValues["layoutMoralePercent"] != 108 || result.Recommended.ProjectedValues["layoutMoralePercent"] != 128 {
		t.Fatalf("morale projection is incorrect: current=%#v candidate=%#v", result.CurrentValues, result.Recommended)
	}
}

func TestPreviewRejectsStaleRevision(t *testing.T) {
	store := testBuildingStore(t)
	state := State.NewGameState()
	state.Revision = 9
	state.Castles[1] = testBuildingCastle()
	_, err := Preview(state, store, PreviewRequest{ExpectedRevision: uint64Pointer(8), CastleID: 1})
	if _, ok := err.(RevisionMismatchError); !ok {
		t.Fatalf("error = %T %v, want RevisionMismatchError", err, err)
	}
}

func TestNormalizePreviewLayoutDoesNotMutateSourceMaps(t *testing.T) {
	catalog, err := testBuildingStore(t).BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ground := State.Building{InstanceID: 600, DefinitionID: 200}
	castle := State.CastleState{
		Buildings: map[State.BuildingInstanceID]State.Building{600: ground},
		Layout: State.CastleLayout{
			Ground:  map[State.BuildingInstanceID]State.Building{},
			Objects: map[State.BuildingInstanceID]State.Building{},
			Fixed:   map[State.BuildingInstanceID]State.Building{},
		},
	}
	normalized := normalizePreviewLayout(castle, catalog)
	if len(castle.Layout.Ground) != 0 || len(castle.Layout.Objects) != 0 || len(castle.Layout.Fixed) != 0 {
		t.Fatalf("source layout maps were mutated: %#v", castle.Layout)
	}
	if _, found := normalized.Layout.Ground[600]; !found {
		t.Fatalf("derived layout did not contain ground building: %#v", normalized.Layout)
	}
}

func testBuildingStore(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{"version":"test"},
		"units":[],
		"resources":[
			{"resourceID":"1","JSONKey":"C1","name":"currency1"},
			{"resourceID":"2","JSONKey":"C2","name":"currency2"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"}
		],
		"currencies":[],
		"buildings":[
			{"wodID":"200","name":"Castle","group":"Ground","type":"0","level":"0","width":"10","height":"10"},
			{"wodID":"338","name":"FactionDeco","type":"FactionFlag","group":"Building","shopCategory":"DECO","level":"1","width":"2","height":"2","kIDs":"10","eventIDs":"3","Moral":"8","costWood":"10"},
			{"wodID":"242","name":"FactionUnittent","type":"Level1","group":"Building","shopCategory":"MILITARY","level":"1","width":"2","height":"2","kIDs":"10","eventIDs":"3","Moral":"-5","unitSize":"5","costWood":"10","costStone":"10","buildDuration":"30"},
			{"wodID":"339","name":"FactionTrainingground","type":"Level1","group":"Building","shopCategory":"DECO","level":"1","width":"2","height":"2","kIDs":"10","eventIDs":"3","Moral":"20","costWood":"10","costStone":"10","buildDuration":"30"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testBuildingCastle() State.CastleState {
	ground := State.Building{
		InstanceID: 600, DefinitionID: 200, GridX: 0, GridY: 0, Layer: State.BuildingLayerBG, Placed: true,
	}
	decoration := State.Building{
		InstanceID: 700, DefinitionID: 338, GridX: 0, GridY: 0, Layer: State.BuildingLayerBD, Placed: true,
	}
	return State.CastleState{
		ID: 1, KingdomID: 10, SlotType: 1, Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{
			3: {Amount: 1000}, 4: {Amount: 1000},
		},
		Buildings: map[State.BuildingInstanceID]State.Building{600: ground, 700: decoration},
		Layout: State.CastleLayout{
			Ground:  map[State.BuildingInstanceID]State.Building{600: ground},
			Objects: map[State.BuildingInstanceID]State.Building{700: decoration},
		},
		BuildingQueue: State.BuildingConstructionQueue{
			SlotCount: 1,
			Slots:     []State.BuildingConstructionQueueSlot{{Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable}},
		},
	}
}

func hasBlocker(candidate Candidate, code string) bool {
	for _, blocker := range candidate.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

func uint64Pointer(value uint64) *uint64 { return &value }
