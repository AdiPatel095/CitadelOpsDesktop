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
	defenseWallEffectTypeID       = int64(6)
	defenseGateEffectTypeID       = int64(7)
	defenseMoatEffectTypeID       = int64(8)
	defenseMeleeEffectTypeID      = int64(9)
	defenseRangeEffectTypeID      = int64(10)
	wallReductionEffectTypeID     = int64(19)
	gateReductionEffectTypeID     = int64(20)
	moatReductionEffectTypeID     = int64(21)
	offensiveMeleeEffectTypeID    = int64(23)
	offensiveRangeEffectTypeID    = int64(24)
	defenseGlobalEffectTypeID     = int64(31)
	defenseYardEffectTypeID       = int64(32)
	offensiveYardEffectTypeID     = int64(33)
	offensiveGlobalEffectTypeID   = int64(36)
	defenseFrontEffectTypeID      = int64(49)
	defenseFlankEffectTypeID      = int64(50)
	offensiveFrontEffectTypeID    = int64(53)
	offensiveFlankEffectTypeID    = int64(54)
	unitAttackEffectTypeID        = int64(148)
	killDefendingMeleeYardTypeID  = int64(175)
	killDefendingRangedYardTypeID = int64(176)
	killDefendingAnyYardTypeID    = int64(177)

	battleResearchPhaseWinThreshold           = 0.75
	battleResearchAttackerWinCasualtyExponent = 1.4
	battleResearchDefenderWinCasualtyExponent = 1.7
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
	count               int64
	meleePower          float64
	rangePower          float64
	meleeUnitBonusPower float64
	rangeUnitBonusPower float64
	knownCount          int64
}

type researchPowerParts struct {
	melee      float64
	rangePower float64
}

func (parts researchPowerParts) total() float64 {
	return parts.melee + parts.rangePower
}

func (force researchOffenseForce) effectivePower(
	modifiers researchCombatModifiers,
	tools researchToolModifiers,
	phaseBonus float64,
) researchPowerParts {
	common := modifiers.globalBonus + phaseBonus
	parts := researchPowerParts{
		melee: force.meleePower*positiveBattleFactor(
			modifiers.meleeBonus+common+tools.offMeleeBonus,
		) + force.meleeUnitBonusPower,
		rangePower: force.rangePower*positiveBattleFactor(
			modifiers.rangeBonus+common+tools.offRangeBonus,
		) + force.rangeUnitBonusPower,
	}
	if parts.total() <= 0 {
		parts.melee = float64(force.count)
	}
	return parts
}

func (force researchOffenseForce) scale(count int64) researchOffenseForce {
	if force.count <= 0 || count <= 0 {
		return researchOffenseForce{}
	}
	ratio := float64(count) / float64(force.count)
	return researchOffenseForce{
		count: count, meleePower: force.meleePower * ratio, rangePower: force.rangePower * ratio,
		meleeUnitBonusPower: force.meleeUnitBonusPower * ratio,
		rangeUnitBonusPower: force.rangeUnitBonusPower * ratio,
		knownCount:          int64(math.Round(float64(force.knownCount) * ratio)),
	}
}

func (force *researchOffenseForce) add(other researchOffenseForce) {
	force.count += other.count
	force.meleePower += other.meleePower
	force.rangePower += other.rangePower
	force.meleeUnitBonusPower += other.meleeUnitBonusPower
	force.rangeUnitBonusPower += other.rangeUnitBonusPower
	force.knownCount += other.knownCount
}

type researchDefenseForce struct {
	count      int64
	stacks     []researchDefenseStack
	knownCount int64
}

type researchDefenseStack struct {
	amount       float64
	meleeDefense float64
	rangeDefense float64
	ranged       bool
}

type researchYardKills struct {
	melee  float64
	ranged float64
	any    float64
}

