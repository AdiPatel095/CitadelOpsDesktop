package Equipment

import (
	"maps"
	"reflect"
	"strconv"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestOptimizeSelectsBestCanonicalEffectsBySlot(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		weakID := State.EquipmentInstanceID(100 + slot)
		strongID := State.EquipmentInstanceID(200 + slot)
		gameState.Inventory.Equipment[weakID] = optimizerTestItem(weakID, slot, 10)
		gameState.Inventory.Equipment[strongID] = optimizerTestItem(strongID, slot, float64(20+slot))
		leader := gameState.Commanders[0]
		leader.Equipment[strconv.Itoa(slot)] = weakID
		gameState.Commanders[0] = leader
	}
	for slot := 1; slot <= 2; slot++ {
		id := State.GemInstanceID(300 + slot)
		gameState.Inventory.Gems[-id] = State.GemInstance{
			ID: -id, CompatibleWearerID: 2, CombatMode: "pvp",
			Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: 9001, Values: []float64{5}}},
		}
		leader := gameState.Commanders[0]
		leader.Gems[strconv.Itoa(slot)] = -id
		gameState.Commanders[0] = leader
	}

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for slot := 1; slot <= 4; slot++ {
		want := State.EquipmentInstanceID(200 + slot)
		if got := result.Proposed.Equipment[strconv.Itoa(slot)]; got != want {
			t.Fatalf("slot %d = %d, want %d", slot, got, want)
		}
	}
	if result.Proposed.Score <= result.Current.Score {
		t.Fatalf("proposed score %.0f did not improve current %.0f", result.Proposed.Score, result.Current.Score)
	}
	for slot := 1; slot <= 2; slot++ {
		if result.Proposed.Gems[strconv.Itoa(slot)] != result.Current.Gems[strconv.Itoa(slot)] {
			t.Fatalf("equal-scoring gem in slot %d moved: current %#v proposed %#v", slot, result.Current.Gems, result.Proposed.Gems)
		}
	}
}

func TestOptimizeReturnsUsefulStableAlternativesFromOneSearch(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		for variant := 1; variant <= 3; variant++ {
			id := State.EquipmentInstanceID(slot*100 + variant)
			gameState.Inventory.Equipment[id] = optimizerTestItem(id, slot, float64(100-variant))
		}
	}
	request := OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	}
	first, err := Optimize(gameState, nil, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Optimize(gameState, nil, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Alternatives) != 1 {
		t.Fatalf("alternatives = %d, want one non-dominated outcome", len(first.Alternatives))
	}
	if !reflect.DeepEqual(first.Proposed, first.Alternatives[0]) {
		t.Fatal("legacy proposed loadout is not the first ranked alternative")
	}
	seen := map[string]struct{}{}
	for index, alternative := range first.Alternatives {
		key := semanticOutcomeKey(alternative.Effects)
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("alternative %d duplicates semantic outcome %q", index, key)
		}
		seen[key] = struct{}{}
		if index > 0 && alternative.Score > first.Alternatives[index-1].Score {
			t.Fatalf("alternative %d score %.0f exceeds prior score %.0f", index, alternative.Score, first.Alternatives[index-1].Score)
		}
		if index > 0 && !materiallyDifferent(alternative.Effects, first.Alternatives[0].Effects) && alternative.ExtractionCost.MaximumRubySpend >= first.Alternatives[0].ExtractionCost.MaximumRubySpend {
			t.Fatalf("alternative %d has no material stat or cost trade-off", index)
		}
	}
	if !reflect.DeepEqual(first.Alternatives, second.Alternatives) {
		t.Fatal("unchanged inputs produced unstable alternative ordering")
	}
}

func TestOptimizeReturnsFewerAlternativesWhenAssignmentsAreExhausted(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		gameState.Inventory.Equipment[id] = optimizerTestItem(id, slot, float64(slot))
	}
	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alternatives) != 1 {
		t.Fatalf("alternatives = %d, want 1", len(result.Alternatives))
	}
}

func TestOptimizeRejectsGemOnAnotherLeadersCarrier(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 901}, Gems: map[string]State.GemInstanceID{"1": 501}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		gameState.Inventory.Equipment[id] = optimizerTestItem(id, slot, float64(slot))
	}
	gameState.Inventory.Equipment[901] = State.EquipmentInstance{ID: 901, Slot: 1, TypeID: 2, RelicKnown: true, WearerKind: "commander", WearerID: 1}
	gameState.Inventory.Gems[501] = State.GemInstance{
		ID: 501, EquipmentInstanceID: 901, CompatibleWearerID: 2, CombatMode: "pvp",
		Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: 9001, Values: []float64{1000}}},
	}
	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidates.Gems != 0 {
		t.Fatalf("borrowed carrier gem counted as eligible: %#v", result.Candidates)
	}
}

