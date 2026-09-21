package Equipment

import (
	"CitadelDesktop/Server/Localization"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
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
	maximumResultCount      = 10
)

var (
	optimizerSlots     = []int{1, 2, 3, 4, 6}
	officialRulesCache sync.Map // map[*GameData.Store]officialRules; game-data stores are immutable.
)

type officialRules struct {
	caps          map[int64]effectCap
	setBonuses    map[int64][]setBonus
	effects       map[int64]effectDefinition
	pvpAreaScores map[int64]int
	pveAreaScores map[int64]int
}

type effectDefinition struct {
	effectTypeID    int64
	name            string
	unit            string
	precision       int
	categorical     bool
	areaTypeIDs     []int64
	scope           string
	structuredScope string
}

type semanticEffect struct {
	identity     string
	definitionID int64
	argumentID   int64
	value        float64
	rawValue     float64
	unit         string
	precision    int
	categorical  bool
	cap          effectCap
}

type effectCap struct {
	id  int64
	max float64
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

// A grouped client priority assigns the same tier and position to every
// official effect ID in one top-level effect group. Tier 2's presence bonus is
// therefore awarded once for the row, while every effect still contributes
// its own capped value to the score. Legacy clients used unique positions, so
// their behavior remains unchanged.
type coverageGroupTracker struct {
	position int
	present  bool
	weight   float64
}

func newCoverageGroupTracker() coverageGroupTracker {
	return coverageGroupTracker{position: -1}
}

func (tracker *coverageGroupTracker) observe(priority weightedPriority, value float64) float64 {
	if priority.tier != 2 {
		return 0
	}
	bonus := 0.0
	if tracker.position != priority.position {
		bonus = tracker.finish()
		tracker.position = priority.position
		tracker.weight = priority.weight
	}
	if value > 0 {
		tracker.present = true
	}
	return bonus
}

func (tracker *coverageGroupTracker) finish() float64 {
	bonus := 0.0
	if tracker.position >= 0 && tracker.present {
		bonus = tracker.weight * 10
	}
	tracker.present = false
	return bonus
}

// scoringRules contains just the information that affects the requested
// priorities. Search candidates and partial loadouts do not need to carry the
// complete effect catalogue or repeatedly build effect-total maps.
type scoringRules struct {
	priorityIndex map[int64]int
	caps          []effectCap
	capGroups     []scoredCapGroup
	setBonuses    map[int64][]scoredSetBonus
	setPotential  map[int64]float64
}

type scoredCapGroup struct {
	maximum float64
	indexes []int
}

type scoredSetBonus struct {
	neededItems int
	values      []float64
}

type optimizerCandidate struct {
	id        int64
	setID     int64
	family    LoadoutFamily
	values    []float64
	rank      float64
	signature string
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
		return OptimizeResponse{}, Localization.WithError(fmt.Errorf("combatMode must be pvp or pve"), Localization.New("server.equipment.combatmode_must_be_pvp.4140c885", "combatMode must be pvp or pve", nil))
	}
	if request.ResultCount < 0 {
		return OptimizeResponse{}, Localization.WithError(fmt.Errorf("resultCount cannot be negative"), Localization.New("server.equipment.resultcount_cannot_be_negative.95f2697f", "resultCount cannot be negative", nil))
	}
	if request.ResultCount == 0 {
		request.ResultCount = 1
	}
	if request.ResultCount > maximumResultCount {
		request.ResultCount = maximumResultCount
	}
	for _, areaTypeID := range request.TargetAreaTypeIDs {
		if areaTypeID <= 0 {
			return OptimizeResponse{}, Localization.WithError(fmt.Errorf("targetAreaTypeIds require positive official area type IDs"), Localization.New("server.equipment.targetareatypeids_require_positive_official.afe30c77", "targetAreaTypeIds require positive official area type IDs", nil))
		}
	}
	currentEquipment, currentGems, err := currentLeaderLoadout(gameState, request.LeaderKind, request.LeaderID)
	if err != nil {
		return OptimizeResponse{}, err
	}
	appearanceFamily, appearanceRestrictsFamily, err := RetainedAppearanceFamily(gameState, currentEquipment)
	if err != nil {
		return OptimizeResponse{}, err
	}
	priorities, err := preparePriorities(gameData, request.Priorities)
	if err != nil {
		return OptimizeResponse{}, err
	}
	rules := loadOfficialRules(gameData)
	scoring := buildScoringRules(priorities, rules)

	equipmentBySlot := candidateEquipment(gameState, request.LeaderKind, request.LeaderID, priorities, scoring, rules, request)
	counts := CandidateCounts{EquipmentBySlot: map[string]int{}}
	for _, slot := range optimizerSlots {
		candidates := equipmentBySlot[slot]
		counts.EquipmentBySlot[strconv.Itoa(slot)] = len(candidates)
		if slot <= 4 && len(candidates) == 0 {
			return OptimizeResponse{}, Localization.WithError(fmt.Errorf("no eligible %s equipment exists for slot %d", request.LeaderKind, slot), Localization.New("server.equipment.no_eligible_p_equipment.4934ad8d", "no eligible {p0} equipment exists for slot {p1}", Localization.Params{"p0": fmt.Sprintf("%s", request.LeaderKind), "p1": slot}))
		}
	}

	gemCandidates := candidateGems(gameState, request.LeaderKind, request.LeaderID, request.CombatMode, priorities, scoring, rules, request)
	counts.Gems = len(gemCandidates)
	beam := make([]partialLoadout, 0, optimizerBeamWidth*2)
	for _, family := range []LoadoutFamily{LoadoutFamilyOrdinary, LoadoutFamilyRelic} {
		if appearanceRestrictsFamily && family != appearanceFamily {
			continue
		}
		beam = append(beam, optimizeFamily(
			equipmentBySlot, gemCandidates, family, currentEquipment, currentGems, priorities, scoring, true,
		)...)
		beam = append(beam, optimizeFamily(
			equipmentBySlot, gemCandidates, family, currentEquipment, currentGems, priorities, scoring, false,
		)...)
	}
	if len(beam) == 0 {
		return OptimizeResponse{}, Localization.WithError(fmt.Errorf("equipment optimizer found no valid loadout"), Localization.New("server.equipment.equipment_optimizer_found_no.3f49c64a", "equipment optimizer found no valid loadout", nil))
	}
	sort.Slice(beam, func(left, right int) bool { return betterLoadout(beam[left], beam[right]) })
	alternatives := make([]Loadout, 0, request.ResultCount)
	seenAssignments := make(map[string]struct{}, request.ResultCount)
	type evaluatedCandidate struct {
		loadout Loadout
		key     string
	}
	evaluated := make([]evaluatedCandidate, 0, len(beam))
	for _, candidate := range beam {
		proposedEquipment, proposedGems := candidate.assignments()
		// Appearance is immutable in Reconfigure, but its item, socketed gem and
		// set membership still contribute to the authoritative comparison.
		if appearanceID := currentEquipment["5"]; appearanceID != 0 {
			proposedEquipment["5"] = appearanceID
		}
		assignmentKey := loadoutAssignmentKey(proposedEquipment, proposedGems)
		if _, duplicate := seenAssignments[assignmentKey]; duplicate {
			continue
		}
		seenAssignments[assignmentKey] = struct{}{}
		transition, transitionErr := BuildReconfigurationTransition(gameState, currentEquipment, proposedEquipment, proposedGems)
		if transitionErr != nil {
			return OptimizeResponse{}, transitionErr
		}
		quote, quoteErr := QuoteReconfiguration(gameData, transition)
		if quoteErr != nil {
			return OptimizeResponse{}, quoteErr
		}
		loadout := buildLoadout(gameState, proposedEquipment, proposedGems, priorities, rules, request)
		loadout.ExtractionCost = quote
		evaluated = append(evaluated, evaluatedCandidate{loadout: loadout, key: semanticOutcomeKey(loadout.Effects)})
	}
	if len(evaluated) == 0 {
		return OptimizeResponse{}, Localization.WithError(fmt.Errorf("equipment optimizer found no distinct loadout"), Localization.New("server.equipment.equipment_optimizer_found_no.d5319520", "equipment optimizer found no distinct loadout", nil))
	}
	// Collapse exact effective outcomes first. A cheaper trusted quote wins;
	// equal-cost outcomes retain the optimizer's deterministic assignment order.
	exact := make([]evaluatedCandidate, 0, len(evaluated))
	exactIndex := map[string]int{}
	for _, candidate := range evaluated {
		if index, found := exactIndex[candidate.key]; found {
			if candidate.loadout.ExtractionCost.MaximumRubySpend < exact[index].loadout.ExtractionCost.MaximumRubySpend {
				exact[index] = candidate
			}
			continue
		}
		exactIndex[candidate.key] = len(exact)
		exact = append(exact, candidate)
	}
	sort.SliceStable(exact, func(left, right int) bool {
		if exact[left].loadout.Score != exact[right].loadout.Score {
			return exact[left].loadout.Score > exact[right].loadout.Score
		}
		return loadoutAssignmentKey(exact[left].loadout.Equipment, exact[left].loadout.Gems) < loadoutAssignmentKey(exact[right].loadout.Equipment, exact[right].loadout.Gems)
	})
	current := buildLoadout(gameState, currentEquipment, currentGems, priorities, rules, request)
	for _, candidate := range exact {
		useful := len(alternatives) == 0 || materiallyDifferent(candidate.loadout.Effects, alternatives[0].Effects) || candidate.loadout.ExtractionCost.MaximumRubySpend < alternatives[0].ExtractionCost.MaximumRubySpend
		if !useful {
			continue
		}
		if len(alternatives) > 0 && isDominatedOutcome(candidate.loadout, alternatives[0], priorities) {
			continue
		}
		nearExisting := false
		for _, selected := range alternatives {
			if !materiallyDifferent(candidate.loadout.Effects, selected.Effects) && candidate.loadout.ExtractionCost.MaximumRubySpend >= selected.ExtractionCost.MaximumRubySpend {
				nearExisting = true
				break
			}
		}
		if nearExisting {
			continue
		}
		candidate.loadout.Useful = materiallyDifferent(current.Effects, candidate.loadout.Effects) || candidate.loadout.ExtractionCost.MaximumRubySpend < current.ExtractionCost.MaximumRubySpend
		candidate.loadout.Reason = loadoutReason(current, alternatives, candidate.loadout)
		alternatives = append(alternatives, candidate.loadout)
		if len(alternatives) == request.ResultCount {
			break
		}
	}
	if len(alternatives) == 0 {
		alternatives = append(alternatives, exact[0].loadout)
	}
	fingerprint, err := SnapshotFingerprint(gameState, gameData, request.LeaderKind, request.LeaderID, request.CombatMode)
	if err != nil {
		return OptimizeResponse{}, err
	}
	for index := range alternatives {
		alternative := &alternatives[index]
		alternative.ExtractionCost.Fingerprint = ReconfigurationQuoteFingerprint(
			fingerprint, alternative.Equipment, alternative.Gems, alternative.ExtractionCost,
		)
	}
	noUsefulChange := true
	for _, alternative := range alternatives {
		if alternative.Useful {
			noUsefulChange = false
			break
		}
	}
	return OptimizeResponse{
		LeaderKind: request.LeaderKind, LeaderID: request.LeaderID, StateRevision: gameState.Revision,
		SnapshotFingerprint: fingerprint, Candidates: counts,
		Current: current, Proposed: alternatives[0], Alternatives: alternatives,
		NoUsefulChange: noUsefulChange,
	}, nil
}

