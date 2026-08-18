import type { EquipmentInstanceV2, GemInstanceV2 } from '../../api/Contracts';

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

export interface EquipmentTarget {
	id: string;
	label: string;
	description: string;
	castleTypeID: number;
}

const knownCastleTypeTargets: EquipmentTarget[] = [
	castleTarget(1, 'Main Castle'),
	castleTarget(4, 'Outpost'),
	castleTarget(12, 'Kingdom Castle'),
	castleTarget(3, 'Capital'),
	castleTarget(22, 'Trading Metropolis'),
	castleTarget(10, 'Resource Village'),
	castleTarget(23, 'Royal Tower'),
	castleTarget(26, 'Monument'),
	castleTarget(28, 'Laboratory'),
	castleTarget(21, 'Foreign Castle'),
	castleTarget(34, 'Bloodcrow Castle'),
	castleTarget(2, 'Robber Baron / Kingdom Tower'),
	castleTarget(11, 'Kingdom Fortress'),
	castleTarget(13, 'Event Fortress'),
	castleTarget(27, 'Nomad Camp'),
	castleTarget(35, 'Khan Camp'),
	castleTarget(29, 'Samurai Camp'),
	castleTarget(37, 'Daimyo Castle'),
	castleTarget(38, 'Daimyo Township'),
	castleTarget(15, 'Berimond Camp'),
	castleTarget(16, 'Berimond Resource Village'),
	castleTarget(17, 'Berimond Watchtower'),
	castleTarget(18, 'Berimond Capital'),
	castleTarget(30, 'Berimond Invasion Camp'),
	castleTarget(24, 'Resource Island'),
	castleTarget(25, 'Storm Fort'),
	castleTarget(40, 'Treasure Obelisk'),
	castleTarget(41, 'Alliance Tower'),
	castleTarget(42, "Wolfgard's Lair"),
	castleTarget(43, 'Rift Raid'),
];

export function equipmentTargets(): EquipmentTarget[] {
	return [...knownCastleTypeTargets];
}

export function equipmentTargetLabel(castleTypeID: number): string {
	return knownCastleTypeTargets.find((target) => target.castleTypeID === castleTypeID)?.label
		?? `Official target ${castleTypeID}`;
}

function castleTarget(castleTypeID: number, label: string): EquipmentTarget {
	return {
		id: `castle-${castleTypeID}`,
		castleTypeID,
		label,
		description: `Combines every equipment effect that applies when battling this ${label.toLowerCase()}.`,
	};
}

export interface EquipmentLeader {
	kind: 'commander' | 'castellan';
	id: number;
	name: string;
	position: number;
	available: boolean;
	equipment: Record<string, number>;
	gems: Record<string, number>;
}

export interface EquipmentSlotRow {
	slot: number;
	label: string;
	item?: EquipmentInstanceV2;
	gem?: GemInstanceV2;
}

export const equipmentSlots = [
	{ slot: 1, label: 'Armor' },
	{ slot: 2, label: 'Weapon' },
	{ slot: 3, label: 'Helmet' },
	{ slot: 4, label: 'Artifact' },
	{ slot: 6, label: 'Hero' },
] as const;
