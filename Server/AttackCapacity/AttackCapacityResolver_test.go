package AttackCapacity

import (
	"math"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestResolverCollectsCastleAndCommanderCapacityEffects(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],
		"units":[],
		"buildings":[{"wodID":10,"effects":"66&10"}],
		"constructionItems":[{"constructionItemID":20,"effects":"66&5"}],
		"effects":[
			{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99},
			{"effectID":503,"name":"AttackUnitAmountFront","effectTypeID":34,"capID":99},
			{"effectID":2205,"name":"relicAttackUnitAmountFlankPVE","effectTypeID":28,"capID":1105,"areaTypeID":"2"}
		],
		"effectCaps":[{"capID":99},{"capID":1105,"maxTotalBonus":40}]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 10},
		},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 20, Slot: 1}},
		},
	}
	state.Commanders[7] = State.CommanderState{
		ID:        7,
		Equipment: map[string]State.EquipmentInstanceID{"1": 1001, "2": 1002},
		Gems:      map[string]State.GemInstanceID{"1": 2001},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2205, Values: []float64{25}}},
	}
	state.Inventory.Equipment[1002] = State.EquipmentInstance{
		ID: 1002, Effects: State.EquipmentEffects{{DefinitionID: 2205, Values: []float64{30}}},
	}
	state.Inventory.Gems[2001] = State.GemInstance{
		ID: 2001, Effects: State.EquipmentEffects{{DefinitionID: 503, Values: []float64{20}}},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100,
		CommanderID:    7,
		Target: TargetContext{
			ID: "tower:2:10:20", AreaTypeID: 2,
			BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 155, Front: 240, Right: 155}) {
		t.Fatalf("unexpected capacity: %#v", result.Capacity)
	}
	if result.BonusPercent != (LaneBonus{Left: 55, Front: 20, Right: 55}) {
		t.Fatalf("unexpected bonus: %#v", result.BonusPercent)
	}
	if len(result.CapGroups) != 3 {
		t.Fatalf("expected three cap groups, got %#v", result.CapGroups)
	}
	var pveCap CapGroup
	for _, group := range result.CapGroups {
		if group.Lane == LaneFlank && group.CapID == 1105 {
			pveCap = group
		}
	}
	if pveCap.RawPercent != 55 || pveCap.AppliedPercent != 40 || !pveCap.Capped {
		t.Fatalf("unexpected PvE cap group: %#v", pveCap)
	}
}

func TestResolverSkipsCapacityEffectsForOtherTargetAreas(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],
		"units":[],
		"buildings":[],
		"effects":[
			{"effectID":2305,"name":"relicAttackUnitAmountFlankPVP","effectTypeID":28,"capID":1205,"areaTypeID":"1"}
		],
		"effectCaps":[{"capID":1205,"maxTotalBonus":40}]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{
		ID: 7, Equipment: map[string]State.EquipmentInstanceID{"1": 1001}, Gems: map[string]State.GemInstanceID{},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2305, Values: []float64{20}}},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100,
		CommanderID:    7,
		Target:         TargetContext{AreaTypeID: 2, BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 100, Front: 200, Right: 100}) {
		t.Fatalf("unexpected capacity: %#v", result.Capacity)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "effect does not apply to the target castle type" {
		t.Fatalf("unexpected skipped effects: %#v", result.Skipped)
	}
}

