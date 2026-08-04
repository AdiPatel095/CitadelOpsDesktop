import { useEffect, useLayoutEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import {
  Activity,
  CircleDollarSign,
  Clock3,
  Coins,
  Crosshair,
  Shield,
  ShieldCheck,
  Sparkles,
  Swords,
  TrendingDown,
  TrendingUp,
  Trophy,
} from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import type { FoodFilter, RoleFilter, TypeFilter } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PageHeader, PillSelector, Select } from '../../components/ui';
import { useAuth } from '../../context/AuthContext';
import { useMetadata, type MetadataItem } from '../../context/MetadataContext';
import { Notifications } from '../../components/Notifications';
import { useCitadelAPI } from '../../api/ApiContext';
import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';

interface PlayerTrackerSample {
  timestampUnix: number;
  playerId: number;
  might: number;
  glory: number;
  gallantry: number;
  troopsTotal: number;
  troopsStationed: number;
  troopsTraveling: number;
  troopsHospital: number;
  troopsByUnit?: Record<string, number>;
  coins: number;
  rubies: number;
  currencies?: Record<string, number>;
}

interface TrackerMetricPoint {
  timestampUnix: number;
  value: number;
  source: 'local' | 'gge-tracker';
}

interface TrackerFallbackInfo {
  provider: string;
  status: string;
  server?: string;
  playerName?: string;
  fetchedAtUnix?: number;
  pointsAdded?: number;
}

interface ChartTimeWindow {
  startUnix: number;
  endUnix: number;
}

interface TroopCombatComposition {
  total: number;
  melee: number;
  ranged: number;
  attack: number;
  defense: number;
  typeClassified: number;
  roleClassified: number;
}

interface TrendLineSeries {
  key: string;
  label: string;
  color: string;
  points: TrackerMetricPoint[];
  displayPoints?: TrackerMetricPoint[];
  unitID?: number;
}

interface TroopUnitClassification {
  weaponType: 'melee' | 'range' | null;
  combatRole: 'attack' | 'defense' | null;
  foodType: 'mead' | 'beef' | 'food';
}

interface PlayerTrackerResponse {
  current: PlayerTrackerSample | null;
  samples: PlayerTrackerSample[];
  series: Partial<Record<MetricKey, TrackerMetricPoint[]>>;
  intervalSeconds: number;
  fallback: TrackerFallbackInfo;
  coverage: {
    loot: boolean;
    eventScores: boolean;
  };
}

type CurrencyKey = string;
type MetricKey = string;
type RangeKey = '24h' | '7d' | '30d' | 'all';
type MetricCategory = 'Player stats' | 'Wallet';

interface MetricDefinition {
  key: MetricKey;
  label: string;
  shortLabel: string;
  color: string;
  icon: typeof Activity;
  imageUrl?: string;
  category: MetricCategory | 'Troops';
}

const CORE_METRIC_DEFINITIONS: MetricDefinition[] = [
  { key: 'might', label: 'Might points', shortLabel: 'Might', color: '#34d399', icon: ShieldCheck, imageUrl: '/game-data/resources/images/MightPoints.webp', category: 'Player stats' },
  { key: 'glory', label: 'Glory points', shortLabel: 'Glory', color: '#60a5fa', icon: Trophy, imageUrl: '/game-data/resources/images/Glory.webp', category: 'Player stats' },
  { key: 'loot', label: 'Loot', shortLabel: 'Loot', color: '#2dd4bf', icon: CircleDollarSign, imageUrl: '/game-data/resources/images/Loot.webp', category: 'Player stats' },
  { key: 'gallantry', label: 'Gallantry points', shortLabel: 'Gallantry', color: '#f59e0b', icon: Sparkles, imageUrl: '/game-data/resources/images/Gallantry.webp', category: 'Player stats' },
  { key: 'coins', label: 'Coins', shortLabel: 'Coins', color: '#facc15', icon: Coins, imageUrl: '/game-data/resources/images/Coins.webp', category: 'Wallet' },
  { key: 'rubies', label: 'Rubies', shortLabel: 'Rubies', color: '#fb7185', icon: CircleDollarSign, imageUrl: '/game-data/resources/images/Ruby.webp', category: 'Wallet' },
  { key: 'troopsTotal', label: 'Total troops', shortLabel: 'Troops', color: '#a78bfa', icon: Swords, category: 'Troops' },
];

const HIGHLIGHT_METRIC_KEYS = new Set<MetricKey>(['might', 'glory', 'coins', 'rubies']);

const ranges: Array<{ key: RangeKey; label: string; seconds: number | null }> = [
  { key: '24h', label: '24H', seconds: 24 * 60 * 60 },
  { key: '7d', label: '7D', seconds: 7 * 24 * 60 * 60 },
  { key: '30d', label: '30D', seconds: 30 * 24 * 60 * 60 },
  { key: 'all', label: 'All', seconds: null },
];

const emptyResponse: PlayerTrackerResponse = {
  current: null,
  samples: [],
  series: {},
  intervalSeconds: 60,
  fallback: { provider: 'gge-tracker', status: 'not-needed' },
  coverage: { loot: false, eventScores: false },
};

const MetricIcon = ({ definition, className }: { definition: MetricDefinition; className: string }) => {
  const [imageFailed, setImageFailed] = useState(false);
  const FallbackIcon = definition.icon;

  useEffect(() => setImageFailed(false), [definition.imageUrl]);

  if (definition.imageUrl && !imageFailed) {
    return <img src={definition.imageUrl} alt="" loading="lazy" decoding="async" className={`${className} shrink-0 object-contain`} onError={() => setImageFailed(true)} />;
  }

  return <FallbackIcon className={`${className} shrink-0`} style={{ color: definition.color }} />;
};