func TestOptimizeDoesNotPlaceNormalGemOnRelicEquipment(t *testing.T) {
	gameState := optimizerFamilyTestState(LoadoutFamilyRelic)
	gameState.Inventory.Gems[-501] = optimizerTestGem(-501, 10_000)
	gameState.Inventory.Gems[601] = optimizerTestGem(601, 10)

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, alternative := range result.Alternatives {
		for _, gemID := range alternative.Gems {
			if gemID < 0 {
				t.Fatalf("alternative %d placed normal gem %d on relic equipment: %#v", index, gemID, alternative)
			}
		}
	}
}

func TestOptimizeDoesNotPlaceRelicGemOnOrdinaryEquipment(t *testing.T) {
	gameState := optimizerFamilyTestState(LoadoutFamilyOrdinary)
	gameState.Inventory.Gems[501] = optimizerTestGem(501, 10_000)
	gameState.Inventory.Gems[-601] = optimizerTestGem(-601, 10)

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, alternative := range result.Alternatives {
		for _, gemID := range alternative.Gems {
			if gemID > 0 {
				t.Fatalf("alternative %d placed relic gem %d on ordinary equipment: %#v", index, gemID, alternative)
			}
		}
	}
}

func TestOptimizeExcludesMixedEquipmentAndKeepsBothFamilies(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for _, family := range []LoadoutFamily{LoadoutFamilyOrdinary, LoadoutFamilyRelic} {
		for _, slot := range optimizerSlots {
			id := State.EquipmentInstanceID(int(family)*1_000 + slot)
			item := optimizerTestItem(id, slot, 10)
			item.RarityID = 5
			if slot == 6 {
				item.RarityID = 15
			}
			item.Relic = family == LoadoutFamilyRelic
			gameState.Inventory.Equipment[id] = item
		}
	}

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[LoadoutFamily]bool{}
	for index, alternative := range result.Alternatives {
		family := LoadoutFamilyUnknown
		for rawSlot, id := range alternative.Equipment {
			item := gameState.Inventory.Equipment[id]
			itemFamily := EquipmentFamily(item)
			if family != LoadoutFamilyUnknown && family != itemFamily {
				t.Fatalf("alternative %d mixes equipment families at slot %s: %#v", index, rawSlot, alternative)
			}
			family = itemFamily
		}
		seen[family] = true
	}
	if len(seen) != 1 {
		t.Fatalf("equivalent family outcomes were not collapsed: %#v", result.Alternatives)
	}
}

func TestOptimizeKeepsGemmedAppearanceFamilyAndAllowsUngemmedAppearanceSwitch(t *testing.T) {
	for _, test := range []struct {
		name            string
		appearanceRelic bool
		gemID           State.GemInstanceID
		wantFamilies    map[LoadoutFamily]bool
	}{
		{
			name: "ordinary gemmed appearance", gemID: -501,
			wantFamilies: map[LoadoutFamily]bool{LoadoutFamilyOrdinary: true},
		},
		{
			name: "relic gemmed appearance", appearanceRelic: true, gemID: 501,
			wantFamilies: map[LoadoutFamily]bool{LoadoutFamilyRelic: true},
		},
		{
			name:         "ungemmed appearance",
			wantFamilies: map[LoadoutFamily]bool{LoadoutFamilyOrdinary: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Commanders[0] = State.CommanderState{
				ID: 0, Available: true,
				Equipment: map[string]State.EquipmentInstanceID{"5": 105},
				Gems:      map[string]State.GemInstanceID{},
			}
			gameState.Inventory.Equipment[105] = State.EquipmentInstance{
				ID: 105, Slot: 5, TypeID: 2, RelicKnown: true, Relic: test.appearanceRelic,
				WearerKind: "commander", WearerID: 0,
			}
			if test.gemID != 0 {
				gameState.Inventory.Gems[test.gemID] = State.GemInstance{
					ID: test.gemID, EquipmentInstanceID: 105, WearerKind: "commander", WearerID: 0,
				}
			}
			for _, family := range []LoadoutFamily{LoadoutFamilyOrdinary, LoadoutFamilyRelic} {
				for slot := 1; slot <= 4; slot++ {
					id := State.EquipmentInstanceID(int(family)*1_000 + slot)
					item := optimizerTestItem(id, slot, 10)
					item.Relic = family == LoadoutFamilyRelic
					gameState.Inventory.Equipment[id] = item
				}
			}

			result, err := Optimize(gameState, nil, OptimizeRequest{
				LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
				Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
			})
			if err != nil {
				t.Fatal(err)
			}
			seen := map[LoadoutFamily]bool{}
			for _, alternative := range result.Alternatives {
				seen[EquipmentFamily(gameState.Inventory.Equipment[alternative.Equipment["1"]])] = true
			}
			if !maps.Equal(seen, test.wantFamilies) {
				t.Fatalf("families = %#v, want %#v", seen, test.wantFamilies)
			}
		})
	}
}

