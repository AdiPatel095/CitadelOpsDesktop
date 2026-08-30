import type { EquipmentEffectV2 } from '../../api/Contracts';
import type { MetadataItem } from '../../context/MetadataContext';
import {
	buildOfficialEquipmentTargetIndex,
	officialEquipmentEffectAppliesToArea,
	officialEquipmentEffectScope,
	type OfficialEquipmentEffectScope,
} from '../EquipmentEffectApplicability';

export type EquipmentEffectScope = OfficialEquipmentEffectScope;

export interface EquipmentEffectSource {
	label: string;
	effects: EquipmentEffectV2[];
}

export interface MappedEquipmentEffect {
	key: string;
	definitionId: number;
	effectTypeId: number;
	label: string;
	template: string;
	value: number;
	rawValue: number;
	polarityValue: number;
	displayValue?: string;
	unit: 'percent' | 'number';
	scope: EquipmentEffectScope;
	areaTypeIds: number[];
	category: number;
	categoryName: string;
	group: number;
	groupKey: string;
	groupLabel: string;
	groupActiveTemplate: string;
	groupMalusTemplate: string;
	sortOrder: string;
	capId?: number;
	cap?: number;
	capped: boolean;
	sources: string[];
	argumentId?: number;
	argumentLabel?: string;
	commonCapTemplate: string;
}

export interface EquipmentEffectGroup {
	key: string;
	category: number;
	group: number;
	label: string;
	activeTemplate: string;
	malusTemplate: string;
	value: number;
	rawValue: number;
	polarityValue: number;
	displayValue?: string;
	unit: 'percent' | 'number';
	capped: boolean;
	rows: MappedEquipmentEffect[];
	commonCaps: Array<{ capId: number; max: number }>;
	commonCapTemplate: string;
}

export interface EquipmentEffectSection {
	key: string;
	title: string;
	description: string;
	effectCount: number;
	groups: EquipmentEffectGroup[];
}

export interface EquipmentEffectShowcase {
	key: string;
	label: string;
	value: number;
	rawValue: number;
	displayValue?: string;
	unit: 'percent' | 'number';
	category: number;
	group: number;
	capped: boolean;
}

export interface EquipmentEffectProfile {
	showcase: EquipmentEffectShowcase[];
	sections: EquipmentEffectSection[];
}

interface EffectContribution {
	definitionId: number;
	effectTypeId: number;
	label: string;
	template: string;
	value: number;
	polarityValue: number;
	displayValue?: string;
	unit: 'percent' | 'number';
	scope: EquipmentEffectScope;
	areaTypeIds: number[];
	category: number;
	categoryName: string;
	group: number;
	groupKey: string;
	groupLabel: string;
	groupActiveTemplate: string;
	groupMalusTemplate: string;
	sortOrder: string;
	capId: number;
	cap: number;
	source: string;
	argumentId?: number;
	argumentLabel?: string;
	commonCapTemplate: string;
}

interface EffectValue {
	value: number;
	argumentId?: number;
	argumentLabel?: string;
}

export interface OfficialEquipmentEffectGroupingMetadata {
	name?: unknown;
	internalName?: unknown;
	effectTypeId?: unknown;
	effectTypeName?: unknown;
	combatType?: unknown;
	sortCategory?: unknown;
	sortGroup?: unknown;
	categoryName?: unknown;
	effectGroupPassive?: unknown;
	effectGroupActive?: unknown;
}

export interface OfficialEquipmentEffectGroupIdentity {
	key: string;
	category: number;
	categoryLabel: string;
	group: number;
	label: string;
	effectTypeId: number;
}

const categoryNames: Record<number, string> = {
	1: 'Unit effects',
	2: 'Defense structure effects',
	3: 'Attack effects',
	4: 'Defense unit effects',
	5: 'Pre-battle effects',
	6: 'Post-battle effects',
	7: 'Economy effects',
	8: 'Courtyard battle effects',
	9: 'Other effects',
};

