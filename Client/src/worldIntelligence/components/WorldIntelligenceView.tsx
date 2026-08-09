import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import {
	Activity,
	ArrowDown,
	ChevronLeft,
	ChevronRight,
	Cloud,
	CloudOff,
	Database,
	Globe2,
	RefreshCw,
	Search,
	UserRound,
	Users,
	X,
} from 'lucide-react';
import { CitadelAPI } from '../../api/CitadelClient';
import type {
	WorldIntelligenceAllianceProfileV1,
	WorldIntelligenceCatalogDatasetCatalogV1,
	WorldIntelligenceCatalogDatasetV1,
	WorldIntelligenceCoverageResponseV1,
	WorldIntelligencePlayerObservationV1,
	WorldIntelligencePlayerProfileV1,
	WorldIntelligenceRankingEntryV1,
	WorldIntelligenceRankingMetricV1,
	WorldIntelligenceRankingResponseV1,
	WorldIntelligenceSearchResultV1,
	WorldIntelligenceStatusV1,
} from '../../api/Contracts';
import {
	Badge,
	Button,
	Card,
	CardContent,
	EmptyState,
	Input,
	MetricTile,
	PageHeader,
	PillSelector,
	Select,
	SectionCard,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import DetailBackButton from '../../components/DetailBackButton';
import WorldPlayerDetailView from './WorldPlayerDetailView';

type RankingType = 'players' | 'alliances';
type SelectedEntity = { type: 'player' | 'alliance'; id: number; worldId: string };
type IntelligenceTableRow = Omit<WorldIntelligenceRankingEntryV1, 'rank' | 'value'> & {
	rank?: number;
	value?: number;
	publicProfile?: WorldIntelligencePlayerObservationV1['publicProfile'];
	publicMetrics?: WorldIntelligencePlayerObservationV1['publicMetrics'];
};

const tablePageSize = 25;

const WorldIntelligenceView = () => {
	const { state } = useCitadelAPI();
	const worldId = state?.account.worldId || state?.session.serverUrl || '';
	const [status, setStatus] = useState<WorldIntelligenceStatusV1 | null>(null);
	const [catalog, setCatalog] = useState<WorldIntelligenceCatalogDatasetCatalogV1>({ source: 'ggs-official-items', datasets: [] });
	const [catalogDatasetKey, setCatalogDatasetKey] = useState('islandrewardranks');
	const [catalogDataset, setCatalogDataset] = useState<WorldIntelligenceCatalogDatasetV1 | null>(null);
	const [catalogLoading, setCatalogLoading] = useState(false);
	const [catalogPage, setCatalogPage] = useState(0);
	const [coverage, setCoverage] = useState<WorldIntelligenceCoverageResponseV1>({ worlds: [] });
	const [query, setQuery] = useState('');
	const [appliedQuery, setAppliedQuery] = useState('');
	const [searchResults, setSearchResults] = useState<WorldIntelligenceSearchResultV1[]>([]);
	const [searching, setSearching] = useState(false);
	const [rankingType, setRankingType] = useState<RankingType>('players');
	const [rankingMetric, setRankingMetric] = useState('might');
	const [rankingMetricCatalog, setRankingMetricCatalog] = useState<Record<RankingType, WorldIntelligenceRankingMetricV1[]>>({ players: [], alliances: [] });
	const [ranking, setRanking] = useState<WorldIntelligenceRankingResponseV1 | null>(null);
	const [rankingLoading, setRankingLoading] = useState(false);
	const [tablePage, setTablePage] = useState(0);
	const [selected, setSelected] = useState<SelectedEntity | null>(null);
	const [playerProfile, setPlayerProfile] = useState<WorldIntelligencePlayerProfileV1 | null>(null);
	const [allianceProfile, setAllianceProfile] = useState<WorldIntelligenceAllianceProfileV1 | null>(null);
	const [profileLoading, setProfileLoading] = useState(false);
	const [error, setError] = useState('');
	const directoryScrollRef = useRef(0);

	const refreshStatus = useCallback(async () => {
		try {
			setStatus(await CitadelAPI.getWorldIntelligenceStatus());
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not read World Intelligence status.'));
		}
	}, []);

	const refreshCatalog = useCallback(async () => {
		try {
			const result = await CitadelAPI.getWorldIntelligenceCatalogDatasets();
			setCatalog(result);
			setCatalogDatasetKey((current) => {
				if (result.datasets.some((dataset) => dataset.datasetKey === current)) return current;
				return result.datasets.find((dataset) => dataset.datasetKey === 'islandrewardranks')?.datasetKey
					?? result.datasets[0]?.datasetKey
					?? '';
			});
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load the official ranking catalog.'));
		}
	}, []);

	const refreshCatalogDataset = useCallback(async () => {
		if (!catalogDatasetKey) {
			setCatalogDataset(null);
			return;
		}
		setCatalogLoading(true);
		try {
			setCatalogDataset(await CitadelAPI.getWorldIntelligenceCatalogDataset(catalogDatasetKey, 50));
			setCatalogPage(0);
		} catch (requestError) {
			setCatalogDataset(null);
			setError(errorMessage(requestError, 'Could not load this official catalog dataset.'));
		} finally {
			setCatalogLoading(false);
		}
	}, [catalogDatasetKey]);

	const refreshCoverage = useCallback(async () => {
		if (!worldId) {
			setCoverage({ worlds: [] });
			return;
		}
		try {
			setCoverage(await CitadelAPI.getWorldIntelligenceCoverage(worldId));
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load cloud coverage.'));
		}
	}, [worldId]);

	const refreshRankingMetrics = useCallback(async () => {
		if (!worldId) {
			setRankingMetricCatalog({ players: [], alliances: [] });
			return;
		}
		try {
			const [players, alliances] = await Promise.all([
				CitadelAPI.getWorldIntelligenceRankingMetrics(worldId, 'players'),
				CitadelAPI.getWorldIntelligenceRankingMetrics(worldId, 'alliances'),
			]);
			setRankingMetricCatalog({ players: players.metrics ?? [], alliances: alliances.metrics ?? [] });
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load the public ranking catalog.'));
		}
	}, [worldId]);

	const refreshRanking = useCallback(async () => {
		if (!worldId) {
			setRanking(null);
			return;
		}
		setRankingLoading(true);
		try {
			const result = await CitadelAPI.getWorldIntelligenceRankings({
				worldId,
				type: rankingType,
				metric: rankingMetric,
				limit: 250,
			});
			setRanking(result);
		} catch (requestError) {
			setRanking(null);
			setError(errorMessage(requestError, 'Could not load cloud rankings.'));
		} finally {
			setRankingLoading(false);
		}
	}, [rankingMetric, rankingType, worldId]);

	useEffect(() => {
		void refreshStatus();
		const interval = window.setInterval(() => void refreshStatus(), 30_000);
		return () => window.clearInterval(interval);
	}, [refreshStatus]);

	useEffect(() => {
		void refreshCatalog();
	}, [refreshCatalog]);

	useEffect(() => {
		void refreshCatalogDataset();
	}, [refreshCatalogDataset]);

	useEffect(() => {
		void refreshCoverage();
	}, [refreshCoverage]);

	useEffect(() => {
		void refreshRankingMetrics();
	}, [refreshRankingMetrics]);

	useEffect(() => {
		void refreshRanking();
	}, [refreshRanking]);

	const submitSearch = async (event?: FormEvent) => {
		event?.preventDefault();
		if (!worldId) return;
		const normalizedQuery = query.trim();
		if (!normalizedQuery) {
			setAppliedQuery('');
			setSearchResults([]);
			setTablePage(0);
			return;
		}
		setSearching(true);
		setError('');
		try {
			const response = await CitadelAPI.searchWorldIntelligence({
				worldId,
				query: normalizedQuery,
				type: rankingType === 'players' ? 'player' : 'alliance',
				limit: 100,
			});
			setAppliedQuery(normalizedQuery);
			setSearchResults(response.results ?? []);
			setTablePage(0);
		} catch (requestError) {
			setSearchResults([]);
			setError(errorMessage(requestError, 'Search failed.'));
		} finally {
			setSearching(false);
		}
	};

	const clearSearch = () => {
		setQuery('');
		setAppliedQuery('');
		setSearchResults([]);
		setTablePage(0);
	};

	const selectRankingType = (value: string) => {
		setRankingType(value as RankingType);
		setRankingMetric('might');
		clearSearch();
		setSelected(null);
	};

	const selectRankingMetric = (metric: string) => {
		setRankingMetric(metric);
		setTablePage(0);
	};

	const openEntity = useCallback(async (entity: SelectedEntity) => {
		if (!selected) directoryScrollRef.current = window.scrollY;
		setSelected(entity);
		setPlayerProfile(null);
		setAllianceProfile(null);
		setProfileLoading(true);
		setError('');
		window.scrollTo({ top: 0, behavior: 'smooth' });
		try {
			if (entity.type === 'player') {
				setPlayerProfile(await CitadelAPI.getWorldIntelligencePlayer(entity.worldId, entity.id, 1_000));
			} else {
				setAllianceProfile(await CitadelAPI.getWorldIntelligenceAlliance(entity.worldId, entity.id));
			}
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load this profile.'));
		} finally {
			setProfileLoading(false);
		}
	}, [selected]);

	const closeProfile = () => {
		const directoryScroll = directoryScrollRef.current;
		setSelected(null);
		setPlayerProfile(null);
		setAllianceProfile(null);
		setError('');
		window.requestAnimationFrame(() => window.scrollTo({ top: directoryScroll }));
	};

	const currentCoverage = coverage.worlds[0];
	const catalogOptions = useMemo(() => catalog.datasets.map((dataset) => ({
		value: dataset.datasetKey,
		label: `${humanizeCatalogKey(dataset.category)} · ${dataset.datasetLabel}`,
	})), [catalog.datasets]);
	const playerRankingOptions = useMemo(
		() => rankingOptions(rankingMetricCatalog.players),
		[rankingMetricCatalog.players],
	);
	const allianceRankingOptions = useMemo(
		() => rankingOptions(rankingMetricCatalog.alliances),
		[rankingMetricCatalog.alliances],
	);
	const currentRankingOptions = rankingType === 'players' ? playerRankingOptions : allianceRankingOptions;
	const currentRankingLabel = currentRankingOptions.find((option) => option.value === rankingMetric)?.label ?? rankingMetric;
	const currentRankingDefinition = rankingMetricCatalog[rankingType].find((metric) => metric.metric === rankingMetric);
	useEffect(() => {
		if (currentRankingOptions.some((option) => option.value === rankingMetric)) return;
		setRankingMetric(currentRankingOptions[0]?.value ?? 'might');
		setTablePage(0);
	}, [currentRankingOptions, rankingMetric]);
	const rankedEntries = ranking?.entries ?? [];
	const tableRows = useMemo<IntelligenceTableRow[]>(() => {
		if (!appliedQuery) return rankedEntries;
		const entityType = rankingType === 'players' ? 'player' : 'alliance';
		return searchResults
			.filter((result) => result.type === entityType)
			.map((result) => ({ ...result }))
			.sort((left, right) => {
				const difference = tableMetricValue(right, rankingMetric) - tableMetricValue(left, rankingMetric);
				return difference || left.name.localeCompare(right.name);
			});
	}, [appliedQuery, rankedEntries, rankingMetric, rankingType, searchResults]);
	const pageCount = Math.max(1, Math.ceil(tableRows.length / tablePageSize));
	const visibleRows = tableRows.slice(tablePage * tablePageSize, (tablePage + 1) * tablePageSize);
	const featureReady = Boolean(worldId);

	if (selected) {
		return (
			<div className="flex flex-col gap-6 pb-8">
				<nav aria-label="World Intelligence detail navigation" className="sticky top-3 z-30 self-start">
					<DetailBackButton label="Back to World Intelligence" onClick={closeProfile} className="shadow-lg backdrop-blur" />
				</nav>
				{error && (
					<div className="flex items-start justify-between gap-3 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error" role="alert">
						<span>{error}</span>
						<button type="button" aria-label="Dismiss error" onClick={() => setError('')}><X className="h-4 w-4" /></button>
					</div>
				)}
				{profileLoading ? (
					<>
						<PageHeader
							eyebrow="World Intelligence dossier"
							title={selected.type === 'player' ? 'Loading player…' : 'Loading alliance…'}
							description={`${displayWorld(selected.worldId)} · public ID ${selected.id}`}
							icon={selected.type === 'player' ? <UserRound className="h-6 w-6" /> : <Users className="h-6 w-6" />}
						/>
						<Card><CardContent className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading public history…</CardContent></Card>
					</>
				) : playerProfile ? (
					<WorldPlayerDetailView
						profile={playerProfile}
						onOpenAlliance={(allianceId) => void openEntity({ type: 'alliance', id: allianceId, worldId: playerProfile.current.worldId })}
					/>
				) : allianceProfile ? (
					<>
						<PageHeader
							eyebrow="World Intelligence alliance"
							title={allianceProfile.current.name}
							description={`${displayWorld(allianceProfile.current.worldId)} · Alliance ${allianceProfile.current.allianceId}`}
							icon={<Users className="h-6 w-6" />}
							meta={<Badge variant="outline">Observed {relativeTime(allianceProfile.current.observedAt)}</Badge>}
						/>
						<AllianceProfile profile={allianceProfile} onOpenPlayer={(player) => void openEntity({ type: 'player', id: player.playerId, worldId: player.worldId })} />
					</>
				) : (
					<>
						<PageHeader
							eyebrow="World Intelligence dossier"
							title="Profile unavailable"
							description={`${displayWorld(selected.worldId)} · public ID ${selected.id}`}
							icon={selected.type === 'player' ? <UserRound className="h-6 w-6" /> : <Users className="h-6 w-6" />}
						/>
						<EmptyState size="lg" title="Profile unavailable" description="No usable public observations were returned for this entity." />
					</>
				)}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-6 pb-8">
			<PageHeader
				eyebrow="Shared public intelligence"
				title="World Intelligence"
				description="Inspect versioned ranking, Storm, gacha, event, and reward data from the official GGS CDN, with preserved player history available below."
				icon={<Globe2 className="h-6 w-6" />}
				meta={(
					<div className="flex flex-wrap justify-end gap-2">
						<Badge variant={status == null ? 'secondary' : 'success'}>
							<Cloud className="mr-1 h-3.5 w-3.5" />
							{status == null ? 'Checking cloud' : 'Cloud enabled'}
						</Badge>
						{status?.worldId && <Badge variant="outline">{displayWorld(status.worldId)}</Badge>}
					</div>
				)}
			/>

			{error && (
				<div className="flex items-start justify-between gap-3 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error" role="alert">
					<span>{error}</span>
					<button type="button" aria-label="Dismiss error" onClick={() => setError('')}><X className="h-4 w-4" /></button>
				</div>
			)}

			<SectionCard
				title="Official ranking and event catalogs"
				description="Every option below is collected from the same versioned Goodgame Studios items endpoint used by CitadelOps game data. These are public definitions and thresholds, not values inferred from a logged-in game session."
				icon={<Database className="h-5 w-5" />}
				actions={<Button variant="ghost" size="icon" aria-label="Refresh official catalog" onClick={() => void refreshCatalog()} isLoading={catalogLoading}><RefreshCw className="h-4 w-4" /></Button>}
			>
				<div className="mb-4 grid gap-3 lg:grid-cols-[minmax(18rem,1fr)_auto] lg:items-end">
					<div>
						<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Public dataset</div>
						<Select
							value={catalogDatasetKey}
							onChange={(value) => setCatalogDatasetKey(value)}
							options={catalogOptions}
							ariaLabel="Select an official ranking or event dataset"
							searchable
							searchPlaceholder="Find Storm, gacha, league, rank, or reward data"
							placeholder="No official datasets collected yet"
							disabled={catalogOptions.length === 0}
							menuGrowToViewport
						/>
					</div>
					<div className="flex flex-wrap gap-2 lg:justify-end">
						<Badge variant="success">Official GGS CDN</Badge>
						<Badge variant="outline">Items v{catalogDataset?.sourceVersion ?? status?.catalogVersion ?? '—'}</Badge>
						<Badge variant="outline">{formatCount(catalogDataset?.rowCount)} rows</Badge>
						<Badge variant="outline">{catalog.datasets.length} datasets</Badge>
						{catalogDataset && <Badge variant="outline">{catalogDataset.contributorCount}/2 collectors</Badge>}
					</div>
				</div>
				{catalogDataset && (
					<div className="mb-4 flex flex-col gap-2 rounded-global border border-border-base bg-bg-input/35 px-4 py-3 text-xs text-text-muted lg:flex-row lg:items-center lg:justify-between">
						<div>
							<span className="font-bold text-text-main">{catalogDataset.datasetLabel}</span>
							<span className="ml-2">Captured {relativeTime(catalogDataset.capturedAt)} · {catalogDataset.history.length} stored version{catalogDataset.history.length === 1 ? '' : 's'}</span>
						</div>
						<a className="font-bold text-primary hover:underline" href={catalogDataset.sourceUrl} target="_blank" rel="noreferrer">Open official source</a>
					</div>
				)}
				<CatalogDatasetTable
					dataset={catalogDataset}
					loading={catalogLoading}
					page={catalogPage}
					onPageChange={setCatalogPage}
				/>
			</SectionCard>

			<div className="flex flex-wrap items-center gap-2">
				<Badge variant="outline">{formatCount(currentCoverage?.players)} players</Badge>
				<Badge variant="outline">{formatCount(currentCoverage?.alliances)} alliances</Badge>
				<Badge variant="outline">{formatCount(currentCoverage?.holdings)} holdings</Badge>
				<Badge variant="outline">{formatCount(currentCoverage?.observationCount)} observations</Badge>
				<Badge variant={freshnessTone(currentCoverage?.lastObservedAt)}>
					Updated {currentCoverage?.lastObservedAt ? relativeTime(currentCoverage.lastObservedAt) : 'never'}
				</Badge>
			</div>

			{!featureReady ? (
				<EmptyState
					size="lg"
					icon={<CloudOff className="h-7 w-7" />}
					title="Connect a game world first"
					description="The active game world is required so players with the same ID on different servers never get mixed."
				/>
			) : (
				<>
					<SectionCard
						title={rankingType === 'players' ? 'Historical player directory' : 'Historical alliance directory'}
						description={`Preserved observations for ${displayWorld(worldId)}. New scheduled collection now comes from the official CDN catalog above.`}
						icon={<Database className="h-5 w-5" />}
						actions={<Button variant="ghost" size="icon" aria-label="Refresh directory" onClick={() => void refreshRanking()} isLoading={rankingLoading}><RefreshCw className="h-4 w-4" /></Button>}
					>
						<div className="mb-4 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
							<PillSelector
								ariaLabel="Directory entity type"
								value={rankingType}
								onChange={selectRankingType}
								options={[{ value: 'players', label: 'Players' }, { value: 'alliances', label: 'Alliances' }]}
								size="body"
							/>
							<div className="w-full sm:w-72">
								<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Ranking category</div>
								<Select
									value={rankingMetric}
									onChange={selectRankingMetric}
									options={currentRankingOptions}
									ariaLabel={`Rank ${rankingType} by category`}
									searchable={rankingType === 'players'}
									searchPlaceholder="Find a public category"
									placeholder="No populated categories"
									disabled={currentRankingOptions.length === 0}
									menuGrowToViewport
								/>
							</div>
							<form className="flex w-full flex-col gap-2 sm:flex-row xl:max-w-2xl" onSubmit={(event) => void submitSearch(event)}>
								<Input
									value={query}
									onChange={(event) => setQuery(event.target.value)}
									placeholder={`Search ${rankingType === 'players' ? 'players' : 'alliances'}`}
									leftIcon={<Search className="h-4 w-4" />}
								/>
								<Button type="submit" isLoading={searching}>Search</Button>
								{appliedQuery && <Button type="button" variant="ghost" onClick={clearSearch}>Clear</Button>}
							</form>
						</div>

						<div className="mb-4 flex flex-col gap-3 rounded-global border border-border-base bg-bg-input/35 px-4 py-3 text-xs text-text-muted lg:flex-row lg:items-center lg:justify-between">
							<div>
								<span className="font-bold text-text-main">
									{status?.collector
										? `Collector slot ${(status.collectorSlot ?? 0) + 1}/${status.collectorSlots ?? 1}`
										: 'Shared-data reader'}
								</span>
								<span className="ml-2">
									{status?.catalogCollectionInProgress
										? `Snapshotting official catalog · ${formatCount(status.catalogDatasets)} datasets prepared`
										: status?.lastCatalogAt ? `Official catalog checked ${relativeTime(status.lastCatalogAt)}` : 'Waiting for the first official catalog snapshot'}
								</span>
							</div>
							<div className="flex flex-wrap gap-2">
								<Badge variant="outline">{status?.pendingBatches ?? 0} queued</Badge>
								<Badge variant="outline">Sorted by {currentRankingLabel}</Badge>
								{currentRankingDefinition && <Badge variant="outline">{formatCount(currentRankingDefinition.populatedRows)} populated</Badge>}
								{currentRankingDefinition && <Badge variant="outline">{rankingSourceLabel(currentRankingDefinition.source)}</Badge>}
								{currentRankingDefinition?.latestObservedAt && <Badge variant={freshnessTone(currentRankingDefinition.latestObservedAt)}>Observed {relativeTime(currentRankingDefinition.latestObservedAt)}</Badge>}
								{appliedQuery && <Badge variant="primary">Search: {appliedQuery}</Badge>}
								{status?.lastCatalogError && <Badge variant="warning">Catalog retry pending</Badge>}
								{status?.lastUploadError && <Badge variant="warning">Cloud retry pending</Badge>}
							</div>
						</div>

						<IntelligenceTable
							rows={visibleRows}
							entityType={rankingType}
							metric={rankingMetric}
							metricLabel={currentRankingLabel}
							availableMetrics={currentRankingOptions.map((option) => option.value)}
							loading={rankingLoading || searching}
							searchQuery={appliedQuery}
							page={tablePage}
							pageCount={pageCount}
							totalRows={tableRows.length}
							onSort={selectRankingMetric}
							onPageChange={setTablePage}
							onOpen={openEntity}
						/>
					</SectionCard>

				</>
			)}
		</div>
	);
};

const CatalogDatasetTable = ({ dataset, loading, page, onPageChange }: {
	dataset: WorldIntelligenceCatalogDatasetV1 | null;
	loading: boolean;
	page: number;
	onPageChange: (page: number) => void;
}) => {
	if (loading && !dataset) return <div className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading official catalog…</div>;
	if (!dataset || dataset.rows.length === 0) {
		return <EmptyState size="md" icon={<Database className="h-6 w-6" />} title="No official rows collected yet" description="Adolphus and James will upload the next versioned catalog snapshot when their assigned collection slots run." />;
	}
	const fields = visibleCatalogFields(dataset);
	const pageCount = Math.max(1, Math.ceil(dataset.rows.length / tablePageSize));
	const safePage = Math.min(page, pageCount - 1);
	const rows = dataset.rows.slice(safePage * tablePageSize, (safePage + 1) * tablePageSize);
	const firstVisible = safePage * tablePageSize + 1;
	const lastVisible = Math.min(dataset.rows.length, firstVisible + rows.length - 1);
	return (
		<div className={`overflow-hidden rounded-global border border-border-base transition-opacity ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
			<div className="max-h-[42rem] overflow-auto custom-scrollbar">
				<table className="min-w-full text-sm">
					<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted">
						<tr>
							<th className="px-3 py-2 text-right">#</th>
							{fields.map((field) => <th key={field} className="whitespace-nowrap px-3 py-2 text-left">{humanizeCatalogKey(field)}</th>)}
						</tr>
					</thead>
					<tbody>
						{rows.map((row, rowIndex) => (
							<tr key={`${dataset.datasetDigest}-${firstVisible + rowIndex}`} className="border-t border-border-base align-top hover:bg-bg-input/45">
								<td className="px-3 py-2 text-right font-mono text-xs text-text-muted">{firstVisible + rowIndex}</td>
								{fields.map((field) => (
									<td key={field} className="max-w-80 px-3 py-2 text-text-main" title={catalogCellTitle(row[field])}>
										<span className="line-clamp-3 break-words">{formatCatalogCell(row[field])}</span>
									</td>
								))}
							</tr>
						))}
					</tbody>
				</table>
			</div>
			<div className="flex flex-col gap-2 border-t border-border-base bg-bg-card px-3 py-2 text-xs text-text-muted sm:flex-row sm:items-center sm:justify-between">
				<span>Rows {formatCount(firstVisible)}–{formatCount(lastVisible)} of {formatCount(dataset.rows.length)}</span>
				<div className="flex items-center gap-2">
					<Button variant="ghost" size="sm" disabled={safePage <= 0} onClick={() => onPageChange(Math.max(0, safePage - 1))}><ChevronLeft className="mr-1 h-4 w-4" />Previous</Button>
					<span>Page {safePage + 1} of {pageCount}</span>
					<Button variant="ghost" size="sm" disabled={safePage >= pageCount - 1} onClick={() => onPageChange(Math.min(pageCount - 1, safePage + 1))}>Next<ChevronRight className="ml-1 h-4 w-4" /></Button>
				</div>
			</div>
		</div>
	);
};

const IntelligenceTable = ({ rows, entityType, metric, metricLabel, availableMetrics, loading, searchQuery, page, pageCount, totalRows, onSort, onPageChange, onOpen }: {
	rows: IntelligenceTableRow[];
	entityType: RankingType;
	metric: string;
	metricLabel: string;
	availableMetrics: string[];
	loading: boolean;
	searchQuery: string;
	page: number;
	pageCount: number;
	totalRows: number;
	onSort: (metric: string) => void;
	onPageChange: (page: number) => void;
	onOpen: (entity: SelectedEntity) => void;
}) => {
	if (loading && rows.length === 0) return <div className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading world directory…</div>;
	if (rows.length === 0) {
		return (
			<EmptyState
				size="md"
				icon={<Database className="h-6 w-6" />}
				title={searchQuery ? `No ${entityType} match “${searchQuery}”` : 'No ranked observations yet'}
				description={searchQuery ? 'Try another public name.' : isExtraPlayerMetric(metric) ? `No collected ${metricLabel} observations are available yet.` : 'This world will populate when the first designated collector scan reaches the cloud.'}
			/>
		);
	}
	const firstVisible = page * tablePageSize + 1;
	const lastVisible = Math.min(totalRows, firstVisible + rows.length - 1);
	return (
		<div className={`overflow-hidden rounded-global border border-border-base transition-opacity ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
			<div className="max-h-[42rem] overflow-auto custom-scrollbar">
				<table className={`w-full border-collapse text-sm ${entityType === 'players' ? isExtraPlayerMetric(metric) ? 'min-w-[70rem]' : 'min-w-[58rem]' : 'min-w-[42rem]'}`}>
					<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wider text-text-muted shadow-[0_1px_0_var(--border-base)]">
						{entityType === 'players' ? (
							<tr>
								<th className="w-16 px-3 py-3 text-left">#</th>
								<th className="min-w-48 px-3 py-3 text-left">Player</th>
							<SortableHeader label="Might" metric="might" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
							<SortableHeader label="Weekly loot" metric="weeklyLoot" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
							<SortableHeader label="Glory" metric="glory" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
							<SortableHeader label="Honor" metric="honor" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
								{isExtraPlayerMetric(metric) && <th className="min-w-44 px-3 py-3 text-right text-primary">{metricLabel}</th>}
								<th className="min-w-48 px-3 py-3 text-left">Alliance</th>
								<th className="px-3 py-3 text-right">Updated</th>
							</tr>
						) : (
							<tr>
								<th className="w-16 px-3 py-3 text-left">#</th>
								<th className="min-w-64 px-3 py-3 text-left">Alliance</th>
							<SortableHeader label="Members" metric="members" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
							<SortableHeader label="Combined might" metric="might" activeMetric={metric} availableMetrics={availableMetrics} onSort={onSort} />
								<th className="px-3 py-3 text-right">Updated</th>
							</tr>
						)}
					</thead>
					<tbody>
						{rows.map((entry) => (
							<tr key={`${entry.type}:${entry.id}`} className="border-t border-border-base hover:bg-bg-card-hover">
								<td className="px-3 py-2.5 font-mono font-bold text-primary">{entry.rank ? `#${entry.rank}` : '—'}</td>
								<td className="px-3 py-2.5">
									<button type="button" className="block max-w-72 truncate text-left font-bold text-text-main hover:text-primary" onClick={() => onOpen({ type: entry.type, id: entry.id, worldId: entry.worldId })}>
										{entry.name}
									</button>
								</td>
								{entityType === 'players' ? (
									<>
										<NumericCell value={entry.might} emphasis={metric === 'might'} />
										<NumericCell value={entry.weeklyLoot} emphasis={metric === 'weeklyLoot'} />
										<NumericCell value={entry.glory} emphasis={metric === 'glory'} />
										<NumericCell value={entry.honor} emphasis={metric === 'honor'} />
										{isExtraPlayerMetric(metric) && <NumericCell value={optionalTableMetricValue(entry, metric)} emphasis />}
										<td className="px-3 py-2.5">
											{entry.allianceId ? (
												<button type="button" className="block max-w-56 truncate font-semibold text-text-main hover:text-primary" onClick={() => onOpen({ type: 'alliance', id: entry.allianceId!, worldId: entry.worldId })}>
													{entry.allianceName || `Alliance ${entry.allianceId}`}
												</button>
											) : <span className="text-text-muted">No alliance</span>}
										</td>
									</>
								) : (
									<>
										<NumericCell value={entry.memberCount} emphasis={metric === 'members'} />
										<NumericCell value={entry.might} emphasis={metric === 'might'} />
									</>
								)}
								<td className="whitespace-nowrap px-3 py-2.5 text-right text-xs text-text-muted">{relativeTime(entry.lastObservedAt)}</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
			<div className="flex flex-col gap-3 border-t border-border-base bg-bg-input/25 px-4 py-3 text-xs text-text-muted sm:flex-row sm:items-center sm:justify-between">
				<span>{formatCount(firstVisible)}–{formatCount(lastVisible)} of {formatCount(totalRows)} {searchQuery ? 'matches' : `top ${entityType}`}</span>
				<div className="flex items-center gap-2">
					<Button type="button" variant="ghost" size="sm" disabled={page <= 0} onClick={() => onPageChange(page - 1)}><ChevronLeft className="mr-1 h-4 w-4" />Previous</Button>
					<span className="min-w-20 text-center">Page {page + 1} of {pageCount}</span>
					<Button type="button" variant="ghost" size="sm" disabled={page + 1 >= pageCount} onClick={() => onPageChange(page + 1)}>Next<ChevronRight className="ml-1 h-4 w-4" /></Button>
				</div>
			</div>
		</div>
	);
};

const SortableHeader = ({ label, metric, activeMetric, availableMetrics, onSort }: {
	label: string;
	metric: string;
	activeMetric: string;
	availableMetrics: string[];
	onSort: (metric: string) => void;
}) => {
	const active = metric === activeMetric;
	const available = availableMetrics.includes(metric);
	return (
		<th className="px-3 py-3 text-right" aria-sort={active ? 'descending' : 'none'}>
			<button
				type="button"
				disabled={!available}
				className={`ml-auto inline-flex items-center gap-1 whitespace-nowrap font-bold transition-colors ${available ? 'hover:text-primary' : 'cursor-default opacity-45'} ${active ? 'text-primary' : ''}`}
				onClick={() => onSort(metric)}
				aria-label={`Sort by ${label}`}
			>
				{label}
				<ArrowDown className={`h-3.5 w-3.5 transition-opacity ${active ? 'opacity-100' : 'opacity-30'}`} />
			</button>
		</th>
	);
};

const NumericCell = ({ value, emphasis = false }: { value?: number; emphasis?: boolean }) => (
	<td className={`whitespace-nowrap px-3 py-2.5 text-right font-mono ${emphasis ? 'font-black text-primary' : 'font-semibold text-text-main'}`}>
		{formatNumber(value)}
	</td>
);

const AllianceProfile = ({ profile, onOpenPlayer }: {
	profile: WorldIntelligenceAllianceProfileV1;
	onOpenPlayer: (player: WorldIntelligencePlayerObservationV1) => void;
}) => {
	const current = profile.current;
	const mightPoints = profile.history.map((row) => ({ at: row.observedAt, value: row.totalMight ?? 0 })).filter((point) => point.value > 0);
	return (
		<div className="space-y-5">
			<div className="flex flex-wrap items-start justify-between gap-4">
				<div><div className="text-2xl font-black text-text-main">{current.name}</div><div className="mt-1 text-sm text-text-muted">Alliance ID {current.allianceId} · {current.memberCount ?? profile.members.length} observed members</div></div>
				<Badge variant="outline">Observed {relativeTime(current.observedAt)}</Badge>
			</div>
			<div className="grid gap-3 sm:grid-cols-3">
				<MetricTile label="Combined might" value={formatNumber(current.totalMight)} tone="brand" />
				<MetricTile label="Members" value={current.memberCount ?? profile.members.length} tone="info" />
				<MetricTile label="Public holdings" value={profile.holdings.length} />
			</div>
			<div className="grid gap-5 xl:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)]">
				<Card><CardContent><div className="mb-4 flex items-center gap-2 font-bold text-text-main"><Activity className="h-4 w-4 text-primary" /> Combined might history</div><Sparkline points={mightPoints} /></CardContent></Card>
				<Card>
					<CardContent>
						<div className="mb-3 flex items-center justify-between gap-3"><div className="flex items-center gap-2 font-bold text-text-main"><Users className="h-4 w-4 text-primary" /> Observed roster</div><Badge variant="outline">{profile.members.length} players</Badge></div>
						<div className="max-h-72 overflow-auto rounded-global border border-border-base custom-scrollbar">
							<table className="w-full text-sm">
								<thead className="sticky top-0 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted"><tr><th className="px-3 py-2 text-left">Player</th><th className="px-3 py-2 text-right">Level</th><th className="px-3 py-2 text-right">Might</th></tr></thead>
								<tbody>{profile.members.map((member) => <tr key={member.playerId} className="border-t border-border-base"><td className="px-3 py-2"><button type="button" className="font-bold text-text-main hover:text-primary" onClick={() => onOpenPlayer(member)}>{member.name}</button></td><td className="px-3 py-2 text-right text-text-muted">{member.legendLevel ? `L${member.legendLevel}` : member.level || '—'}</td><td className="px-3 py-2 text-right font-mono font-bold text-text-main">{formatNumber(member.might)}</td></tr>)}</tbody>
							</table>
						</div>
					</CardContent>
				</Card>
			</div>
		</div>
	);
};

const Sparkline = ({ points }: { points: Array<{ at: string; value: number }> }) => {
	if (points.length < 2) return <EmptyState size="sm" surface="plain" icon={<Activity className="h-5 w-5" />} title="History is still forming" description="At least two distinct observations are needed for a trend." />;
	const width = 640;
	const height = 180;
	const padding = 14;
	const values = points.map((point) => point.value);
	const minimum = Math.min(...values);
	const maximum = Math.max(...values);
	const range = Math.max(1, maximum - minimum);
	const path = points.map((point, index) => {
		const x = padding + (index / Math.max(1, points.length - 1)) * (width - padding * 2);
		const y = height - padding - ((point.value - minimum) / range) * (height - padding * 2);
		return `${index === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`;
	}).join(' ');
	return (
		<div>
			<svg viewBox={`0 0 ${width} ${height}`} className="h-44 w-full overflow-visible" role="img" aria-label={`History from ${formatNumber(minimum)} to ${formatNumber(maximum)}`}>
				<defs><linearGradient id="world-intel-history-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="var(--primary)" stopOpacity="0.28" /><stop offset="1" stopColor="var(--primary)" stopOpacity="0" /></linearGradient></defs>
				<path d={`${path} L ${width - padding} ${height - padding} L ${padding} ${height - padding} Z`} fill="url(#world-intel-history-fill)" />
				<path d={path} fill="none" stroke="var(--primary)" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
			</svg>
			<div className="flex justify-between text-[11px] text-text-muted"><span>{formatDateTime(points[0].at)}</span><span>{formatNumber(maximum)} peak</span><span>{formatDateTime(points[points.length - 1].at)}</span></div>
		</div>
	);
};

function displayWorld(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return '';
	try {
		const parsed = new URL(trimmed.includes('://') ? trimmed : `https://${trimmed}`);
		const port = parsed.port && parsed.port !== '443' && parsed.port !== '80' ? `:${parsed.port}` : '';
		return `${parsed.hostname}${port}` || trimmed;
	} catch {
		return trimmed.replace(/^wss?:\/\//, '').split('/')[0].replace(/:(443|80)$/, '');
	}
}

function visibleCatalogFields(dataset: WorldIntelligenceCatalogDatasetV1): string[] {
	const populated = dataset.fields.filter((field) => dataset.rows.some((row) => {
		const value = row[field];
		return value != null && value !== '' && (!Array.isArray(value) || value.length > 0);
	}));
	const priorities = [
		'comment1', 'comment2', 'eventID', 'eventTypeID', 'leaguetypeID', 'leagueID',
		'rank', 'rankID', 'minRank', 'maxRank', 'lowestRank', 'highestRank', 'threshold',
		'cargoPointRequirement', 'neededPointsForRewards', 'pointsPerTier', 'topXValue',
		'minPulls', 'maxPulls', 'multiPullMax', 'tombolaSpinsAmount',
		'rewardIDs', 'rewardID', 'rewardSetID',
	];
	const priorityIndex = new Map(priorities.map((field, index) => [field, index]));
	return populated.sort((left, right) => {
		const leftPriority = priorityIndex.get(left) ?? catalogFieldPriority(left);
		const rightPriority = priorityIndex.get(right) ?? catalogFieldPriority(right);
		return leftPriority - rightPriority || left.localeCompare(right);
	}).slice(0, 10);
}

function catalogFieldPriority(field: string): number {
	const normalized = field.toLowerCase();
	if (/(rank|point|score|threshold|pull|spin|cost|reward)/.test(normalized)) return 100;
	if (normalized.endsWith('id')) return 200;
	return 300;
}

function humanizeCatalogKey(value: string): string {
	return value
		.replace(/[_-]+/g, ' ')
		.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
		.replace(/\b(id|ids)\b/gi, (match) => match.toUpperCase())
		.replace(/^./, (character) => character.toUpperCase());
}

function formatCatalogCell(value: unknown): string {
	if (value == null || value === '') return '—';
	if (typeof value === 'number') return formatNumber(value);
	if (typeof value === 'boolean') return value ? 'Yes' : 'No';
	if (typeof value === 'string') return value;
	try {
		const encoded = JSON.stringify(value);
		return encoded.length > 180 ? `${encoded.slice(0, 177)}…` : encoded;
	} catch {
		return String(value);
	}
}

function catalogCellTitle(value: unknown): string | undefined {
	if (value == null) return undefined;
	if (typeof value === 'string') return value;
	try {
		return JSON.stringify(value);
	} catch {
		return String(value);
	}
}

function formatNumber(value?: number): string {
	if (value == null || !Number.isFinite(value)) return '—';
	return new Intl.NumberFormat(undefined, { notation: Math.abs(value) >= 100_000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value);
}

function formatCount(value?: number): string {
	return new Intl.NumberFormat().format(value ?? 0);
}

function tableMetricValue(row: IntelligenceTableRow, metric: string): number {
	return optionalTableMetricValue(row, metric) ?? 0;
}

function optionalTableMetricValue(row: IntelligenceTableRow, metric: string): number | undefined {
	let value: number | undefined;
	if (metric.startsWith('public:')) {
		value = row.publicMetrics?.[metric.slice('public:'.length)]?.value ?? row.value;
	} else if (metric === 'achievementPoints') {
		value = row.publicProfile?.achievementPoints ?? row.value;
	} else if (metric === 'highestGlory') {
		value = row.publicProfile?.highestGlory ?? row.value;
	} else if (metric === 'level') {
		value = row.level ?? row.value;
	} else if (metric === 'legendLevel') {
		value = row.legendLevel ?? row.value;
	} else {
		value = metric === 'members'
			? row.memberCount
			: metric === 'weeklyLoot'
				? row.weeklyLoot
				: metric === 'glory'
					? row.glory
					: metric === 'honor'
						? row.honor
						: row.might;
	}
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function rankingOptions(catalog: WorldIntelligenceRankingMetricV1[]): Array<{ value: string; label: string }> {
	return catalog
		.filter((metric) => metric.populatedRows > 0)
		.map((metric) => ({ value: metric.metric, label: metric.label }));
}

function rankingSourceLabel(source: WorldIntelligenceRankingMetricV1['source']): string {
	return ({
		'public-ranking': 'Public ranking feed',
		'public-profile': 'Public player profiles',
		'gge-highscore': 'Live GGE event board',
	} as const)[source] ?? source;
}

function isExtraPlayerMetric(metric: string): boolean {
	return !['might', 'weeklyLoot', 'glory', 'honor', 'members'].includes(metric);
}

function relativeTime(value: string): string {
	const timestamp = Date.parse(value);
	if (!Number.isFinite(timestamp)) return 'Unknown';
	const delta = Math.round((Date.now() - timestamp) / 1000);
	const seconds = Math.abs(delta);
	if (seconds < 60) return 'just now';
	if (delta < 0 && seconds < 3600) return `in ${Math.ceil(seconds / 60)}m`;
	if (delta < 0 && seconds < 86_400) return `in ${Math.ceil(seconds / 3600)}h`;
	if (delta < 0) return `in ${Math.ceil(seconds / 86_400)}d`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
	if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h ago`;
	return `${Math.floor(seconds / 86_400)}d ago`;
}

function formatDateTime(value: string): string {
	const timestamp = Date.parse(value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'Unknown';
}

function freshnessTone(value?: string): 'outline' | 'success' | 'warning' {
	if (!value) return 'outline';
	const age = Date.now() - Date.parse(value);
	return age <= 60 * 60 * 1000 ? 'success' : 'warning';
}

function errorMessage(error: unknown, fallback: string): string {
	return error instanceof Error && error.message ? error.message : fallback;
}

export default WorldIntelligenceView;
