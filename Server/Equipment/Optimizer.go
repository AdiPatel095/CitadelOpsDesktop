package Equipment

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	equipmentCandidateLimit = 48
	gemCandidateLimit       = 64
	optimizerBeamWidth      = 512
)

var optimizerSlots = []int{1, 2, 3, 4, 6}

type officialRules struct {
	caps       map[int64]float64
	setBonuses map[int64][]setBonus
}

type setBonus struct {
	neededItems int
	effects     State.EquipmentEffects
}

type weightedPriority struct {
	effectID int64
	tier     int
	position int
	weight   float64
}

type partialLoadout struct {
	equipment map[string]State.EquipmentInstanceID
	gems      map[string]State.GemInstanceID
	score     float64
	changes   int
	key       string
}

func Optimize(gameState State.GameState, gameData *GameData.Store, request OptimizeRequest) (OptimizeResponse, error) {
	request.LeaderKind = strings.ToLower(strings.TrimSpace(request.LeaderKind))
	request.CombatMode = strings.ToLower(strings.TrimSpace(request.CombatMode))
	if request.CombatMode != "pvp" && request.CombatMode != "pve" {
		return OptimizeResponse{}, fmt.Errorf("combatMode must be pvp or pve")
	}
	currentEquipment, currentGems, err := currentLeaderLoadout(gameState, request.LeaderKind, request.LeaderID)
	if err != nil {
		return OptimizeResponse{}, err
	}
	priorities, err := preparePriorities(gameData, request.Priorities)
	if err != nil {
		return OptimizeResponse{}, err
	}
	rules := loadOfficialRules(gameData)

	equipmentBySlot := candidateEquipment(gameState, request.LeaderKind, request.LeaderID)
	counts := CandidateCounts{EquipmentBySlot: map[string]int{}}
	for _, slot := range optimizerSlots {
		counts.EquipmentBySlot[strconv.Itoa(slot)] = len(equipmentBySlot[slot])
		if slot <= 4 && len(equipmentBySlot[slot]) == 0 {
			return OptimizeResponse{}, fmt.Errorf("no eligible %s equipment exists for slot %d", request.LeaderKind, slot)
		}
		sortEquipmentCandidates(equipmentBySlot[slot], priorities, rules)
		if len(equipmentBySlot[slot]) > equipmentCandidateLimit {
			equipmentBySlot[slot] = equipmentBySlot[slot][:equipmentCandidateLimit]
		}
	}

	beam := []partialLoadout{{equipment: map[string]State.EquipmentInstanceID{}, gems: map[string]State.GemInstanceID{}}}
	for _, slot := range optimizerSlots {
		candidates := equipmentBySlot[slot]
		if slot == 6 {
			candidates = append([]State.EquipmentInstance{{}}, candidates...)
		}
		next := make([]partialLoadout, 0, len(beam)*len(candidates))
		for _, partial := range beam {
			for _, item := range candidates {
				candidate := clonePartial(partial)
				if item.ID > 0 {
					candidate.equipment[strconv.Itoa(slot)] = item.ID
				}
				candidate.score = scoreAssignments(gameState, candidate.equipment, candidate.gems, priorities, rules)
				candidate.changes = assignmentChanges(candidate.equipment, candidate.gems, currentEquipment, currentGems)
				candidate.key = assignmentKey(candidate.equipment, candidate.gems)
				next = append(next, candidate)
			}
		}
		beam = trimBeam(next, optimizerBeamWidth)
	}

	gemCandidates := candidateGems(gameState, request.LeaderKind, request.LeaderID, request.CombatMode)
	counts.Gems = len(gemCandidates)
	sortGemCandidates(gemCandidates, priorities, rules)
	if len(gemCandidates) > gemCandidateLimit {
		gemCandidates = gemCandidates[:gemCandidateLimit]
	}
	for slot := 1; slot <= 4 && len(gemCandidates) > 0; slot++ {
		next := make([]partialLoadout, 0, len(beam)*(len(gemCandidates)+1))
		for _, partial := range beam {
			none := clonePartial(partial)
			none.changes = assignmentChanges(none.equipment, none.gems, currentEquipment, currentGems)
			none.key = assignmentKey(none.equipment, none.gems)
			next = append(next, none)
			for _, gem := range gemCandidates {
				if partialHasGem(partial, gem.ID) {
					continue
				}
				candidate := clonePartial(partial)
				candidate.gems[strconv.Itoa(slot)] = gem.ID
				candidate.score = scoreAssignments(gameState, candidate.equipment, candidate.gems, priorities, rules)
				candidate.changes = assignmentChanges(candidate.equipment, candidate.gems, currentEquipment, currentGems)
				candidate.key = assignmentKey(candidate.equipment, candidate.gems)
				next = append(next, candidate)
			}
		}
		beam = trimBeam(next, optimizerBeamWidth)
	}
	if len(beam) == 0 {
		return OptimizeResponse{}, fmt.Errorf("equipment optimizer found no valid loadout")
	}
	best := beam[0]
	return OptimizeResponse{
		LeaderKind: request.LeaderKind, LeaderID: request.LeaderID, Candidates: counts,
		Current:  buildLoadout(gameState, currentEquipment, currentGems, priorities, rules),
		Proposed: buildLoadout(gameState, best.equipment, best.gems, priorities, rules),
	}, nil
}