export function officialEquipmentEffectGroupIdentity(
	definitionId: number,
	definition: OfficialEquipmentEffectGroupingMetadata | undefined,
): OfficialEquipmentEffectGroupIdentity {
	// effecttypes.sortCategory/sortGroup is the highest hierarchy published by
	// the game. Only fall back to an effect-type row when that hierarchy is absent.
	const effectTypeId = positiveMetadataInteger(definition?.effectTypeId) || positiveMetadataInteger(definitionId) || definitionId;
	const officialCategory = positiveMetadataInteger(definition?.sortCategory);
	const officialGroup = positiveMetadataInteger(definition?.sortGroup);
	const category = officialCategory || 99;
	const categoryLabel = cleanOfficialGroupingLabel(definition?.categoryName)
		|| categoryNames[category]
		|| (officialCategory ? `Official category ${officialCategory}` : 'Other effects');

	if (officialCategory && officialGroup) {
		return {
			key: `official-group-${officialCategory}-${officialGroup}`,
			category,
			categoryLabel,
			group: officialGroup,
			label: cleanOfficialGroupingLabel(definition?.effectGroupPassive)
				|| cleanOfficialGroupingLabel(definition?.effectGroupActive)
				|| `Official effect group ${officialCategory}.${officialGroup}`,
			effectTypeId,
		};
	}

	return {
		key: `official-effect-type-${effectTypeId}`,
		category,
		categoryLabel,
		group: effectTypeId,
		label: humanizeOfficialGroupingIdentifier(definition?.effectTypeName)
			|| cleanOfficialGroupingLabel(definition?.name)
			|| humanizeOfficialGroupingIdentifier(definition?.internalName)
			|| `Official effect type ${effectTypeId}`,
		effectTypeId,
	};
}

export function buildEquipmentEffectProfile(
	sources: EquipmentEffectSource[],
	definitions: Record<number, MetadataItem>,
	units: Record<number, MetadataItem>,
	castleTypeID: number,
	combatMode: 'PvP' | 'PvE' | null = null,
): EquipmentEffectProfile {
	const targetIndex = buildOfficialEquipmentTargetIndex(definitions);
	const contributions = sources.flatMap((source) => source.effects.flatMap((effect) => {
		const definition = definitions[effect.definitionId];
		const internalName = metadataString(definition?.internalName);
		const effectTypeName = metadataString(definition?.effectTypeName) || internalName;
		const template = metadataString(definition?.effectTemplate);
		const identity = officialEquipmentEffectGroupIdentity(effect.definitionId, definition);
		const effectTypeId = identity.effectTypeId;
		const areaTypeIds = metadataIntegerList(definition?.areaTypeIds ?? definition?.areaTypeID);
		return resolveEffectValues(effect.values, units).flatMap((resolved) => {
			const value = normalizeSemanticValue(template, resolved.value);
			if (!Number.isFinite(value) || value === 0) return [];
			const isUnitReplacement = effectTypeId === 118 || effectTypeName.toLowerCase() === 'strongerpeasant';
			const replacementUnitID = isUnitReplacement ? Math.trunc(resolved.value) : 0;
			const replacementUnitName = replacementUnitID > 0
				? units[replacementUnitID]?.name?.trim() || `Unit ${replacementUnitID}`
				: '';
			return [{
				definitionId: effect.definitionId,
				effectTypeId,
				label: definition?.name?.trim() || `Effect ${effect.definitionId}`,
				template,
				value,
				polarityValue: resolved.value,
				displayValue: replacementUnitName || undefined,
				unit: isUnitReplacement ? 'number' : effectUnit(effectTypeName, template),
					scope: officialEquipmentEffectScope(definition),
				areaTypeIds,
				category: identity.category,
				categoryName: identity.categoryLabel,
				group: identity.group,
				groupKey: identity.key,
				groupLabel: identity.label,
				groupActiveTemplate: metadataString(definition?.effectGroupActive),
				groupMalusTemplate: metadataString(definition?.effectGroupActiveMalus),
				sortOrder: metadataString(definition?.sortOrder),
				capId: metadataInteger(definition?.capId),
				cap: metadataNumber(definition?.maxTotalBonus),
				source: source.label,
				argumentId: replacementUnitID || resolved.argumentId,
				argumentLabel: replacementUnitName || resolved.argumentLabel,
				commonCapTemplate: metadataString(definition?.commonEffectCapTemplate),
			} satisfies EffectContribution];
		});
	}));

	const aggregatedDetails = Array.from(groupBy(contributions, detailKey), ([key, rows]) => aggregateDetail(key, rows))
		.sort(compareMappedEffects);
	const detailedByRenderedEffect = new Map<string, MappedEquipmentEffect>();
	for (const effect of aggregatedDetails) {
		const key = renderedDetailKey(effect);
		if (!detailedByRenderedEffect.has(key)) detailedByRenderedEffect.set(key, effect);
	}
	const detailed = Array.from(detailedByRenderedEffect.values());
	const sections = Array.from(groupBy(detailed, (effect) => effect.category), ([category, rows]) => ({
		key: String(category),
		title: rows[0]?.categoryName || categoryNames[category] || 'Other effects',
		description: 'Canonical effects grouped by the official game effect category.',
		effectCount: rows.length,
		groups: Array.from(groupBy(rows, groupKey), ([key, groupRows]) => buildEffectGroup(key, groupRows))
			.sort(compareEffectGroups),
	})).sort((left, right) => Number(left.key) - Number(right.key));

	const showcase = Array.from(
		groupBy(
			detailed.filter((effect) => officialEquipmentEffectAppliesToArea(
				definitions[effect.definitionId],
				castleTypeID,
				targetIndex,
				combatMode,
			)),
			groupKey,
		),
		([key, rows]) => buildShowcase(buildEffectGroup(key, rows)),
	).filter((effect) => effect.value !== 0).sort(compareShowcase);

	return { showcase, sections };
}

