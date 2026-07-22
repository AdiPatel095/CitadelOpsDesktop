import { memo, useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
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
import { Badge, Button, Input, PageHeader, SectionCard, Select } from '../../components/ui';
import { SpyReportDetail, type SpyReport } from '../../spyReports/components/SpyReportsView';
import { useCitadelAPI } from '../../api/ApiContext';
import type {
  AllianceTargetCastleV2,
  AllianceTargetQueryV2,
  AllianceTargetV2,
  AllianceTargetViewV2,
} from '../../api/Contracts';

type TargetCastle = AllianceTargetCastleV2;
type AllianceTarget = AllianceTargetV2;
type AllianceTargetViewData = AllianceTargetViewV2;

type SortKey = 'player' | 'might' | 'rpt' | 'target' | 'closestCastle' | 'distance';
type SortDirection = 'asc' | 'desc';
type StatusFilter = 'all' | 'under-bird' | 'attackable';
interface TargetQueryState {
  search: string;
  status: StatusFilter;
  sort: SortKey;
  direction: SortDirection;
  page: number;
}

const defaultPageSize = 20;
const initialTargetQuery: TargetQueryState = {
  search: '', status: 'all', sort: 'distance', direction: 'asc', page: 1,
};
const compactNumber = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 });
const statusFilterOptions = [
  { value: 'all', label: 'All targets' },
  { value: 'under-bird', label: 'Under bird' },
  { value: 'attackable', label: 'Attackable' },
];

const AllianceTargetsView = () => {
  const { state, submitIntent, getAllianceTargets } = useCitadelAPI();

  return (
    <AllianceTargetsContent
      getAllianceTargets={getAllianceTargets}
      serverUrl={state?.session.serverUrl ?? ''}
      submitIntent={submitIntent}
    />
  );
};

interface AllianceTargetsContentProps {
  getAllianceTargets: ReturnType<typeof useCitadelAPI>['getAllianceTargets'];
  serverUrl: string;
  submitIntent: ReturnType<typeof useCitadelAPI>['submitIntent'];
}

