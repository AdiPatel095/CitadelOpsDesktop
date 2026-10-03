import { FixtureMetadataContext, type FixtureMetadata, type FixtureMetadataValue } from './useMetadata.mock';
export type { MetadataItem, FixtureMetadata } from './useMetadata.mock';
import { useMemo, type ReactNode } from 'react';


export function FixtureMetadataProvider({ children, metadata }: { children: ReactNode; metadata: FixtureMetadata }) {
	const value = useMemo<FixtureMetadataValue>(() => ({
		...metadata,
		isLoading: false,
		effectsStatus: 'ready',
		getEffect: (id) => metadata.effects[id],
		getEquipment: (id) => metadata.equipments[id],
		getGem: (id) => metadata.gems[id],
	}), [metadata]);
	return <FixtureMetadataContext.Provider value={value}>{children}</FixtureMetadataContext.Provider>;
}
