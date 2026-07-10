/** Parsed GAM movement row (Server/Models/Movement/GAMMovement). */
export interface GAMMovement {
  mid: number;
  pt: number;
  tt: number;
  d: number;
  kid: number;
  sid: number;
  oid: number;
  targetType: number;
  targetX: number;
  targetY: number;
  sourceX: number;
  sourceY: number;
  commanderID: number;
  troopArray: number[][];
  pwd: number;
  twd: number;
  receivedUnix: number;
}

export type CommanderState =
  | 'syncing'
  | 'unknown'
  | 'free'
  | 'outbound'
  | 'busy'
  | 'posted'
  | 'returning';

export interface CommanderStatusRow {
  commanderID: number;
  name: string;
  visiblePosition: number;
  status: CommanderState;
  busy: boolean;
  movement: GAMMovement | null;
}

export interface MovementState {
  activeMovements: GAMMovement[];
  commanderStatuses: CommanderStatusRow[];
  snapshotReady: boolean;
  snapshotFresh: boolean;
  lastSnapshotUnix: number;
  freshnessWindowSec: number;
}

/** GAA map-node type labels (mirrors Server/Models/MapState/GaaNodeLabels.go). */
const GAA_NODE_TYPE_LABELS: Record<number, string> = {
  1: 'Castle (main)',
  2: 'Kingdom tower',
  3: 'Castle (capital)',
  4: 'Castle (occupied)',
  10: 'Alliance camp',
  11: 'Unknown (type 11)',
  12: 'Castle (foreign kingdom)',
  22: 'Castle (metropolis)',
  23: 'Coord marker',
  25: 'Unknown (type 25)',
  26: 'Monument',
  27: 'Nomad camp',
  28: 'Resource node',
  29: 'Unknown (type 29)',
  31: 'Empty terrain',
  34: 'Unknown (type 34)',
  35: 'Khan camp',
  37: 'Unknown (type 37)',
  38: 'Unknown (type 38)',
  43: 'Rift',
};

const KINGDOM_LABELS: Record<number, string> = {
  0: 'Main',
  1: 'Desert',
  2: 'Ice',
  3: 'Fire',
  4: 'Storm',
  10: 'Beri World',
};

export function labelTargetType(typeID: number): string {
  if (typeID === 0) return '—';
  return GAA_NODE_TYPE_LABELS[typeID] ?? `Unknown (type ${typeID})`;
}

export function labelKingdom(kid: number): string {
  const name = KINGDOM_LABELS[kid];
  return name ? `${name} (${kid})` : `Kingdom ${kid}`;
}

export function directionLabel(d: number): string {
  return d === 1 ? 'Return' : 'Outbound';
}

export function formatTroopSummary(troopArray: number[][] | undefined): string {
  if (!Array.isArray(troopArray) || troopArray.length === 0) return '—';
  const total = troopArray.reduce((sum, pair) => sum + (pair[1] ?? 0), 0);
  return `${troopArray.length} stack${troopArray.length === 1 ? '' : 's'} · ${total} troops`;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null;
}

function numberOr(raw: unknown, fallback: number): number {
  const value = Number(raw);
  return Number.isFinite(value) ? value : fallback;
}

function parseGAMMovement(raw: unknown): GAMMovement | null {
  if (!isRecord(raw)) return null;
  const troopArray = Array.isArray(raw.troopArray)
    ? raw.troopArray
        .filter((row): row is unknown[] => Array.isArray(row))
        .map((row) => [Number(row[0]) || 0, Number(row[1]) || 0])
    : [];
  return {
    mid: numberOr(raw.mid, 0),
    pt: numberOr(raw.pt, 0),
    tt: numberOr(raw.tt, 0),
    d: numberOr(raw.d, 0),
    kid: numberOr(raw.kid, 0),
    sid: numberOr(raw.sid, 0),
    oid: numberOr(raw.oid, 0),
    targetType: numberOr(raw.targetType, 0),
    targetX: numberOr(raw.targetX, 0),
    targetY: numberOr(raw.targetY, 0),
    sourceX: numberOr(raw.sourceX, 0),
    sourceY: numberOr(raw.sourceY, 0),
    commanderID: numberOr(raw.commanderID, -1),
    troopArray,
    pwd: numberOr(raw.pwd, 0),
    twd: numberOr(raw.twd, 0),
    receivedUnix: numberOr(raw.receivedUnix, 0),
  };
}

const COMMANDER_STATES = new Set<CommanderState>([
  'syncing',
  'unknown',
  'free',
  'outbound',
  'busy',
  'posted',
  'returning',
]);

