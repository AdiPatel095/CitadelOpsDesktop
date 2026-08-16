export type OfficialEquipmentCombatMode = 'PvP' | 'PvE';

export type OfficialEquipmentEffectScope = 'Always' | OfficialEquipmentCombatMode;

export interface OfficialEquipmentEffectApplicabilityMetadata {
	name?: unknown;
	internalName?: unknown;
	effectTypeName?: unknown;
	areaTypeID?: unknown;
	areaTypeIds?: unknown;
	isPvPFight?: unknown;
	isPvEFight?: unknown;
}

export interface OfficialEquipmentTargetIndex {
	pvpAreaScores: ReadonlyMap<number, number>;
	pveAreaScores: ReadonlyMap<number, number>;
}

export function buildOfficialEquipmentTargetIndex(
	definitions: Record<number, OfficialEquipmentEffectApplicabilityMetadata>,
): OfficialEquipmentTargetIndex {
	const pvpAreaScores = new Map<number, number>();
	const pveAreaScores = new Map<number, number>();
	for (const definition of Object.values(definitions)) {
		const scope = officialEquipmentEffectScope(definition);
		if (scope === 'Always') continue;
		const scores = scope === 'PvP' ? pvpAreaScores : pveAreaScores;
		for (const areaTypeID of officialEquipmentEffectAreaTypeIDs(definition)) {
			scores.set(areaTypeID, (scores.get(areaTypeID) ?? 0) + 1);
		}
	}
	return { pvpAreaScores, pveAreaScores };
}

export function officialEquipmentEffectAreaTypeIDs(
	definition: OfficialEquipmentEffectApplicabilityMetadata | undefined,
): number[] {
	const source = definition?.areaTypeIds ?? definition?.areaTypeID;
	const values = Array.isArray(source) ? source : typeof source === 'string' ? source.split(/[#,]/) : [source];
	return Array.from(new Set(
		values
			.map((value) => Number(value))
			.filter((value) => Number.isSafeInteger(value) && value > 0),
	)).sort((left, right) => left - right);
}

export function officialEquipmentEffectStructuredScope(
	definition: OfficialEquipmentEffectApplicabilityMetadata | undefined,
): OfficialEquipmentCombatMode | null {
	const pvp = officialOptionalBoolean(definition?.isPvPFight);
	if (pvp != null) return pvp ? 'PvP' : 'PvE';
	const pve = officialOptionalBoolean(definition?.isPvEFight);
	if (pve != null) return pve ? 'PvE' : 'PvP';
	return null;
}

export function officialEquipmentEffectScope(
	definition: OfficialEquipmentEffectApplicabilityMetadata | undefined,
): OfficialEquipmentEffectScope {
	const structured = officialEquipmentEffectStructuredScope(definition);
	if (structured) return structured;

	// A few legacy official rows predate the structured fight flags. Their
	// official identifiers are the only catalog-provided PvP/PvE discriminator.
	const identifier = [definition?.internalName, definition?.effectTypeName, definition?.name]
		.filter((value): value is string => typeof value === 'string' && value.trim() !== '')
		.join(' ')
		.toLowerCase();
	const namesPvP = identifier.includes('pvp') || identifier.includes('castlelord');
	const namesPvE = identifier.includes('pve') || identifier.includes('npc');
	if (namesPvP !== namesPvE) return namesPvP ? 'PvP' : 'PvE';
	return 'Always';
}

export function officialEquipmentEffectMatchesCombatMode(
	definition: OfficialEquipmentEffectApplicabilityMetadata | undefined,
	combatMode: OfficialEquipmentCombatMode,
	includeIdentifierFallback = true,
): boolean {
	const structured = officialEquipmentEffectStructuredScope(definition);
	if (structured) return structured === combatMode;
	if (!includeIdentifierFallback) return true;
	const scope = officialEquipmentEffectScope(definition);
	return scope === 'Always' || scope === combatMode;
}

export function officialEquipmentAreaSupportsCombatMode(
	areaTypeID: number,
	combatMode: OfficialEquipmentCombatMode,
	index: OfficialEquipmentTargetIndex,
): boolean {
	if (!Number.isSafeInteger(areaTypeID) || areaTypeID <= 0) return false;
	const scores = combatMode === 'PvP' ? index.pvpAreaScores : index.pveAreaScores;
	return (scores.get(areaTypeID) ?? 0) > 0;
}

export function officialEquipmentTargetCombatMode(
	areaTypeID: number,
	index: OfficialEquipmentTargetIndex,
): OfficialEquipmentCombatMode | null {
	return officialEquipmentTargetCombatModeForAreas([areaTypeID], index);
}

export function officialEquipmentTargetCombatModeForAreas(
	areaTypeIDs: readonly number[],
	index: OfficialEquipmentTargetIndex,
): OfficialEquipmentCombatMode | null {
	let pvp = 0;
	let pve = 0;
	for (const areaTypeID of new Set(areaTypeIDs)) {
		pvp += index.pvpAreaScores.get(areaTypeID) ?? 0;
		pve += index.pveAreaScores.get(areaTypeID) ?? 0;
	}
	if (pvp === pve) return null;
	return pvp > pve ? 'PvP' : 'PvE';
}

export function officialEquipmentEffectAppliesToArea(
	definition: OfficialEquipmentEffectApplicabilityMetadata | undefined,
	targetAreaTypeID: number,
	index: OfficialEquipmentTargetIndex,
	targetCombatMode: OfficialEquipmentCombatMode | null = null,
): boolean {
	if (!definition || !Number.isSafeInteger(targetAreaTypeID) || targetAreaTypeID <= 0) return false;
	const areaTypeIDs = officialEquipmentEffectAreaTypeIDs(definition);
	if (areaTypeIDs.length > 0 && !areaTypeIDs.includes(targetAreaTypeID)) return false;

	const structuredScope = officialEquipmentEffectStructuredScope(definition);
	if (structuredScope) {
		return targetCombatMode
			? structuredScope === targetCombatMode
			: officialEquipmentAreaSupportsCombatMode(targetAreaTypeID, structuredScope, index);
	}
	// The official area list is authoritative. Identifier fallback is only needed
	// for legacy rows that publish no area or structured fight restriction.
	if (areaTypeIDs.length > 0) return true;
	const scope = officialEquipmentEffectScope(definition);
	if (scope === 'Always') return true;
	return targetCombatMode
		? scope === targetCombatMode
		: officialEquipmentAreaSupportsCombatMode(targetAreaTypeID, scope, index);
}

function officialOptionalBoolean(value: unknown): boolean | null {
	if (value === true || value === 1) return true;
	if (value === false || value === 0) return false;
	if (typeof value !== 'string') return null;
	const normalized = value.trim().toLowerCase();
	if (normalized === '1' || normalized === 'true') return true;
	if (normalized === '0' || normalized === 'false') return false;
	return null;
}
