import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import {
  Activity,
  PackageOpen,
  RefreshCw,
  TrendingUp,
  Trophy,
} from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PageHeader, PillSelector } from '../../components/ui';
import { Notifications } from '../../components/Notifications';
import { useMetadata, type MetadataItem } from '../../context/MetadataContext';

const featureDefinitions = [
  { id: 'autoInvasion', label: 'Auto Invasion', description: 'Foreign Lord and Bloodcrow castles', color: '#f97316' },
  { id: 'autoTowers', label: 'Auto Towers', description: 'Robber-baron and kingdom towers', color: '#f59e0b' },
  { id: 'autoStorm', label: 'Auto Storm', description: 'Storm forts and resource islands', color: '#38bdf8' },
  { id: 'autoBeriWorld', label: 'Auto Beri', description: 'Berimond towers', color: '#a855f7' },
] as const;

export type AttackEconomyFeatureID = typeof featureDefinitions[number]['id'];
type RangeKey = '24h' | '7d' | '30d' | 'all';
const gallantryMetricKey = '__gallantry__';

interface AttackEconomyViewProps {
  selectedFeature?: AttackEconomyFeatureID;
  onFeatureChange?: (feature: AttackEconomyFeatureID) => void;
  showFeatureSelector?: boolean;
  embedded?: boolean;
}

const ranges: Array<{ key: RangeKey; label: string; seconds: number | null }> = [
  { key: '24h', label: '24H', seconds: 24 * 60 * 60 },
  { key: '7d', label: '7D', seconds: 7 * 24 * 60 * 60 },
  { key: '30d', label: '30D', seconds: 30 * 24 * 60 * 60 },
  { key: 'all', label: 'All', seconds: null },
];

interface AttackEconomyReport {
  automationFeature?: string;
  dateMs?: number;
  occurredAt?: string;
  role?: string;
  gallantryPoints?: number;
  loot?: Record<string, number>;
}

interface EconomySummary {
  gallantryPoints: number;
  loot: Record<string, number>;
}

interface ChartPoint {
  timestampUnix: number;
  value: number;
}

interface ChartTimeWindow {
  startUnix: number;
  endUnix: number;
}

const emptySummary = (): EconomySummary => ({
  gallantryPoints: 0,
  loot: {},
});

