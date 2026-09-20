import type { EquipmentExtractionCostV2, EquipmentInstanceV2, EquipmentLoadoutV2, GameStateV2, GemInstanceV2 } from '../../api/Contracts';
import type { EquipmentPriorityGroup, EquipmentPriorityProfile } from './EquipmentOptimizerState';
import type { EquipmentLeader } from './EquipmentTypes';

export function equipmentPriorityCatalogKey(groups: readonly EquipmentPriorityGroup[]): string {
	return groups.map((group) => `${group.key}:${group.effectIDs.join(',')}`).join('|');
}

export function equipmentPriorityProfileKey(profile: EquipmentPriorityProfile): string {
	return `1:${profile.tier1.join(',')}|2:${profile.tier2.join(',')}`;
}

export function equipmentOptimizerInitializationChange(
	previousSection: string | null | undefined,
	nextSection: string | null,
	previousCatalogKey: string,
	nextCatalogKey: string,
	previousProfileKey: string,
	nextProfileKey: string,
): 'unchanged' | 'retain-preview' | 'invalidate' {
	if (previousSection === undefined) return 'invalidate';
	if (previousSection !== nextSection) return 'invalidate';
	if (previousCatalogKey !== nextCatalogKey) return 'retain-preview';
	return previousProfileKey === nextProfileKey ? 'unchanged' : 'invalidate';
}

export function equipmentSharedCapLabel(caps: readonly number[]): string {
	if (caps.length === 0) return '';
	return caps.map((cap) => `max ${Number.isInteger(cap) ? cap.toLocaleString() : cap.toLocaleString(undefined, { maximumFractionDigits: 1 })}`).join(' · ');
}

export function equipmentAlternativeApplyDisabled(
	current: Pick<EquipmentLoadoutV2, 'equipment' | 'gems'>,
	selected: Pick<EquipmentLoadoutV2, 'equipment' | 'gems' | 'useful'>,
	noUsefulChange = false,
): boolean {
	return selected.useful === false
		|| equipmentAssignmentKey(selected) === equipmentAssignmentKey(current)
		|| noUsefulChange && selected.useful !== true;
}

function equipmentAssignmentKey(loadout: Pick<EquipmentLoadoutV2, 'equipment' | 'gems'>): string {
	return [1, 2, 3, 4, 6].map((slot) => `e${slot}:${loadout.equipment[String(slot)] ?? 0}`).join('|')
		+ [1, 2, 3, 4].map((slot) => `|g${slot}:${loadout.gems[String(slot)] ?? 0}`).join('');
}

export function equipmentOptimizerEffectSummary(
	effects: readonly { definitionId: number; values: number[] }[] | undefined,
	getEffectName: (id: number) => string,
): string {
	if (!effects?.length) return 'Effects unavailable';
	return effects.slice(0, 2).map((effect) => {
		const catalogName = getEffectName(effect.definitionId).trim();
		const unknown = !catalogName || /^Effect \d+$/i.test(catalogName);
		const name = unknown ? `Unknown effect (effect ${effect.definitionId})` : catalogName;
		const value = effect.values.at(-1);
		return value == null || !Number.isFinite(value) ? name : `${name} ${value > 0 ? '+' : ''}${value.toLocaleString(undefined, { maximumFractionDigits: 1 })}`;
	}).join(' · ');
}

export function equipmentExtractionApplyLabel(alternative: number, cost: EquipmentExtractionCostV2): string {
	return `Apply Alternative ${alternative} · up to ${cost.maximumRubySpend.toLocaleString()} rubies`;
}

export function equipmentExtractionCostNotices(cost: EquipmentExtractionCostV2): string[] {
	const notices = [`${cost.rubyExtractionCount.toLocaleString()} ordinary gem extraction${cost.rubyExtractionCount === 1 ? '' : 's'} · up to ${cost.maximumRubySpend.toLocaleString()} rubies`];
	if (cost.socketInsertionCount > 0) notices.push('Coin socketing costs also apply.');
	if (cost.relicExtractionCount > 0) notices.push('Relic gem extraction costs also apply in relic fragments.');
	return notices;
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
	const appearanceID = source.equipment['5'] ?? 0;
	const equipment = Object.values(state.inventory.equipment)
		.filter((item) => item.typeId === expectedType && (optimizerSlot(item.slot) || item.slot === 5 && item.id === appearanceID))
		.filter((item) => !item.wearerKind || item.wearerKind === leader.kind && item.wearerId === leader.id)
		.sort((left, right) => left.id - right.id)
		.map(equipmentSnapshot);
	const gems = Object.values(state.inventory.gems)
		.filter((gem) => gem.equipmentInstanceId === appearanceID && appearanceID !== 0
			|| gemEligible(state, gem, leader.kind, leader.id, expectedType)
				&& (Boolean(gem.equipmentInstanceId) || gemMatchesMode(gem, leader.kind, combatMode)))
		.sort((left, right) => left.id - right.id)
		.map(gemSnapshot);
	return JSON.stringify({
		account: [state.account.worldId ?? state.session.serverUrl ?? '', state.account.playerId ?? state.player.id],
		session: [state.session.generation, state.session.connectionGeneration],
		leader: [leader.kind, leader.id, 'available' in source ? source.available : true, orderedSlots(source.equipment, [1, 2, 3, 4, 5, 6]), orderedSlots(source.gems, [1, 2, 3, 4])],
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