func (force researchDefenseForce) effectivePower(
	attacker researchPowerParts,
	modifiers researchCombatModifiers,
	attackTools researchToolModifiers,
	phaseBonus float64,
) float64 {
	if force.count <= 0 {
		return 0
	}
	totalAttack := attacker.total()
	meleeShare := 0.5
	if totalAttack > 0 {
		meleeShare = attacker.melee / totalAttack
	}
	rangeShare := 1 - meleeShare
	common := modifiers.globalBonus + phaseBonus
	power := 0.0
	for _, stack := range force.stacks {
		denominator := meleeShare/maxFloat(0.0001, stack.meleeDefense) +
			rangeShare/maxFloat(0.0001, stack.rangeDefense)
		if denominator > 0 {
			roleBonus := modifiers.meleeBonus - attackTools.defMeleeBonus
			if stack.ranged {
				roleBonus = modifiers.rangeBonus - attackTools.defRangeBonus
			}
			power += stack.amount / denominator * positiveBattleFactor(roleBonus+common)
		}
	}
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
	result := researchDefenseForce{
		count: count, knownCount: int64(math.Round(float64(force.knownCount) * ratio)),
	}
	result.stacks = make([]researchDefenseStack, 0, len(force.stacks))
	for _, stack := range force.stacks {
		stack.amount *= ratio
		result.stacks = append(result.stacks, stack)
	}
	return result
}

func (force *researchDefenseForce) add(other researchDefenseForce) {
	force.count += other.count
	force.stacks = append(force.stacks, other.stacks...)
	force.knownCount += other.knownCount
}

func (force *researchDefenseForce) remove(amount float64, role string) int64 {
	if force == nil || force.count <= 0 || amount <= 0 {
		return 0
	}
	available := 0.0
	for _, stack := range force.stacks {
		if role == "any" || (role == "ranged") == stack.ranged {
			available += stack.amount
		}
	}
	remove := math.Min(amount, available)
	removedCount := minInt64(force.count, int64(math.Round(remove)))
	if remove <= 0 || removedCount <= 0 {
		return 0
	}
	keepFactor := 1 - float64(removedCount)/available
	for index := range force.stacks {
		if role == "any" || (role == "ranged") == force.stacks[index].ranged {
			force.stacks[index].amount *= maxFloat(0, keepFactor)
		}
	}
	originalCount := force.count
	force.count -= removedCount
	force.knownCount = int64(math.Round(
		float64(force.knownCount) * float64(force.count) / float64(originalCount),
	))
	return removedCount
}

type researchCombatModifiers struct {
	meleeBonus  float64
	rangeBonus  float64
	globalBonus float64
	frontBonus  float64
	flankBonus  float64
	yardBonus   float64
	wall        float64
	gate        float64
	moat        float64
	unitAttack  map[int64]float64
}

type researchToolModifiers struct {
	offMeleeBonus float64
	offRangeBonus float64
	defMeleeBonus float64
	defRangeBonus float64
	wall          float64
	gate          float64
	moat          float64
}