function parseCommanderStatus(raw: unknown): CommanderStatusRow | null {
  if (!isRecord(raw)) return null;
  const commanderID = numberOr(raw.commanderID, -1);
  if (commanderID < 0) return null;
  const rawStatus = typeof raw.status === 'string' ? raw.status : 'unknown';
  const status = COMMANDER_STATES.has(rawStatus as CommanderState)
    ? (rawStatus as CommanderState)
    : 'unknown';
  return {
    commanderID,
    name: typeof raw.name === 'string' ? raw.name : '',
    visiblePosition: numberOr(raw.visiblePosition, Number.MAX_SAFE_INTEGER),
    status,
    busy: typeof raw.busy === 'boolean' ? raw.busy : status !== 'free',
    movement: parseGAMMovement(raw.movement),
  };
}

function stateForMovement(movement: GAMMovement): CommanderState {
  if (movement.d === 1) return 'returning';
  if (movement.tt > 0 && movement.pt >= movement.tt) {
    return movement.twd > 0 ? 'posted' : 'busy';
  }
  return 'outbound';
}

function statusesFromActiveMovements(activeMovements: GAMMovement[]): CommanderStatusRow[] {
  const byCommander = new Map<number, GAMMovement>();
  activeMovements.forEach((movement) => {
    if (movement.commanderID >= 0) byCommander.set(movement.commanderID, movement);
  });
  return [...byCommander.entries()].map(([commanderID, movement]) => ({
    commanderID,
    name: '',
    visiblePosition: Number.MAX_SAFE_INTEGER,
    status: stateForMovement(movement),
    busy: true,
    movement,
  }));
}

export function parseMovementUpdatePayload(payload: unknown): MovementState | null {
  if (!isRecord(payload)) return null;
  const rows = Array.isArray(payload.activeMovements) ? payload.activeMovements : [];
  const activeMovements = rows
    .map(parseGAMMovement)
    .filter((m): m is GAMMovement => m != null);
  const rawStatuses = Array.isArray(payload.commanderStatuses) ? payload.commanderStatuses : [];
  const commanderStatuses = rawStatuses
    .map(parseCommanderStatus)
    .filter((row): row is CommanderStatusRow => row != null);
  return {
    activeMovements,
    commanderStatuses:
      commanderStatuses.length > 0
        ? commanderStatuses
        : statusesFromActiveMovements(activeMovements),
    snapshotReady: payload.snapshotReady === true,
    snapshotFresh: payload.snapshotFresh === true,
    lastSnapshotUnix: numberOr(payload.lastSnapshotUnix, 0),
    freshnessWindowSec: numberOr(payload.freshnessWindowSec, 45),
  };
}

/** Hydrate from persisted `gameState.movement` in a snapshot file. */
export function movementFromSnapshot(snapshot: Record<string, unknown> | null): MovementState | null {
  if (!snapshot || !isRecord(snapshot.gameState)) return null;
  const gameState = snapshot.gameState;
  const movement = gameState.movement;
  if (!isRecord(movement)) return null;
  const parsed = parseMovementUpdatePayload(movement);
  if (!parsed) return null;

  let roster: unknown[] = Array.isArray(movement.commanderRoster)
    ? movement.commanderRoster
    : [];
  if (roster.length === 0 && isRecord(gameState.equipment)) {
    roster = Array.isArray(gameState.equipment.commActualArray)
      ? gameState.equipment.commActualArray.map((commander) => {
          if (!isRecord(commander)) return commander;
          return {
            commanderID: commander.id,
            name: commander.name,
            visiblePosition: commander.visiblePosition,
          };
        })
      : [];
  }
  const activeByCommander = new Map<number, GAMMovement>();
  parsed.activeMovements.forEach((active) => {
    if (active.commanderID >= 0) activeByCommander.set(active.commanderID, active);
  });
  const commanderStatuses = roster
    .map((entry) => {
      if (!isRecord(entry)) return null;
      const commanderID = numberOr(entry.commanderID, -1);
      if (commanderID < 0) return null;
      return {
        commanderID,
        name: typeof entry.name === 'string' ? entry.name : '',
        visiblePosition: numberOr(entry.visiblePosition, Number.MAX_SAFE_INTEGER),
        status: 'unknown' as CommanderState,
        busy: true,
        movement: activeByCommander.get(commanderID) ?? null,
      };
    })
    .filter((row): row is CommanderStatusRow => row != null);

  return {
    ...parsed,
    commanderStatuses:
      commanderStatuses.length > 0 ? commanderStatuses : parsed.commanderStatuses,
    snapshotFresh: false,
  };
}
