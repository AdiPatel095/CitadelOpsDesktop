import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { CitadelAPI } from '../api/CitadelClient';
import { officialEquipmentEffectScope } from '../equipment/EquipmentEffectApplicability';

export interface MetadataItem {
  id: number;
  name: string;
  image?: string;
  level?: number;
  outputAmount?: number;
  [key: string]: unknown;
}

interface MetadataContextValue {
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
	buildings: Record<number, MetadataItem>;
  decorations: Record<number, MetadataItem>;
	resources: Record<number, MetadataItem>;
	currencies: Record<number, MetadataItem>;
	equipments: Record<number, MetadataItem>;
	gems: Record<number, MetadataItem>;
	effects: Record<number, MetadataItem>;
	kingdoms: Record<number, MetadataItem>;
  craftingRecipes: Record<number, MetadataItem>;
  isLoading: boolean;
  unitsLoading: boolean;
  unitsError: string | null;
  getTroop: (id: number) => MetadataItem | undefined;
  getTool: (id: number) => MetadataItem | undefined;
	getBuilding: (id: number) => MetadataItem | undefined;
	getEquipment: (id: number) => MetadataItem | undefined;
	getGem: (id: number) => MetadataItem | undefined;
	getEffect: (id: number) => MetadataItem | undefined;
	getCraftingRecipe: (id: number) => MetadataItem | undefined;
  getDecoration: (id: number) => MetadataItem | undefined;
}

const MetadataContext = createContext<MetadataContextValue | undefined>(undefined);

