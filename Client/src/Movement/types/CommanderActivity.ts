import type { MovementStateV2 } from '../../api/Contracts';
import type { MessageKey } from '../../i18n/messages';
import type { CommanderActivity, CommanderStatusRow, MovementViewModel } from './MovementState';

/**
 * Commander activity as shown in Movement → Commanders, shared with automation
 * setup (CIT-18). Pure: reads the movement view model only, never requests a
 * refresh from the game.
 */

export function effectiveProgress(movement: MovementStateV2, nowUnix: number): number {
  if (movement.arrivesAt && movement.travelSeconds) {
    const remaining = Math.max(0, Math.floor(Date.parse(movement.arrivesAt) / 1000) - nowUnix);
    return Math.max(0, movement.travelSeconds - remaining);
  }
  return movement.progressSeconds ?? 0;
}

export function statusForRow(
  row: CommanderStatusRow,
  gameLoggedIn: boolean,
  snapshotReady: boolean,
  snapshotFresh: boolean,
  nowUnix: number,
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

/** Same freshness rule as the Commanders view. */
export function movementSnapshotFresh(
  movement: MovementViewModel | null,
  gameLoggedIn: boolean,
  nowUnix: number,
): boolean {
  if (!gameLoggedIn || movement == null || !movement.snapshotReady) return false;
  const freshnessWindow = movement.freshnessWindowSec;
  return freshnessWindow <= 0
    || (movement.lastSnapshotUnix > 0 && nowUnix - movement.lastSnapshotUnix <= freshnessWindow);
}

/**
 * Commander status for a launch control (Rift attack template): the row's activity, or
 * `syncing`/`unknown` while the movement snapshot is missing or stale. Freshness follows
 * movementSnapshotFresh, so a view model with no freshness window (the default: sparse
 * movement frames keep a connection's baseline live, and unchanged polls do not advance
 * the snapshot time) is fresh once the baseline is ready.
 */
export function commanderStatusForLaunch(
  movement: MovementViewModel | null,
  commanderID: number | undefined,
  gameLoggedIn: boolean,
  nowUnix: number,
  fallbackStatus: CommanderActivity | undefined,
): CommanderActivity {
  if (!gameLoggedIn || commanderID == null || commanderID < 0) return 'unknown';
  if (!movement) return fallbackStatus ?? 'syncing';
  if (!movement.snapshotReady) return 'syncing';
  if (!movementSnapshotFresh(movement, gameLoggedIn, nowUnix)) return 'unknown';
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

const message = (key: MessageKey): MessageKey => key;

export const COMMANDER_ACTIVITY_LABEL_KEYS: Readonly<Record<CommanderActivity, MessageKey>> = {
  syncing: message('commanderActivity.syncing'),
  unknown: message('commanderActivity.unknown'),
  free: message('commanderActivity.free'),
  outbound: message('commanderActivity.outbound'),
  busy: message('commanderActivity.busy'),
  posted: message('commanderActivity.posted'),
  returning: message('commanderActivity.returning'),
};
