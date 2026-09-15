import { createContext, useContext, useMemo, type ReactNode } from 'react';

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

interface FixtureMetadataValue extends FixtureMetadata {
	isLoading: boolean;
	getEffect: (id: number) => MetadataItem | undefined;
	getEquipment: (id: number) => MetadataItem | undefined;
	getGem: (id: number) => MetadataItem | undefined;
}

const FixtureMetadataContext = createContext<FixtureMetadataValue | null>(null);

export function FixtureMetadataProvider({ children, metadata }: { children: ReactNode; metadata: FixtureMetadata }) {
	const value = useMemo<FixtureMetadataValue>(() => ({
		...metadata,
		isLoading: false,
		getEffect: (id) => metadata.effects[id],
		getEquipment: (id) => metadata.equipments[id],
		getGem: (id) => metadata.gems[id],
	}), [metadata]);
	return <FixtureMetadataContext.Provider value={value}>{children}</FixtureMetadataContext.Provider>;
}

export function useMetadata(): FixtureMetadataValue {
	const context = useContext(FixtureMetadataContext);
	if (!context) throw new Error('CIT-6 fixture metadata provider is missing');
	return context;
}
