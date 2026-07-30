package AttackCapacity

import (
	"math"

	"CitadelDesktop/Server/State"
)

// DungeonTargetLevel mirrors the game client's DungeonConst.getLevel formula.
// GAA tower rows provide a victory count; that value is not a lane capacity.
func DungeonTargetLevel(victoryCount int64, kingdomID State.KingdomID) int {
	offset := 0
	switch kingdomID {
	case 0:
		offset = 1
	case 2:
		offset = 20
	case 1:
		offset = 35
	case 3:
		offset = 45
	}
	return int(math.Floor(1.9*math.Pow(math.Abs(float64(victoryCount)), 0.555))) + offset
}

// BaseCapacityForTargetLevel mirrors CombatConst's target-level lane limits.
// Commander, castle, construction-item, and other bonuses are applied later.
func BaseCapacityForTargetLevel(targetLevel int) LaneCapacity {
	maxAttackers := maxAttackersForTargetLevel(targetLevel)
	if maxAttackers <= 0 {
		return LaneCapacity{}
	}
	flank := (maxAttackers + 4) / 5
	return LaneCapacity{Left: flank, Front: maxAttackers - 2*flank, Right: flank}
}

func maxAttackersForTargetLevel(targetLevel int) int64 {
	if targetLevel <= 0 {
		return 0
	}
	if targetLevel <= 69 {
		return min(260, int64(5*targetLevel+8))
	}
	return 320
}