const PlayerTrackerView = () => {
  const { state } = useCitadelAPI();
  const { gameLoggedIn } = useAuth();
  const { troops: troopMetadata, resources: resourceMetadata, currencies: currencyMetadata } = useMetadata();
  const metricDefinitions = useMemo(
    () => buildMetricDefinitions(state, resourceMetadata, currencyMetadata),
    [currencyMetadata, resourceMetadata, state],
  );
  const primaryMetricDefinitions = useMemo(
    () => metricDefinitions.filter((definition) => definition.key !== 'troopsTotal'),
    [metricDefinitions],
  );
	const highlightMetricDefinitions = useMemo(
		() => primaryMetricDefinitions.filter((definition) => HIGHLIGHT_METRIC_KEYS.has(definition.key)),
		[primaryMetricDefinitions],
	);
	const extraMetricDefinitions = useMemo(
		() => primaryMetricDefinitions.filter((definition) => !HIGHLIGHT_METRIC_KEYS.has(definition.key)),
		[primaryMetricDefinitions],
	);
  const troopMetricDefinition = metricDefinitions.find((definition) => definition.key === 'troopsTotal')!;
  const [tracker, setTracker] = useState<PlayerTrackerResponse>(emptyResponse);
  const [selectedMetric, setSelectedMetric] = useState<MetricKey>('might');
  const [selectedRange, setSelectedRange] = useState<RangeKey>('7d');
  const [customWindow, setCustomWindow] = useState<ChartTimeWindow | null>(null);
  const [troopRange, setTroopRange] = useState<RangeKey>('7d');
  const [troopWindow, setTroopWindow] = useState<ChartTimeWindow | null>(null);
  const [troopTypeFilter, setTroopTypeFilter] = useState<TypeFilter>('all');
  const [troopRoleFilter, setTroopRoleFilter] = useState<RoleFilter>('all');
  const [troopFoodFilter, setTroopFoodFilter] = useState<FoodFilter>('all');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
	const selectedExtraMetric = extraMetricDefinitions.find((definition) => definition.key === selectedMetric);
	const activePlayerID = Math.max(0, finite(state?.player.id));
	const scopedTracker = activePlayerID > 0 && tracker.current?.playerId !== activePlayerID ? emptyResponse : tracker;

	useEffect(() => {
		if (metricDefinitions.some((definition) => definition.key === selectedMetric)) return;
		setSelectedMetric('might');
	}, [metricDefinitions, selectedMetric]);

  useEffect(() => {
    if (loadError) {
      Notifications.error(loadError, 'player-tracker-load');
    }
  }, [loadError]);

  const liveSample = useMemo<PlayerTrackerSample | null>(() => {
    if (!state || !gameLoggedIn) return null;
    let stationed = 0;
    let traveling = 0;
    let hospital = 0;
    const troopsByUnit: Record<string, number> = {};
    for (const playerCastle of Object.values(state.castles)) {
      stationed += sumTroopRecord(playerCastle.units.stationed, troopMetadata);
      traveling += sumTroopRecord(playerCastle.units.traveling, troopMetadata);
      hospital += sumTroopRecord(playerCastle.units.hospital, troopMetadata)
        + sumTroopRecord(playerCastle.units.specialHospital, troopMetadata);
      addTroopsByUnit(troopsByUnit, playerCastle.units.stationed, troopMetadata);
      addTroopsByUnit(troopsByUnit, playerCastle.units.traveling, troopMetadata);
      addTroopsByUnit(troopsByUnit, playerCastle.units.hospital, troopMetadata);
      addTroopsByUnit(troopsByUnit, playerCastle.units.specialHospital, troopMetadata);
    }
    const wallet = playerWallet(state.player.resources, state.player.currencies, resourceMetadata);
    return {
      timestampUnix: Math.floor(Date.now() / 1000),
      playerId: state.player.id,
      might: finite(state.player.might),
      glory: finite(state.player.glory),
      gallantry: finite(state.player.gallantry),
      troopsTotal: stationed + traveling + hospital,
      troopsStationed: stationed,
      troopsTraveling: traveling,
      troopsHospital: hospital,
      troopsByUnit,
      coins: finite(wallet.coins),
      rubies: finite(wallet.rubies),
      currencies: wallet,
    };
  }, [gameLoggedIn, resourceMetadata, state, troopMetadata]);

  useEffect(() => {
	setTracker(emptyResponse);
	setLoading(true);
  }, [activePlayerID]);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const primarySeconds = ranges.find((range) => range.key === selectedRange)?.seconds ?? 365 * 24 * 60 * 60;
        const troopSeconds = ranges.find((range) => range.key === troopRange)?.seconds ?? 365 * 24 * 60 * 60;
        const rangeSeconds = Math.max(primarySeconds, troopSeconds);
		const response = await fetch(`/api/v2/history/player-tracker?rangeSeconds=${rangeSeconds}`, { cache: 'no-store' });
        if (!response.ok) throw new Error(`Tracker returned HTTP ${response.status}`);
        const payload = await response.json() as PlayerTrackerResponse;
        if (!active) return;
        const normalized = normalizeResponse(payload, metricDefinitions, troopMetadata);
		setTracker(activePlayerID > 0 && normalized.current?.playerId !== activePlayerID ? emptyResponse : normalized);
        setLoadError(null);
      } catch (error) {
        if (!active) return;
        setLoadError(error instanceof Error ? error.message : 'Could not load player history');
      } finally {
        if (active) setLoading(false);
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 60_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [activePlayerID, metricDefinitions, selectedRange, troopMetadata, troopRange]);

  const current = liveSample ?? scopedTracker.current;
  const series = useMemo(
    () => mergeLiveSampleIntoSeries(scopedTracker.series, liveSample, metricDefinitions),
    [scopedTracker.series, liveSample, metricDefinitions],
  );
  const visibleSeries = useMemo(
    () => filterSeriesRange(series, selectedRange, metricDefinitions),
    [series, selectedRange, metricDefinitions],
  );
  const selectedDefinition = metricDefinitions.find((definition) => definition.key === selectedMetric) ?? CORE_METRIC_DEFINITIONS[0];
  const selectedPoints = visibleSeries[selectedDefinition.key] ?? [];
  const chartPoints = useMemo(
    () => bucketMetricPoints(selectedPoints, selectedRange),
    [selectedPoints, selectedRange],
  );
  const displayedPoints = useMemo(
    () => customWindow
      ? chartPoints.filter((point) => point.timestampUnix >= customWindow.startUnix && point.timestampUnix <= customWindow.endUnix)
      : chartPoints,
    [chartPoints, customWindow],
  );
  const currentMetricPoint = displayedPoints[displayedPoints.length - 1];
  const currentForMetric = currentMetricPoint?.value ?? 0;
  const firstForMetric = displayedPoints[0]?.value ?? currentForMetric;
  const metricDelta = currentForMetric - firstForMetric;
  const troopFiltersActive = troopTypeFilter !== 'all' || troopRoleFilter !== 'all' || troopFoodFilter !== 'all';
  const troopPoints = useMemo(() => {
    const points = troopFiltersActive
      ? buildFilteredTroopPoints(
          scopedTracker.samples,
          current,
          troopMetadata,
          troopTypeFilter,
          troopRoleFilter,
          troopFoodFilter,
        )
      : series.troopsTotal ?? [];
    return filterMetricPointsRange(points, troopRange);
  }, [
    current,
    series.troopsTotal,
    scopedTracker.samples,
    troopFoodFilter,
    troopMetadata,
    troopRange,
    troopRoleFilter,
    troopTypeFilter,
    troopFiltersActive,
  ]);
  const troopChartPoints = useMemo(
    () => bucketMetricPoints(troopPoints, troopRange),
    [troopPoints, troopRange],
  );
  const displayedTroopPoints = useMemo(
    () => troopWindow
      ? troopChartPoints.filter((point) => point.timestampUnix >= troopWindow.startUnix && point.timestampUnix <= troopWindow.endUnix)
      : troopChartPoints,
    [troopChartPoints, troopWindow],
  );
  const currentTroopPoint = displayedTroopPoints[displayedTroopPoints.length - 1];
  const hasCurrentTroopValue = currentTroopPoint != null || (!troopFiltersActive && current != null);
  const currentTroopTotal = currentTroopPoint?.value ?? (!troopFiltersActive ? current?.troopsTotal ?? 0 : 0);
  const firstTroopTotal = displayedTroopPoints[0]?.value ?? currentTroopTotal;
  const troopDelta = currentTroopTotal - firstTroopTotal;
  const sampledTroopRatePoints = useMemo(() => {
    if (troopFiltersActive) {
      return buildFilteredTroopPoints(
        scopedTracker.samples,
        null,
        troopMetadata,
        troopTypeFilter,
        troopRoleFilter,
        troopFoodFilter,
      );
    }
    return scopedTracker.samples.flatMap((sample) => sample.troopsByUnit
      ? [{ timestampUnix: sample.timestampUnix, value: sample.troopsTotal, source: 'local' as const }]
      : []);
  }, [
    scopedTracker.samples,
    troopFiltersActive,
    troopFoodFilter,
    troopMetadata,
    troopRoleFilter,
    troopTypeFilter,
  ]);
  const latestTroopRatePoint = sampledTroopRatePoints[sampledTroopRatePoints.length - 1];
  const previousTroopRatePoint = sampledTroopRatePoints[sampledTroopRatePoints.length - 2];
  const troopRateValue = latestTroopRatePoint && previousTroopRatePoint
    ? latestTroopRatePoint.value - previousTroopRatePoint.value
    : undefined;
  const troopComposition = useMemo(
    () => calculateTroopCombatComposition(Object.values(state?.castles ?? {}), troopMetadata),
    [state?.castles, troopMetadata],
  );
  const troopTrendLines = useMemo(
    () => buildTroopTrendLines(
      scopedTracker.samples,
      current,
      troopMetadata,
      troopTypeFilter,
      troopRoleFilter,
      troopFoodFilter,
      troopRange,
    ),
    [
      current,
      scopedTracker.samples,
      troopFoodFilter,
      troopMetadata,
      troopRange,
      troopRoleFilter,
      troopTypeFilter,
    ],
  );
  useEffect(() => {
    setCustomWindow(null);
  }, [selectedMetric, selectedRange]);

  useEffect(() => {
    setTroopWindow(null);
  }, [troopRange, troopTypeFilter, troopRoleFilter, troopFoodFilter]);

  return (
    <div className="flex flex-col gap-6 pb-8">
      <StaleSessionBanner />

      <PageHeader
        title="My Stats"
        description={scopedTracker.fallback.playerName
          ? `${scopedTracker.fallback.playerName}${scopedTracker.fallback.server ? ` · ${scopedTracker.fallback.server}` : ''}`
          : current?.playerId ? `Your account · Player ${current.playerId}` : 'Your account analytics'}
        icon={<Activity className="h-6 w-6" />}
        meta={(
          <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="gap-1.5">
            <Clock3 className="h-3.5 w-3.5" />
            {formatSampleInterval(scopedTracker.intervalSeconds)} samples
          </Badge>
          {loading && <Badge variant="outline">Loading history…</Badge>}
          {(scopedTracker.fallback.status === 'backfilled' || scopedTracker.fallback.status === 'partial') && (
            <Badge variant="secondary">
              GGE Tracker backfill · {scopedTracker.fallback.pointsAdded ?? 0} points
            </Badge>
          )}
          {loadError && <Badge variant="danger">Live values only</Badge>}
          </div>
        )}
      />

      {!current ? (
        <Card>
          <CardContent className="flex min-h-56 items-center justify-center p-8 text-center text-text-muted">
            Connect the game once to begin collecting player analytics.
          </CardContent>
        </Card>
      ) : (
        <>
          <Card className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent flex-wrap gap-4">
              <div className="flex w-full flex-wrap items-center justify-between gap-4">
                <div>
                  <CardTitle className="flex items-center gap-2 text-lg">
                    <MetricIcon definition={selectedDefinition} className="h-5 w-5" />
                    {selectedDefinition.label} trend
                  </CardTitle>
                  <div className="mt-2 flex items-baseline gap-3">
                    <span className="font-mono text-3xl font-bold text-text-main">
                      {currentMetricPoint ? formatNumber(currentForMetric) : '—'}
                    </span>
                    {currentMetricPoint && (
                      <Delta
                        value={metricDelta}
                        base={firstForMetric}
                        startUnix={displayedPoints[0]?.timestampUnix}
                        endUnix={currentMetricPoint.timestampUnix}
                        range={selectedRange}
                      />
                    )}
                  </div>
                </div>
                <PillSelector
                  ariaLabel="Player history range"
                  value={selectedRange}
                  onChange={(value) => setSelectedRange(value as RangeKey)}
                  options={ranges.map((range) => ({ value: range.key, label: range.label }))}
                  size="header"
                />
              </div>
            </CardHeader>
            <CardContent className="liquid-prominent-header-content p-5 sm:p-6">
              <div className="mb-4 flex flex-wrap gap-2">
                <PillSelector
                  ariaLabel="Highlighted player metric"
                  value={selectedMetric}
                  onChange={(value) => setSelectedMetric(value as MetricKey)}
                  options={highlightMetricDefinitions.map((definition) => ({
                    value: definition.key,
                    label: definition.shortLabel,
                    icon: <MetricIcon definition={definition} className="h-4 w-4" />,
                  }))}
                  size="body"
                />
				{extraMetricDefinitions.length > 0 && (
					<Select
						value={selectedExtraMetric?.key ?? ''}
						onChange={setSelectedMetric}
						placeholder="More metrics"
						className="w-full sm:w-72"
						menuGrowToViewport
						searchable
						searchPlaceholder="Filter metrics"
						options={extraMetricDefinitions.map((definition) => ({
							value: definition.key,
							searchText: `${definition.label} ${definition.shortLabel} ${definition.category} ${definition.key}`,
							label: (
								<span className="flex min-w-0 items-center gap-2">
									<MetricIcon definition={definition} className="h-4 w-4" />
									<span className="min-w-0 flex-1 truncate">{definition.label}</span>
									<span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-text-muted">{definition.category}</span>
								</span>
							),
						}))}
					/>
				)}
              </div>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>{hoverPointHint(selectedRange)} Drag horizontally to inspect a custom time period.</span>
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
              <TrendChart
                points={chartPoints}
                metric={selectedDefinition.key}
                color={selectedDefinition.color}
                range={selectedRange}
                selectedWindow={customWindow}
                onWindowSelect={setCustomWindow}
              />
              {displayedPoints.some((point) => point.source === 'gge-tracker') && (
                <div className="mt-3 flex items-center gap-2 text-xs text-text-muted">
                  <span className="h-2 w-2 rounded-full bg-info" />
                  Missing intervals were filled by GGE Tracker; local observations take priority.
                </div>
              )}
              <div className="mt-3 flex justify-between text-xs text-text-muted">
                <span>{displayedPoints.length > 0 ? formatDate(displayedPoints[0].timestampUnix) : 'Waiting for history'}</span>
                <span>{displayedPoints.length} sample{displayedPoints.length === 1 ? '' : 's'}</span>
                <span>{displayedPoints.length > 0 ? formatDate(displayedPoints[displayedPoints.length - 1].timestampUnix) : 'Now'}</span>
              </div>
            </CardContent>
          </Card>

          <Card className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent flex-wrap gap-4">
              <div className="flex w-full flex-wrap items-center justify-between gap-4">
                <div>
                  <CardTitle className="flex items-center gap-2 text-lg">
                    <Swords className="h-5 w-5" style={{ color: troopMetricDefinition.color }} />
                    Troop strength
                  </CardTitle>
                  <p className="mt-1 text-xs font-medium text-text-muted">
                    {troopFilterLabel(troopTypeFilter, troopRoleFilter, troopFoodFilter)}
                  </p>
                  <div className="mt-2 flex items-baseline gap-3">
                    <span className="font-mono text-3xl font-bold text-text-main">
                      {hasCurrentTroopValue ? formatNumber(currentTroopTotal) : '—'}
                    </span>
                    {hasCurrentTroopValue && (
                      <Delta
                        value={troopDelta}
                        base={firstTroopTotal}
                        startUnix={displayedTroopPoints[0]?.timestampUnix}
                        endUnix={currentTroopPoint?.timestampUnix}
                        range={troopRange}
                        rateValue={troopRateValue}
                        rateStartUnix={previousTroopRatePoint?.timestampUnix}
                        rateEndUnix={latestTroopRatePoint?.timestampUnix}
                        rateUnitOverride="hr"
                      />
                    )}
                  </div>
                </div>
                <PillSelector
                  ariaLabel="Troop history range"
                  value={troopRange}
                  onChange={(value) => setTroopRange(value as RangeKey)}
                  options={ranges.map((range) => ({ value: range.key, label: range.label }))}
                  size="header"
                />
              </div>
            </CardHeader>
            <CardContent className="liquid-prominent-header-content p-5 sm:p-6">
              <div className="mb-5 rounded-2xl border border-border-base bg-bg-input/45 p-4">
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <p className="text-xs font-bold uppercase tracking-[0.18em] text-text-muted">Chart filters</p>
                    <p className="mt-1 text-xs text-text-muted">Type, role, and food filters combine together.</p>
                  </div>
                  {troopFiltersActive && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setTroopTypeFilter('all');
                        setTroopRoleFilter('all');
                        setTroopFoodFilter('all');
                      }}
                    >
                      Clear filters
                    </Button>
                  )}
                </div>
                <div className="flex flex-wrap gap-3">
                  <PillSelector
                    ariaLabel="Troop type filter"
                    value={troopTypeFilter}
                    onChange={(value) => setTroopTypeFilter(value as TypeFilter)}
                    options={[
                      { value: 'all', label: 'All Types' },
                      { value: 'melee', label: 'Melee' },
                      { value: 'range', label: 'Range' },
                    ]}
                    size="body"
                  />
                  <PillSelector
                    ariaLabel="Troop role filter"
                    value={troopRoleFilter}
                    onChange={(value) => setTroopRoleFilter(value as RoleFilter)}
                    options={[
                      { value: 'all', label: 'All Roles' },
                      { value: 'attack', label: 'Attack' },
                      { value: 'defense', label: 'Defense' },
                    ]}
                    size="body"
                  />
                  <PillSelector
                    ariaLabel="Troop food filter"
                    value={troopFoodFilter}
                    onChange={(value) => setTroopFoodFilter(value as FoodFilter)}
                    options={[
                      { value: 'all', label: 'All Food' },
                      { value: 'mead', label: 'Mead' },
                      { value: 'beef', label: 'Beef' },
                      { value: 'food', label: 'Food' },
                    ]}
                    size="body"
                  />
                </div>
              </div>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>{hoverPointHint(troopRange)} Drag horizontally to inspect a custom time period.</span>
                {troopWindow && (
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">
                      {formatDate(troopWindow.startUnix)} – {formatDate(troopWindow.endUnix)}
                    </Badge>
                    <Button variant="ghost" size="sm" onClick={() => setTroopWindow(null)}>
                      Clear selection
                    </Button>
                  </div>
                )}
              </div>
              <div className="mb-3 flex flex-wrap gap-2" aria-label="Troop trend lines">
                {troopTrendLines.map((line) => {
                  const legendPoints = line.displayPoints ?? line.points;
                  const latest = legendPoints[legendPoints.length - 1];
                  return (
                    <div key={line.key} className="flex items-center gap-2 rounded-full border border-border-base bg-bg-input/50 px-2.5 py-1.5 text-xs">
                      {line.unitID ? (
                        <UnitImage unitId={line.unitID} size={28} showLevel className="!bg-transparent" />
                      ) : (
                        <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: line.color }} />
                      )}
                      <span className="font-medium text-text-main">{line.label}</span>
                      <span className="font-mono text-text-muted">{latest ? formatNumber(latest.value) : '—'}</span>
                    </div>
                  );
                })}
              </div>
              <TrendChart
                points={troopChartPoints}
                metric="troopsTotal"
                color={troopMetricDefinition.color}
                lines={troopTrendLines}
                range={troopRange}
                selectedWindow={troopWindow}
                onWindowSelect={setTroopWindow}
                emptyMessage={troopFiltersActive
                  ? 'Filtered troop history will appear as detailed troop samples are collected.'
                  : undefined}
              />
              {troopTrendLines.length > 1 && (
                <p className="mt-3 text-xs text-text-muted">
                  Six stacked lines: the five highest-count matching units plus Other. The upper boundary equals the filtered total.
                </p>
              )}
              {troopFiltersActive && troopChartPoints.length < 2 && (
                <p className="mt-3 text-xs leading-5 text-text-muted">
                  Earlier total-only samples cannot be separated by category. New one-minute samples will build this filtered trend.
                </p>
              )}
              <div className="mt-3 flex justify-between text-xs text-text-muted">
                <span>{displayedTroopPoints.length > 0 ? formatDate(displayedTroopPoints[0].timestampUnix) : 'Waiting for history'}</span>
                <span>{displayedTroopPoints.length} sample{displayedTroopPoints.length === 1 ? '' : 's'}</span>
                <span>{displayedTroopPoints.length > 0 ? formatDate(displayedTroopPoints[displayedTroopPoints.length - 1].timestampUnix) : 'Now'}</span>
              </div>

              <div className="mt-7 border-t border-border-base pt-6">
                <div className="mb-4">
                  <h3 className="text-base font-semibold text-text-main">Combat composition</h3>
                  <p className="mt-1 text-sm text-text-muted">Current troops grouped by weapon type and battlefield role.</p>
                </div>
                <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                  <CombatCompositionCard
                    label="Melee"
                    value={troopComposition.melee}
                    total={troopComposition.typeClassified}
                    icon={Swords}
                    accentClass="text-primary"
                    barClass="bg-primary"
                  />
                  <CombatCompositionCard
                    label="Ranged"
                    value={troopComposition.ranged}
                    total={troopComposition.typeClassified}
                    icon={Crosshair}
                    accentClass="text-info"
                    barClass="bg-info"
                  />
                  <CombatCompositionCard
                    label="Attack"
                    value={troopComposition.attack}
                    total={troopComposition.roleClassified}
                    icon={TrendingUp}
                    accentClass="text-warning"
                    barClass="bg-warning"
                  />
                  <CombatCompositionCard
                    label="Defense"
                    value={troopComposition.defense}
                    total={troopComposition.roleClassified}
                    icon={Shield}
                    accentClass="text-success"
                    barClass="bg-success"
                  />
                </div>
                {(troopComposition.typeClassified < troopComposition.total || troopComposition.roleClassified < troopComposition.total) && (
                  <p className="mt-3 text-xs text-text-muted">
                    Composition includes troops recognized by the current game metadata; the total trend still includes every tracked troop.
                  </p>
                )}
              </div>
            </CardContent>
          </Card>

        </>
      )}
    </div>
  );
};

