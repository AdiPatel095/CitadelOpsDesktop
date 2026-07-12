import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { CitadelAPI } from '../api/CitadelClient';

export interface MetadataItem {
  id: number;
  name: string;
  image?: string;
  level?: number;
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
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setIsLoading(true);
      try {
		const [
			unitsResponse,
			buildingsResponse,
			resourcesResponse,
			currenciesResponse,
			equipmentsResponse,
			gemsResponse,
			effectsResponse,
			kingdomsResponse,
			craftingResponse,
		] = await Promise.all([
          CitadelAPI.getCatalog<OfficialRecord>('units'),
          CitadelAPI.getCatalog<OfficialRecord>('buildings'),
			CitadelAPI.getCatalog<OfficialRecord>('resources'),
			CitadelAPI.getCatalog<OfficialRecord>('currencies'),
			CitadelAPI.getCatalog<OfficialRecord>('equipments'),
			CitadelAPI.getCatalog<OfficialRecord>('gems'),
			CitadelAPI.getCatalog<OfficialRecord>('effects'),
			CitadelAPI.getCatalog<OfficialRecord>('kingdoms'),
			CitadelAPI.getProjection<CraftingProjection>('crafting'),
        ]);
		const keys = localizationKeys([
			...kingdomsResponse.items,
			...unitsResponse.items,
			...buildingsResponse.items,
			...resourcesResponse.items,
			...currenciesResponse.items,
			...equipmentsResponse.items,
			...gemsResponse.items,
			...effectsResponse.items,
		]);
        const translations = await CitadelAPI.localize(keys);
        if (cancelled) return;

        const nextTroops: Record<number, MetadataItem> = {};
        const nextTools: Record<number, MetadataItem> = {};
        for (const row of unitsResponse.items) {
          const id = positiveID(row.wodID);
          if (id === 0) continue;
          const item: MetadataItem = {
            ...row,
            id,
            name: displayName(row, translations, `Unit ${id}`),
            image: `/game-data/${isTool(row) ? 'tools' : 'troops'}/images/${id}.webp`,
          };
          if (isTool(row)) nextTools[id] = item;
          else nextTroops[id] = item;
        }

		const nextBuildings: Record<number, MetadataItem> = {};
        const nextDecorations: Record<number, MetadataItem> = {};
        for (const row of buildingsResponse.items) {
          const id = positiveID(row.wodID);
          if (id === 0) continue;
          const level = positiveID(row.level);
			const item: MetadataItem = {
            ...row,
            id,
			internalName: typeof row.name === 'string' ? row.name : undefined,
			name: displayName(row, translations, `Building ${id}`),
            ...(level > 0 ? { level } : {}),
          };
			nextBuildings[id] = item;
			if (isDecoration(row)) {
				nextDecorations[id] = { ...item, image: `/game-data/decorations/images/${id}.webp` };
			}
        }
		const nextResources = definitionMetadata(resourcesResponse.items, 'resourceID', translations, 'Resource');
		const nextCurrencies = definitionMetadata(currenciesResponse.items, 'currencyID', translations, 'Currency');
		const nextEquipments = definitionMetadata(equipmentsResponse.items, 'equipmentID', translations, 'Equipment');
		const nextGems = definitionMetadata(gemsResponse.items, 'gemID', translations, 'Gem');
		const nextEffects = definitionMetadata(effectsResponse.items, 'effectID', translations, 'Effect');
		const nextKingdoms = definitionMetadata(kingdomsResponse.items, 'kID', translations, 'Kingdom');
		const nextCraftingRecipes: Record<number, MetadataItem> = {};
		for (const recipe of craftingResponse.recipes ?? []) {
			const id = positiveID(recipe.recipeID);
			if (id === 0) continue;
			const outputName = typeof recipe.output?.name === 'string' && recipe.output.name.trim()
				? recipe.output.name.trim()
				: `Recipe ${id}`;
			nextCraftingRecipes[id] = {
				...recipe,
				id,
				name: recipe.level > 0 ? `${outputName} · L${recipe.level}` : outputName,
				image: recipe.output?.iconUrl,
			};
		}
        setTroops(nextTroops);
        setTools(nextTools);
		setBuildings(nextBuildings);
        setDecorations(nextDecorations);
		setResources(nextResources);
		setCurrencies(nextCurrencies);
		setEquipments(nextEquipments);
		setGems(nextGems);
		setEffects(nextEffects);
		setKingdoms(nextKingdoms);
		setCraftingRecipes(nextCraftingRecipes);
      } catch (error) {
        if (!cancelled) console.error('Could not load official game metadata', error);
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

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
		output?: { name?: string; iconUrl?: string };
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

function localizationKeys(rows: OfficialRecord[]): string[] {
  const keys = new Set<string>();
  for (const row of rows) {
    for (const value of [row.type, row.name, row.Name, row.JSONKey, row.kingdomName, row.comment2]) {
      if (typeof value !== 'string' || value.trim() === '') continue;
      keys.add(`${value}_name`);
      keys.add(value);
    }
  }
  return Array.from(keys).slice(0, 5000);
}

function displayName(row: OfficialRecord, translations: Record<string, string>, fallback: string): string {
  if (typeof row._display_name === 'string' && row._display_name.trim() !== '') return row._display_name;
  for (const value of [row.type, row.name, row.Name, row.JSONKey, row.kingdomName, row.comment2]) {
    if (typeof value !== 'string' || value.trim() === '') continue;
    const translated = translations[`${value}_name`] ?? translations[value];
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
): Record<number, MetadataItem> {
	const result: Record<number, MetadataItem> = {};
	for (const row of rows) {
		const id = positiveID(row[idField]);
		if (id === 0) continue;
		const internalName = [row.name, row.Name, row.kingdomName, row.assetName]
			.find((value): value is string => typeof value === 'string' && value.trim() !== '');
		result[id] = {
			...row,
			id,
			internalName,
			name: displayName(row, translations, internalName ? splitIdentifier(internalName) : `${fallbackPrefix} ${id}`),
			image: typeof row.assetName === 'string' && row.assetName.trim()
				? `/game-data/resources/images/${row.assetName}.webp`
				: undefined,
		};
	}
	return result;
}

function splitIdentifier(value: string): string {
	return value
		.replace(/[_-]+/g, ' ')
		.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
		.trim();
}
