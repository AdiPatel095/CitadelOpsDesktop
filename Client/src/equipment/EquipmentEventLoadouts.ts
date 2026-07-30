import type { GameStateV2 } from '../api/Contracts';
import type { MetadataItem } from '../context/MetadataContext';
import type { EquipmentLeader } from './components/EquipmentTypes';

export type EquipmentEventKey =
	| 'nomad'
	| 'samurai'
	| 'berimond'
	| 'foreign_lords'
	| 'hollow_moon_pvp';

export interface EquipmentEventOption {
	value: EquipmentEventKey;
	label: string;
	description: string;
	setIDs: number[];
}

export interface EquipmentEventAvailability {
	option: EquipmentEventOption;
	setID: number;
	tier?: 'Bronze' | 'Silver' | 'Gold';
	equipmentCount: number;
	gemCount: number;
	complete: boolean;
}

export const equipmentEventOptions: EquipmentEventOption[] = [
	{
		value: 'nomad',
		label: 'Nomad Invasion',
		description: 'Commander equipment built for Nomad and Khan camps.',
		setIDs: [1087],
	},
	{
		value: 'samurai',
		label: 'Samurai Invasion',
		description: 'Commander equipment built for Samurai and Daimyo targets.',
		setIDs: [1088],
	},
	{
		value: 'berimond',
		label: 'Battle for Berimond',
		description: 'Commander equipment built for Berimond camps and towers.',
		setIDs: [1089],
	},
	{
		value: 'foreign_lords',
		label: 'Foreign Lords & Bloodcrows',
		description: 'The Glory set for Foreign Lord and Bloodcrow castles.',
		setIDs: [1090],
	},
	{
		value: 'hollow_moon_pvp',
		label: 'Hollow Moon PvP',
		description: 'Chooses one coherent Bronze, Silver, or Gold 2026 PvP set.',
		setIDs: [1096, 1095, 1094],
	},
];

export function resolveEquipmentEventAvailability(
	state: GameStateV2,
	leader: EquipmentLeader,
	equipmentMetadata: Record<number, MetadataItem>,
	gemMetadata: Record<number, MetadataItem>,
): EquipmentEventAvailability[] {
	return equipmentEventOptions.map((option) => {
		const candidates = option.setIDs.map((setID) => eventSetAvailability(
			state,
			leader,
			equipmentMetadata,
			gemMetadata,
			option,
			setID,
		));
		candidates.sort((left, right) => {
			if (left.complete !== right.complete) return left.complete ? -1 : 1;
			if (left.equipmentCount !== right.equipmentCount) return right.equipmentCount - left.equipmentCount;
			if (left.gemCount !== right.gemCount) return right.gemCount - left.gemCount;
			return right.setID - left.setID;
		});
		return candidates[0]!;
	});
}

function eventSetAvailability(
	state: GameStateV2,
	leader: EquipmentLeader,
	equipmentMetadata: Record<number, MetadataItem>,
	gemMetadata: Record<number, MetadataItem>,
	option: EquipmentEventOption,
	setID: number,
): EquipmentEventAvailability {
	const equipmentSlots = new Set<number>();
	const accessibleEquipment = new Set<number>();
	for (const item of Object.values(state.inventory.equipment)) {
		if (!equipmentAvailableToLeader(item.wearerKind, item.wearerId, leader) || item.typeId !== 2) continue;
		accessibleEquipment.add(item.id);
		const observedSetID = Number(item.setId ?? equipmentMetadata[item.definitionId]?.setID ?? 0);
		if (observedSetID === setID && [1, 2, 3, 4, 6].includes(item.slot)) equipmentSlots.add(item.slot);
	}

	const gemDefinitions = new Set<number>();
	for (const gem of Object.values(state.inventory.gems)) {
		const observedSetID = Number(gem.setId ?? gemMetadata[gem.definitionId]?.setID ?? 0);
		if (observedSetID !== setID) continue;
		if (gem.equipmentInstanceId) {
			if (!accessibleEquipment.has(gem.equipmentInstanceId)) continue;
		} else if (gem.wearerKind) {
			continue;
		}
		gemDefinitions.add(gem.definitionId);
	}
	for (const [rawDefinitionID, amount] of Object.entries(state.inventory.gemStacks)) {
		if (amount <= 0) continue;
		const definitionID = Number(rawDefinitionID);
		if (Number(gemMetadata[definitionID]?.setID ?? 0) === setID) gemDefinitions.add(definitionID);
	}

	const equipmentCount = equipmentSlots.size;
	const gemCount = Math.min(4, gemDefinitions.size);
	return {
		option,
		setID,
		tier: hollowMoonTier(setID),
		equipmentCount,
		gemCount,
		complete: equipmentCount === 5 && gemCount === 4,
	};
}

function equipmentAvailableToLeader(
	wearerKind: string | undefined,
	wearerID: number | undefined,
	leader: EquipmentLeader,
): boolean {
	if (!wearerKind) return true;
	return wearerKind === leader.kind && (wearerID ?? 0) === leader.id;
}

function hollowMoonTier(setID: number): 'Bronze' | 'Silver' | 'Gold' | undefined {
	if (setID === 1096) return 'Gold';
	if (setID === 1095) return 'Silver';
	if (setID === 1094) return 'Bronze';
	return undefined;
}
