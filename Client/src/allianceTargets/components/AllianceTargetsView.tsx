import { memo, useCallback, useDeferredValue, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
	AlertTriangle,
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Binoculars,
  Castle,
  ChevronLeft,
  ChevronRight,
	Clock3,
  Crosshair,
	FileSearch,
	MapPin,
  RefreshCw,
  Search,
	ShieldCheck,
	Swords,
  Users,
} from 'lucide-react';
import { Badge, Button, Input, Modal, ModalTitle, PageHeader, SectionCard, Select } from '../../components/ui';
import { SpyReportDetail, type SpyReport } from '../../spyReports/components/SpyReportsView';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import {
	ATTACK_PRESETS_SECTION,
	parseAttackPresetDocument,
	summarizeAttackPreset,
} from '../../attackPresets/AttackPresetTypes';
import { Notifications } from '../../components/Notifications';
import type {
  AllianceTargetAttackPreviewV2,
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
	const [attackTarget, setAttackTarget] = useState<AllianceTarget | null>(null);
	const spyCaptureVersion = useMemo(() => Object.values(state?.reports.spyCaptures ?? {})
		.map((capture) => `${capture.messageId}:${capture.capturedAt}`)
		.sort()
		.join('|'), [state?.reports.spyCaptures]);

  return (
	<>
	  <AllianceTargetsContent
		getAllianceTargets={getAllianceTargets}
		serverUrl={state?.session.serverUrl ?? ''}
		spyCaptureVersion={spyCaptureVersion}
		submitIntent={submitIntent}
		onAttack={setAttackTarget}
	  />
	  {attackTarget ? (
		<AllianceTargetAttackModal
		  key={castleKey(attackTarget.targetCastle)}
		  target={attackTarget}
		  onClose={() => setAttackTarget(null)}
		/>
	  ) : null}
	</>
  );
};

interface AllianceTargetsContentProps {
  getAllianceTargets: ReturnType<typeof useCitadelAPI>['getAllianceTargets'];
  serverUrl: string;
  spyCaptureVersion: string;
  submitIntent: ReturnType<typeof useCitadelAPI>['submitIntent'];
	onAttack: (target: AllianceTarget) => void;
}

