import React, { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { Pencil, Play, SlidersHorizontal, Trash2 } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { Badge, Card, CardContent, CardHeader, CardTitle, Button, Input } from '../../components/ui';
import { useMovement } from '../../Movement/context/MovementContext';
import type { CommanderState, GAMMovement, MovementState } from '../../Movement/types/MovementState';
import { useRiftMap } from '../context/RiftMapContext';
import { formatSavedAt, riftLaunchLabel, type RiftCRALaunchEntry } from '../types/RiftCRALaunch';
import { arriveAtUnixFromOffset, isEarliestOffset } from '../types/RiftArrivalTime';
import RiftArrivalClock from './RiftArrivalClock';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';

const AttackSetupModal = React.lazy(() => import('../../components/AttackSetupModal'));

const COMMANDER_STATUS_META: Record<
  CommanderState,
  { label: string; variant: 'secondary' | 'success' | 'warning' | 'danger' | 'outline' }
> = {
  syncing: { label: 'Syncing', variant: 'secondary' },
  unknown: { label: 'Unknown', variant: 'danger' },
  free: { label: 'Free', variant: 'success' },
  outbound: { label: 'Outbound', variant: 'warning' },
  busy: { label: 'Busy', variant: 'warning' },
  posted: { label: 'Posted', variant: 'warning' },
  returning: { label: 'Returning', variant: 'outline' },
};

function effectivePT(movement: GAMMovement, nowUnix: number): number {
  if (movement.receivedUnix <= 0) return movement.pt;
  return movement.pt + Math.max(0, nowUnix - movement.receivedUnix);
}

function commanderStatusForLaunch(
  movement: MovementState | null,
  commanderID: number | undefined,
  gameLoggedIn: boolean,
  nowUnix: number,
  fallbackStatus: CommanderState | undefined
): CommanderState {
  if (!gameLoggedIn || commanderID == null || commanderID < 0) return 'unknown';
  if (!movement) return fallbackStatus ?? 'syncing';
  if (!movement?.snapshotReady) return 'syncing';
  const snapshotFresh =
    movement.lastSnapshotUnix > 0 &&
    nowUnix >= movement.lastSnapshotUnix &&
    nowUnix - movement.lastSnapshotUnix <= movement.freshnessWindowSec;
  if (!snapshotFresh) return 'unknown';
  const row = movement.commanderStatuses.find((candidate) => candidate.commanderID === commanderID);
  if (!row) return 'unknown';
  if (
    row.status === 'outbound' &&
    row.movement != null &&
    row.movement.tt > 0 &&
    effectivePT(row.movement, nowUnix) >= row.movement.tt
  ) {
    return row.movement.twd > 0 ? 'posted' : 'busy';
  }
  return row.status;
}

function commanderStatusTitle(status: CommanderState, commanderID: number | undefined): string {
  const lid = commanderID ?? '—';
  if (status === 'free') return `Commander LID ${lid} is available`;
  if (status === 'syncing') return `Waiting for Commander LID ${lid} status`;
  if (status === 'unknown') return `Commander LID ${lid} status is unavailable or stale`;
  return `Commander LID ${lid} is ${COMMANDER_STATUS_META[status].label.toLowerCase()}`;
}

function summarizeAttackSetup(draft: AttackSetupDraft | undefined) {
  if (!draft) return null;
  let troops = 0;
  let tools = 0;
  for (const wave of draft.waves) {
    for (const lane of [wave.L, wave.M, wave.R]) {
      for (const slot of lane.troops) {
        if (slot.itemId != null && slot.quantity > 0) troops += slot.quantity;
      }
      for (const slot of lane.tools) {
        if (slot.itemId != null && slot.quantity > 0) tools += slot.quantity;
      }
    }
  }
  return `${draft.waves.length} wave${draft.waves.length === 1 ? '' : 's'} | ${troops.toLocaleString()} troops | ${tools.toLocaleString()} tools`;
}

const RiftAttackTemplate: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();
  const { movement } = useMovement();
  const { riftCRALaunch, replayRiftCRALaunch, renameRiftCRALaunch, deleteRiftCRALaunch } = useRiftMap();
  const [offsetMinutesById, setOffsetMinutesById] = useState<Record<string, number>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftName, setDraftName] = useState('');
  const [attackSetupOpen, setAttackSetupOpen] = useState(false);
  const [attackSetupDraft, setAttackSetupDraft] = useState<AttackSetupDraft | undefined>();
  const [nowUnix, setNowUnix] = useState(() => Math.floor(Date.now() / 1000));

  const launches = riftCRALaunch?.launches ?? [];
  const attackSetupSummary = useMemo(() => summarizeAttackSetup(attackSetupDraft), [attackSetupDraft]);

  useEffect(() => {
    const timer = window.setInterval(() => setNowUnix(Math.floor(Date.now() / 1000)), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const setOffsetFor = useCallback((launchId: string, offsetMinutes: number) => {
    setOffsetMinutesById((prev) => ({ ...prev, [launchId]: offsetMinutes }));
  }, []);

  const startRename = useCallback((entry: RiftCRALaunchEntry) => {
    setEditingId(entry.id);
    setDraftName(entry.displayName?.trim() ?? '');
  }, []);

  const cancelRename = useCallback(() => {
    setEditingId(null);
    setDraftName('');
  }, []);

  const commitRename = useCallback(
    (launchId: string) => {
      renameRiftCRALaunch(launchId, draftName);
      setEditingId(null);
      setDraftName('');
    },
    [draftName, renameRiftCRALaunch]
  );

  const handleDelete = useCallback(
    (launchId: string) => {
      if (!window.confirm('Delete this captured Rift attack template?')) return;
      deleteRiftCRALaunch(launchId);
      setOffsetMinutesById((prev) => {
        const next = { ...prev };
        delete next[launchId];
        return next;
      });
      if (editingId === launchId) {
        cancelRename();
      }
    },
    [cancelRename, deleteRiftCRALaunch, editingId]
  );

  const handleAttack = useCallback(
    (entry: RiftCRALaunchEntry, commanderAvailable: boolean) => {
      if (!commanderAvailable) return;
      const useFocusCoords =
        castleFocus?.mapPX != null &&
        castleFocus?.mapPY != null &&
        (castleFocus.mapPX !== 0 || castleFocus.mapPY !== 0);
      const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
      const scheduled = !isEarliestOffset(offsetMinutes);
      const arriveAtUnix = scheduled ? arriveAtUnixFromOffset(entry, offsetMinutes) : null;
      replayRiftCRALaunch({
        launchId: entry.id,
        commanderID: entry.commanderID != null && entry.commanderID >= 0 ? entry.commanderID : undefined,
        sourceX: useFocusCoords ? castleFocus!.mapPX : undefined,
        sourceY: useFocusCoords ? castleFocus!.mapPY : undefined,
        ...(scheduled && arriveAtUnix != null ? { arriveAtUnix } : {}),
      });
    },
    [offsetMinutesById, castleFocus, replayRiftCRALaunch]
  );

  return (
    <>
      <Card className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent">
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div>
              <CardTitle className="text-lg text-primary">Captured Rift attacks</CardTitle>
              <p className="text-xs text-text-muted mt-1">
                Name templates for quick recognition. Feather travel time from the last successful launch sets the earliest
                arrival — use the clock to schedule later, then resend or schedule.
              </p>
              {attackSetupSummary ? (
                <p className="mt-2 text-xs font-mono text-text-muted">{attackSetupSummary}</p>
              ) : null}
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setAttackSetupOpen(true)}
              leftIcon={<SlidersHorizontal className="w-3.5 h-3.5" />}
              className="shrink-0 self-start"
            >
              Attack setup
            </Button>
          </div>
        </CardHeader>
        <CardContent className="liquid-prominent-header-content">
        {launches.length === 0 ? (
          <p className="text-sm text-text-muted">
            {gameLoggedIn
              ? 'No captured attacks yet. Launch a castle attack on the Rift and it will appear here.'
              : 'No captured attacks in the last session. Connect and attack the Rift once to save a template.'}
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border-base">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-base bg-bg-card/50 text-left text-[10px] uppercase tracking-wider text-text-muted">
                  <th className="px-3 py-2 font-semibold">Name</th>
                  <th className="px-3 py-2 font-semibold">Commander</th>
                  <th className="px-3 py-2 font-semibold">Layout</th>
                  <th className="px-3 py-2 font-semibold">Travel</th>
                  <th className="px-3 py-2 font-semibold">Captured</th>
                  <th className="px-3 py-2 font-semibold text-right">Arrive · Attack</th>
                </tr>
              </thead>
              <tbody>
                {launches.map((entry) => {
                  const commanderStatus = commanderStatusForLaunch(
                    movement,
                    entry.commanderID,
                    gameLoggedIn,
                    nowUnix,
                    entry.commanderStatus
                  );
                  const commanderAvailable = commanderStatus === 'free';
                  const canAttack = gameLoggedIn && commanderAvailable;
                  const commanderStatusMeta = COMMANDER_STATUS_META[commanderStatus];
                  const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
                  const scheduled = !isEarliestOffset(offsetMinutes);
                  const isEditing = editingId === entry.id;
                  const label = riftLaunchLabel(entry);
                  return (
                    <tr key={entry.id} className="border-b border-border-base/70 last:border-b-0">
                      <td className="px-3 py-3 min-w-[160px]">
                        {isEditing ? (
                          <Input
                            value={draftName}
                            onChange={(e) => setDraftName(e.target.value)}
                            onBlur={() => commitRename(entry.id)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') commitRename(entry.id);
                              if (e.key === 'Escape') cancelRename();
                            }}
                            placeholder={riftLaunchLabel(entry)}
                            className="h-8 text-sm"
                            autoFocus
                            maxLength={80}
                          />
                        ) : (
                          <div className="flex items-center gap-1.5 min-w-0">
                            <span
                              className={`truncate font-medium ${entry.displayName?.trim() ? 'text-text-main' : 'text-text-muted'}`}
                              title={label}
                            >
                              {label}
                            </span>
                            <button
                              type="button"
                              onClick={() => startRename(entry)}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-primary hover:bg-bg-card-hover"
                              title="Rename template"
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleDelete(entry.id)}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-error hover:bg-error/10"
                              title="Delete template"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-3 text-text-main">
                        <div className="font-mono">LID {entry.commanderID ?? '—'}</div>
                        <Badge
                          variant={commanderStatusMeta.variant}
                          className="mt-1 normal-case tracking-normal"
                        >
                          {commanderStatusMeta.label}
                        </Badge>
                      </td>
                      <td className="px-3 py-3 text-text-main">
                        {entry.waveCount ?? 0} wave{(entry.waveCount ?? 0) === 1 ? '' : 's'}
                        {entry.useTravelFeather ? (
                          <span className="text-text-muted"> · feather</span>
                        ) : null}
                        <p className="text-xs font-mono text-text-muted mt-0.5">
                          ({entry.sourceX}, {entry.sourceY}) → ({entry.targetX}, {entry.targetY})
                        </p>
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {entry.oneWayTTSeconds != null && entry.oneWayTTSeconds > 0 ? (
                          <span className="font-mono">{entry.oneWayTTSeconds}s</span>
                        ) : (
                          <span className="text-xs">pending success</span>
                        )}
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {formatSavedAt(entry.savedAtUnix)}
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <RiftArrivalClock
                            entry={entry}
                            offsetMinutes={offsetMinutes}
                            onOffsetChange={(offset) => setOffsetFor(entry.id, offset)}
                          />
                          <Button
                            variant="primary"
                            size="sm"
                            disabled={!canAttack}
                            onClick={() => handleAttack(entry, commanderAvailable)}
                            title={
                              !gameLoggedIn
                                ? 'Connect to attack'
                                : !commanderAvailable
                                  ? commanderStatusTitle(commanderStatus, entry.commanderID)
                                  : scheduled
                                    ? 'Schedule attack for the selected arrival time'
                                    : 'Resend now (earliest feather arrival)'
                            }
                            leftIcon={<Play className="w-3.5 h-3.5" />}
                          >
                            {scheduled ? 'Schedule' : 'Resend'}
                          </Button>
                        </div>
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
      {attackSetupOpen ? (
        <Suspense fallback={null}>
          <AttackSetupModal
            isOpen={attackSetupOpen}
            initialDraft={attackSetupDraft}
            onClose={() => setAttackSetupOpen(false)}
            onSave={(nextDraft) => {
              setAttackSetupDraft(nextDraft);
              setAttackSetupOpen(false);
            }}
          />
        </Suspense>
      ) : null}
    </>
  );
};

export default RiftAttackTemplate;
