import type { EquipmentLeader } from './EquipmentTypes';

const priorityCachePrefix = 'citadel.equipment.optimizer.';

export interface EquipmentPriorityProfile {
	tier1: number[];
	tier2: number[];
}

export interface EquipmentPriorityGroup {
	key: string;
	label: string;
	effectIDs: number[];
}

type EffectMetadata = {
	internalName?: unknown;
	name?: unknown;
	scope?: unknown;
	effectTypeId?: unknown;
	effectTypeName?: unknown;
	areaTypeID?: unknown;
	areaTypeIds?: unknown;
};

interface StoredEquipmentPriorityProfile extends EquipmentPriorityProfile {
	version: 1;
}

export function equipmentPrioritySection(
	playerID: number | undefined,
	leader: EquipmentLeader | null,
	targetID: string,
): string | null {
	if (!leader || !Number.isSafeInteger(playerID) || (playerID ?? -1) < 0 || !/^[a-z0-9-]+$/i.test(targetID)) return null;
	return `equipment.optimizerPriorities.v1.${playerID}.${leader.kind}.${leader.id}.${targetID}`;
}

export function inferredEquipmentPriorityProfile(
	candidateEffectIDs: readonly number[],
	effects: Record<number, EffectMetadata>,
	leaderKind: EquipmentLeader['kind'] | undefined,
	castleTypeID: number,
): EquipmentPriorityProfile {
	const matchesRole = (id: number) => {
		const name = effectName(effects[id]);
		const offense = /(offen|attack)/.test(name);
		const defense = /(defen|defend)/.test(name);
		return /(melee|range|ranged)/.test(name) && (leaderKind === 'castellan' ? defense : offense);
	};
	const applicable = targetCandidateEffectIDs(candidateEffectIDs, effects, castleTypeID);
	const roleEffects = applicable.filter(matchesRole).sort((left, right) => {
		const leftAreas = effectAreaTypeIDs(effects[left]);
		const rightAreas = effectAreaTypeIDs(effects[right]);
		const leftSpecificity = leftAreas.length || Number.MAX_SAFE_INTEGER;
		const rightSpecificity = rightAreas.length || Number.MAX_SAFE_INTEGER;
		return leftSpecificity - rightSpecificity || left - right;
	});
	const tier1 = roleEffects.filter((id) => effectAreaTypeIDs(effects[id]).length > 0).slice(0, 4);
	const tier1IDs = new Set(tier1);
	let tier2 = roleEffects.filter((id) => !tier1IDs.has(id)).slice(0, 4);
	if (tier1.length === 0 && tier2.length === 0) tier2 = applicable.slice(0, 2);
	return normalizeEquipmentPriorityProfile({ tier1, tier2 }, applicable);
}

export function targetCandidateEffectIDs(
	candidateEffectIDs: readonly number[],
	effects: Record<number, EffectMetadata>,
	castleTypeID: number,
): number[] {
	return candidateEffectIDs.filter((id) => {
		const areaTypeIDs = effectAreaTypeIDs(effects[id]);
		return areaTypeIDs.length === 0 || areaTypeIDs.includes(castleTypeID);
	});
}

export function groupEquipmentPriorityEffects(
	candidateEffectIDs: readonly number[],
	effects: Record<number, EffectMetadata>,
): EquipmentPriorityGroup[] {
	const groups = new Map<string, EquipmentPriorityGroup>();
	for (const id of candidateEffectIDs) {
		const effect = effects[id];
		const group = effectPriorityGroup(id, effect);
		const current = groups.get(group.key) ?? { ...group, effectIDs: [] };
		current.effectIDs.push(id);
		groups.set(group.key, current);
	}
	return [...groups.values()]
		.map((group) => ({ ...group, effectIDs: group.effectIDs.sort((left, right) => left - right) }))
		.sort((left, right) => left.label.localeCompare(right.label) || left.key.localeCompare(right.key));
}

