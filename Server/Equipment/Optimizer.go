package Equipment

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	equipmentCandidateLimit = 48
	gemCandidateLimit       = 64
	optimizerBeamWidth      = 512
	optimizerSlotCount      = 5
	gemSlotCount            = 4
)

var (
	optimizerSlots     = []int{1, 2, 3, 4, 6}
	officialRulesCache sync.Map // map[*GameData.Store]officialRules; game-data stores are immutable.
)

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

// scoringRules contains just the information that affects the requested
// priorities. Search candidates and partial loadouts do not need to carry the
// complete effect catalogue or repeatedly build effect-total maps.
type scoringRules struct {
	priorityIndex map[int64]int
	caps          []float64
	setBonuses    map[int64][]scoredSetBonus
	setPotential  map[int64]float64
}

type scoredSetBonus struct {
	neededItems int
	values      []float64
}

type optimizerCandidate struct {
	id     int64
	setID  int64
	values []float64
	rank   float64
}

// partialLoadout is intentionally value-only. The prior implementation cloned
// two maps and recomputed all equipment effects for every search branch. Fixed
// slot arrays make branching allocation-free and retain only the beam width.
type partialLoadout struct {
	equipment [optimizerSlotCount]*optimizerCandidate
	gems      [gemSlotCount]*optimizerCandidate
	score     float64
	changes   int
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
	scoring := buildScoringRules(priorities, rules)

	equipmentBySlot := candidateEquipment(gameState, request.LeaderKind, request.LeaderID, priorities, scoring)
	counts := CandidateCounts{EquipmentBySlot: map[string]int{}}
	beam := []partialLoadout{{}}
	for slotIndex, slot := range optimizerSlots {
		candidates := equipmentBySlot[slot]
		counts.EquipmentBySlot[strconv.Itoa(slot)] = len(candidates)
		if slot <= 4 && len(candidates) == 0 {
			return OptimizeResponse{}, fmt.Errorf("no eligible %s equipment exists for slot %d", request.LeaderKind, slot)
		}
		candidates = limitCandidates(candidates, equipmentCandidateLimit, map[int64]struct{}{
			int64(currentEquipment[strconv.Itoa(slot)]): {},
		})
		choices := candidatePointers(candidates, slot == 6)
		beam = expandEquipmentBeam(beam, slotIndex, choices, int64(currentEquipment[strconv.Itoa(slot)]), priorities, scoring)
	}

	gemCandidates := candidateGems(gameState, request.LeaderKind, request.LeaderID, request.CombatMode, priorities, scoring)
	counts.Gems = len(gemCandidates)
	currentGemIDs := make(map[int64]struct{}, len(currentGems))
	for _, id := range currentGems {
		if id > 0 {
			currentGemIDs[int64(id)] = struct{}{}
		}
	}
	gemCandidates = limitCandidates(gemCandidates, gemCandidateLimit, currentGemIDs)
	gemChoices := candidatePointers(gemCandidates, true)
	for slotIndex := range gemSlotCount {
		beam = expandGemBeam(beam, slotIndex, gemChoices, int64(currentGems[strconv.Itoa(slotIndex+1)]), priorities, scoring)
	}
	if len(beam) == 0 {
		return OptimizeResponse{}, fmt.Errorf("equipment optimizer found no valid loadout")
	}
	best := beam[0]
	proposedEquipment, proposedGems := best.assignments()
	return OptimizeResponse{
		LeaderKind: request.LeaderKind, LeaderID: request.LeaderID, StateRevision: gameState.Revision, Candidates: counts,
		Current:  buildLoadout(gameState, currentEquipment, currentGems, priorities, rules),
		Proposed: buildLoadout(gameState, proposedEquipment, proposedGems, priorities, rules),
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

func buildScoringRules(priorities []weightedPriority, rules officialRules) scoringRules {
	scoring := scoringRules{
		priorityIndex: make(map[int64]int, len(priorities)),
		caps:          make([]float64, len(priorities)),
		setBonuses:    make(map[int64][]scoredSetBonus, len(rules.setBonuses)),
		setPotential:  make(map[int64]float64, len(rules.setBonuses)),
	}
	for index, priority := range priorities {
		scoring.priorityIndex[priority.effectID] = index
		scoring.caps[index] = rules.caps[priority.effectID]
	}
	for setID, bonuses := range rules.setBonuses {
		for _, bonus := range bonuses {
			values := effectValues(bonus.effects, scoring.priorityIndex, len(priorities))
			if scoreValues(values, priorities, scoring.caps) == 0 {
				continue
			}
			scoring.setBonuses[setID] = append(scoring.setBonuses[setID], scoredSetBonus{
				neededItems: bonus.neededItems, values: values,
			})
			needed := bonus.neededItems
			if needed < 1 {
				needed = 1
			}
			scoring.setPotential[setID] += scoreValues(values, priorities, scoring.caps) / float64(needed)
		}
	}
	return scoring
}

func candidateEquipment(gameState State.GameState, kind string, leaderID int64, priorities []weightedPriority, scoring scoringRules) map[int][]optimizerCandidate {
	expectedType := 2
	if kind == "castellan" {
		expectedType = 1
	}
	result := map[int][]optimizerCandidate{}
	for _, item := range gameState.Inventory.Equipment {
		if item.TypeID != expectedType || !optimizerSlot(item.Slot) {
			continue
		}
		if item.WearerKind != "" && (item.WearerKind != kind || item.WearerID != leaderID) {
			continue
		}
		result[item.Slot] = append(result[item.Slot], makeCandidate(int64(item.ID), item.SetID, item.Effects, priorities, scoring))
	}
	return result
}

func candidateGems(gameState State.GameState, kind string, leaderID int64, combatMode string, priorities []weightedPriority, scoring scoringRules) []optimizerCandidate {
	result := make([]optimizerCandidate, 0, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if gem.WearerKind != "" && (gem.WearerKind != kind || gem.WearerID != leaderID) {
			continue
		}
		if !gemMatchesMode(gem, kind, combatMode) {
			continue
		}
		result = append(result, makeCandidate(int64(gem.ID), gem.SetID, gem.Effects, priorities, scoring))
	}
	return result
}

func makeCandidate(id int64, setID int64, effects State.EquipmentEffects, priorities []weightedPriority, scoring scoringRules) optimizerCandidate {
	values := effectValues(effects, scoring.priorityIndex, len(priorities))
	return optimizerCandidate{
		id: id, setID: setID, values: values,
		rank: scoreValues(values, priorities, scoring.caps) + scoring.setPotential[setID],
	}
}

func effectValues(effects State.EquipmentEffects, priorityIndex map[int64]int, count int) []float64 {
	values := make([]float64, count)
	for _, effect := range effects {
		if index, found := priorityIndex[effect.DefinitionID]; found {
			values[index] += effectMagnitude(effect.Values)
		}
	}
	return values
}

func limitCandidates(candidates []optimizerCandidate, limit int, retained map[int64]struct{}) []optimizerCandidate {
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].rank != candidates[right].rank {
			return candidates[left].rank > candidates[right].rank
		}
		return candidates[left].id < candidates[right].id
	})
	if len(candidates) <= limit {
		return candidates
	}
	result := make([]optimizerCandidate, 0, limit+len(retained))
	for index, candidate := range candidates {
		_, keep := retained[candidate.id]
		if index < limit || keep {
			result = append(result, candidate)
		}
	}
	return result
}

