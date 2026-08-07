import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
	Activity,
	Cloud,
	CloudOff,
	Database,
	Globe2,
	History,
	RefreshCw,
	Search,
	ShieldCheck,
	Trophy,
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
	Select,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';

type SearchType = 'all' | 'player' | 'alliance';
type RankingType = 'players' | 'alliances';
type SelectedEntity = { type: 'player' | 'alliance'; id: number; worldId: string };

const playerMetricOptions = [
	{ value: 'might', label: 'Might' },
	{ value: 'glory', label: 'Glory' },
	{ value: 'weeklyLoot', label: 'Weekly loot' },
	{ value: 'honor', label: 'Honor' },
	{ value: 'legendLevel', label: 'Legend level' },
	{ value: 'level', label: 'Level' },
];

const allianceMetricOptions = [
	{ value: 'might', label: 'Combined might' },
	{ value: 'members', label: 'Members' },
];

const WorldIntelligenceView = () => {
	const { state } = useCitadelAPI();
	const worldId = state?.account.worldId || state?.session.serverUrl || '';
	const [status, setStatus] = useState<WorldIntelligenceStatusV1 | null>(null);
	const [coverage, setCoverage] = useState<WorldIntelligenceCoverageResponseV1>({ worlds: [] });
	const [query, setQuery] = useState('');
	const [searchType, setSearchType] = useState<SearchType>('all');
	const [searchResults, setSearchResults] = useState<WorldIntelligenceSearchResultV1[]>([]);
	const [searching, setSearching] = useState(false);
	const [rankingType, setRankingType] = useState<RankingType>('players');
	const [rankingMetric, setRankingMetric] = useState('might');
	const [ranking, setRanking] = useState<WorldIntelligenceRankingResponseV1 | null>(null);
	const [rankingLoading, setRankingLoading] = useState(false);
	const [selected, setSelected] = useState<SelectedEntity | null>(null);
	const [playerProfile, setPlayerProfile] = useState<WorldIntelligencePlayerProfileV1 | null>(null);
	const [allianceProfile, setAllianceProfile] = useState<WorldIntelligenceAllianceProfileV1 | null>(null);
	const [profileLoading, setProfileLoading] = useState(false);
	const [error, setError] = useState('');

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
				limit: 100,
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
		setSearching(true);
		setError('');
		try {
			const response = await CitadelAPI.searchWorldIntelligence({
				worldId,
				query,
				type: searchType === 'all' ? undefined : searchType,
				limit: 50,
			});
			setSearchResults(response.results ?? []);
		} catch (requestError) {
			setSearchResults([]);
			setError(errorMessage(requestError, 'Search failed.'));
		} finally {
			setSearching(false);
		}
	};

	const openEntity = useCallback(async (entity: SelectedEntity) => {
		setSelected(entity);
		setPlayerProfile(null);
		setAllianceProfile(null);
		setProfileLoading(true);
		setError('');
		try {
			if (entity.type === 'player') {
				setPlayerProfile(await CitadelAPI.getWorldIntelligencePlayer(entity.worldId, entity.id));
			} else {
				setAllianceProfile(await CitadelAPI.getWorldIntelligenceAlliance(entity.worldId, entity.id));
			}
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load this profile.'));
		} finally {
			setProfileLoading(false);
		}
	}, []);

	const currentCoverage = coverage.worlds[0];
	const rankedEntries = ranking?.entries ?? [];
	const featureReady = Boolean(worldId);

	return (
		<div className="flex flex-col gap-6 pb-8">
			<PageHeader
				eyebrow="Shared public intelligence"
				title="World Intelligence"
				description="Search players and alliances, compare public rankings, and follow 15-minute history collected from designated CitadelOps accounts."
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

			<div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
				<MetricTile label="Players observed" value={currentCoverage?.players ?? 0} tone="brand" size="lg" />
				<MetricTile label="Alliances observed" value={currentCoverage?.alliances ?? 0} tone="info" size="lg" />
				<MetricTile label="Public holdings" value={currentCoverage?.holdings ?? 0} tone="default" size="lg" />
				<MetricTile label="Observations" value={currentCoverage?.observationCount ?? 0} tone="success" size="lg" />
				<MetricTile
					label="Cloud freshness"
					value={currentCoverage?.lastObservedAt ? relativeTime(currentCoverage.lastObservedAt) : 'No data'}
					monospace={false}
					tone={freshnessTone(currentCoverage?.lastObservedAt)}
					size="lg"
				/>
			</div>

			<SectionCard
				title="Collection status"
				description="World Intelligence is always available. Only designated owned accounts scan GGE's public leaderboards; every other desktop is a reader."
				icon={<ShieldCheck className="h-5 w-5" />}
			>
				<div className="grid gap-4 lg:grid-cols-2">
					<div className="rounded-global border border-border-base bg-bg-input/45 p-4">
						<div className="font-bold text-text-main">{status?.collector ? 'Owned collector account' : 'Shared-data reader'}</div>
						<div className="mt-1 text-xs leading-relaxed text-text-muted">
							{status?.collector
								? `Alternating slot ${(status.collectorSlot ?? 0) + 1} of ${status.collectorSlots ?? 1}; this account scans every ${(status.collectorSlots ?? 1) * 15} minutes.`
								: 'This profile reads the shared cloud dataset and does not send leaderboard traffic to GGE.'}
						</div>
					</div>
					<div className="rounded-global border border-border-base bg-bg-input/45 p-4">
						<div className="font-bold text-text-main">{status?.scanInProgress ? 'Leaderboard scan running' : 'Collector idle'}</div>
						<div className="mt-1 text-xs leading-relaxed text-text-muted">
							{status?.scanInProgress
								? `${formatNumber(status.scannedPlayers)} public players captured so far.`
								: status?.nextScanAt ? `Next assigned scan ${relativeTime(status.nextScanAt)}.` : 'No scan slot is assigned to this profile.'}
						</div>
					</div>
				</div>
				<div className="mt-4 flex flex-wrap gap-2 text-xs text-text-muted">
					<Badge variant="outline">{status?.pendingBatches ?? 0} queued batches</Badge>
					<Badge variant="outline">Last scan {status?.lastScanAt ? relativeTime(status.lastScanAt) : 'not yet'}</Badge>
					<Badge variant="outline">Last upload {status?.lastUploadAt ? relativeTime(status.lastUploadAt) : 'not yet'}</Badge>
					<Badge variant="outline">Public fields only</Badge>
					{status?.lastScanError && <Badge variant="warning">Scan retry pending</Badge>}
					{status?.lastUploadError && <Badge variant="warning">Cloud retry pending</Badge>}
				</div>
			</SectionCard>

			{!featureReady ? (
				<EmptyState
					size="lg"
					icon={<CloudOff className="h-7 w-7" />}
					title="Connect a game world first"
					description="The active game world is required so players with the same ID on different servers never get mixed."
				/>
			) : (
				<>
					<div className="grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
						<SectionCard title="Find an entity" description={`Search ${displayWorld(worldId)} by public player or alliance name.`} icon={<Search className="h-5 w-5" />}>
							<form className="flex flex-col gap-3" onSubmit={(event) => void submitSearch(event)}>
								<div className="flex flex-col gap-3 sm:flex-row">
									<Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Player or alliance name" leftIcon={<Search className="h-4 w-4" />} />
									<Select
										value={searchType}
										onChange={(value) => setSearchType(value as SearchType)}
										ariaLabel="Search entity type"
										className="sm:w-44"
										options={[
											{ value: 'all', label: 'Players & alliances' },
											{ value: 'player', label: 'Players' },
											{ value: 'alliance', label: 'Alliances' },
										]}
									/>
									<Button type="submit" isLoading={searching}>Search</Button>
								</div>
							</form>
							<div className="mt-4 space-y-2">
								{searchResults.length === 0 ? (
									<EmptyState size="sm" surface="plain" title="Search the shared dataset" description="Results always include their observed world and freshness." />
								) : searchResults.map((result) => (
									<EntityResult key={`${result.type}:${result.id}`} result={result} onOpen={() => void openEntity({ type: result.type, id: result.id, worldId: result.worldId })} />
								))}
							</div>
						</SectionCard>

						<SectionCard
							title="Public rankings"
							description="Collector-observed, not authoritative. Rankings use each entity’s freshest cloud observation from the alternating 15-minute scans."
							icon={<Trophy className="h-5 w-5" />}
							actions={<Button variant="ghost" size="icon" aria-label="Refresh rankings" onClick={() => void refreshRanking()} isLoading={rankingLoading}><RefreshCw className="h-4 w-4" /></Button>}
						>
							<div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
								<PillSelector
									ariaLabel="Ranking entity type"
									value={rankingType}
									onChange={(value) => {
										setRankingType(value as RankingType);
										setRankingMetric('might');
									}}
									options={[{ value: 'players', label: 'Players' }, { value: 'alliances', label: 'Alliances' }]}
									size="body"
								/>
								<Select
									value={rankingMetric}
									onChange={setRankingMetric}
									ariaLabel="Ranking metric"
									className="sm:w-48"
									options={rankingType === 'players' ? playerMetricOptions : allianceMetricOptions}
								/>
							</div>
							<RankingTable entries={rankedEntries} metric={ranking?.metric ?? rankingMetric} loading={rankingLoading} onOpen={openEntity} />
						</SectionCard>
					</div>

					{selected && (
						<SectionCard
							title={selected.type === 'player' ? 'Player intelligence' : 'Alliance intelligence'}
							description={`${displayWorld(selected.worldId)} · stable ID ${selected.id}`}
							icon={selected.type === 'player' ? <UserRound className="h-5 w-5" /> : <Users className="h-5 w-5" />}
							actions={<Button variant="ghost" size="icon" aria-label="Close profile" onClick={() => setSelected(null)}><X className="h-4 w-4" /></Button>}
						>
							{profileLoading ? (
								<div className="flex min-h-52 items-center justify-center text-sm text-text-muted">Loading cloud history…</div>
							) : playerProfile ? (
								<PlayerProfile profile={playerProfile} />
							) : allianceProfile ? (
								<AllianceProfile profile={allianceProfile} onOpenPlayer={(player) => void openEntity({ type: 'player', id: player.playerId, worldId: player.worldId })} />
							) : (
								<EmptyState size="sm" title="Profile unavailable" description="No usable observations were returned." />
							)}
						</SectionCard>
					)}
				</>
			)}
		</div>
	);
};

