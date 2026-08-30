import { useEffect, useMemo, useState } from 'react';
import { Activity, Castle, ShieldCheck, Sparkles, Users } from 'lucide-react';
import type {
	WorldIntelligenceAllianceObservationV1,
	WorldIntelligenceAllianceProfileV1,
	WorldIntelligencePlayerObservationV1,
	WorldIntelligencePublicMetricV1,
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
} from '../../components/ui';

type AllianceMetricDefinition = {
	key: string;
	label: string;
	shortLabel: string;
	color: string;
	icon: typeof Activity;
	tone: 'brand' | 'info';
};

const allianceMetrics: AllianceMetricDefinition[] = [
	{ key: 'totalMight', label: 'Combined might', shortLabel: 'Might', color: '#a78bfa', icon: ShieldCheck, tone: 'brand' },
	{ key: 'memberCount', label: 'Observed members', shortLabel: 'Members', color: '#60a5fa', icon: Users, tone: 'info' },
];

const alliancePublicMetricColors = ['#fb7185', '#22d3ee', '#facc15', '#4ade80'];

const historyRanges: Array<{ value: RangeKey; label: string; seconds: number | null }> = [
	{ value: '24h', label: '24H', seconds: 24 * 60 * 60 },
	{ value: '7d', label: '7D', seconds: 7 * 24 * 60 * 60 },
	{ value: '30d', label: '30D', seconds: 30 * 24 * 60 * 60 },
	{ value: 'all', label: 'All', seconds: null },
];

interface WorldAllianceDetailViewProps {
	profile: WorldIntelligenceAllianceProfileV1;
	onOpenPlayer: (player: WorldIntelligencePlayerObservationV1) => void;
}

