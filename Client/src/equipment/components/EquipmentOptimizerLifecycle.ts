import type { EquipmentInstanceV2, GameStateV2, GemInstanceV2 } from '../../api/Contracts';
import type { EquipmentPriorityGroup, EquipmentPriorityProfile } from './EquipmentOptimizerState';
import type { EquipmentLeader } from './EquipmentTypes';

export function equipmentPriorityCatalogKey(groups: readonly EquipmentPriorityGroup[]): string {
	return groups.map((group) => `${group.key}:${group.effectIDs.join(',')}`).join('|');
}

export function equipmentPriorityProfileKey(profile: EquipmentPriorityProfile): string {
	return `1:${profile.tier1.join(',')}|2:${profile.tier2.join(',')}`;
}

export function equipmentOptimizerScopeKey(
	prioritySection: string | null,
	groups: readonly EquipmentPriorityGroup[],
): string {
	return `${prioritySection ?? 'none'}|${equipmentPriorityCatalogKey(groups)}`;
}

export function equipmentOptimizerSnapshotKey(
	state: GameStateV2 | null,
	leader: Pick<EquipmentLeader, 'kind' | 'id'> | null,
	combatMode: 'pvp' | 'pve',
): string {
	if (!state || !leader) return 'unavailable';
	const source = leader.kind === 'commander'
		? state.commanders[String(leader.id)]
		: state.castellans[String(leader.id)];
	if (!source) return 'missing';
	const expectedType = leader.kind === 'commander' ? 2 : 1;
	const equipment = Object.values(state.inventory.equipment)
		.filter((item) => item.typeId === expectedType && optimizerSlot(item.slot))
		.filter((item) => !item.wearerKind || item.wearerKind === leader.kind && item.wearerId === leader.id)
		.sort((left, right) => left.id - right.id)
		.map(equipmentSnapshot);
	const gems = Object.values(state.inventory.gems)
		.filter((gem) => gemEligible(state, gem, leader.kind, leader.id, expectedType))
		.filter((gem) => gemMatchesMode(gem, leader.kind, combatMode))
		.sort((left, right) => left.id - right.id)
		.map(gemSnapshot);
	return JSON.stringify({
		account: [state.account.worldId ?? state.session.serverUrl ?? '', state.account.playerId ?? state.player.id],
		session: [state.session.generation, state.session.connectionGeneration],
		leader: [leader.kind, leader.id, 'available' in source ? source.available : true, orderedSlots(source.equipment, [1, 2, 3, 4, 6]), orderedSlots(source.gems, [1, 2, 3, 4])],
		combatMode,
		equipment,
		gems,
	});
}

function equipmentSnapshot(item: EquipmentInstanceV2) {
	return [item.id, item.definitionId, item.slot, item.typeId ?? 0, item.rarityId ?? 0, item.relic ?? false,
		item.relicKnown ?? false, item.setId ?? 0, item.level ?? 0, item.wearerKind ?? '', item.wearerId ?? 0,
		effectSnapshot(item.effects)];
}

function gemSnapshot(gem: GemInstanceV2) {
	return [gem.id, gem.definitionId, gem.typeId ?? 0, gem.compatibleWearerId ?? 0, gem.combatMode ?? '',
		gem.setId ?? 0, gem.slot ?? 0, gem.level ?? 0, gem.equipmentInstanceId ?? 0,
		gem.wearerKind ?? '', gem.wearerId ?? 0, effectSnapshot(gem.effects)];
}

function effectSnapshot(effects: EquipmentInstanceV2['effects']) {
	return [...effects]
		.sort((left, right) => left.definitionId - right.definitionId || left.wireId - right.wireId)
		.map((effect) => [effect.definitionId, effect.wireId, effect.rollPercent ?? null, effect.values]);
}

function orderedSlots(source: Record<string, number>, slots: readonly number[]) {
	return slots.map((slot) => source[String(slot)] ?? 0);
}

function optimizerSlot(slot: number): boolean {
	return slot >= 1 && slot <= 4 || slot === 6;
}

function gemEligible(
	state: GameStateV2,
	gem: GemInstanceV2,
	kind: EquipmentLeader['kind'],
	leaderID: number,
	expectedType: number,
): boolean {
	if (gem.wearerKind && (gem.wearerKind !== kind || gem.wearerId !== leaderID)) return false;
	if (!gem.equipmentInstanceId) return true;
	const carrier = state.inventory.equipment[String(gem.equipmentInstanceId)];
	if (!carrier || carrier.typeId !== expectedType || carrier.slot < 1 || carrier.slot > 4) return false;
	return !carrier.wearerKind || carrier.wearerKind === kind && carrier.wearerId === leaderID;
}

function gemMatchesMode(gem: GemInstanceV2, kind: EquipmentLeader['kind'], combatMode: 'pvp' | 'pve'): boolean {
	const expectedWearerID = kind === 'castellan' ? 1 : 2;
	if ((gem.compatibleWearerId ?? 0) > 0 && gem.compatibleWearerId !== expectedWearerID) return false;
	if (gem.combatMode === 'pvp' || gem.combatMode === 'pve') return gem.combatMode === combatMode;
	if (!gem.effects.length) return false;
	const wireID = gem.effects[0]?.wireId ?? 0;
	if (kind === 'castellan') return combatMode === 'pvp' ? wireID >= 10300 && wireID < 10400 : wireID >= 10200 && wireID < 10300;
	return combatMode === 'pvp' ? wireID >= 300 && wireID < 400 : wireID >= 200 && wireID < 300;
}