export function MetadataProvider({ children }: { children: React.ReactNode }) {
	const { catalogs } = useCitadelAPI();
  const [troops, setTroops] = useState<Record<number, MetadataItem>>({});
  const [tools, setTools] = useState<Record<number, MetadataItem>>({});
	const [buildings, setBuildings] = useState<Record<number, MetadataItem>>({});
  const [decorations, setDecorations] = useState<Record<number, MetadataItem>>({});
	const [resources, setResources] = useState<Record<number, MetadataItem>>({});
	const [currencies, setCurrencies] = useState<Record<number, MetadataItem>>({});
	const [equipments, setEquipments] = useState<Record<number, MetadataItem>>({});
	const [gems, setGems] = useState<Record<number, MetadataItem>>({});
	const [effects, setEffects] = useState<Record<number, MetadataItem>>({});
	const [kingdoms, setKingdoms] = useState<Record<number, MetadataItem>>({});
	const [craftingRecipes, setCraftingRecipes] = useState<Record<number, MetadataItem>>({});
	const [unitsLoading, setUnitsLoading] = useState(true);
	const [optionalLoading, setOptionalLoading] = useState(true);
	const [unitsError, setUnitsError] = useState<string | null>(null);
	const [unitsRetryNonce, setUnitsRetryNonce] = useState(0);
	const [optionalRetryNonce, setOptionalRetryNonce] = useState(0);
	const unitsCatalogKey = useRef('');
	const optionalCatalogKey = useRef('');
	const catalogKey = [
		catalogs?.metadata.digestSha256 ?? '',
		catalogs?.metadata.languageVersion ?? '',
	].join(':');
	const isLoading = unitsLoading || optionalLoading;

  useEffect(() => {
    let cancelled = false;
		let retryTimer: ReturnType<typeof setTimeout> | null = null;
    const load = async () => {
			if (unitsCatalogKey.current !== catalogKey) {
				unitsCatalogKey.current = catalogKey;
				setUnitsLoading(true);
			}
			let unitsResponse: { items: OfficialRecord[] };
			try {
				unitsResponse = await CitadelAPI.getCatalog<OfficialRecord>('units');
			} catch (error) {
				if (cancelled) return;
				console.error('Could not load official unit metadata', error);
				setUnitsError('Official troop and tool metadata is temporarily unavailable. CitadelOps will retry automatically.');
				setUnitsLoading(false);
				retryTimer = setTimeout(() => setUnitsRetryNonce((current) => current + 1), 5_000);
				return;
			}
			let unitTranslations: Record<string, string> = {};
			try {
				unitTranslations = await CitadelAPI.localize(localizationKeys(unitsResponse.items));
			} catch (error) {
				console.warn('Could not localize official unit metadata; using catalog names', error);
			}
			if (cancelled) return;
			const nextTroops: Record<number, MetadataItem> = {};
			const nextTools: Record<number, MetadataItem> = {};
			for (const row of unitsResponse.items) {
				const id = positiveID(row.wodID);
				if (id === 0) continue;
				const item: MetadataItem = {
					...row,
					id,
					name: displayName(row, unitTranslations, `Unit ${id}`),
					image: `/game-data/${isTool(row) ? 'tools' : 'troops'}/images/${id}.webp`,
				};
				if (isTool(row)) nextTools[id] = item;
				else nextTroops[id] = item;
			}
			setTroops(nextTroops);
			setTools(nextTools);
			setUnitsError(null);
			setUnitsLoading(false);
	    };
	    void load();
	    return () => {
	      cancelled = true;
			if (retryTimer != null) clearTimeout(retryTimer);
	    };
	  }, [catalogKey, unitsRetryNonce]);

	useEffect(() => {
		let cancelled = false;
		let retryTimer: ReturnType<typeof setTimeout> | null = null;
		const load = async () => {
			if (optionalCatalogKey.current !== catalogKey) {
				optionalCatalogKey.current = catalogKey;
				setOptionalLoading(true);
			}
			const results = await Promise.allSettled([
				CitadelAPI.getCatalog<OfficialRecord>('buildings'),
				CitadelAPI.getCatalog<OfficialRecord>('resources'),
				CitadelAPI.getCatalog<OfficialRecord>('currencies'),
				loadCurrencyIconRecords(),
				CitadelAPI.getCatalog<OfficialRecord>('equipments'),
				CitadelAPI.getCatalog<OfficialRecord>('gems'),
				CitadelAPI.getCatalog<OfficialRecord>('effects'),
				CitadelAPI.getCatalog<OfficialRecord>('effecttypes'),
				CitadelAPI.getCatalog<OfficialRecord>('effectCaps'),
				CitadelAPI.getCatalog<OfficialRecord>('kingdoms'),
				CitadelAPI.getProjection<CraftingProjection>('crafting'),
			] as const);
			if (cancelled) return;
			for (const result of results) {
				if (result.status === 'rejected') console.error('Could not load optional official game metadata', result.reason);
			}
			const [buildingsResult, resourcesResult, currenciesResult, currencyIconsResult, equipmentsResult,
				gemsResult, effectsResult, effectTypesResult, effectCapsResult, kingdomsResult, craftingResult] = results;

			if (buildingsResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization([
					...localizationKeys(buildingsResult.value.items),
					...decorationLocalizationKeys(buildingsResult.value.items),
				]);
				if (cancelled) return;
				const nextBuildings: Record<number, MetadataItem> = {};
				const nextDecorations: Record<number, MetadataItem> = {};
				for (const row of buildingsResult.value.items) {
					const id = positiveID(row.wodID);
					if (id === 0) continue;
					const level = positiveID(row.level);
					const decoration = isDecoration(row);
					const internalName = decoration ? row.type : row.name;
					const item: MetadataItem = {
						...row, id,
						internalName: typeof internalName === 'string' ? internalName : undefined,
						name: decoration
							? decorationDisplayName(row, translations, id)
							: displayName(row, translations, `Building ${id}`),
						...(level > 0 ? { level } : {}),
					};
					nextBuildings[id] = item;
					if (decoration) nextDecorations[id] = { ...item, image: `/game-data/decorations/images/${id}.webp` };
				}
				setBuildings(nextBuildings);
				setDecorations(nextDecorations);
			}
			if (resourcesResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization(localizationKeys(resourcesResult.value.items));
				if (cancelled) return;
				const next = definitionMetadata(resourcesResult.value.items, 'resourceID', translations, 'Resource');
				for (const resource of Object.values(next)) resource.image = resourceImageURL(resource.internalName) ?? resource.image;
				setResources(next);
			}
			if (currenciesResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization([
					...localizationKeys(currenciesResult.value.items),
					...currencyLocalizationKeys(currenciesResult.value.items),
				]);
				if (cancelled) return;
				const next = definitionMetadata(currenciesResult.value.items, 'currencyID', translations, 'Currency');
				const iconRows = currencyIconsResult.status === 'fulfilled' ? currencyIconsResult.value : [];
				const icons = new Map(iconRows.flatMap((row) => {
					const assetName = typeof row.assetName === 'string' ? row.assetName.trim() : '';
					const iconURL = typeof row.url === 'string' ? row.url.trim() : '';
					return assetName && iconURL ? [[assetName, iconURL] as const] : [];
				}));
				for (const currency of Object.values(next)) {
					const assetName = typeof currency.assetName === 'string' ? currency.assetName.trim() : '';
					currency.image = icons.get(assetName) ?? localCurrencyImageURL(assetName);
				}
				setCurrencies(next);
			}
			if (equipmentsResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization(localizationKeys(equipmentsResult.value.items));
				if (cancelled) return;
				setEquipments(definitionMetadata(equipmentsResult.value.items, 'equipmentID', translations, 'Equipment'));
			}
			if (gemsResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization(localizationKeys(gemsResult.value.items));
				if (cancelled) return;
				setGems(definitionMetadata(gemsResult.value.items, 'gemID', translations, 'Gem'));
			}
			if (effectsResult.status === 'fulfilled' && effectTypesResult.status === 'fulfilled' && effectCapsResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization([
					...localizationKeys([...effectsResult.value.items, ...effectTypesResult.value.items]),
					...effectLocalizationKeys(effectsResult.value.items, effectTypesResult.value.items),
				]);
				if (cancelled) return;
				setEffects(effectDefinitionMetadata(
					effectsResult.value.items, effectTypesResult.value.items, effectCapsResult.value.items, translations,
				));
			}
			if (kingdomsResult.status === 'fulfilled') {
				const translations = await bestEffortLocalization(localizationKeys(kingdomsResult.value.items));
				if (cancelled) return;
				setKingdoms(definitionMetadata(kingdomsResult.value.items, 'kID', translations, 'Kingdom', true));
			}
			if (craftingResult.status === 'fulfilled') {
				const next: Record<number, MetadataItem> = {};
				for (const recipe of craftingResult.value.recipes ?? []) {
					const id = positiveID(recipe.recipeID);
					if (id === 0) continue;
					const outputName = typeof recipe.output?.name === 'string' && recipe.output.name.trim()
						? recipe.output.name.trim() : `Recipe ${id}`;
					next[id] = {
						...recipe, id,
						name: recipe.level > 0 ? `${outputName} · L${recipe.level}` : outputName,
						image: recipe.output?.iconUrl,
						outputAmount: recipe.output?.amount,
					};
				}
				setCraftingRecipes(next);
			}
			setOptionalLoading(false);
			if (results.some((result) => result.status === 'rejected')) {
				retryTimer = setTimeout(() => setOptionalRetryNonce((current) => current + 1), 5_000);
			}
		};
		void load();
		return () => {
			cancelled = true;
			if (retryTimer != null) clearTimeout(retryTimer);
		};
	}, [catalogKey, optionalRetryNonce]);

  const getTroop = useCallback((id: number) => troops[id], [troops]);
  const getTool = useCallback((id: number) => tools[id], [tools]);
	const getBuilding = useCallback((id: number) => buildings[id], [buildings]);
	const getEquipment = useCallback((id: number) => equipments[id], [equipments]);
	const getGem = useCallback((id: number) => gems[id], [gems]);
	const getEffect = useCallback((id: number) => effects[id], [effects]);
	const getCraftingRecipe = useCallback((id: number) => craftingRecipes[id], [craftingRecipes]);
  const getDecoration = useCallback((id: number) => decorations[id], [decorations]);
  const value = useMemo<MetadataContextValue>(() => ({
    troops,
    tools,
		buildings,
    decorations,
		resources,
		currencies,
		equipments,
		gems,
		effects,
		kingdoms,
		craftingRecipes,
    isLoading,
		unitsLoading,
		unitsError,
    getTroop,
    getTool,
		getBuilding,
		getEquipment,
		getGem,
		getEffect,
		getCraftingRecipe,
    getDecoration,
	}), [
		buildings,
		craftingRecipes,
		currencies,
		decorations,
		effects,
		equipments,
		gems,
		getBuilding,
		getDecoration,
		getEffect,
		getCraftingRecipe,
		getEquipment,
		getGem,
		getTool,
		getTroop,
		isLoading,
		kingdoms,
		resources,
		tools,
		troops,
		unitsLoading,
		unitsError,
	]);

  return <MetadataContext.Provider value={value}>{children}</MetadataContext.Provider>;
}