const AllianceTargetsContent = memo(({
  getAllianceTargets,
  serverUrl,
  submitIntent,
}: AllianceTargetsContentProps) => {
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
  const [viewError, setViewError] = useState('');
  const selectedServerRef = useRef('');
  const selectedAllianceRef = useRef('');
  const queryRef = useRef<TargetQueryState>({ ...initialTargetQuery });
  const latestRequestRef = useRef(0);

  const loadTargets = useCallback(async (
    allianceId: string,
    refresh: boolean,
    inspect: boolean,
    query: TargetQueryState,
    includeAlliances = false,
  ) => {
    const requestID = ++latestRequestRef.current;
    setLoading(true);
    setViewError('');
    try {
      const request: AllianceTargetQueryV2 = {
        allianceId,
        server: selectedServerRef.current,
        refresh,
        search: query.search,
        status: query.status,
        sort: query.sort,
        direction: query.direction,
        page: query.page,
        includeAlliances,
      };
      let next = await getAllianceTargets(request);
      if (requestID !== latestRequestRef.current) return;
      let inspectError = '';
      const selected = next.selectedAlliance;
      if (inspect && selected && next.canInspect) {
        try {
          await submitIntent('alliance.inspect', { allianceId: selected.allianceId });
          if (requestID !== latestRequestRef.current) return;
          const inspected = await getAllianceTargets({
            ...request,
            allianceId: selected.externalId,
            server: next.server,
            refresh: true,
            includeAlliances: false,
          });
          next = { ...inspected, alliances: next.alliances };
        } catch (error) {
          inspectError = error instanceof Error ? error.message : 'Live alliance inspection failed; showing tracker data.';
        }
      }
      if (requestID !== latestRequestRef.current) return;
      selectedServerRef.current = next.server;
      selectedAllianceRef.current = next.selectedAlliance?.externalId ?? '';
      queryRef.current = { ...query, page: next.page };
      setData((current) => ({
        ...next,
        alliances: includeAlliances ? next.alliances : (current?.alliances ?? next.alliances),
      }));
      setSelectedAlliance(selectedAllianceRef.current);
      setPage(next.page);
      if (inspectError) setViewError(inspectError);
    } catch (error) {
      if (requestID !== latestRequestRef.current) return;
      setViewError(error instanceof Error ? error.message : 'Could not load alliance targets.');
    } finally {
      if (requestID === latestRequestRef.current) setLoading(false);
    }
  }, [getAllianceTargets, submitIntent]);

  useEffect(() => {
    selectedServerRef.current = '';
    selectedAllianceRef.current = '';
    const query = { ...queryRef.current, page: 1 };
    queryRef.current = query;
    setPage(1);
    void loadTargets('', false, true, query, true);
  }, [loadTargets, serverUrl]);

  const allianceOptions = useMemo(() => (data?.alliances ?? []).map((alliance) => ({
    value: alliance.externalId,
    label: `#${alliance.rank} ${alliance.name} · ${alliance.playerCount} players`,
  })), [data?.alliances]);

  useEffect(() => {
    const normalizedSearch = deferredSearch.trim();
    if (normalizedSearch === queryRef.current.search) return;
    const timeout = window.setTimeout(() => {
      const query = { ...queryRef.current, search: normalizedSearch, page: 1 };
      queryRef.current = query;
      setPage(1);
      void loadTargets(selectedAllianceRef.current, false, false, query);
    }, 200);
    return () => window.clearTimeout(timeout);
  }, [deferredSearch, loadTargets]);

  const selectAlliance = useCallback((allianceId: string) => {
    selectedAllianceRef.current = allianceId;
    setSelectedAlliance(allianceId);
    const query = { ...queryRef.current, page: 1 };
    queryRef.current = query;
    setPage(1);
    void loadTargets(allianceId, false, true, query);
  }, [loadTargets]);

  const refresh = useCallback(() => {
    void loadTargets(selectedAllianceRef.current, true, true, queryRef.current, true);
  }, [loadTargets]);

  const changeStatus = useCallback((value: string) => {
    const status = value as StatusFilter;
    const query = { ...queryRef.current, status, page: 1 };
    queryRef.current = query;
    setStatusFilter(status);
    setPage(1);
    void loadTargets(selectedAllianceRef.current, false, false, query);
  }, [loadTargets]);

  const changeSort = useCallback((key: SortKey) => {
    const direction = queryRef.current.sort === key
      ? (queryRef.current.direction === 'asc' ? 'desc' : 'asc')
      : (key === 'might' || key === 'rpt' ? 'desc' : 'asc');
    const query = { ...queryRef.current, sort: key, direction, page: 1 };
    queryRef.current = query;
    setSortKey(key);
    setSortDirection(direction);
    setPage(1);
    void loadTargets(selectedAllianceRef.current, false, false, query);
  }, [loadTargets]);

  const changePage = useCallback((nextPage: number) => {
    const query = { ...queryRef.current, page: nextPage };
    queryRef.current = query;
    setPage(nextPage);
    void loadTargets(selectedAllianceRef.current, false, false, query);
  }, [loadTargets]);

  const sendSpy = useCallback((target: AllianceTarget) => {
    const targetKey = castleKey(target.targetCastle);
    setSendingTarget(targetKey);
    setViewError('');
    void submitIntent('spy.launch', {
      sourceCastleId: data?.spies.sourceCastleId,
      targetX: target.targetCastle.x,
      targetY: target.targetCastle.y,
      kingdomId: 0,
      spyCount: Math.max(1, data?.spies.available ?? 1),
    })
      .catch((error) => setViewError(error instanceof Error ? error.message : 'Could not launch spy mission'))
      .finally(() => setSendingTarget(''));
  }, [data?.spies.available, data?.spies.sourceCastleId, submitIntent]);

  const openCastleIntel = useCallback(async (target: AllianceTarget) => {
    const targetKey = castleKey(target.targetCastle);
    setLoadingIntelTarget(targetKey);
    setViewError('');
    try {
      const response = await fetch('/api/v2/history/spy-reports', { cache: 'no-cache' });
      if (!response.ok) throw new Error(`Spy reports request failed (${response.status})`);
      const reports = await response.json() as SpyReport[];
      const report = (Array.isArray(reports) ? reports : []).find((candidate) => {
        if (target.targetCastle.castleId && candidate.castle.id) {
          return candidate.castle.id === target.targetCastle.castleId;
        }
        return candidate.castle.x === target.targetCastle.x && candidate.castle.y === target.targetCastle.y;
      });
      if (!report) {
        setViewError(`No normalized spy report has been captured for ${target.targetCastle.name || targetKey}.`);
        return;
      }
      setSelectedSpyReport(report);
    } catch (error) {
      setViewError(error instanceof Error ? error.message : 'Could not load castle intelligence.');
    } finally {
      setLoadingIntelTarget('');
    }
  }, []);

  if (selectedSpyReport) {
    return <SpyReportDetail report={selectedSpyReport} onBack={() => setSelectedSpyReport(null)} />;
  }

  const spies = data?.spies;
  const canSpy = spies?.canLaunch === true;
  const totalTargets = data?.totalTargets ?? 0;
  const safePage = data?.page ?? page;
  const pageSize = data?.pageSize ?? defaultPageSize;
  const pageCount = data?.pageCount ?? 1;
  const pageTargets = data?.targets ?? [];
  const firstResult = totalTargets === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const lastResult = Math.min(safePage * pageSize, totalTargets);

  return (
    <div className="data-view-render-stable space-y-4">
      {viewError && <p className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error">{viewError}</p>}
      <PageHeader
        title="Alliance Targets"
        description="Nearby castles from a top-50 alliance."
        icon={<Crosshair className="h-5 w-5" />}
        actions={(
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
              onChange={changeStatus}
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
        )}
      />

      <SectionCard
        title={data?.selectedAlliance?.name || 'Player targets'}
        description={loading ? 'Loading targets…' : `${totalTargets} matching castles`}
        descriptionClassName=""
        headerClassName="flex-wrap gap-3"
        actions={<div className="w-full max-w-sm">
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Player, castle, or coordinates"
            leftIcon={<Search className="h-4 w-4" />}
          />
        </div>}
        contentClassName="overflow-hidden"
        flush
      >
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
                    key={key}
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

        {!loading && totalTargets === 0 && (
          <div className="px-5 py-12 text-center text-sm text-text-muted">No castles match the current filters.</div>
        )}

        {totalTargets > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border-base bg-bg-card/35 px-4 py-3">
            <span className="text-xs text-text-muted">
              Showing {firstResult}–{lastResult} of {totalTargets}
            </span>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                disabled={loading || safePage <= 1}
                onClick={() => changePage(Math.max(1, safePage - 1))}
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
                disabled={loading || safePage >= pageCount}
                onClick={() => changePage(Math.min(pageCount, safePage + 1))}
                aria-label="Next page"
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </SectionCard>
    </div>
  );
});

AllianceTargetsContent.displayName = 'AllianceTargetsContent';

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

export default AllianceTargetsView;