const AttackEconomyView = ({
  selectedFeature: controlledFeature,
  onFeatureChange,
  showFeatureSelector = true,
  embedded = false,
}: AttackEconomyViewProps) => {
  const { resources: resourceMetadata, currencies: currencyMetadata } = useMetadata();
  const [reports, setReports] = useState<AttackEconomyReport[]>([]);
  const [selectedRange, setSelectedRange] = useState<RangeKey>('7d');
  const [customWindow, setCustomWindow] = useState<ChartTimeWindow | null>(null);
  const [localFeature, setLocalFeature] = useState<AttackEconomyFeatureID>('autoTowers');
  const [requestedMetrics, setRequestedMetrics] = useState<Partial<Record<AttackEconomyFeatureID, string>>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const selectedFeature = controlledFeature ?? localFeature;
  const selectFeature = (feature: AttackEconomyFeatureID) => {
    if (controlledFeature == null) setLocalFeature(feature);
    onFeatureChange?.(feature);
  };

  const loadReports = useCallback(async () => {
    try {
      const response = await fetch('/api/v2/analytics/battle-reports?limit=10000', { cache: 'no-store' });
      if (!response.ok) throw new Error(`Battle history returned HTTP ${response.status}`);
      const payload = await response.json() as { reports?: AttackEconomyReport[] } | AttackEconomyReport[];
      const rows = Array.isArray(payload) ? payload : payload.reports ?? [];
      setReports(rows.filter((report) => report && typeof report === 'object'));
      setLoadError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Could not load attack loot history';
      setLoadError(message);
      Notifications.error(message, 'attack-economy-load');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadReports();
    const timer = window.setInterval(() => void loadReports(), 60_000);
    return () => window.clearInterval(timer);
  }, [loadReports]);

  const nowUnix = Math.floor(Date.now() / 1000);
  const rangeSeconds = ranges.find((range) => range.key === selectedRange)?.seconds ?? null;
  const trackedReports = useMemo(
    () => reports
      .filter((report) => report.role === 'attacker' && isFeatureID(report.automationFeature) && reportTimestamp(report) > 0)
      .sort((left, right) => reportTimestamp(left) - reportTimestamp(right)),
    [reports],
  );
  const rangedReports = useMemo(
    () => trackedReports.filter((report) => rangeSeconds == null || reportTimestamp(report) >= nowUnix - rangeSeconds),
    [nowUnix, rangeSeconds, trackedReports],
  );
  const visibleReports = useMemo(
    () => rangedReports.filter((report) => report.automationFeature === selectedFeature),
    [rangedReports, selectedFeature],
  );
  const displayedReports = useMemo(
    () => customWindow
      ? visibleReports.filter((report) => {
          const timestamp = reportTimestamp(report);
          return timestamp >= customWindow.startUnix && timestamp <= customWindow.endUnix;
        })
      : visibleReports,
    [customWindow, visibleReports],
  );
  const summary = useMemo(() => summarizeReports(displayedReports), [displayedReports]);
  const resourceDefinitions = useMemo(
    () => metadataByJSONKey(resourceMetadata, currencyMetadata),
    [currencyMetadata, resourceMetadata],
  );
  const resourceRows = useMemo(
    () => Object.entries(summary.loot)
      .filter(([key, amount]) => amount > 0 && isVisibleResource(selectedFeature, key))
      .sort((left, right) => right[1] - left[1]),
    [selectedFeature, summary.loot],
  );
  const metricRows = useMemo(
    () => selectedFeature === 'autoBeriWorld'
      ? [[gallantryMetricKey, summary.gallantryPoints] as [string, number], ...resourceRows]
      : resourceRows,
    [resourceRows, selectedFeature, summary.gallantryPoints],
  );
  const requestedMetricKey = requestedMetrics[selectedFeature]
    ?? (selectedFeature === 'autoBeriWorld' ? gallantryMetricKey : 'C1');
  const selectedMetricKey = metricRows.some(([key]) => key === requestedMetricKey)
    ? requestedMetricKey
    : metricRows[0]?.[0] ?? '';
  const selectedMetric = metricPresentation(selectedMetricKey, resourceDefinitions[selectedMetricKey]);
  const chartPoints = useMemo(
    () => selectedMetricKey
      ? buildCumulativePoints(displayedReports, selectedMetricKey, selectedRange, nowUnix, customWindow)
      : [],
    [customWindow, displayedReports, nowUnix, selectedMetricKey, selectedRange],
  );
  const metricTotal = selectedMetricKey === gallantryMetricKey
    ? summary.gallantryPoints
    : summary.loot[selectedMetricKey] ?? 0;
  const rate = metricRate(metricTotal, displayedReports, selectedRange, nowUnix, customWindow);
  const selectedFeatureLabel = featureDefinitions.find((feature) => feature.id === selectedFeature)?.label ?? selectedFeature;
  const selectMetric = (metricKey: string) => {
    setRequestedMetrics((current) => ({ ...current, [selectedFeature]: metricKey }));
  };

  useEffect(() => {
    setCustomWindow(null);
  }, [requestedMetricKey, selectedFeature, selectedRange]);

  return (
    <div className={`flex flex-col gap-6 ${embedded ? '' : 'pb-8'}`}>
      {!embedded && <StaleSessionBanner />}

      {!embedded && <PageHeader
        title={selectedFeature === 'autoBeriWorld' ? 'Auto Beri Stats' : 'Attack Economy'}
        description={selectedFeature === 'autoBeriWorld'
          ? 'Confirmed Gallantry points and loot earned from Auto Beri battle reports'
          : selectedFeature === 'autoInvasion'
            ? 'Confirmed resources looted by Auto Invasion battle reports'
            : 'Confirmed loot produced by non-event attack automations'}
        icon={<TrendingUp className="h-6 w-6" />}
        actions={(
          <Button
            variant="secondary"
            size="sm"
            leftIcon={<RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />}
            onClick={() => void loadReports()}
            disabled={loading}
          >
            Refresh
          </Button>
        )}
        meta={(
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">Battle-report verified</Badge>
            {(selectedFeature === 'autoTowers' || selectedFeature === 'autoStorm') && (
              <Badge variant="secondary">Events excluded</Badge>
            )}
            {loadError && <Badge variant="danger">History unavailable</Badge>}
          </div>
        )}
      />}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <PillSelector
          ariaLabel="Attack economy range"
          value={selectedRange}
          onChange={(value) => setSelectedRange(value as RangeKey)}
          options={ranges.map((range) => ({ value: range.key, label: range.label }))}
          size="header"
        />
        {metricRows.length > 0 && (
          <PillSelector
            ariaLabel="Reward earned"
            value={selectedMetricKey}
            onChange={selectMetric}
            options={metricRows.map(([key]) => {
              const presentation = metricPresentation(key, resourceDefinitions[key]);
              return {
                value: key,
                label: presentation.label,
                icon: presentation.image
                  ? <img src={presentation.image} alt="" className="h-4 w-4 object-contain" />
                  : <PackageOpen className="h-4 w-4" />,
              };
            })}
            size="header"
          />
        )}
      </div>

      {showFeatureSelector && (
        <PillSelector
          ariaLabel="Attack feature"
          value={selectedFeature}
          onChange={(value) => selectFeature(value as AttackEconomyFeatureID)}
          options={featureDefinitions.map((feature) => ({
            value: feature.id,
            label: feature.label,
            icon: <span className="h-2 w-2 rounded-full" style={{ backgroundColor: feature.color }} />,
          }))}
          size="header"
          className="w-full"
        />
      )}

      <Card className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent flex-wrap gap-4">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5 text-primary" />
              {selectedFeatureLabel} · {selectedMetric.label}
            </CardTitle>
            <div className="mt-2 flex flex-wrap items-baseline gap-3">
              <span className="font-mono text-3xl font-bold text-text-main">+{formatNumber(metricTotal)}</span>
              <span className="inline-flex items-center gap-1 text-xs font-semibold text-success">
                <TrendingUp className="h-3.5 w-3.5" />
                Delta +{formatNumber(metricTotal)} · {formatRate(rate, selectedRange, customWindow)}
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-5 sm:p-6">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
            <span>Drag horizontally to inspect a custom time period.</span>
            {customWindow && (
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">
                  {formatDate(customWindow.startUnix)} – {formatDate(customWindow.endUnix)}
                </Badge>
                <Button variant="ghost" size="sm" onClick={() => setCustomWindow(null)}>
                  Clear selection
                </Button>
              </div>
            )}
          </div>
          <EconomyChart
            points={chartPoints}
            color={metricColor(selectedMetricKey)}
            empty={metricTotal <= 0}
            metricLabel={selectedMetric.label}
            selectedWindow={customWindow}
            onWindowSelect={setCustomWindow}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <PackageOpen className="h-4.5 w-4.5 text-success" />
            {selectedFeature === 'autoInvasion'
              ? 'Resources looted'
              : selectedFeature === 'autoBeriWorld'
                ? 'Loot earned'
                : 'Resources earned'}
          </CardTitle>
          <Badge variant="outline">{resourceRows.length} types</Badge>
        </CardHeader>
        <CardContent className="space-y-2 p-4">
          {resourceRows.length === 0 ? (
            <EmptyAnalyticsState compact />
          ) : resourceRows.map(([key, amount]) => (
            <ResourceRow key={key} resourceKey={key} amount={amount} definition={resourceDefinitions[key]} />
          ))}
        </CardContent>
      </Card>
    </div>
  );
};

function ResourceRow({ resourceKey, amount, definition }: { resourceKey: string; amount: number; definition?: MetadataItem }) {
  const presentation = metricPresentation(resourceKey, definition);
  return (
    <div className="flex items-center gap-3 rounded-global border border-border-base bg-bg-input/30 px-3 py-2.5">
      {presentation.image ? (
        <img src={presentation.image} alt="" className="h-8 w-8 shrink-0 object-contain" loading="lazy" />
      ) : (
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-bold text-primary">
          {presentation.label.slice(0, 1)}
        </span>
      )}
      <div className="min-w-0 flex-1 truncate text-sm font-semibold text-text-main">{presentation.label}</div>
      <div className="font-mono text-sm font-bold text-success">+{formatNumber(amount)}</div>
    </div>
  );
}

function EmptyAnalyticsState({ compact = false, metricLabel = 'loot' }: { compact?: boolean; metricLabel?: string }) {
  return (
    <div className={`flex flex-col items-center justify-center text-center text-text-muted ${compact ? 'min-h-32 py-4' : 'min-h-40 py-6'}`}>
      <Trophy className="mb-3 h-8 w-8 opacity-50" />
      <div className="text-sm font-semibold text-text-main">No attributed {metricLabel.toLocaleLowerCase()} yet</div>
      <p className="mt-1 max-w-sm text-xs">New confirmed reports for this automation will begin populating this view.</p>
    </div>
  );
}

function EconomyChart({
  points,
  color,
  empty,
  metricLabel,
  selectedWindow,
  onWindowSelect,
}: {
  points: ChartPoint[];
  color: string;
  empty: boolean;
  metricLabel: string;
  selectedWindow: ChartTimeWindow | null;
  onWindowSelect: (window: ChartTimeWindow) => void;
}) {
  const svgRef = useRef<SVGSVGElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const gradientID = `attack-economy-area-${useId().replaceAll(':', '')}`;
  const [drag, setDrag] = useState<{ startX: number; currentX: number } | null>(null);
  const [chartSize, setChartSize] = useState({ width: 1000, height: 270 });
  const displayedPoints = points;
  const renderable = !empty && displayedPoints.length >= 2;

  useLayoutEffect(() => {
    const element = containerRef.current;
    if (!element || !renderable) return;
    const updateSize = () => {
      const rect = element.getBoundingClientRect();
      const next = { width: Math.max(1, Math.round(rect.width)), height: Math.max(1, Math.round(rect.height)) };
      setChartSize((current) => current.width === next.width && current.height === next.height ? current : next);
    };
    updateSize();
    const observer = new ResizeObserver(updateSize);
    observer.observe(element);
    return () => observer.disconnect();
  }, [renderable]);

  useEffect(() => setDrag(null), [selectedWindow]);

  if (!renderable) {
    return <EmptyAnalyticsState metricLabel={metricLabel} />;
  }

  const { width, height } = chartSize;
  const paddingLeft = 70;
  const paddingRight = 20;
  const paddingTop = 20;
  const paddingBottom = 38;
  const plotWidth = width - paddingLeft - paddingRight;
  const plotHeight = height - paddingTop - paddingBottom;
  const minimumTime = displayedPoints[0].timestampUnix;
  const maximumTime = displayedPoints[displayedPoints.length - 1].timestampUnix;
  const maximumValue = Math.max(1, ...displayedPoints.map((point) => point.value));
  const x = (timestampUnix: number) => paddingLeft + ((timestampUnix - minimumTime) / Math.max(1, maximumTime - minimumTime)) * plotWidth;
  const y = (value: number) => paddingTop + plotHeight - (value / maximumValue) * plotHeight;
  const path = displayedPoints.map((point, index) => `${index === 0 ? 'M' : 'L'} ${x(point.timestampUnix).toFixed(2)} ${y(point.value).toFixed(2)}`).join(' ');
  const area = `${path} L ${x(displayedPoints[displayedPoints.length - 1].timestampUnix).toFixed(2)} ${paddingTop + plotHeight} L ${x(displayedPoints[0].timestampUnix).toFixed(2)} ${paddingTop + plotHeight} Z`;
  const gridValues = [0, 0.25, 0.5, 0.75, 1];
  const pointerX = (event: ReactPointerEvent<SVGSVGElement>) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect || rect.width <= 0) return paddingLeft;
    const viewBoxX = ((event.clientX - rect.left) / rect.width) * width;
    return Math.max(paddingLeft, Math.min(width - paddingRight, viewBoxX));
  };
  const handlePointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (event.button !== 0) return;
    const startX = pointerX(event);
    event.currentTarget.setPointerCapture(event.pointerId);
    setDrag({ startX, currentX: startX });
  };
  const handlePointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (!drag) return;
    const currentX = pointerX(event);
    setDrag((current) => current ? { ...current, currentX } : null);
  };
  const finishPointerSelection = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (!drag) return;
    const endX = pointerX(event);
    const startX = Math.min(drag.startX, endX);
    const finishX = Math.max(drag.startX, endX);
    setDrag(null);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    if (finishX - startX < 24) return;
    const span = Math.max(1, maximumTime - minimumTime);
    const startUnix = minimumTime + ((startX - paddingLeft) / plotWidth) * span;
    const endUnix = minimumTime + ((finishX - paddingLeft) / plotWidth) * span;
    const selectedPoints = displayedPoints.filter((point) => point.timestampUnix >= startUnix && point.timestampUnix <= endUnix);
    if (selectedPoints.length < 2) return;
    onWindowSelect({
      startUnix: selectedPoints[0].timestampUnix,
      endUnix: selectedPoints[selectedPoints.length - 1].timestampUnix,
    });
  };
  const brushStart = drag ? Math.min(drag.startX, drag.currentX) : 0;
  const brushEnd = drag ? Math.max(drag.startX, drag.currentX) : 0;
  return (
    <div className="h-[270px] overflow-hidden rounded-global border border-border-base bg-bg-input/25 p-2">
      <div ref={containerRef} className="h-full w-full">
      <svg
        ref={svgRef}
        viewBox={`0 0 ${width} ${height}`}
        className="h-full w-full cursor-crosshair touch-none select-none"
        role="img"
        aria-label={`Cumulative ${metricLabel.toLocaleLowerCase()} over time. Drag horizontally to select a custom time period.`}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={finishPointerSelection}
        onPointerCancel={() => setDrag(null)}
      >
        <defs>
          <linearGradient id={gradientID} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.3" />
            <stop offset="100%" stopColor={color} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        {gridValues.map((ratio) => {
          const lineY = paddingTop + plotHeight - ratio * plotHeight;
          return (
            <g key={ratio}>
              <line x1={paddingLeft} y1={lineY} x2={width - paddingRight} y2={lineY} stroke="currentColor" className="text-border-base" strokeOpacity="0.65" />
              <text x={paddingLeft - 10} y={lineY + 4} textAnchor="end" className="fill-text-muted text-[11px]">{formatCompact(maximumValue * ratio)}</text>
            </g>
          );
        })}
        <path d={area} fill={`url(#${gradientID})`} />
        <path d={path} fill="none" stroke={color} strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx={x(displayedPoints[displayedPoints.length - 1].timestampUnix)} cy={y(displayedPoints[displayedPoints.length - 1].value)} r="5" fill={color} />
        {drag && <g pointerEvents="none">
          <rect x={brushStart} y={paddingTop} width={Math.max(1, brushEnd - brushStart)} height={plotHeight} fill={color} fillOpacity="0.14" />
          <line x1={brushStart} x2={brushStart} y1={paddingTop} y2={paddingTop + plotHeight} stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
          <line x1={brushEnd} x2={brushEnd} y1={paddingTop} y2={paddingTop + plotHeight} stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
        </g>}
        <text x={paddingLeft} y={height - 10} textAnchor="start" className="fill-text-muted text-[11px]">{formatDate(minimumTime)}</text>
        <text x={width - paddingRight} y={height - 10} textAnchor="end" className="fill-text-muted text-[11px]">{formatDate(maximumTime)}</text>
      </svg>
      </div>
    </div>
  );
}