export function useMetadata(): MetadataContextValue {
  const context = useContext(MetadataContext);
  if (!context) throw new Error('useMetadata must be used within MetadataProvider');
  return context;
}

type OfficialRecord = Record<string, unknown>;

interface CraftingProjection {
	recipes?: Array<{
		recipeID: number;
		level?: number;
		output?: { name?: string; amount?: number; iconUrl?: string };
		[key: string]: unknown;
	}>;
}

function isTool(row: OfficialRecord): boolean {
  if (Array.isArray(row.slotTypes)) return row.slotTypes.length > 0;
  return typeof row.slotTypes === 'string' && row.slotTypes.trim() !== '';
}

function isDecoration(row: OfficialRecord): boolean {
  return row.buildingGroundType === 'DECO'
    || row.shopCategory === 'DECO'
    || row.name === 'Deco'
    || (typeof row.type === 'string' && row.type.includes('Deco'));
}

function decorationLocalizationKeys(rows: OfficialRecord[]): string[] {
	const keys = new Set<string>();
	for (const row of rows) {
		if (!isDecoration(row) || typeof row.type !== 'string' || !row.type.trim()) continue;
		keys.add(`deco_${row.type.trim()}_name`);
	}
	return Array.from(keys).slice(0, 5000);
}

function decorationDisplayName(
	row: OfficialRecord,
	translations: Record<string, string>,
	id: number,
): string {
	const type = typeof row.type === 'string' ? row.type.trim() : '';
	if (type) {
		const localized = translations[`deco_${type}_name`];
		if (localized?.trim()) return localized.trim();
	}
	const explicit = typeof row._display_name === 'string' ? row._display_name.trim() : '';
	if (explicit && !/^decorative items?$/i.test(explicit)) return explicit;
	return `Decoration ${id}`;
}

