import { useMemo, useState } from 'react';
import {
	Activity,
	Award,
	CalendarDays,
	Clock3,
	Database,
	History,
	ShieldCheck,
	Trophy,
	UserRound,
	Users,
} from 'lucide-react';
import type {
	WorldIntelligencePlayerObservationV1,
	WorldIntelligencePlayerProfileV1,
} from '../../api/Contracts';
import DetailBackButton from '../../components/DetailBackButton';
import {
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
} from '../../components/ui';

type PlayerMetricKey = 'might' | 'weeklyLoot' | 'glory' | 'honor';

interface PlayerMetricDefinition {
	key: PlayerMetricKey;
	label: string;
	shortLabel: string;
	color: string;
	imageUrl?: string;
	icon: typeof Activity;
	tone: 'default' | 'brand' | 'success' | 'info';
}

interface WorldPlayerDetailViewProps {
	profile: WorldIntelligencePlayerProfileV1;
	onBack: () => void;
	onOpenAlliance: (allianceId: number) => void;
}

const playerMetrics: PlayerMetricDefinition[] = [
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

const WorldPlayerDetailView = ({ profile, onBack, onOpenAlliance }: WorldPlayerDetailViewProps) => {
	const [selectedMetric, setSelectedMetric] = useState<PlayerMetricKey>('might');
	const [selectedRange, setSelectedRange] = useState<RangeKey>('24h');
	const [selectedWindow, setSelectedWindow] = useState<ChartTimeWindow | null>(null);
	const current = profile.current;
	const history = useMemo(() => normalizedHistory(profile), [profile]);
	const latestTimestamp = Date.parse(history[history.length - 1]?.observedAt ?? current.observedAt);
	const rangeSeconds = historyRanges.find((range) => range.value === selectedRange)?.seconds ?? null;
	const visibleHistory = useMemo(() => {
		if (rangeSeconds == null || !Number.isFinite(latestTimestamp)) return history;
		const cutoff = latestTimestamp - rangeSeconds * 1_000;
		return history.filter((observation) => Date.parse(observation.observedAt) >= cutoff);
	}, [history, latestTimestamp, rangeSeconds]);
	const selectedDefinition = playerMetrics.find((metric) => metric.key === selectedMetric) ?? playerMetrics[0];
	const chartPoints: TrackerMetricPoint[] = visibleHistory.flatMap((observation) => {
		const value = observation[selectedMetric];
		return typeof value === 'number' && Number.isFinite(value)
			? [{ timestampUnix: Math.floor(Date.parse(observation.observedAt) / 1_000), value, source: 'local' as const }]
			: [];
	});
	const displayedPoints = selectedWindow
		? chartPoints.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
		: chartPoints;
	const firstSelectedValue = displayedPoints[0]?.value;
	const currentSelectedValue = displayedPoints[displayedPoints.length - 1]?.value;
	const changes = playerChanges(history);

	return (
		<>
			<PageHeader
				eyebrow="World Intelligence player"
				title={current.name}
				description={`${current.allianceName || 'No observed alliance'} · Player ${current.playerId} · ${displayWorld(current.worldId)}`}
				icon={<UserRound className="h-6 w-6" />}
				actions={<DetailBackButton label="Back to World Intelligence" onClick={onBack} />}
				meta={(
					<div className="flex flex-wrap justify-end gap-2">
						<Badge variant="outline" className="gap-1.5"><Clock3 className="h-3.5 w-3.5" />15-minute public scans</Badge>
						<Badge variant={freshnessTone(current.observedAt)}>Observed {relativeTime(current.observedAt)}</Badge>
						<Badge variant="outline">{history.length} observation{history.length === 1 ? '' : 's'}</Badge>
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
					<div className="mb-4">
						<PillSelector
							ariaLabel="Public player metric"
							value={selectedMetric}
							onChange={(value) => setSelectedMetric(value as PlayerMetricKey)}
							options={playerMetrics.map((metric) => ({
								value: metric.key,
								label: metric.shortLabel,
								icon: <PlayerMetricIcon definition={metric} className="h-4 w-4" />,
							}))}
							size="body"
						/>
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

			<div className="grid items-start gap-5 xl:grid-cols-[minmax(0,0.72fr)_minmax(0,1.28fr)]">
				<div className="space-y-5">
					<Card>
						<CardHeader><CardTitle className="flex items-center gap-2"><UserRound className="h-5 w-5 text-primary" />Public profile</CardTitle></CardHeader>
						<CardContent className="grid gap-3 pt-0 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
							<ProfileFact label="Alliance" value={current.allianceName || 'No alliance'} action={current.allianceId ? () => onOpenAlliance(current.allianceId!) : undefined} />
							<ProfileFact label="Player ID" value={String(current.playerId)} />
							<ProfileFact label="World" value={displayWorld(current.worldId)} />
							<ProfileFact label="Progression" value={progressionLabel(current)} />
							<ProfileFact label="Latest source" value={sourceLabel(current.source)} />
							<ProfileFact label="Last observed" value={formatDateTime(current.observedAt)} />
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
				</div>

				<Card>
					<CardHeader className="flex-wrap gap-3">
						<div>
							<CardTitle className="flex items-center gap-2"><CalendarDays className="h-5 w-5 text-primary" />Observation history</CardTitle>
							<p className="mt-1 text-xs text-text-muted">The most recent public snapshots collected for this player.</p>
						</div>
						<Badge variant="outline">{history.length} total</Badge>
					</CardHeader>
					<CardContent className="pt-0">
						<div className="max-h-[34rem] overflow-auto rounded-global border border-border-base custom-scrollbar">
							<table className="min-w-[48rem] w-full text-sm">
								<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted">
									<tr><th className="px-3 py-2 text-left">Observed</th><th className="px-3 py-2 text-right">Might</th><th className="px-3 py-2 text-right">Weekly loot</th><th className="px-3 py-2 text-right">Glory</th><th className="px-3 py-2 text-right">Honor</th><th className="px-3 py-2 text-left">Alliance</th></tr>
								</thead>
								<tbody>
									{history.slice().reverse().map((observation) => (
										<tr key={observation.observedAt} className="border-t border-border-base hover:bg-bg-card-hover">
											<td className="whitespace-nowrap px-3 py-2.5 text-xs text-text-muted">{formatDateTime(observation.observedAt)}</td>
											<td className="px-3 py-2.5 text-right font-mono font-semibold text-text-main">{formatNumber(observation.might)}</td>
											<td className="px-3 py-2.5 text-right font-mono font-semibold text-text-main">{formatNumber(observation.weeklyLoot)}</td>
											<td className="px-3 py-2.5 text-right font-mono font-semibold text-text-main">{formatNumber(observation.glory)}</td>
											<td className="px-3 py-2.5 text-right font-mono font-semibold text-text-main">{formatNumber(observation.honor)}</td>
											<td className="max-w-48 truncate px-3 py-2.5 text-text-muted">{observation.allianceName || 'No alliance'}</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					</CardContent>
				</Card>
			</div>
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

const ProfileFact = ({ label, value, action }: { label: string; value: string; action?: () => void }) => (
	<div className="rounded-global border border-border-base bg-bg-input/35 px-3 py-2.5">
		<div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{label}</div>
		{action ? (
			<Button type="button" variant="ghost" size="sm" className="-ml-2 mt-0.5 max-w-full" onClick={action}><Users className="mr-1.5 h-3.5 w-3.5" /><span className="truncate">{value}</span></Button>
		) : <div className="mt-1 truncate text-sm font-semibold text-text-main" title={value}>{value}</div>}
	</div>
);

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

function metricValue(observation: WorldIntelligencePlayerObservationV1 | undefined, metric: PlayerMetricKey): number | undefined {
	const value = observation?.[metric];
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function progressionLabel(observation: WorldIntelligencePlayerObservationV1): string {
	if (observation.legendLevel) return `Legend ${observation.legendLevel}`;
	if (observation.level) return `Level ${observation.level}`;
	return 'Not observed';
}

function sourceLabel(source: WorldIntelligencePlayerObservationV1['source']): string {
	return ({ account: 'Account profile', alliance: 'Alliance roster', 'event-ranking': 'Event ranking', leaderboard: 'Public leaderboard' } as const)[source] ?? source;
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
