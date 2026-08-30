import { useEffect, useMemo, useState } from 'react';
import {
	Activity,
	Award,
	Crown,
	Database,
	History,
	Sparkles,
	ShieldCheck,
	Trophy,
	UserRound,
	Users,
	Waves,
} from 'lucide-react';
import { CitadelAPI } from '../../api/CitadelClient';
import type {
	WorldIntelligencePlayerObservationV1,
	WorldIntelligencePlayerProfileV1,
} from '../../api/Contracts';
import {
	bucketMetricPoints,
	TrendChart,
	type ChartTimeWindow,
	type RangeKey,
	type TrackerMetricPoint,
} from '../../playerTracker/components/PlayerTrackerView';
import {
	Badge,
	Button,
	Card,
	CardContent,
	CardHeader,
	CardTitle,
	MetricTile,
	PageHeader,
	PillSelector,
	Select,
} from '../../components/ui';

type PlayerCoreMetricKey = 'might' | 'weeklyLoot' | 'glory' | 'honor';
type PlayerMetricKey = PlayerCoreMetricKey | `public:${string}`;

interface PlayerMetricDefinition {
	key: PlayerMetricKey;
	publicMetricKey?: string;
	label: string;
	shortLabel: string;
	color: string;
	imageUrl?: string;
	icon: typeof Activity;
	tone: 'default' | 'brand' | 'success' | 'info';
}

interface WorldPlayerDetailViewProps {
	profile: WorldIntelligencePlayerProfileV1;
	onOpenAlliance: (allianceId: number) => void;
}

const playerMetrics: Array<PlayerMetricDefinition & { key: PlayerCoreMetricKey }> = [
	{
		key: 'might',
		label: 'Might points',
		shortLabel: 'Might',
		color: '#a78bfa',
		imageUrl: '/game-data/resources/images/MightPoints.webp',
		icon: ShieldCheck,
		tone: 'brand',
	},
	{
		key: 'weeklyLoot',
		label: 'Weekly loot',
		shortLabel: 'Loot',
		color: '#34d399',
		imageUrl: '/game-data/resources/images/Loot.webp',
		icon: Database,
		tone: 'success',
	},
	{
		key: 'glory',
		label: 'Glory points',
		shortLabel: 'Glory',
		color: '#60a5fa',
		imageUrl: '/game-data/resources/images/Glory.webp',
		icon: Trophy,
		tone: 'info',
	},
	{
		key: 'honor',
		label: 'Honor',
		shortLabel: 'Honor',
		color: '#f59e0b',
		icon: Award,
		tone: 'default',
	},
];

const historyRanges: Array<{ value: RangeKey; label: string; seconds: number | null }> = [
	{ value: '24h', label: '24H', seconds: 24 * 60 * 60 },
	{ value: '7d', label: '7D', seconds: 7 * 24 * 60 * 60 },
	{ value: '30d', label: '30D', seconds: 30 * 24 * 60 * 60 },
	{ value: 'all', label: 'All', seconds: null },
];

