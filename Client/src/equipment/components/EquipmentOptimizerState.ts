import {
	equipmentTargetLabel,
	equipmentTargets,
	type CombatMode,
	type EquipmentLeader,
} from './EquipmentTypes';
import {
	buildOfficialEquipmentTargetIndex,
	officialEquipmentAreaSupportsCombatMode,
	officialEquipmentEffectAreaTypeIDs,
	officialEquipmentEffectMatchesCombatMode,
	officialEquipmentEffectScope,
	officialEquipmentTargetCombatModeForAreas,
	type OfficialEquipmentTargetIndex,
} from '../EquipmentEffectApplicability';
import {
	officialEquipmentEffectGroupIdentity,
	type OfficialEquipmentEffectGroupingMetadata,
} from './EquipmentEffects';

const priorityCachePrefix = 'citadel.equipment.optimizer.';
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

export interface EquipmentEffectMetadata extends OfficialEquipmentEffectGroupingMetadata {
	scope?: unknown;
	isPvPFight?: unknown;
	isPvEFight?: unknown;
	effectTemplate?: unknown;
	areaTypeID?: unknown;
	areaTypeIds?: unknown;
}

interface StoredEquipmentPriorityProfileV4 extends EquipmentPriorityProfile {
	version: 4;
}

interface TargetProfileSeed {
	id: string;
	label: string;
	description: string;
	areaTypeIDs: number[];
}

interface TargetSignature {
	areaTypeIDs: number[];
	effectIDs: number[];
	groupKeys: Set<string>;
}

const knownEventProfileSeeds: TargetProfileSeed[] = [
	{
		id: 'glory-invasion',
		label: 'Glory Invasion',
		description: 'PvP loadouts for Foreign Lords and Bloodcrows, including effects scoped specifically to Glory invasion castles.',
		areaTypeIDs: [21, 34],
	},
	{
		id: 'nomad-khan',
		label: 'Nomad & Khan',
		description: 'PvE loadouts for Nomad camps and the Khan, including their official event-only combat and loot effects.',
		areaTypeIDs: [27, 35],
	},
	{
		id: 'samurai-daimyo',
		label: 'Samurai & Daimyo',
		description: 'PvE loadouts for Samurai camps and Daimyo targets, including effects restricted to that event family.',
		areaTypeIDs: [29, 37],
	},
	{
		id: 'berimond',
		label: 'Berimond',
		description: 'Event loadouts for Berimond kingdom and invasion targets, including faction-point and Berimond-only effects.',
		areaTypeIDs: [15, 16, 17, 18, 30],
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
	effects: Record<number, EquipmentEffectMetadata>,
): string[] {
	if (!leader || !Number.isSafeInteger(playerID) || (playerID ?? -1) < 0) return [];
	const targetIndex = buildOfficialEquipmentTargetIndex(effects);
	const areaTypeIDs = target.kind === 'event'
		? target.areaTypeIDs
		: equipmentTargets()
			.filter((candidate) => officialEquipmentAreaSupportsCombatMode(
				candidate.castleTypeID,
				target.combatMode,
				targetIndex,
			))
			.map((candidate) => candidate.castleTypeID);
	return areaTypeIDs.map((areaTypeID) => (
		`equipment.optimizerPriorities.v1.${playerID}.${leader.kind}.${leader.id}.castle-${areaTypeID}`
	));
}

export function equipmentTargetProfiles(
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentTargetProfile[] {
	const targetIndex = buildOfficialEquipmentTargetIndex(effects);
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
			officialGroupCount: countProfileGroups(effects, 'PvP', targetIndex),
			discovered: false,
		},
		{
			id: 'pve',
			label: 'PvE',
			description: 'One broadly applicable profile for towers, Storm targets, forts, and other non-player targets.',
			combatMode: 'PvE',
			areaTypeIDs: [],
			kind: 'base',
			officialGroupCount: countProfileGroups(effects, 'PvE', targetIndex),
			discovered: false,
		},
	];
	const knownProfiles = knownEventProfileSeeds.flatMap((seed): EquipmentTargetProfile[] => {
		const combatMode = officialEquipmentTargetCombatModeForAreas(seed.areaTypeIDs, targetIndex);
		if (!combatMode) return [];
		return [{
			id: seed.id,
			label: seed.label,
			description: seed.description,
			combatMode,
			areaTypeIDs: [...seed.areaTypeIDs],
			kind: 'event',
			officialGroupCount: signatureByKey.get(areaSignature(seed.areaTypeIDs))?.groupKeys.size ?? 0,
			discovered: false,
		}];
	});
	const knownSignatures = new Set(knownEventProfileSeeds.map((seed) => areaSignature(seed.areaTypeIDs)));
	const discoveredProfiles = signatures
		.filter((signature) => (
			signature.areaTypeIDs.length > 0
			&& signature.areaTypeIDs.length <= 5
			&& signature.groupKeys.size >= minimumDiscoveredTargetGroups
			&& !knownSignatures.has(areaSignature(signature.areaTypeIDs))
		))
		.flatMap((signature): EquipmentTargetProfile[] => {
			const combatMode = officialEquipmentTargetCombatModeForAreas(signature.areaTypeIDs, targetIndex);
			if (!combatMode) return [];
			const label = discoveredTargetLabel(signature.areaTypeIDs);
			return [{
				id: `official-${signature.areaTypeIDs.join('-')}`,
				label,
				description: `Official equipment defines a complete target-specific effect family for ${targetListLabel(signature.areaTypeIDs)}.`,
				combatMode,
				areaTypeIDs: [...signature.areaTypeIDs],
				kind: 'event',
				officialGroupCount: signature.groupKeys.size,
				discovered: true,
			}];
		})
		.sort((left, right) => left.label.localeCompare(right.label) || left.id.localeCompare(right.id));
	return [...baseProfiles, ...knownProfiles, ...discoveredProfiles];
}