func currentLeaderLoadout(gameState State.GameState, kind string, id int64) (map[string]State.EquipmentInstanceID, map[string]State.GemInstanceID, error) {
	switch kind {
	case "commander":
		leader, ok := gameState.Commanders[State.CommanderID(id)]
		if !ok {
			return nil, nil, fmt.Errorf("commander %d is not in current state", id)
		}
		return cloneMap(leader.Equipment), cloneMap(leader.Gems), nil
	case "castellan":
		leader, ok := gameState.Castellans[State.CastellanID(id)]
		if !ok {
			return nil, nil, fmt.Errorf("castellan %d is not in current state", id)
		}
		return cloneMap(leader.Equipment), cloneMap(leader.Gems), nil
	default:
		return nil, nil, fmt.Errorf("leaderKind must be commander or castellan")
	}
}

func preparePriorities(gameData *GameData.Store, input []Priority) ([]weightedPriority, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("at least one effect priority is required")
	}
	seen := map[int64]struct{}{}
	result := make([]weightedPriority, 0, len(input))
	var effects *GameData.Catalog
	if gameData != nil {
		effects, _ = gameData.Catalog("effects")
	}
	for _, priority := range input {
		if priority.EffectID <= 0 || priority.Tier < 1 || priority.Tier > 2 || priority.Position < 0 {
			return nil, fmt.Errorf("effect priorities require a positive effectId, tier 1 or 2, and non-negative position")
		}
		if _, duplicate := seen[priority.EffectID]; duplicate {
			return nil, fmt.Errorf("effect %d appears more than once", priority.EffectID)
		}
		if effects != nil {
			if _, found := effects.Find(strconv.FormatInt(priority.EffectID, 10)); !found {
				return nil, fmt.Errorf("effect %d is not in the official effect catalog", priority.EffectID)
			}
		}
		seen[priority.EffectID] = struct{}{}
		base, decay := 10_000.0, 0.90
		if priority.Tier == 2 {
			base, decay = 100.0, 0.95
		}
		result = append(result, weightedPriority{
			effectID: priority.EffectID, tier: priority.Tier, position: priority.Position,
			weight: base * math.Pow(decay, float64(priority.Position)),
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].tier != result[right].tier {
			return result[left].tier < result[right].tier
		}
		if result[left].position != result[right].position {
			return result[left].position < result[right].position
		}
		return result[left].effectID < result[right].effectID
	})
	return result, nil
}

func candidateEquipment(gameState State.GameState, kind string, leaderID int64) map[int][]State.EquipmentInstance {
	expectedType := 2
	if kind == "castellan" {
		expectedType = 1
	}
	result := map[int][]State.EquipmentInstance{}
	for _, item := range gameState.Inventory.Equipment {
		if item.TypeID != expectedType || !optimizerSlot(item.Slot) {
			continue
		}
		if item.WearerKind != "" && (item.WearerKind != kind || item.WearerID != leaderID) {
			continue
		}
		result[item.Slot] = append(result[item.Slot], item)
	}
	return result
}

func candidateGems(gameState State.GameState, kind string, leaderID int64, combatMode string) []State.GemInstance {
	result := make([]State.GemInstance, 0, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if gem.WearerKind != "" && (gem.WearerKind != kind || gem.WearerID != leaderID) {
			continue
		}
		if !gemMatchesMode(gem, kind, combatMode) {
			continue
		}
		result = append(result, gem)
	}
	return result
}