type researchFortification struct {
	wall float64
	gate float64
	moat float64
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
	pvp := battleResearchIsPvP(spy)
	supportEffects := battleResearchSupportEffects(formation.SupportToolIDs, gameData)
	attackModifiers := battleResearchCombatModifiers(
		context, supportEffects, movement.TargetTypeID, pvp, gameData,
	)
	supportKills := battleResearchYardKillsFromEffects(
		supportEffects, movement.TargetTypeID, pvp, gameData,
	)
	defenseModifiers := battleResearchSpyCombatModifiers(spy, movement.TargetTypeID, pvp, gameData)
	baseFortification := battleResearchBaseFortification(spy.Castle, gameData)
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
		AttackerMeleeBonus: attackModifiers.meleeBonus,
		AttackerRangeBonus: attackModifiers.rangeBonus,
		WallReduction:      attackModifiers.wall, GateReduction: attackModifiers.gate,
		MoatReduction: attackModifiers.moat,
		Considered:    append([]string(nil), researchCalculatorInfo().Considered...),
		RecordedNotModeled: []string{
			"defender wall and courtyard support tools, because espionage does not reveal their placement",
			"general abilities whose phase triggers are unresolved before combat",
			"nonstandard target fortification bases absent from the building catalog",
			"unrecognized future catalog effects and hidden server modifiers",
		},
		Assumptions: []string{
			"the pre-battle spy deployment remains unchanged until impact",
			"combat percentages share an additive bucket while fortification remains a separate multiplier",
			"a losing phase loses all remaining troops while winning casualties follow side-specific calibrated opposing-power curves",
			"the courtyard is reached only after at least one wall section is breached",
		},
	}

	leftDefense := defenses[0]
	centerDefense := defenses[1]
	rightDefense := defenses[2]
	courtyardAttackers := researchOffenseForce{}
	breached := map[int]bool{}
	knownAttackers := int64(0)
	wallAttackerLost := int64(0)
	wallDefenderLost := int64(0)
	initialAttackPower := 0.0
	initialDefensePower := 0.0
	for _, wave := range formation.Waves {
		leftAttack := researchOffenseFromPairs(wave.Left.Units, gameData, attackModifiers)
		centerAttack := researchOffenseFromPairs(wave.Center.Units, gameData, attackModifiers)
		rightAttack := researchOffenseFromPairs(wave.Right.Units, gameData, attackModifiers)
		leftTools := researchToolModifiersFromPairs(wave.Left.Tools, gameData)
		centerTools := researchToolModifiersFromPairs(wave.Center.Tools, gameData)
		rightTools := researchToolModifiersFromPairs(wave.Right.Tools, gameData)
		knownAttackers += leftAttack.knownCount + centerAttack.knownCount + rightAttack.knownCount

		leftFortification := battleResearchLaneFortification(
			baseFortification, attackModifiers, defenseModifiers, leftTools, false,
		)
		centerFortification := battleResearchLaneFortification(
			baseFortification, attackModifiers, defenseModifiers, centerTools, true,
		)
		rightFortification := battleResearchLaneFortification(
			baseFortification, attackModifiers, defenseModifiers, rightTools, false,
		)
		left, leftSurvivors, leftRemaining := resolveResearchPhase(
			leftAttack, leftDefense, attackModifiers, defenseModifiers, leftTools,
			attackModifiers.flankBonus, defenseModifiers.flankBonus, leftFortification,
		)
		center, centerSurvivors, centerRemaining := resolveResearchPhase(
			centerAttack, centerDefense, attackModifiers, defenseModifiers, centerTools,
			attackModifiers.frontBonus, defenseModifiers.frontBonus, centerFortification,
		)
		right, rightSurvivors, rightRemaining := resolveResearchPhase(
			rightAttack, rightDefense, attackModifiers, defenseModifiers, rightTools,
			attackModifiers.flankBonus, defenseModifiers.flankBonus, rightFortification,
		)
		initialAttackPower += left.AttackerPower + center.AttackerPower + right.AttackerPower
		if wave.Index == 1 {
			initialDefensePower += left.DefenderPower + center.DefenderPower + right.DefenderPower
		}
		leftDefense, centerDefense, rightDefense = leftRemaining, centerRemaining, rightRemaining
		if left.Winner == "attacker" {
			courtyardAttackers.add(leftSurvivors)
			breached[0] = true
		}
		if center.Winner == "attacker" {
			courtyardAttackers.add(centerSurvivors)
			breached[1] = true
		}
		if right.Winner == "attacker" {
			courtyardAttackers.add(rightSurvivors)
			breached[2] = true
		}
		wallAttackerLost += left.AttackerLost + center.AttackerLost + right.AttackerLost
		wallDefenderLost += left.DefenderLost + center.DefenderLost + right.DefenderLost
		prediction.Waves = append(prediction.Waves, BattleWavePrediction{
			Wave: wave.Index, Left: left, Center: center, Right: right,
		})
	}

	support := researchOffenseFromPairs(formation.SupportTroops, gameData, attackModifiers)
	knownAttackers += support.knownCount
	courtyardAttackers.add(support)
	courtyardDefense := defenses[3]
	for index, force := range defenses {
		if index >= 4 {
			courtyardDefense.add(force)
		}
	}
	wallDefenderLost += courtyardDefense.remove(supportKills.melee, "melee")
	wallDefenderLost += courtyardDefense.remove(supportKills.ranged, "ranged")
	wallDefenderLost += courtyardDefense.remove(supportKills.any, "any")
	// Defenders still holding a wall section join the courtyard once another
	// section has been breached.
	courtyardDefense.add(leftDefense)
	courtyardDefense.add(centerDefense)
	courtyardDefense.add(rightDefense)
	if initialDefensePower <= 0 {
		attackParts := courtyardAttackers.effectivePower(
			attackModifiers, researchToolModifiers{}, attackModifiers.yardBonus,
		)
		initialDefensePower = courtyardDefense.effectivePower(
			attackParts, defenseModifiers, researchToolModifiers{}, defenseModifiers.yardBonus,
		)
	}
	if len(breached) == 0 {
		prediction.Courtyard = BattlePhasePrediction{
			Winner: "defender", AttackerStarted: courtyardAttackers.count,
			DefenderStarted:   courtyardDefense.count,
			AttackerSurvivors: courtyardAttackers.count,
			DefenderSurvivors: courtyardDefense.count,
		}
	} else {
		sectionBonus := map[int]float64{1: -30, 2: 0, 3: 30}[len(breached)]
		prediction.Courtyard, _, _ = resolveResearchPhase(
			courtyardAttackers, courtyardDefense, attackModifiers, defenseModifiers,
			researchToolModifiers{}, attackModifiers.yardBonus+sectionBonus,
			defenseModifiers.yardBonus, 0,
		)
	}
	if len(breached) > 0 && prediction.Courtyard.Winner == "attacker" {
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
	ratio := initialAttackPower / maxFloat(1, initialDefensePower*battleResearchPhaseWinThreshold)
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
		meleePower := float64(pair.Amount) * stats.meleeAttack
		rangePower := 0.0
		if stats.ranged {
			meleePower = 0
			rangePower = float64(pair.Amount) * stats.rangeAttack
		}
		force.meleePower += meleePower
		force.rangePower += rangePower
		unitBonus := modifiers.unitAttack[pair.WodID] / 100
		force.meleeUnitBonusPower += meleePower * unitBonus
		force.rangeUnitBonusPower += rangePower * unitBonus
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
			force.stacks = append(force.stacks, researchDefenseStack{
				amount: float64(count.Amount), meleeDefense: 1, rangeDefense: 1,
			})
			continue
		}
		force.knownCount += count.Amount
		force.stacks = append(force.stacks, researchDefenseStack{
			amount: float64(count.Amount), meleeDefense: stats.meleeDefense,
			rangeDefense: stats.rangeDefense, ranged: stats.ranged,
		})
	}
	return force
}

