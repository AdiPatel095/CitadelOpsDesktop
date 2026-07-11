import { memo, useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Binoculars,
  Castle,
  ChevronLeft,
  ChevronRight,
  Crosshair,
  RefreshCw,
  Search,
  Users,
} from 'lucide-react';
import { FrontendWebsocket } from '../../Websocket';
import { Badge, Button, Card, CardHeader, CardTitle, Input, Select } from '../../components/ui';
import { SpyReportDetail, type SpyReport } from '../../spyReports/components/SpyReportsView';

interface AllianceOption {
  externalId: string;
  allianceId: number;
  name: string;
  rank: number;
  might: number;
  playerCount: number;
}

interface TargetCastle {
  castleId?: number;
  name: string;
  typeName?: string;
  x: number;
  y: number;
  type?: number;
}

interface AllianceTarget {
  playerId: number;
  name: string;
  might: number;
  underBird: boolean;
  rptSeconds: number;
  birdUntil?: string;
  targetCastle: TargetCastle;
  closestOwnCastle: TargetCastle;
  distance: number;
}

interface SpyAvailability {
  available: number;
  buildingRowsLoaded: boolean;
  sourceCastle: TargetCastle;
}

interface AllianceTargetViewData {
  alliances: AllianceOption[];
  selectedAlliance?: AllianceOption;
  targets: AllianceTarget[];
  spies: SpyAvailability;
}

type SortKey = 'player' | 'might' | 'rpt' | 'target' | 'closestCastle' | 'distance';
type SortDirection = 'asc' | 'desc';
type StatusFilter = 'all' | 'under-bird' | 'attackable';

const pageSize = 20;
const compactNumber = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 });
const statusFilterOptions = [
  { value: 'all', label: 'All targets' },
  { value: 'under-bird', label: 'Under bird' },
  { value: 'attackable', label: 'Attackable' },
];

