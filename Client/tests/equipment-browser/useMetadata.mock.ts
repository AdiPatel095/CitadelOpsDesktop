import { createContext, useContext } from 'react';

export interface MetadataItem {
	id: number;
	name: string;
	image?: string;
	level?: number;
	[key: string]: unknown;
}

export interface FixtureMetadata {
	effects: Record<number, MetadataItem>;
	equipments: Record<number, MetadataItem>;
	gems: Record<number, MetadataItem>;
}

export interface FixtureMetadataValue extends FixtureMetadata {
	isLoading: boolean;
	effectsStatus: 'ready';
	getEffect: (id: number) => MetadataItem | undefined;
	getEquipment: (id: number) => MetadataItem | undefined;
	getGem: (id: number) => MetadataItem | undefined;
}

export const FixtureMetadataContext = createContext<FixtureMetadataValue | null>(null);

export function useMetadata(): FixtureMetadataValue {
	const context = useContext(FixtureMetadataContext);
	if (!context) throw new Error('CIT-7 fixture metadata provider is missing');
	return context;
}