type researchUnitCombatStats struct {
	meleeAttack  float64
	rangeAttack  float64
	meleeDefense float64
	rangeDefense float64
	ranged       bool
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
	role, _ := record.String("role")
	stats.ranged = strings.EqualFold(strings.TrimSpace(role), "ranged") ||
		(strings.TrimSpace(role) == "" && stats.rangeAttack > stats.meleeAttack)
	return stats, stats.meleeAttack > 0 || stats.rangeAttack > 0 || stats.meleeDefense > 0 || stats.rangeDefense > 0
}

func resolveResearchPhase(
	attacker researchOffenseForce,
	defender researchDefenseForce,
	attackModifiers researchCombatModifiers,
	defenseModifiers researchCombatModifiers,
	attackTools researchToolModifiers,
	attackPhaseBonus float64,
	defensePhaseBonus float64,
	fortificationPercent float64,
) (BattlePhasePrediction, researchOffenseForce, researchDefenseForce) {
	result := BattlePhasePrediction{AttackerStarted: attacker.count, DefenderStarted: defender.count}
	attackParts := attacker.effectivePower(attackModifiers, attackTools, attackPhaseBonus)
	result.AttackerPower = math.Round(attackParts.total()*100) / 100
	result.DefenderPower = math.Round(
		defender.effectivePower(
			attackParts, defenseModifiers, attackTools, defensePhaseBonus,
		)*positiveBattleFactor(fortificationPercent)*100,
	) / 100
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
	if result.AttackerPower > result.DefenderPower*battleResearchPhaseWinThreshold {
		result.Winner = "attacker"
		result.DefenderLost = defender.count
		lossShare := math.Pow(
			maxFloat(0, result.DefenderPower)/maxFloat(1, result.AttackerPower),
			battleResearchAttackerWinCasualtyExponent,
		)
		result.AttackerLost = minInt64(attacker.count, int64(math.Round(float64(attacker.count)*lossShare)))
		result.AttackerSurvivors = attacker.count - result.AttackerLost
		return result, attacker.scale(result.AttackerSurvivors), researchDefenseForce{}
	}
	result.Winner = "defender"
	result.AttackerLost = attacker.count
	lossShare := math.Pow(
		maxFloat(0, result.AttackerPower)/maxFloat(1, result.DefenderPower),
		battleResearchDefenderWinCasualtyExponent,
	)
	result.DefenderLost = minInt64(defender.count, int64(math.Round(float64(defender.count)*lossShare)))
	result.DefenderSurvivors = defender.count - result.DefenderLost
	return result, researchOffenseForce{}, defender.scale(result.DefenderSurvivors)
}

