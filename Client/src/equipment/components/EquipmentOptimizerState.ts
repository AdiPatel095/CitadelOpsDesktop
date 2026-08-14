import {
	equipmentTargetLabel,
	equipmentTargets,
	targetCombatMode,
	type CombatMode,
	type EquipmentLeader,
} from './EquipmentTypes';

const priorityCachePrefix = 'citadel.equipment.optimizer.';
const officialGroupPrefix = 'official-group-';
// A narrow area signature must span a substantial official effect family
// before it earns its own card. This keeps one-off castle modifiers folded into
// PvP/PvE while allowing newly published complete families to appear without a
// target-specific client mapping.
const minimumDiscoveredTargetGroups = 13;

export interface EquipmentPriorityProfile {
	tier1: string[];
	tier2: string[];
}

export interface EquipmentPriorityGroup {
	key: string;
	label: string;
	category: number;
	categoryLabel: string;
	group: number;
	effectIDs: number[];
}

export interface EquipmentTargetProfile {
	id: string;
	label: string;
	description: string;
	combatMode: CombatMode;
	areaTypeIDs: number[];
	kind: 'base' | 'event';
	officialGroupCount: number;
	discovered: boolean;
}

export type EquipmentEffectMetadata = {
	internalName?: unknown;
	name?: unknown;
	scope?: unknown;
	isPvPFight?: unknown;
	isPvEFight?: unknown;
	effectTypeId?: unknown;
	effectTypeName?: unknown;
	sortCategory?: unknown;
	sortGroup?: unknown;
	categoryName?: unknown;
	effectGroupPassive?: unknown;
	effectGroupActive?: unknown;
	areaTypeID?: unknown;
	areaTypeIds?: unknown;
};

interface StoredEquipmentPriorityProfileV2 extends EquipmentPriorityProfile {
	version: 2;
}

interface TargetProfileSeed {
	id: string;
	label: string;
	description: string;
	combatMode: CombatMode;
	areaTypeIDs: number[];
	tokens: string[];
}

interface TargetSignature {
	areaTypeIDs: number[];
	effectIDs: number[];
	groupKeys: Set<string>;
	pvpSignals: number;
	pveSignals: number;
}

const knownEventProfileSeeds: TargetProfileSeed[] = [
	{
		id: 'glory-invasion',
		label: 'Glory Invasion',
		description: 'PvP loadouts for Foreign Lords and Bloodcrows, including effects scoped specifically to Glory invasion castles.',
		combatMode: 'PvP',
		areaTypeIDs: [21, 34],
		tokens: ['alien', 'bloodcrow', 'gloryinvasion', 'glory_invasion'],
	},
	{
		id: 'nomad-khan',
		label: 'Nomad & Khan',
		description: 'PvE loadouts for Nomad camps and the Khan, including their official event-only combat and loot effects.',
		combatMode: 'PvE',
		areaTypeIDs: [27, 35],
		tokens: ['nomad', 'khan'],
	},
	{
		id: 'samurai-daimyo',
		label: 'Samurai & Daimyo',
		description: 'PvE loadouts for Samurai camps and Daimyo targets, including effects restricted to that event family.',
		combatMode: 'PvE',
		areaTypeIDs: [29, 37],
		tokens: ['samurai', 'daimyo'],
	},
	{
		id: 'berimond',
		label: 'Berimond',
		description: 'Event loadouts for Berimond kingdom and invasion targets, including faction-point and Berimond-only effects.',
		combatMode: 'PvE',
		areaTypeIDs: [15, 16, 17, 18, 30],
		tokens: ['berimond'],
	},
];

export function equipmentPrioritySection(
	playerID: number | undefined,
	leader: EquipmentLeader | null,
	targetID: string,
): string | null {
	if (!leader || !Number.isSafeInteger(playerID) || (playerID ?? -1) < 0 || !/^[a-z0-9-]+$/i.test(targetID)) return null;
	return `equipment.optimizerPriorities.v2.${playerID}.${leader.kind}.${leader.id}.${targetID}`;
}