func gemMatchesMode(gem State.GemInstance, kind string, combatMode string) bool {
	expectedWearerID := 2
	if kind == "castellan" {
		expectedWearerID = 1
	}
	if gem.CompatibleWearerID > 0 && gem.CompatibleWearerID != expectedWearerID {
		return false
	}
	if gem.CombatMode == "pvp" || gem.CombatMode == "pve" {
		return gem.CombatMode == combatMode
	}
	if len(gem.Effects) == 0 {
		return false
	}
	wireID := gem.Effects[0].WireID
	pvp := combatMode == "pvp"
	if kind == "castellan" {
		if pvp {
			return wireID >= 10300 && wireID < 10400
		}
		return wireID >= 10200 && wireID < 10300
	}
	if pvp {
		return wireID >= 300 && wireID < 400
	}
	return wireID >= 200 && wireID < 300
}

func sortEquipmentCandidates(items []State.EquipmentInstance, priorities []weightedPriority, rules officialRules) {
	sort.Slice(items, func(left, right int) bool {
		leftScore := scoreEffects(items[left].Effects, priorities, rules.caps)
		rightScore := scoreEffects(items[right].Effects, priorities, rules.caps)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return items[left].ID < items[right].ID
	})
}

func sortGemCandidates(gems []State.GemInstance, priorities []weightedPriority, rules officialRules) {
	sort.Slice(gems, func(left, right int) bool {
		leftScore := scoreEffects(gems[left].Effects, priorities, rules.caps)
		rightScore := scoreEffects(gems[right].Effects, priorities, rules.caps)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return gems[left].ID < gems[right].ID
	})
}

func scoreAssignments(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	priorities []weightedPriority,
	rules officialRules,
) float64 {
	effects := assignmentEffects(gameState, equipment, gems, rules)
	return scoreEffectTotals(effects, priorities, rules.caps)
}

func scoreEffects(effects State.EquipmentEffects, priorities []weightedPriority, caps map[int64]float64) float64 {
	totals := map[int64]float64{}
	for _, effect := range effects {
		totals[effect.DefinitionID] += effectMagnitude(effect.Values)
	}
	return scoreEffectTotals(totals, priorities, caps)
}

func scoreEffectTotals(totals map[int64]float64, priorities []weightedPriority, caps map[int64]float64) float64 {
	score := 0.0
	for _, priority := range priorities {
		value := totals[priority.effectID]
		if capValue := caps[priority.effectID]; capValue > 0 && value > capValue {
			value = capValue
		}
		if priority.tier == 2 && value > 0 {
			score += priority.weight * 10
		}
		score += value * priority.weight
	}
	return score
}

func assignmentEffects(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	rules officialRules,
) map[int64]float64 {
	totals := map[int64]float64{}
	setCounts := map[int64]int{}
	for _, id := range equipment {
		item, ok := gameState.Inventory.Equipment[id]
		if !ok {
			continue
		}
		addEffects(totals, item.Effects)
		if item.SetID > 0 {
			setCounts[item.SetID]++
		}
	}
	for _, id := range gems {
		gem, ok := gameState.Inventory.Gems[id]
		if ok {
			addEffects(totals, gem.Effects)
			if gem.SetID > 0 {
				setCounts[gem.SetID]++
			}
		}
	}
	for setID, count := range setCounts {
		for _, bonus := range rules.setBonuses[setID] {
			if count >= bonus.neededItems {
				addEffects(totals, bonus.effects)
			}
		}
	}
	return totals
}

func addEffects(totals map[int64]float64, effects State.EquipmentEffects) {
	for _, effect := range effects {
		totals[effect.DefinitionID] += effectMagnitude(effect.Values)
	}
}

func effectMagnitude(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	if len(values)%2 == 0 {
		looksPaired := true
		for index := 0; index < len(values); index += 2 {
			if values[index] < 1 || math.Trunc(values[index]) != values[index] {
				looksPaired = false
				break
			}
		}
		if looksPaired {
			best := values[1]
			for index := 3; index < len(values); index += 2 {
				if values[index] > best {
					best = values[index]
				}
			}
			return best
		}
	}
	return values[len(values)-1]
}

func buildLoadout(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	priorities []weightedPriority,
	rules officialRules,
) Loadout {
	totals := assignmentEffects(gameState, equipment, gems, rules)
	effects := make([]EffectTotal, 0, len(totals))
	for definitionID, rawValue := range totals {
		value := rawValue
		var capPointer *float64
		capped := false
		if capValue := rules.caps[definitionID]; capValue > 0 {
			capCopy := capValue
			capPointer = &capCopy
			if value > capValue {
				value = capValue
				capped = true
			}
		}
		effects = append(effects, EffectTotal{DefinitionID: definitionID, Value: value, Cap: capPointer, Capped: capped})
	}
	sort.Slice(effects, func(left, right int) bool { return effects[left].DefinitionID < effects[right].DefinitionID })
	return Loadout{
		Equipment: cloneMap(equipment), Gems: cloneMap(gems), Effects: effects,
		Score: scoreEffectTotals(totals, priorities, rules.caps),
	}
}