export function formatEquipmentEffectValue(
	effect: Pick<MappedEquipmentEffect | EquipmentEffectGroup | EquipmentEffectShowcase, 'unit' | 'displayValue'>,
	value: number,
): string {
	if (effect.displayValue) return effect.displayValue;
	const sign = value > 0 ? '+' : value < 0 ? '-' : '';
	return `${sign}${formatUnsignedNumber(value)}${effect.unit === 'percent' ? '%' : ''}`;
}

export function formatEquipmentEffectText(effect: MappedEquipmentEffect, includeCap = true): string {
	const argument = effect.argumentLabel || (effect.argumentId ? `Unit ${effect.argumentId}` : '');
	let text = effect.template.includes('{0}')
		? effect.template
			.replace(/\{0\}/g, formatUnsignedNumber(effect.rawValue))
			.replace(/\{1\}/g, argument)
		: `${effect.label}: ${formatEquipmentEffectValue(effect, effect.rawValue)}`;
	text = cleanTemplate(text.replace(/\{\d+\}/g, ''));
	if (includeCap && effect.cap && effect.cap > 0) {
		text += ` (Max: ${formatLimit(effect.cap, effect.unit)})`;
	}
	return text;
}

export function formatEquipmentEffectLabel(effect: MappedEquipmentEffect): string {
	const argument = effect.argumentLabel || (effect.argumentId ? `Unit ${effect.argumentId}` : '');
	if (!effect.template) return effect.label;
	const label = cleanTemplate(effect.template
		.replace(/\{0\}/g, '')
		.replace(/\{1\}/g, argument)
		.replace(/\{\d+\}/g, '')
		.replace(/^[+\-%\s:]+/, '')
		.replace(/\+\s+(?=[A-Za-z])/g, ''));
	return label ? label.charAt(0).toUpperCase() + label.slice(1) : effect.label;
}

export function formatEquipmentEffectGroupTitle(group: EquipmentEffectGroup): string {
	const template = group.polarityValue < 0 && group.malusTemplate ? group.malusTemplate : group.activeTemplate;
	if (!template.includes('{0}')) return `${formatEquipmentEffectValue(group, group.value)} ${group.label}`.trim();
	return cleanTemplate(template.replace(/\{0\}/g, formatUnsignedNumber(group.value)).replace(/\{\d+\}/g, ''));
}

export function formatEquipmentCommonCap(group: EquipmentEffectGroup, cap: number): string {
	const template = group.commonCapTemplate || 'Common effect group cap: max {0}%';
	return cleanTemplate(template.replace(/\{0\}/g, formatUnsignedNumber(cap)).replace(/\{\d+\}/g, ''));
}

function aggregateDetail(key: string, rows: EffectContribution[]): MappedEquipmentEffect {
	const first = [...rows].sort(compareContributions)[0];
	const rawValue = round(rows.reduce((total, row) => total + row.value, 0));
	const polarityValue = round(rows.reduce((total, row) => total + row.polarityValue, 0));
	const value = round(first.capId > 0 && first.cap > 0 ? clampSigned(rawValue, first.cap) : rawValue);
	return {
		key,
		definitionId: first.definitionId,
		effectTypeId: first.effectTypeId,
		label: first.label,
		template: first.template,
		value,
		rawValue,
		polarityValue,
		displayValue: first.displayValue,
		unit: first.unit,
		scope: first.scope,
		areaTypeIds: first.areaTypeIds,
		category: first.category,
		categoryName: first.categoryName,
		group: first.group,
		groupKey: first.groupKey,
		groupLabel: first.groupLabel,
		groupActiveTemplate: first.groupActiveTemplate,
		groupMalusTemplate: first.groupMalusTemplate,
		sortOrder: first.sortOrder,
		capId: first.capId || undefined,
		cap: first.cap || undefined,
		capped: value !== rawValue,
		sources: Array.from(new Set(rows.map((row) => row.source))),
		argumentId: first.argumentId,
		argumentLabel: first.argumentLabel,
		commonCapTemplate: first.commonCapTemplate,
	};
}