export function readEquipmentPriorityProfile(raw: unknown, candidateEffectIDs: readonly number[]): EquipmentPriorityProfile | null {
	if (!isRecord(raw) || raw.version !== 1) return null;
	return normalizeEquipmentPriorityProfile({ tier1: raw.tier1, tier2: raw.tier2 }, candidateEffectIDs);
}

export function readCachedEquipmentPriorityProfile(section: string | null, candidateEffectIDs: readonly number[]): EquipmentPriorityProfile | null {
	if (!section) return null;
	try {
		const raw = localStorage.getItem(priorityCachePrefix + section);
		return raw == null ? null : readEquipmentPriorityProfile(JSON.parse(raw), candidateEffectIDs);
	} catch {
		return null;
	}
}

export function cacheEquipmentPriorityProfile(section: string, profile: EquipmentPriorityProfile): void {
	try {
		localStorage.setItem(priorityCachePrefix + section, JSON.stringify(storedEquipmentPriorityProfile(profile)));
	} catch {
		// The server configuration remains the durable fallback when browser storage is unavailable.
	}
}

export function normalizeEquipmentPriorityProfile(raw: { tier1?: unknown; tier2?: unknown }, candidateEffectIDs: readonly number[]): EquipmentPriorityProfile {
	const candidates = new Set(candidateEffectIDs);
	const used = new Set<number>();
	const tier1 = normalizeTier(raw.tier1, candidates, used);
	const tier2 = normalizeTier(raw.tier2, candidates, used);
	return { tier1, tier2 };
}

export function storedEquipmentPriorityProfile(profile: EquipmentPriorityProfile): StoredEquipmentPriorityProfile {
	return { version: 1, tier1: [...profile.tier1], tier2: [...profile.tier2] };
}

function normalizeTier(source: unknown, candidates: Set<number>, used: Set<number>): number[] {
	if (!Array.isArray(source)) return [];
	const result: number[] = [];
	for (const value of source) {
		if (!Number.isSafeInteger(value) || !candidates.has(value) || used.has(value)) continue;
		used.add(value);
		result.push(value);
	}
	return result;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value != null && !Array.isArray(value);
}

function effectPriorityGroup(id: number, effect: EffectMetadata | undefined): Omit<EquipmentPriorityGroup, 'effectIDs'> {
	const name = effectName(effect);
	const offense = /(offen|attack)/.test(name);
	const defense = /(defen|defend)/.test(name);
	const melee = /melee/.test(name);
	const ranged = /range|ranged/.test(name);
	if ((offense || defense) && (melee || ranged)) {
		const role = offense ? 'Offensive' : 'Defensive';
		const troopType = melee ? 'Melee' : 'Ranged';
		return { key: `${role.toLowerCase()}-${troopType.toLowerCase()}-power`, label: `${role} ${troopType} Power` };
	}
	const effectTypeID = Number(effect?.effectTypeId);
	const effectTypeName = String(effect?.effectTypeName ?? '').trim();
	if (Number.isSafeInteger(effectTypeID) && effectTypeID > 0) {
		return { key: `effect-type-${effectTypeID}`, label: effectTypeName || `Effect Type ${effectTypeID}` };
	}
	return { key: `effect-${id}`, label: String(effect?.name ?? effect?.internalName ?? `Effect ${id}`).trim() || `Effect ${id}` };
}

function effectName(effect: EffectMetadata | undefined): string {
	return `${String(effect?.internalName ?? '')} ${String(effect?.name ?? '')}`.toLowerCase();
}

function effectAreaTypeIDs(effect: EffectMetadata | undefined): number[] {
	const source = effect?.areaTypeIds ?? effect?.areaTypeID;
	const values = Array.isArray(source) ? source : typeof source === 'string' ? source.split(',') : [source];
	return Array.from(new Set(values.map(Number).filter((value) => Number.isSafeInteger(value) && value > 0)));
}
