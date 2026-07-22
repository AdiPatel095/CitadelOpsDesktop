import React, { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { Pencil, Play, SlidersHorizontal, Trash2, Users } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { Badge, Button, EmptyState, Input, SectionCard, Select } from '../../components/ui';
import { useMovement } from '../../Movement/context/MovementContext';
import type { CommanderActivity, MovementViewModel } from '../../Movement/types/MovementState';
import type { MovementStateV2 } from '../../api/Contracts';
import { useRiftMap } from '../context/RiftMapContext';
import { formatSavedAt, riftLaunchLabel, type RiftCRALaunchEntry } from '../types/RiftCRALaunch';
import {
  arriveAtUnixFromOffset,
  formatLocalArrivalFromUnix,
  formatTravelDuration,
  isEarliestOffset,
} from '../types/RiftArrivalTime';
import RiftArrivalClock from './RiftArrivalClock';
import type { AttackSetupDraft, AttackSetupInventory } from '../../components/AttackSetupModal';
import { useMetadata } from '../../context/MetadataContext';
import { useCitadelAPI } from '../../api/ApiContext';
import {
  COMMANDER_FEATURE_SECTION,
  parseCommanderFeatureAssignments,
} from '../../Movement/types/CommanderFeatureAssignments';
import HorseTravelBoostSelect from '../../settings/components/HorseTravelBoostSelect';
import type { HorseTravelBoostID } from '../../settings/HorseTravelBoost';
import {
  RIFT_ATTACK_PREFERENCES_SECTION,
  parseRiftAttackPreferences,
} from '../types/RiftAttackPreferences';

const AttackSetupModal = React.lazy(() => import('../../components/AttackSetupModal'));

type ReplayCommanderMode = 'captured' | 'any';

const REPLAY_COMMANDER_OPTIONS = [
  { value: 'captured', label: 'Captured commander' },
  { value: 'any', label: 'Any available commander' },
];

const COMMANDER_STATUS_META: Record<
  CommanderActivity,
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

function effectiveProgress(movement: MovementStateV2, nowUnix: number): number {
  if (movement.arrivesAt && movement.travelSeconds) {
    const remaining = Math.max(0, Math.floor(Date.parse(movement.arrivesAt) / 1000) - nowUnix);
    return Math.max(0, movement.travelSeconds - remaining);
  }
  return movement.progressSeconds ?? 0;
}

function commanderStatusForLaunch(
  movement: MovementViewModel | null,
  commanderID: number | undefined,
  gameLoggedIn: boolean,
  nowUnix: number,
  fallbackStatus: CommanderActivity | undefined
): CommanderActivity {
  if (!gameLoggedIn || commanderID == null || commanderID < 0) return 'unknown';
  if (!movement) return fallbackStatus ?? 'syncing';
  if (!movement?.snapshotReady) return 'syncing';
  const snapshotFresh =
    movement.lastSnapshotUnix > 0 &&
    nowUnix >= movement.lastSnapshotUnix &&
    nowUnix - movement.lastSnapshotUnix <= movement.freshnessWindowSec;
  if (!snapshotFresh) return 'unknown';
  const row = movement.commanderStatuses.find((candidate) => candidate.commanderId === commanderID);
  if (!row) return 'unknown';
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

function commanderStatusTitle(status: CommanderActivity, commanderID: number | undefined): string {
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

function storedAttackSetup(value: unknown): AttackSetupDraft | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const draft = value as Partial<AttackSetupDraft>;
  if (typeof draft.name !== 'string' || !Array.isArray(draft.waves) || draft.waves.length === 0) return undefined;
  if (!draft.waves.every((wave) => wave && typeof wave === 'object' && !Array.isArray(wave))) return undefined;
  return draft as AttackSetupDraft;
}

function formatCoords(x: number | undefined, y: number | undefined): string {
  return x == null || y == null ? 'Unknown coordinates' : `(${x}, ${y})`;
}

const RiftAttackTemplate: React.FC = () => {
  const { configuration, updateConfiguration } = useCitadelAPI();
  const { gameLoggedIn } = useAuth();
  const { castle } = useCastleFocus();
  const { troops, tools } = useMetadata();
  const { movement } = useMovement();
  const { riftCRALaunch, replayRiftCRALaunch, renameRiftCRALaunch, deleteRiftCRALaunch } = useRiftMap();
  const [offsetMinutesById, setOffsetMinutesById] = useState<Record<string, number>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftName, setDraftName] = useState('');
  const [attackSetupOpen, setAttackSetupOpen] = useState(false);
  const [attackSetupDraft, setAttackSetupDraft] = useState<AttackSetupDraft | undefined>();
  const [nowUnix, setNowUnix] = useState(() => Math.floor(Date.now() / 1000));
  const [commanderMode, setCommanderMode] = useState<ReplayCommanderMode>('captured');
  const [horseTravelBoostId, setHorseTravelBoostId] = useState<HorseTravelBoostID>(-1);
  const [activeActionId, setActiveActionId] = useState<string | null>(null);
  const [actionStatus, setActionStatus] = useState<{ message: string; error: boolean } | null>(null);

  const launches = useMemo(
    () => [...(riftCRALaunch?.launches ?? [])].sort(
      (left, right) => (right.savedAtUnix ?? 0) - (left.savedAtUnix ?? 0)
        || riftLaunchLabel(left).localeCompare(riftLaunchLabel(right))
    ),
    [riftCRALaunch?.launches]
  );
  const attackSetupSummary = useMemo(() => summarizeAttackSetup(attackSetupDraft), [attackSetupDraft]);
  const savedAttackSetup = useMemo(
    () => storedAttackSetup(configuration?.sections['rift.attackSetup']),
    [configuration?.sections['rift.attackSetup']],
  );
  const commanderAssignments = useMemo(
    () => parseCommanderFeatureAssignments(configuration?.sections[COMMANDER_FEATURE_SECTION]),
    [configuration?.sections],
  );
  const assignedReplayCommanders = commanderAssignments.assignments.riftReplay ?? [];
  const attackSetupInventory = useMemo<AttackSetupInventory | undefined>(() => {
    if (!castle) return undefined;
    const troopStock: Record<number, number> = {};
    const toolStock: Record<number, number> = {};
    for (const [rawID, rawAmount] of Object.entries(castle.units.stationed)) {
      const id = Number(rawID);
      const amount = Number(rawAmount);
      if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(amount) || amount <= 0) continue;
      if (tools[id]) toolStock[id] = Math.trunc(amount);
      else if (troops[id]) troopStock[id] = Math.trunc(amount);
    }
    return {
      label: `${castle.name || `Castle ${castle.id}`} inventory`,
      troopStock,
      toolStock,
    };
  }, [castle, tools, troops]);

  useEffect(() => {
    const timer = window.setInterval(() => setNowUnix(Math.floor(Date.now() / 1000)), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (!attackSetupDraft && savedAttackSetup) setAttackSetupDraft(savedAttackSetup);
  }, [attackSetupDraft, savedAttackSetup]);

  useEffect(() => {
    const preferences = parseRiftAttackPreferences(configuration?.sections[RIFT_ATTACK_PREFERENCES_SECTION]);
    setHorseTravelBoostId(preferences.replayHorseTravelBoostId);
  }, [configuration?.sections]);

  const freeCommanderCount = useMemo(
    () => (movement?.commanderStatuses ?? []).filter((row) => (
      assignedReplayCommanders.includes(row.commanderId)
      && commanderStatusForLaunch(movement, row.commanderId, gameLoggedIn, nowUnix, row.status) === 'free'
    )).length,
    [assignedReplayCommanders, gameLoggedIn, movement, nowUnix]
  );
  const commanderCount = assignedReplayCommanders.length;

  const updateReplayHorseTravelBoost = useCallback((next: HorseTravelBoostID) => {
    setHorseTravelBoostId(next);
    const current = parseRiftAttackPreferences(configuration?.sections[RIFT_ATTACK_PREFERENCES_SECTION]);
    void updateConfiguration(RIFT_ATTACK_PREFERENCES_SECTION, { ...current, replayHorseTravelBoostId: next })
      .catch((error) => setActionStatus({
        message: error instanceof Error ? error.message : 'Could not save the Rift replay travel boost.',
        error: true,
      }));
  }, [configuration?.sections, updateConfiguration]);

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
      if (activeActionId) return;
      const nextName = draftName.trim();
      setActiveActionId(`rename:${launchId}`);
      setActionStatus(null);
      setEditingId(null);
      setDraftName('');
      void renameRiftCRALaunch(launchId, draftName)
        .then(() => setActionStatus({
          message: nextName ? `Renamed template to “${nextName}”.` : 'Cleared the custom template name.',
          error: false,
        }))
        .catch((error) => setActionStatus({
          message: error instanceof Error ? error.message : 'Could not rename the Rift template.',
          error: true,
        }))
        .finally(() => setActiveActionId(null));
    },
    [activeActionId, draftName, renameRiftCRALaunch]
  );

  const handleDelete = useCallback(
    (entry: RiftCRALaunchEntry) => {
      if (activeActionId) return;
      const label = riftLaunchLabel(entry);
      if (!window.confirm(`Delete “${label}”? This also cancels its scheduled replay.`)) return;
      setActiveActionId(`delete:${entry.id}`);
      setActionStatus(null);
      void deleteRiftCRALaunch(entry.id)
        .then(() => {
          setOffsetMinutesById((prev) => {
            const next = { ...prev };
            delete next[entry.id];
            return next;
          });
          if (editingId === entry.id) cancelRename();
          setActionStatus({ message: `Deleted “${label}”.`, error: false });
        })
        .catch((error) => setActionStatus({
          message: error instanceof Error ? error.message : 'Could not delete the Rift template.',
          error: true,
        }))
        .finally(() => setActiveActionId(null));
    },
    [activeActionId, cancelRename, deleteRiftCRALaunch, editingId]
  );

  const handleAttack = useCallback(
    (entry: RiftCRALaunchEntry, canAttack: boolean) => {
      if (!canAttack || activeActionId) return;
      const useFocusCoords = castle != null && (castle.x !== 0 || castle.y !== 0);
      const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
      const scheduled = !isEarliestOffset(offsetMinutes);
      const arriveAtUnix = scheduled ? arriveAtUnixFromOffset(entry, offsetMinutes) : null;
      const label = riftLaunchLabel(entry);
      setActiveActionId(`replay:${entry.id}`);
      setActionStatus(null);
      void replayRiftCRALaunch({
        launchId: entry.id,
        ...(commanderMode === 'any'
          ? { commanderSelection: {
            candidates: assignedReplayCommanders,
            count: 1,
            strategy: 'first_available' as const,
          } }
          : { commanderID: entry.commanderID != null && entry.commanderID >= 0 ? entry.commanderID : undefined }),
        horseTravelBoostId,
        sourceCastleId: castle?.id,
        sourceX: useFocusCoords ? castle!.x : undefined,
        sourceY: useFocusCoords ? castle!.y : undefined,
        ...(attackSetupDraft ? { attackSetup: attackSetupDraft } : {}),
        ...(scheduled && arriveAtUnix != null ? { arriveAtUnix } : {}),
      })
        .then(() => setActionStatus({
          message: scheduled && arriveAtUnix != null
            ? `Scheduled “${label}” to arrive at ${formatLocalArrivalFromUnix(arriveAtUnix)}.`
            : `Submitted “${label}” for replay.`,
          error: false,
        }))
        .catch((error) => setActionStatus({
          message: error instanceof Error ? error.message : 'Could not replay the Rift template.',
          error: true,
        }))
        .finally(() => setActiveActionId(null));
    },
    [activeActionId, assignedReplayCommanders, attackSetupDraft, offsetMinutesById, castle, commanderMode, horseTravelBoostId, replayRiftCRALaunch]
  );

  return (
    <>
      <SectionCard
        variant="glass"
        title="Captured Rift attacks"
        titleClassName="text-lg text-primary"
        description={(
          <>
            Name templates for quick recognition. Feather travel time from the last successful launch sets the earliest
            arrival — use the clock to schedule later, then resend or schedule.
            {attackSetupSummary ? <span className="mt-2 block font-mono">Attack setup · {attackSetupSummary}</span> : null}
            {actionStatus ? (
              <span
                role="status"
                aria-live="polite"
                className={`mt-2 block ${actionStatus.error ? 'text-error' : 'text-success'}`}
              >
                {actionStatus.message}
              </span>
            ) : null}
          </>
        )}
        descriptionClassName=""
        headerClassName="flex-col items-stretch gap-4 lg:flex-row lg:items-start"
        actions={<div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end lg:shrink-0">
          {launches.length > 0 ? (
            <div className="min-w-[13rem]">
              <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-text-muted">
                Replay commander
              </p>
              <Select
                value={commanderMode}
                options={REPLAY_COMMANDER_OPTIONS}
                onChange={(value) => setCommanderMode(value as ReplayCommanderMode)}
                disabled={activeActionId != null}
                icon={<Users className="h-3.5 w-3.5" />}
              />
              <p className="mt-1 text-[10px] text-text-muted">
                {commanderMode === 'any'
                  ? `${freeCommanderCount} free now · checked again at launch`
                  : 'Reuses the commander stored in each template'}
              </p>
            </div>
          ) : null}
          {launches.length > 0 ? (
            <div className="min-w-[15rem]">
              <HorseTravelBoostSelect value={horseTravelBoostId} onChange={updateReplayHorseTravelBoost} />
            </div>
          ) : null}
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setAttackSetupOpen(true)}
            disabled={!attackSetupInventory}
            title={attackSetupInventory ? 'Configure the formation used for Rift replays' : 'Castle inventory is not available'}
            leftIcon={<SlidersHorizontal className="w-3.5 h-3.5" />}
            className="shrink-0"
          >
            Attack setup
          </Button>
        </div>}
      >
        {launches.length === 0 ? (
          <EmptyState
            size="sm"
            title="No replay templates have been captured yet."
            description={gameLoggedIn
                ? 'Launch one castle attack on the Rift in-game. Citadel Ops will capture its commander, formation, and travel time here for reuse.'
                : 'Connect to the game and launch one castle attack on the Rift to create your first replay template.'}
            className="rounded-lg bg-bg-card/30"
          />
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
                  <th className="px-3 py-2 font-semibold text-right">Arrival · Action</th>
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
                  const commanderStatusMeta = COMMANDER_STATUS_META[commanderStatus];
                  const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
                  const scheduled = !isEarliestOffset(offsetMinutes);
                  const hasCapturedCommander = entry.commanderID != null && entry.commanderID >= 0;
                  const capturedCommanderAssigned = !hasCapturedCommander
                    || assignedReplayCommanders.includes(entry.commanderID!);
                  const commanderReady = commanderMode === 'any'
                    ? scheduled ? commanderCount > 0 : freeCommanderCount > 0
                    : capturedCommanderAssigned && (scheduled ? hasCapturedCommander : commanderAvailable);
                  const canAttack = gameLoggedIn && commanderReady;
                  const isEditing = editingId === entry.id;
                  const label = riftLaunchLabel(entry);
                  const attackTitle = !gameLoggedIn
                    ? 'Connect to attack'
                    : commanderMode === 'any' && commanderCount === 0
                      ? 'Commander data is not available yet'
                      : commanderMode === 'any' && !scheduled && freeCommanderCount === 0
                        ? 'No commander is currently available'
                        : commanderMode === 'captured' && !hasCapturedCommander
                          ? 'This template has no captured commander'
                          : commanderMode === 'captured' && !capturedCommanderAssigned
                            ? 'This commander is not assigned to Rift Replay in Movement / Features'
                          : commanderMode === 'captured' && !scheduled && !commanderAvailable
                            ? commanderStatusTitle(commanderStatus, entry.commanderID)
                            : scheduled
                              ? 'Schedule attack for the selected arrival time'
                              : 'Resend now for the earliest feather arrival';
                  return (
                    <tr key={entry.id} className="border-b border-border-base/70 last:border-b-0">
                      <td className="px-3 py-3 min-w-[160px]">
                        {isEditing ? (
                          <Input
                            value={draftName}
                            onChange={(e) => setDraftName(e.target.value)}
                            onBlur={() => commitRename(entry.id)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') e.currentTarget.blur();
                              if (e.key === 'Escape') {
                                e.preventDefault();
                                cancelRename();
                              }
                            }}
                            placeholder={riftLaunchLabel(entry)}
                            className="h-8 text-sm"
                            autoFocus
                            maxLength={80}
                            disabled={activeActionId != null}
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
                              disabled={activeActionId != null}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-primary hover:bg-bg-card-hover disabled:cursor-not-allowed disabled:opacity-40"
                              title="Rename template"
                              aria-label={`Rename ${label}`}
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleDelete(entry)}
                              disabled={activeActionId != null}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-error hover:bg-error/10 disabled:cursor-not-allowed disabled:opacity-40"
                              title="Delete template"
                              aria-label={`Delete ${label}`}
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
                        {commanderMode === 'any' ? (
                          <p className="mt-1 text-[10px] text-text-muted">Any available overrides this LID</p>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 text-text-main">
                        {entry.waveCount ?? 0} wave{(entry.waveCount ?? 0) === 1 ? '' : 's'}
                        {entry.useTravelFeather ? (
                          <span className="text-text-muted"> · feather</span>
                        ) : null}
                        <p className="text-xs font-mono text-text-muted mt-0.5">
                          {formatCoords(entry.sourceX, entry.sourceY)} → {formatCoords(entry.targetX, entry.targetY)}
                        </p>
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {entry.oneWayTTSeconds != null && entry.oneWayTTSeconds > 0 ? (
                          <span className="font-mono">{formatTravelDuration(entry.oneWayTTSeconds)}</span>
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
                            disabled={!canAttack || (activeActionId != null && activeActionId !== `replay:${entry.id}`)}
                            isLoading={activeActionId === `replay:${entry.id}`}
                            onClick={() => handleAttack(entry, canAttack)}
                            title={attackTitle}
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
      </SectionCard>
      {attackSetupOpen ? (
        <Suspense fallback={null}>
          <AttackSetupModal
            isOpen={attackSetupOpen}
            initialDraft={attackSetupDraft}
            inventory={attackSetupInventory}
            onClose={() => setAttackSetupOpen(false)}
            onSave={(nextDraft) => {
              setAttackSetupDraft(nextDraft);
              void updateConfiguration('rift.attackSetup', nextDraft);
              setAttackSetupOpen(false);
            }}
          />
        </Suspense>
      ) : null}
    </>
  );
};

export default RiftAttackTemplate;