function buildEffectGroup(key: string, rows: MappedEquipmentEffect[]): EquipmentEffectGroup {
	const sortedRows = [...rows].sort(compareMappedEffects);
	const first = sortedRows[0];
	const totals = aggregateCappedValues(sortedRows.map((row) => ({
		value: row.rawValue,
		capId: row.capId || 0,
		cap: row.cap || 0,
	})));
	const capCounts = new Map<number, number>();
	for (const row of sortedRows) {
		if (row.capId && row.cap) capCounts.set(row.capId, (capCounts.get(row.capId) ?? 0) + 1);
	}
	return {
		key,
		category: first.category,
		group: first.group,
		label: first.groupLabel || `Official effect group ${first.category}.${first.group}`,
		activeTemplate: first.groupActiveTemplate,
		malusTemplate: first.groupMalusTemplate,
		value: totals.value,
		rawValue: totals.rawValue,
		polarityValue: round(sortedRows.reduce((total, row) => total + row.polarityValue, 0)),
		displayValue: sortedRows.every((row) => row.displayValue === first.displayValue) ? first.displayValue : undefined,
		unit: sortedRows.every((row) => row.unit === 'number') ? 'number' : 'percent',
		capped: totals.value !== totals.rawValue,
		rows: sortedRows,
		commonCaps: Array.from(capCounts, ([capId, count]) => ({
			capId,
			max: count > 1 ? sortedRows.find((row) => row.capId === capId)?.cap ?? 0 : 0,
		})).filter((cap) => cap.max > 0),
		commonCapTemplate: first.commonCapTemplate,
	};
}

function buildShowcase(group: EquipmentEffectGroup): EquipmentEffectShowcase {
	return {
		key: group.key,
		label: group.label,
		value: group.value,
		rawValue: group.rawValue,
		displayValue: group.displayValue,
		unit: group.unit,
		category: group.category,
		group: group.group,
		capped: group.capped,
	};
}

function aggregateCappedValues(rows: Array<{ value: number; capId: number; cap: number }>): { value: number; rawValue: number } {
	const capBuckets = new Map<number, { value: number; cap: number }>();
	let uncappedValue = 0;
	let rawValue = 0;
	for (const row of rows) {
		rawValue += row.value;
		if (row.capId > 0 && row.cap > 0) {
			const bucket = capBuckets.get(row.capId) ?? { value: 0, cap: row.cap };
			bucket.value += row.value;
			bucket.cap = Math.max(bucket.cap, row.cap);
			capBuckets.set(row.capId, bucket);
		} else {
			uncappedValue += row.value;
		}
	}
	const value = uncappedValue + Array.from(capBuckets.values())
		.reduce((total, bucket) => total + clampSigned(bucket.value, bucket.cap), 0);
	return { value: round(value), rawValue: round(rawValue) };
}

function resolveEffectValues(values: number[], units: Record<number, MetadataItem>): EffectValue[] {
	if (values.length === 0) return [];
	const hasArgumentPairs = values.length % 2 === 0
		&& values.every((value, index) => index % 2 === 1 || Number.isInteger(value) && value >= 1);
	if (!hasArgumentPairs) return [{ value: values[values.length - 1] }];
	const resolved: EffectValue[] = [];
	for (let index = 0; index < values.length; index += 2) {
		const argumentId = values[index];
		resolved.push({
			value: values[index + 1],
			argumentId,
			argumentLabel: units[argumentId]?.name?.trim() || undefined,
		});
	}
	return resolved;
}

function normalizeSemanticValue(template: string, value: number): number {
	if (/-\s*\{0\}|\{0\}\s*-/.test(template)) return -Math.abs(value);
	if (/\+\s*\{0\}|\{0\}\s*\+/.test(template)) return Math.abs(value);
	return value;
}

function effectUnit(effectTypeName: string, template: string): 'percent' | 'number' {
	if (/unitamountyard|unitwallabsolute/i.test(effectTypeName)) return 'number';
	if (template.includes('%')) return 'percent';
	return /wave|supportunits|reinforcement|slotbonus|amount(?:spies|guards|peasant|population)/i.test(effectTypeName)
		? 'number'
		: 'percent';
}