func candidatePointers(candidates []optimizerCandidate, includeEmpty bool) []*optimizerCandidate {
	result := make([]*optimizerCandidate, 0, len(candidates)+1)
	if includeEmpty {
		result = append(result, nil)
	}
	for index := range candidates {
		result = append(result, &candidates[index])
	}
	return result
}

func expandEquipmentBeam(beam []partialLoadout, slotIndex int, choices []*optimizerCandidate, currentID int64, priorities []weightedPriority, scoring scoringRules) []partialLoadout {
	builder := newBeamBuilder(optimizerBeamWidth)
	for _, partial := range beam {
		for _, item := range choices {
			candidate := partial
			candidate.equipment[slotIndex] = item
			candidate.changes += changeCount(item, currentID)
			candidate.score = scorePartial(candidate, priorities, scoring)
			builder.offer(candidate)
		}
	}
	return builder.finish()
}

func expandGemBeam(beam []partialLoadout, slotIndex int, choices []*optimizerCandidate, currentID int64, priorities []weightedPriority, scoring scoringRules) []partialLoadout {
	builder := newBeamBuilder(optimizerBeamWidth)
	for _, partial := range beam {
		for _, gem := range choices {
			if gem != nil && partial.hasGem(gem.id) {
				continue
			}
			candidate := partial
			candidate.gems[slotIndex] = gem
			candidate.changes += changeCount(gem, currentID)
			candidate.score = scorePartial(candidate, priorities, scoring)
			builder.offer(candidate)
		}
	}
	return builder.finish()
}

func changeCount(candidate *optimizerCandidate, currentID int64) int {
	if candidateID(candidate) != currentID {
		return 1
	}
	return 0
}

func scorePartial(loadout partialLoadout, priorities []weightedPriority, scoring scoringRules) float64 {
	var setIDs [optimizerSlotCount + gemSlotCount]int64
	var setCounts [optimizerSlotCount + gemSlotCount]int
	setLength := 0
	addCandidate := func(candidate *optimizerCandidate) {
		if candidate == nil || candidate.setID <= 0 {
			return
		}
		for index := 0; index < setLength; index++ {
			if setIDs[index] == candidate.setID {
				setCounts[index]++
				return
			}
		}
		setIDs[setLength] = candidate.setID
		setCounts[setLength] = 1
		setLength++
	}
	for _, item := range loadout.equipment {
		addCandidate(item)
	}
	for _, gem := range loadout.gems {
		addCandidate(gem)
	}

	score := 0.0
	for priorityIndex, priority := range priorities {
		value := 0.0
		for _, item := range loadout.equipment {
			if item != nil {
				value += item.values[priorityIndex]
			}
		}
		for _, gem := range loadout.gems {
			if gem != nil {
				value += gem.values[priorityIndex]
			}
		}
		for index := 0; index < setLength; index++ {
			for _, bonus := range scoring.setBonuses[setIDs[index]] {
				if setCounts[index] >= bonus.neededItems {
					value += bonus.values[priorityIndex]
				}
			}
		}
		if cap := scoring.caps[priorityIndex]; cap > 0 && value > cap {
			value = cap
		}
		if priority.tier == 2 && value > 0 {
			score += priority.weight * 10
		}
		score += value * priority.weight
	}
	return score
}

