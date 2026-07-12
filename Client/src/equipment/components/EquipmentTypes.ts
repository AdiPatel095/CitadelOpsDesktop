import type { EquipmentInstanceV2, GemInstanceV2 } from '../../api/Contracts';

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

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