function detailKey(effect: EffectContribution): string {
	return `${effect.definitionId}:${effect.argumentId || 0}:${effect.scope}`;
}

function groupKey(effect: MappedEquipmentEffect): string {
	return effect.groupKey;
}

function compareEffectGroups(left: EquipmentEffectGroup, right: EquipmentEffectGroup): number {
	return left.category - right.category || left.group - right.group || left.label.localeCompare(right.label);
}

function renderedDetailKey(effect: MappedEquipmentEffect): string {
	return `${effect.definitionId}:${effect.scope}:${formatEquipmentEffectLabel(effect).toLowerCase()}:${effect.rawValue}`;
}

function compareShowcase(left: EquipmentEffectShowcase, right: EquipmentEffectShowcase): number {
	return left.category - right.category || left.group - right.group || left.label.localeCompare(right.label);
}

function compareMappedEffects(left: MappedEquipmentEffect, right: MappedEquipmentEffect): number {
	return left.category - right.category
		|| left.group - right.group
		|| compareSortOrder(left.sortOrder, right.sortOrder)
		|| left.label.localeCompare(right.label);
}

function compareContributions(left: EffectContribution, right: EffectContribution): number {
	return left.category - right.category
		|| left.group - right.group
		|| compareSortOrder(left.sortOrder, right.sortOrder)
		|| left.label.localeCompare(right.label);
}

function compareSortOrder(left: string, right: string): number {
	const leftParts = left.split('.').map(Number);
	const rightParts = right.split('.').map(Number);
	for (let index = 0; index < Math.max(leftParts.length, rightParts.length); index += 1) {
		const leftValue = Number.isFinite(leftParts[index]) ? leftParts[index] : 0;
		const rightValue = Number.isFinite(rightParts[index]) ? rightParts[index] : 0;
		if (leftValue !== rightValue) return leftValue - rightValue;
	}
	return 0;
}

function groupBy<T, K>(values: T[], keyFor: (value: T) => K): Map<K, T[]> {
	const grouped = new Map<K, T[]>();
	for (const value of values) {
		const key = keyFor(value);
		grouped.set(key, [...(grouped.get(key) ?? []), value]);
	}
	return grouped;
}

function cleanTemplate(value: string): string {
	return value
		.replace(/\s+/g, ' ')
		.replace(/\+\+/g, '+')
		.replace(/--/g, '-')
		.replace(/%%/g, '%')
		.replace(/\s+([,.;:])/g, '$1')
		.trim();
}

function formatUnsignedNumber(value: number): string {
	const absolute = Math.abs(value);
	return absolute.toLocaleString(undefined, {
		minimumFractionDigits: Number.isInteger(absolute) ? 0 : 1,
		maximumFractionDigits: 1,
	});
}

function formatLimit(value: number, unit: 'percent' | 'number'): string {
	return `${formatUnsignedNumber(value)}${unit === 'percent' ? '%' : ''}`;
}

function clampSigned(value: number, cap: number): number {
	return cap > 0 ? Math.max(-cap, Math.min(cap, value)) : value;
}

function metadataString(value: unknown): string {
	return typeof value === 'string' ? value.trim() : '';
}

function metadataInteger(value: unknown): number {
	const parsed = typeof value === 'number' ? value : Number(value);
	return Number.isFinite(parsed) ? Math.trunc(parsed) : 0;
}

function positiveMetadataInteger(value: unknown): number {
	return Math.max(0, metadataInteger(value));
}

function cleanOfficialGroupingLabel(value: unknown): string {
	const label = metadataString(value)
		.replace(/\{\d+\}/g, '')
		.replace(/^[+\-%\s:]+/, '')
		.replace(/[+\-%\s:]+$/, '')
		.replace(/\s+/g, ' ')
		.trim();
	return label ? label.charAt(0).toUpperCase() + label.slice(1) : '';
}

function humanizeOfficialGroupingIdentifier(value: unknown): string {
	const label = metadataString(value)
		.replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
		.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
		.replace(/[_-]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
	return label ? label.charAt(0).toUpperCase() + label.slice(1) : '';
}

function metadataNumber(value: unknown): number {
	const parsed = typeof value === 'number' ? value : Number(value);
	return Number.isFinite(parsed) ? parsed : 0;
}

function metadataIntegerList(value: unknown): number[] {
	const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : [value];
	return Array.from(new Set(values.map(metadataInteger).filter((entry) => entry > 0)));
}

function round(value: number): number {
	return Math.round(value * 10) / 10;
}