export function legacyEquipmentPrioritySections(
	playerID: number | undefined,
	leader: EquipmentLeader | null,
	target: EquipmentTargetProfile,
): string[] {
	if (!leader || !Number.isSafeInteger(playerID) || (playerID ?? -1) < 0) return [];
	const areaTypeIDs = target.kind === 'event'
		? target.areaTypeIDs
		: equipmentTargets()
			.filter((candidate) => targetCombatMode(candidate.castleTypeID) === target.combatMode)
			.map((candidate) => candidate.castleTypeID);
	return areaTypeIDs.map((areaTypeID) => (
		`equipment.optimizerPriorities.v1.${playerID}.${leader.kind}.${leader.id}.castle-${areaTypeID}`
	));
}

export function equipmentTargetProfiles(
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentTargetProfile[] {
	const signatures = targetSignatures(effects);
	const signatureByKey = new Map(signatures.map((signature) => [areaSignature(signature.areaTypeIDs), signature]));
	const baseProfiles: EquipmentTargetProfile[] = [
		{
			id: 'pvp',
			label: 'PvP',
			description: 'One broadly applicable profile for player castles, Foreign Lords, Bloodcrows, and other targets treated as PvP by the game.',
			combatMode: 'PvP',
			areaTypeIDs: [],
			kind: 'base',
			officialGroupCount: countProfileGroups(effects, 'PvP'),
			discovered: false,
		},
		{
			id: 'pve',
			label: 'PvE',
			description: 'One broadly applicable profile for towers, Storm targets, forts, and other non-player targets.',
			combatMode: 'PvE',
			areaTypeIDs: [],
			kind: 'base',
			officialGroupCount: countProfileGroups(effects, 'PvE'),
			discovered: false,
		},
	];
	const knownProfiles = knownEventProfileSeeds.map((seed): EquipmentTargetProfile => ({
		id: seed.id,
		label: seed.label,
		description: seed.description,
		combatMode: seed.combatMode,
		areaTypeIDs: [...seed.areaTypeIDs],
		kind: 'event',
		officialGroupCount: signatureByKey.get(areaSignature(seed.areaTypeIDs))?.groupKeys.size ?? 0,
		discovered: false,
	}));
	const knownSignatures = new Set(knownEventProfileSeeds.map((seed) => areaSignature(seed.areaTypeIDs)));
	const discoveredProfiles = signatures
		.filter((signature) => (
			signature.areaTypeIDs.length > 0
			&& signature.areaTypeIDs.length <= 5
			&& signature.groupKeys.size >= minimumDiscoveredTargetGroups
			&& !knownSignatures.has(areaSignature(signature.areaTypeIDs))
		))
		.map((signature): EquipmentTargetProfile => {
			const label = discoveredTargetLabel(signature.areaTypeIDs);
			return {
				id: `official-${signature.areaTypeIDs.join('-')}`,
				label,
				description: `Official equipment defines a complete target-specific effect family for ${targetListLabel(signature.areaTypeIDs)}.`,
				combatMode: inferredSignatureCombatMode(signature),
				areaTypeIDs: [...signature.areaTypeIDs],
				kind: 'event',
				officialGroupCount: signature.groupKeys.size,
				discovered: true,
			};
		})
		.sort((left, right) => left.label.localeCompare(right.label) || left.id.localeCompare(right.id));
	return [...baseProfiles, ...knownProfiles, ...discoveredProfiles];
}

export function targetProfileEffectIDs(
	effectIDs: readonly number[],
	effects: Record<number, EquipmentEffectMetadata>,
	target: EquipmentTargetProfile,
): number[] {
	return Array.from(new Set(effectIDs))
		.filter((id) => Number.isSafeInteger(id) && id > 0 && effectMatchesTargetProfile(effects[id], target))
		.sort((left, right) => left - right);
}

export function groupEquipmentPriorityEffects(
	candidateEffectIDs: readonly number[],
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentPriorityGroup[] {
	const groups = new Map<string, EquipmentPriorityGroup & { labelRank: number }>();
	for (const id of candidateEffectIDs) {
		const effect = effects[id];
		const definition = officialPriorityGroup(id, effect);
		const current = groups.get(definition.key);
		if (!current) {
			groups.set(definition.key, { ...definition, effectIDs: [id] });
			continue;
		}
		current.effectIDs.push(id);
		if (definition.labelRank > current.labelRank) {
			current.label = definition.label;
			current.categoryLabel = definition.categoryLabel;
			current.labelRank = definition.labelRank;
		}
	}
	return [...groups.values()]
		.map((group) => ({
			key: group.key,
			label: group.label,
			category: group.category,
			categoryLabel: group.categoryLabel,
			group: group.group,
			effectIDs: Array.from(new Set(group.effectIDs)).sort((left, right) => left - right),
		}))
		.sort((left, right) => (
			left.category - right.category
			|| left.group - right.group
			|| left.label.localeCompare(right.label)
			|| left.key.localeCompare(right.key)
		));
}

export function inferredEquipmentPriorityProfile(
	groups: readonly EquipmentPriorityGroup[],
	leaderKind: EquipmentLeader['kind'] | undefined,
): EquipmentPriorityProfile {
	const preferred = leaderKind === 'castellan'
		? [`${officialGroupPrefix}1-7`, `${officialGroupPrefix}1-8`]
		: [`${officialGroupPrefix}1-2`, `${officialGroupPrefix}1-3`];
	const available = new Set(groups.map((group) => group.key));
	let tier2 = preferred.filter((key) => available.has(key));
	if (tier2.length === 0) {
		tier2 = groups
			.filter((group) => /melee|range|ranged/i.test(group.label))
			.slice(0, 2)
			.map((group) => group.key);
	}
	if (tier2.length === 0) tier2 = groups.slice(0, 2).map((group) => group.key);
	return normalizeEquipmentPriorityProfile({ tier1: [], tier2 }, groups);
}

export function readEquipmentPriorityProfile(
	raw: unknown,
	groups: readonly EquipmentPriorityGroup[],
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentPriorityProfile | null {
	if (!isRecord(raw)) return null;
	if (raw.version === 2) {
		return normalizeEquipmentPriorityProfile({ tier1: raw.tier1, tier2: raw.tier2 }, groups);
	}
	if (raw.version === 1) {
		return normalizeEquipmentPriorityProfile({
			tier1: legacyTierToGroups(raw.tier1, effects),
			tier2: legacyTierToGroups(raw.tier2, effects),
		}, groups);
	}
	return null;
}

export function readCachedEquipmentPriorityProfile(
	section: string | null,
	groups: readonly EquipmentPriorityGroup[],
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentPriorityProfile | null {
	if (!section) return null;
	try {
		const raw = localStorage.getItem(priorityCachePrefix + section);
		return raw == null ? null : readEquipmentPriorityProfile(JSON.parse(raw), groups, effects);
	} catch {
		return null;
	}
}

export function readFirstCachedEquipmentPriorityProfile(
	sections: readonly string[],
	groups: readonly EquipmentPriorityGroup[],
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentPriorityProfile | null {
	for (const section of sections) {
		const profile = readCachedEquipmentPriorityProfile(section, groups, effects);
		if (profile) return profile;
	}
	return null;
}

export function cacheEquipmentPriorityProfile(section: string, profile: EquipmentPriorityProfile): void {
	try {
		localStorage.setItem(priorityCachePrefix + section, JSON.stringify(storedEquipmentPriorityProfile(profile)));
	} catch {
		// The server configuration remains the durable fallback when browser storage is unavailable.
	}
}

export function normalizeEquipmentPriorityProfile(
	raw: { tier1?: unknown; tier2?: unknown },
	groups: readonly EquipmentPriorityGroup[],
): EquipmentPriorityProfile {
	const candidates = new Set(groups.map((group) => group.key));
	const used = new Set<string>();
	const tier1 = normalizeTier(raw.tier1, candidates, used);
	const tier2 = normalizeTier(raw.tier2, candidates, used);
	return { tier1, tier2 };
}

export function storedEquipmentPriorityProfile(profile: EquipmentPriorityProfile): StoredEquipmentPriorityProfileV2 {
	return { version: 2, tier1: [...profile.tier1], tier2: [...profile.tier2] };
}

function targetSignatures(effects: Record<number, EquipmentEffectMetadata>): TargetSignature[] {
	const signatures = new Map<string, TargetSignature>();
	for (const [rawID, effect] of Object.entries(effects)) {
		const id = Number(rawID);
		const areaTypeIDs = effectAreaTypeIDs(effect);
		const groupKey = officialGroupKey(effect);
		if (!Number.isSafeInteger(id) || id <= 0 || areaTypeIDs.length === 0 || !groupKey) continue;
		const key = areaSignature(areaTypeIDs);
		const signature = signatures.get(key) ?? {
			areaTypeIDs,
			effectIDs: [],
			groupKeys: new Set<string>(),
			pvpSignals: 0,
			pveSignals: 0,
		};
		signature.effectIDs.push(id);
		signature.groupKeys.add(groupKey);
		const mode = effectCombatMode(effect);
		if (mode === 'PvP') signature.pvpSignals += 1;
		if (mode === 'PvE') signature.pveSignals += 1;
		signatures.set(key, signature);
	}
	return [...signatures.values()];
}

function countProfileGroups(effects: Record<number, EquipmentEffectMetadata>, combatMode: CombatMode): number {
	const ids = Object.keys(effects).map(Number);
	return groupEquipmentPriorityEffects(
		ids.filter((id) => effectMatchesBaseProfile(effects[id], combatMode)),
		effects,
	).length;
}

function effectMatchesTargetProfile(
	effect: EquipmentEffectMetadata | undefined,
	target: EquipmentTargetProfile,
): boolean {
	if (!effect) return false;
	if (target.kind === 'base') return effectMatchesBaseProfile(effect, target.combatMode);
	const areas = effectAreaTypeIDs(effect);
	const targetAreas = new Set(target.areaTypeIDs);
	if (areas.length > 0 && areas.every((areaTypeID) => targetAreas.has(areaTypeID))) return true;
	const namedProfile = namedEventProfileID(effect);
	if (areas.length === 0 && namedProfile) return namedProfile === target.id;
	return effectMatchesBaseProfile(effect, target.combatMode);
}

function effectMatchesBaseProfile(effect: EquipmentEffectMetadata, combatMode: CombatMode): boolean {
	const areas = effectAreaTypeIDs(effect);
	if (areas.length === 0) {
		if (namedEventProfileID(effect)) return false;
		const mode = effectCombatMode(effect);
		return mode == null || mode === combatMode;
	}
	// A short official area list represents a target-specific modifier. Those
	// effects belong on the matching event card, not the broad PvP/PvE profile.
	if (areas.length <= 5) return false;
	const mode = effectCombatMode(effect) ?? inferredAreaCombatMode(areas);
	return mode == null || mode === combatMode;
}

function namedEventProfileID(effect: EquipmentEffectMetadata): string | null {
	const name = `${String(effect.internalName ?? '')} ${String(effect.name ?? '')}`.toLowerCase();
	for (const seed of knownEventProfileSeeds) {
		if (seed.tokens.some((token) => name.includes(token))) return seed.id;
	}
	return null;
}

function officialPriorityGroup(
	id: number,
	effect: EquipmentEffectMetadata | undefined,
): Omit<EquipmentPriorityGroup, 'effectIDs'> & { labelRank: number } {
	const category = metadataInteger(effect?.sortCategory);
	const group = metadataInteger(effect?.sortGroup);
	const passiveLabel = cleanOfficialLabel(effect?.effectGroupPassive);
	const activeLabel = cleanOfficialLabel(effect?.effectGroupActive);
	const effectTypeLabel = cleanOfficialLabel(effect?.effectTypeName);
	const effectLabel = cleanOfficialLabel(effect?.name ?? effect?.internalName);
	const categoryLabel = cleanOfficialLabel(effect?.categoryName) || (category > 0 ? `Official category ${category}` : 'Other effects');
	if (category > 0 && group > 0) {
		return {
			key: `${officialGroupPrefix}${category}-${group}`,
			label: passiveLabel || activeLabel || effectTypeLabel || effectLabel || `Official effect group ${category}.${group}`,
			category,
			categoryLabel,
			group,
			labelRank: passiveLabel ? 4 : activeLabel ? 3 : effectTypeLabel ? 2 : effectLabel ? 1 : 0,
		};
	}
	const effectTypeID = metadataInteger(effect?.effectTypeId);
	if (effectTypeID > 0) {
		return {
			key: `effect-type-${effectTypeID}`,
			label: effectTypeLabel || effectLabel || `Effect type ${effectTypeID}`,
			category: category || 99,
			categoryLabel,
			group: group || effectTypeID,
			labelRank: effectTypeLabel ? 2 : effectLabel ? 1 : 0,
		};
	}
	return {
		key: `effect-${id}`,
		label: effectLabel || `Effect ${id}`,
		category: category || 99,
		categoryLabel,
		group: group || id,
		labelRank: effectLabel ? 1 : 0,
	};
}

function officialGroupKey(effect: EquipmentEffectMetadata | undefined): string | null {
	const category = metadataInteger(effect?.sortCategory);
	const group = metadataInteger(effect?.sortGroup);
	return category > 0 && group > 0 ? `${officialGroupPrefix}${category}-${group}` : null;
}

function legacyTierToGroups(source: unknown, effects: Record<number, EquipmentEffectMetadata>): string[] {
	if (!Array.isArray(source)) return [];
	const result: string[] = [];
	const used = new Set<string>();
	for (const value of source) {
		if (!Number.isSafeInteger(value)) continue;
		const key = officialPriorityGroup(Number(value), effects[Number(value)]).key;
		if (used.has(key)) continue;
		used.add(key);
		result.push(key);
	}
	return result;
}

function normalizeTier(source: unknown, candidates: Set<string>, used: Set<string>): string[] {
	if (!Array.isArray(source)) return [];
	const result: string[] = [];
	for (const value of source) {
		if (typeof value !== 'string' || !candidates.has(value) || used.has(value)) continue;
		used.add(value);
		result.push(value);
	}
	return result;
}

function inferredSignatureCombatMode(signature: TargetSignature): CombatMode {
	if (signature.pvpSignals !== signature.pveSignals) return signature.pvpSignals > signature.pveSignals ? 'PvP' : 'PvE';
	return inferredAreaCombatMode(signature.areaTypeIDs) ?? 'PvE';
}

function inferredAreaCombatMode(areaTypeIDs: readonly number[]): CombatMode | null {
	let pvp = 0;
	let pve = 0;
	for (const areaTypeID of areaTypeIDs) {
		if (targetCombatMode(areaTypeID) === 'PvP') pvp += 1;
		else pve += 1;
	}
	if (pvp === pve) return null;
	return pvp > pve ? 'PvP' : 'PvE';
}

function effectCombatMode(effect: EquipmentEffectMetadata | undefined): CombatMode | null {
	const scope = String(effect?.scope ?? '').trim().toLowerCase();
	if (scope === 'pvp' || officialFlag(effect?.isPvPFight)) return 'PvP';
	if (scope === 'pve' || officialFlag(effect?.isPvEFight)) return 'PvE';
	const name = `${String(effect?.internalName ?? '')} ${String(effect?.effectTypeName ?? '')}`.toLowerCase();
	if (/\bpvp\b|castlelord/.test(name)) return 'PvP';
	if (/\bpve\b|\bnpc\b/.test(name)) return 'PvE';
	return null;
}

function officialFlag(value: unknown): boolean {
	return value === true || value === 1 || String(value).trim() === '1';
}

function discoveredTargetLabel(areaTypeIDs: readonly number[]): string {
	if (areaTypeIDs.length === 1) return equipmentTargetLabel(areaTypeIDs[0]);
	return targetListLabel(areaTypeIDs);
}

function targetListLabel(areaTypeIDs: readonly number[]): string {
	const labels = areaTypeIDs.map(equipmentTargetLabel);
	if (labels.length <= 2) return labels.join(' & ');
	const commonWords = labels
		.map((label) => new Set(label.toLowerCase().split(/\s+/).filter((word) => word.length > 3)))
		.reduce<Set<string> | null>((common, words) => (
			common == null ? words : new Set([...common].filter((word) => words.has(word)))
		), null);
	const common = commonWords ? [...commonWords][0] : '';
	if (common) return common.charAt(0).toUpperCase() + common.slice(1);
	return `${labels[0]} + ${labels.length - 1} related targets`;
}

function areaSignature(areaTypeIDs: readonly number[]): string {
	return [...new Set(areaTypeIDs)].sort((left, right) => left - right).join(',');
}

function effectAreaTypeIDs(effect: EquipmentEffectMetadata | undefined): number[] {
	const source = effect?.areaTypeIds ?? effect?.areaTypeID;
	const values = Array.isArray(source) ? source : typeof source === 'string' ? source.split(',') : [source];
	return Array.from(new Set(values.map(Number).filter((value) => Number.isSafeInteger(value) && value > 0)))
		.sort((left, right) => left - right);
}

function metadataInteger(value: unknown): number {
	const parsed = Number(value);
	return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

function cleanOfficialLabel(value: unknown): string {
	const text = String(value ?? '')
		.replace(/\{\d+\}/g, '')
		.replace(/^[+\-\s%:]+/, '')
		.replace(/\s+/g, ' ')
		.trim();
	return text ? text.charAt(0).toUpperCase() + text.slice(1) : '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value != null && !Array.isArray(value);
}