const WorldPlayerDetailView = ({ profile, onOpenAlliance }: WorldPlayerDetailViewProps) => {
	const [selectedMetric, setSelectedMetric] = useState<PlayerMetricKey>('might');
	const [selectedRange, setSelectedRange] = useState<RangeKey>('24h');
	const [selectedWindow, setSelectedWindow] = useState<ChartTimeWindow | null>(null);
	const [officialTitles, setOfficialTitles] = useState<Record<string, string>>({});
	const current = profile.current;
	const history = useMemo(() => normalizedHistory(profile), [profile]);
	const stormMetrics = useMemo(() => stormMetricDefinitions(profile), [profile]);
	const metricDefinitions = useMemo(() => [...playerMetrics, ...stormMetrics], [stormMetrics]);
	const latestTimestamp = Date.parse(history[history.length - 1]?.observedAt ?? current.observedAt);
	const rangeSeconds = historyRanges.find((range) => range.value === selectedRange)?.seconds ?? null;
	const selectedDefinition = metricDefinitions.find((metric) => metric.key === selectedMetric) ?? playerMetrics[0];
	const selectedStormMetric = stormMetrics.find((metric) => metric.key === selectedMetric);
	const allChartPoints = useMemo(
		() => selectedDefinition.publicMetricKey
			? publicMetricHistoryPoints(history, selectedDefinition.publicMetricKey)
			: coreMetricHistoryPoints(history, selectedDefinition.key as PlayerCoreMetricKey),
		[history, selectedDefinition.key, selectedDefinition.publicMetricKey],
	);
	const chartPoints = useMemo(() => {
		const visible = rangeSeconds == null || !Number.isFinite(latestTimestamp)
			? allChartPoints
			: allChartPoints.filter((point) => point.timestampUnix >= Math.floor((latestTimestamp - rangeSeconds * 1_000) / 1_000));
		return bucketMetricPoints(visible, selectedRange);
	}, [allChartPoints, latestTimestamp, rangeSeconds, selectedRange]);
	const displayedPoints = selectedWindow
		? chartPoints.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
		: chartPoints;
	const firstSelectedValue = displayedPoints[0]?.value;
	const currentSelectedValue = displayedPoints[displayedPoints.length - 1]?.value;
	const changes = playerChanges(history);
	const publicMetrics = useMemo(
		() => latestPublicMetrics(profile).filter((metric) => !isChartableStormMetric(metric)),
		[profile],
	);
	useEffect(() => {
		if (metricDefinitions.some((metric) => metric.key === selectedMetric)) return;
		setSelectedMetric('might');
		setSelectedWindow(null);
	}, [metricDefinitions, selectedMetric]);
	useEffect(() => {
		const ids = [current.publicProfile?.titlePrefixId, current.publicProfile?.titleSuffixId, current.publicProfile?.titleId]
			.filter((id): id is number => id != null && id > 0);
		if (ids.length === 0) {
			setOfficialTitles({});
			return;
		}
		let active = true;
		void CitadelAPI.localize([...new Set(ids)].map((id) => `playerTitle_${id}`))
			.then((values) => { if (active) setOfficialTitles(values); })
			.catch(() => { if (active) setOfficialTitles({}); });
		return () => { active = false; };
	}, [current.publicProfile?.titleId, current.publicProfile?.titlePrefixId, current.publicProfile?.titleSuffixId]);
	const publicTitle = publicTitleLabel(current, officialTitles);

	return (
		<>
			<PageHeader
				eyebrow="World Intelligence player"
				title={current.name}
				description={`${progressionLabel(current)} · ${displayWorld(current.worldId)}`}
				icon={<UserRound className="h-6 w-6" />}
				meta={(
					<div className="flex flex-wrap justify-end gap-2">
						{current.allianceId ? (
							<Button type="button" variant="ghost" size="sm" onClick={() => onOpenAlliance(current.allianceId!)}><Users className="mr-1.5 h-3.5 w-3.5" />{current.allianceName || 'Observed alliance'}</Button>
						) : <Badge variant="outline">No alliance</Badge>}
						{publicTitle && <Badge variant="primary" className="gap-1.5"><Crown className="h-3.5 w-3.5" />{publicTitle}</Badge>}
						{current.publicProfile?.achievementPoints != null && <Badge variant="outline">{formatNumber(current.publicProfile.achievementPoints)} achievements</Badge>}
						{current.publicProfile?.highestGlory != null && <Badge variant="outline">{formatNumber(current.publicProfile.highestGlory)} highest glory</Badge>}
						{current.publicProfile?.bestRank != null && current.publicProfile.bestRank > 0 && <Badge variant="outline">Best rank #{formatNumber(current.publicProfile.bestRank)}</Badge>}
						{current.publicProfile?.ruined === true && <Badge variant="warning">Ruined</Badge>}
						<Badge variant={freshnessTone(current.observedAt)}>Observed {relativeTime(current.observedAt)}</Badge>
					</div>
				)}
			/>

			<Card className="liquid-prominent-header-card">
				<CardHeader className="liquid-card-header-prominent flex-wrap gap-4">
					<div className="flex w-full flex-wrap items-center justify-between gap-4">
						<div>
							<CardTitle className="flex items-center gap-2 text-lg">
								<PlayerMetricIcon definition={selectedDefinition} className="h-5 w-5" />
								{selectedDefinition.label} trend
							</CardTitle>
							<div className="mt-2 flex flex-wrap items-baseline gap-3">
								<span className="font-mono text-3xl font-bold text-text-main">{formatNumber(currentSelectedValue)}</span>
								<MetricDelta current={currentSelectedValue} first={firstSelectedValue} />
							</div>
						</div>
						<PillSelector
							ariaLabel="Public player history range"
							value={selectedRange}
							onChange={(value) => setSelectedRange(value as RangeKey)}
							options={historyRanges.map((range) => ({ value: range.value, label: range.label }))}
							size="header"
						/>
					</div>
				</CardHeader>
				<CardContent className="liquid-prominent-header-content p-5 sm:p-6">
					<div className="mb-4 flex flex-wrap gap-2">
						<PillSelector
							ariaLabel="Public player metric"
							value={selectedMetric}
							onChange={(value) => { setSelectedMetric(value as PlayerMetricKey); setSelectedWindow(null); }}
							options={playerMetrics.map((metric) => ({
								value: metric.key,
								label: metric.shortLabel,
								icon: <PlayerMetricIcon definition={metric} className="h-4 w-4" />,
							}))}
							size="body"
						/>
						{stormMetrics.length > 0 && (
							<Select
								value={selectedStormMetric?.key ?? ''}
								onChange={(value) => { setSelectedMetric(value as PlayerMetricKey); setSelectedWindow(null); }}
								placeholder="Storm metrics"
								ariaLabel="More public player metrics"
								className="w-full sm:w-80"
								searchable
								searchPlaceholder="Filter Storm metrics"
								menuGrowToViewport
								options={stormMetrics.map((metric) => ({
									value: metric.key,
									searchText: `${metric.label} Storm ${metric.publicMetricKey ?? ''}`,
									label: (
										<span className="flex min-w-0 items-center gap-2">
											<PlayerMetricIcon definition={metric} className="h-4 w-4" />
											<span className="min-w-0 flex-1 truncate">{metric.label}</span>
											<span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-text-muted">Storm</span>
										</span>
									),
								}))}
							/>
						)}
					</div>
					<div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
						<span>Hover to inspect a public observation. Drag horizontally to inspect a custom time period.</span>
						{selectedWindow && (
							<div className="flex flex-wrap items-center gap-2">
								<Badge variant="outline">{formatChartTime(selectedWindow.startUnix)} – {formatChartTime(selectedWindow.endUnix)}</Badge>
								<Button variant="ghost" size="sm" onClick={() => setSelectedWindow(null)}>Clear selection</Button>
							</div>
						)}
					</div>
					<TrendChart
						points={chartPoints}
						metric={selectedMetric}
						color={selectedDefinition.color}
						range={selectedRange}
						selectedWindow={selectedWindow}
						onWindowSelect={setSelectedWindow}
						emptyMessage="A trend appears after two public observations are available in this range."
					/>
					<div className="mt-3 flex justify-between gap-3 text-xs text-text-muted">
						<span>{displayedPoints[0] ? formatChartTime(displayedPoints[0].timestampUnix) : 'Waiting for history'}</span>
						<span>{displayedPoints.length} sample{displayedPoints.length === 1 ? '' : 's'}</span>
						<span>{displayedPoints.length > 0 ? formatChartTime(displayedPoints[displayedPoints.length - 1].timestampUnix) : 'Now'}</span>
					</div>
				</CardContent>
			</Card>

			<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
				{playerMetrics.map((definition) => {
					const firstValue = metricValue(history[0], definition.key);
					const currentValue = metricValue(current, definition.key);
					return (
						<MetricTile
							key={definition.key}
							label={<span className="inline-flex items-center gap-1.5"><PlayerMetricIcon definition={definition} className="h-3.5 w-3.5" />{definition.label}</span>}
							value={formatNumber(currentValue)}
							tone={definition.tone}
							size="lg"
							caption={<MetricDelta current={currentValue} first={firstValue} compact />}
						/>
					);
				})}
			</div>

			<Card className="liquid-prominent-header-card">
				<CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
					<div>
						<CardTitle className="flex items-center gap-2"><Sparkles className="h-5 w-5 text-primary" />Public scores & event activity</CardTitle>
						<p className="mt-1 text-xs text-text-muted">Gallantry, gacha spins, timestamps, and other one-off public values appear here when their event or board is available. Chartable Storm values are available in the graph above.</p>
					</div>
					<Badge variant="outline">{publicMetrics.length} observed</Badge>
				</CardHeader>
				<CardContent className="liquid-prominent-header-content p-5 sm:p-6">
					{publicMetrics.length === 0 ? (
						<p className="text-sm text-text-muted">No additional public event score has been observed for this player yet. Optional boards are discovered independently so an inactive event cannot interrupt the core World Intel scan.</p>
					) : (
						<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
							{publicMetrics.map((metric) => (
								<div key={metric.key} className="rounded-global border border-border-base bg-bg-input/35 px-3.5 py-3" title={publicMetricProvenance(metric)}>
									<div className="flex items-start justify-between gap-2">
										<div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{metric.label}</div>
										{metric.rank != null && metric.rank > 0 && <Badge variant="outline">#{formatNumber(metric.rank)}</Badge>}
									</div>
									<div className="mt-1 font-mono text-2xl font-bold text-text-main">{formatNumber(metric.value)}</div>
									<div className="mt-1 flex flex-wrap items-center justify-between gap-2 text-[11px] text-text-muted">
										<span>{metric.unit || 'points'} · {publicMetricSourceLabel(metric.source)}</span>
										<span title={formatDateTime(metric.observedAt)}>{relativeTime(metric.observedAt)}</span>
									</div>
								</div>
							))}
						</div>
					)}
				</CardContent>
			</Card>

			<Card>
				<CardHeader><CardTitle className="flex items-center gap-2"><History className="h-5 w-5 text-primary" />Identity history</CardTitle></CardHeader>
				<CardContent className="pt-0">
					{changes.length === 0 ? (
						<p className="text-sm text-text-muted">No player name or alliance changes have been observed.</p>
					) : (
						<div className="max-h-64 space-y-3 overflow-auto custom-scrollbar">
							{changes.slice().reverse().map((change, index) => (
								<div key={`${change.at}:${index}`} className="border-l-2 border-primary/30 pl-3">
									<div className="text-sm font-semibold text-text-main">{change.label}</div>
									<div className="text-[11px] text-text-muted">{formatDateTime(change.at)}</div>
								</div>
							))}
						</div>
					)}
				</CardContent>
			</Card>
		</>
	);
};

const PlayerMetricIcon = ({ definition, className }: { definition: PlayerMetricDefinition; className: string }) => {
	if (definition.imageUrl) return <img src={definition.imageUrl} alt="" className={`${className} shrink-0 object-contain`} />;
	const Icon = definition.icon;
	return <Icon className={`${className} shrink-0`} style={{ color: definition.color }} />;
};

const MetricDelta = ({ current, first, compact = false }: { current?: number; first?: number; compact?: boolean }) => {
	if (current == null || first == null || !Number.isFinite(current) || !Number.isFinite(first)) {
		return <span className="text-xs text-text-muted">No comparison yet</span>;
	}
	const delta = current - first;
	if (delta === 0) return <span className="text-xs text-text-muted">No change</span>;
	const positive = delta > 0;
	const percent = first !== 0 ? Math.abs((delta / first) * 100) : null;
	return (
		<span className={`inline-flex items-center gap-1 text-xs font-bold ${positive ? 'text-success' : 'text-error'}`}>
			{positive ? '+' : '−'}{formatNumber(Math.abs(delta))}
			{!compact && percent != null && <span className="font-medium opacity-80">({percent.toFixed(percent >= 10 ? 0 : 1)}%)</span>}
		</span>
	);
};

type PublicMetricValue = NonNullable<WorldIntelligencePlayerObservationV1['publicMetrics']>[string] & { key: string };

function stormMetricDefinitions(profile: WorldIntelligencePlayerProfileV1): PlayerMetricDefinition[] {
	const latest = new Map<string, PublicMetricValue>();
	for (const observation of [...profile.history, profile.current]) {
		for (const [key, metric] of Object.entries(observation.publicMetrics ?? {})) {
			const candidate = { key, ...metric };
			if (!isChartableStormMetric(candidate) || !Number.isFinite(Date.parse(metric.observedAt))) continue;
			const previous = latest.get(key);
			if (!previous || Date.parse(metric.observedAt) >= Date.parse(previous.observedAt)) latest.set(key, candidate);
		}
	}
	return [...latest.values()]
		.sort((left, right) => stormMetricPriority(left.key) - stormMetricPriority(right.key) || left.label.localeCompare(right.label))
		.map((metric) => ({
			key: `public:${metric.key}`,
			publicMetricKey: metric.key,
			label: metric.label,
			shortLabel: metric.label,
			color: stormMetricColor(metric.key),
			imageUrl: metric.key.includes('aquamarine') ? '/game-data/resources/images/Aquamarine.webp' : undefined,
			icon: Waves,
			tone: 'info',
		}));
}

function coreMetricHistoryPoints(history: WorldIntelligencePlayerObservationV1[], key: PlayerCoreMetricKey): TrackerMetricPoint[] {
	return history.flatMap((observation) => {
		const value = observation[key];
		const timestampUnix = Math.floor(Date.parse(observation.observedAt) / 1_000);
		return typeof value === 'number' && Number.isFinite(value) && Number.isFinite(timestampUnix)
			? [{ timestampUnix, value, source: 'local' as const }]
			: [];
	});
}

function publicMetricHistoryPoints(history: WorldIntelligencePlayerObservationV1[], key: string): TrackerMetricPoint[] {
	const byTimestamp = new Map<number, TrackerMetricPoint>();
	for (const observation of history) {
		const metric = observation.publicMetrics?.[key];
		const timestampUnix = Math.floor(Date.parse(metric?.observedAt ?? '') / 1_000);
		if (!metric || !Number.isFinite(metric.value) || !Number.isFinite(timestampUnix)) continue;
		byTimestamp.set(timestampUnix, { timestampUnix, value: metric.value, source: 'local' });
	}
	return [...byTimestamp.values()].sort((left, right) => left.timestampUnix - right.timestampUnix);
}

function isChartableStormMetric(metric: PublicMetricValue): boolean {
	return metric.key.startsWith('storm-')
		&& metric.key !== 'storm-first-points-at'
		&& metric.unit !== 'unix-seconds'
		&& Number.isFinite(metric.value);
}

function stormMetricPriority(key: string): number {
	return ({
		'storm-cargo-points': 0,
		'storm-aquamarine-total': 1,
		'storm-aquamarine-pvp': 2,
		'storm-aquamarine-islands': 3,
		'storm-aquamarine-fortresses': 4,
		'storm-aquamarine-lost-to-players': 5,
		'storm-aquamarine-spent-cargo-ships': 6,
	} as Record<string, number>)[key] ?? 20;
}

function stormMetricColor(key: string): string {
	return ({
		'storm-cargo-points': '#38bdf8',
		'storm-aquamarine-total': '#22d3ee',
		'storm-aquamarine-pvp': '#fb7185',
		'storm-aquamarine-islands': '#34d399',
		'storm-aquamarine-fortresses': '#f59e0b',
		'storm-aquamarine-lost-to-players': '#f87171',
		'storm-aquamarine-spent-cargo-ships': '#a78bfa',
	} as Record<string, string>)[key] ?? '#60a5fa';
}

function latestPublicMetrics(profile: WorldIntelligencePlayerProfileV1): PublicMetricValue[] {
	const latest = new Map<string, PublicMetricValue>();
	for (const observation of [...profile.history, profile.current]) {
		for (const [key, metric] of Object.entries(observation.publicMetrics ?? {})) {
			if (!metric || !metric.source || !Number.isFinite(metric.value) || !Number.isFinite(Date.parse(metric.observedAt))) continue;
			if (metric.validUntil && Date.parse(metric.validUntil) <= Date.now()) continue;
			const current = latest.get(key);
			if (!current || Date.parse(metric.observedAt) >= Date.parse(current.observedAt)) {
				latest.set(key, { key, ...metric });
			}
		}
	}
	return [...latest.values()].sort((left, right) => {
		const priorityDifference = publicMetricPriority(left.key) - publicMetricPriority(right.key);
		return priorityDifference || left.label.localeCompare(right.label);
	});
}

function publicMetricSourceLabel(source?: NonNullable<WorldIntelligencePlayerObservationV1['publicMetrics']>[string]['source']): string {
	return ({
			'gge-highscore': 'Live GGE board',
			'gge-player-event': 'Live player event',
		} as const)[source ?? 'gge-highscore'] ?? 'Public ranking';
}

function publicMetricProvenance(metric: PublicMetricValue): string {
	const values = [publicMetricSourceLabel(metric.source)];
	if (metric.eventId != null) values.push(`event ${metric.eventId}`);
	if (metric.metricId != null) values.push(`metric ${metric.metricId}`);
	if (metric.listType != null) values.push(`list ${metric.listType}`);
	if (metric.leagueId != null) values.push(`league ${metric.leagueId}`);
	return values.join(' · ');
}

function publicMetricPriority(key: string): number {
	if (key === 'storm-points') return 0;
	if (key.startsWith('gallantry')) return 1;
	if (key.includes('gacha')) return 2;
	if (key.includes('wheel')) return 3;
	return 4;
}

function publicTitleLabel(observation: WorldIntelligencePlayerObservationV1, officialTitles: Record<string, string>): string {
	const profile = observation.publicProfile;
	if (!profile) return '';
	const ids = [profile.titlePrefixId, profile.titleSuffixId, profile.titleId]
		.filter((id): id is number => id != null && id > 0);
	return [...new Set(ids.map((id) => officialTitles[`playerTitle_${id}`]?.trim()).filter((title): title is string => Boolean(title)))]
		.join(' · ');
}

function normalizedHistory(profile: WorldIntelligencePlayerProfileV1): WorldIntelligencePlayerObservationV1[] {
	const byTimestamp = new Map<string, WorldIntelligencePlayerObservationV1>();
	for (const observation of [...profile.history, profile.current]) {
		if (!observation?.observedAt || !Number.isFinite(Date.parse(observation.observedAt))) continue;
		byTimestamp.set(observation.observedAt, observation);
	}
	return [...byTimestamp.values()].sort((left, right) => Date.parse(left.observedAt) - Date.parse(right.observedAt));
}

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

function metricValue(observation: WorldIntelligencePlayerObservationV1 | undefined, metric: PlayerCoreMetricKey): number | undefined {
	const value = observation?.[metric];
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function progressionLabel(observation: WorldIntelligencePlayerObservationV1): string {
	if (observation.legendLevel) return `Legend ${observation.legendLevel}`;
	if (observation.level) return `Level ${observation.level}`;
	return 'Not observed';
}

function displayWorld(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return 'Unknown world';
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

function freshnessTone(value: string): 'success' | 'warning' {
	const age = Date.now() - Date.parse(value);
	return Number.isFinite(age) && age <= 60 * 60 * 1000 ? 'success' : 'warning';
}

function formatDateTime(value: string): string {
	const timestamp = Date.parse(value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'Unknown';
}

function formatChartTime(timestampUnix: number): string {
	return new Date(timestampUnix * 1_000).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

export default WorldPlayerDetailView;