const EntityResult = ({ result, onOpen }: { result: WorldIntelligenceSearchResultV1; onOpen: () => void }) => (
	<button type="button" onClick={onOpen} className="m3-card-interactive flex w-full items-center justify-between gap-3 rounded-global border border-border-base px-4 py-3 text-left">
		<div className="flex min-w-0 items-center gap-3">
			<span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
				{result.type === 'player' ? <UserRound className="h-4 w-4" /> : <Users className="h-4 w-4" />}
			</span>
			<div className="min-w-0">
				<div className="truncate font-bold text-text-main">{result.name}</div>
				<div className="truncate text-xs text-text-muted">
					{result.type === 'player' ? result.allianceName || 'No observed alliance' : `${formatNumber(result.memberCount)} members`}
				</div>
			</div>
		</div>
		<div className="shrink-0 text-right">
			<div className="font-mono text-sm font-bold text-text-main">{formatNumber(result.might)}</div>
			<div className="text-[10px] uppercase tracking-wide text-text-muted">{relativeTime(result.lastObservedAt)}</div>
		</div>
	</button>
);

const RankingTable = ({ entries, metric, loading, onOpen }: {
	entries: WorldIntelligenceRankingEntryV1[];
	metric: string;
	loading: boolean;
	onOpen: (entity: SelectedEntity) => void;
}) => {
	if (loading && entries.length === 0) return <div className="flex min-h-64 items-center justify-center text-sm text-text-muted">Loading rankings…</div>;
	if (entries.length === 0) return <EmptyState size="md" icon={<Database className="h-6 w-6" />} title="No ranked observations yet" description="This world will populate when the first designated collector scan reaches the cloud." />;
	return (
		<div className="max-h-[34rem] overflow-auto rounded-global border border-border-base custom-scrollbar">
			<table className="w-full border-collapse text-sm">
				<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wider text-text-muted">
					<tr><th className="px-3 py-2 text-left">Rank</th><th className="px-3 py-2 text-left">Name</th><th className="px-3 py-2 text-right">{metricLabel(metric)}</th><th className="px-3 py-2 text-right">Freshness</th></tr>
				</thead>
				<tbody>
					{entries.map((entry) => (
						<tr key={`${entry.type}:${entry.id}`} className="border-t border-border-base hover:bg-bg-card-hover">
							<td className="px-3 py-2 font-mono font-bold text-primary">#{entry.rank}</td>
							<td className="px-3 py-2">
								<button type="button" className="block max-w-72 text-left" onClick={() => onOpen({ type: entry.type, id: entry.id, worldId: entry.worldId })}>
									<span className="block truncate font-bold text-text-main hover:text-primary">{entry.name}</span>
									{entry.type === 'player' && <span className="block truncate text-[11px] text-text-muted">{entry.allianceName || 'No observed alliance'}</span>}
								</button>
							</td>
							<td className="px-3 py-2 text-right font-mono font-bold text-text-main">{formatNumber(entry.value)}</td>
							<td className="px-3 py-2 text-right text-xs text-text-muted">{relativeTime(entry.lastObservedAt)}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
};

const PlayerProfile = ({ profile }: { profile: WorldIntelligencePlayerProfileV1 }) => {
	const [metric, setMetric] = useState<'might' | 'glory' | 'weeklyLoot' | 'honor'>('might');
	const current = profile.current;
	const changes = playerChanges(profile.history);
	const points = profile.history.map((row) => ({ at: row.observedAt, value: row[metric] ?? 0 })).filter((point) => point.value > 0);
	return (
		<div className="space-y-5">
			<div className="flex flex-wrap items-start justify-between gap-4">
				<div>
					<div className="text-2xl font-black text-text-main">{current.name}</div>
					<div className="mt-1 text-sm text-text-muted">{current.allianceName || 'No observed alliance'} · level {current.level || '—'}{current.legendLevel ? ` · legend ${current.legendLevel}` : ''}</div>
				</div>
				<Badge variant="outline">Observed {relativeTime(current.observedAt)}</Badge>
			</div>
			<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
				<MetricTile label="Might" value={formatNumber(current.might)} tone="brand" />
				<MetricTile label="Glory" value={formatNumber(current.glory)} tone="info" />
				<MetricTile label="Weekly loot" value={formatNumber(current.weeklyLoot)} tone="success" />
				<MetricTile label="Honor" value={formatNumber(current.honor)} />
				<MetricTile label="Level" value={current.level || '—'} />
				<MetricTile label="Legend level" value={current.legendLevel || '—'} />
			</div>
			<div className="grid gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(16rem,0.7fr)]">
				<Card>
					<CardContent>
						<div className="mb-4 flex items-center justify-between gap-3">
							<div className="flex items-center gap-2 font-bold text-text-main"><Activity className="h-4 w-4 text-primary" /> Public history</div>
							<PillSelector ariaLabel="Player history metric" value={metric} onChange={(value) => setMetric(value as 'might' | 'glory' | 'weeklyLoot' | 'honor')} options={[{ value: 'might', label: 'Might' }, { value: 'glory', label: 'Glory' }, { value: 'weeklyLoot', label: 'Loot' }, { value: 'honor', label: 'Honor' }]} size="body" />
						</div>
						<Sparkline points={points} />
					</CardContent>
				</Card>
				<Card>
					<CardContent>
						<div className="mb-3 flex items-center gap-2 font-bold text-text-main"><History className="h-4 w-4 text-primary" /> Identity changes</div>
						{changes.length === 0 ? <div className="text-sm text-text-muted">No name or alliance changes observed yet.</div> : (
							<div className="max-h-52 space-y-3 overflow-auto custom-scrollbar">
								{changes.slice().reverse().map((change, index) => <div key={`${change.at}:${index}`} className="border-l-2 border-primary/30 pl-3"><div className="text-sm font-semibold text-text-main">{change.label}</div><div className="text-[11px] text-text-muted">{formatDateTime(change.at)}</div></div>)}
							</div>
						)}
					</CardContent>
				</Card>
			</div>
		</div>
	);
};

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

function playerChanges(history: WorldIntelligencePlayerObservationV1[]): Array<{ at: string; label: string }> {
	const changes: Array<{ at: string; label: string }> = [];
	for (let index = 1; index < history.length; index += 1) {
		const previous = history[index - 1];
		const current = history[index];
		if (previous.name !== current.name) changes.push({ at: current.observedAt, label: `${previous.name} → ${current.name}` });
		if ((previous.allianceId ?? 0) !== (current.allianceId ?? 0)) changes.push({ at: current.observedAt, label: `${previous.allianceName || 'No alliance'} → ${current.allianceName || 'No alliance'}` });
	}
	return changes;
}

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

function metricLabel(metric: string): string {
	return ({ might: 'Might', glory: 'Glory', weeklyLoot: 'Weekly loot', honor: 'Honor', level: 'Level', legendLevel: 'Legend', members: 'Members' } as Record<string, string>)[metric] || metric;
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

function freshnessTone(value?: string): 'default' | 'success' | 'warning' {
	if (!value) return 'default';
	const age = Date.now() - Date.parse(value);
	return age <= 60 * 60 * 1000 ? 'success' : 'warning';
}

function errorMessage(error: unknown, fallback: string): string {
	return error instanceof Error && error.message ? error.message : fallback;
}

export default WorldIntelligenceView;