function summarizeReports(reports: AttackEconomyReport[]): EconomySummary {
  const result = emptySummary();
  for (const report of reports) {
    result.gallantryPoints += finitePositive(report.gallantryPoints);
    for (const [key, rawAmount] of Object.entries(report.loot ?? {})) {
      const amount = finitePositive(rawAmount);
      if (amount <= 0) continue;
      result.loot[key] = (result.loot[key] ?? 0) + amount;
    }
  }
  return result;
}

function buildCumulativePoints(
  reports: AttackEconomyReport[],
  metricKey: string,
  range: RangeKey,
  nowUnix: number,
  window: ChartTimeWindow | null,
): ChartPoint[] {
  const configuredSeconds = ranges.find((candidate) => candidate.key === range)?.seconds ?? null;
  const firstReportAt = reports.length > 0 ? reportTimestamp(reports[0]) : nowUnix;
  const startUnix = window?.startUnix ?? (configuredSeconds == null ? firstReportAt : nowUnix - configuredSeconds);
  const endUnix = window?.endUnix ?? nowUnix;
  const span = Math.max(1, endUnix - startUnix);
  const bucketSeconds = Math.max(60, Math.ceil(span / 72));
  const increments = new Map<number, number>();
  for (const report of reports) {
    const timestampUnix = reportTimestamp(report);
    if (timestampUnix < startUnix || timestampUnix > endUnix) continue;
    const bucket = Math.min(71, Math.max(0, Math.floor((timestampUnix - startUnix) / bucketSeconds)));
    increments.set(bucket, (increments.get(bucket) ?? 0) + reportMetricValue(report, metricKey));
  }
  const points: ChartPoint[] = [{ timestampUnix: startUnix, value: 0 }];
  let cumulative = 0;
  for (let bucket = 0; bucket < 72; bucket++) {
    cumulative += increments.get(bucket) ?? 0;
    points.push({ timestampUnix: Math.min(endUnix, startUnix + (bucket + 1) * bucketSeconds), value: cumulative });
    if (points[points.length - 1].timestampUnix >= endUnix) break;
  }
  if (points[points.length - 1].timestampUnix < endUnix) points.push({ timestampUnix: endUnix, value: cumulative });
  return points;
}

