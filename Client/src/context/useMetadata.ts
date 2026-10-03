import { createContext, useContext } from 'react';

export interface MetadataItem {
  id: number;
  name: string;
  nameLocale?: string;
  localizationKey?: string;
  translationStatus?: 'official' | 'fallback';
  image?: string;
  level?: number;
  outputAmount?: number;
  [key: string]: unknown;
}

export interface MetadataContextValue {
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
  effectsStatus: 'loading' | 'ready' | 'unavailable';
  getTroop: (id: number) => MetadataItem | undefined;
  getTool: (id: number) => MetadataItem | undefined;
	getBuilding: (id: number) => MetadataItem | undefined;
	getEquipment: (id: number) => MetadataItem | undefined;
	getGem: (id: number) => MetadataItem | undefined;
	getEffect: (id: number) => MetadataItem | undefined;
	getCraftingRecipe: (id: number) => MetadataItem | undefined;
  getDecoration: (id: number) => MetadataItem | undefined;
}

export const MetadataContext = createContext<MetadataContextValue | undefined>(undefined);

export function useMetadata(): MetadataContextValue {
  const context = useContext(MetadataContext);
  if (!context) throw new Error('useMetadata must be used within MetadataProvider');
  return context;
}