func loadOfficialRules(gameData *GameData.Store) officialRules {
	rules := officialRules{caps: map[int64]float64{}, setBonuses: map[int64][]setBonus{}}
	if gameData == nil {
		return rules
	}
	capByID := map[int64]float64{}
	if catalog, err := gameData.Catalog("effectCaps"); err == nil {
		for _, raw := range catalog.Rows() {
			record, decodeErr := GameData.DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, idOK := record.Int64("capID")
			value, valueOK := record.Float64("maxTotalBonus")
			if idOK && valueOK && value > 0 {
				capByID[id] = value
			}
		}
	}
	if catalog, err := gameData.Catalog("effects"); err == nil {
		for _, raw := range catalog.Rows() {
			record, decodeErr := GameData.DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			effectID, effectOK := record.Int64("effectID")
			capID, capOK := record.Int64("capID")
			if effectOK && capOK && capByID[capID] > 0 {
				rules.caps[effectID] = capByID[capID]
			}
		}
	}
	if catalog, err := gameData.Catalog("equipment_sets"); err == nil {
		for _, raw := range catalog.Rows() {
			record, decodeErr := GameData.DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			setID, setOK := record.Int64("setID")
			needed, neededOK := record.Int64("neededItems")
			effects, effectsOK := record.String("effects")
			if !setOK || !neededOK || !effectsOK {
				continue
			}
			parsed := parseOfficialEffects(effects, func(wireID int64) int64 {
				return officialNormalEffectID(gameData, wireID)
			})
			if len(parsed) > 0 {
				rules.setBonuses[setID] = append(rules.setBonuses[setID], setBonus{neededItems: int(needed), effects: parsed})
			}
		}
	}
	return rules
}

func officialNormalEffectID(gameData *GameData.Store, wireID int64) int64 {
	catalog, err := gameData.Catalog("equipment_effects")
	if err != nil {
		return wireID
	}
	raw, ok := catalog.Find(strconv.FormatInt(wireID, 10))
	if !ok {
		return wireID
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return wireID
	}
	effectID, ok := record.Int64("effectID")
	if !ok || effectID <= 0 {
		return wireID
	}
	return effectID
}

func optimizerSlot(slot int) bool {
	for _, allowed := range optimizerSlots {
		if allowed == slot {
			return true
		}
	}
	return false
}

func clonePartial(source partialLoadout) partialLoadout {
	return partialLoadout{equipment: cloneMap(source.equipment), gems: cloneMap(source.gems), score: source.score, changes: source.changes, key: source.key}
}

func partialHasGem(partial partialLoadout, id State.GemInstanceID) bool {
	for _, current := range partial.gems {
		if current == id {
			return true
		}
	}
	return false
}

func trimBeam(beam []partialLoadout, limit int) []partialLoadout {
	sort.Slice(beam, func(left, right int) bool {
		if beam[left].score != beam[right].score {
			return beam[left].score > beam[right].score
		}
		if beam[left].changes != beam[right].changes {
			return beam[left].changes < beam[right].changes
		}
		return beam[left].key < beam[right].key
	})
	if len(beam) > limit {
		beam = beam[:limit]
	}
	return beam
}

func assignmentChanges(
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	currentEquipment map[string]State.EquipmentInstanceID,
	currentGems map[string]State.GemInstanceID,
) int {
	changes := 0
	for _, slot := range optimizerSlots {
		key := strconv.Itoa(slot)
		if equipment[key] != currentEquipment[key] {
			changes++
		}
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		if gems[key] != currentGems[key] {
			changes++
		}
	}
	return changes
}

func assignmentKey(equipment map[string]State.EquipmentInstanceID, gems map[string]State.GemInstanceID) string {
	values := make([]string, 0, 9)
	for _, slot := range optimizerSlots {
		values = append(values, fmt.Sprintf("%020d", equipment[strconv.Itoa(slot)]))
	}
	for slot := 1; slot <= 4; slot++ {
		values = append(values, fmt.Sprintf("%020d", gems[strconv.Itoa(slot)]))
	}
	return strings.Join(values, ":")
}

func cloneMap[Key comparable, Value any](source map[Key]Value) map[Key]Value {
	result := make(map[Key]Value, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