function Delta({
  value,
  base,
  startUnix,
  endUnix,
  range,
  rateValue,
  rateStartUnix,
  rateEndUnix,
  rateUnitOverride,
}: {
  value: number;
  base: number;
  startUnix?: number;
  endUnix?: number;
  range: RangeKey;
  rateValue?: number;
  rateStartUnix?: number;
  rateEndUnix?: number;
  rateUnitOverride?: 'hr' | 'day';
}) {
  const percentage = base === 0 ? null : (value / Math.abs(base)) * 100;
  const positive = value > 0;
  const negative = value < 0;
  const rateUnit = rateUnitOverride ?? (range === '24h' ? 'hr' : 'day');
  const effectiveRateUnitSeconds = rateUnit === 'hr' ? 60 * 60 : 24 * 60 * 60;
  const effectiveStartUnix = rateUnitOverride ? rateStartUnix : startUnix;
  const effectiveEndUnix = rateUnitOverride ? rateEndUnix : endUnix;
  const effectiveRateValue = rateUnitOverride ? rateValue : value;
  const elapsedSeconds = effectiveStartUnix != null && effectiveEndUnix != null ? effectiveEndUnix - effectiveStartUnix : 0;
  const rate = elapsedSeconds > 0 && effectiveRateValue != null
    ? effectiveRateValue / (elapsedSeconds / effectiveRateUnitSeconds)
    : null;
  return (
    <span className={`inline-flex items-center gap-1 text-xs font-semibold ${positive ? 'text-success' : negative ? 'text-error' : 'text-text-muted'}`}>
      {positive ? <TrendingUp className="h-3.5 w-3.5" /> : negative ? <TrendingDown className="h-3.5 w-3.5" /> : null}
      {value === 0 ? 'No change' : `${value > 0 ? '+' : ''}${formatNumber(value)}${percentage == null ? '' : ` (${percentage > 0 ? '+' : ''}${percentage.toFixed(1)}%)`}`}
      {rate != null && (
        <span className="ml-1 opacity-80">
          · Rate {rate > 0 ? '+' : ''}{formatRateValue(rate)}/{rateUnit}
        </span>
      )}
    </span>
  );
}

