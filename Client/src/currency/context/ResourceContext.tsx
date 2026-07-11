import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import type { PlayerGlobalResources } from '../../types/PlayerGlobalResources';

interface ResourceContextType {
	resources: PlayerGlobalResources | null;
}

const ResourceContext = createContext<ResourceContextType | undefined>(undefined);

export function useResources(): ResourceContextType {
	const context = useContext(ResourceContext);
	if (!context) throw new Error('useResources must be used within a ResourceProvider');
	return context;
}

export function ResourceProvider({ children }: { children: ReactNode }) {
	const { state } = useCitadelAPI();
	const { resources: definitions } = useMetadata();
	const resources = useMemo((): PlayerGlobalResources | null => {
		if (!state) return null;
		let coins = 0;
		let rubies = 0;
		for (const [rawID, amount] of Object.entries(state.player.resources)) {
			const internalName = definitions[Number(rawID)]?.internalName;
			if (internalName === 'currency1') coins = amount;
			if (internalName === 'currency2') rubies = amount;
		}
		return {
			coins,
			rubies,
			might_pt: state.player.might ?? 0,
			glory_pt: state.player.glory ?? 0,
			gallan_pt: state.player.gallantry ?? 0,
		};
	}, [definitions, state]);
	return <ResourceContext.Provider value={{ resources }}>{children}</ResourceContext.Provider>;
}
