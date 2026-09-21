import type { MessageKey } from '../../i18n/messages';
import type { EquipmentInstanceV2, GemInstanceV2 } from '../../api/Contracts';

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

export interface EquipmentTarget {
	id: string;
	label: string;
	description: string;
	castleTypeID: number;
	labelKey?: MessageKey;
	descriptionKey?: MessageKey;
}

const knownCastleTypeTargets: EquipmentTarget[] = [
	castleTarget(1, 'Main Castle', 'equipment.target.1.label', 'equipment.target.1.description'),
	castleTarget(4, 'Outpost', 'equipment.target.4.label', 'equipment.target.4.description'),
	castleTarget(12, 'Kingdom Castle', 'equipment.target.12.label', 'equipment.target.12.description'),
	castleTarget(3, 'Capital', 'equipment.target.3.label', 'equipment.target.3.description'),
	castleTarget(22, 'Trading Metropolis', 'equipment.target.22.label', 'equipment.target.22.description'),
	castleTarget(10, 'Resource Village', 'equipment.target.10.label', 'equipment.target.10.description'),
	castleTarget(23, 'Royal Tower', 'equipment.target.23.label', 'equipment.target.23.description'),
	castleTarget(26, 'Monument', 'equipment.target.26.label', 'equipment.target.26.description'),
	castleTarget(28, 'Laboratory', 'equipment.target.28.label', 'equipment.target.28.description'),
	castleTarget(21, 'Foreign Castle', 'equipment.target.21.label', 'equipment.target.21.description'),
	castleTarget(34, 'Bloodcrow Castle', 'equipment.target.34.label', 'equipment.target.34.description'),
	castleTarget(2, 'Robber Baron / Kingdom Tower', 'equipment.target.2.label', 'equipment.target.2.description'),
	castleTarget(11, 'Kingdom Fortress', 'equipment.target.11.label', 'equipment.target.11.description'),
	castleTarget(13, 'Event Fortress', 'equipment.target.13.label', 'equipment.target.13.description'),
	castleTarget(27, 'Nomad Camp', 'equipment.target.27.label', 'equipment.target.27.description'),
	castleTarget(35, 'Khan Camp', 'equipment.target.35.label', 'equipment.target.35.description'),
	castleTarget(29, 'Samurai Camp', 'equipment.target.29.label', 'equipment.target.29.description'),
	castleTarget(37, 'Daimyo Castle', 'equipment.target.37.label', 'equipment.target.37.description'),
	castleTarget(38, 'Daimyo Township', 'equipment.target.38.label', 'equipment.target.38.description'),
	castleTarget(15, 'Berimond Camp', 'equipment.target.15.label', 'equipment.target.15.description'),
	castleTarget(16, 'Berimond Resource Village', 'equipment.target.16.label', 'equipment.target.16.description'),
	castleTarget(17, 'Berimond Watchtower', 'equipment.target.17.label', 'equipment.target.17.description'),
	castleTarget(18, 'Berimond Capital', 'equipment.target.18.label', 'equipment.target.18.description'),
	castleTarget(30, 'Berimond Invasion Camp', 'equipment.target.30.label', 'equipment.target.30.description'),
	castleTarget(24, 'Resource Island', 'equipment.target.24.label', 'equipment.target.24.description'),
	castleTarget(25, 'Storm Fort', 'equipment.target.25.label', 'equipment.target.25.description'),
	castleTarget(40, 'Treasure Obelisk', 'equipment.target.40.label', 'equipment.target.40.description'),
	castleTarget(41, 'Alliance Tower', 'equipment.target.41.label', 'equipment.target.41.description'),
	castleTarget(42, "Wolfgard's Lair", 'equipment.target.42.label', 'equipment.target.42.description'),
	castleTarget(43, 'Rift Raid', 'equipment.target.43.label', 'equipment.target.43.description'),
];

export function equipmentTargets(): EquipmentTarget[] {
	return [...knownCastleTypeTargets];
}

export function equipmentTargetLabel(castleTypeID: number): string {
	return knownCastleTypeTargets.find((target) => target.castleTypeID === castleTypeID)?.label
		?? `Official target ${castleTypeID}`;
}

function castleTarget(castleTypeID: number, label: string, labelKey: MessageKey, descriptionKey: MessageKey): EquipmentTarget {
	return {
		id: `castle-${castleTypeID}`,
		castleTypeID,
		labelKey,
		descriptionKey,
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
	{ slot: 1, label: 'Armor', labelKey: 'equipment.slot.armor' },
	{ slot: 2, label: 'Weapon', labelKey: 'equipment.slot.weapon' },
	{ slot: 3, label: 'Helmet', labelKey: 'equipment.slot.helmet' },
	{ slot: 4, label: 'Artifact', labelKey: 'equipment.slot.artifact' },
	{ slot: 6, label: 'Hero', labelKey: 'equipment.slot.hero' },
] as const;