function TrendChart({
  points: metricPoints,
  metric,
  color,
  lines,
  range,
  selectedWindow,
  onWindowSelect,
  emptyMessage,
}: {
  points: TrackerMetricPoint[];
  metric: string;
  color: string;
  lines?: TrendLineSeries[];
  range: RangeKey;
  selectedWindow: ChartTimeWindow | null;
  onWindowSelect: (window: ChartTimeWindow) => void;
  emptyMessage?: string;
}) {
  const svgRef = useRef<SVGSVGElement | null>(null);
  const chartContainerRef = useRef<HTMLDivElement | null>(null);
  const [drag, setDrag] = useState<{ startX: number; currentX: number } | null>(null);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const [chartSize, setChartSize] = useState({ width: 1000, height: 482 });
  const sourceLines = lines?.length
    ? lines
    : [{
        key: metric,
        label: metric,
        color,
        points: metricPoints,
      }];
  const lineSignature = sourceLines.map((line) => line.key).join('|');
  const chartLines = sourceLines.map((line) => ({
    ...line,
    points: selectedWindow
      ? line.points.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
      : line.points,
    displayPoints: selectedWindow && line.displayPoints
      ? line.displayPoints.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
      : line.displayPoints,
  }));
  const referenceLineIndex = chartLines.length > 1 ? chartLines.length - 1 : 0;
  const filteredMetricPoints = selectedWindow
    ? metricPoints.filter((point) => point.timestampUnix >= selectedWindow.startUnix && point.timestampUnix <= selectedWindow.endUnix)
    : metricPoints;
  const chartPoints = lines?.length ? filteredMetricPoints : chartLines[referenceLineIndex]?.points ?? [];
  const hasRenderablePoints = chartPoints.length >= 2;

  useLayoutEffect(() => {
    const element = chartContainerRef.current;
    if (!element || !hasRenderablePoints) return;
    const updateSize = () => {
      const rect = element.getBoundingClientRect();
      const next = { width: Math.max(1, Math.round(rect.width)), height: Math.max(1, Math.round(rect.height)) };
      setChartSize((current) => current.width === next.width && current.height === next.height ? current : next);
    };
    updateSize();
    const observer = new ResizeObserver(updateSize);
    observer.observe(element);
    return () => observer.disconnect();
  }, [hasRenderablePoints]);

  const { width, height } = chartSize;
  const plotLeft = 96;
  const plotRight = width - 24;
  const plotTop = 20;
  const plotBottom = height - 54;
  const plotWidth = plotRight - plotLeft;

  useEffect(() => {
    setDrag(null);
    setHoveredIndex(null);
  }, [metric, range, selectedWindow, lineSignature]);

  if (!hasRenderablePoints) {
    return (
      <div className="flex h-[500px] items-center justify-center rounded-2xl border border-dashed border-border-base bg-bg-input/40 text-sm text-text-muted">
        {emptyMessage ?? 'The trend line will appear after the next sample is collected.'}
      </div>
    );
  }

  const domainStart = chartPoints[0].timestampUnix;
  const domainEnd = chartPoints[chartPoints.length - 1].timestampUnix;
  const domainSpan = Math.max(1, domainEnd - domainStart);
  const values = [
    ...chartLines.flatMap((line) => line.points.map((point) => finite(point.value))),
    ...(lines?.length ? chartPoints.map((point) => finite(point.value)) : []),
  ];
  const rawMin = chartLines.length > 1 ? 0 : Math.min(...values);
  const rawMax = Math.max(...values);
  const rawSpread = rawMax - rawMin || Math.max(1, Math.abs(rawMax) * 0.02);
  const verticalPadding = rawSpread * 0.08;
  const min = chartLines.length > 1 ? 0 : rawMin - verticalPadding;
  const max = rawMax + verticalPadding;
  const spread = max - min;
  const plotHeight = plotBottom - plotTop;
  const plottedLines = chartLines.map((line) => {
    const coords = line.points.map((point) => ({
      x: plotLeft + ((point.timestampUnix - domainStart) / domainSpan) * plotWidth,
      y: plotTop + ((max - finite(point.value)) / spread) * plotHeight,
    }));
    return {
      ...line,
      coords,
      polyline: coords.map((point) => `${point.x.toFixed(1)},${point.y.toFixed(1)}`).join(' '),
    };
  });
  const pointCoords = chartPoints.map((point) => ({
    x: plotLeft + ((point.timestampUnix - domainStart) / domainSpan) * plotWidth,
    y: plotTop + ((max - finite(point.value)) / spread) * plotHeight,
  }));
  const primaryPolyline = plottedLines[referenceLineIndex]?.polyline ?? '';
  const area = `${plotLeft},${plotBottom} ${primaryPolyline} ${plotRight},${plotBottom}`;
  const yTicks = Array.from({ length: 5 }, (_, index) => {
    const ratio = index / 4;
    return {
      value: max - spread * ratio,
      y: plotTop + plotHeight * ratio,
    };
  });
  const xTicks = Array.from({ length: 5 }, (_, index) => {
    const ratio = index / 4;
    return {
      timestampUnix: domainStart + domainSpan * ratio,
      x: plotLeft + plotWidth * ratio,
    };
  });
  const brushStart = drag ? Math.min(drag.startX, drag.currentX) : 0;
  const brushEnd = drag ? Math.max(drag.startX, drag.currentX) : 0;
  const hoveredPoint = hoveredIndex == null || hoveredIndex >= chartPoints.length
    ? null
    : { point: chartPoints[hoveredIndex], coord: pointCoords[hoveredIndex] };
  const hoveredLinePoints = hoveredPoint
    ? plottedLines.flatMap((line) => {
        const index = line.points.findIndex((point) => point.timestampUnix === hoveredPoint.point.timestampUnix);
        if (index < 0) return [];
        const displayPoint = line.displayPoints?.find((point) => point.timestampUnix === hoveredPoint.point.timestampUnix);
        return [{ line, point: displayPoint ?? line.points[index], coord: line.coords[index] }];
      })
    : [];

  const pointerX = (event: ReactPointerEvent<SVGSVGElement>) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect || rect.width <= 0) return plotLeft;
    const viewBoxX = ((event.clientX - rect.left) / rect.width) * width;
    return Math.max(plotLeft, Math.min(plotRight, viewBoxX));
  };

  const handlePointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (event.button !== 0) return;
    const x = pointerX(event);
    event.currentTarget.setPointerCapture(event.pointerId);
    setHoveredIndex(null);
    setDrag({ startX: x, currentX: x });
  };

  const nearestPointIndex = (x: number) => {
    let nearest = 0;
    let distance = Number.POSITIVE_INFINITY;
    for (let index = 0; index < pointCoords.length; index += 1) {
      const nextDistance = Math.abs(pointCoords[index].x - x);
      if (nextDistance < distance) {
        nearest = index;
        distance = nextDistance;
      }
    }
    return nearest;
  };

  const handlePointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const currentX = pointerX(event);
    if (drag) {
      setDrag((current) => current ? { ...current, currentX } : null);
      return;
    }
    setHoveredIndex(nearestPointIndex(currentX));
  };

  const finishPointerSelection = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (!drag) return;
    const endX = pointerX(event);
    const startX = Math.min(drag.startX, endX);
    const finishX = Math.max(drag.startX, endX);
    setDrag(null);
    setHoveredIndex(nearestPointIndex(endX));
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    if (finishX - startX < 24) return;
    const startUnix = domainStart + ((startX - plotLeft) / plotWidth) * domainSpan;
    const endUnix = domainStart + ((finishX - plotLeft) / plotWidth) * domainSpan;
    const pointsInWindow = chartPoints.filter((point) => point.timestampUnix >= startUnix && point.timestampUnix <= endUnix);
    if (pointsInWindow.length < 2) return;
    onWindowSelect({
      startUnix: pointsInWindow[0].timestampUnix,
      endUnix: pointsInWindow[pointsInWindow.length - 1].timestampUnix,
    });
  };

  return (
    <div className="h-[500px] overflow-hidden rounded-2xl border border-border-base bg-bg-input/40 p-2">
      <div ref={chartContainerRef} className="relative h-full w-full">
        <svg
          ref={svgRef}
          viewBox={`0 0 ${width} ${height}`}
          preserveAspectRatio="xMidYMid meet"
          className="h-full w-full cursor-crosshair touch-none select-none"
          role="img"
          aria-label={`${metric} trend chart. Hover to inspect a point or drag horizontally to select a custom time period.`}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={finishPointerSelection}
          onPointerCancel={() => { setDrag(null); setHoveredIndex(null); }}
          onPointerLeave={() => { if (!drag) setHoveredIndex(null); }}
        >
        <defs>
          <linearGradient id={`tracker-${metric}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.34" />
            <stop offset="100%" stopColor={color} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        {yTicks.map((tick) => (
          <g key={tick.y}>
            <line x1={plotLeft} x2={plotRight} y1={tick.y} y2={tick.y} stroke="currentColor" className="text-border-base" strokeDasharray="6 8" />
            <text x={plotLeft - 12} y={tick.y + 4} textAnchor="end" fill="currentColor" className="font-mono text-[12px] font-medium text-text-muted">
              {formatAxisValue(tick.value, rawSpread)}
            </text>
          </g>
        ))}
        {xTicks.map((tick, index) => (
          <g key={tick.x}>
            <line x1={tick.x} x2={tick.x} y1={plotTop} y2={plotBottom} stroke="currentColor" className="text-border-base/60" />
            <text
              x={tick.x}
              y={height - 18}
              textAnchor={index === 0 ? 'start' : index === xTicks.length - 1 ? 'end' : 'middle'}
              fill="currentColor"
              className="text-[12px] font-medium tracking-wide text-text-muted"
            >
              {formatAxisTime(tick.timestampUnix, domainSpan)}
            </text>
          </g>
        ))}
          {(plottedLines[referenceLineIndex]?.coords.length ?? 0) >= 2 && (
            <polygon points={area} fill={`url(#tracker-${metric})`} />
          )}
          {plottedLines.map((line, index) => (
            <g key={line.key}>
              <polyline
                points={line.polyline}
                fill="none"
                stroke={line.color}
                strokeWidth={index === referenceLineIndex ? 4 : 2.5}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeOpacity={index === referenceLineIndex ? 1 : 0.9}
                vectorEffect="non-scaling-stroke"
              />
              {line.coords.length === 1 && (
                <circle
                  cx={line.coords[0].x}
                  cy={line.coords[0].y}
                  r={index === referenceLineIndex ? 5 : 4}
                  fill={line.color}
                  stroke="white"
                  strokeWidth="2"
                  vectorEffect="non-scaling-stroke"
                />
              )}
            </g>
          ))}
          {hoveredPoint && !drag && (
            <g pointerEvents="none">
              <line
                x1={hoveredPoint.coord.x}
                x2={hoveredPoint.coord.x}
                y1={plotTop}
                y2={plotBottom}
                stroke={color}
                strokeWidth="1.5"
                strokeDasharray="5 6"
                vectorEffect="non-scaling-stroke"
              />
              <line
                x1={plotLeft}
                x2={plotRight}
                y1={hoveredPoint.coord.y}
                y2={hoveredPoint.coord.y}
                stroke={color}
                strokeOpacity="0.35"
                strokeWidth="1"
                vectorEffect="non-scaling-stroke"
              />
              {hoveredLinePoints.map(({ line, coord }, index) => (
                <circle
                  key={`${line.key}-${index}`}
                  cx={coord.x}
                  cy={coord.y}
                  r={line.key === plottedLines[referenceLineIndex]?.key ? 7 : 5}
                  fill={line.color}
                  stroke="white"
                  strokeWidth="2.5"
                  vectorEffect="non-scaling-stroke"
                />
              ))}
            </g>
          )}
        {drag && (
          <g pointerEvents="none">
            <rect
              x={brushStart}
              y={plotTop}
              width={Math.max(1, brushEnd - brushStart)}
              height={plotHeight}
              fill={color}
              fillOpacity="0.14"
            />
            <line x1={brushStart} x2={brushStart} y1={plotTop} y2={plotBottom} stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            <line x1={brushEnd} x2={brushEnd} y1={plotTop} y2={plotBottom} stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
          </g>
        )}
        </svg>
        {hoveredPoint && !drag && (
          <div
            className="pointer-events-none absolute z-10 min-w-52 rounded-xl border border-border-base bg-bg-card/95 px-3 py-2 text-xs shadow-xl"
            style={{
              left: `${(hoveredPoint.coord.x / width) * 100}%`,
              top: `${(hoveredPoint.coord.y / height) * 100}%`,
              transform: `translate(${hoveredPoint.coord.x > width * 0.72 ? '-100%' : '12px'}, ${hoveredPoint.coord.y > height * 0.36 ? '-112%' : '12px'})`,
            }}
          >
            <p className="mb-2 whitespace-nowrap font-medium text-text-muted">{formatHoverTime(hoveredPoint.point.timestampUnix, range)}</p>
            <div className="space-y-1.5">
              {hoveredLinePoints.map(({ line, point }) => (
                <div key={line.key} className="flex items-center justify-between gap-4">
                  <span className="flex min-w-0 items-center gap-2 text-text-main">
                    <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: line.color }} />
                    <span className="truncate">{line.label}</span>
                  </span>
                  <span className="font-mono font-semibold text-text-main">{formatNumber(point.value)}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function CombatCompositionCard({
  label,
  value,
  total,
  icon: Icon,
  accentClass,
  barClass,
}: {
  label: string;
  value: number;
  total: number;
  icon: typeof Activity;
  accentClass: string;
  barClass: string;
}) {
  const share = total > 0 ? (value / total) * 100 : 0;
  return (
    <div className="rounded-2xl border border-border-base bg-bg-input/50 p-4">
      <div className="flex items-center gap-2 text-sm font-semibold text-text-muted">
        <Icon className={`h-4 w-4 ${accentClass}`} />
        {label}
      </div>
      <p className="mt-3 font-mono text-xl font-bold text-text-main">{formatNumber(value)}</p>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-border-base/60">
        <div className={`h-full rounded-full ${barClass}`} style={{ width: `${Math.min(100, share)}%` }} />
      </div>
      <p className="mt-2 text-xs text-text-muted">{share.toFixed(1)}% of classified troops</p>
    </div>
  );
}

function buildTroopTrendLines(
  samples: PlayerTrackerSample[],
  current: PlayerTrackerSample | null,
  metadata: Record<number, MetadataItem>,
  typeFilter: TypeFilter,
  roleFilter: RoleFilter,
  foodFilter: FoodFilter,
  range: RangeKey,
): TrendLineSeries[] {
  const currentValues = current?.troopsByUnit;
  const rankedUnits = currentValues
    ? Object.entries(currentValues)
        .map(([unitIDText, rawCount]) => ({ unitID: Number(unitIDText), count: Math.max(0, finite(rawCount)) }))
        .filter(({ unitID, count }) => Number.isFinite(unitID)
          && count > 0
          && troopMatchesFilters(unitID, metadata, typeFilter, roleFilter, foodFilter))
        .sort((left, right) => right.count - left.count || left.unitID - right.unitID)
        .slice(0, 5)
    : [];

  const detailedSamples = new Map<number, PlayerTrackerSample>();
  for (const sample of samples) {
    if (sample.troopsByUnit) detailedSamples.set(sample.timestampUnix, sample);
  }
  if (current?.troopsByUnit) detailedSamples.set(current.timestampUnix, current);

  const unitPoints = new Map<number, TrackerMetricPoint[]>();
  for (const unit of rankedUnits) unitPoints.set(unit.unitID, []);
  const otherPoints: TrackerMetricPoint[] = [];

  for (const sample of [...detailedSamples.values()].sort((left, right) => left.timestampUnix - right.timestampUnix)) {
    let includedTotal = 0;
    for (const [unitIDText, rawCount] of Object.entries(sample.troopsByUnit ?? {})) {
      const unitID = Number(unitIDText);
      const count = Math.max(0, finite(rawCount));
      if (Number.isFinite(unitID) && count > 0 && troopMatchesFilters(unitID, metadata, typeFilter, roleFilter, foodFilter)) {
        includedTotal += count;
      }
    }

    let topUnitsTotal = 0;
    for (const unit of rankedUnits) {
      const value = Math.max(0, finite(sample.troopsByUnit?.[String(unit.unitID)]));
      topUnitsTotal += value;
      unitPoints.get(unit.unitID)?.push({ timestampUnix: sample.timestampUnix, value, source: 'local' });
    }
    otherPoints.push({
      timestampUnix: sample.timestampUnix,
      value: Math.max(0, includedTotal - topUnitsTotal),
      source: 'local',
    });
  }

  const colors = ['#34d399', '#60a5fa', '#f59e0b', '#fb7185', '#22d3ee'];
  const preparePoints = (points: TrackerMetricPoint[]) => bucketMetricPoints(filterMetricPointsRange(points, range), range);
  const rawLines: TrendLineSeries[] = [];

  rankedUnits.forEach((unit, index) => {
	const item = metadata[unit.unitID];
	const name = item?.name || `Unit #${unit.unitID}`;
	const rawLevel = Number(item?.level);
	const level = Number.isFinite(rawLevel) && rawLevel > 0 ? rawLevel : undefined;
    rawLines.push({
      key: `unit-${unit.unitID}`,
      label: level ? `${name} · Lv ${level}` : name,
      color: colors[index],
      points: preparePoints(unitPoints.get(unit.unitID) ?? []),
      unitID: unit.unitID,
    });
  });

  rawLines.push({
    key: 'troops-other',
    label: 'Other',
    color: '#94a3b8',
    points: preparePoints(otherPoints),
  });

  const cumulativeByTimestamp = new Map<number, number>();
  return rawLines.map((line) => {
    const displayPoints = line.points;
    const points = displayPoints.map((point) => {
      const value = (cumulativeByTimestamp.get(point.timestampUnix) ?? 0) + point.value;
      cumulativeByTimestamp.set(point.timestampUnix, value);
      return { ...point, value };
    });
    return { ...line, points, displayPoints };
  });
}

function buildFilteredTroopPoints(
  samples: PlayerTrackerSample[],
  current: PlayerTrackerSample | null,
  metadata: Record<number, MetadataItem>,
  typeFilter: TypeFilter,
  roleFilter: RoleFilter,
  foodFilter: FoodFilter,
): TrackerMetricPoint[] {
  const pointsByTimestamp = new Map<number, TrackerMetricPoint>();
  const appendSample = (sample: PlayerTrackerSample) => {
    if (!sample.troopsByUnit) return;
    let value = 0;
    for (const [unitIDText, rawAmount] of Object.entries(sample.troopsByUnit)) {
      const unitID = Number(unitIDText);
      const amount = Math.max(0, finite(rawAmount));
      if (!Number.isFinite(unitID) || amount <= 0) continue;
      if (troopMatchesFilters(unitID, metadata, typeFilter, roleFilter, foodFilter)) {
        value += amount;
      }
    }
    pointsByTimestamp.set(sample.timestampUnix, {
      timestampUnix: sample.timestampUnix,
      value,
      source: 'local',
    });
  };

  for (const sample of samples) appendSample(sample);
  if (current) appendSample(current);
  return [...pointsByTimestamp.values()].sort((left, right) => left.timestampUnix - right.timestampUnix);
}

function currencyValue(sample: PlayerTrackerSample | null, key: CurrencyKey): number | undefined {
  const detailedValue = sample?.currencies?.[key];
  if (typeof detailedValue === 'number' && Number.isFinite(detailedValue)) return detailedValue;
  if (!sample) return undefined;
  switch (key) {
    case 'coins':
      return finite(sample.coins);
    case 'rubies':
      return finite(sample.rubies);
    case 'might_pt':
      return finite(sample.might);
    case 'glory_pt':
      return finite(sample.glory);
    case 'gallan_pt':
      return finite(sample.gallantry);
    default:
      return undefined;
  }
}

function metricValue(sample: PlayerTrackerSample, key: MetricKey): number | undefined {
  switch (key) {
    case 'might':
      return finite(sample.might);
    case 'glory':
      return finite(sample.glory);
    case 'gallantry':
      return finite(sample.gallantry);
    case 'troopsTotal':
      return sample.troopsByUnit ? finite(sample.troopsTotal) : undefined;
    case 'loot':
      return undefined;
    default:
      return currencyValue(sample, key);
  }
}

function troopMatchesFilters(
  unitID: number,
  metadata: Record<number, MetadataItem>,
  typeFilter: TypeFilter,
  roleFilter: RoleFilter,
  foodFilter: FoodFilter,
): boolean {
	if (!metadata[unitID]) return false;
  return classificationMatchesFilters(
    troopUnitClassification(unitID, metadata),
    typeFilter,
    roleFilter,
    foodFilter,
  );
}

function troopUnitClassification(unitID: number, metadata: Record<number, MetadataItem>): TroopUnitClassification {
  const item = metadata[unitID];
	const inferredType = troopWeaponType(item);
	const weaponType = inferredType === 'ranged' ? 'range' : inferredType;
	const combatRole = troopCombatRole(item);
	const rawFood = metadataNumber(item?.meadSupply) > 0
	  ? 'mead'
	  : metadataNumber(item?.beefSupply) > 0 ? 'beef' : 'food';
  const foodType = rawFood === 'mead' || rawFood === 'beef' ? rawFood : 'food';
  return { weaponType, combatRole, foodType };
}

function classificationMatchesFilters(
  classification: TroopUnitClassification,
  typeFilter: TypeFilter,
  roleFilter: RoleFilter,
  foodFilter: FoodFilter,
): boolean {
  return (typeFilter === 'all' || classification.weaponType === typeFilter)
    && (roleFilter === 'all' || classification.combatRole === roleFilter)
    && (foodFilter === 'all' || classification.foodType === foodFilter);
}

function troopFilterLabel(typeFilter: TypeFilter, roleFilter: RoleFilter, foodFilter: FoodFilter): string {
  const labels: string[] = [];
  if (typeFilter === 'melee') labels.push('Melee');
  if (typeFilter === 'range') labels.push('Ranged');
  if (roleFilter === 'attack') labels.push('Attack');
  if (roleFilter === 'defense') labels.push('Defense');
  if (foodFilter === 'mead') labels.push('Mead');
  if (foodFilter === 'beef') labels.push('Beef');
  if (foodFilter === 'food') labels.push('Food');
  return labels.length > 0 ? labels.join(' · ') : 'All troops';
}

function addTroopsByUnit(
  target: Record<string, number>,
  values: Record<string, number> | undefined,
  metadata: Record<number, MetadataItem>,
) {
  if (!values) return;
  for (const [unitID, rawAmount] of Object.entries(values)) {
    if (!metadata[Number(unitID)]) continue;
    const amount = Math.max(0, finite(rawAmount));
    if (amount > 0) target[unitID] = (target[unitID] ?? 0) + amount;
  }
}

function playerWallet(
  resources: Record<string, number>,
  currencies: Record<string, number>,
  resourceDefinitions: Record<number, MetadataItem>,
): Record<string, number> {
  const wallet: Record<string, number> = {};
  for (const [rawID, amount] of Object.entries(resources)) {
    const definition = resourceDefinitions[Number(rawID)];
    const internalName = typeof definition?.internalName === 'string' ? definition.internalName : '';
    const jsonKey = typeof definition?.JSONKey === 'string' ? definition.JSONKey : '';
    wallet[`resource:${rawID}`] = amount;
    if (jsonKey === 'C1' || internalName === 'currency1') wallet.coins = amount;
    if (jsonKey === 'C2' || internalName === 'currency2') wallet.rubies = amount;
  }
  for (const [rawID, amount] of Object.entries(currencies)) {
    wallet[`currency:${rawID}`] = amount;
  }
  return wallet;
}

function buildMetricDefinitions(
  state: GameStateV2 | null,
  resourceDefinitions: Record<number, MetadataItem>,
  currencyDefinitions: Record<number, MetadataItem>,
): MetricDefinition[] {
  const definitions = CORE_METRIC_DEFINITIONS.map((definition) => ({ ...definition }));
  if (!state) return definitions;
  const palette = ['#22d3ee', '#a78bfa', '#f59e0b', '#34d399', '#fb7185', '#60a5fa', '#2dd4bf', '#f97316'];
  const append = (kind: 'resource' | 'currency', rawID: string, metadata: MetadataItem | undefined) => {
    const id = Number(rawID);
    if (!Number.isFinite(id) || id <= 0) return;
    const jsonKey = typeof metadata?.JSONKey === 'string' ? metadata.JSONKey : '';
    const internalName = typeof metadata?.internalName === 'string' ? metadata.internalName : '';
    if (kind === 'resource' && (jsonKey === 'C1' || jsonKey === 'C2' || internalName === 'currency1' || internalName === 'currency2')) return;
    const name = metadata?.name?.trim() || `${kind === 'resource' ? 'Resource' : 'Currency'} ${id}`;
    definitions.push({
      key: `${kind}:${id}`,
      label: name,
      shortLabel: name.length > 24 ? `${name.slice(0, 22)}…` : name,
      color: palette[Math.abs(id) % palette.length],
      icon: Coins,
      imageUrl: metadata?.image,
      category: 'Wallet',
    });
  };
  Object.entries(state.player.resources)
    .filter(([, amount]) => amount !== 0)
    .forEach(([id]) => append('resource', id, resourceDefinitions[Number(id)]));
  Object.entries(state.player.currencies)
    .filter(([, amount]) => amount !== 0)
    .forEach(([id]) => append('currency', id, currencyDefinitions[Number(id)]));
  const troop = definitions.find((definition) => definition.key === 'troopsTotal');
  return [
    ...definitions.filter((definition) => definition.key !== 'troopsTotal'),
    ...(troop ? [troop] : []),
  ];
}

function calculateTroopCombatComposition(
  castles: CastleStateV2[],
  metadata: Record<number, MetadataItem>,
): TroopCombatComposition {
  const composition: TroopCombatComposition = {
    total: 0,
    melee: 0,
    ranged: 0,
    attack: 0,
    defense: 0,
    typeClassified: 0,
    roleClassified: 0,
  };

  for (const castle of castles) {
    const troopPools = [
      castle.units.stationed,
      castle.units.traveling,
      castle.units.hospital,
      castle.units.specialHospital,
    ];
    for (const pool of troopPools) {
      if (!pool) continue;
      for (const [unitIDText, rawAmount] of Object.entries(pool)) {
        const unitID = Number(unitIDText);
        const amount = Math.max(0, finite(rawAmount));
        if (!Number.isFinite(unitID) || amount <= 0) continue;

        const item = metadata[unitID];
		if (!item) continue;
		composition.total += amount;
		const weaponType = troopWeaponType(item);
        if (weaponType === 'melee') {
          composition.melee += amount;
          composition.typeClassified += amount;
        } else if (weaponType === 'ranged') {
          composition.ranged += amount;
          composition.typeClassified += amount;
        }

		const combatRole = troopCombatRole(item);
        if (combatRole === 'attack') {
          composition.attack += amount;
          composition.roleClassified += amount;
        } else if (combatRole === 'defense') {
          composition.defense += amount;
          composition.roleClassified += amount;
        }
      }
    }
  }

  return composition;
}

function troopWeaponType(
	item: MetadataItem | undefined,
): 'melee' | 'ranged' | null {
  const value = String(item?.role ?? '').toLowerCase();
  if (value.includes('melee')) return 'melee';
  if (value.includes('range')) return 'ranged';
  return null;
}

function troopCombatRole(
	item: MetadataItem | undefined,
): 'attack' | 'defense' | null {
  const attack = Math.max(metadataNumber(item?.meleeAttack), metadataNumber(item?.rangeAttack));
  const defense = Math.max(metadataNumber(item?.meleeDefence), metadataNumber(item?.rangeDefence));
  if (attack > defense) return 'attack';
  if (defense > attack) return 'defense';
  return null;
}

function metadataNumber(value: unknown): number {
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : 0;
}

function normalizeResponse(
  value: PlayerTrackerResponse,
  definitions: MetricDefinition[],
  troopMetadata: Record<number, MetadataItem>,
): PlayerTrackerResponse {
	const hasTroopMetadata = Object.keys(troopMetadata).length > 0;
  const samples = Array.isArray(value?.samples)
    ? value.samples
        .filter((sample) => Number.isFinite(sample?.timestampUnix))
        .map((sample) => normalizeTroopSample(sample, troopMetadata, hasTroopMetadata))
        .sort((a, b) => a.timestampUnix - b.timestampUnix)
    : [];
	const current = value?.current ? normalizeTroopSample(value.current, troopMetadata, hasTroopMetadata) : null;
  return {
    current,
    samples,
    series: normalizeSeries(value?.series, samples, current, definitions),
    intervalSeconds: finite(value?.intervalSeconds) || 60,
    fallback: {
      provider: value?.fallback?.provider || 'gge-tracker',
      status: value?.fallback?.status || 'not-needed',
      server: value?.fallback?.server,
      playerName: value?.fallback?.playerName,
      fetchedAtUnix: finite(value?.fallback?.fetchedAtUnix) || undefined,
      pointsAdded: finite(value?.fallback?.pointsAdded) || undefined,
    },
    coverage: {
      loot: value?.coverage?.loot === true,
      eventScores: value?.coverage?.eventScores === true,
    },
  };
}

function normalizeTroopSample(
  sample: PlayerTrackerSample,
  metadata: Record<number, MetadataItem>,
	metadataReady = Object.keys(metadata).length > 0,
): PlayerTrackerSample {
	if (!sample.troopsByUnit || !metadataReady) return sample;
	const troopsByUnit: Record<string, number> = {};
	let troopsTotal = 0;
	for (const [rawID, rawAmount] of Object.entries(sample.troopsByUnit)) {
		if (!metadata[Number(rawID)]) continue;
		const amount = Math.max(0, finite(rawAmount));
		if (amount <= 0) continue;
		troopsByUnit[rawID] = amount;
		troopsTotal += amount;
	}
	return { ...sample, troopsTotal, troopsByUnit };
}

function normalizeSeries(
  raw: Partial<Record<MetricKey, TrackerMetricPoint[]>> | undefined,
  samples: PlayerTrackerSample[],
  current: PlayerTrackerSample | null,
  definitions: MetricDefinition[],
): Partial<Record<MetricKey, TrackerMetricPoint[]>> {
  const normalized: Partial<Record<MetricKey, TrackerMetricPoint[]>> = {};
  for (const definition of definitions) {
    const points = raw?.[definition.key];
    if (Array.isArray(points)) {
      normalized[definition.key] = points
        .filter((point) => Number.isFinite(point?.timestampUnix) && Number.isFinite(point?.value))
        .map((point) => ({
          timestampUnix: point.timestampUnix,
          value: point.value,
          source: point.source === 'gge-tracker' ? 'gge-tracker' : 'local',
        }))
        .sort((a, b) => a.timestampUnix - b.timestampUnix);
    }
  }
  const localSamples = current ? [...samples, current] : samples;
  for (const definition of definitions) {
    if (normalized[definition.key] != null) continue;
    normalized[definition.key] = localSamples.flatMap((sample) => {
      const value = metricValue(sample, definition.key);
      return value == null
        ? []
        : [{ timestampUnix: sample.timestampUnix, value, source: 'local' as const }];
    });
  }
  return normalized;
}

function mergeLiveSampleIntoSeries(
  series: Partial<Record<MetricKey, TrackerMetricPoint[]>>,
  liveSample: PlayerTrackerSample | null,
  definitions: MetricDefinition[],
): Partial<Record<MetricKey, TrackerMetricPoint[]>> {
  const merged: Partial<Record<MetricKey, TrackerMetricPoint[]>> = {};
  for (const definition of definitions) {
    merged[definition.key] = [...(series[definition.key] ?? [])];
  }
  if (liveSample) addSampleToSeries(merged, liveSample, definitions, true);
  return merged;
}

function addSampleToSeries(
  series: Partial<Record<MetricKey, TrackerMetricPoint[]>>,
  sample: PlayerTrackerSample,
  definitions: MetricDefinition[],
  replaceNearby = false,
) {
  for (const definition of definitions) {
    const value = metricValue(sample, definition.key);
    if (value == null) continue;
    let points = series[definition.key] ?? [];
    if (replaceNearby) {
      points = points.filter((point) => point.source !== 'local' || Math.abs(point.timestampUnix - sample.timestampUnix) > 30);
    }
    points.push({ timestampUnix: sample.timestampUnix, value, source: 'local' });
    points.sort((a, b) => a.timestampUnix - b.timestampUnix);
    series[definition.key] = points;
  }
}

function filterSeriesRange(
  series: Partial<Record<MetricKey, TrackerMetricPoint[]>>,
  range: RangeKey,
  definitions: MetricDefinition[],
): Partial<Record<MetricKey, TrackerMetricPoint[]>> {
  const definition = ranges.find((candidate) => candidate.key === range);
  if (!definition?.seconds) return series;
  const cutoff = Math.floor(Date.now() / 1000) - definition.seconds;
  const filtered: Partial<Record<MetricKey, TrackerMetricPoint[]>> = {};
  for (const metric of definitions) {
    filtered[metric.key] = (series[metric.key] ?? []).filter((point) => point.timestampUnix >= cutoff);
  }
  return filtered;
}

function filterMetricPointsRange(points: TrackerMetricPoint[], range: RangeKey): TrackerMetricPoint[] {
  const definition = ranges.find((candidate) => candidate.key === range);
  if (!definition?.seconds) return points;
  const cutoff = Math.floor(Date.now() / 1000) - definition.seconds;
  return points.filter((point) => point.timestampUnix >= cutoff);
}

function bucketMetricPoints(points: TrackerMetricPoint[], range: RangeKey): TrackerMetricPoint[] {
  const bucketSeconds = range === '24h'
    ? 60
    : range === '7d'
      ? 60 * 60
      : 24 * 60 * 60;
  const buckets = new Map<number, TrackerMetricPoint>();
  for (const point of points) {
    const bucket = Math.floor(point.timestampUnix / bucketSeconds);
    const existing = buckets.get(bucket);
    if (!existing || point.source === 'local' || existing.source !== 'local') {
      buckets.set(bucket, point);
    }
  }
  return [...buckets.values()].sort((left, right) => left.timestampUnix - right.timestampUnix);
}

function hoverPointHint(range: RangeKey): string {
  switch (range) {
    case '24h':
      return 'Hover for 1-minute points.';
    case '7d':
      return 'Hover for 1-hour points.';
    default:
      return 'Hover for 1-day points.';
  }
}

function sumTroopRecord(
  values: Record<string, number> | undefined,
  metadata: Record<number, MetadataItem>,
): number {
  if (!values) return 0;
  return Object.entries(values).reduce(
    (total, [rawID, value]) => total + (metadata[Number(rawID)] ? Math.max(0, finite(value)) : 0),
    0,
  );
}

function finite(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function formatNumber(value: number): string {
  return Math.round(finite(value)).toLocaleString();
}

function formatRateValue(value: number): string {
  const absolute = Math.abs(value);
  const maximumFractionDigits = absolute < 1 ? 2 : absolute < 10 ? 1 : 0;
  return value.toLocaleString(undefined, { maximumFractionDigits });
}

function formatAxisValue(value: number, spread: number): string {
  if (Math.abs(spread) < 10_000) {
    return Math.round(value).toLocaleString();
  }
  const ratio = Math.abs(value) / Math.max(1, Math.abs(spread));
  const maximumFractionDigits = ratio > 1_000 ? 3 : ratio > 100 ? 2 : 1;
  return new Intl.NumberFormat(undefined, {
    notation: 'compact',
    maximumFractionDigits,
  }).format(value);
}

function formatAxisTime(timestampUnix: number, domainSpanSec: number): string {
  const date = new Date(timestampUnix * 1000);
  if (domainSpanSec <= 2 * 24 * 60 * 60) {
    return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  }
  if (domainSpanSec <= 120 * 24 * 60 * 60) {
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
  return date.toLocaleDateString(undefined, { month: 'short', year: '2-digit' });
}

function formatHoverTime(timestampUnix: number, range: RangeKey): string {
  const date = new Date(timestampUnix * 1000);
  if (range === '24h') {
    return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
  }
  if (range === '7d') {
    return date.toLocaleString(undefined, { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric' });
  }
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

function formatSampleInterval(intervalSeconds: number): string {
  const minutes = Math.max(1, Math.round(finite(intervalSeconds) / 60));
  return `${minutes} minute${minutes === 1 ? '' : 's'}`;
}

function formatDate(timestampUnix: number): string {
  return new Date(timestampUnix * 1000).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
}

export default PlayerTrackerView;