export function targetProfileEffectIDs(
	effectIDs: readonly number[],
	effects: Record<number, EquipmentEffectMetadata>,
	target: EquipmentTargetProfile,
): number[] {
	const targetIndex = buildOfficialEquipmentTargetIndex(effects);
	return Array.from(new Set(effectIDs))
		.filter((id) => Number.isSafeInteger(id) && id > 0 && effectMatchesTargetProfile(effects[id], target, targetIndex))
		.sort((left, right) => left - right);
}

export function groupEquipmentPriorityEffects(
	candidateEffectIDs: readonly number[],
	effects: Record<number, EquipmentEffectMetadata>,
): EquipmentPriorityGroup[] {
	const groups = new Map<string, EquipmentPriorityGroup>();
	for (const id of candidateEffectIDs) {
		const effect = effects[id];
		const definition = officialPriorityGroup(id, effect);
		const current = groups.get(definition.key);
		if (!current) {
			groups.set(definition.key, { ...definition, effectIDs: [id] });
			continue;
		}
		current.effectIDs.push(id);
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
	const combatStrengthGroups = groups.filter((group) => (
		/combat strength/i.test(group.label) && /melee|range|ranged/i.test(group.label)
	));
	let tier2 = combatStrengthGroups
		.filter((group) => leaderKind === 'castellan' ? /defensive/i.test(group.label) : !/defensive/i.test(group.label))
		.slice(0, 2)
		.map((group) => group.key);
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
	if (raw.version === 4) {
		return normalizeEquipmentPriorityProfile({ tier1: raw.tier1, tier2: raw.tier2 }, groups);
	}
	if (raw.version === 3) {
		return normalizeEquipmentPriorityProfile({
			tier1: migrateEffectTypeTier(raw.tier1, groups, effects),
			tier2: migrateEffectTypeTier(raw.tier2, groups, effects),
		}, groups);
	}
	if (raw.version === 2) {
		return normalizeEquipmentPriorityProfile({
			tier1: migrateOfficialGroupTier(raw.tier1, groups),
			tier2: migrateOfficialGroupTier(raw.tier2, groups),
		}, groups);
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

export function storedEquipmentPriorityProfile(profile: EquipmentPriorityProfile): StoredEquipmentPriorityProfileV4 {
	return { version: 4, tier1: [...profile.tier1], tier2: [...profile.tier2] };
}

function migrateOfficialGroupTier(
	source: unknown,
	groups: readonly EquipmentPriorityGroup[],
): string[] {
	if (!Array.isArray(source)) return [];
	const currentKeys = new Set(groups.map((group) => group.key));
	const migrated: string[] = [];
	for (const value of source) {
		if (typeof value !== 'string') continue;
		if (currentKeys.has(value)) {
			migrated.push(value);
			continue;
		}
		const match = /^official-group-(\d+)-(\d+)$/.exec(value);
		if (!match) continue;
		const category = Number(match[1]);
		const groupID = Number(match[2]);
		for (const group of groups) {
			if (group.category === category && group.group === groupID) migrated.push(group.key);
		}
	}
	return Array.from(new Set(migrated));
}

function migrateEffectTypeTier(
	source: unknown,
	groups: readonly EquipmentPriorityGroup[],
	effects: Record<number, EquipmentEffectMetadata>,
): string[] {
	if (!Array.isArray(source)) return [];
	const currentKeys = new Set(groups.map((group) => group.key));
	const migrated: string[] = [];
	for (const value of source) {
		if (typeof value !== 'string') continue;
		if (currentKeys.has(value)) {
			migrated.push(value);
			continue;
		}
		const match = /^effect-type-(\d+)(?::argument-\d+)?$/.exec(value);
		if (!match) continue;
		const effectTypeID = Number(match[1]);
		for (const group of groups) {
			if (group.effectIDs.some((id) => (
				officialEquipmentEffectGroupIdentity(id, effects[id]).effectTypeId === effectTypeID
			))) migrated.push(group.key);
		}
	}
	return Array.from(new Set(migrated));
}

function targetSignatures(effects: Record<number, EquipmentEffectMetadata>): TargetSignature[] {
	const signatures = new Map<string, TargetSignature>();
	for (const [rawID, effect] of Object.entries(effects)) {
		const id = Number(rawID);
		if (!Number.isSafeInteger(id) || id <= 0) continue;
		const areaTypeIDs = officialEquipmentEffectAreaTypeIDs(effect);
		const groupKey = officialPriorityGroup(id, effect).key;
		if (areaTypeIDs.length === 0) continue;
		const key = areaSignature(areaTypeIDs);
		const signature = signatures.get(key) ?? {
			areaTypeIDs,
			effectIDs: [],
			groupKeys: new Set<string>(),
		};
		signature.effectIDs.push(id);
		signature.groupKeys.add(groupKey);
		signatures.set(key, signature);
	}
	return [...signatures.values()];
}

function countProfileGroups(
	effects: Record<number, EquipmentEffectMetadata>,
	combatMode: CombatMode,
	targetIndex: OfficialEquipmentTargetIndex,
): number {
	const ids = Object.keys(effects).map(Number);
	return groupEquipmentPriorityEffects(
		ids.filter((id) => effectMatchesBaseProfile(effects[id], combatMode, targetIndex)),
		effects,
	).length;
}

function effectMatchesTargetProfile(
	effect: EquipmentEffectMetadata | undefined,
	target: EquipmentTargetProfile,
	targetIndex: OfficialEquipmentTargetIndex,
): boolean {
	if (!effect) return false;
	if (target.kind === 'base') return effectMatchesBaseProfile(effect, target.combatMode, targetIndex);
	const areas = officialEquipmentEffectAreaTypeIDs(effect);
	const targetAreas = new Set(target.areaTypeIDs);
	if (areas.length > 0 && !areas.some((areaTypeID) => targetAreas.has(areaTypeID))) return false;
	return officialEquipmentEffectMatchesCombatMode(effect, target.combatMode, areas.length === 0);
}

function effectMatchesBaseProfile(
	effect: EquipmentEffectMetadata,
	combatMode: CombatMode,
	targetIndex: OfficialEquipmentTargetIndex,
): boolean {
	const areas = officialEquipmentEffectAreaTypeIDs(effect);
	if (areas.length === 0) {
		return officialEquipmentEffectMatchesCombatMode(effect, combatMode);
	}
	// A short official area list represents a target-specific modifier. Those
	// effects belong on the matching event card, not the broad PvP/PvE profile.
	if (areas.length <= 5) return false;
	const scope = officialEquipmentEffectScope(effect);
	if (scope !== 'Always') return scope === combatMode;
	return areas.some((areaTypeID) => officialEquipmentAreaSupportsCombatMode(areaTypeID, combatMode, targetIndex));
}

function officialPriorityGroup(
	id: number,
	effect: EquipmentEffectMetadata | undefined,
): Omit<EquipmentPriorityGroup, 'effectIDs'> {
	const identity = officialEquipmentEffectGroupIdentity(id, effect);
	return {
		key: identity.key,
		label: identity.label,
		category: identity.category,
		categoryLabel: identity.categoryLabel,
		group: identity.group,
	};
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

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value != null && !Array.isArray(value);
}
