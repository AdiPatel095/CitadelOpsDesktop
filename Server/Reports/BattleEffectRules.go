package Reports

// battleEffectOperation is intentionally semantic. Catalog values are not all
// percentages in the same bucket: some modify a role, some modify a phase,
// some alter fortification, and some have already changed the packet's troop
// formation before the calculator sees it.
type battleEffectOperation string

const (
	battleEffectAttackRolePercent         battleEffectOperation = "attack-role-percent"
	battleEffectDefenseRolePercent        battleEffectOperation = "defense-role-percent"
	battleEffectAttackGlobalPercent       battleEffectOperation = "attack-global-percent"
	battleEffectDefenseGlobalPercent      battleEffectOperation = "defense-global-percent"
	battleEffectAttackPhasePercent        battleEffectOperation = "attack-phase-percent"
	battleEffectDefensePhasePercent       battleEffectOperation = "defense-phase-percent"
	battleEffectAttackFortificationReduce battleEffectOperation = "attack-fortification-reduce"
	battleEffectDefenseFortificationAdd   battleEffectOperation = "defense-fortification-add"
	battleEffectAttackUnitPercent         battleEffectOperation = "attack-unit-percent"
	battleEffectFormationEmbedded         battleEffectOperation = "formation-embedded"
	battleEffectUnitReplacementEmbedded   battleEffectOperation = "unit-replacement-embedded"
	battleEffectPostBattleRecovery        battleEffectOperation = "post-battle-recovery"
	battleEffectPrecombatReserveKill      battleEffectOperation = "precombat-reserve-kill"
)

var battleEffectOperations = map[int64]battleEffectOperation{
	6:   battleEffectDefenseFortificationAdd,
	7:   battleEffectDefenseFortificationAdd,
	8:   battleEffectDefenseFortificationAdd,
	9:   battleEffectDefenseRolePercent,
	10:  battleEffectDefenseRolePercent,
	19:  battleEffectAttackFortificationReduce,
	20:  battleEffectAttackFortificationReduce,
	21:  battleEffectAttackFortificationReduce,
	23:  battleEffectAttackRolePercent,
	24:  battleEffectAttackRolePercent,
	31:  battleEffectDefenseGlobalPercent,
	32:  battleEffectDefensePhasePercent,
	33:  battleEffectAttackPhasePercent,
	36:  battleEffectAttackGlobalPercent,
	49:  battleEffectDefensePhasePercent,
	50:  battleEffectDefensePhasePercent,
	53:  battleEffectAttackPhasePercent,
	54:  battleEffectAttackPhasePercent,
	58:  battleEffectPostBattleRecovery,
	148: battleEffectAttackUnitPercent,
	172: battleEffectPrecombatReserveKill,
	173: battleEffectPrecombatReserveKill,
	174: battleEffectPrecombatReserveKill,
	175: battleEffectPrecombatReserveKill,
	176: battleEffectPrecombatReserveKill,
	177: battleEffectPrecombatReserveKill,
	208: battleEffectPrecombatReserveKill,
}

func init() {
	for _, effectType := range []int64{12, 28, 34, 46, 47, 51, 99, 156, 179, 180, 181, 182, 183, 194} {
		battleEffectOperations[effectType] = battleEffectFormationEmbedded
	}
	battleEffectOperations[118] = battleEffectUnitReplacementEmbedded
}

func battleEffectOperationFor(effectType int64) (battleEffectOperation, bool) {
	operation, found := battleEffectOperations[effectType]
	return operation, found
}

// General ability rows in BLM are already resolved to a phase, lane, and
// magnitude. The replay calculator uses this registry to keep opposing-army
// reducers and defender-side attenuation directional.
var battleGeneralOperations = map[int64]string{
	1001: "self-percent",
	1002: "flat-power",
	1003: "giant-slayer-flat-power",
	1005: "enemy-global-reduce",
	1007: "self-percent",
	1010: "self-percent",
	1011: "self-melee-percent",
	1012: "attenuate-enemy-general",
	1013: "attenuate-enemy-tools",
	1014: "flat-power",
	1015: "casualty-only-enemy-ranged-reduce",
	1016: "formation-embedded",
	1018: "self-percent",
	1019: "self-percent",
	1020: "enemy-global-reduce",
	1021: "precombat-nullification-embedded",
	1022: "precombat-kill",
	1023: "minimum-ranged-percent",
	1025: "attenuate-enemy-ranged-reduce",
	1026: "add-courtyard-units",
	1027: "self-percent",
	1028: "noncombat",
	1029: "self-percent",
	1030: "dynamic-courtyard-percent",
	1033: "self-ranged-and-enemy-ranged-reduce",
	1034: "precombat-melee-kill",
	1035: "enemy-role-reduce",
	1038: "dynamic-courtyard-percent",
	1039: "enemy-global-reduce",
}
