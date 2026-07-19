import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowUpRight,
  CheckCircle2,
  Clock3,
  HelpCircle,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Save,
  XCircle,
} from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Notifications } from '../../components/Notifications';
import { useAuth } from '../../context/AuthContext';
import { useCitadelAPI } from '../../api/ApiContext';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PillSelector } from '../../components/ui';
import { useMovement } from '../context/MovementContext';
import {
  COMMANDER_FEATURE_SECTION,
  defaultCommanderFeatureAssignments,
  isCommanderAssigned,
  parseCommanderFeatureAssignments,
  setCommanderAssignment,
  type CommanderFeatureAssignmentsV1,
  type CommanderFeatureID,
} from '../types/CommanderFeatureAssignments';
import {
  formatTroopSummary,
  labelKingdom,
  labelTargetType,
  type CommanderActivity,
  type CommanderStatusRow,
} from '../types/MovementState';
import type { MovementStateV2 } from '../../api/Contracts';

type MovementMode = 'Movement' | 'Features';

const COMMANDER_FEATURES: Array<{ id: CommanderFeatureID; label: string }> = [
  { id: 'autoTowers', label: 'Auto Towers' },
  { id: 'autoInvasion', label: 'Auto Invasion' },
  { id: 'autoNomad', label: 'Auto Nomad / Samurai' },
  { id: 'autoKhan', label: 'Auto Khan' },
  { id: 'autoStorm', label: 'Auto Storm' },
  { id: 'riftMaiden', label: 'Rift Maiden Waves' },
  { id: 'riftReplay', label: 'Rift Replay' },
];

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
  const { configuration, updateConfiguration } = useCitadelAPI();
  const { movement, refreshMovement } = useMovement();
  const [mode, setMode] = useState<MovementMode>('Movement');
  const [nowUnix, setNowUnix] = useState(() => Math.floor(Date.now() / 1000));
  const configuredAssignments = useMemo(
    () => parseCommanderFeatureAssignments(configuration?.sections[COMMANDER_FEATURE_SECTION]),
    [configuration?.sections[COMMANDER_FEATURE_SECTION]],
  );
  const [featureAssignments, setFeatureAssignments] = useState<CommanderFeatureAssignmentsV1>(
    defaultCommanderFeatureAssignments,
  );
  const [assignmentsDirty, setAssignmentsDirty] = useState(false);
  const [savingAssignments, setSavingAssignments] = useState(false);

  useEffect(() => {
    const timer = window.setInterval(() => setNowUnix(Math.floor(Date.now() / 1000)), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    setFeatureAssignments(configuredAssignments);
    setAssignmentsDirty(false);
  }, [configuredAssignments]);

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
  const knownCommanderIDs = rows.map((row) => row.commanderId);

  const toggleCommanderFeature = (
    featureID: CommanderFeatureID,
    commanderID: number,
    assigned: boolean,
  ) => {
    setFeatureAssignments((current) => setCommanderAssignment(
      current,
      featureID,
      commanderID,
      assigned,
      knownCommanderIDs,
    ));
    setAssignmentsDirty(true);
  };

  const setAllCommanderFeatures = (commanderID: number, assigned: boolean) => {
    setFeatureAssignments((current) => COMMANDER_FEATURES.reduce(
      (next, feature) => setCommanderAssignment(
        next,
        feature.id,
        commanderID,
        assigned,
        knownCommanderIDs,
      ),
      current,
    ));
    setAssignmentsDirty(true);
  };

  const saveCommanderFeatures = async () => {
    if (savingAssignments || !assignmentsDirty) return;
    setSavingAssignments(true);
    try {
      await updateConfiguration(COMMANDER_FEATURE_SECTION, featureAssignments);
      setAssignmentsDirty(false);
      Notifications.success('Commander feature assignments saved.');
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save commander feature assignments.');
    } finally {
      setSavingAssignments(false);
    }
  };

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
          <PillSelector
            ariaLabel="Movement workspace mode"
            value={mode}
            options={['Movement', 'Features']}
            onChange={(value) => setMode(value as MovementMode)}
            size="sm"
          />
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 max-[720px]:w-full max-[720px]:flex-none">
            <CardTitle className="text-lg text-primary">
              {mode === 'Movement' ? 'Commanders' : 'Feature assignments'}
              <span className="ml-2 text-sm font-normal text-text-muted">({rows.length})</span>
            </CardTitle>
            {mode === 'Movement' ? <Badge variant={snapshotBadge.variant}>{snapshotBadge.label}</Badge> : null}
            {assignmentsDirty ? <Badge variant="warning">Unsaved</Badge> : null}
            {mode === 'Movement' && rows.length > 0 ? (
              <span className="text-xs text-text-muted">{availableCount} available</span>
            ) : null}
            {mode === 'Movement' ? <span className="text-xs text-text-muted">Auto-refreshes every 5s</span> : null}
          </div>
          {mode === 'Features' ? (
            <Button
              variant="primary"
              size="sm"
              disabled={!assignmentsDirty}
              isLoading={savingAssignments}
              onClick={() => void saveCommanderFeatures()}
              leftIcon={<Save className="h-4 w-4" />}
            >
              Save assignments
            </Button>
          ) : null}
          <Button
            variant="secondary"
            size="sm"
            className="shrink-0"
            disabled={!gameLoggedIn}
            onClick={() => refreshMovement(true)}
            title={gameLoggedIn ? 'Refresh commander status' : 'Connect to refresh'}
          >
            <RefreshCw className="mr-1.5 h-4 w-4" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="liquid-prominent-header-content">
          {mode === 'Features' ? (
            <div className="flex flex-col gap-4">
              <div className="rounded-global border border-border-light bg-bg-card/45 px-4 py-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-xl">
                <p className="text-sm font-semibold text-text-main">Choose the commanders each automation may launch.</p>
                <p className="mt-1 text-xs text-text-muted">
                  Features without a saved assignment continue to use every commander. Live availability and feature-specific requirements are still checked before each launch.
                </p>
              </div>
              {rows.length === 0 ? (
                <p className="text-sm text-text-muted">
                  {gameLoggedIn
                    ? 'Waiting for the commander roster.'
                    : 'No commander roster was saved for the last session.'}
                </p>
              ) : (
                <div className="overflow-x-auto rounded-global border border-border-light bg-bg-card/30 shadow-[var(--glass-shadow-compact)] backdrop-blur-xl custom-scrollbar">
                  <table className="min-w-[44rem] w-full text-sm">
                    <thead>
                      <tr className="border-b border-border-light bg-bg-card/65 text-left text-[10px] uppercase tracking-wider text-text-muted">
                        <th className="w-64 px-4 py-2.5 font-semibold">Commander</th>
                        <th className="px-4 py-2.5 font-semibold">Allowed features</th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((row) => {
                        const selectedFeatureCount = COMMANDER_FEATURES.filter((feature) => (
                          isCommanderAssigned(featureAssignments, feature.id, row.commanderId)
                        )).length;
                        const visiblePosition = Number.isFinite(row.visiblePosition)
                          && row.visiblePosition < Number.MAX_SAFE_INTEGER
                          ? row.visiblePosition
                          : null;
                        return (
                          <tr
                            key={row.commanderId}
                            className="border-b border-border-base/70 transition-colors last:border-b-0 hover:bg-bg-card-hover/45"
                          >
                            <td className="px-4 py-4 align-top">
                              <div className="flex items-center gap-3">
                                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-primary/25 bg-primary/10 text-xs font-black text-primary shadow-[0_0_14px_color-mix(in_srgb,var(--color-primary)_12%,transparent)]">
                                  {visiblePosition ?? '—'}
                                </span>
                                <div className="min-w-0">
                                  <div className="truncate font-semibold text-text-main">
                                    {row.name || `Commander ${row.commanderId}`}
                                  </div>
                                  <div className="mt-0.5 font-mono text-[11px] text-text-muted">
                                    LID {row.commanderId}
                                  </div>
                                </div>
                              </div>
                            </td>
                            <td className="px-4 py-4 align-top">
                              <div className="flex flex-wrap items-start gap-3">
                                <div className="flex shrink-0 flex-wrap gap-2">
                                  <button
                                    type="button"
                                    className="rounded-full transition-transform hover:-translate-y-0.5 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 disabled:pointer-events-none disabled:opacity-40"
                                    disabled={savingAssignments || selectedFeatureCount === COMMANDER_FEATURES.length}
                                    onClick={() => setAllCommanderFeatures(row.commanderId, true)}
                                    aria-label={`Select all features for ${row.name || `commander ${row.commanderId}`}`}
                                  >
                                    <Badge variant="primary" className="gap-1.5 cursor-pointer normal-case tracking-normal shadow-sm">
                                      <CheckCircle2 className="h-3 w-3" />
                                      Select all
                                    </Badge>
                                  </button>
                                  <button
                                    type="button"
                                    className="rounded-full transition-transform hover:-translate-y-0.5 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 disabled:pointer-events-none disabled:opacity-40"
                                    disabled={savingAssignments || selectedFeatureCount === 0}
                                    onClick={() => setAllCommanderFeatures(row.commanderId, false)}
                                    aria-label={`Unselect all features for ${row.name || `commander ${row.commanderId}`}`}
                                  >
                                    <Badge variant="danger" className="gap-1.5 cursor-pointer normal-case tracking-normal shadow-sm">
                                      <XCircle className="h-3 w-3" />
                                      Unselect all
                                    </Badge>
                                  </button>
                                </div>
                                <div className="flex min-w-48 flex-1 flex-wrap gap-2 border-l border-border-base pl-3">
                                  {COMMANDER_FEATURES.map((feature) => {
                                    const assigned = isCommanderAssigned(
                                      featureAssignments,
                                      feature.id,
                                      row.commanderId,
                                    );
                                    return (
                                      <button
                                        key={feature.id}
                                        type="button"
                                        className="rounded-full transition-transform hover:-translate-y-0.5 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 disabled:pointer-events-none disabled:opacity-40"
                                        aria-pressed={assigned}
                                        disabled={savingAssignments}
                                        onClick={() => toggleCommanderFeature(
                                          feature.id,
                                          row.commanderId,
                                          !assigned,
                                        )}
                                        aria-label={`${assigned ? 'Disallow' : 'Allow'} ${feature.label} for ${row.name || `commander ${row.commanderId}`}`}
                                      >
                                        <Badge
                                          variant={assigned ? 'success' : 'danger'}
                                          className="gap-1.5 cursor-pointer normal-case tracking-normal shadow-sm"
                                        >
                                          {assigned
                                            ? <CheckCircle2 className="h-3 w-3" />
                                            : <XCircle className="h-3 w-3" />}
                                          {feature.label}
                                        </Badge>
                                      </button>
                                    );
                                  })}
                                </div>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          ) : rows.length === 0 ? (
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