func TestResolverIncludesEquipmentSetBonusAndSkipsExpiredConstructionItems(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],
		"units":[],
		"buildings":[],
		"constructionItems":[{"constructionItemID":20,"effects":"66&30"}],
		"effects":[{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99}],
		"effectCaps":[{"capID":99}],
		"equipment_effects":[{"equipmentEffectID":900,"effectID":66}],
		"equipment_sets":[{"setID":50,"neededItems":2,"effects":"900&12"}]
	}`)
	remaining := 30
	observedAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100, ConstructionSlotsObservedAt: observedAt,
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 20, Slot: 1, RemainingSec: &remaining}},
		},
	}
	state.Commanders[7] = State.CommanderState{
		ID: 7, Equipment: map[string]State.EquipmentInstanceID{"1": 1001, "2": 1002}, Gems: map[string]State.GemInstanceID{},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{ID: 1001, SetID: 50}
	state.Inventory.Equipment[1002] = State.EquipmentInstance{ID: 1002, SetID: 50}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100,
		CommanderID:    7,
		Target:         TargetContext{AreaTypeID: 2, BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
		EvaluatedAt:    observedAt.Add(31 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 112, Front: 200, Right: 112}) {
		t.Fatalf("unexpected capacity: %#v", result.Capacity)
	}
	if len(result.Modifiers) != 1 || result.Modifiers[0].SourceKind != SourceEquipmentSet || result.Modifiers[0].Percent != 12 {
		t.Fatalf("unexpected modifiers: %#v", result.Modifiers)
	}
}

func TestResolveContextRoundsWholeTroopCapacityUpLikeGameClient(t *testing.T) {
	result, err := (Resolver{}).ResolveContext(Context{
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 101, Front: 101, Right: 101}},
		Modifiers: []Modifier{
			{EffectID: 1, Lane: LaneFlank, CapID: 99, Percent: 2.5},
			{EffectID: 2, Lane: LaneFront, CapID: 99, Percent: 2.5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 104, Front: 104, Right: 104}) {
		t.Fatalf("unexpected rounded capacity: %#v", result.Capacity)
	}
	if math.Abs(result.BonusPercent.Left-2.5) > 0.000001 || math.Abs(result.BonusPercent.Front-2.5) > 0.000001 {
		t.Fatalf("unexpected bonus: %#v", result.BonusPercent)
	}
}

func TestResolveContextInfersTowerBaseCapacityFromVictoryCount(t *testing.T) {
	result, err := (Resolver{}).ResolveContext(Context{
		Target: TargetContext{
			Map: &MapTarget{KingdomID: 0, TypeID: 2, VictoryCount: 845}, AreaTypeID: 2,
		},
		Modifiers: []Modifier{{EffectID: 2108, Lane: LaneFlank, CapID: 1008, Percent: 45}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target.Level != 81 || result.BaseCapacity != (LaneCapacity{Left: 64, Front: 192, Right: 64}) {
		t.Fatalf("unexpected inferred tower base: %#v", result)
	}
	if result.Capacity != (LaneCapacity{Left: 93, Front: 192, Right: 93}) {
		t.Fatalf("unexpected tower capacity: %#v", result.Capacity)
	}
}

func TestResolveContextAppliesLowLevelFlankBonusBeforeCeiling(t *testing.T) {
	result, err := (Resolver{}).ResolveContext(Context{
		Target:    TargetContext{Map: &MapTarget{KingdomID: 0, TypeID: 2, VictoryCount: 0}, AreaTypeID: 2},
		Modifiers: []Modifier{{EffectID: 2108, Lane: LaneFlank, CapID: 1008, Percent: 45}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target.Level != 1 || result.BaseCapacity != (LaneCapacity{Left: 3, Front: 7, Right: 3}) {
		t.Fatalf("unexpected low-level tower base: %#v", result)
	}
	if result.Capacity != (LaneCapacity{Left: 4, Front: 7, Right: 4}) {
		t.Fatalf("unexpected low-level tower capacity: %#v", result.Capacity)
	}
}

func TestResolverUsesFocusedCastleWhenSourceIsOmitted(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],
		"units":[],
		"buildings":[{"wodID":10,"effects":"66&10"}],
		"effects":[{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99}],
		"effectCaps":[{"capID":99}]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100, Focused: true,
		Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 10}},
	}
	state.Commanders[7] = State.CommanderState{ID: 7}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		CommanderID: 7,
		Target:      TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCastleID != 100 || result.Capacity != (LaneCapacity{Left: 110, Front: 200, Right: 110}) {
		t.Fatalf("focused resolution failed: %#v", result)
	}
}

func TestResolverAcceptsWireCommanderIDZero(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[0] = State.CommanderState{ID: 0}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100,
		CommanderID:    0,
		Target:         TargetContext{BaseCapacity: LaneCapacity{Left: 64, Front: 192, Right: 64}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CommanderID != 0 || result.Capacity != (LaneCapacity{Left: 64, Front: 192, Right: 64}) {
		t.Fatalf("wire commander zero was not resolved: %#v", result)
	}
}

func TestResolverUsesAttackDialogEffectsWithoutDoubleCountingCastleState(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],
		"units":[],
		"buildings":[{"wodID":10,"effects":"66&10"}],
		"constructionItems":[{"constructionItemID":20,"effects":"66&5"}],
		"effects":[{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99}],
		"effectCaps":[{"capID":99}]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100, Focused: true,
		Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 10}},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 20, Slot: 1}},
		},
	}
	state.Commanders[7] = State.CommanderState{ID: 7}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 100,
		KingdomID:      2,
		Target:         State.AttackDialogTarget{TypeID: 34, X: 205, Y: 938, ObjectID: 70},
		ActiveEffects:  []State.AttackDialogEffect{{EffectID: 66, Values: []float64{40}, Source: "CI"}},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		CommanderID: 7, UseAttackDialogEffects: true,
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 140, Front: 200, Right: 140}) {
		t.Fatalf("unexpected dialog capacity: %#v", result.Capacity)
	}
	if len(result.Modifiers) != 1 || result.Modifiers[0].SourceKind != SourceConstructionItem {
		t.Fatalf("unexpected modifiers: %#v", result.Modifiers)
	}
	if result.Target.Map == nil || *result.Target.Map != (MapTarget{KingdomID: 2, TypeID: 34, X: 205, Y: 938, ObjectID: 70}) {
		t.Fatalf("attack-dialog target was not inferred: %#v", result.Target.Map)
	}
	if result.Target.CastleTypeID != 34 {
		t.Fatalf("attack-dialog castle type = %d, want 34", result.Target.CastleTypeID)
	}
}

func TestResolverUsesOfficialEffectTypesEvenWhenNamesDiffer(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[
			{"effectID":23,"name":"attackBoostFront2","effectTypeID":34,"capID":99},
			{"effectID":24,"name":"attackBoostFlank2","effectTypeID":28,"capID":99}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{
		ID: 7, Equipment: map[string]State.EquipmentInstanceID{"1": 1001}, Gems: map[string]State.GemInstanceID{},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{
			{DefinitionID: 23, Values: []float64{10}},
			{DefinitionID: 24, Values: []float64{20}},
		},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 120, Front: 220, Right: 120}) {
		t.Fatalf("capacity = %#v", result.Capacity)
	}
}

func TestResolverUsesTargetCastleTypeAsAuthoritativeEffectFilter(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[
			{"effectID":2807,"name":"relicAttackUnitAmountFrontPVP","effectTypeID":34,"areaTypeID":"1,21,34","capID":1707}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{
		ID: 7, Equipment: map[string]State.EquipmentInstanceID{"1": 1001}, Gems: map[string]State.GemInstanceID{},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2807, Values: []float64{25}}},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{
			Map: &MapTarget{TypeID: 21}, CastleTypeID: 21, PvP: false,
			BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity.Front != 250 || len(result.Modifiers) != 1 {
		t.Fatalf("target castle type did not select effect 2807: %#v", result)
	}
}

func TestResolverUsesBerimondTowerTargetTypeAndLevel(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[
			{"effectID":2705,"name":"relicAttackUnitAmountFlankBerimond","effectTypeID":28,"areaTypeID":"15,16,17,18,30","capID":1605}
		],
		"effectCaps":[{"capID":1605,"maxTotalBonus":50}]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{
		ID: 7, Equipment: map[string]State.EquipmentInstanceID{"1": 1001}, Gems: map[string]State.GemInstanceID{},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2705, Values: []float64{20}}},
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{
			TargetType: TargetTypeBerimondTower,
			Map: &MapTarget{
				KingdomID: State.KingdomID(GameData.BerimondKingdomID),
				TypeID:    BerimondTowerMapTypeID, X: 1472, Y: 29, ObjectID: -410, Level: 55,
			},
			Level: 55, CastleTypeID: BerimondTowerMapTypeID, PvP: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target.TargetType != TargetTypeBerimondTower ||
		result.Target.CastleTypeID != BerimondTowerMapTypeID ||
		result.BaseCapacity != (LaneCapacity{Left: 52, Front: 156, Right: 52}) {
		t.Fatalf("unexpected Berimond target context: %#v", result)
	}
	if result.Capacity != (LaneCapacity{Left: 63, Front: 156, Right: 63}) ||
		len(result.Modifiers) != 1 || result.Modifiers[0].EffectID != 2705 {
		t.Fatalf("unexpected Berimond tower capacity: %#v", result)
	}

	_, err = (Resolver{}).ResolveContext(Context{
		Target: TargetContext{
			TargetType: TargetTypeBerimondTower,
			Map:        &MapTarget{KingdomID: 0, TypeID: BerimondTowerMapTypeID, Level: 55},
		},
	})
	if err == nil {
		t.Fatal("Berimond tower target accepted a non-Berimond kingdom")
	}
}

func TestResolverCalculatesTargetApplicableAttackWaves(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[
			{"effectID":29,"name":"additionalWaves","effectTypeID":156,"capID":99},
			{"effectID":447,"name":"equipmentAREAdditionalWaves","effectTypeID":156,"areaTypeID":"43","capID":99},
			{"effectID":484,"name":"newPVPAdditionalWaves","effectTypeID":156,"isPvPFight":1,"capID":99}
		],
		"generalSkills":[{"skillID":101001,"generalID":101,"effects":"29&1"}],
		"legendskills":[
			{"skillID":101,"effectType":"additionalWave","totalEffectValue":1},
			{"skillID":102,"effectType":"additionalAttackToolAmountFlank","totalEffectValue":12}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{
		ID: 7, GeneralID: 101,
		Equipment: map[string]State.EquipmentInstanceID{"1": 1001}, Gems: map[string]State.GemInstanceID{"1": 2001},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{
			{DefinitionID: 29, Values: []float64{4}},
			{DefinitionID: 447, Values: []float64{5}},
			{DefinitionID: 484, Values: []float64{3}},
		},
	}
	state.Inventory.Gems[2001] = State.GemInstance{
		ID: 2001, Effects: State.EquipmentEffects{{DefinitionID: 29, Values: []float64{1}}},
	}
	state.Generals[101] = State.GeneralState{
		ID: 101, ActiveSkillIDs: []int64{101001}, ObservedAt: time.Now().UTC(),
	}
	state.Player.LegendSkills = State.LegendSkillState{ActiveIDs: []int64{101, 102}, ObservedAt: time.Now().UTC()}

	resolve := func(castleTypeID int, pvp bool, legendaryFight bool) Result {
		t.Helper()
		state.AttackDialog = State.AttackDialogState{
			SourceCastleID: 100,
			Target:         State.AttackDialogTarget{TypeID: castleTypeID},
			ActiveEffects: []State.AttackDialogEffect{
				{EffectID: 29, Values: []float64{1}, Source: "CI"},
				{EffectID: 447, Values: []float64{2}, Source: "DE"},
			},
			ObservedAt: time.Now().UTC(),
		}
		result, err := (Resolver{}).Resolve(state, gameData, Request{
			SourceCastleID: 100, CommanderID: 7, UseAttackDialogEffects: true,
			Target: TargetContext{
				Map: &MapTarget{TypeID: castleTypeID}, CastleTypeID: castleTypeID,
				PvP: pvp, LegendaryFight: legendaryFight,
				BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	invasion := resolve(21, true, true)
	if invasion.BaseWaves != 4 || invasion.AdditionalWaves != 11 || invasion.MaximumWaves != 15 {
		t.Fatalf("invasion waves = base %d additional %d maximum %d", invasion.BaseWaves, invasion.AdditionalWaves, invasion.MaximumWaves)
	}
	if len(invasion.WaveModifiers) != 6 {
		t.Fatalf("invasion wave modifiers = %#v", invasion.WaveModifiers)
	}
	if invasion.BaseToolCapacity != (LaneCapacity{Left: 40, Front: 50, Right: 40}) ||
		invasion.LegendToolBonus != 10 ||
		invasion.ToolCapacity != (LaneCapacity{Left: 50, Front: 50, Right: 50}) {
		t.Fatalf("legendary PvP tool capacity = %#v", invasion)
	}

	regularPlayer := resolve(21, true, false)
	if regularPlayer.BaseToolCapacity != (LaneCapacity{Left: 30, Front: 40, Right: 30}) ||
		regularPlayer.LegendToolBonus != 0 ||
		regularPlayer.ToolCapacity != (LaneCapacity{Left: 30, Front: 40, Right: 30}) {
		t.Fatalf("non-legendary PvP tool capacity = %#v", regularPlayer)
	}

	rift := resolve(43, false, true)
	if rift.BaseWaves != 4 || rift.AdditionalWaves != 15 || rift.MaximumWaves != 19 {
		t.Fatalf("Rift waves = base %d additional %d maximum %d", rift.BaseWaves, rift.AdditionalWaves, rift.MaximumWaves)
	}
	if rift.LegendToolBonus != 0 || rift.ToolCapacity != (LaneCapacity{Left: 30, Front: 40, Right: 30}) {
		t.Fatalf("PvE tool capacity = %#v", rift)
	}
}

func TestResolverAddsObservedGeneralSkills(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],"buildings":[],
		"generalSkills":[{"skillID":101036,"generalID":101,"effects":"66&60,503&25"}],
		"effects":[
			{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99},
			{"effectID":503,"name":"AttackUnitAmountFront","effectTypeID":34,"capID":99}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100}
	state.Commanders[7] = State.CommanderState{ID: 7, GeneralID: 101}
	state.Generals[101] = State.GeneralState{
		ID: 101, ActiveSkillIDs: []int64{101036}, ObservedAt: time.Now().UTC(),
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 160, Front: 250, Right: 160}) {
		t.Fatalf("capacity = %#v", result.Capacity)
	}
	if len(result.Modifiers) != 2 || result.Modifiers[0].SourceKind != SourceGeneralSkill || result.Modifiers[1].SourceKind != SourceGeneralSkill {
		t.Fatalf("general modifiers = %#v", result.Modifiers)
	}
}

func TestResolverCalculatesCappedCourtyardSupportCapacity(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],
		"buildings":[{"wodID":10,"effects":"700&1000"}],
		"constructionItems":[{"constructionItemID":20,"effects":"701&10"}],
		"generalSkills":[{"skillID":101024,"generalID":101,"effects":"700&2400"}],
		"effects":[
			{"effectID":700,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":99},
			{"effectID":701,"name":"attackUnitAmountReinforcementBoost","effectTypeID":180,"capID":33},
			{"effectID":2115,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":11413}
		],
		"effectCaps":[
			{"capID":33,"maxTotalBonus":30},
			{"capID":99},
			{"capID":11413,"maxTotalBonus":3000}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 10},
		},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 20, Slot: 1}},
		},
	}
	state.Commanders[7] = State.CommanderState{
		ID: 7, GeneralID: 101,
		Equipment: map[string]State.EquipmentInstanceID{"1": 1001},
		Gems:      map[string]State.GemInstanceID{"1": 2001},
	}
	state.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2115, Values: []float64{2000}}},
	}
	state.Inventory.Gems[2001] = State.GemInstance{
		ID: 2001, Effects: State.EquipmentEffects{{DefinitionID: 2115, Values: []float64{1500}}},
	}
	state.Generals[101] = State.GeneralState{
		ID: 101, ActiveSkillIDs: []int64{101024}, ObservedAt: time.Now().UTC(),
	}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
		AdditionalSources: []EffectSource{{
			Kind: SourceAdditional, Effects: State.EquipmentEffects{{DefinitionID: 701, Values: []float64{25}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SupportUnitAmount != 6400 || result.SupportBoostPercent != 30 || result.SupportCapacity != 8320 {
		t.Fatalf(
			"support capacity = units %.0f boost %.0f capacity %d",
			result.SupportUnitAmount,
			result.SupportBoostPercent,
			result.SupportCapacity,
		)
	}
	if len(result.SupportModifiers) != 6 || len(result.SupportCapGroups) != 3 {
		t.Fatalf("support effect details = modifiers %#v groups %#v", result.SupportModifiers, result.SupportCapGroups)
	}
	var relicUnits, supportBoost SupportCapGroup
	for _, group := range result.SupportCapGroups {
		switch {
		case group.Kind == SupportModifierUnits && group.CapID == 11413:
			relicUnits = group
		case group.Kind == SupportModifierBoost && group.CapID == 33:
			supportBoost = group
		}
	}
	if relicUnits.RawAmount != 3500 || relicUnits.AppliedAmount != 3000 || !relicUnits.Capped {
		t.Fatalf("relic support cap group = %#v", relicUnits)
	}
	if supportBoost.RawAmount != 35 || supportBoost.AppliedAmount != 30 || !supportBoost.Capped {
		t.Fatalf("support boost cap group = %#v", supportBoost)
	}
}

func TestResolverIgnoresStoredObjectsAndClassifiesPlacedDecorations(t *testing.T) {
	gameData := resolverTestGameData(t, `{
		"versionInfo":[],"units":[],
		"buildings":[
			{"wodID":10,"group":"Building","effects":"66&50"},
			{"wodID":11,"group":"Building","decoPoints":100,"effects":"503&20"}
		],
		"effects":[
			{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99},
			{"effectID":503,"name":"AttackUnitAmountFront","effectTypeID":34,"capID":99}
		]
	}`)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{
		ID: 100,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 10, Layer: State.BuildingLayerBD, Placed: false},
			2: {InstanceID: 2, DefinitionID: 11, Layer: State.BuildingLayerBD, Placed: true},
		},
	}
	state.Commanders[7] = State.CommanderState{ID: 7}

	result, err := (Resolver{}).Resolve(state, gameData, Request{
		SourceCastleID: 100, CommanderID: 7,
		Target: TargetContext{BaseCapacity: LaneCapacity{Left: 100, Front: 200, Right: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Capacity != (LaneCapacity{Left: 100, Front: 240, Right: 100}) {
		t.Fatalf("capacity = %#v", result.Capacity)
	}
	if len(result.Modifiers) != 1 || result.Modifiers[0].SourceKind != SourceDecoration {
		t.Fatalf("decoration modifiers = %#v", result.Modifiers)
	}
}

func resolverTestGameData(t *testing.T, document string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(document), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