function localizationKeys(rows: OfficialRecord[]): string[] {
  const keys = new Set<string>();
  for (const row of rows) {
	if (typeof row.kingdomName === 'string' && row.kingdomName.trim()) {
		keys.add(`kingdomName_${row.kingdomName.trim()}`);
	}
    for (const value of [row.type, row.name, row.Name, row.JSONKey, row.kingdomName, row.comment2]) {
      if (typeof value !== 'string' || value.trim() === '') continue;
      keys.add(`${value}_name`);
      keys.add(value);
    }
  }
  return Array.from(keys).slice(0, 5000);
}

async function bestEffortLocalization(keys: string[]): Promise<Record<string, string>> {
	if (keys.length === 0) return {};
	try {
		return await CitadelAPI.localize(Array.from(new Set(keys)).slice(0, 5000));
	} catch (error) {
		console.warn('Could not localize optional official metadata; using catalog names', error);
		return {};
	}
}

async function loadCurrencyIconRecords(): Promise<OfficialRecord[]> {
	return (await CitadelAPI.getCatalog<OfficialRecord>('currency-icons')).items;
}

function effectLocalizationKeys(effectRows: OfficialRecord[], effectTypeRows: OfficialRecord[]): string[] {
	const keys = new Set<string>();
	for (const row of effectRows) {
		const name = typeof row.name === 'string' ? row.name.trim() : '';
		if (!name) continue;
		keys.add(`relicequip_effect_description_${name}`);
		keys.add(`equip_effect_description_${name}`);
		keys.add(`ci_effect_${name}`);
		keys.add(`effect_name_${name}`);
	}
	for (const row of effectTypeRows) {
		const category = metadataInteger(row.sortCategory);
		const group = metadataInteger(row.sortGroup);
		if (category) keys.add(`effect_category_${category}`);
		if (!category || !group) continue;
		const prefix = `effect_group_${category}_${group}`;
		keys.add(`${prefix}_passive`);
		keys.add(`${prefix}_active`);
		keys.add(`${prefix}_active_malus`);
	}
	keys.add('effect_category_commonEffectCap');
	return Array.from(keys).slice(0, 5000);
}

function currencyLocalizationKeys(rows: OfficialRecord[]): string[] {
	const keys = new Set<string>();
	for (const row of rows) {
		for (const value of [row.assetName, row.Name]) {
			if (typeof value !== 'string' || !value.trim()) continue;
			const name = value.trim();
			keys.add(`currency_name_${name}`);
			keys.add(`currency_name_${lowerFirst(name)}`);
		}
	}
	return Array.from(keys);
}

