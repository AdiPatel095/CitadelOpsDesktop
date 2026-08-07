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
	WorldIntelligenceCoverageResponseV1,
	WorldIntelligencePlayerObservationV1,
	WorldIntelligencePlayerProfileV1,
	WorldIntelligenceRankingEntryV1,
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
	SectionCard,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import DetailBackButton from '../../components/DetailBackButton';
import WorldPlayerDetailView from './WorldPlayerDetailView';

type RankingType = 'players' | 'alliances';
type SelectedEntity = { type: 'player' | 'alliance'; id: number; worldId: string };
type IntelligenceTableRow = Omit<WorldIntelligenceRankingEntryV1, 'rank' | 'value'> & { rank?: number };

const tablePageSize = 25;

const WorldIntelligenceView = () => {
	const { state } = useCitadelAPI();
	const worldId = state?.account.worldId || state?.session.serverUrl || '';
	const [status, setStatus] = useState<WorldIntelligenceStatusV1 | null>(null);
	const [coverage, setCoverage] = useState<WorldIntelligenceCoverageResponseV1>({ worlds: [] });
	const [query, setQuery] = useState('');
	const [appliedQuery, setAppliedQuery] = useState('');
	const [searchResults, setSearchResults] = useState<WorldIntelligenceSearchResultV1[]>([]);
	const [searching, setSearching] = useState(false);
	const [rankingType, setRankingType] = useState<RankingType>('players');
	const [rankingMetric, setRankingMetric] = useState('might');
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
		void refreshCoverage();
	}, [refreshCoverage]);

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
							actions={<DetailBackButton label="Back to World Intelligence" onClick={closeProfile} />}
						/>
						<Card><CardContent className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading public history…</CardContent></Card>
					</>
				) : playerProfile ? (
					<WorldPlayerDetailView
						profile={playerProfile}
						onBack={closeProfile}
						onOpenAlliance={(allianceId) => void openEntity({ type: 'alliance', id: allianceId, worldId: playerProfile.current.worldId })}
					/>
				) : allianceProfile ? (
					<>
						<PageHeader
							eyebrow="World Intelligence alliance"
							title={allianceProfile.current.name}
							description={`${displayWorld(allianceProfile.current.worldId)} · Alliance ${allianceProfile.current.allianceId}`}
							icon={<Users className="h-6 w-6" />}
							actions={<DetailBackButton label="Back to World Intelligence" onClick={closeProfile} />}
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
							actions={<DetailBackButton label="Back to World Intelligence" onClick={closeProfile} />}
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
				description="Browse the world in one sortable directory, then open any player or alliance for its 15-minute history."
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
						title={rankingType === 'players' ? 'Player directory' : 'Alliance directory'}
						description={`One table for ${displayWorld(worldId)}. Select any metric header to rank the world by that field.`}
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
									{status?.scanInProgress
										? `Scanning · ${formatCount(status.scannedPlayers)} players captured`
										: status?.lastScanAt ? `Last scan ${relativeTime(status.lastScanAt)}` : 'Waiting for the first scan'}
								</span>
							</div>
							<div className="flex flex-wrap gap-2">
								<Badge variant="outline">{status?.pendingBatches ?? 0} queued</Badge>
								<Badge variant="outline">Sorted by {metricLabel(rankingMetric)}</Badge>
								{appliedQuery && <Badge variant="primary">Search: {appliedQuery}</Badge>}
								{status?.lastScanError && <Badge variant="warning">Scan retry pending</Badge>}
								{status?.lastUploadError && <Badge variant="warning">Cloud retry pending</Badge>}
							</div>
						</div>

						<IntelligenceTable
							rows={visibleRows}
							entityType={rankingType}
							metric={rankingMetric}
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

const IntelligenceTable = ({ rows, entityType, metric, loading, searchQuery, page, pageCount, totalRows, onSort, onPageChange, onOpen }: {
	rows: IntelligenceTableRow[];
	entityType: RankingType;
	metric: string;
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
				description={searchQuery ? 'Try another public name.' : 'This world will populate when the first designated collector scan reaches the cloud.'}
			/>
		);
	}
	const firstVisible = page * tablePageSize + 1;
	const lastVisible = Math.min(totalRows, firstVisible + rows.length - 1);
	return (
		<div className={`overflow-hidden rounded-global border border-border-base transition-opacity ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
			<div className="max-h-[42rem] overflow-auto custom-scrollbar">
				<table className={`w-full border-collapse text-sm ${entityType === 'players' ? 'min-w-[58rem]' : 'min-w-[42rem]'}`}>
					<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wider text-text-muted shadow-[0_1px_0_var(--border-base)]">
						{entityType === 'players' ? (
							<tr>
								<th className="w-16 px-3 py-3 text-left">#</th>
								<th className="min-w-48 px-3 py-3 text-left">Player</th>
								<SortableHeader label="Might" metric="might" activeMetric={metric} onSort={onSort} />
								<SortableHeader label="Weekly loot" metric="weeklyLoot" activeMetric={metric} onSort={onSort} />
								<SortableHeader label="Glory" metric="glory" activeMetric={metric} onSort={onSort} />
								<SortableHeader label="Honor" metric="honor" activeMetric={metric} onSort={onSort} />
								<th className="min-w-48 px-3 py-3 text-left">Alliance</th>
								<th className="px-3 py-3 text-right">Updated</th>
							</tr>
						) : (
							<tr>
								<th className="w-16 px-3 py-3 text-left">#</th>
								<th className="min-w-64 px-3 py-3 text-left">Alliance</th>
								<SortableHeader label="Members" metric="members" activeMetric={metric} onSort={onSort} />
								<SortableHeader label="Combined might" metric="might" activeMetric={metric} onSort={onSort} />
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

const SortableHeader = ({ label, metric, activeMetric, onSort }: {
	label: string;
	metric: string;
	activeMetric: string;
	onSort: (metric: string) => void;
}) => {
	const active = metric === activeMetric;
	return (
		<th className="px-3 py-3 text-right" aria-sort={active ? 'descending' : 'none'}>
			<button
				type="button"
				className={`ml-auto inline-flex items-center gap-1 whitespace-nowrap font-bold transition-colors hover:text-primary ${active ? 'text-primary' : ''}`}
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

function formatNumber(value?: number): string {
	if (value == null || !Number.isFinite(value)) return '—';
	return new Intl.NumberFormat(undefined, { notation: Math.abs(value) >= 100_000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value);
}

function formatCount(value?: number): string {
	return new Intl.NumberFormat().format(value ?? 0);
}

function tableMetricValue(row: IntelligenceTableRow, metric: string): number {
	const value = metric === 'members'
		? row.memberCount
		: metric === 'weeklyLoot'
			? row.weeklyLoot
			: metric === 'glory'
				? row.glory
				: metric === 'honor'
					? row.honor
					: row.might;
	return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function metricLabel(metric: string): string {
	return ({ might: 'Might', glory: 'Glory', weeklyLoot: 'Weekly loot', honor: 'Honor', members: 'Members' } as Record<string, string>)[metric] || metric;
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