function metricRate(total: number, reports: AttackEconomyReport[], range: RangeKey, nowUnix: number, window: ChartTimeWindow | null): number {
  if (total <= 0 || reports.length === 0) return 0;
  const configuredSeconds = ranges.find((candidate) => candidate.key === range)?.seconds ?? null;
  const elapsedSeconds = window
    ? Math.max(60, window.endUnix - window.startUnix)
    : configuredSeconds ?? Math.max(60 * 60, nowUnix - reportTimestamp(reports[0]));
  const unitSeconds = elapsedSeconds <= 48 * 60 * 60 ? 60 * 60 : 24 * 60 * 60;
  return total / Math.max(1, elapsedSeconds / unitSeconds);
}

function metadataByJSONKey(resources: Record<number, MetadataItem>, currencies: Record<number, MetadataItem>): Record<string, MetadataItem> {
  const result: Record<string, MetadataItem> = {};
  for (const definition of [...Object.values(resources), ...Object.values(currencies)]) {
    const key = typeof definition.JSONKey === 'string' ? definition.JSONKey.trim() : '';
    if (key) result[key] = definition;
  }
  return result;
}

function metricPresentation(key: string, definition?: MetadataItem) {
  const fallback: Record<string, { label: string; image?: string }> = {
    [gallantryMetricKey]: { label: 'Gallantry points', image: '/game-data/resources/images/Gallantry.webp' },
    C1: { label: 'Coins', image: '/game-data/resources/images/Coins.webp' },
    C2: { label: 'Rubies', image: '/game-data/resources/images/Ruby.webp' },
    W: { label: 'Wood' },
    S: { label: 'Stone' },
    F: { label: 'Food' },
    C: { label: 'Charcoal' },
    O: { label: 'Olive oil' },
    G: { label: 'Glass' },
    A: { label: 'Aquamarine' },
    I: { label: 'Iron ore' },
    HONEY: { label: 'Honey' },
    MEAD: { label: 'Mead' },
    BEEF: { label: 'Beef' },
  };
  const walletLabel = key === 'C1' || key === 'C2' ? fallback[key]?.label : undefined;
  const label = capitalizeFirst(walletLabel || definition?.name || fallback[key]?.label || 'Resource');
  return { label, image: definition?.image || fallback[key]?.image };
}

