package Reports

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	offensiveMeleeEffectTypeID = int64(23)
	offensiveRangeEffectTypeID = int64(24)
	wallReductionEffectTypeID  = int64(19)
	gateReductionEffectTypeID  = int64(20)
	moatReductionEffectTypeID  = int64(21)
)

func parseBattleResearchFormation(payload json.RawMessage, capturedAt time.Time) (BattleResearchFormation, error) {
	var wire struct {
		SourceX     int               `json:"SX"`
		SourceY     int               `json:"SY"`
		TargetX     int               `json:"TX"`
		TargetY     int               `json:"TY"`
		KingdomID   State.KingdomID   `json:"KID"`
		CommanderID State.CommanderID `json:"LID"`
		Waves       []struct {
			Left struct {
				Tools [][2]int64 `json:"T"`
				Units [][2]int64 `json:"U"`
			} `json:"L"`
			Center struct {
				Tools [][2]int64 `json:"T"`
				Units [][2]int64 `json:"U"`
			} `json:"M"`
			Right struct {
				Tools [][2]int64 `json:"T"`
				Units [][2]int64 `json:"U"`
			} `json:"R"`
		} `json:"A"`
		SupportTroops [][2]int64 `json:"RW"`
		SupportTools  []int64    `json:"AST"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		return BattleResearchFormation{}, fmt.Errorf("decode CRA formation: %w", err)
	}
	if wire.TargetX < 0 || wire.TargetY < 0 || wire.CommanderID < 0 || len(wire.Waves) == 0 {
		return BattleResearchFormation{}, fmt.Errorf("CRA formation is missing a target, commander, or waves")
	}
	formation := BattleResearchFormation{
		CapturedAt: capturedAt.UTC(), SourceX: wire.SourceX, SourceY: wire.SourceY,
		TargetX: wire.TargetX, TargetY: wire.TargetY, KingdomID: wire.KingdomID,
		CommanderID: wire.CommanderID, Raw: append(json.RawMessage(nil), payload...),
		Waves: make([]ResearchWave, 0, len(wire.Waves)),
	}
	for index, wave := range wire.Waves {
		formation.Waves = append(formation.Waves, ResearchWave{
			Index:  index + 1,
			Left:   ResearchLane{Units: researchPairs(wave.Left.Units), Tools: researchPairs(wave.Left.Tools)},
			Center: ResearchLane{Units: researchPairs(wave.Center.Units), Tools: researchPairs(wave.Center.Tools)},
			Right:  ResearchLane{Units: researchPairs(wave.Right.Units), Tools: researchPairs(wave.Right.Tools)},
		})
	}
	formation.SupportTroops = researchPairs(wire.SupportTroops)
	for _, toolID := range wire.SupportTools {
		if toolID > 0 {
			formation.SupportToolIDs = append(formation.SupportToolIDs, toolID)
		}
	}
	if battleResearchFormationTroops(formation) <= 0 {
		return BattleResearchFormation{}, fmt.Errorf("CRA formation contains no attacking troops")
	}
	return formation, nil
}

func researchPairs(values [][2]int64) []ResearchPair {
	result := make([]ResearchPair, 0, len(values))
	for _, value := range values {
		if value[0] > 0 && value[1] > 0 {
			result = append(result, ResearchPair{WodID: value[0], Amount: value[1]})
		}
	}
	return result
}

func battleResearchFormationTroops(formation BattleResearchFormation) int64 {
	total := researchPairTotal(formation.SupportTroops)
	for _, wave := range formation.Waves {
		total += researchPairTotal(wave.Left.Units) + researchPairTotal(wave.Center.Units) + researchPairTotal(wave.Right.Units)
	}
	return total
}

func researchPairTotal(pairs []ResearchPair) int64 {
	var total int64
	for _, pair := range pairs {
		if pair.Amount > 0 {
			total += pair.Amount
		}
	}
	return total
}

type researchOffenseForce struct {
	count      int64
	meleePower float64
	rangePower float64
	knownCount int64
}

func (force researchOffenseForce) power() float64 {
	power := force.meleePower + force.rangePower
	if power <= 0 {
		return float64(force.count)
	}
	return power
}

func (force researchOffenseForce) scale(count int64) researchOffenseForce {
	if force.count <= 0 || count <= 0 {
		return researchOffenseForce{}
	}
	ratio := float64(count) / float64(force.count)
	return researchOffenseForce{
		count: count, meleePower: force.meleePower * ratio, rangePower: force.rangePower * ratio,
		knownCount: int64(math.Round(float64(force.knownCount) * ratio)),
	}
}

func (force *researchOffenseForce) add(other researchOffenseForce) {
	force.count += other.count
	force.meleePower += other.meleePower
	force.rangePower += other.rangePower
	force.knownCount += other.knownCount
}

type researchDefenseForce struct {
	count             int64
	meleeDefensePower float64
	rangeDefensePower float64
	knownCount        int64
}

func (force researchDefenseForce) effectivePower(attacker researchOffenseForce) float64 {
	totalAttack := attacker.meleePower + attacker.rangePower
	if totalAttack <= 0 {
		if force.meleeDefensePower+force.rangeDefensePower > 0 {
			return (force.meleeDefensePower + force.rangeDefensePower) / 2
		}
		return float64(force.count)
	}
	meleeShare := attacker.meleePower / totalAttack
	rangeShare := 1 - meleeShare
	power := force.meleeDefensePower*meleeShare + force.rangeDefensePower*rangeShare
	if power <= 0 {
		return float64(force.count)
	}
	return power
}

func (force researchDefenseForce) scale(count int64) researchDefenseForce {
	if force.count <= 0 || count <= 0 {
		return researchDefenseForce{}
	}
	ratio := float64(count) / float64(force.count)
	return researchDefenseForce{
		count: count, meleeDefensePower: force.meleeDefensePower * ratio,
		rangeDefensePower: force.rangeDefensePower * ratio,
		knownCount:        int64(math.Round(float64(force.knownCount) * ratio)),
	}
}

func (force *researchDefenseForce) add(other researchDefenseForce) {
	force.count += other.count
	force.meleeDefensePower += other.meleeDefensePower
	force.rangeDefensePower += other.rangeDefensePower
	force.knownCount += other.knownCount
}

type researchCombatModifiers struct {
	meleeBonus    float64
	rangeBonus    float64
	wallReduction float64
	gateReduction float64
	moatReduction float64
}

func BuildBattleResearchPrediction(
	formation BattleResearchFormation,
	context BattleResearchAttackerContext,
	movement State.MovementState,
	spy SpyReport,
	gameData *GameData.Store,
	generatedAt time.Time,
) (BattlePrediction, error) {
	if gameData == nil {
		return BattlePrediction{}, fmt.Errorf("official game data is unavailable")
	}
	if spy.Status == "failed" || len(spy.Setup) == 0 {
		return BattlePrediction{}, fmt.Errorf("a successful or partial pre-battle spy deployment is required")
	}
	modifiers := battleResearchCombatModifiers(context, movement.TargetTypeID, gameData)
	defenses := map[int]researchDefenseForce{}
	defenderObserved := int64(0)
	knownDefenders := int64(0)
	for _, section := range spy.Setup {
		force := researchDefenseFromCounts(section.Units, gameData)
		defenses[section.Index] = force
		defenderObserved += force.count
		knownDefenders += force.knownCount
	}

	prediction := BattlePrediction{
		ModelVersion: BattleResearchModelVersion, GeneratedAt: generatedAt.UTC(), Confidence: "low",
		AttackerSent: battleResearchFormationTroops(formation), DefenderObserved: defenderObserved,
		AttackerMeleeBonus: modifiers.meleeBonus, AttackerRangeBonus: modifiers.rangeBonus,
		WallReduction: modifiers.wallReduction, GateReduction: modifiers.gateReduction,
		MoatReduction: modifiers.moatReduction,
		Considered:    append([]string(nil), researchCalculatorInfo().Considered...),
		RecordedNotModeled: []string{
			"attacker and defender tools", "unrecognized equipment, gem, general, and legend-skill effects",
			"defender equipment detail and hidden or temporary server modifiers",
		},
		Assumptions: []string{
			"the pre-battle spy deployment remains unchanged until impact",
			"a losing phase loses all remaining troops while winning survivors scale by the opposing power ratio",
			"reserve sections join courtyard defense and surviving successful wall lanes enter the courtyard",
		},
	}

	wall := 0.0
	gate := 0.0
	moat := 0.0
	courtyard := 0.0
	if spy.Castellan != nil {
		wall = float64(spy.Castellan.Wall)
		gate = float64(spy.Castellan.Gate)
		moat = float64(spy.Castellan.Moat)
		courtyard = float64(spy.Castellan.Courtyard)
	}
	flankMultiplier := defenseMultiplier(wall - modifiers.wallReduction + moat - modifiers.moatReduction)
	centerMultiplier := defenseMultiplier(wall - modifiers.wallReduction + gate - modifiers.gateReduction + moat - modifiers.moatReduction)

	leftDefense := defenses[0]
	centerDefense := defenses[1]
	rightDefense := defenses[2]
	courtyardAttackers := researchOffenseForce{}
	knownAttackers := int64(0)
	wallAttackerLost := int64(0)
	wallDefenderLost := int64(0)
	initialAttackPower := 0.0
	initialDefensePower := 0.0
	for _, wave := range formation.Waves {
		leftAttack := researchOffenseFromPairs(wave.Left.Units, gameData, modifiers)
		centerAttack := researchOffenseFromPairs(wave.Center.Units, gameData, modifiers)
		rightAttack := researchOffenseFromPairs(wave.Right.Units, gameData, modifiers)
		knownAttackers += leftAttack.knownCount + centerAttack.knownCount + rightAttack.knownCount
		initialAttackPower += leftAttack.power() + centerAttack.power() + rightAttack.power()

		left, leftSurvivors, leftRemaining := resolveResearchPhase(leftAttack, leftDefense, flankMultiplier)
		center, centerSurvivors, centerRemaining := resolveResearchPhase(centerAttack, centerDefense, centerMultiplier)
		right, rightSurvivors, rightRemaining := resolveResearchPhase(rightAttack, rightDefense, flankMultiplier)
		if wave.Index == 1 {
			initialDefensePower += left.DefenderPower + center.DefenderPower + right.DefenderPower
		}
		leftDefense, centerDefense, rightDefense = leftRemaining, centerRemaining, rightRemaining
		if left.Winner == "attacker" {
			courtyardAttackers.add(leftSurvivors)
		}
		if center.Winner == "attacker" {
			courtyardAttackers.add(centerSurvivors)
		}
		if right.Winner == "attacker" {
			courtyardAttackers.add(rightSurvivors)
		}
		wallAttackerLost += left.AttackerLost + center.AttackerLost + right.AttackerLost
		wallDefenderLost += left.DefenderLost + center.DefenderLost + right.DefenderLost
		prediction.Waves = append(prediction.Waves, BattleWavePrediction{
			Wave: wave.Index, Left: left, Center: center, Right: right,
		})
	}

	support := researchOffenseFromPairs(formation.SupportTroops, gameData, modifiers)
	knownAttackers += support.knownCount
	initialAttackPower += support.power()
	courtyardAttackers.add(support)
	courtyardDefense := defenses[3]
	for index, force := range defenses {
		if index >= 4 {
			courtyardDefense.add(force)
		}
	}
	if initialDefensePower <= 0 {
		initialDefensePower = courtyardDefense.effectivePower(courtyardAttackers)
	}
	prediction.Courtyard, _, _ = resolveResearchPhase(
		courtyardAttackers, courtyardDefense, defenseMultiplier(courtyard),
	)
	if prediction.Courtyard.Winner == "attacker" {
		prediction.PredictedResult = "Victory"
	} else {
		prediction.PredictedResult = "Defeat"
	}
	prediction.ExpectedAttackerLost = minInt64(prediction.AttackerSent, wallAttackerLost+prediction.Courtyard.AttackerLost)
	prediction.ExpectedDefenderLost = minInt64(defenderObserved, wallDefenderLost+prediction.Courtyard.DefenderLost)
	prediction.ExpectedAttackerSurvivors = maxInt64(0, prediction.AttackerSent-prediction.ExpectedAttackerLost)
	prediction.ExpectedDefenderSurvivors = maxInt64(0, defenderObserved-prediction.ExpectedDefenderLost)
	knownTotal := knownAttackers + knownDefenders
	observedTotal := prediction.AttackerSent + defenderObserved
	if observedTotal > 0 {
		prediction.UnitStatCoverage = math.Round(float64(knownTotal)/float64(observedTotal)*10_000) / 10_000
	}
	if initialAttackPower <= 0 {
		initialAttackPower = float64(prediction.AttackerSent)
	}
	if initialDefensePower <= 0 {
		initialDefensePower = float64(maxInt64(1, defenderObserved))
	}
	ratio := initialAttackPower / maxFloat(1, initialDefensePower)
	weighted := math.Pow(maxFloat(0.0001, ratio), 1.6)
	prediction.AttackWinProbability = clampFloat(weighted/(1+weighted), 0.02, 0.98)
	prediction.AttackWinProbability = math.Round(prediction.AttackWinProbability*10_000) / 10_000
	return prediction, nil
}

func researchOffenseFromPairs(pairs []ResearchPair, gameData *GameData.Store, modifiers researchCombatModifiers) researchOffenseForce {
	force := researchOffenseForce{}
	for _, pair := range pairs {
		if pair.Amount <= 0 {
			continue
		}
		force.count += pair.Amount
		stats, found := researchUnitStats(pair.WodID, gameData)
		if !found {
			force.meleePower += float64(pair.Amount)
			continue
		}
		force.knownCount += pair.Amount
		force.meleePower += float64(pair.Amount) * stats.meleeAttack * (1 + modifiers.meleeBonus/100)
		force.rangePower += float64(pair.Amount) * stats.rangeAttack * (1 + modifiers.rangeBonus/100)
	}
	return force
}

func researchDefenseFromCounts(counts []UnitCount, gameData *GameData.Store) researchDefenseForce {
	force := researchDefenseForce{}
	for _, count := range counts {
		if count.Amount <= 0 {
			continue
		}
		force.count += count.Amount
		stats, found := researchUnitStats(count.WodID, gameData)
		if !found {
			force.meleeDefensePower += float64(count.Amount)
			force.rangeDefensePower += float64(count.Amount)
			continue
		}
		force.knownCount += count.Amount
		force.meleeDefensePower += float64(count.Amount) * stats.meleeDefense
		force.rangeDefensePower += float64(count.Amount) * stats.rangeDefense
	}
	return force
}

type researchUnitCombatStats struct {
	meleeAttack  float64
	rangeAttack  float64
	meleeDefense float64
	rangeDefense float64
}

func researchUnitStats(id int64, gameData *GameData.Store) (researchUnitCombatStats, bool) {
	if gameData == nil || id <= 0 {
		return researchUnitCombatStats{}, false
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return researchUnitCombatStats{}, false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return researchUnitCombatStats{}, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil || GameData.IsToolRecord(record) {
		return researchUnitCombatStats{}, false
	}
	stats := researchUnitCombatStats{}
	stats.meleeAttack, _ = record.Float64("meleeAttack")
	stats.rangeAttack, _ = record.Float64("rangeAttack")
	stats.meleeDefense, _ = record.Float64("meleeDefence")
	stats.rangeDefense, _ = record.Float64("rangeDefence")
	return stats, stats.meleeAttack > 0 || stats.rangeAttack > 0 || stats.meleeDefense > 0 || stats.rangeDefense > 0
}

func resolveResearchPhase(
	attacker researchOffenseForce,
	defender researchDefenseForce,
	defenseMultiplierValue float64,
) (BattlePhasePrediction, researchOffenseForce, researchDefenseForce) {
	result := BattlePhasePrediction{AttackerStarted: attacker.count, DefenderStarted: defender.count}
	result.AttackerPower = math.Round(attacker.power()*100) / 100
	result.DefenderPower = math.Round(defender.effectivePower(attacker)*defenseMultiplierValue*100) / 100
	if attacker.count <= 0 {
		result.Winner = "defender"
		result.DefenderSurvivors = defender.count
		return result, researchOffenseForce{}, defender
	}
	if defender.count <= 0 {
		result.Winner = "attacker"
		result.AttackerSurvivors = attacker.count
		return result, attacker, researchDefenseForce{}
	}
	if result.AttackerPower > result.DefenderPower {
		result.Winner = "attacker"
		result.DefenderLost = defender.count
		lossShare := clampFloat(result.DefenderPower/maxFloat(1, result.AttackerPower), 0, 0.98)
		result.AttackerLost = minInt64(attacker.count, int64(math.Round(float64(attacker.count)*lossShare)))
		result.AttackerSurvivors = attacker.count - result.AttackerLost
		return result, attacker.scale(result.AttackerSurvivors), researchDefenseForce{}
	}
	result.Winner = "defender"
	result.AttackerLost = attacker.count
	lossShare := clampFloat(result.AttackerPower/maxFloat(1, result.DefenderPower), 0, 0.98)
	result.DefenderLost = minInt64(defender.count, int64(math.Round(float64(defender.count)*lossShare)))
	result.DefenderSurvivors = defender.count - result.DefenderLost
	return result, researchOffenseForce{}, defender.scale(result.DefenderSurvivors)
}

func battleResearchCombatModifiers(
	context BattleResearchAttackerContext,
	targetTypeID int,
	gameData *GameData.Store,
) researchCombatModifiers {
	effects := make([]State.EquipmentEffect, 0)
	for _, equipment := range context.Equipment {
		effects = append(effects, equipment.Effects...)
	}
	for _, gem := range context.Gems {
		effects = append(effects, gem.Effects...)
	}
	if context.AttackDialog != nil {
		for _, effect := range context.AttackDialog.ActiveEffects {
			effects = append(effects, State.EquipmentEffect{
				WireID: effect.EffectID, DefinitionID: effect.EffectID,
				Values: append([]float64(nil), effect.Values...),
			})
		}
	}
	type capKey struct{ effectType, capID int64 }
	groups := map[capKey]float64{}
	maximums := battleResearchCapLimits(gameData)
	effectCatalog, err := gameData.Catalog("effects")
	if err != nil {
		return researchCombatModifiers{}
	}
	for _, effect := range effects {
		effectID := effect.DefinitionID
		if effectID <= 0 {
			effectID = effect.WireID
		}
		raw, found := effectCatalog.Find(strconv.FormatInt(effectID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil || !battleResearchEffectApplies(record, targetTypeID) {
			continue
		}
		effectType, ok := record.Int64("effectTypeID")
		if !ok {
			continue
		}
		switch effectType {
		case offensiveMeleeEffectTypeID, offensiveRangeEffectTypeID,
			wallReductionEffectTypeID, gateReductionEffectTypeID, moatReductionEffectTypeID:
		default:
			continue
		}
		capID, _ := record.Int64("capID")
		groups[capKey{effectType: effectType, capID: capID}] += researchEffectMagnitude(effect.Values)
	}
	modifiers := researchCombatModifiers{}
	for key, value := range groups {
		if maximum := maximums[key.capID]; maximum > 0 && value > maximum {
			value = maximum
		}
		switch key.effectType {
		case offensiveMeleeEffectTypeID:
			modifiers.meleeBonus += value
		case offensiveRangeEffectTypeID:
			modifiers.rangeBonus += value
		case wallReductionEffectTypeID:
			modifiers.wallReduction += value
		case gateReductionEffectTypeID:
			modifiers.gateReduction += value
		case moatReductionEffectTypeID:
			modifiers.moatReduction += value
		}
	}
	return modifiers
}

func battleResearchEffectApplies(record GameData.Record, targetTypeID int) bool {
	if isPvP, exists := record.Int64("isPvPFight"); exists && isPvP == 0 {
		return false
	}
	name, _ := record.String("name")
	if strings.Contains(strings.ToLower(name), "pve") {
		return false
	}
	if areaTypes, exists := record.String("areaTypeID"); exists && strings.TrimSpace(areaTypes) != "" {
		matched := false
		for _, value := range strings.FieldsFunc(areaTypes, func(character rune) bool { return character == ',' || character == '#' }) {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && parsed == targetTypeID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func battleResearchCapLimits(gameData *GameData.Store) map[int64]float64 {
	result := map[int64]float64{}
	catalog, err := gameData.Catalog("effectCaps")
	if err != nil {
		return result
	}
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		capID, capOK := record.Int64("capID")
		maximum, maximumOK := record.Float64("maxTotalBonus")
		if capOK && maximumOK && maximum > 0 {
			result[capID] = maximum
		}
	}
	return result
}

func researchEffectMagnitude(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	if len(values)%2 == 0 {
		paired := true
		for index := 0; index < len(values); index += 2 {
			if values[index] < 1 || math.Trunc(values[index]) != values[index] {
				paired = false
				break
			}
		}
		if paired {
			best := values[1]
			for index := 3; index < len(values); index += 2 {
				best = maxFloat(best, values[index])
			}
			return best
		}
	}
	return values[len(values)-1]
}

func defenseMultiplier(percent float64) float64 {
	return maxFloat(0.1, 1+percent/100)
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func clampFloat(value, minimum, maximum float64) float64 {
	return math.Min(maximum, math.Max(minimum, value))
}
