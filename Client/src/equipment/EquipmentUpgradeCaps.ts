export type EquipmentUpgradeKind = 'equipment' | 'gem';

export interface EquipmentUpgradeClassification {
	rarityID?: number | null;
	relic?: boolean;
	relicKnown?: boolean;
	slot?: number | null;
}

const officialEquipmentLevelCaps = new Map<number, number>([
	[0, 20],
	[1, 3],
	[2, 8],
	[3, 12],
	[4, 16],
	[5, 50],
]);

export function officialUpgradeLevelCap(
	kind: EquipmentUpgradeKind,
	classification: EquipmentUpgradeClassification = {},
): number | null {
	if (kind === 'gem') return 50;
	const { rarityID, relic, relicKnown, slot } = classification;
	if (relicKnown !== true || typeof slot !== 'number' || !Number.isInteger(slot)) return null;
	if (relic === true) return 50;
	if (slot < 1 || slot > 4) return null;
	if (typeof rarityID !== 'number' || !Number.isInteger(rarityID)) return null;
	return officialEquipmentLevelCaps.get(rarityID) ?? null;
}