function displayName(row: OfficialRecord, translations: Record<string, string>, fallback: string): string {
  if (typeof row._display_name === 'string' && row._display_name.trim() !== '') return row._display_name;
  for (const value of [row.type, row.name, row.Name, row.JSONKey, row.kingdomName, row.comment2]) {
    if (typeof value !== 'string' || value.trim() === '') continue;
    const translated = translations[`kingdomName_${value}`] ?? translations[`${value}_name`] ?? translations[value];
    if (translated?.trim()) return translated;
  }
  for (const value of [row.type, row.name, row.Name, row.kingdomName, row.comment2]) {
    if (typeof value === 'string' && value.trim()) return value;
  }
  return fallback;
}

function positiveID(value: unknown): number {
  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(parsed)) return 0;
  const integer = Math.trunc(parsed);
  return integer > 0 ? integer : 0;
}

function definitionMetadata(
	rows: OfficialRecord[],
	idField: string,
	translations: Record<string, string>,
	fallbackPrefix: string,
	allowZero = false,
): Record<number, MetadataItem> {
	const result: Record<number, MetadataItem> = {};
	for (const row of rows) {
		const parsedID = Number(row[idField]);
		const id = Number.isFinite(parsedID) ? Math.trunc(parsedID) : -1;
		if (id < 0 || (!allowZero && id === 0)) continue;
		const internalName = [row.name, row.Name, row.kingdomName, row.assetName]
			.find((value): value is string => typeof value === 'string' && value.trim() !== '');
		const officialCurrencyName = idField === 'currencyID' ? currencyDisplayName(row, translations) : '';
		result[id] = {
			...row,
			id,
			internalName,
			name: officialCurrencyName || displayName(row, translations, internalName ? splitIdentifier(internalName) : `${fallbackPrefix} ${id}`),
			image: typeof row.assetName === 'string' && row.assetName.trim()
				? `/game-data/resources/images/${row.assetName}.webp`
				: undefined,
		};
	}
	return result;
}

function currencyDisplayName(row: OfficialRecord, translations: Record<string, string>): string {
	for (const value of [row.assetName, row.Name]) {
		if (typeof value !== 'string' || !value.trim()) continue;
		const name = value.trim();
		for (const key of [`currency_name_${name}`, `currency_name_${lowerFirst(name)}`]) {
			const translated = translations[key];
			if (translated?.trim()) return translated.trim();
		}
	}
	const fallback = [row.assetName, row.Name]
		.find((value): value is string => typeof value === 'string' && value.trim() !== '');
	return fallback ? splitIdentifier(fallback) : '';
}

function lowerFirst(value: string): string {
	return value ? value.charAt(0).toLowerCase() + value.slice(1) : value;
}

function splitIdentifier(value: string): string {
	return value
		.replace(/[_-]+/g, ' ')
		.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
		.trim();
}

const resourceAssetByInternalName: Record<string, string> = {
	currency1: 'Coins',
	currency2: 'Ruby',
	wood: 'Wood',
	stone: 'Stone',
	food: 'Food',
	coal: 'Charcoal',
	oil: 'OliveOil',
	glass: 'Glass',
	aquamarine: 'Aquamarine',
	iron: 'Iron_Ore',
	honey: 'Honey',
	mead: 'Mead',
	beef: 'Beef',
};

function resourceImageURL(internalName: unknown): string | undefined {
	const key = typeof internalName === 'string' ? internalName.trim().toLowerCase() : '';
	const asset = resourceAssetByInternalName[key];
	return asset ? `/game-data/resources/images/${asset}.webp` : undefined;
}

const localCurrencyAssetByOfficialName: Record<string, string> = {
	'1MinSkip': 'TimeSkip1Minute',
	'5MinSkip': 'TimeSkip5Minutes',
	'10MinSkip': 'TimeSkip10Minutes',
	'30MinSkip': 'TimeSkip30Minutes',
	'60MinSkip': 'TimeSkip1Hour',
	'5HourSkip': 'TimeSkip5Hours',
	'24HourSkip': 'TimeSkip24Hours',
	DragonScaleSplinters: 'DragonScaleSplinters',
	DragonScaleTile: 'DragonScaleTiles',
	ImperialDucat: 'ImperialDucat',
	Plaster: 'Plaster',
	RelicFragment: 'Relic_Shards',
	SceatToken: 'Sceat',
};

function localCurrencyImageURL(assetName: string): string | undefined {
	const localAsset = localCurrencyAssetByOfficialName[assetName];
	return localAsset ? `/game-data/resources/images/${localAsset}.webp` : undefined;
}

