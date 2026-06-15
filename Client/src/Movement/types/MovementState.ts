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
}

export interface MovementState {
  activeMovements: GAMMovement[];
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
  22: 'Castle (type 22)',
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

function parseGAMMovement(raw: unknown): GAMMovement | null {
  if (!isRecord(raw)) return null;
  const troopArray = Array.isArray(raw.troopArray)
    ? raw.troopArray
        .filter((row): row is unknown[] => Array.isArray(row))
        .map((row) => [Number(row[0]) || 0, Number(row[1]) || 0])
    : [];
  return {
    mid: Number(raw.mid) || 0,
    pt: Number(raw.pt) || 0,
    tt: Number(raw.tt) || 0,
    d: Number(raw.d) || 0,
    kid: Number(raw.kid) || 0,
    sid: Number(raw.sid) || 0,
    oid: Number(raw.oid) || 0,
    targetType: Number(raw.targetType) || 0,
    targetX: Number(raw.targetX) || 0,
    targetY: Number(raw.targetY) || 0,
    sourceX: Number(raw.sourceX) || 0,
    sourceY: Number(raw.sourceY) || 0,
    commanderID: Number(raw.commanderID) ?? -1,
    troopArray,
  };
}

export function parseMovementUpdatePayload(payload: unknown): MovementState | null {
  if (!isRecord(payload)) return null;
  const rows = Array.isArray(payload.activeMovements) ? payload.activeMovements : [];
  const activeMovements = rows
    .map(parseGAMMovement)
    .filter((m): m is GAMMovement => m != null);
  return { activeMovements };
}

/** Hydrate from persisted `gameState.movement` in a snapshot file. */
export function movementFromSnapshot(snapshot: Record<string, unknown> | null): MovementState | null {
  if (!snapshot || !isRecord(snapshot.gameState)) return null;
  const movement = snapshot.gameState.movement;
  if (!isRecord(movement)) return null;
  return parseMovementUpdatePayload(movement);
}