const AllianceTargetsContent = memo(({
  getAllianceTargets,
  serverUrl,
  spyCaptureVersion,
  submitIntent,
	onAttack,
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
	const observedSpyCaptureVersionRef = useRef(spyCaptureVersion);

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

	useEffect(() => {
		if (observedSpyCaptureVersionRef.current === spyCaptureVersion) return;
		observedSpyCaptureVersionRef.current = spyCaptureVersion;
		if (!selectedAllianceRef.current) return;
		void loadTargets(selectedAllianceRef.current, false, false, queryRef.current);
	}, [loadTargets, spyCaptureVersion]);

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
      const response = await fetch('/api/v2/history/spy-reports?limit=10000', { cache: 'no-cache' });
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
          <table className="w-full min-w-[1240px] table-fixed text-left text-sm">
            <thead className="border-b border-border-base bg-bg-card/80 text-xs uppercase text-text-muted">
              <tr>
                <SortableHeader label="Player" column="player" width="w-[12%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Might" column="might" width="w-[8%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} align="right" />
                <SortableHeader label="Status" column="rpt" width="w-[11%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Target castle" column="target" width="w-[19%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <th className="w-[14%] px-4 py-3 font-semibold">Spy report</th>
                <SortableHeader label="Closest castle" column="closestCastle" width="w-[14%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} />
                <SortableHeader label="Distance" column="distance" width="w-[7%]" activeColumn={sortKey} direction={sortDirection} onSort={changeSort} align="right" />
                <th className="w-[15%] px-4 py-3 text-right font-semibold">Action</th>
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
					onAttack={onAttack}
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
	onAttack: (target: AllianceTarget) => void;
}

const TargetRow = memo(({ target, canSpy, sending, sendingBlocked, loadingIntel, intelBlocked, onSpy, onIntel, onAttack }: TargetRowProps) => (
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
	  {target.spyReport ? (
		<button
		  type="button"
		  className="group w-full rounded-global border border-border-base bg-bg-card/45 px-2.5 py-2 text-left transition hover:border-primary/45 hover:bg-primary/6 disabled:cursor-wait disabled:opacity-60"
		  disabled={intelBlocked}
		  onClick={() => onIntel(target)}
		  title="Open latest spy intelligence"
		>
		  <span className="flex items-center justify-between gap-2">
			<span className="inline-flex items-center gap-1.5 text-xs font-bold text-text-main group-hover:text-primary">
			  <FileSearch className="h-3.5 w-3.5" />
			  {loadingIntel ? 'Loading…' : `${target.spyReport.totalTroops.toLocaleString()} troops`}
			</span>
			<Badge variant={target.spyReport.status === 'success' ? 'success' : 'warning'} className="px-1.5 py-0.5 normal-case tracking-normal">
			  {target.spyReport.status || 'captured'}
			</Badge>
		  </span>
		  <span className="mt-1 flex items-center gap-1 text-[11px] tabular-nums text-text-muted">
			<Clock3 className="h-3 w-3" />
			{formatReportAge(target.spyReport.capturedAtUnixMillis)}
		  </span>
		</button>
	  ) : (
		<div className="flex items-center gap-1.5 text-xs text-text-muted">
		  <FileSearch className="h-3.5 w-3.5 opacity-60" />
		  No report
		</div>
	  )}
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
		  onClick={() => onAttack(target)}
		  leftIcon={<Swords className="h-4 w-4" />}
        >
		  Attack
        </Button>
      </div>
    </td>
  </tr>
));

TargetRow.displayName = 'TargetRow';

interface AllianceTargetAttackModalProps {
	target: AllianceTarget;
	onClose: () => void;
}

interface PresetRequirement {
	kind: 'Troop' | 'Tool';
	itemId: number;
	name: string;
	requested: number;
	available: number;
}

const AllianceTargetAttackModal = ({ target, onClose }: AllianceTargetAttackModalProps) => {
	const { state, configuration, previewAllianceTargetAttack, submitIntent } = useCitadelAPI();
	const { troops, tools } = useMetadata();
	const [sourceCastleID, setSourceCastleID] = useState('');
	const [presetID, setPresetID] = useState('');
	const [preview, setPreview] = useState<AllianceTargetAttackPreviewV2 | null>(null);
	const [previewLoading, setPreviewLoading] = useState(false);
	const [previewError, setPreviewError] = useState('');
	const [launching, setLaunching] = useState(false);
	const [launchError, setLaunchError] = useState('');
	const latestPreviewRequestRef = useRef(0);

	const sourceCastles = useMemo(() => Object.values(state?.castles ?? {})
		.filter((castle) => castle.id > 0 && castle.kingdomId === 0 && (castle.x !== 0 || castle.y !== 0))
		.sort((left, right) => {
			const leftDistance = routeDistance(left.x, left.y, target.targetCastle.x, target.targetCastle.y);
			const rightDistance = routeDistance(right.x, right.y, target.targetCastle.x, target.targetCastle.y);
			return leftDistance - rightDistance || (left.name ?? '').localeCompare(right.name ?? '') || left.id - right.id;
		}), [state?.castles, target.targetCastle.x, target.targetCastle.y]);
	const document = useMemo(
		() => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
		[configuration?.sections],
	);

	useEffect(() => {
		setSourceCastleID((current) => {
			if (current && sourceCastles.some((castle) => String(castle.id) === current)) return current;
			const closest = sourceCastles.find((castle) =>
				castle.x === target.closestOwnCastle.x && castle.y === target.closestOwnCastle.y);
			return closest ? String(closest.id) : sourceCastles[0] ? String(sourceCastles[0].id) : '';
		});
	}, [sourceCastles, target.closestOwnCastle.x, target.closestOwnCastle.y]);

	useEffect(() => {
		setPresetID((current) => current && document.presets.some((preset) => preset.id === current) ? current : '');
	}, [document.presets]);

	const sourceCastle = sourceCastles.find((castle) => String(castle.id) === sourceCastleID);
	const preset = document.presets.find((candidate) => candidate.id === presetID);
	const distance = sourceCastle
		? routeDistance(sourceCastle.x, sourceCastle.y, target.targetCastle.x, target.targetCastle.y)
		: null;

	useEffect(() => {
		const requestID = ++latestPreviewRequestRef.current;
		setPreview(null);
		setPreviewError('');
		if (!sourceCastle || !preset) {
			setPreviewLoading(false);
			return;
		}
		if (!target.targetCastle.typeId || !target.level) {
			setPreviewLoading(false);
			setPreviewError('Target level or castle type is unavailable, so CRA caps cannot be calculated.');
			return;
		}
		setPreviewLoading(true);
		void previewAllianceTargetAttack({
			sourceCastleId: sourceCastle.id,
			kingdomId: 0,
			targetX: target.targetCastle.x,
			targetY: target.targetCastle.y,
			targetCastleId: target.targetCastle.castleId,
			targetTypeId: target.targetCastle.typeId,
			targetLevel: target.level,
			targetLegendLevel: target.legendLevel,
			preset: { id: preset.id, name: preset.name, waves: preset.waves },
		})
			.then((nextPreview) => {
				if (requestID !== latestPreviewRequestRef.current) return;
				setPreview(nextPreview);
			})
			.catch((error) => {
				if (requestID !== latestPreviewRequestRef.current) return;
				setPreviewError(error instanceof Error ? error.message : 'Could not calculate CRA formation caps.');
			})
			.finally(() => {
				if (requestID === latestPreviewRequestRef.current) setPreviewLoading(false);
			});
		return () => {
			if (latestPreviewRequestRef.current === requestID) latestPreviewRequestRef.current += 1;
		};
	}, [
		preset,
		previewAllianceTargetAttack,
		sourceCastle?.id,
		state?.revision,
		target.legendLevel,
		target.level,
		target.targetCastle.castleId,
		target.targetCastle.typeId,
		target.targetCastle.x,
		target.targetCastle.y,
	]);

	const requirements = useMemo(
		() => (preview?.requirements ?? []).map((requirement): PresetRequirement => {
			const kind = requirement.kind === 'troop' ? 'Troop' : 'Tool';
			const metadata = requirement.kind === 'troop' ? troops[requirement.itemId] : tools[requirement.itemId];
			return {
				kind,
				itemId: requirement.itemId,
				name: metadata?.name || `${kind} #${requirement.itemId}`,
				requested: requirement.required,
				available: requirement.available,
			};
		}).sort((left, right) =>
			left.kind.localeCompare(right.kind) || left.name.localeCompare(right.name) || left.itemId - right.itemId),
		[preview?.requirements, tools, troops],
	);
	const shortages = requirements.filter((requirement) => requirement.requested > requirement.available);
	const presetSummary = preset ? summarizeAttackPreset(preset) : null;
	const freeCommanders = Object.values(state?.commanders ?? {}).filter((commander) => commander.available).length;
	const sessionReady = state?.session.loggedIn === true && state.session.socketReady === true;
	const blockReason = attackBlockReason({
		target,
		sourceCastleSelected: sourceCastle != null,
		inventoryObserved: Boolean(sourceCastle?.unitsObservedAt),
		presetSelected: preset != null,
		preview,
		previewLoading,
		previewError,
		shortageCount: shortages.length,
		freeCommanders,
		sessionReady,
	});

	const sourceOptions = sourceCastles.map((castle) => ({
		value: String(castle.id),
		label: `${castle.name || `Castle ${castle.id}`} · ${castle.x}:${castle.y}`,
	}));
	const presetOptions = document.presets.map((candidate) => {
		const summary = summarizeAttackPreset(candidate);
		return {
			value: candidate.id,
			label: `${candidate.name} · ${summary.troops.toLocaleString()} troops · ${summary.tools.toLocaleString()} tools`,
		};
	});

	const launch = async () => {
		if (launching || blockReason || !sourceCastle || !preset || !preview) return;
		setLaunching(true);
		setLaunchError('');
		try {
			await submitIntent('alliance.target.attack', {
				sourceCastleId: sourceCastle.id,
				kingdomId: 0,
				targetX: target.targetCastle.x,
				targetY: target.targetCastle.y,
				targetPlayerId: target.playerId,
				targetCastleId: target.targetCastle.castleId,
				targetTypeId: target.targetCastle.typeId,
				targetLevel: target.level,
				targetLegendLevel: target.legendLevel,
				previewCommanderId: preview.commanderId,
				preset: { id: preset.id, name: preset.name, waves: preset.waves },
			});
			Notifications.success(`Attack launched toward ${target.targetCastle.name || `${target.targetCastle.x}:${target.targetCastle.y}`}.`);
			onClose();
		} catch (error) {
			setLaunchError(error instanceof Error ? error.message : 'Could not launch the attack.');
		} finally {
			setLaunching(false);
		}
	};

	return (
		<Modal
			isOpen
			onClose={() => { if (!launching) onClose(); }}
			maxWidth="2xl"
			title={(
				<ModalTitle
					icon={<Swords className="h-5 w-5" />}
					description={`${target.name} · ${target.targetCastle.name || target.targetCastle.typeName || 'Player castle'} · ${target.targetCastle.x}:${target.targetCastle.y}`}
				>
					Attack alliance target
				</ModalTitle>
			)}
			footer={(
				<div className="flex w-full flex-wrap items-center justify-between gap-3">
					<p className={`min-w-0 text-xs font-semibold ${blockReason ? 'text-text-muted' : 'text-success'}`}>
						{blockReason || 'CRA-capped formation and live source inventory are ready.'}
					</p>
					<div className="flex items-center gap-2">
						<Button variant="ghost" disabled={launching} onClick={onClose}>Cancel</Button>
						<Button
							variant="primary"
							disabled={Boolean(blockReason) || launching}
							isLoading={launching}
							onClick={() => void launch()}
							leftIcon={<Swords className="h-4 w-4" />}
						>
							Attack
						</Button>
					</div>
				</div>
			)}
		>
			<div className="space-y-4">
				{launchError ? (
					<div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-medium text-error">
						{launchError}
					</div>
				) : null}

				<div className="grid gap-3 sm:grid-cols-3">
					<AttackMetric
						icon={<MapPin className="h-4 w-4" />}
						label="Route distance"
						value={distance == null ? '—' : distance.toFixed(1)}
						detail={sourceCastle ? `From ${sourceCastle.name || `castle ${sourceCastle.id}`}` : 'Choose a source castle'}
					/>
					<AttackMetric
						icon={<FileSearch className="h-4 w-4" />}
						label="Reported defense"
						value={target.spyReport ? target.spyReport.totalTroops.toLocaleString() : 'Unknown'}
						detail={target.spyReport ? formatReportAge(target.spyReport.capturedAtUnixMillis) : 'No spy report available'}
					/>
					<AttackMetric
						icon={<ShieldCheck className="h-4 w-4" />}
						label="Free commanders"
						value={freeCommanders.toLocaleString()}
						detail={sessionReady ? 'Live session ready' : 'Game session unavailable'}
					/>
				</div>

				<div className="grid gap-4 rounded-global border border-border-base bg-bg-card/45 p-4 md:grid-cols-2">
					<label className="block min-w-0">
						<span className="mb-1.5 block text-[11px] font-black uppercase tracking-wider text-text-muted">Source castle</span>
						<Select
							value={sourceCastleID}
							options={sourceOptions}
							onChange={setSourceCastleID}
							placeholder="Choose a source castle"
							icon={<Castle className="h-4 w-4" />}
							disabled={sourceOptions.length === 0 || launching}
							searchable
							menuGrowToViewport
						/>
					</label>
					<label className="block min-w-0">
						<span className="mb-1.5 block text-[11px] font-black uppercase tracking-wider text-text-muted">Attack preset</span>
						<Select
							value={presetID}
							options={presetOptions}
							onChange={setPresetID}
							placeholder={presetOptions.length > 0 ? 'Choose an attack preset' : 'No attack presets configured'}
							icon={<Swords className="h-4 w-4" />}
							disabled={presetOptions.length === 0 || launching}
							searchable
							menuGrowToViewport
						/>
					</label>
				</div>

				<div className={`rounded-global border p-4 ${
					previewError || shortages.length > 0
						? 'border-error/30 bg-error/8'
						: preview
							? 'border-success/25 bg-success/7'
							: 'border-border-base bg-bg-card/35'
				}`}>
					<div className="flex flex-wrap items-start justify-between gap-3">
						<div>
							<div className="flex items-center gap-2 font-black text-text-main">
								{previewLoading ? (
									<RefreshCw className="h-4 w-4 animate-spin text-primary" />
								) : previewError || shortages.length > 0 ? (
									<AlertTriangle className="h-4 w-4 text-error" />
								) : (
									<ShieldCheck className="h-4 w-4 text-success" />
								)}
								CRA-capped formation availability
							</div>
							<p className="mt-1 text-xs text-text-muted">
								{preview
									? `${preview.totalTroops.toLocaleString()} troops and ${preview.totalTools.toLocaleString()} tools after CRA caps across ${preview.appliedWaves} of ${preview.presetWaves} preset wave${preview.presetWaves === 1 ? '' : 's'}.`
									: previewLoading
										? 'Applying the target, commander, source-castle, wave, and lane caps used by the CRA builder.'
										: previewError
											? previewError
											: presetSummary
												? 'Calculating the launch formation from this preset.'
									: 'Choose a preset to compare it with the selected castle.'}
							</p>
							{preview ? (
								<p className="mt-1.5 font-mono text-[11px] text-text-muted">
									Per wave: L {preview.capacity.left.toLocaleString()} · C {preview.capacity.front.toLocaleString()} · R {preview.capacity.right.toLocaleString()} · max {preview.maximumWaves} waves
								</p>
							) : null}
						</div>
						{preset && sourceCastle ? (
							<Badge variant={previewError ? 'danger' : previewLoading ? 'warning' : shortages.length > 0 ? 'danger' : preview ? 'success' : 'outline'} className="normal-case tracking-normal">
								{previewError
									? 'Preview unavailable'
									: previewLoading
										? 'Calculating caps'
										: shortages.length > 0
											? `${shortages.length} shortage${shortages.length === 1 ? '' : 's'}`
											: preview ? 'All capped stock available' : 'Waiting for preview'}
							</Badge>
						) : null}
					</div>
					{shortages.length > 0 ? (
						<div className="mt-3 flex flex-wrap gap-2">
							{shortages.map((shortage) => (
								<span key={`${shortage.kind}-${shortage.itemId}`} className="rounded-full border border-error/25 bg-bg-card/55 px-3 py-1.5 text-xs font-semibold text-error">
									{shortage.name}: {shortage.available.toLocaleString()} / {shortage.requested.toLocaleString()}
								</span>
							))}
						</div>
					) : null}
				</div>
			</div>
		</Modal>
	);
};

const AttackMetric = ({ icon, label, value, detail }: { icon: ReactNode; label: string; value: string; detail: string }) => (
	<div className="rounded-global border border-border-base bg-bg-card/45 p-3">
		<div className="flex items-center gap-1.5 text-[10px] font-black uppercase tracking-wider text-text-muted">{icon}{label}</div>
		<div className="mt-1 text-xl font-black tabular-nums text-text-main">{value}</div>
		<div className="mt-0.5 truncate text-xs text-text-muted" title={detail}>{detail}</div>
	</div>
);

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

function formatReportAge(capturedAtUnixMillis: number): string {
	const ageSeconds = Math.max(0, Math.floor((Date.now() - capturedAtUnixMillis) / 1000));
	if (ageSeconds < 60) return 'Captured just now';
	const minutes = Math.floor(ageSeconds / 60);
	if (minutes < 60) return `${minutes}m old`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ${minutes % 60}m old`;
	const days = Math.floor(hours / 24);
	return `${days}d ${hours % 24}h old`;
}

function routeDistance(sourceX: number, sourceY: number, targetX: number, targetY: number): number {
	return Math.hypot(targetX - sourceX, targetY - sourceY);
}

function attackBlockReason(input: {
	target: AllianceTarget;
	sourceCastleSelected: boolean;
	inventoryObserved: boolean;
	presetSelected: boolean;
	preview: AllianceTargetAttackPreviewV2 | null;
	previewLoading: boolean;
	previewError: string;
	shortageCount: number;
	freeCommanders: number;
	sessionReady: boolean;
}): string | null {
	if (input.target.underBird) return `Target is under bird for ${formatDuration(input.target.rptSeconds).replace(' remaining', '')}.`;
	if (!input.sessionReady) return 'The live game session is not ready.';
	if (!input.sourceCastleSelected) return 'Choose a source castle.';
	if (!input.inventoryObserved) return 'The selected castle inventory has not been observed.';
	if (!input.presetSelected) return 'Choose an attack preset.';
	if (input.freeCommanders < 1) return 'No commander is currently free.';
	if (input.previewLoading) return 'Calculating CRA formation caps.';
	if (input.previewError) return input.previewError;
	if (!input.preview) return 'CRA formation caps are not available yet.';
	if (input.preview.totalTroops < 1) return 'The CRA-capped formation has no troops.';
	if (input.shortageCount > 0) return 'The selected castle does not have the complete CRA-capped formation.';
	if (!input.preview.ready) return 'The CRA-capped formation is not ready.';
	return null;
}

export default AllianceTargetsView;