func optimizerFamilyTestState(family LoadoutFamily) State.GameState {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		item := optimizerTestItem(id, slot, 10)
		item.Relic = family == LoadoutFamilyRelic
		gameState.Inventory.Equipment[id] = item
	}
	return gameState
}

func optimizerTestGem(id State.GemInstanceID, value float64) State.GemInstance {
	return State.GemInstance{
		ID: id, CompatibleWearerID: 2, CombatMode: "pvp",
		Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: 9001, Values: []float64{value}}},
	}
}

func TestSnapshotFingerprintIgnoresUnrelatedStateAndTracksRelevantChanges(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 44
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		gameState.Inventory.Equipment[id] = optimizerTestItem(id, slot, float64(slot))
	}
	baseline, err := SnapshotFingerprint(gameState, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	unrelated := gameState
	unrelated.Revision++
	unrelated.Player.Level++
	unchanged, err := SnapshotFingerprint(unrelated, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != baseline {
		t.Fatal("unrelated player/revision update changed equipment fingerprint")
	}
	looseOffMode := gameState
	looseOffMode.Inventory.Gems = maps.Clone(gameState.Inventory.Gems)
	looseOffMode.Inventory.Gems[501] = State.GemInstance{ID: 501, CompatibleWearerID: 2, CombatMode: "pve", DefinitionID: 77}
	looseFingerprint, err := SnapshotFingerprint(looseOffMode, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if looseFingerprint != baseline {
		t.Fatal("loose off-mode gem changed PvP fingerprint")
	}
	attachedOffMode := looseOffMode
	attachedOffMode.Inventory.Gems = maps.Clone(looseOffMode.Inventory.Gems)
	gem := attachedOffMode.Inventory.Gems[501]
	gem.EquipmentInstanceID = 101
	attachedOffMode.Inventory.Gems[501] = gem
	attachedFingerprint, err := SnapshotFingerprint(attachedOffMode, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if attachedFingerprint == baseline {
		t.Fatal("off-mode socket on an eligible carrier did not change fingerprint")
	}
	appearance := gameState
	appearance.Commanders = maps.Clone(gameState.Commanders)
	appearance.Inventory.Equipment = maps.Clone(gameState.Inventory.Equipment)
	appearanceLeader := appearance.Commanders[0]
	appearanceLeader.Equipment = maps.Clone(appearanceLeader.Equipment)
	appearanceLeader.Equipment["5"] = 105
	appearance.Commanders[0] = appearanceLeader
	appearance.Inventory.Equipment[105] = State.EquipmentInstance{
		ID: 105, Slot: 5, TypeID: 2, RelicKnown: true, WearerKind: "commander", WearerID: 0,
	}
	appearanceFingerprint, err := SnapshotFingerprint(appearance, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if appearanceFingerprint == baseline {
		t.Fatal("retained appearance did not change fingerprint")
	}
	gemmedAppearance := appearance
	gemmedAppearance.Inventory.Gems = maps.Clone(appearance.Inventory.Gems)
	gemmedAppearance.Inventory.Gems[-505] = State.GemInstance{ID: -505, EquipmentInstanceID: 105}
	gemmedAppearanceFingerprint, err := SnapshotFingerprint(gemmedAppearance, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if gemmedAppearanceFingerprint == appearanceFingerprint {
		t.Fatal("retained appearance gem did not change fingerprint")
	}
	relevant := gameState
	item := relevant.Inventory.Equipment[101]
	item.Effects = append(State.EquipmentEffects(nil), item.Effects...)
	item.Effects[0].Values = append([]float64(nil), item.Effects[0].Values...)
	item.Effects[0].Values[0]++
	relevant.Inventory.Equipment[101] = item
	changed, err := SnapshotFingerprint(relevant, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if changed == baseline {
		t.Fatal("equipment effect change did not change fingerprint")
	}
	firstCatalog, err := GameData.DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "1", DigestSHA256: "first"})
	if err != nil {
		t.Fatal(err)
	}
	secondCatalog, err := GameData.DecodeStore([]byte(`{"versionInfo":{},"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "1", DigestSHA256: "second"})
	if err != nil {
		t.Fatal(err)
	}
	firstCatalogFingerprint, err := SnapshotFingerprint(gameState, firstCatalog, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	secondCatalogFingerprint, err := SnapshotFingerprint(gameState, secondCatalog, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	if firstCatalogFingerprint == secondCatalogFingerprint {
		t.Fatal("official catalog digest change did not change fingerprint")
	}
}

func TestOptimizeResultCountBounds(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		for variant := 1; variant <= 4; variant++ {
			id := State.EquipmentInstanceID(slot*100 + variant)
			gameState.Inventory.Equipment[id] = optimizerTestItem(id, slot, float64(variant))
		}
	}
	request := OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 100, Priorities: []Priority{{EffectID: 9001, Tier: 1}}}
	result, err := Optimize(gameState, nil, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alternatives) < 1 || len(result.Alternatives) > maximumResultCount {
		t.Fatalf("alternatives = %d, want bounded useful result count", len(result.Alternatives))
	}
	request.ResultCount = -1
	if _, err := Optimize(gameState, nil, request); err == nil {
		t.Fatal("negative resultCount unexpectedly succeeded")
	}
}

func optimizerTestItem(id State.EquipmentInstanceID, slot int, value float64) State.EquipmentInstance {
	return State.EquipmentInstance{
		ID: id, Slot: slot, TypeID: 2, RelicKnown: true,
		Effects: State.EquipmentEffects{{WireID: 1, DefinitionID: 9001, Values: []float64{value}}},
	}
}

func TestOptimizeScoresOfficialSetBonusesDuringSearch(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effects":[{"effectID":"9001"}],
		"equipment_sets":[{"setID":"77","neededItems":"2","effects":"9001&100"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Revision = 17
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		if slot <= 2 {
			gameState.Inventory.Equipment[State.EquipmentInstanceID(100+slot)] = optimizerTestItem(State.EquipmentInstanceID(100+slot), slot, 10)
			gameState.Inventory.Equipment[State.EquipmentInstanceID(200+slot)] = State.EquipmentInstance{
				ID: State.EquipmentInstanceID(200 + slot), Slot: slot, TypeID: 2, RelicKnown: true, SetID: 77,
			}
			continue
		}
		gameState.Inventory.Equipment[State.EquipmentInstanceID(100+slot)] = optimizerTestItem(State.EquipmentInstanceID(100+slot), slot, 0)
	}

	result, err := Optimize(gameState, gameData, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StateRevision != 17 {
		t.Fatalf("state revision = %d, want 17", result.StateRevision)
	}
	for slot := 1; slot <= 2; slot++ {
		want := State.EquipmentInstanceID(200 + slot)
		if got := result.Proposed.Equipment[strconv.Itoa(slot)]; got != want {
			t.Fatalf("set slot %d = %d, want %d", slot, got, want)
		}
	}
	if result.Proposed.Score != 1_000_000 {
		t.Fatalf("set loadout score = %.0f, want 1000000", result.Proposed.Score)
	}
}

func TestOptimizeCapsSharedEffectGroupIncludingSetBonus(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"23","maxTotalBonus":"90"}],
		"effects":[{"effectID":"9001","capID":"23"},{"effectID":"9002","capID":"23"}],
		"equipment_sets":[{"setID":"77","neededItems":"2","effects":"9002&60"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	gameState.Inventory.Equipment[101] = optimizerTestItem(101, 1, 60)
	item := gameState.Inventory.Equipment[101]
	item.SetID = 77
	gameState.Inventory.Equipment[101] = item
	gameState.Inventory.Equipment[102] = State.EquipmentInstance{ID: 102, Slot: 2, TypeID: 2, RelicKnown: true, SetID: 77}
	gameState.Inventory.Equipment[103] = State.EquipmentInstance{ID: 103, Slot: 3, TypeID: 2, RelicKnown: true}
	gameState.Inventory.Equipment[104] = State.EquipmentInstance{ID: 104, Slot: 4, TypeID: 2, RelicKnown: true}
	request := OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10,
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}, {EffectID: 9002, Tier: 1, Position: 0}},
	}
	first, err := Optimize(gameState, gameData, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Optimize(gameState, gameData, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Proposed.Score != 900_000 {
		t.Fatalf("shared-cap score = %.0f, want 900000", first.Proposed.Score)
	}
	total := 0.0
	for _, effect := range first.Proposed.Effects {
		if effect.DefinitionID == 9001 || effect.DefinitionID == 9002 {
			total += effect.Value
			if effect.CapID != 23 || effect.Cap == nil || *effect.Cap != 90 {
				t.Fatalf("effect cap metadata = %#v", effect)
			}
		}
	}
	if total != 90 {
		t.Fatalf("shared capped total = %.0f, want 90", total)
	}
	if !reflect.DeepEqual(first.Alternatives, second.Alternatives) {
		t.Fatal("shared-cap ranking was not stable")
	}
}

func TestOptimizeKeepsCurrentLoadoutWhenNoPriorityEffectIsAvailable(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		currentID := State.EquipmentInstanceID(100 + slot)
		alternativeID := State.EquipmentInstanceID(200 + slot)
		leader.Equipment[strconv.Itoa(slot)] = currentID
		current := optimizerTestItem(currentID, slot, 0)
		current.Effects[0].DefinitionID = 9002
		current.WearerKind, current.WearerID = "commander", 0
		alternative := optimizerTestItem(alternativeID, slot, 0)
		alternative.Effects[0].DefinitionID = 9002
		gameState.Inventory.Equipment[currentID] = current
		gameState.Inventory.Equipment[alternativeID] = alternative
	}
	gameState.Commanders[0] = leader

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Current.Score != 0 || result.Proposed.Score != 0 {
		t.Fatalf("scores = current %.0f, proposed %.0f", result.Current.Score, result.Proposed.Score)
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		if got, want := result.Proposed.Equipment[key], leader.Equipment[key]; got != want {
			t.Fatalf("slot %d = %d, want current equipment %d", slot, got, want)
		}
	}
}

func TestCoverageBonusIsAppliedOncePerGroupedPosition(t *testing.T) {
	grouped := []weightedPriority{
		{effectID: 9001, tier: 2, position: 0, weight: 100},
		{effectID: 9002, tier: 2, position: 0, weight: 100},
	}
	values := []float64{1, 2}
	if got, want := scoreValues(values, grouped, []effectCap{{}, {}}), 1300.0; got != want {
		t.Fatalf("grouped coverage score = %.0f, want %.0f", got, want)
	}
	if got, want := scoreEffectTotals(
		map[int64]float64{9001: 1, 9002: 2},
		grouped,
		map[int64]effectCap{},
	), 1300.0; got != want {
		t.Fatalf("grouped total score = %.0f, want %.0f", got, want)
	}

	separate := []weightedPriority{
		{effectID: 9001, tier: 2, position: 0, weight: 100},
		{effectID: 9002, tier: 2, position: 1, weight: 95},
	}
	if got, want := scoreValues(values, separate, []effectCap{{}, {}}), 2240.0; got != want {
		t.Fatalf("separate coverage score = %.0f, want %.0f", got, want)
	}
}

func TestBuildLoadoutUsesCompleteSemanticTotalsAndTargetScope(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"23","maxTotalBonus":"90"}],
		"effecttypes":[
			{"effectTypeID":"10","name":"MeleeAttack","sortCategory":"3","sortGroup":"1"},
			{"effectTypeID":"11","name":"UnitAmountYard","sortCategory":"8","sortGroup":"1"},
			{"effectTypeID":"12","name":"LegacyPVE","sortCategory":"9","sortGroup":"1"}
		],
		"effects":[
			{"effectID":"9001","effectTypeID":"10","capID":"23","areaTypeID":"1,2,3,4,5,6"},
			{"effectID":"9002","effectTypeID":"10","capID":"23","areaTypeID":"1,2,3,4,5,6"},
			{"effectID":"9003","effectTypeID":"11","areaTypeID":"1,2,3,4,5,6"},
			{"effectID":"9004","effectTypeID":"10","areaTypeID":"27"},
			{"effectID":"9005","effectTypeID":"11","areaTypeID":"1,2,3,4,5,6"},
			{"effectID":"9006","effectTypeID":"12","areaTypeID":"1,2,3,4,5,6"}
		],
		"equipment_sets":[{"setID":"77","neededItems":"2","effects":"9002&60"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, SetID: 77, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{60}}, {DefinitionID: 9005, Values: []float64{101, 2, 102, 3}}}}
	state.Inventory.Equipment[102] = State.EquipmentInstance{ID: 102, Slot: 2, TypeID: 2, SetID: 77}
	state.Inventory.Equipment[105] = State.EquipmentInstance{ID: 105, Slot: 5, TypeID: 2, Effects: State.EquipmentEffects{{DefinitionID: 9003, Values: []float64{2}}, {DefinitionID: 9004, Values: []float64{25}}}}
	state.Inventory.Gems[-501] = State.GemInstance{ID: -501, EquipmentInstanceID: 105, Effects: State.EquipmentEffects{{DefinitionID: 9003, Values: []float64{3}}}}
	equipment := map[string]State.EquipmentInstanceID{"1": 101, "2": 102, "5": 105}
	priorities := []weightedPriority{{effectID: 9001, tier: 1, position: 0, weight: 10_000}}
	rules := loadOfficialRules(gameData)
	base := buildLoadout(state, equipment, nil, priorities, rules, OptimizeRequest{CombatMode: "pve"})
	if len(base.Effects) != 4 {
		t.Fatalf("base effects = %#v", base.Effects)
	}
	var cappedTotal, appearanceTotal float64
	arguments := map[int64]float64{}
	for _, effect := range base.Effects {
		if effect.CapID == 23 {
			cappedTotal += effect.Value
		}
		if effect.DefinitionID == 9003 {
			appearanceTotal += effect.Value
		}
		if effect.ArgumentID != nil {
			arguments[*effect.ArgumentID] = effect.Value
		}
		if effect.DefinitionID == 9004 {
			t.Fatalf("event-only effect leaked into base profile: %#v", effect)
		}
	}
	if cappedTotal != 90 {
		t.Fatalf("shared capped total = %v, want 90", cappedTotal)
	}
	if appearanceTotal != 5 {
		t.Fatalf("retained appearance item+gem total = %v, want 5", appearanceTotal)
	}
	if arguments[101] != 2 || arguments[102] != 3 {
		t.Fatalf("argument totals = %#v", arguments)
	}
	event := buildLoadout(state, equipment, nil, priorities, rules, OptimizeRequest{CombatMode: "pve", TargetAreaTypeIDs: []int64{27}})
	foundEvent := false
	for _, effect := range event.Effects {
		if effect.DefinitionID == 9004 {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Fatalf("target-specific effect missing: %#v", event.Effects)
	}
}

func TestMaterialDifferenceUsesUnitAwareThresholds(t *testing.T) {
	percentage := func(value float64) []EffectTotal {
		cap := 150.0
		return []EffectTotal{{SemanticKey: "attack", Value: value, Unit: "percent", Precision: 1, Cap: &cap}}
	}
	if materiallyDifferent(percentage(150), percentage(150.2)) {
		t.Fatal("150% and 150.2% should collapse")
	}
	if !materiallyDifferent(percentage(150), percentage(155)) {
		t.Fatal("five percentage points should remain distinct")
	}
	count := func(value float64) []EffectTotal {
		return []EffectTotal{{SemanticKey: "waves", Value: value, Unit: "count", Precision: 0}}
	}
	if materiallyDifferent(count(4), count(4.5)) {
		t.Fatal("sub-count delta should collapse")
	}
	if !materiallyDifferent(count(4), count(5)) {
		t.Fatal("one-count delta should remain distinct")
	}
	if !materiallyDifferent([]EffectTotal{{SemanticKey: "unit:1", Value: 1, Unit: "categorical", Categorical: true}}, []EffectTotal{{SemanticKey: "unit:2", Value: 1, Unit: "categorical", Categorical: true}}) {
		t.Fatal("categorical substitution should remain distinct")
	}
}

func TestOptimizeCollapsesEquivalentIDsAndKeepsMeaningfulGemTradeoff(t *testing.T) {
	state := State.NewGameState()
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		for variant := 1; variant <= 20; variant++ {
			id := State.EquipmentInstanceID(slot*1000 + variant)
			state.Inventory.Equipment[id] = optimizerTestItem(id, slot, 10)
		}
	}
	for index := 1; index <= 4; index++ {
		state.Inventory.Gems[State.GemInstanceID(-500-index)] = optimizerTestGem(State.GemInstanceID(-500-index), 10)
	}
	tradeoff := optimizerTestGem(-600, 30)
	tradeoff.Effects[0].DefinitionID = 9002
	state.Inventory.Gems[-600] = tradeoff
	result, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alternatives) >= 10 {
		t.Fatalf("equivalent ID permutations padded results: %d", len(result.Alternatives))
	}
	if result.Alternatives[0].Score < 800_000 {
		t.Fatalf("strongest lane lost: %#v", result.Alternatives[0])
	}
	foundOneGemTradeoff := false
	for _, alternative := range result.Alternatives {
		if len(alternative.Gems) == 4 && containsGem(alternative.Gems, -600) {
			foundOneGemTradeoff = true
		}
	}
	if !foundOneGemTradeoff {
		t.Fatalf("meaningful one-gem tradeoff missing: %#v", result.Alternatives)
	}
	second, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Alternatives, second.Alternatives) {
		t.Fatal("useful alternatives are not deterministic")
	}
}

func containsGem(gems map[string]State.GemInstanceID, wanted State.GemInstanceID) bool {
	for _, id := range gems {
		if id == wanted {
			return true
		}
	}
	return false
}

func TestOptimizeMarksNoUsefulCurrentOutcome(t *testing.T) {
	state := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		item := optimizerTestItem(id, slot, 10)
		item.WearerKind, item.WearerID = "commander", 0
		state.Inventory.Equipment[id] = item
	}
	state.Commanders[0] = leader
	result, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NoUsefulChange || len(result.Alternatives) != 1 {
		t.Fatalf("no-useful result = %#v", result)
	}
	if !reflect.DeepEqual(result.Current.Equipment, result.Alternatives[0].Equipment) {
		t.Fatal("no-useful result did not retain current assignment")
	}
}

func TestBuildLoadoutCapsDistinctBucketsBeforeSemanticMerge(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"1","maxTotalBonus":"10"},{"capID":"2","maxTotalBonus":"20"}],
		"effecttypes":[{"effectTypeID":"10","name":"MeleeAttack"}],
		"effects":[
			{"effectID":"9001","effectTypeID":"10","capID":"1"},
			{"effectID":"9002","effectTypeID":"10","capID":"2"},
			{"effectID":"9003","effectTypeID":"10"}
		]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{
		{DefinitionID: 9001, Values: []float64{20}},
		{DefinitionID: 9002, Values: []float64{40}},
		{DefinitionID: 9003, Values: []float64{5}},
	}}
	rules := loadOfficialRules(gameData)
	for run := 0; run < 30; run++ {
		loadout := buildLoadout(state, map[string]State.EquipmentInstanceID{"1": 101}, nil, nil, rules, OptimizeRequest{CombatMode: "pvp"})
		if len(loadout.Effects) != 1 {
			t.Fatalf("run %d effects = %#v, want one semantic display outcome", run, loadout.Effects)
		}
		effect := loadout.Effects[0]
		if effect.Value != 35 || effect.RawValue != 65 || !effect.Capped || effect.CapID != 0 || effect.Cap != nil {
			t.Fatalf("run %d merged cap outcome = %#v, want value 35 raw 65 with mixed cap identity", run, effect)
		}
	}
}

func TestOptimizePreservesCapIdentityDuringCandidatePruning(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"1","maxTotalBonus":"10"},{"capID":"2","maxTotalBonus":"20"}],
		"effecttypes":[{"effectTypeID":"10","name":"MeleeAttack"}],
		"effects":[{"effectID":"9001","effectTypeID":"10","capID":"1"},{"effectID":"9002","effectTypeID":"10","capID":"2"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	state.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{10}}}}
	state.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9002, Values: []float64{10}}}}
	state.Inventory.Equipment[102] = State.EquipmentInstance{ID: 102, Slot: 2, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{10}}}}
	for slot := 3; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		state.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true}
	}
	result, err := Optimize(state, gameData, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposed.Equipment["1"] != 201 || result.Proposed.Score != 200_000 {
		t.Fatalf("cap-distinct candidate lane was pruned: %#v", result.Proposed)
	}
}

func TestOptimizeScoresOriginalDefinitionsAfterDisplayMerge(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effecttypes":[{"effectTypeID":"10","name":"MeleeAttack"}],
		"effects":[{"effectID":"9001","effectTypeID":"10"},{"effectID":"9002","effectTypeID":"10"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	current := State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, RelicKnown: true, WearerKind: "commander", Effects: State.EquipmentEffects{{DefinitionID: 9002, Values: []float64{90}}}}
	state.Inventory.Equipment[101] = current
	state.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{1}}, {DefinitionID: 9002, Values: []float64{100}}}}
	leader.Equipment["1"] = 101
	for slot := 2; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		state.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true, WearerKind: "commander"}
	}
	state.Commanders[0] = leader
	result, err := Optimize(state, gameData, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", Priorities: []Priority{{EffectID: 9002, Tier: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposed.Equipment["1"] != 201 || result.Proposed.Score != 1_000_000 {
		t.Fatalf("merged-display scoring chose %#v at score %.0f, want item 201 at 1000000", result.Proposed.Equipment, result.Proposed.Score)
	}
}

func TestOptimizeRetainsUsefulTradeoffBeyondNearVariantCutoff(t *testing.T) {
	state := State.NewGameState()
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for index := 0; index < 65; index++ {
		id := State.EquipmentInstanceID(1000 + index)
		state.Inventory.Equipment[id] = optimizerTestItem(id, 1, 100+float64(index)/100)
	}
	state.Inventory.Equipment[2000] = State.EquipmentInstance{ID: 2000, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{95}}, {DefinitionID: 9002, Values: []float64{20}}}}
	for slot := 2; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(2000 + slot)
		state.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true}
	}
	result, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, alternative := range result.Alternatives {
		found = found || alternative.Equipment["1"] == 2000
	}
	if !found {
		t.Fatalf("material tradeoff was pruned by near variants: %#v", result.Alternatives)
	}
}

func TestOptimizeScoreLanePreservesBestCombinationAcrossDiversityBuckets(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"1","maxTotalBonus":"150"}],
		"effects":[{"effectID":"9001","capID":"1"},{"effectID":"9002"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		aID := State.EquipmentInstanceID(slot*100 + 1)
		bID := State.EquipmentInstanceID(slot*100 + 2)
		state.Inventory.Equipment[aID] = State.EquipmentInstance{ID: aID, Slot: slot, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{100.8}}, {DefinitionID: 9002, Values: []float64{1.1}}}}
		state.Inventory.Equipment[bID] = State.EquipmentInstance{ID: bID, Slot: slot, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{100.7}}, {DefinitionID: 9002, Values: []float64{1.9}}}}
	}
	result, err := Optimize(state, gameData, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	for slot := 1; slot <= 4; slot++ {
		if got, want := result.Proposed.Equipment[strconv.Itoa(slot)], State.EquipmentInstanceID(slot*100+2); got != want {
			t.Fatalf("slot %d = %d, want score-lane item %d; proposed=%#v", slot, got, want, result.Proposed)
		}
	}
	if result.Proposed.Score != 1_501_760 {
		t.Fatalf("score-lane total = %.0f, want 1501760", result.Proposed.Score)
	}
}

func TestOptimizeCurrentFirstStillMarksSecondTradeoffUseful(t *testing.T) {
	state := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	current := optimizerTestItem(101, 1, 100)
	current.WearerKind = "commander"
	state.Inventory.Equipment[101] = current
	leader.Equipment["1"] = 101
	state.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{90}}, {DefinitionID: 9002, Values: []float64{20}}}}
	for slot := 2; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		state.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true, WearerKind: "commander"}
	}
	state.Commanders[0] = leader
	result, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}, {EffectID: 9002, Tier: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alternatives) < 2 || result.Alternatives[0].Useful || !result.Alternatives[1].Useful || result.NoUsefulChange {
		t.Fatalf("selected usefulness = %#v, noUsefulChange=%t", result.Alternatives, result.NoUsefulChange)
	}
}

func TestOptimizeRetainsUnknownDirectionReductionTradeoff(t *testing.T) {
	state := State.NewGameState()
	state.Commanders[0] = State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	state.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{100}}, {DefinitionID: 9002, Values: []float64{-10}}}}
	state.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{90}}, {DefinitionID: 9002, Values: []float64{-20}}}}
	for slot := 2; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		state.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true}
	}
	result, err := Optimize(state, nil, OptimizeRequest{LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", ResultCount: 10, Priorities: []Priority{{EffectID: 9001, Tier: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, alternative := range result.Alternatives {
		found = found || alternative.Equipment["1"] == 201
	}
	if !found {
		t.Fatalf("unknown-direction reduction tradeoff was treated as a pure loss: %#v", result.Alternatives)
	}
}

func TestOptimizeEventAreaOverridesLegacyIdentifierFallback(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effecttypes":[{"effectTypeID":"10","name":"MeleeAttack"}],
		"effects":[{"effectID":"9001","effectTypeID":"10","name":"LegacyMeleePVP","areaTypeID":"27"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Slot: 1, TypeID: 2, RelicKnown: true, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{40}}}}
	loadout := buildLoadout(state, map[string]State.EquipmentInstanceID{"1": 101}, nil, []weightedPriority{{effectID: 9001, tier: 1, weight: 10_000}}, loadOfficialRules(gameData), OptimizeRequest{CombatMode: "pve", TargetAreaTypeIDs: []int64{27}})
	if len(loadout.Effects) != 1 || loadout.Effects[0].Value != 40 || loadout.Score != 400_000 {
		t.Fatalf("event-area applicability diverged from client: %#v", loadout)
	}
}

func BenchmarkOptimizeLargeStorage(b *testing.B) {
	gameState := largeOptimizerState(1_000, 2_000)
	request := OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}, {EffectID: 9002, Tier: 2, Position: 0}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := Optimize(gameState, nil, request); err != nil {
			b.Fatal(err)
		}
	}
}

func largeOptimizerState(equipmentPerSlot int, gemCount int) State.GameState {
	gameState := State.NewGameState()
	leader := State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		for index := 0; index < equipmentPerSlot; index++ {
			id := State.EquipmentInstanceID(slot*1_000_000 + index)
			gameState.Inventory.Equipment[id] = State.EquipmentInstance{
				ID: id, Slot: slot, TypeID: 2, RelicKnown: true,
				Effects: State.EquipmentEffects{
					{WireID: 1, DefinitionID: 9001, Values: []float64{float64(index % 101)}},
					{WireID: 2, DefinitionID: 9002, Values: []float64{float64(index % 37)}},
				},
			}
			if index == 0 {
				leader.Equipment[strconv.Itoa(slot)] = id
				item := gameState.Inventory.Equipment[id]
				item.WearerKind, item.WearerID = "commander", 0
				gameState.Inventory.Equipment[id] = item
			}
		}
	}
	for index := 0; index < gemCount; index++ {
		id := -State.GemInstanceID(10_000_000 + index)
		gameState.Inventory.Gems[id] = State.GemInstance{
			ID: id, CompatibleWearerID: 2, CombatMode: "pvp",
			Effects: State.EquipmentEffects{
				{WireID: 301, DefinitionID: 9001, Values: []float64{float64(index % 67)}},
				{WireID: 302, DefinitionID: 9002, Values: []float64{float64(index % 29)}},
			},
		}
	}
	gameState.Commanders[0] = leader
	return gameState
}
