import { createContext, useContext, useMemo, type ReactNode } from 'react';
import type {
	CatalogManifest,
	ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
	GameStateV2,
	IntentReceipt,
} from '../../src/api/Contracts';

export type FixtureRequestMode = 'normal' | 'few' | 'no-gear' | 'error' | 'delayed' | 'timeout';
export type FixtureApplyMode = 'success' | 'authoritative-failure' | 'stale-rejection';

interface FixtureAPIContextValue {
	state: GameStateV2;
	catalogs: CatalogManifest;
	configuration: ConfigurationSnapshot;
	optimizeEquipment: (input: EquipmentOptimizeRequest) => Promise<EquipmentOptimizeResponse>;
	submitIntent: (name: string, args?: Record<string, unknown>) => Promise<IntentReceipt>;
	updateConfiguration: (section: string, value: unknown) => Promise<ConfigurationSnapshot>;
}

interface FixtureAPIProviderProps {
	children: ReactNode;
	state: GameStateV2;
	catalogs: CatalogManifest;
	configuration: ConfigurationSnapshot;
	requestMode: FixtureRequestMode;
	applyMode: FixtureApplyMode;
	onConfiguration: (next: ConfigurationSnapshot, section: string, value: unknown) => void;
	onApply: (name: string, args: Record<string, unknown>, outcome: FixtureApplyMode) => void;
}

const FixtureAPIContext = createContext<FixtureAPIContextValue | null>(null);

export function FixtureAPIProvider({
	children,
	state,
	catalogs,
	configuration,
	requestMode,
	applyMode,
	onConfiguration,
	onApply,
}: FixtureAPIProviderProps) {
	const value = useMemo<FixtureAPIContextValue>(() => ({
		state,
		catalogs,
		configuration,
		async optimizeEquipment(input) {
			const requestID = crypto.randomUUID();
			const startedAt = performance.now();
			window.dispatchEvent(new CustomEvent('cit6-optimize-start', { detail: { requestID, startedAt, input } }));
			try {
				const response = await fetch(`/api/v2/equipment/optimize?scenario=${encodeURIComponent(requestMode)}`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CIT6-Browser-Fixture': '1' },
					body: JSON.stringify(input),
				});
				const payload = await response.json().catch(() => null) as EquipmentOptimizeResponse | { error?: { message?: string } } | null;
				if (!response.ok) {
					const message = payload && 'error' in payload ? payload.error?.message : '';
					throw new Error(message || `Equipment optimizer returned HTTP ${response.status}`);
				}
				const result = payload as EquipmentOptimizeResponse;
				window.dispatchEvent(new CustomEvent('cit6-optimize-http', {
					detail: { requestID, startedAt, httpFinishedAt: performance.now(), alternatives: result.alternatives.length },
				}));
				return result;
			} catch (error) {
				window.dispatchEvent(new CustomEvent('cit6-optimize-error', { detail: { requestID } }));
				throw error;
			}
		},
		async submitIntent(name, args = {}) {
			onApply(name, args, applyMode);
			await new Promise((resolve) => window.setTimeout(resolve, 120));
			if (applyMode === 'authoritative-failure') {
				throw new Error('Synthetic authoritative verification failed after ggm/gei/gli refresh');
			}
			if (applyMode === 'stale-rejection') {
				throw new Error('Synthetic server rejected stale snapshot fingerprint');
			}
			return {
				id: crypto.randomUUID(), intent: name, actor: 'cit6-fixture', priority: 0,
				status: 'succeeded', phase: 'completed', submittedAt: new Date().toISOString(), completedAt: new Date().toISOString(),
			} as IntentReceipt;
		},
		async updateConfiguration(section, sectionValue) {
			const next = {
				...configuration,
				revision: configuration.revision + 1,
				updatedAt: new Date().toISOString(),
				sections: { ...configuration.sections, [section]: sectionValue },
			};
			onConfiguration(next, section, sectionValue);
			return next;
		},
	}), [applyMode, catalogs, configuration, onApply, onConfiguration, requestMode, state]);

	return <FixtureAPIContext.Provider value={value}>{children}</FixtureAPIContext.Provider>;
}

export function useCitadelAPI(): FixtureAPIContextValue {
	const context = useContext(FixtureAPIContext);
	if (!context) throw new Error('CIT-7 fixture API provider is missing');
	return context;
}