func battleResearchSupportEffects(
	toolIDs []int64,
	gameData *GameData.Store,
) []State.EquipmentEffect {
	if len(toolIDs) == 0 || gameData == nil {
		return nil
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return nil
	}
	result := make([]State.EquipmentEffect, 0)
	for _, toolID := range toolIDs {
		if toolID <= 0 {
			continue
		}
		raw, found := catalog.Find(strconv.FormatInt(toolID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		encoded, _ := record.String("effects")
		for _, effect := range strings.Split(encoded, ",") {
			parts := strings.SplitN(strings.TrimSpace(effect), "&", 2)
			if len(parts) != 2 {
				continue
			}
			effectID, parseErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if parseErr != nil || effectID <= 0 {
				continue
			}
			values := make([]float64, 0)
			for _, value := range strings.FieldsFunc(parts[1], func(character rune) bool {
				return character == '#' || character == '+'
			}) {
				parsed, valueErr := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if valueErr == nil {
					values = append(values, parsed)
				}
			}
			if len(values) > 0 {
				result = append(result, State.EquipmentEffect{
					WireID: effectID, DefinitionID: effectID, Values: values,
				})
			}
		}
	}
	return result
}

func battleResearchYardKillsFromEffects(
	effects []State.EquipmentEffect,
	targetTypeID int,
	pvp bool,
	gameData *GameData.Store,
) researchYardKills {
	result := researchYardKills{}
	if len(effects) == 0 || gameData == nil {
		return result
	}
	catalog, err := gameData.Catalog("effects")
	if err != nil {
		return result
	}
	for _, effect := range effects {
		raw, found := catalog.Find(strconv.FormatInt(effect.DefinitionID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil || !battleResearchEffectApplies(record, targetTypeID, pvp) {
			continue
		}
		effectType, _ := record.Int64("effectTypeID")
		value := researchEffectMagnitude(effect.Values)
		switch effectType {
		case killDefendingMeleeYardTypeID:
			result.melee += value
		case killDefendingRangedYardTypeID:
			result.ranged += value
		case killDefendingAnyYardTypeID:
			result.any += value
		}
	}
	return result
}

func battleResearchCombatModifiers(
	context BattleResearchAttackerContext,
	supportEffects []State.EquipmentEffect,
	targetTypeID int,
	pvp bool,
	gameData *GameData.Store,
) researchCombatModifiers {
	effects := make([]State.EquipmentEffect, 0)
	effects = append(effects, supportEffects...)
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
	modifiers := battleResearchModifiersFromEffects(effects, targetTypeID, pvp, true, gameData)
	battleResearchApplyLegendSkills(&modifiers, context.LegendSkills.ActiveIDs, true, gameData)
	return modifiers
}

func battleResearchSpyCombatModifiers(
	spy SpyReport,
	targetTypeID int,
	pvp bool,
	gameData *GameData.Store,
) researchCombatModifiers {
	if spy.Castellan == nil {
		return researchCombatModifiers{unitAttack: map[int64]float64{}}
	}
	effects := battleResearchSpyEffects(*spy.Castellan, gameData)
	return battleResearchModifiersFromEffects(effects, targetTypeID, pvp, false, gameData)
}

type battleResearchCapKey struct {
	effectType int64
	capID      int64
}

type battleResearchUnitCapKey struct {
	unitID int64
	capID  int64
}

func battleResearchModifiersFromEffects(
	effects []State.EquipmentEffect,
	targetTypeID int,
	pvp bool,
	attacker bool,
	gameData *GameData.Store,
) researchCombatModifiers {
	groups := map[battleResearchCapKey]float64{}
	unitGroups := map[battleResearchUnitCapKey]float64{}
	maximums := battleResearchCapLimits(gameData)
	effectCatalog, err := gameData.Catalog("effects")
	if err != nil {
		return researchCombatModifiers{unitAttack: map[int64]float64{}}
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
		if decodeErr != nil || !battleResearchEffectApplies(record, targetTypeID, pvp) {
			continue
		}
		effectType, ok := record.Int64("effectTypeID")
		if !ok {
			continue
		}
		operation, modeled := battleEffectOperationFor(effectType)
		if !modeled || operation == battleEffectFormationEmbedded ||
			operation == battleEffectUnitReplacementEmbedded ||
			operation == battleEffectPostBattleRecovery ||
			operation == battleEffectPrecombatReserveKill {
			continue
		}
		if attacker && operation != battleEffectAttackRolePercent &&
			operation != battleEffectAttackGlobalPercent &&
			operation != battleEffectAttackPhasePercent &&
			operation != battleEffectAttackFortificationReduce &&
			operation != battleEffectAttackUnitPercent {
			continue
		}
		if !attacker && operation != battleEffectDefenseRolePercent &&
			operation != battleEffectDefenseGlobalPercent &&
			operation != battleEffectDefensePhasePercent &&
			operation != battleEffectDefenseFortificationAdd {
			continue
		}
		capID, _ := record.Int64("capID")
		if effectType == unitAttackEffectTypeID {
			for index := 0; index+1 < len(effect.Values); index += 2 {
				unitID := int64(effect.Values[index])
				if unitID > 0 {
					unitGroups[battleResearchUnitCapKey{unitID: unitID, capID: capID}] += effect.Values[index+1]
				}
			}
			continue
		}
		groups[battleResearchCapKey{effectType: effectType, capID: capID}] += researchEffectMagnitude(effect.Values)
	}
	modifiers := researchCombatModifiers{unitAttack: map[int64]float64{}}
	for key, value := range groups {
		if maximum := maximums[key.capID]; maximum > 0 && value > maximum {
			value = maximum
		}
		switch key.effectType {
		case offensiveMeleeEffectTypeID:
			modifiers.meleeBonus += value
		case offensiveRangeEffectTypeID:
			modifiers.rangeBonus += value
		case offensiveGlobalEffectTypeID:
			modifiers.globalBonus += value
		case offensiveFrontEffectTypeID:
			modifiers.frontBonus += value
		case offensiveFlankEffectTypeID:
			modifiers.flankBonus += value
		case offensiveYardEffectTypeID:
			modifiers.yardBonus += value
		case wallReductionEffectTypeID:
			modifiers.wall += value
		case gateReductionEffectTypeID:
			modifiers.gate += value
		case moatReductionEffectTypeID:
			modifiers.moat += value
		case defenseMeleeEffectTypeID:
			modifiers.meleeBonus += value
		case defenseRangeEffectTypeID:
			modifiers.rangeBonus += value
		case defenseGlobalEffectTypeID:
			modifiers.globalBonus += value
		case defenseFrontEffectTypeID:
			modifiers.frontBonus += value
		case defenseFlankEffectTypeID:
			modifiers.flankBonus += value
		case defenseYardEffectTypeID:
			modifiers.yardBonus += value
		case defenseWallEffectTypeID:
			modifiers.wall += value
		case defenseGateEffectTypeID:
			modifiers.gate += value
		case defenseMoatEffectTypeID:
			modifiers.moat += value
		}
	}
	for key, value := range unitGroups {
		if maximum := maximums[key.capID]; maximum > 0 && value > maximum {
			value = maximum
		}
		modifiers.unitAttack[key.unitID] += value
	}
	if attacker {
		// Ignore defense-only rows that can coexist in generic active-effect lists.
		modifiers = modifiers.attackOnly()
	} else {
		modifiers = modifiers.defenseOnly()
	}
	return modifiers
}

func (modifiers researchCombatModifiers) attackOnly() researchCombatModifiers {
	return researchCombatModifiers{
		meleeBonus: modifiers.meleeBonus, rangeBonus: modifiers.rangeBonus,
		globalBonus: modifiers.globalBonus, frontBonus: modifiers.frontBonus,
		flankBonus: modifiers.flankBonus, yardBonus: modifiers.yardBonus,
		wall: modifiers.wall, gate: modifiers.gate, moat: modifiers.moat,
		unitAttack: modifiers.unitAttack,
	}
}

func (modifiers researchCombatModifiers) defenseOnly() researchCombatModifiers {
	modifiers.unitAttack = map[int64]float64{}
	return modifiers
}

func battleResearchEffectApplies(record GameData.Record, targetTypeID int, pvp bool) bool {
	if isPvP, exists := record.Int64("isPvPFight"); exists && (isPvP != 0) != pvp {
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

func battleResearchIsPvP(spy SpyReport) bool {
	if spy.Target.ID <= 0 {
		return false
	}
	if spy.Target.Dummy != nil {
		return !*spy.Target.Dummy
	}
	return spy.Source.ID > 0
}

func battleResearchApplyLegendSkills(
	modifiers *researchCombatModifiers,
	skillIDs []int64,
	attacker bool,
	gameData *GameData.Store,
) {
	if modifiers == nil || len(skillIDs) == 0 {
		return
	}
	catalog, err := gameData.Catalog("legendskills")
	if err != nil {
		return
	}
	for _, skillID := range skillIDs {
		raw, found := catalog.Find(strconv.FormatInt(skillID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		effectType, _ := record.String("effectType")
		value, _ := record.Float64("totalEffectValue")
		if attacker {
			switch effectType {
			case "attackMeleeBonus":
				modifiers.meleeBonus += value
			case "attackRangeBonus":
				modifiers.rangeBonus += value
			case "attackBonus":
				modifiers.globalBonus += value
			case "attackYardBonus":
				modifiers.yardBonus += value
			case "wallReduction":
				modifiers.wall += value
			case "gateReduction":
				modifiers.gate += value
			case "moatReduction":
				modifiers.moat += value
			}
			continue
		}
		switch effectType {
		case "defenseMeleeBonus":
			modifiers.meleeBonus += value
		case "defenseRangeBonus":
			modifiers.rangeBonus += value
		case "defenseBonus":
			modifiers.globalBonus += value
		case "defenseYardBonus":
			modifiers.yardBonus += value
		case "wallBonus":
			modifiers.wall += value
		case "gateBonus":
			modifiers.gate += value
		case "moatBonus":
			modifiers.moat += value
		}
	}
}

func battleResearchSpyEffects(castellan SpyCastellan, gameData *GameData.Store) []State.EquipmentEffect {
	result := battleResearchDecodeWireEffects(castellan.Effects, func(id int64) int64 { return id })
	for _, rawEquipment := range battleResearchRawRows(castellan.Equipment) {
		var equipment []json.RawMessage
		if json.Unmarshal(rawEquipment, &equipment) != nil || len(equipment) <= 5 {
			continue
		}
		typeID, _ := rawInteger(equipment[3])
		relic := typeID == 5 || typeID == 15
		result = append(result, battleResearchDecodeRawEffectRows(
			equipment[5],
			func(id int64) int64 { return battleResearchResolveWireEffectID(id, relic, gameData) },
		)...)
		if len(equipment) <= 12 {
			continue
		}
		var socket []json.RawMessage
		if json.Unmarshal(equipment[12], &socket) != nil || len(socket) <= 3 {
			continue
		}
		var gem []json.RawMessage
		if json.Unmarshal(socket[3], &gem) != nil || len(gem) <= 4 {
			continue
		}
		result = append(result, battleResearchDecodeRawEffectRows(
			gem[4],
			func(id int64) int64 { return battleResearchResolveWireEffectID(id, true, gameData) },
		)...)
	}
	return result
}

func battleResearchRawRows(value any) []json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	return rows
}

func battleResearchDecodeWireEffects(
	value any,
	resolve func(int64) int64,
) []State.EquipmentEffect {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return battleResearchDecodeRawEffectRows(raw, resolve)
}

func battleResearchDecodeRawEffectRows(
	raw json.RawMessage,
	resolve func(int64) int64,
) []State.EquipmentEffect {
	var encodedRows []json.RawMessage
	if json.Unmarshal(raw, &encodedRows) != nil {
		return nil
	}
	result := make([]State.EquipmentEffect, 0, len(encodedRows))
	for _, encoded := range encodedRows {
		var row []json.RawMessage
		if json.Unmarshal(encoded, &row) != nil || len(row) < 2 {
			continue
		}
		wireID, ok := rawInteger(row[0])
		if !ok || wireID <= 0 {
			continue
		}
		values := battleResearchRawFloatSlice(row[1])
		if len(values) == 0 && len(row) >= 3 {
			values = battleResearchRawFloatSlice(row[2])
		}
		if len(values) == 0 {
			continue
		}
		definitionID := wireID
		if resolve != nil {
			definitionID = resolve(wireID)
		}
		result = append(result, State.EquipmentEffect{
			WireID: wireID, DefinitionID: definitionID, Values: values,
		})
	}
	return result
}

func battleResearchRawFloatSlice(raw json.RawMessage) []float64 {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make([]float64, 0, len(values))
	for _, value := range values {
		var number json.Number
		if json.Unmarshal(value, &number) == nil {
			parsed, err := number.Float64()
			if err == nil {
				result = append(result, parsed)
				continue
			}
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err == nil {
				result = append(result, parsed)
			}
		}
	}
	return result
}

func battleResearchResolveWireEffectID(id int64, relic bool, gameData *GameData.Store) int64 {
	catalogName := "equipment_effects"
	if relic {
		catalogName = "relicEffects"
	}
	catalog, err := gameData.Catalog(catalogName)
	if err != nil {
		return id
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return id
	}
	record, decodeErr := GameData.DecodeRecord(raw)
	if decodeErr != nil {
		return id
	}
	effectID, ok := record.Int64("effectID")
	if !ok || effectID <= 0 {
		return id
	}
	return effectID
}

func researchToolModifiersFromPairs(pairs []ResearchPair, gameData *GameData.Store) researchToolModifiers {
	result := researchToolModifiers{}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return result
	}
	for _, pair := range pairs {
		if pair.WodID <= 0 || pair.Amount <= 0 {
			continue
		}
		raw, found := catalog.Find(strconv.FormatInt(pair.WodID, 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil || !GameData.IsToolRecord(record) {
			continue
		}
		amount := float64(pair.Amount)
		result.offMeleeBonus += amount * battleResearchRecordFloat(record, "offMeleeBonus")
		result.offRangeBonus += amount * battleResearchRecordFloat(record, "offRangeBonus")
		result.defMeleeBonus += amount * battleResearchRecordFloat(record, "defMeleeBonus")
		result.defRangeBonus += amount * battleResearchRecordFloat(record, "defRangeBonus")
		result.wall += amount * battleResearchRecordFloat(record, "wallBonus")
		result.gate += amount * battleResearchRecordFloat(record, "gateBonus")
		result.moat += amount * battleResearchRecordFloat(record, "moatBonus")
	}
	return result
}

func battleResearchRecordFloat(record GameData.Record, field string) float64 {
	value, _ := record.Float64(field)
	return value
}

func battleResearchBaseFortification(castle Castle, gameData *GameData.Store) researchFortification {
	moatLevel := castle.MoatLevel
	moatName := "Basic"
	if moatLevel > 4 {
		moatLevel -= 4
		moatName = "Premium"
	}
	return researchFortification{
		wall: battleResearchBuildingBonus(gameData, "wallBonus", castle.WallLevel, "Castlewall"),
		gate: battleResearchBuildingBonus(gameData, "gateBonus", castle.GateLevel, "Basic"),
		moat: battleResearchBuildingBonus(gameData, "moatBonus", moatLevel, moatName),
	}
}

func battleResearchBuildingBonus(
	gameData *GameData.Store,
	field string,
	level int,
	preferredName string,
) float64 {
	if level <= 0 {
		return 0
	}
	catalog, err := gameData.Catalog("buildings")
	if err != nil {
		return 0
	}
	best := 0.0
	preferred := 0.0
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		recordLevel, ok := record.Int64("level")
		if !ok || recordLevel != int64(level) {
			continue
		}
		bonus, ok := record.Float64(field)
		if !ok || bonus <= 0 {
			continue
		}
		best = maxFloat(best, bonus)
		name, _ := record.String("name")
		if strings.EqualFold(strings.TrimSpace(name), preferredName) {
			preferred = maxFloat(preferred, bonus)
		}
	}
	if preferred > 0 {
		return preferred
	}
	return best
}

func battleResearchLaneFortification(
	base researchFortification,
	attack researchCombatModifiers,
	defense researchCombatModifiers,
	tools researchToolModifiers,
	center bool,
) float64 {
	wall := maxFloat(0, base.wall+defense.wall-attack.wall-tools.wall)
	gate := maxFloat(0, base.gate+defense.gate-attack.gate-tools.gate)
	moat := maxFloat(0, base.moat+defense.moat-attack.moat-tools.moat)
	result := wall + moat
	if center {
		result += gate
	}
	return result
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

func positiveBattleFactor(percent float64) float64 {
	return maxFloat(0, 1+percent/100)
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