function capitalizeFirst(value: string): string {
  const label = value.trim();
  return label ? label.charAt(0).toLocaleUpperCase() + label.slice(1) : 'Resource';
}

function isVisibleResource(feature: AttackEconomyFeatureID, resourceKey: string): boolean {
  return feature !== 'autoStorm' || resourceKey !== 'C2';
}

function isFeatureID(value?: string): value is AttackEconomyFeatureID {
  return featureDefinitions.some((feature) => feature.id === value);
}

function reportTimestamp(report: AttackEconomyReport): number {
  const dateMs = Number(report.dateMs);
  if (Number.isFinite(dateMs) && dateMs > 0) return Math.floor(dateMs / 1000);
  const parsed = report.occurredAt ? Date.parse(report.occurredAt) : Number.NaN;
  return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : 0;
}

function finitePositive(value: unknown): number {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
}

function reportMetricValue(report: AttackEconomyReport, metricKey: string): number {
  return metricKey === gallantryMetricKey
    ? finitePositive(report.gallantryPoints)
    : finitePositive(report.loot?.[metricKey]);
}

function formatNumber(value: number): string {
  return Math.round(value).toLocaleString();
}

function formatCompact(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}

function formatRate(value: number, range: RangeKey, window: ChartTimeWindow | null): string {
  const elapsedSeconds = window ? window.endUnix - window.startUnix : null;
  const unit = elapsedSeconds != null ? (elapsedSeconds <= 48 * 60 * 60 ? 'hr' : 'day') : (range === '24h' ? 'hr' : 'day');
  return `+${formatCompact(value)}/${unit}`;
}

function formatDate(timestampUnix: number): string {
  return new Date(timestampUnix * 1000).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
  });
}

function metricColor(metricKey: string): string {
  const colors: Record<string, string> = {
    [gallantryMetricKey]: '#f59e0b',
    C1: '#facc15',
    C2: '#fb7185',
    W: '#d97706',
    S: '#94a3b8',
    F: '#84cc16',
    C: '#64748b',
    O: '#a3a32e',
    G: '#67e8f9',
    A: '#38bdf8',
    I: '#a8a29e',
    HONEY: '#f59e0b',
    MEAD: '#eab308',
    BEEF: '#ef4444',
  };
  return colors[metricKey] ?? '#2dd4bf';
}

export default AttackEconomyView;