function effectDefinitionMetadata(
	rows: OfficialRecord[],
	effectTypeRows: OfficialRecord[],
	effectCapRows: OfficialRecord[],
	translations: Record<string, string>,
): Record<number, MetadataItem> {
	const effectTypes = new Map(effectTypeRows.map((row) => [String(row.effectTypeID ?? ''), row]));
	const effectCaps = new Map(effectCapRows.map((row) => [String(row.capID ?? ''), row]));
	const result: Record<number, MetadataItem> = {};
	for (const row of rows) {
		const id = positiveID(row.effectID);
		if (id === 0) continue;
		const internalName = typeof row.name === 'string' ? row.name.trim() : '';
		const effectType = effectTypes.get(String(row.effectTypeID ?? ''));
		const effectTypeName = typeof effectType?.name === 'string' ? effectType.name.trim() : '';
		const translatedName = displayName(row, translations, internalName || `Effect ${id}`);
		const cap = effectCaps.get(String(row.capID ?? ''));
		const officialScope = officialEquipmentEffectScope({ ...row, internalName, effectTypeName });
		const scope = officialScope === 'PvP' ? 'pvp' : officialScope === 'PvE' ? 'pve' : 'generic';
		const category = metadataInteger(effectType?.sortCategory);
		const group = metadataInteger(effectType?.sortGroup);
		const effectTemplate = firstTranslation(translations, [
			`relicequip_effect_description_${internalName}`,
			`equip_effect_description_${internalName}`,
			`ci_effect_${internalName}`,
			`effect_name_${internalName}`,
		]);
		const groupPrefix = category && group ? `effect_group_${category}_${group}` : '';
		result[id] = {
			...row,
			id,
			internalName,
			name: effectTemplate
				? humanizeEffectTemplate(effectTemplate)
				: translatedName !== internalName
				? translatedName
				: humanizeEffectName(effectTypeName || internalName || `Effect ${id}`),
			effectTypeId: metadataInteger(row.effectTypeID),
			effectTypeName,
			combatType: metadataInteger(effectType?.combatType),
			sortCategory: category,
			sortGroup: group,
			categoryName: category ? translations[`effect_category_${category}`] : undefined,
			effectTemplate,
			effectGroupPassive: groupPrefix ? translations[`${groupPrefix}_passive`] : undefined,
			effectGroupActive: groupPrefix ? translations[`${groupPrefix}_active`] : undefined,
			effectGroupActiveMalus: groupPrefix ? translations[`${groupPrefix}_active_malus`] : undefined,
			commonEffectCapTemplate: translations.effect_category_commonEffectCap,
			capId: metadataInteger(row.capID),
			maxTotalBonus: metadataNumber(cap?.maxTotalBonus),
			areaTypeIds: metadataIntegerList(row.areaTypeID),
			scope,
		};
	}
	return result;
}

function metadataIntegerList(value: unknown): number[] {
	const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : [value];
	return Array.from(new Set(values.map(metadataInteger).filter((entry) => entry > 0)));
}

function firstTranslation(translations: Record<string, string>, keys: string[]): string | undefined {
	for (const key of keys) {
		const value = translations[key];
		if (value?.trim() && !/lost its powers|seems to have run out/i.test(value)) return value.trim();
	}
	return undefined;
}

function humanizeEffectTemplate(value: string): string {
	const label = value
		.replace(/\{\d+\}/g, '')
		.replace(/^[+\-\s%:]+/, '')
		.replace(/\s+/g, ' ')
		.replace(/\s+([,.;:])/g, '$1')
		.trim();
	return label ? label.charAt(0).toUpperCase() + label.slice(1) : value;
}

function humanizeEffectName(value: string): string {
	const withoutTechnicalAffixes = value
		.replace(/^(?:equipmentARE|equipment|relic)/i, '')
		.replace(/(?:PVP|PVE)$/i, '');
	const words = splitIdentifier(withoutTechnicalAffixes)
		.replace(/\bYard\b/gi, 'Courtyard')
		.replace(/\bRange\b/gi, 'Ranged')
		.trim();
	return words ? words.charAt(0).toUpperCase() + words.slice(1) : value;
}

function metadataInteger(value: unknown): number | undefined {
	const parsed = typeof value === 'number' ? value : Number(value);
	return Number.isFinite(parsed) ? Math.trunc(parsed) : undefined;
}

function metadataNumber(value: unknown): number | undefined {
	const parsed = typeof value === 'number' ? value : Number(value);
	return Number.isFinite(parsed) ? parsed : undefined;
}