// SnapshotFingerprint identifies only state that can change optimizer results
// or make a selected assignment unsafe to apply. Global state revisions are
// intentionally excluded so unrelated automation and UI updates do not expire
// an otherwise current equipment preview.
func SnapshotFingerprint(gameState State.GameState, gameData *GameData.Store, kind string, leaderID int64, combatMode string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	combatMode = strings.ToLower(strings.TrimSpace(combatMode))
	if combatMode != "pvp" && combatMode != "pve" {
		return "", Localization.WithError(fmt.Errorf("combatMode must be pvp or pve"), Localization.New("server.equipment.combatmode_must_be_pvp.4140c885", "combatMode must be pvp or pve", nil))
	}
	equipment, gems, err := currentLeaderLoadout(gameState, kind, leaderID)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	worldID, playerID := State.BoundAccount(gameState)
	catalogVersion, catalogDigest := "", ""
	if gameData != nil {
		metadata := gameData.Metadata()
		catalogVersion, catalogDigest = metadata.ItemVersion, metadata.DigestSHA256
	}
	fingerprintWrite(digest, "equipment-optimizer-v1", strings.TrimSpace(worldID), int64(playerID),
		gameState.Session.Generation, gameState.Session.ConnectionGeneration, catalogVersion, catalogDigest,
		kind, leaderID, combatMode)
	available := true
	if kind == "commander" {
		available = gameState.Commanders[State.CommanderID(leaderID)].Available
	}
	fingerprintWrite(digest, available)
	writeAssignmentFingerprint(digest, equipment, gems)
	appearanceID := equipment["5"]

	equipmentIDs := make([]int64, 0, len(gameState.Inventory.Equipment))
	for id, item := range gameState.Inventory.Equipment {
		if item.TypeID == optimizerEquipmentType(kind) && (optimizerSlot(item.Slot) || item.Slot == 5 && item.ID == appearanceID) &&
			(item.WearerKind == "" || item.WearerKind == kind && item.WearerID == leaderID) {
			equipmentIDs = append(equipmentIDs, int64(id))
		}
	}
	sort.Slice(equipmentIDs, func(left, right int) bool { return equipmentIDs[left] < equipmentIDs[right] })
	for _, rawID := range equipmentIDs {
		item := gameState.Inventory.Equipment[State.EquipmentInstanceID(rawID)]
		fingerprintWrite(digest, "equipment", int64(item.ID), int64(item.DefinitionID), item.Slot, item.TypeID,
			item.RarityID, item.Relic, item.RelicKnown, item.SetID, item.Level, item.WearerKind, item.WearerID)
		writeEffectFingerprint(digest, item.Effects)
	}

	gemIDs := make([]int64, 0, len(gameState.Inventory.Gems))
	for id, gem := range gameState.Inventory.Gems {
		if gem.EquipmentInstanceID == appearanceID && appearanceID != 0 ||
			gemEligibleForLeader(gameState, gem, kind, leaderID) && (gem.EquipmentInstanceID != 0 || gemMatchesMode(gem, kind, combatMode)) {
			gemIDs = append(gemIDs, int64(id))
		}
	}
	sort.Slice(gemIDs, func(left, right int) bool { return gemIDs[left] < gemIDs[right] })
	for _, rawID := range gemIDs {
		gem := gameState.Inventory.Gems[State.GemInstanceID(rawID)]
		fingerprintWrite(digest, "gem", int64(gem.ID), int64(gem.DefinitionID), gem.TypeID,
			gem.CompatibleWearerID, gem.CombatMode, gem.SetID, gem.Slot, gem.Level,
			int64(gem.EquipmentInstanceID), gem.WearerKind, gem.WearerID)
		writeEffectFingerprint(digest, gem.Effects)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeAssignmentFingerprint(
	digest hash.Hash,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
) {
	for _, slot := range optimizerSlots {
		key := strconv.Itoa(slot)
		fingerprintWrite(digest, "equipped", slot, int64(equipment[key]))
	}
	fingerprintWrite(digest, "equipped", 5, int64(equipment["5"]))
	for slot := 1; slot <= gemSlotCount; slot++ {
		key := strconv.Itoa(slot)
		fingerprintWrite(digest, "socketed", slot, int64(gems[key]))
	}
}

func writeEffectFingerprint(digest hash.Hash, source State.EquipmentEffects) {
	effects := append(State.EquipmentEffects(nil), source...)
	sort.Slice(effects, func(left, right int) bool {
		if effects[left].DefinitionID != effects[right].DefinitionID {
			return effects[left].DefinitionID < effects[right].DefinitionID
		}
		if effects[left].WireID != effects[right].WireID {
			return effects[left].WireID < effects[right].WireID
		}
		leftKey := fmt.Sprintf("%v:%v", effects[left].RollPercent, effects[left].Values)
		rightKey := fmt.Sprintf("%v:%v", effects[right].RollPercent, effects[right].Values)
		return leftKey < rightKey
	})
	for _, effect := range effects {
		fingerprintWrite(digest, "effect", effect.DefinitionID, effect.WireID)
		if effect.RollPercent == nil {
			fingerprintWrite(digest, "roll:nil")
		} else {
			fingerprintWrite(digest, "roll", *effect.RollPercent)
		}
		for _, value := range effect.Values {
			fingerprintWrite(digest, value)
		}
	}
}

func fingerprintWrite(digest hash.Hash, values ...any) {
	for _, value := range values {
		fmt.Fprintf(digest, "%T:%v\x00", value, value)
	}
}

func loadoutAssignmentKey(
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
) string {
	var builder strings.Builder
	for _, slot := range optimizerSlots {
		fmt.Fprintf(&builder, "e%d=%d;", slot, equipment[strconv.Itoa(slot)])
	}
	for slot := 1; slot <= gemSlotCount; slot++ {
		fmt.Fprintf(&builder, "g%d=%d;", slot, gems[strconv.Itoa(slot)])
	}
	return builder.String()
}

func currentLeaderLoadout(gameState State.GameState, kind string, id int64) (map[string]State.EquipmentInstanceID, map[string]State.GemInstanceID, error) {
	switch kind {
	case "commander":
		leader, ok := gameState.Commanders[State.CommanderID(id)]
		if !ok {
			return nil, nil, Localization.WithError(fmt.Errorf("commander %d is not in current state", id), Localization.New("server.equipment.commander_p_is_not.7a3d451e", "commander {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		return cloneMap(leader.Equipment), cloneMap(leader.Gems), nil
	case "castellan":
		leader, ok := gameState.Castellans[State.CastellanID(id)]
		if !ok {
			return nil, nil, Localization.WithError(fmt.Errorf("castellan %d is not in current state", id), Localization.New("server.equipment.castellan_p_is_not.cce883a3", "castellan {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		return cloneMap(leader.Equipment), cloneMap(leader.Gems), nil
	default:
		return nil, nil, Localization.WithError(fmt.Errorf("leaderKind must be commander or castellan"), Localization.New("server.equipment.leaderkind_must_be_commander.eb1dcd4c", "leaderKind must be commander or castellan", nil))
	}
}

func preparePriorities(gameData *GameData.Store, input []Priority) ([]weightedPriority, error) {
	if len(input) == 0 {
		return nil, Localization.WithError(fmt.Errorf("at least one effect priority is required"), Localization.New("server.equipment.at_least_one_effect.158e0ea8", "at least one effect priority is required", nil))
	}
	seen := map[int64]struct{}{}
	result := make([]weightedPriority, 0, len(input))
	var effects *GameData.Catalog
	if gameData != nil {
		effects, _ = gameData.Catalog("effects")
	}
	for _, priority := range input {
		if priority.EffectID <= 0 || priority.Tier < 1 || priority.Tier > 2 || priority.Position < 0 {
			return nil, Localization.WithError(fmt.Errorf("effect priorities require a positive effectId, tier 1 or 2, and non-negative position"), Localization.New("server.equipment.effect_priorities_require_a.c4166362", "effect priorities require a positive effectId, tier 1 or 2, and non-negative position", nil))
		}
		if _, duplicate := seen[priority.EffectID]; duplicate {
			return nil, Localization.WithError(fmt.Errorf("effect %d appears more than once", priority.EffectID), Localization.New("server.equipment.effect_p_appears_more.f46787a0", "effect {p0} appears more than once", Localization.Params{"p0": fmt.Sprintf("%d", priority.EffectID)}))
		}
		if effects != nil {
			if _, found := effects.Find(strconv.FormatInt(priority.EffectID, 10)); !found {
				return nil, Localization.WithError(fmt.Errorf("effect %d is not in the official effect catalog", priority.EffectID), Localization.New("server.equipment.effect_p_is_not.8474b873", "effect {p0} is not in the official effect catalog", Localization.Params{"p0": fmt.Sprintf("%d", priority.EffectID)}))
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
		caps:          make([]effectCap, len(priorities)),
		setBonuses:    make(map[int64][]scoredSetBonus, len(rules.setBonuses)),
		setPotential:  make(map[int64]float64, len(rules.setBonuses)),
	}
	for index, priority := range priorities {
		scoring.priorityIndex[priority.effectID] = index
		scoring.caps[index] = rules.caps[priority.effectID]
	}
	scoring.capGroups = buildScoredCapGroups(scoring.caps)
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

func candidateEquipment(gameState State.GameState, kind string, leaderID int64, priorities []weightedPriority, scoring scoringRules, rules officialRules, request OptimizeRequest) map[int][]optimizerCandidate {
	expectedType := optimizerEquipmentType(kind)
	result := map[int][]optimizerCandidate{}
	carrierState := map[State.EquipmentInstanceID]string{}
	for _, gem := range gameState.Inventory.Gems {
		if gem.EquipmentInstanceID != 0 {
			carrierState[gem.EquipmentInstanceID] = fmt.Sprintf("gem-family=%d:def=%d:level=%d", GemFamily(gem), gem.DefinitionID, gem.Level)
		}
	}
	for _, item := range gameState.Inventory.Equipment {
		if item.TypeID != expectedType || !optimizerSlot(item.Slot) {
			continue
		}
		family := EquipmentFamily(item)
		if family == LoadoutFamilyUnknown {
			continue
		}
		if item.WearerKind != "" && (item.WearerKind != kind || item.WearerID != leaderID) {
			continue
		}
		result[item.Slot] = append(result[item.Slot], makeCandidate(int64(item.ID), item.SetID, family, item.Effects, priorities, scoring, rules, request, carrierState[item.ID]))
	}
	return result
}

func candidateGems(gameState State.GameState, kind string, leaderID int64, combatMode string, priorities []weightedPriority, scoring scoringRules, rules officialRules, request OptimizeRequest) []optimizerCandidate {
	result := make([]optimizerCandidate, 0, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if !gemEligibleForLeader(gameState, gem, kind, leaderID) {
			continue
		}
		if !gemMatchesMode(gem, kind, combatMode) {
			continue
		}
		family := GemFamily(gem)
		if family == LoadoutFamilyUnknown {
			continue
		}
		costState := "storage"
		if gem.EquipmentInstanceID != 0 {
			costState = fmt.Sprintf("carrier=%d", gem.EquipmentInstanceID)
		}
		result = append(result, makeCandidate(int64(gem.ID), gem.SetID, family, gem.Effects, priorities, scoring, rules, request, costState))
	}
	return result
}

func optimizerEquipmentType(kind string) int {
	if kind == "castellan" {
		return 1
	}
	return 2
}

func gemEligibleForLeader(gameState State.GameState, gem State.GemInstance, kind string, leaderID int64) bool {
	if gem.WearerKind != "" && (gem.WearerKind != kind || gem.WearerID != leaderID) {
		return false
	}
	if gem.EquipmentInstanceID == 0 {
		return true
	}
	carrier, found := gameState.Inventory.Equipment[gem.EquipmentInstanceID]
	if !found || carrier.TypeID != optimizerEquipmentType(kind) || carrier.Slot < 1 || carrier.Slot > 4 {
		return false
	}
	return carrier.WearerKind == "" || carrier.WearerKind == kind && carrier.WearerID == leaderID
}

// GemMatchesLeaderAndMode applies the same ownership, carrier and combat-mode
// rules used by the optimizer. The reconfiguration planner calls it again so a
// forged or stale client cannot apply a gem the preview would not select.
func GemMatchesLeaderAndMode(gameState State.GameState, gem State.GemInstance, kind string, leaderID int64, combatMode string) bool {
	return gemEligibleForLeader(gameState, gem, kind, leaderID) && gemMatchesMode(gem, kind, combatMode)
}

func makeCandidate(id int64, setID int64, family LoadoutFamily, effects State.EquipmentEffects, priorities []weightedPriority, scoring scoringRules, rules officialRules, request OptimizeRequest, costState string) optimizerCandidate {
	values := effectValues(effects, scoring.priorityIndex, len(priorities))
	return optimizerCandidate{
		id: id, setID: setID, family: family, values: values,
		rank:      scoreValues(values, priorities, scoring.caps) + scoring.setPotential[setID],
		signature: fmt.Sprintf("family=%d:set=%d:%s:cost=%s", family, setID, semanticCandidateSignature(effects, rules, request), costState),
	}
}

func semanticCandidateSignature(effects State.EquipmentEffects, rules officialRules, request OptimizeRequest) string {
	totals := map[string]semanticEffect{}
	addSemanticEffects(totals, effects, rules, request)
	applySemanticCaps(totals)
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&builder, "%s=%g;", key, materialBucket(totals[key]))
	}
	return builder.String()
}

func materialBucket(effect semanticEffect) float64 {
	if effect.categorical {
		return float64(effect.argumentID)
	}
	step := 1.0
	switch effect.unit {
	case "percent":
		magnitude := math.Abs(effect.value)
		if effect.cap.max > 0 {
			step = math.Max(1, math.Abs(effect.cap.max)*0.01)
		} else if magnitude > 0 {
			step = math.Max(1, math.Pow(10, math.Floor(math.Log10(magnitude))-2))
		}
	case "count":
		step = 1
	default:
		step = math.Pow10(-effect.precision)
	}
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		step = 1
	}
	return math.Floor(effect.value/step+1e-9) * step
}

func optimizeFamily(
	equipmentBySlot map[int][]optimizerCandidate,
	gemCandidates []optimizerCandidate,
	family LoadoutFamily,
	currentEquipment map[string]State.EquipmentInstanceID,
	currentGems map[string]State.GemInstanceID,
	priorities []weightedPriority,
	scoring scoringRules,
	diversityLane bool,
) []partialLoadout {
	beam := []partialLoadout{{}}
	for slotIndex, slot := range optimizerSlots {
		candidates := candidatesInFamily(equipmentBySlot[slot], family)
		if slot <= 4 && len(candidates) == 0 {
			return nil
		}
		retained := map[int64]struct{}{
			int64(currentEquipment[strconv.Itoa(slot)]): {},
		}
		if diversityLane {
			candidates = limitCandidates(candidates, equipmentCandidateLimit, 1, retained)
		} else {
			candidates = limitCandidatesByScore(candidates, equipmentCandidateLimit, retained)
		}
		beam = expandEquipmentBeam(
			beam, slotIndex, candidatePointers(candidates, slot == 6),
			int64(currentEquipment[strconv.Itoa(slot)]), priorities, scoring, diversityLane,
		)
	}

	currentGemIDs := make(map[int64]struct{}, len(currentGems))
	for _, id := range currentGems {
		if id != 0 && GemFamily(State.GemInstance{ID: id}) == family {
			currentGemIDs[int64(id)] = struct{}{}
		}
	}
	gems := candidatesInFamily(gemCandidates, family)
	if diversityLane {
		gems = limitCandidates(gems, gemCandidateLimit, gemSlotCount, currentGemIDs)
	} else {
		gems = limitCandidatesByScore(gems, gemCandidateLimit, currentGemIDs)
	}
	choices := candidatePointers(gems, true)
	for slotIndex := range gemSlotCount {
		beam = expandGemBeam(
			beam, slotIndex, choices, int64(currentGems[strconv.Itoa(slotIndex+1)]), priorities, scoring, diversityLane,
		)
	}
	return beam
}

func candidatesInFamily(candidates []optimizerCandidate, family LoadoutFamily) []optimizerCandidate {
	result := make([]optimizerCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.family == family {
			result = append(result, candidate)
		}
	}
	return result
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

func limitCandidates(candidates []optimizerCandidate, limit int, equivalentMultiplicity int, retained map[int64]struct{}) []optimizerCandidate {
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].rank != candidates[right].rank {
			return candidates[left].rank > candidates[right].rank
		}
		return candidates[left].id < candidates[right].id
	})
	if equivalentMultiplicity < 1 {
		equivalentMultiplicity = 1
	}
	seen := map[string]int{}
	result := make([]optimizerCandidate, 0, limit+len(retained))
	for _, candidate := range candidates {
		_, keep := retained[candidate.id]
		equivalentCount := seen[candidate.signature]
		if equivalentCount >= equivalentMultiplicity && !keep {
			continue
		}
		if len(result) >= limit && !keep {
			continue
		}
		seen[candidate.signature] = equivalentCount + 1
		result = append(result, candidate)
	}
	return result
}

func limitCandidatesByScore(candidates []optimizerCandidate, limit int, retained map[int64]struct{}) []optimizerCandidate {
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].rank != candidates[right].rank {
			return candidates[left].rank > candidates[right].rank
		}
		return candidates[left].id < candidates[right].id
	})
	result := make([]optimizerCandidate, 0, limit+len(retained))
	for _, candidate := range candidates {
		_, keep := retained[candidate.id]
		if len(result) >= limit && !keep {
			continue
		}
		result = append(result, candidate)
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

func expandEquipmentBeam(beam []partialLoadout, slotIndex int, choices []*optimizerCandidate, currentID int64, priorities []weightedPriority, scoring scoringRules, diversityLane bool) []partialLoadout {
	builder := newBeamBuilder(optimizerBeamWidth, diversityLane)
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

func expandGemBeam(beam []partialLoadout, slotIndex int, choices []*optimizerCandidate, currentID int64, priorities []weightedPriority, scoring scoringRules, diversityLane bool) []partialLoadout {
	builder := newBeamBuilder(optimizerBeamWidth, diversityLane)
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

	values := make([]float64, len(priorities))
	for priorityIndex := range priorities {
		for _, item := range loadout.equipment {
			if item != nil {
				values[priorityIndex] += item.values[priorityIndex]
			}
		}
		for _, gem := range loadout.gems {
			if gem != nil {
				values[priorityIndex] += gem.values[priorityIndex]
			}
		}
		for index := 0; index < setLength; index++ {
			for _, bonus := range scoring.setBonuses[setIDs[index]] {
				if setCounts[index] >= bonus.neededItems {
					values[priorityIndex] += bonus.values[priorityIndex]
				}
			}
		}
	}
	applyScoredCapGroups(values, scoring.capGroups)
	score := 0.0
	coverage := newCoverageGroupTracker()
	for priorityIndex, priority := range priorities {
		value := values[priorityIndex]
		score += coverage.observe(priority, value)
		score += value * priority.weight
	}
	return score + coverage.finish()
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
	keys      []string
	indexes   map[string]int
	limit     int
	heapified bool
	diversity bool
}

func newBeamBuilder(limit int, diversity bool) beamBuilder {
	return beamBuilder{values: make([]partialLoadout, 0, limit), keys: make([]string, 0, limit), indexes: make(map[string]int, limit), limit: limit, diversity: diversity}
}

// offer retains only the strongest beam-width branches. Once full, this is a
// worst-first heap, so expansion stays bounded rather than sorting every
// candidate combination for each slot.
func (builder *beamBuilder) offer(candidate partialLoadout) {
	key := ""
	if builder.diversity {
		key = partialDiversityKey(candidate)
	}
	if index, found := builder.indexes[key]; builder.diversity && found {
		if betterLoadout(candidate, builder.values[index]) {
			builder.values[index] = candidate
			if builder.heapified {
				builder.siftDown(index)
			}
		}
		return
	}
	if len(builder.values) < builder.limit {
		if builder.diversity {
			builder.indexes[key] = len(builder.values)
		}
		builder.values = append(builder.values, candidate)
		builder.keys = append(builder.keys, key)
		return
	}
	if !builder.heapified {
		builder.heapify()
		builder.heapified = true
	}
	if betterLoadout(candidate, builder.values[0]) {
		if builder.diversity {
			delete(builder.indexes, builder.keys[0])
		}
		builder.values[0] = candidate
		builder.keys[0] = key
		if builder.diversity {
			builder.indexes[key] = 0
		}
		builder.siftDown(0)
	}
}

func (builder *beamBuilder) finish() []partialLoadout {
	sort.Slice(builder.values, func(left, right int) bool {
		return betterLoadout(builder.values[left], builder.values[right])
	})
	return builder.values
}

func partialDiversityKey(loadout partialLoadout) string {
	var builder strings.Builder
	for index, item := range loadout.equipment {
		if item != nil {
			fmt.Fprintf(&builder, "e%d=%s;", index, item.signature)
		}
	}
	gemSignatures := make([]string, 0, len(loadout.gems))
	for index, gem := range loadout.gems {
		if gem == nil {
			continue
		}
		gemSignatures = append(gemSignatures, gem.signature)
		if index < len(loadout.equipment) && loadout.equipment[index] != nil && strings.Contains(gem.signature, fmt.Sprintf("carrier=%d", loadout.equipment[index].id)) {
			fmt.Fprintf(&builder, "attached%d=1;", index)
		}
	}
	sort.Strings(gemSignatures)
	for _, signature := range gemSignatures {
		fmt.Fprintf(&builder, "g=%s;", signature)
	}
	return builder.String()
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
		builder.keys[index], builder.keys[worst] = builder.keys[worst], builder.keys[index]
		if builder.diversity {
			builder.indexes[builder.keys[index]] = index
			builder.indexes[builder.keys[worst]] = worst
		}
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

func scoreEffectTotals(totals map[int64]float64, priorities []weightedPriority, caps map[int64]effectCap) float64 {
	values := make([]float64, len(priorities))
	priorityCaps := make([]effectCap, len(priorities))
	for index, priority := range priorities {
		values[index] = totals[priority.effectID]
		priorityCaps[index] = caps[priority.effectID]
	}
	applyScoredCapGroups(values, buildScoredCapGroups(priorityCaps))
	return scoreCappedValues(values, priorities)
}

func scoreValues(values []float64, priorities []weightedPriority, caps []effectCap) float64 {
	values = append([]float64(nil), values...)
	applyScoredCapGroups(values, buildScoredCapGroups(caps))
	return scoreCappedValues(values, priorities)
}

func scoreCappedValues(values []float64, priorities []weightedPriority) float64 {
	score := 0.0
	coverage := newCoverageGroupTracker()
	for index, priority := range priorities {
		value := values[index]
		score += coverage.observe(priority, value)
		score += value * priority.weight
	}
	return score + coverage.finish()
}

func buildScoredCapGroups(caps []effectCap) []scoredCapGroup {
	groupIndexes := map[int64]int{}
	groups := make([]scoredCapGroup, 0)
	for index, cap := range caps {
		if cap.id <= 0 || cap.max <= 0 {
			continue
		}
		groupIndex, found := groupIndexes[cap.id]
		if !found {
			groupIndex = len(groups)
			groupIndexes[cap.id] = groupIndex
			groups = append(groups, scoredCapGroup{})
		}
		groups[groupIndex].maximum = math.Max(groups[groupIndex].maximum, cap.max)
		groups[groupIndex].indexes = append(groups[groupIndex].indexes, index)
	}
	return groups
}

func applyScoredCapGroups(values []float64, groups []scoredCapGroup) {
	for _, group := range groups {
		total := 0.0
		for _, index := range group.indexes {
			total += values[index]
		}
		maximum := group.maximum
		if total <= maximum && total >= -maximum || total == 0 {
			continue
		}
		scale := maximum / math.Abs(total)
		for _, index := range group.indexes {
			values[index] *= scale
		}
	}
}

func cappedPriorityTotals(totals map[int64]float64, priorities []weightedPriority, caps map[int64]effectCap) map[int64]float64 {
	values := make([]float64, len(priorities))
	orderedCaps := make([]effectCap, len(priorities))
	for index, priority := range priorities {
		values[index] = totals[priority.effectID]
		orderedCaps[index] = caps[priority.effectID]
	}
	applyScoredCapGroups(values, buildScoredCapGroups(orderedCaps))
	result := cloneMap(totals)
	for index, priority := range priorities {
		result[priority.effectID] = values[index]
	}
	return result
}

func assignmentEffects(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	rules officialRules,
	request OptimizeRequest,
) map[string]semanticEffect {
	totals := map[string]semanticEffect{}
	setCounts := map[int64]int{}
	for _, id := range equipment {
		item, ok := gameState.Inventory.Equipment[id]
		if !ok {
			continue
		}
		addSemanticEffects(totals, item.Effects, rules, request)
		if item.SetID > 0 {
			setCounts[item.SetID]++
		}
	}
	seenGems := map[State.GemInstanceID]struct{}{}
	for _, id := range gems {
		gem, ok := gameState.Inventory.Gems[id]
		if ok {
			seenGems[id] = struct{}{}
			addSemanticEffects(totals, gem.Effects, rules, request)
			if gem.SetID > 0 {
				setCounts[gem.SetID]++
			}
		}
	}
	// The retained appearance socket is not a mutable numbered gem assignment.
	// Discover it from its authoritative carrier and include it exactly once.
	if appearanceID := equipment["5"]; appearanceID != 0 {
		for id, gem := range gameState.Inventory.Gems {
			if gem.EquipmentInstanceID != appearanceID {
				continue
			}
			if _, exists := seenGems[id]; exists {
				continue
			}
			seenGems[id] = struct{}{}
			addSemanticEffects(totals, gem.Effects, rules, request)
			if gem.SetID > 0 {
				setCounts[gem.SetID]++
			}
		}
	}
	for setID, count := range setCounts {
		for _, bonus := range rules.setBonuses[setID] {
			if count >= bonus.neededItems {
				addSemanticEffects(totals, bonus.effects, rules, request)
			}
		}
	}
	applySemanticCaps(totals)
	return mergeSemanticBuckets(totals)
}

func assignmentScoringMetadata(
	gameState State.GameState,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	rules officialRules,
	request OptimizeRequest,
) (map[int64]float64, map[string]map[int64]struct{}) {
	totals := map[int64]float64{}
	semanticDefinitions := map[string]map[int64]struct{}{}
	setCounts := map[int64]int{}
	add := func(effects State.EquipmentEffects) {
		for _, effect := range effects {
			definition := rules.effects[effect.DefinitionID]
			if !effectAppliesToTarget(definition, rules, request) {
				continue
			}
			totals[effect.DefinitionID] += effectMagnitude(effect.Values)
			for _, resolved := range resolveSemanticValues(effect, definition, rules.caps[effect.DefinitionID]) {
				key := semanticKey(resolved)
				definitions := semanticDefinitions[key]
				if definitions == nil {
					definitions = map[int64]struct{}{}
					semanticDefinitions[key] = definitions
				}
				definitions[effect.DefinitionID] = struct{}{}
			}
		}
	}
	for _, id := range equipment {
		item, ok := gameState.Inventory.Equipment[id]
		if !ok {
			continue
		}
		add(item.Effects)
		if item.SetID > 0 {
			setCounts[item.SetID]++
		}
	}
	seenGems := map[State.GemInstanceID]struct{}{}
	for _, id := range gems {
		gem, ok := gameState.Inventory.Gems[id]
		if !ok {
			continue
		}
		seenGems[id] = struct{}{}
		add(gem.Effects)
		if gem.SetID > 0 {
			setCounts[gem.SetID]++
		}
	}
	if appearanceID := equipment["5"]; appearanceID != 0 {
		for id, gem := range gameState.Inventory.Gems {
			if gem.EquipmentInstanceID != appearanceID {
				continue
			}
			if _, exists := seenGems[id]; exists {
				continue
			}
			seenGems[id] = struct{}{}
			add(gem.Effects)
			if gem.SetID > 0 {
				setCounts[gem.SetID]++
			}
		}
	}
	for setID, count := range setCounts {
		for _, bonus := range rules.setBonuses[setID] {
			if count >= bonus.neededItems {
				add(bonus.effects)
			}
		}
	}
	return totals, semanticDefinitions
}

func addSemanticEffects(totals map[string]semanticEffect, effects State.EquipmentEffects, rules officialRules, request OptimizeRequest) {
	for _, effect := range effects {
		definition := rules.effects[effect.DefinitionID]
		if !effectAppliesToTarget(definition, rules, request) {
			continue
		}
		for _, resolved := range resolveSemanticValues(effect, definition, rules.caps[effect.DefinitionID]) {
			key := semanticBucketKey(resolved)
			current := totals[key]
			if current.definitionID == 0 {
				current = resolved
				current.value = 0
				current.rawValue = 0
			}
			if resolved.categorical {
				current.value = 1
				current.rawValue = 1
			} else {
				current.value += resolved.value
				current.rawValue += resolved.value
			}
			if resolved.definitionID < current.definitionID {
				current.definitionID = resolved.definitionID
			}
			totals[key] = current
		}
	}
}

func resolveSemanticValues(effect State.EquipmentEffect, definition effectDefinition, cap effectCap) []semanticEffect {
	if len(effect.Values) == 0 {
		return nil
	}
	if definition.categorical {
		definition.unit = "categorical"
		argument := int64(math.Round(effect.Values[len(effect.Values)-1]))
		return []semanticEffect{{identity: effectSemanticIdentity(effect.DefinitionID, definition, 1), definitionID: effect.DefinitionID, argumentID: argument, value: 1, unit: "categorical", categorical: true, cap: cap}}
	}
	if definition.unit == "" {
		definition.unit, definition.precision = "percent", 1
	}
	paired := len(effect.Values)%2 == 0
	if paired {
		for index := 0; index < len(effect.Values); index += 2 {
			if effect.Values[index] < 1 || math.Trunc(effect.Values[index]) != effect.Values[index] {
				paired = false
				break
			}
		}
	}
	if !paired {
		value := effect.Values[len(effect.Values)-1]
		return []semanticEffect{{identity: effectSemanticIdentity(effect.DefinitionID, definition, value), definitionID: effect.DefinitionID, value: value, unit: definition.unit, precision: definition.precision, cap: cap}}
	}
	resolved := make([]semanticEffect, 0, len(effect.Values)/2)
	for index := 0; index < len(effect.Values); index += 2 {
		value := effect.Values[index+1]
		resolved = append(resolved, semanticEffect{identity: effectSemanticIdentity(effect.DefinitionID, definition, value), definitionID: effect.DefinitionID, argumentID: int64(effect.Values[index]), value: value, unit: definition.unit, precision: definition.precision, cap: cap})
	}
	return resolved
}

func semanticOutcomeKey(effects []EffectTotal) string {
	var builder strings.Builder
	for _, effect := range effects {
		fmt.Fprintf(&builder, "%s=%s;", effect.SemanticKey, strconv.FormatFloat(effect.Value, 'g', -1, 64))
	}
	return builder.String()
}

// materiallyDifferent implements the CIT-7 near-duplicate heuristic after
// official caps. Percentage rows require at least one percentage point or one
// percent of the shared cap (one percent of the compared magnitude uncapped).
// Counts require one; other continuous values use their display precision.
func materiallyDifferent(left, right []EffectTotal) bool {
	leftByKey := make(map[string]EffectTotal, len(left))
	rightByKey := make(map[string]EffectTotal, len(right))
	keys := map[string]struct{}{}
	for _, effect := range left {
		leftByKey[effect.SemanticKey] = effect
		keys[effect.SemanticKey] = struct{}{}
	}
	for _, effect := range right {
		rightByKey[effect.SemanticKey] = effect
		keys[effect.SemanticKey] = struct{}{}
	}
	for key := range keys {
		leftEffect, leftOK := leftByKey[key]
		rightEffect, rightOK := rightByKey[key]
		if leftEffect.Categorical || rightEffect.Categorical {
			if leftOK != rightOK || leftEffect.ArgumentID == nil != (rightEffect.ArgumentID == nil) || leftEffect.ArgumentID != nil && rightEffect.ArgumentID != nil && *leftEffect.ArgumentID != *rightEffect.ArgumentID {
				return true
			}
			continue
		}
		difference := math.Abs(leftEffect.Value - rightEffect.Value)
		unit := leftEffect.Unit
		if unit == "" {
			unit = rightEffect.Unit
		}
		threshold := 1.0
		switch unit {
		case "percent":
			maximum := math.Max(math.Abs(leftEffect.Value), math.Abs(rightEffect.Value))
			if leftEffect.Cap != nil {
				maximum = math.Max(maximum, math.Abs(*leftEffect.Cap))
			}
			if rightEffect.Cap != nil {
				maximum = math.Max(maximum, math.Abs(*rightEffect.Cap))
			}
			threshold = math.Max(1, maximum*0.01)
		case "count":
			threshold = 1
		default:
			precision := leftEffect.Precision
			if rightEffect.Precision > precision {
				precision = rightEffect.Precision
			}
			threshold = math.Pow10(-precision)
		}
		if difference+1e-9 >= threshold {
			return true
		}
	}
	return false
}

func loadoutReason(current Loadout, selected []Loadout, candidate Loadout) string {
	if len(selected) == 0 {
		if materiallyDifferent(current.Effects, candidate.Effects) {
			return "Strongest configured-priority result"
		}
		return "Current loadout is already strongest"
	}
	if candidate.ExtractionCost.MaximumRubySpend < selected[0].ExtractionCost.MaximumRubySpend {
		return fmt.Sprintf("Saves up to %d rubies with a near-equivalent outcome", selected[0].ExtractionCost.MaximumRubySpend-candidate.ExtractionCost.MaximumRubySpend)
	}
	return "Offers a material stat trade-off"
}

func isDominatedOutcome(candidate, stronger Loadout, priorities []weightedPriority) bool {
	if candidate.ExtractionCost.MaximumRubySpend < stronger.ExtractionCost.MaximumRubySpend {
		return false
	}
	priorityIDs := make(map[int64]struct{}, len(priorities))
	for _, priority := range priorities {
		priorityIDs[priority.effectID] = struct{}{}
	}
	strongerByKey := make(map[string]EffectTotal, len(stronger.Effects))
	candidateByKey := make(map[string]EffectTotal, len(candidate.Effects))
	keys := map[string]struct{}{}
	for _, effect := range stronger.Effects {
		strongerByKey[effect.SemanticKey] = effect
		keys[effect.SemanticKey] = struct{}{}
	}
	for _, effect := range candidate.Effects {
		candidateByKey[effect.SemanticKey] = effect
		keys[effect.SemanticKey] = struct{}{}
	}
	for key := range keys {
		left, leftOK := candidateByKey[key]
		right, rightOK := strongerByKey[key]
		if !materiallyDifferent(singleEffect(left, leftOK), singleEffect(right, rightOK)) {
			continue
		}
		definitions := map[int64]struct{}{}
		for definitionID := range candidate.semanticDefinitions[key] {
			definitions[definitionID] = struct{}{}
		}
		for definitionID := range stronger.semanticDefinitions[key] {
			definitions[definitionID] = struct{}{}
		}
		if len(definitions) == 0 {
			return false
		}
		for definitionID := range definitions {
			if _, knownDirection := priorityIDs[definitionID]; !knownDirection {
				return false
			}
		}
	}
	worse := false
	for _, priority := range priorities {
		candidateValue := candidate.priorityValues[priority.effectID]
		strongerValue := stronger.priorityValues[priority.effectID]
		if candidateValue > strongerValue {
			return false
		}
		if candidateValue < strongerValue {
			worse = true
		}
	}
	return worse
}

func singleEffect(effect EffectTotal, exists bool) []EffectTotal {
	if !exists {
		return nil
	}
	return []EffectTotal{effect}
}

func semanticKey(effect semanticEffect) string {
	return fmt.Sprintf("%s:arg=%d", effect.identity, effect.argumentID)
}

func semanticBucketKey(effect semanticEffect) string {
	return fmt.Sprintf("%s:cap=%d", semanticKey(effect), effect.cap.id)
}

func mergeSemanticBuckets(buckets map[string]semanticEffect) map[string]semanticEffect {
	result := map[string]semanticEffect{}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, bucketKey := range keys {
		bucket := buckets[bucketKey]
		key := semanticKey(bucket)
		current, found := result[key]
		if !found {
			result[key] = bucket
			continue
		}
		current.value += bucket.value
		current.rawValue += bucket.rawValue
		if bucket.definitionID < current.definitionID {
			current.definitionID = bucket.definitionID
		}
		if current.cap.id != bucket.cap.id {
			current.cap = effectCap{}
		}
		result[key] = current
	}
	return result
}

func effectSemanticIdentity(definitionID int64, definition effectDefinition, value float64) string {
	meaning := fmt.Sprintf("definition=%d", definitionID)
	if definition.effectTypeID > 0 {
		meaning = fmt.Sprintf("type=%d", definition.effectTypeID)
	}
	areas := append([]int64(nil), definition.areaTypeIDs...)
	sort.Slice(areas, func(left, right int) bool { return areas[left] < areas[right] })
	polarity := "zero"
	if value > 0 {
		polarity = "positive"
	}
	if value < 0 {
		polarity = "negative"
	}
	return fmt.Sprintf("%s:unit=%s:scope=%s:areas=%v:polarity=%s", meaning, definition.unit, definition.scope, areas, polarity)
}

func applySemanticCaps(totals map[string]semanticEffect) {
	buckets := map[int64][]string{}
	for key, effect := range totals {
		if effect.cap.id > 0 && effect.cap.max > 0 && !effect.categorical {
			buckets[effect.cap.id] = append(buckets[effect.cap.id], key)
		}
	}
	for _, keys := range buckets {
		total, maximum := 0.0, 0.0
		for _, key := range keys {
			effect := totals[key]
			total += effect.value
			maximum = math.Max(maximum, effect.cap.max)
		}
		if total == 0 || math.Abs(total) <= maximum {
			continue
		}
		scale := maximum / math.Abs(total)
		for _, key := range keys {
			effect := totals[key]
			effect.value *= scale
			totals[key] = effect
		}
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
	request OptimizeRequest,
) Loadout {
	totals := assignmentEffects(gameState, equipment, gems, rules, request)
	scoreTotals, semanticDefinitions := assignmentScoringMetadata(gameState, equipment, gems, rules, request)
	priorityValues := cappedPriorityTotals(scoreTotals, priorities, rules.caps)
	effects := make([]EffectTotal, 0, len(totals))
	for key, semantic := range totals {
		rawValue := semantic.rawValue
		value := semantic.value
		var capPointer *float64
		capped := false
		capID := int64(0)
		if cap := semantic.cap; cap.max > 0 {
			capID = cap.id
			capCopy := cap.max
			capPointer = &capCopy
		}
		capped = value != rawValue
		var argument *int64
		if semantic.argumentID != 0 {
			copy := semantic.argumentID
			argument = &copy
		}
		effects = append(effects, EffectTotal{SemanticKey: key, DefinitionID: semantic.definitionID, ArgumentID: argument, Value: value, RawValue: rawValue, Unit: semantic.unit, Precision: semantic.precision, Categorical: semantic.categorical, CapID: capID, Cap: capPointer, Capped: capped})
	}
	sort.Slice(effects, func(left, right int) bool { return effects[left].SemanticKey < effects[right].SemanticKey })
	return Loadout{
		Equipment: cloneMap(equipment), Gems: cloneMap(gems), Effects: effects,
		Score:          scoreEffectTotals(scoreTotals, priorities, rules.caps),
		priorityValues: priorityValues, semanticDefinitions: semanticDefinitions,
	}
}

func loadOfficialRules(gameData *GameData.Store) officialRules {
	if gameData == nil {
		return officialRules{caps: map[int64]effectCap{}, setBonuses: map[int64][]setBonus{}, effects: map[int64]effectDefinition{}, pvpAreaScores: map[int64]int{}, pveAreaScores: map[int64]int{}}
	}
	if cached, found := officialRulesCache.Load(gameData); found {
		return cached.(officialRules)
	}
	rules := buildOfficialRules(gameData)
	actual, _ := officialRulesCache.LoadOrStore(gameData, rules)
	return actual.(officialRules)
}

func buildOfficialRules(gameData *GameData.Store) officialRules {
	rules := officialRules{caps: map[int64]effectCap{}, setBonuses: map[int64][]setBonus{}, effects: map[int64]effectDefinition{}, pvpAreaScores: map[int64]int{}, pveAreaScores: map[int64]int{}}
	capByID := map[int64]float64{}
	effectTypes := map[int64]GameData.Record{}
	if catalog, err := gameData.Catalog("effecttypes"); err == nil {
		for _, raw := range catalog.Rows() {
			record, decodeErr := GameData.DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, ok := record.Int64("effectTypeID")
			if ok {
				effectTypes[id] = record
			}
		}
	}
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
			if effectOK {
				typeID, _ := record.Int64("effectTypeID")
				typeName, _ := effectTypes[typeID].String("name")
				name, _ := record.String("name")
				unit := "percent"
				precision := 1
				if regexpCountEffect(typeName) {
					unit, precision = "count", 0
				}
				structuredScope, scope := effectCatalogScopes(record, typeName+" "+name)
				definition := effectDefinition{
					effectTypeID: typeID, name: strings.TrimSpace(typeName + " " + name),
					unit: unit, precision: precision,
					categorical: typeID == 118 || strings.EqualFold(typeName, "strongerPeasant"),
					areaTypeIDs: recordInt64List(record, "areaTypeID"),
					scope:       scope, structuredScope: structuredScope,
				}
				if definition.categorical {
					definition.unit, definition.precision = "categorical", 0
				}
				rules.effects[effectID] = definition
			}
			capID, capOK := record.Int64("capID")
			if effectOK && capOK && capByID[capID] > 0 {
				rules.caps[effectID] = effectCap{id: capID, max: capByID[capID]}
			}
		}
	}
	for _, definition := range rules.effects {
		if definition.scope == "always" || definition.scope == "" {
			continue
		}
		scores := rules.pveAreaScores
		if definition.scope == "pvp" {
			scores = rules.pvpAreaScores
		}
		for _, areaTypeID := range definition.areaTypeIDs {
			scores[areaTypeID]++
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

func regexpCountEffect(name string) bool {
	normalized := strings.ToLower(name)
	for _, marker := range []string{"unitamountyard", "unitwallabsolute", "wave", "supportunits", "reinforcement", "slotbonus", "amountspies", "amountguards", "amountpeasant", "population"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func recordInt64List(record GameData.Record, field string) []int64 {
	raw, ok := record[field]
	if !ok {
		return nil
	}
	var source any
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil
	}
	values := []int64{}
	appendValue := func(value any) {
		for _, part := range strings.FieldsFunc(fmt.Sprint(value), func(character rune) bool { return character == ',' || character == '#' }) {
			parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && parsed > 0 {
				values = append(values, parsed)
			}
		}
	}
	switch typed := source.(type) {
	case []any:
		for _, value := range typed {
			appendValue(value)
		}
	default:
		appendValue(typed)
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return values
}

func effectCatalogScopes(record GameData.Record, names string) (string, string) {
	if value, ok := recordOptionalBool(record, "isPvPFight"); ok {
		if value {
			return "pvp", "pvp"
		}
		return "pve", "pve"
	}
	if value, ok := recordOptionalBool(record, "isPvEFight"); ok {
		if value {
			return "pve", "pve"
		}
		return "pvp", "pvp"
	}
	normalized := strings.ToLower(names)
	pvp := strings.Contains(normalized, "pvp") || strings.Contains(normalized, "castlelord")
	pve := strings.Contains(normalized, "pve") || strings.Contains(normalized, "npc")
	if pvp != pve {
		if pvp {
			return "", "pvp"
		}
		return "", "pve"
	}
	return "", "always"
}

func recordOptionalBool(record GameData.Record, field string) (bool, bool) {
	raw, ok := record[field]
	if !ok {
		return false, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func effectAppliesToTarget(definition effectDefinition, rules officialRules, request OptimizeRequest) bool {
	if len(request.TargetAreaTypeIDs) > 0 {
		if len(definition.areaTypeIDs) > 0 {
			matched := false
			for _, effectArea := range definition.areaTypeIDs {
				for _, targetArea := range request.TargetAreaTypeIDs {
					if effectArea == targetArea {
						matched = true
						break
					}
				}
			}
			if !matched {
				return false
			}
		}
		if definition.structuredScope != "" {
			return definition.structuredScope == request.CombatMode
		}
		if len(definition.areaTypeIDs) > 0 {
			return true
		}
		return definition.scope == "" || definition.scope == "always" || definition.scope == request.CombatMode
	}
	if len(definition.areaTypeIDs) > 0 && len(definition.areaTypeIDs) <= 5 {
		return false
	}
	if definition.scope != "" && definition.scope != "always" {
		return definition.scope == request.CombatMode
	}
	if len(definition.areaTypeIDs) == 0 {
		return true
	}
	scores := rules.pveAreaScores
	if request.CombatMode == "pvp" {
		scores = rules.pvpAreaScores
	}
	for _, areaTypeID := range definition.areaTypeIDs {
		if scores[areaTypeID] > 0 {
			return true
		}
	}
	return false
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
