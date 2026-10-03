import { createContext, useContext } from 'react';
import type {
	CatalogManifest,
	ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
	GameStateV2,
	IntentReceipt,
} from '../../src/api/Contracts';

export interface FixtureAPIContextValue {
	state: GameStateV2;
	catalogs: CatalogManifest;
	configuration: ConfigurationSnapshot;
	optimizeEquipment: (input: EquipmentOptimizeRequest) => Promise<EquipmentOptimizeResponse>;
	submitIntent: (name: string, args?: Record<string, unknown>) => Promise<IntentReceipt>;
	updateConfiguration: (section: string, value: unknown) => Promise<ConfigurationSnapshot>;
}

export const FixtureAPIContext = createContext<FixtureAPIContextValue | null>(null);


export function useCitadelAPI(): FixtureAPIContextValue {
	const context = useContext(FixtureAPIContext);
	if (!context) throw new Error('CIT-7 fixture API provider is missing');
	return context;
}
