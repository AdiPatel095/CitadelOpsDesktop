export interface RiftMapNode {
  type: number;
  typeLabel?: string;
  x: number;
  y: number;
  name?: string;
  level?: number;
  castleId?: number;
  playerId?: number;
  cooldownSec?: number;
  lastHitter?: string;
}

export interface RiftMapCoords {
  castleAid: number;
  kingdomID: number;
  centerX: number;
  centerY: number;
  riftKingdomID: number;
  found: boolean;
  rift: RiftMapNode | null;
  deltaX: number;
  deltaY: number;
  distance: number;
}

/** gaa AI[0] for the single world Rift POI (KID 0). */
export const GAA_NODE_RIFT_TYPE = 43;

function parseNode(raw: unknown): RiftMapNode | null {
  if (!raw || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  const x = Number(o.x ?? o.X);
  const y = Number(o.y ?? o.Y);
  if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
  const type = Number(o.type ?? o.Type) || 0;
  const name = typeof o.name === 'string' ? o.name : typeof o.Name === 'string' ? o.Name : undefined;
  const levelRaw = o.level ?? o.Level;
  const level = levelRaw != null ? Number(levelRaw) : undefined;
  const castleIdRaw = o.castleId ?? o.CastleID;
  const castleId = castleIdRaw != null ? Number(castleIdRaw) : undefined;
  const playerIdRaw = o.playerId ?? o.PlayerID;
  const playerId = playerIdRaw != null ? Number(playerIdRaw) : undefined;
  const cooldownRaw = o.cooldownSec ?? o.CooldownSeconds;
  const cooldownSec = cooldownRaw != null ? Number(cooldownRaw) : undefined;
  const lastHitter =
    typeof o.lastHitter === 'string'
      ? o.lastHitter
      : typeof o.LastHitter === 'string'
        ? o.LastHitter
        : undefined;
  return {
    type,
    x: Math.trunc(x),
    y: Math.trunc(y),
    ...(typeof o.typeLabel === 'string' ? { typeLabel: o.typeLabel } : {}),
    ...(name ? { name } : {}),
    ...(level != null && Number.isFinite(level) ? { level: Math.trunc(level) } : {}),
    ...(castleId != null && Number.isFinite(castleId) ? { castleId: Math.trunc(castleId) } : {}),
    ...(playerId != null && Number.isFinite(playerId) ? { playerId: Math.trunc(playerId) } : {}),
    ...(cooldownSec != null && Number.isFinite(cooldownSec) ? { cooldownSec: Math.trunc(cooldownSec) } : {}),
    ...(lastHitter ? { lastHitter } : {}),
  };
}

export function parseRiftMapCoordsPayload(raw: unknown): RiftMapCoords | null {
  if (!raw || typeof raw !== 'object') return null;
  const p = raw as Record<string, unknown>;
  const centerX = Number(p.centerX);
  const centerY = Number(p.centerY);
  const riftRaw = p.rift;
  const rift = riftRaw != null ? parseNode(riftRaw) : null;
  const found = Boolean(p.found) || rift != null;
  const deltaX = Number(p.deltaX);
  const deltaY = Number(p.deltaY);
  const distance = Number(p.distance);
  return {
    castleAid: Math.trunc(Number(p.castleAid)) || 0,
    kingdomID: Math.trunc(Number(p.kingdomID)) || 0,
    centerX: Number.isFinite(centerX) ? Math.trunc(centerX) : 0,
    centerY: Number.isFinite(centerY) ? Math.trunc(centerY) : 0,
    riftKingdomID: Math.trunc(Number(p.riftKingdomID)) || 0,
    found,
    rift,
    deltaX: Number.isFinite(deltaX) ? Math.trunc(deltaX) : rift ? rift.x - Math.trunc(centerX) : 0,
    deltaY: Number.isFinite(deltaY) ? Math.trunc(deltaY) : rift ? rift.y - Math.trunc(centerY) : 0,
    distance: Number.isFinite(distance)
      ? Math.trunc(distance)
      : rift && (centerX !== 0 || centerY !== 0)
        ? chebyshevDistance(Math.trunc(centerX), Math.trunc(centerY), rift.x, rift.y)
        : 0,
  };
}

function chebyshevDistance(cx: number, cy: number, x: number, y: number): number {
  return Math.max(Math.abs(x - cx), Math.abs(y - cy));
}

/** Scan persisted snapshot mapKingdoms for the sole Rift tile (type 43). */
export function findRiftInSnapshot(snapshot: Record<string, unknown> | null): {
  node: RiftMapNode;
  kingdomID: number;
} | null {
  if (!snapshot) return null;
  const kingdoms = snapshot.mapKingdoms;
  if (!kingdoms || typeof kingdoms !== 'object') return null;

  const kids = Object.keys(kingdoms as Record<string, unknown>)
    .map((k) => Number(k))
    .filter((k) => Number.isFinite(k))
    .sort((a, b) => (a === 0 ? -1 : b === 0 ? 1 : a - b));

  for (const kid of kids) {
    const tiles = (kingdoms as Record<string, unknown>)[String(kid)];
    if (!tiles || typeof tiles !== 'object') continue;
    for (const nodeRaw of Object.values(tiles as Record<string, unknown>)) {
      const n = parseNode(nodeRaw);
      if (n && n.type === GAA_NODE_RIFT_TYPE) {
        return { node: n, kingdomID: kid };
      }
    }
  }
  return null;
}

/** Offline hydration from persisted snapshot `mapKingdoms`. */
export function riftMapCoordsFromSnapshot(
  snapshot: Record<string, unknown> | null,
  focusAid: number,
  focusKingdomID: number,
  centerX: number,
  centerY: number
): RiftMapCoords | null {
  const located = findRiftInSnapshot(snapshot);
  const base: RiftMapCoords = {
    castleAid: focusAid,
    kingdomID: focusKingdomID,
    centerX,
    centerY,
    riftKingdomID: 0,
    found: false,
    rift: null,
    deltaX: 0,
    deltaY: 0,
    distance: 0,
  };
  if (!located) return base;

  const { node, kingdomID } = located;
  const deltaX = node.x - centerX;
  const deltaY = node.y - centerY;
  return {
    ...base,
    riftKingdomID: kingdomID,
    found: true,
    rift: node,
    deltaX,
    deltaY,
    distance: centerX !== 0 || centerY !== 0 ? chebyshevDistance(centerX, centerY, node.x, node.y) : 0,
  };
}

function formatSigned(n: number): string {
  return n >= 0 ? `+${n}` : String(n);
}

export function formatRiftDelta(deltaX: number, deltaY: number): string {
  return `${formatSigned(deltaX)}, ${formatSigned(deltaY)}`;
}
