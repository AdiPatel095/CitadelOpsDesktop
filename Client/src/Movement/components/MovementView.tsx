import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowUpRight,
  CheckCircle2,
  Clock3,
  HelpCircle,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
} from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { useAuth } from '../../context/AuthContext';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle } from '../../components/ui';
import { useMovement } from '../context/MovementContext';
import {
  formatTroopSummary,
  labelKingdom,
  labelTargetType,
  type CommanderActivity,
  type CommanderStatusRow,
} from '../types/MovementState';
import type { MovementStateV2 } from '../../api/Contracts';

const STATUS_META: Record<
  CommanderActivity,
  {
    label: string;
    variant: 'secondary' | 'success' | 'warning' | 'danger' | 'outline';
    icon: React.ComponentType<{ className?: string }>;
  }
> = {
  syncing: { label: 'Syncing', variant: 'secondary', icon: LoaderCircle },
  unknown: { label: 'Unknown', variant: 'danger', icon: HelpCircle },
  free: { label: 'Free', variant: 'success', icon: CheckCircle2 },
  outbound: { label: 'Outbound', variant: 'warning', icon: ArrowUpRight },
  busy: { label: 'Busy', variant: 'warning', icon: Clock3 },
  posted: { label: 'Posted', variant: 'warning', icon: Clock3 },
  returning: { label: 'Returning', variant: 'outline', icon: RotateCcw },
};

function sortCommanders(rows: CommanderStatusRow[]): CommanderStatusRow[] {
  return [...rows].sort((left, right) => {
    if (left.visiblePosition !== right.visiblePosition) {
      return left.visiblePosition - right.visiblePosition;
    }
    return left.commanderId - right.commanderId;
  });
}

function effectiveProgress(movement: MovementStateV2, nowUnix: number): number {
  if (movement.arrivesAt && movement.travelSeconds) {
    const remaining = Math.max(0, Math.floor(Date.parse(movement.arrivesAt) / 1000) - nowUnix);
    return Math.max(0, movement.travelSeconds - remaining);
  }
  return movement.progressSeconds ?? 0;
}

function statusForRow(
  row: CommanderStatusRow,
  gameLoggedIn: boolean,
  snapshotReady: boolean,
  snapshotFresh: boolean,
  nowUnix: number
): CommanderActivity {
  if (!gameLoggedIn) return 'unknown';
  if (!snapshotReady) return 'syncing';
  if (!snapshotFresh) return 'unknown';
  if (
    row.status === 'outbound' &&
    row.movement != null &&
    (row.movement.travelSeconds ?? 0) > 0 &&
    effectiveProgress(row.movement, nowUnix) >= (row.movement.travelSeconds ?? 0)
  ) {
    return row.movement.returnsAt ? 'posted' : 'busy';
  }
  return row.status;
}

function formatTiming(
  movement: MovementStateV2 | null,
  status: CommanderActivity,
  nowUnix: number
): string {
  if (status === 'free') return 'Available';
  if (status === 'syncing') return 'Waiting for snapshot';
  if (status === 'unknown') return 'Last state is stale';
  if (!movement) return 'In use';

  for (const [timestamp, label] of [[movement.returnsAt, 'to return'], [movement.arrivesAt, 'to arrival']] as const) {
    if (!timestamp) continue;
    const remaining = Math.max(0, Math.floor(Date.parse(timestamp) / 1000) - nowUnix);
    if (remaining > 0) return `${remaining.toLocaleString()}s ${label}`;
  }
  if (status === 'busy') return 'Awaiting return state';
  const travelSeconds = movement.travelSeconds ?? 0;
  const progress = effectiveProgress(movement, nowUnix);
  if (travelSeconds <= 0) return `${progress}s elapsed`;
  const capped = Math.min(progress, travelSeconds);
  const percent = Math.min(100, Math.round(capped / travelSeconds * 100));
  return `${capped}s / ${travelSeconds}s (${percent}%)`;
}

function StatusBadge({ status }: { status: CommanderActivity }) {
  const meta = STATUS_META[status];
  const Icon = meta.icon;
  return (
    <Badge variant={meta.variant} className="gap-1.5 whitespace-nowrap normal-case tracking-normal">
      <Icon className={`h-3.5 w-3.5 ${status === 'syncing' ? 'animate-spin' : ''}`} />
      {meta.label}
    </Badge>
  );
}