const WorldAllianceDetailView = ({ profile, onOpenPlayer }: WorldAllianceDetailViewProps) => {
	const [selectedMetric, setSelectedMetric] = useState('totalMight');
	const [selectedRange, setSelectedRange] = useState<RangeKey>('24h');
	const [selectedWindow, setSelectedWindow] = useState<ChartTimeWindow | null>(null);
	const current = profile.current;
	const history = useMemo(() => normalizedAllianceHistory(profile), [profile]);
	const latestTimestamp = Date.parse(history[history.length - 1]?.observedAt ?? current.observedAt);
	const rangeSeconds = historyRanges.find((range) => range.value === selectedRange)?.seconds ?? null;
	const visibleHistory = useMemo(() => {
		if (rangeSeconds == null || !Number.isFinite(latestTimestamp)) return history;
		const cutoff = latestTimestamp - rangeSeconds * 1_000;
		return history.filter((observation) => Date.parse(observation.observedAt) >= cutoff);
	}, [history, latestTimestamp, rangeSeconds]);
	const publicMetrics = useMemo(() => latestAlliancePublicMetrics(profile), [profile]);
	const metricDefinitions = useMemo<AllianceMetricDefinition[]>(() => [
		...allianceMetrics,
		...publicMetrics
			.filter((metric) => metric.unit !== 'unix-seconds')
			.map((metric, index) => ({
				key: `public:${metric.key}`,
				label: metric.label,
				shortLabel: metric.label,
				color: alliancePublicMetricColors[index % alliancePublicMetricColors.length],
				icon: Activity,
				tone: 'brand' as const,
			})),
	], [publicMetrics]);
	useEffect(() => {
		if (metricDefinitions.some((definition) => definition.key === selectedMetric)) return;
		setSelectedMetric(metricDefinitions[0]?.key ?? 'totalMight');
		setSelectedWindow(null);
	}, [metricDefinitions, selectedMetric]);
	const selectedDefinition = metricDefinitions.find((metric) => metric.key === selectedMetric) ?? metricDefinitions[0];
	const chartPoints = useMemo(() => bucketMetricPoints(
		visibleHistory.flatMap<TrackerMetricPoint>((observation) => {
			const value = allianceMetricValue(observation, selectedMetric);
			return typeof value === 'number' && Number.isFinite(value)
				? [{ timestampUnix: Math.floor(Date.parse(observation.observedAt) / 1_000), value, source: 'local' }]
				: [];
		}),
		selectedRange,
	), [selectedMetric, selectedRange, visibleHistory]);
	const displayedPoints = selectedWindow
		? chartPoints.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
		: chartPoints;
	const firstSelectedValue = displayedPoints[0]?.value;
	const currentSelectedValue = displayedPoints[displayedPoints.length - 1]?.value;

	return (
		<>
			<PageHeader
				eyebrow="World Intelligence alliance"
				title={current.name}
				description={`${formatCount(current.memberCount ?? profile.members.length)} observed members · ${displayWorld(current.worldId)}`}
				icon={<Users className="h-6 w-6" />}
				meta={(
					<div className="flex flex-wrap justify-end gap-2">
						<Badge variant="outline"><Castle className="mr-1.5 h-3.5 w-3.5" />{formatCount(profile.holdings.length)} public holdings</Badge>
						<Badge variant={freshnessTone(current.observedAt)}>Observed {relativeTime(current.observedAt)}</Badge>
					</div>
				)}
			/>

			<Card className="liquid-prominent-header-card">
				<CardHeader className="liquid-card-header-prominent flex-wrap gap-4">
					<div className="flex w-full flex-wrap items-center justify-between gap-4">
						<div>
							<CardTitle className="flex items-center gap-2 text-lg">
								<selectedDefinition.icon className="h-5 w-5" style={{ color: selectedDefinition.color }} />
								{selectedDefinition.label} trend
							</CardTitle>
							<div className="mt-2 flex flex-wrap items-baseline gap-3">
								<span className="font-mono text-3xl font-bold text-text-main">{formatNumber(currentSelectedValue)}</span>
								<MetricDelta current={currentSelectedValue} first={firstSelectedValue} />
							</div>
						</div>
						<PillSelector
							ariaLabel="Public alliance history range"
							value={selectedRange}
							onChange={(value) => { setSelectedRange(value as RangeKey); setSelectedWindow(null); }}
							options={historyRanges.map((range) => ({ value: range.value, label: range.label }))}
							size="header"
						/>
					</div>
				</CardHeader>
				<CardContent className="liquid-prominent-header-content p-5 sm:p-6">
					<div className="mb-4">
						<PillSelector
							ariaLabel="Public alliance metric"
							value={selectedMetric}
							onChange={(value) => { setSelectedMetric(value); setSelectedWindow(null); }}
							options={metricDefinitions.map((metric) => ({ value: metric.key, label: metric.shortLabel, icon: <metric.icon className="h-4 w-4" style={{ color: metric.color }} /> }))}
							size="body"
						/>
					</div>
					<div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
						<span>Hover to inspect a public observation. Drag horizontally to inspect a custom time period.</span>
						{selectedWindow && <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{formatChartTime(selectedWindow.startUnix)} – {formatChartTime(selectedWindow.endUnix)}</Badge><Button variant="ghost" size="sm" onClick={() => setSelectedWindow(null)}>Clear selection</Button></div>}
					</div>
					<TrendChart
						points={chartPoints}
						metric={selectedMetric}
						color={selectedDefinition.color}
						range={selectedRange}
						selectedWindow={selectedWindow}
						onWindowSelect={setSelectedWindow}
						emptyMessage="A trend appears after two public alliance observations are available in this range."
					/>
					<div className="mt-3 flex justify-between gap-3 text-xs text-text-muted">
						<span>{displayedPoints[0] ? formatChartTime(displayedPoints[0].timestampUnix) : 'Waiting for history'}</span>
						<span>{displayedPoints.length} sample{displayedPoints.length === 1 ? '' : 's'}</span>
						<span>{displayedPoints.length > 0 ? formatChartTime(displayedPoints[displayedPoints.length - 1].timestampUnix) : 'Now'}</span>
					</div>
				</CardContent>
			</Card>

			<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
				{allianceMetrics.map((definition) => <MetricTile key={definition.key} label={definition.label} value={formatNumber(allianceMetricValue(current, definition.key))} tone={definition.tone} size="lg" caption={<MetricDelta current={allianceMetricValue(current, definition.key)} first={history[0] ? allianceMetricValue(history[0], definition.key) : undefined} compact />} />)}
				<MetricTile label="Public holdings" value={formatCount(profile.holdings.length)} size="lg" />
				<MetricTile label="Public event scores" value={formatCount(publicMetrics.length)} size="lg" />
			</div>

			<Card className="liquid-prominent-header-card">
				<CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
					<div><CardTitle className="flex items-center gap-2"><Sparkles className="h-5 w-5 text-primary" />Public scores & event activity</CardTitle><p className="mt-1 text-xs text-text-muted">Collected alliance event rankings appear here when their public leaderboard is available.</p></div>
					<Badge variant="outline">{publicMetrics.length} observed</Badge>
				</CardHeader>
				<CardContent className="liquid-prominent-header-content p-5 sm:p-6">
					{publicMetrics.length === 0 ? <p className="text-sm text-text-muted">No additional public event score has been observed for this alliance yet.</p> : (
						<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
							{publicMetrics.map((metric) => <div key={metric.key} className="rounded-global border border-border-base bg-bg-input/35 px-3.5 py-3" title={publicMetricProvenance(metric)}><div className="flex items-start justify-between gap-2"><div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{metric.label}</div>{metric.rank != null && metric.rank > 0 && <Badge variant="outline">#{formatCount(metric.rank)}</Badge>}</div><div className="mt-1 font-mono text-2xl font-bold text-text-main">{formatNumber(metric.value)}</div><div className="mt-1 flex flex-wrap items-center justify-between gap-2 text-[11px] text-text-muted"><span>{metric.unit || 'points'} · {publicMetricSourceLabel(metric.source)}</span><span title={formatDateTime(metric.observedAt)}>{relativeTime(metric.observedAt)}</span></div></div>)}
						</div>
					)}
				</CardContent>
			</Card>

			<Card>
				<CardHeader className="flex-wrap gap-3"><CardTitle className="flex items-center gap-2"><Users className="h-5 w-5 text-primary" />Observed roster</CardTitle><Badge variant="outline">{formatCount(profile.members.length)} players</Badge></CardHeader>
				<CardContent className="pt-0">
					<div className="max-h-[34rem] overflow-auto rounded-global border border-border-base custom-scrollbar">
						<table className="w-full min-w-[40rem] text-sm">
							<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted"><tr><th className="px-3 py-2 text-left">Player</th><th className="px-3 py-2 text-right">Level</th><th className="px-3 py-2 text-right">Might</th></tr></thead>
							<tbody>{profile.members.map((member) => <tr key={member.playerId} className="border-t border-border-base hover:bg-bg-card-hover"><td className="px-3 py-2.5"><button type="button" className="font-bold text-text-main hover:text-primary" onClick={() => onOpenPlayer(member)}>{member.name}</button></td><td className="px-3 py-2.5 text-right text-text-muted">{member.legendLevel ? `Legend ${member.legendLevel}` : member.level ? `Level ${member.level}` : '—'}</td><td className="px-3 py-2.5 text-right font-mono font-bold text-text-main">{formatNumber(member.might)}</td></tr>)}</tbody>
						</table>
					</div>
				</CardContent>
			</Card>
		</>
	);
};

const MetricDelta = ({ current, first, compact = false }: { current?: number; first?: number; compact?: boolean }) => {
	if (current == null || first == null || !Number.isFinite(current) || !Number.isFinite(first)) return <span className="text-xs text-text-muted">No comparison yet</span>;
	const delta = current - first;
	if (delta === 0) return <span className="text-xs text-text-muted">No change</span>;
	const percent = first !== 0 ? Math.abs((delta / first) * 100) : null;
	return <span className={`inline-flex items-center gap-1 text-xs font-bold ${delta > 0 ? 'text-success' : 'text-error'}`}>{delta > 0 ? '+' : '−'}{formatNumber(Math.abs(delta))}{!compact && percent != null && <span className="font-medium opacity-80">({percent.toFixed(percent >= 10 ? 0 : 1)}%)</span>}</span>;
};

type AlliancePublicMetric = WorldIntelligencePublicMetricV1 & { key: string };

function normalizedAllianceHistory(profile: WorldIntelligenceAllianceProfileV1): WorldIntelligenceAllianceObservationV1[] {
	const byTimestamp = new Map<string, WorldIntelligenceAllianceObservationV1>();
	for (const observation of [...profile.history, profile.current]) {
		if (!observation?.observedAt || !Number.isFinite(Date.parse(observation.observedAt))) continue;
		byTimestamp.set(observation.observedAt, observation);
	}
	return [...byTimestamp.values()].sort((left, right) => Date.parse(left.observedAt) - Date.parse(right.observedAt));
}

function latestAlliancePublicMetrics(profile: WorldIntelligenceAllianceProfileV1): AlliancePublicMetric[] {
	const latest = new Map<string, AlliancePublicMetric>();
	for (const observation of [...profile.history, profile.current]) {
		for (const [key, metric] of Object.entries(observation.publicMetrics ?? {})) {
			if (!metric || !Number.isFinite(metric.value) || !Number.isFinite(Date.parse(metric.observedAt))) continue;
			if (metric.validUntil && Date.parse(metric.validUntil) <= Date.now()) continue;
			const current = latest.get(key);
			if (!current || Date.parse(metric.observedAt) >= Date.parse(current.observedAt)) latest.set(key, { key, ...metric });
		}
	}
	return [...latest.values()].sort((left, right) => left.label.localeCompare(right.label));
}

function allianceMetricValue(observation: WorldIntelligenceAllianceObservationV1, metric: string): number | undefined {
	const value = metric === 'totalMight'
		? observation.totalMight
		: metric === 'memberCount'
			? observation.memberCount
			: observation.publicMetrics?.[metric.replace(/^public:/, '')]?.value;
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function publicMetricSourceLabel(source?: WorldIntelligencePublicMetricV1['source']): string {
	return ({ 'gge-highscore': 'Live GGE board', 'gge-player-event': 'Live player event' } as const)[source ?? 'gge-highscore'] ?? 'Public ranking';
}

function publicMetricProvenance(metric: AlliancePublicMetric): string {
	const values = [publicMetricSourceLabel(metric.source)];
	if (metric.eventId != null) values.push(`event ${metric.eventId}`);
	if (metric.metricId != null) values.push(`metric ${metric.metricId}`);
	if (metric.listType != null) values.push(`list ${metric.listType}`);
	if (metric.leagueId != null) values.push(`league ${metric.leagueId}`);
	return values.join(' · ');
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

function formatCount(value?: number): string {
	return new Intl.NumberFormat().format(value ?? 0);
}

function relativeTime(value: string): string {
	const timestamp = Date.parse(value);
	if (!Number.isFinite(timestamp)) return 'Unknown';
	const seconds = Math.abs(Math.round((Date.now() - timestamp) / 1_000));
	if (seconds < 60) return 'just now';
	if (seconds < 3_600) return `${Math.floor(seconds / 60)}m ago`;
	if (seconds < 86_400) return `${Math.floor(seconds / 3_600)}h ago`;
	return `${Math.floor(seconds / 86_400)}d ago`;
}

function freshnessTone(value: string): 'success' | 'warning' {
	const age = Date.now() - Date.parse(value);
	return Number.isFinite(age) && age <= 60 * 60 * 1_000 ? 'success' : 'warning';
}

function formatDateTime(value: string): string {
	const timestamp = Date.parse(value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'Unknown';
}

function formatChartTime(timestampUnix: number): string {
	return new Date(timestampUnix * 1_000).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

export default WorldAllianceDetailView;
