import { StrictMode, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { createRoot } from 'react-dom/client';
import EquipmentOptimizer from '../../src/equipment/components/EquipmentOptimizer';
import type { ConfigurationSnapshot, GameStateV2 } from '../../src/api/Contracts';
import './fixture.css';
import { FixtureAPIProvider, type FixtureApplyMode, type FixtureRequestMode } from './api-context.mock';
import { FixtureMetadataProvider } from './metadata-context.mock';
import {
	fixtureCatalogs,
	fixtureConfiguration,
	fixtureLeader,
	fixtureMetadata,
	fixtureState,
	type FixtureLeaderKind,
	type MigrationSeed,
} from './fixture-data';

interface TimingSample {
	request: number;
	totalMs: number;
	httpMs: number;
	alternatives: number;
}

interface OptimizeStartDetail { requestID: string; startedAt: number }
interface OptimizeHTTPDetail extends OptimizeStartDetail { httpFinishedAt: number; alternatives: number; clickKind: 'preview' | 'regenerate'; measurable: boolean }

function App() {
	const [leaderKind, setLeaderKind] = useState<FixtureLeaderKind>('commander');
	const [requestMode, setRequestMode] = useState<FixtureRequestMode>('normal');
	const [applyMode, setApplyMode] = useState<FixtureApplyMode>('success');
	const [migrationSeed, setMigrationSeed] = useState<MigrationSeed>('v4');
	const [gameState, setGameState] = useState(() => fixtureState('commander', 'normal'));
	const [configuration, setConfiguration] = useState(() => fixtureConfiguration('v4', 'commander'));
	const [candidateChurn, setCandidateChurn] = useState(false);
	const [catalogExpanded, setCatalogExpanded] = useState(false);
	const [open, setOpen] = useState(true);
	const [mountKey, setMountKey] = useState(0);
	const [requestCount, setRequestCount] = useState(0);
	const [samples, setSamples] = useState<TimingSample[]>([]);
	const [lastPersist, setLastPersist] = useState('none');
	const [lastApply, setLastApply] = useState('none');
	const [seriesRunning, setSeriesRunning] = useState(false);
	const pending = useRef(new Map<string, OptimizeHTTPDetail>());
	const completed = useRef(new Set<string>());
	const lastOptimizeClick = useRef<{ at: number; kind: 'preview' | 'regenerate'; existingAlternatives: number } | null>(null);

	const inventoryMode = requestMode === 'few' || requestMode === 'no-gear' ? requestMode : 'normal';
	const changeLeader = (nextKind: FixtureLeaderKind) => {
		// Keep leader props and their backing state in the same React event batch.
		setLeaderKind(nextKind);
		setGameState(fixtureState(nextKind, inventoryMode));
		setConfiguration(fixtureConfiguration(migrationSeed, nextKind));
		setMountKey((value) => value + 1);
	};
	useEffect(() => {
		setGameState(fixtureState(leaderKind, inventoryMode));
		setConfiguration(fixtureConfiguration(migrationSeed, leaderKind));
		setMountKey((value) => value + 1);
	}, [inventoryMode, leaderKind, migrationSeed]);

	useEffect(() => {
		const captureClick = (event: MouseEvent) => {
			const button = event.target instanceof Element ? event.target.closest('button') : null;
			const text = button?.textContent?.trim() ?? '';
			if (text.includes('Preview Reconfiguration')) {
				lastOptimizeClick.current = { at: performance.now(), kind: 'preview', existingAlternatives: rankedAlternativeButtons().length };
			} else if (text.includes('Regenerate')) {
				lastOptimizeClick.current = { at: performance.now(), kind: 'regenerate', existingAlternatives: rankedAlternativeButtons().length };
			}
		};
		const onStart = (event: Event) => {
			const detail = (event as CustomEvent<OptimizeStartDetail>).detail;
			setRequestCount((value) => value + 1);
			pending.current.clear();
			const click = lastOptimizeClick.current;
			const captured = click && detail.startedAt - click.at < 250 ? click : { at: detail.startedAt, kind: 'preview' as const, existingAlternatives: 0 };
			pending.current.set(detail.requestID, {
				...detail,
				startedAt: captured.at,
				httpFinishedAt: 0,
				alternatives: 0,
				clickKind: captured.kind,
				measurable: captured.kind === 'preview' && captured.existingAlternatives === 0,
			});
			window.setTimeout(() => pending.current.delete(detail.requestID), 12_000);
		};
		const onHTTP = (event: Event) => {
			const detail = (event as CustomEvent<OptimizeHTTPDetail>).detail;
			const current = pending.current.get(detail.requestID);
			if (current) pending.current.set(detail.requestID, { ...current, httpFinishedAt: detail.httpFinishedAt, alternatives: detail.alternatives });
		};
		const onError = (event: Event) => pending.current.delete((event as CustomEvent<{ requestID: string }>).detail.requestID);
		document.addEventListener('click', captureClick, true);
		window.addEventListener('cit6-optimize-start', onStart);
		window.addEventListener('cit6-optimize-http', onHTTP);
		window.addEventListener('cit6-optimize-error', onError);
		const observer = new MutationObserver(() => {
			for (const [requestID, detail] of pending.current) {
				if (!detail.measurable || !detail.httpFinishedAt || completed.current.has(requestID)) continue;
				const rendered = rankedAlternativeButtons().length;
				if (rendered < detail.alternatives) continue;
				completed.current.add(requestID);
				requestAnimationFrame(() => requestAnimationFrame(() => {
					setSamples((current) => [...current, {
						request: current.length + 1,
						totalMs: performance.now() - detail.startedAt,
						httpMs: detail.httpFinishedAt - detail.startedAt,
						alternatives: detail.alternatives,
					}]);
				}));
			}
		});
		observer.observe(document.body, { childList: true, subtree: true, characterData: true });
		return () => {
			observer.disconnect();
			document.removeEventListener('click', captureClick, true);
			window.removeEventListener('cit6-optimize-start', onStart);
			window.removeEventListener('cit6-optimize-http', onHTTP);
			window.removeEventListener('cit6-optimize-error', onError);
		};
	}, []);

	const metadata = useMemo(() => fixtureMetadata(gameState, catalogExpanded), [catalogExpanded, gameState]);
	const catalogs = useMemo(() => fixtureCatalogs(catalogExpanded ? 'fixture-digest-expanded' : 'fixture-digest-a'), [catalogExpanded]);
	const leader = useMemo(() => fixtureLeader(gameState, leaderKind), [gameState, leaderKind]);
	const candidates = candidateChurn ? [9001, 9003, 9004, 9005, 9006, 9007, 9008, 9011] : [9001, 9002, 9003, 9004, 9005, 9006, 9007, 9008];

	const reset = useCallback(() => {
		localStorage.clear();
		setGameState(fixtureState(leaderKind, inventoryMode));
		setConfiguration(fixtureConfiguration(migrationSeed, leaderKind));
		setCandidateChurn(false);
		setCatalogExpanded(false);
		setSamples([]);
		setRequestCount(0);
		pending.current.clear();
		completed.current.clear();
		setLastApply('none');
		setLastPersist('none');
		setOpen(true);
		setMountKey((value) => value + 1);
	}, [inventoryMode, leaderKind, migrationSeed]);

	const runWarmSeries = async () => {
		if (requestMode !== 'normal' || seriesRunning) return;
		setSeriesRunning(true);
		setSamples([]);
		try {
			setOpen(true);
			await waitFor(() => findButton('Configure PvP') ?? findButton('Preview Reconfiguration'));
			findButton('Configure PvP')?.click();
			for (let index = 0; index < 5; index += 1) {
				const previewButton = await waitFor(() => findButton('Preview Reconfiguration'));
				previewButton.click();
				await waitFor(() => rankedAlternativeButtons().length === 10 ? true : null, 10_000);
				await nextPaint();
				const previewDialog = [...document.querySelectorAll<HTMLElement>('[role="dialog"]')]
					.find((dialog) => dialog.textContent?.includes('Reconfiguration Preview'));
				const cancel = previewDialog ? [...previewDialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent?.trim() === 'Cancel') : null;
				cancel?.click();
				await waitFor(() => rankedAlternativeButtons().length === 0 ? true : null);
			}
		} finally {
			setSeriesRunning(false);
		}
	};

	const mutateBackground = () => setGameState((current) => ({
		...current,
		revision: current.revision + 1,
		updatedAt: new Date().toISOString(),
		player: { ...current.player, level: (current.player.level ?? 0) + 1 },
	}));
	const mutateEquipment = () => setGameState((current) => {
		const next = structuredClone(current);
		const item = Object.values(next.inventory.equipment)[0];
		if (item?.effects[0]) item.effects[0].values[0] = (item.effects[0].values[0] ?? 0) + 1;
		next.revision += 1;
		return next;
	});
	const addOffModeSocket = () => setGameState((current) => {
		const next = structuredClone(current);
		const carrier = Object.values(next.inventory.equipment).find((item) => item.slot === 1);
		if (!carrier) return current;
		next.inventory.gems['99001'] = {
			id: 99001, definitionId: 99001, compatibleWearerId: leaderKind === 'commander' ? 2 : 1,
			combatMode: 'pve', equipmentInstanceId: carrier.id, level: 9,
			effects: [{ wireId: 201, definitionId: 9001, values: [17] }],
		};
		next.revision += 1;
		return next;
	});

	const latest = samples.at(-1);
	const warmP95 = percentile(samples.map((sample) => sample.totalMs), 0.95);
	return (
		<FixtureAPIProvider
			state={gameState}
			catalogs={catalogs}
			configuration={configuration}
			requestMode={requestMode}
			applyMode={applyMode}
			onConfiguration={(next, section, value) => {
				setConfiguration(next);
				setLastPersist(`${section} = ${JSON.stringify(value)}`);
			}}
			onApply={(name, args, outcome) => setLastApply(`${name} (${outcome}) ${JSON.stringify(args)}`)}
		>
			<FixtureMetadataProvider metadata={metadata}>
				<main className="fixture-stage">
					<section>
						<p className="fixture-kicker">CIT-6 isolated regression fixture</p>
						<h1>Production EquipmentOptimizer + production HTTP solver</h1>
						<p>Synthetic state only. No login, broker, game transport, credentials, or live mutation is present.</p>
						<button type="button" className="fixture-open" onClick={() => setOpen(true)}>Open optimizer</button>
					</section>
				</main>
				{createPortal(<aside className="fixture-controls" aria-label="CIT-6 fixture controls" data-testid="fixture-controls">
					<h2>CIT-6 controls</h2>
					<label>Leader<select value={leaderKind} onChange={(event) => changeLeader(event.target.value as FixtureLeaderKind)}><option value="commander">Commander · 334 gear · 16 gems</option><option value="castellan">Castellan · 253 gear · 0 PvP gems</option></select></label>
					<label>Request<select value={requestMode} onChange={(event) => setRequestMode(event.target.value as FixtureRequestMode)}><option value="normal">Normal full inventory</option><option value="few">Few alternatives</option><option value="no-gear">No eligible gear</option><option value="error">HTTP error</option><option value="delayed">1.5s delayed success</option><option value="timeout">9s timeout</option></select></label>
					<label>Apply outcome<select value={applyMode} onChange={(event) => setApplyMode(event.target.value as FixtureApplyMode)}><option value="success">Terminal success</option><option value="authoritative-failure">Authoritative failure</option><option value="stale-rejection">Server stale rejection</option></select></label>
					<label>Stored profile<select value={migrationSeed} onChange={(event) => setMigrationSeed(event.target.value as MigrationSeed)}><option value="empty">No profile</option><option value="v1">v1 effect IDs</option><option value="v2">v2 official groups</option><option value="v3">v3 effect types</option><option value="v4">v4 current</option></select></label>
					<div className="fixture-buttons">
						<button type="button" onClick={mutateBackground}>Background update</button>
						<button type="button" onClick={() => setCandidateChurn((value) => !value)}>Candidate ID churn</button>
						<button type="button" onClick={mutateEquipment}>Relevant gear change</button>
						<button type="button" onClick={addOffModeSocket}>Off-mode socket change</button>
						<button type="button" onClick={() => setCatalogExpanded((value) => !value)}>Catalog add + digest</button>
						<button type="button" onClick={reset}>Reset fixture</button>
					</div>
					<button type="button" className="fixture-series" onClick={() => void runWarmSeries()} disabled={requestMode !== 'normal' || seriesRunning}>{seriesRunning ? 'Running…' : 'Run 5 warm full-path samples'}</button>
					<dl className="fixture-metrics" data-testid="fixture-measurements">
						<div><dt>HTTP requests</dt><dd>{requestCount}</dd></div>
						<div><dt>Last click → rendered</dt><dd>{latest ? `${latest.totalMs.toFixed(1)} ms / ${latest.alternatives}` : 'none'}</dd></div>
						<div><dt>Warm p95</dt><dd>{warmP95 == null ? 'none' : `${warmP95.toFixed(1)} ms`}</dd></div>
						<div><dt>Samples</dt><dd>{samples.map((sample) => sample.totalMs.toFixed(0)).join(', ') || 'none'}</dd></div>
					</dl>
					<details><summary>Last persisted profile</summary><pre data-testid="last-persist">{lastPersist}</pre></details>
					<details open={lastApply !== 'none'}><summary>Last apply receipt</summary><pre data-testid="last-apply">{lastApply}</pre></details>
					<p className="fixture-state">candidate set: {candidateChurn ? 'churned, same semantic groups' : 'baseline'} · catalog: {catalogExpanded ? 'expanded/stale' : 'baseline'} · rev {gameState.revision}</p>
				</aside>, document.body)}
				<EquipmentOptimizer
					key={mountKey}
					isOpen={open}
					onClose={() => setOpen(false)}
					leader={leader}
					candidateEffectIDsByMode={{ PvP: candidates, PvE: candidates }}
					disabled={false}
				/>
			</FixtureMetadataProvider>
		</FixtureAPIProvider>
	);
}

function findButton(text: string): HTMLButtonElement | null {
	return [...document.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent?.includes(text)) ?? null;
}

function rankedAlternativeButtons(): HTMLButtonElement[] {
	return [...document.querySelectorAll<HTMLButtonElement>('button')].filter((button) => /^#\d+/.test(button.textContent?.trim() ?? ''));
}

async function waitFor<T>(read: () => T | null, timeoutMs = 3_000): Promise<T> {
	const deadline = performance.now() + timeoutMs;
	while (performance.now() < deadline) {
		const value = read();
		if (value) return value;
		await new Promise((resolve) => window.setTimeout(resolve, 25));
	}
	throw new Error(`Fixture wait expired after ${timeoutMs}ms`);
}

function nextPaint(): Promise<void> {
	return new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
}

function percentile(values: number[], quantile: number): number | null {
	if (values.length === 0) return null;
	const sorted = [...values].sort((left, right) => left - right);
	return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * quantile))]!;
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>);