const MovementView: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { movement, refreshMovement } = useMovement();
  const [nowUnix, setNowUnix] = useState(() => Math.floor(Date.now() / 1000));

  useEffect(() => {
    const timer = window.setInterval(() => setNowUnix(Math.floor(Date.now() / 1000)), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const rows = useMemo(
    () => sortCommanders(movement?.commanderStatuses ?? []),
    [movement?.commanderStatuses]
  );
  const snapshotReady = movement?.snapshotReady === true;
  const freshnessWindow = movement?.freshnessWindowSec ?? 45;
  const snapshotFresh =
    gameLoggedIn &&
    snapshotReady &&
    movement != null &&
    movement.lastSnapshotUnix > 0 &&
    nowUnix - movement.lastSnapshotUnix <= freshnessWindow;
  const availableCount = rows.filter(
    (row) => statusForRow(row, gameLoggedIn, snapshotReady, snapshotFresh, nowUnix) === 'free'
  ).length;

  const snapshotBadge = !gameLoggedIn
    ? { label: 'Saved', variant: 'secondary' as const }
    : !snapshotReady
      ? { label: 'Syncing', variant: 'secondary' as const }
      : snapshotFresh
        ? { label: 'Live', variant: 'success' as const }
        : { label: 'Stale', variant: 'danger' as const };

  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />

      <Card className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 max-[720px]:w-full max-[720px]:flex-none">
            <CardTitle className="text-lg text-primary">
              Commanders
              <span className="ml-2 text-sm font-normal text-text-muted">({rows.length})</span>
            </CardTitle>
            <Badge variant={snapshotBadge.variant}>{snapshotBadge.label}</Badge>
            {rows.length > 0 ? (
              <span className="text-xs text-text-muted">{availableCount} available</span>
            ) : null}
          </div>
          <Button
            variant="secondary"
            size="sm"
            className="ml-auto shrink-0"
            disabled={!gameLoggedIn}
            onClick={() => refreshMovement(true)}
            title={gameLoggedIn ? 'Refresh commander status' : 'Connect to refresh'}
          >
            <RefreshCw className="mr-1.5 h-4 w-4" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="liquid-prominent-header-content">
          {rows.length === 0 ? (
            <p className="text-sm text-text-muted">
              {gameLoggedIn
                ? 'Waiting for the commander roster.'
                : 'No commander roster was saved for the last session.'}
            </p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border-base custom-scrollbar">
              <table className="min-w-[72rem] w-full table-fixed text-sm">
                <thead>
                  <tr className="border-b border-border-base bg-bg-card/50 text-left text-[10px] uppercase tracking-wider text-text-muted">
                    <th className="w-52 px-3 py-2 font-semibold">Commander</th>
                    <th className="w-32 px-3 py-2 font-semibold">Status</th>
                    <th className="w-32 px-3 py-2 font-semibold">Kingdom</th>
                    <th className="w-44 px-3 py-2 font-semibold">Target</th>
                    <th className="w-56 px-3 py-2 font-semibold">Route</th>
                    <th className="w-52 px-3 py-2 font-semibold">Timing</th>
                    <th className="w-40 px-3 py-2 font-semibold">Troops</th>
                    <th className="w-24 px-3 py-2 font-semibold">MID</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => {
                    const status = statusForRow(
                      row,
                      gameLoggedIn,
                      snapshotReady,
                      snapshotFresh,
                      nowUnix
                    );
                    const active = row.movement;
                    return (
                      <tr
                        key={row.commanderId}
                        className="h-[4.25rem] border-b border-border-base/70 last:border-b-0"
                      >
                        <td className="px-3 py-3">
                          <div className="truncate font-medium text-text-main">
                            {row.name || `Commander ${row.commanderId}`}
                          </div>
                          <div className="mt-0.5 font-mono text-xs text-text-muted">
                            LID {row.commanderId}
                            {Number.isFinite(row.visiblePosition) &&
                            row.visiblePosition < Number.MAX_SAFE_INTEGER
                              ? ` · Slot ${row.visiblePosition}`
                              : ''}
                          </div>
                        </td>
                        <td className="px-3 py-3">
                          <StatusBadge status={status} />
                        </td>
                        <td className="truncate px-3 py-3 text-text-main">
                          {active ? labelKingdom(active.kingdomId) : '—'}
                        </td>
                        <td className="px-3 py-3 text-text-main">
                          {active ? labelTargetType(active.typeId) : '—'}
                        </td>
                        <td className="truncate px-3 py-3 font-mono text-xs text-text-muted">
                          {active
                            ? `(${active.sourceX ?? 0}, ${active.sourceY ?? 0}) → (${active.targetX}, ${active.targetY})`
                            : '—'}
                        </td>
                        <td className="px-3 py-3 text-text-muted">
                          {formatTiming(active, status, nowUnix)}
                        </td>
                        <td className="truncate px-3 py-3 text-text-muted">
                          {active ? formatTroopSummary(active.units) : '—'}
                        </td>
                        <td className="px-3 py-3 font-mono text-text-muted">
                          {active ? active.id : '—'}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default MovementView;