func (loadout partialLoadout) hasGem(id int64) bool {
	for _, gem := range loadout.gems {
		if candidateID(gem) == id {
			return true
		}
	}
	return false
}

func (loadout partialLoadout) assignments() (map[string]State.EquipmentInstanceID, map[string]State.GemInstanceID) {
	equipment := make(map[string]State.EquipmentInstanceID, optimizerSlotCount)
	for index, item := range loadout.equipment {
		if item != nil {
			equipment[strconv.Itoa(optimizerSlots[index])] = State.EquipmentInstanceID(item.id)
		}
	}
	gems := make(map[string]State.GemInstanceID, gemSlotCount)
	for index, gem := range loadout.gems {
		if gem != nil {
			gems[strconv.Itoa(index+1)] = State.GemInstanceID(gem.id)
		}
	}
	return equipment, gems
}

type beamBuilder struct {
	values    []partialLoadout
	limit     int
	heapified bool
}

func newBeamBuilder(limit int) beamBuilder {
	return beamBuilder{values: make([]partialLoadout, 0, limit), limit: limit}
}

// offer retains only the strongest beam-width branches. Once full, this is a
// worst-first heap, so expansion stays bounded rather than sorting every
// candidate combination for each slot.
func (builder *beamBuilder) offer(candidate partialLoadout) {
	if len(builder.values) < builder.limit {
		builder.values = append(builder.values, candidate)
		return
	}
	if !builder.heapified {
		builder.heapify()
		builder.heapified = true
	}
	if betterLoadout(candidate, builder.values[0]) {
		builder.values[0] = candidate
		builder.siftDown(0)
	}
}

func (builder *beamBuilder) finish() []partialLoadout {
	sort.Slice(builder.values, func(left, right int) bool {
		return betterLoadout(builder.values[left], builder.values[right])
	})
	return builder.values
}

func (builder *beamBuilder) heapify() {
	for index := len(builder.values)/2 - 1; index >= 0; index-- {
		builder.siftDown(index)
	}
}

func (builder *beamBuilder) siftDown(index int) {
	for {
		worst := index
		left := index*2 + 1
		right := left + 1
		if left < len(builder.values) && worseLoadout(builder.values[left], builder.values[worst]) {
			worst = left
		}
		if right < len(builder.values) && worseLoadout(builder.values[right], builder.values[worst]) {
			worst = right
		}
		if worst == index {
			return
		}
		builder.values[index], builder.values[worst] = builder.values[worst], builder.values[index]
		index = worst
	}
}

func betterLoadout(left partialLoadout, right partialLoadout) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.changes != right.changes {
		return left.changes < right.changes
	}
	for index := range left.equipment {
		if leftID, rightID := candidateID(left.equipment[index]), candidateID(right.equipment[index]); leftID != rightID {
			return leftID < rightID
		}
	}
	for index := range left.gems {
		if leftID, rightID := candidateID(left.gems[index]), candidateID(right.gems[index]); leftID != rightID {
			return leftID < rightID
		}
	}
	return false
}

func worseLoadout(left partialLoadout, right partialLoadout) bool {
	return betterLoadout(right, left)
}

func candidateID(candidate *optimizerCandidate) int64 {
	if candidate == nil {
		return 0
	}
	return candidate.id
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

func scoreValues(values []float64, priorities []weightedPriority, caps []float64) float64 {
	score := 0.0
	for index, priority := range priorities {
		value := values[index]
		if cap := caps[index]; cap > 0 && value > cap {
			value = cap
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
	if gameData == nil {
		return officialRules{caps: map[int64]float64{}, setBonuses: map[int64][]setBonus{}}
	}
	if cached, found := officialRulesCache.Load(gameData); found {
		return cached.(officialRules)
	}
	rules := buildOfficialRules(gameData)
	actual, _ := officialRulesCache.LoadOrStore(gameData, rules)
	return actual.(officialRules)
}

func buildOfficialRules(gameData *GameData.Store) officialRules {
	rules := officialRules{caps: map[int64]float64{}, setBonuses: map[int64][]setBonus{}}
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

func cloneMap[Key comparable, Value any](source map[Key]Value) map[Key]Value {
	result := make(map[Key]Value, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