const AllianceTargetsView = () => {
  const [data, setData] = useState<AllianceTargetViewData | null>(null);
  const [selectedAlliance, setSelectedAlliance] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const [sortKey, setSortKey] = useState<SortKey>('distance');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [sendingTarget, setSendingTarget] = useState('');
  const [loadingIntelTarget, setLoadingIntelTarget] = useState('');
  const [selectedSpyReport, setSelectedSpyReport] = useState<SpyReport | null>(null);

  useEffect(() => {
    const listener = (message: { type?: string; payload?: unknown }) => {
      if (message.type === 'allianceTargets') {
        const next = message.payload as AllianceTargetViewData;
        setData(next);
        setSelectedAlliance(next.selectedAlliance?.externalId ?? '');
        setPage(1);
        setLoading(false);
        return;
      }
      if (message.type === 'allianceTargetsError') {
        const payload = message.payload as { error?: string };
        FrontendWebsocket.showAlert('red', payload.error || 'Could not load alliance targets.');
        setLoading(false);
        return;
      }
      if (message.type === 'allianceTargetSpySent') {
        setSendingTarget('');
      }
    };

    FrontendWebsocket.addMessageListener(listener);
    FrontendWebsocket.sendGetAllianceTargets();
    return () => FrontendWebsocket.removeMessageListener(listener);
  }, []);

  const allianceOptions = useMemo(() => (data?.alliances ?? []).map((alliance) => ({
    value: alliance.externalId,
    label: `#${alliance.rank} ${alliance.name} · ${alliance.playerCount} players`,
  })), [data?.alliances]);

  const sortedTargets = useMemo(() => {
    const query = deferredSearch.trim().toLowerCase();
    const rows = (data?.targets ?? []).filter((target) => {
      const statusMatches = statusFilter === 'all' ||
        (statusFilter === 'under-bird' ? target.underBird : !target.underBird);
      if (!statusMatches) return false;
      if (!query) return true;
      return target.name.toLowerCase().includes(query) ||
        target.targetCastle.name.toLowerCase().includes(query) ||
        `${target.targetCastle.x}:${target.targetCastle.y}`.includes(query);
    });

    rows.sort((left, right) => {
      const comparison = compareTargets(left, right, sortKey);
      const directed = sortDirection === 'asc' ? comparison : -comparison;
      return directed || left.distance - right.distance || left.name.localeCompare(right.name);
    });
    return rows;
  }, [data?.targets, deferredSearch, sortDirection, sortKey, statusFilter]);

  const pageCount = Math.max(1, Math.ceil(sortedTargets.length / pageSize));
  const safePage = Math.min(page, pageCount);
  const pageTargets = useMemo(() => {
    const start = (safePage - 1) * pageSize;
    return sortedTargets.slice(start, start + pageSize);
  }, [safePage, sortedTargets]);

  useEffect(() => {
    setPage(1);
  }, [deferredSearch, selectedAlliance, sortDirection, sortKey, statusFilter]);

  const selectAlliance = useCallback((allianceId: string) => {
    setSelectedAlliance(allianceId);
    setLoading(true);
    FrontendWebsocket.sendGetAllianceTargets(allianceId);
  }, []);

  const refresh = useCallback(() => {
    setLoading(true);
    FrontendWebsocket.sendGetAllianceTargets(selectedAlliance);
  }, [selectedAlliance]);

  const changeSort = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortDirection((current) => current === 'asc' ? 'desc' : 'asc');
      return;
    }
    setSortKey(key);
    setSortDirection(key === 'might' || key === 'rpt' ? 'desc' : 'asc');
  }, [sortKey]);

  const sendSpy = useCallback((target: AllianceTarget) => {
    const targetKey = castleKey(target.targetCastle);
    setSendingTarget(targetKey);
    if (!FrontendWebsocket.sendAllianceTargetSpy(target.targetCastle.x, target.targetCastle.y)) {
      setSendingTarget('');
    }
  }, []);

  const openCastleIntel = useCallback(async (target: AllianceTarget) => {
    const targetKey = castleKey(target.targetCastle);
    setLoadingIntelTarget(targetKey);
    try {
      const response = await fetch('/api/spy-reports', { cache: 'no-cache' });
      if (!response.ok) throw new Error(`Spy reports request failed (${response.status})`);
      const reports = await response.json() as SpyReport[];
      const report = (Array.isArray(reports) ? reports : []).find((candidate) => {
        if (target.targetCastle.castleId && candidate.castle.id) {
          return candidate.castle.id === target.targetCastle.castleId;
        }
        return candidate.castle.x === target.targetCastle.x && candidate.castle.y === target.targetCastle.y;
      });
      if (!report) {
        FrontendWebsocket.showAlert('yellow', `No captured spy report is available for ${target.targetCastle.name || targetKey}.`);
        return;
      }
      setSelectedSpyReport(report);
    } catch (reason) {
      FrontendWebsocket.showAlert('red', reason instanceof Error ? reason.message : 'Could not load castle intelligence.');
    } finally {
      setLoadingIntelTarget('');
    }
  }, []);

  if (selectedSpyReport) {
    return <SpyReportDetail report={selectedSpyReport} onBack={() => setSelectedSpyReport(null)} />;
  }

  const spies = data?.spies;
  const canSpy = Boolean(spies?.buildingRowsLoaded && spies.available > 0 && spies.sourceCastle.castleId);
  const firstResult = sortedTargets.length === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const lastResult = Math.min(safePage * pageSize, sortedTargets.length);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="rounded-xl border border-primary/20 bg-primary/10 p-2.5 text-primary">
            <Crosshair className="h-5 w-5" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-text-main">Alliance Targets</h1>
            <p className="text-sm text-text-muted">Nearby castles from a top-50 alliance.</p>
          </div>
        </div>
        <div className="flex w-full flex-wrap items-center gap-2 xl:w-auto">
          <div className="min-w-[18rem] flex-1 xl:w-[28rem] xl:flex-none">
            <Select
              value={selectedAlliance}
              options={allianceOptions}
              onChange={selectAlliance}
              placeholder="Choose a top-50 alliance"
              icon={<Users className="h-4 w-4" />}
              disabled={allianceOptions.length === 0}
              menuGrowToViewport
            />
          </div>
          <div className="w-40">
            <Select
              value={statusFilter}
              options={statusFilterOptions}
              onChange={(value) => setStatusFilter(value as StatusFilter)}
              icon={<Crosshair className="h-4 w-4" />}
              menuGrowToViewport
            />
          </div>
          <Button
            variant="secondary"
            onClick={refresh}
            isLoading={loading}
            leftIcon={<RefreshCw className="h-4 w-4" />}
          >
            Refresh
          </Button>
        </div>
      </div>

      <Card variant="solid" className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
          <div>
            <CardTitle>{data?.selectedAlliance?.name || 'Player targets'}</CardTitle>
            <p className="mt-1 text-xs text-text-muted">
              {loading ? 'Loading targets…' : `${sortedTargets.length} matching castles`}
            </p>
          </div>
          <div className="w-full max-w-sm">
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Player, castle, or coordinates"
              leftIcon={<Search className="h-4 w-4" />}
            />
          </div>
        </CardHeader>

        <div className="liquid-prominent-header-content liquid-prominent-header-content-flush overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[980px] table-fixed text-left text-sm">
            <thead className="border-b border-border-base bg-bg-card/80 text-xs uppercase text-text-muted">
              <tr>
                <SortableHeader label="Player" column="player" width="w-[14%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Might" column="might" width="w-[9%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} align="right" />
                <SortableHeader label="Status" column="rpt" width="w-[14%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Target castle" column="target" width="w-[23%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Closest castle" column="closestCastle" width="w-[18%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Distance" column="distance" width="w-[9%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} align="right" />
                <th className="w-[13%] px-4 py-3 text-right font-semibold">Action</th>
              </tr>
            </thead>
              <tbody className="divide-y divide-border-base/60">
                {pageTargets.map((target) => {
                  const key = castleKey(target.targetCastle);
                  return (
                    <TargetRow
                      key={`${target.playerId}-${key}`}
                      target={target}
                      canSpy={canSpy}
                      sending={sendingTarget === key}
                      sendingBlocked={Boolean(sendingTarget)}
                      loadingIntel={loadingIntelTarget === key}
                      intelBlocked={Boolean(loadingIntelTarget)}
                      onSpy={sendSpy}
                      onIntel={openCastleIntel}
                    />
                  );
                })}
              </tbody>
            </table>
          </div>

          {!loading && sortedTargets.length === 0 && (
            <div className="px-5 py-12 text-center text-sm text-text-muted">No castles match the current filters.</div>
          )}

          {sortedTargets.length > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border-base bg-bg-card/35 px-4 py-3">
              <span className="text-xs text-text-muted">
                Showing {firstResult}–{lastResult} of {sortedTargets.length}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={safePage <= 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                  aria-label="Previous page"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <span className="min-w-20 text-center text-xs font-medium text-text-main">
                  Page {safePage} of {pageCount}
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={safePage >= pageCount}
                  onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
                  aria-label="Next page"
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
};

interface TargetRowProps {
  target: AllianceTarget;
  canSpy: boolean;
  sending: boolean;
  sendingBlocked: boolean;
  loadingIntel: boolean;
  intelBlocked: boolean;
  onSpy: (target: AllianceTarget) => void;
  onIntel: (target: AllianceTarget) => void;
}

const TargetRow = memo(({ target, canSpy, sending, sendingBlocked, loadingIntel, intelBlocked, onSpy, onIntel }: TargetRowProps) => (
  <tr className="hover:bg-bg-card-hover/35">
    <td className="truncate px-4 py-3 font-semibold text-text-main" title={target.name}>{target.name}</td>
    <td className="px-4 py-3 text-right font-medium tabular-nums">{compactNumber.format(target.might || 0)}</td>
    <td className="px-4 py-3">
      {target.underBird ? (
        <div className="space-y-1">
          <Badge variant="warning">Under bird</Badge>
          <div className="text-xs tabular-nums text-text-muted">{formatDuration(target.rptSeconds)}</div>
        </div>
      ) : (
        <Badge variant="success">Attackable</Badge>
      )}
    </td>
    <td className="px-4 py-3">
      <div className="flex min-w-0 items-center gap-2">
        <Castle className="h-4 w-4 shrink-0 text-primary" />
        <span className="truncate font-medium text-text-main" title={target.targetCastle.name || undefined}>
          {target.targetCastle.name || target.targetCastle.typeName || 'Player castle'}
        </span>
      </div>
      <div className="mt-1 text-xs text-text-muted">
        {target.targetCastle.typeName || 'Player castle'} · {target.targetCastle.x}:{target.targetCastle.y}
      </div>
    </td>
    <td className="px-4 py-3">
      <div className="truncate font-medium" title={target.closestOwnCastle.name}>{target.closestOwnCastle.name || 'Own castle'}</div>
      <div className="text-xs text-text-muted">{target.closestOwnCastle.x}:{target.closestOwnCastle.y}</div>
    </td>
    <td className="px-4 py-3 text-right font-semibold tabular-nums">{target.distance.toFixed(1)}</td>
    <td className="px-4 py-3 text-right">
      <div className="inline-flex items-center gap-1.5">
        <Button
          size="sm"
          variant="outline"
          disabled={!canSpy || target.underBird || sendingBlocked}
          isLoading={sending}
          onClick={() => onSpy(target)}
          leftIcon={<Binoculars className="h-4 w-4" />}
        >
          Spy
        </Button>
        <Button
          size="sm"
          variant="secondary"
          className="px-2.5"
          disabled={intelBlocked}
          isLoading={loadingIntel}
          onClick={() => onIntel(target)}
          title="Open latest spy intelligence"
          aria-label={`Open spy intelligence for ${target.targetCastle.name || `${target.targetCastle.x}:${target.targetCastle.y}`}`}
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </td>
  </tr>
));

TargetRow.displayName = 'TargetRow';

interface SortableHeaderProps {
  label: string;
  column: SortKey;
  activeColumn: SortKey;
  direction: SortDirection;
  onSort: (key: SortKey) => void;
  width: string;
  align?: 'left' | 'right';
}

const SortableHeader = ({ label, column, activeColumn, direction, onSort, width, align = 'left' }: SortableHeaderProps) => {
  const active = activeColumn === column;
  const Icon = active ? (direction === 'asc' ? ArrowUp : ArrowDown) : ArrowUpDown;
  return (
    <th className={`${width} px-4 py-3 font-semibold ${align === 'right' ? 'text-right' : ''}`}>
      <button
        type="button"
        onClick={() => onSort(column)}
        className={`inline-flex items-center gap-1.5 hover:text-primary ${active ? 'text-primary' : ''} ${align === 'right' ? 'ml-auto' : ''}`}
      >
        {label}
        <Icon className="h-3.5 w-3.5" />
      </button>
    </th>
  );
};

function castleKey(castle: TargetCastle): string {
  return `${castle.x}:${castle.y}`;
}

function formatDuration(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds || 0));
  const days = Math.floor(safe / 86400);
  const hours = Math.floor((safe % 86400) / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h remaining`;
  if (hours > 0) return `${hours}h ${minutes}m remaining`;
  return `${minutes}m remaining`;
}

function compareTargets(left: AllianceTarget, right: AllianceTarget, key: SortKey): number {
  switch (key) {
    case 'player':
      return left.name.localeCompare(right.name);
    case 'might':
      return left.might - right.might;
    case 'rpt':
      return left.rptSeconds - right.rptSeconds;
    case 'target':
      return (left.targetCastle.name || left.targetCastle.typeName || '').localeCompare(
        right.targetCastle.name || right.targetCastle.typeName || ''
      ) || left.targetCastle.x - right.targetCastle.x || left.targetCastle.y - right.targetCastle.y;
    case 'closestCastle':
      return left.closestOwnCastle.name.localeCompare(right.closestOwnCastle.name) ||
        left.closestOwnCastle.x - right.closestOwnCastle.x || left.closestOwnCastle.y - right.closestOwnCastle.y;
    case 'distance':
      return left.distance - right.distance;
  }
}

export default AllianceTargetsView;
