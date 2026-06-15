import type { PlayerCastleInfo } from '../../dashboard/models/PlayerCastleInfo';

export interface MainCastleTroopSnapshot {
  aid: number;
  troopsI: Record<string, number>;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return v != null && typeof v === 'object';
}

function troopsIFromCastle(castle: PlayerCastleInfo | undefined): Record<string, number> {
  return castle?.troops?.troopsI ?? {};
}

/** Main castle troopsI from live updates when main castle AID is known (in-game name is not "Main Castle"). */
export function mainCastleFromResourceMap(
  castleResources: Map<number, PlayerCastleInfo>,
  mainCastleAid?: number
): MainCastleTroopSnapshot | null {
  if (mainCastleAid != null && mainCastleAid > 0) {
    const castle = castleResources.get(mainCastleAid);
    if (castle && castle.aid > 0) {
      return { aid: Math.trunc(castle.aid), troopsI: troopsIFromCastle(castle) };
    }
  }
  return null;
}

/** Offline fallback: main castle slot from persisted gameState snapshot. */
export function mainCastleFromSnapshot(gameState: unknown): MainCastleTroopSnapshot | null {
  if (!isRecord(gameState)) return null;
  const castleRoot = gameState.castle;
  if (!isRecord(castleRoot)) return null;
  const slot = castleRoot.mainCastle;
  if (!isRecord(slot)) return null;
  const aid = Math.trunc(Number(slot.aid));
  if (aid <= 0) return null;
  const troops = isRecord(slot.troops) ? (slot.troops as PlayerCastleInfo['troops']) : undefined;
  return { aid, troopsI: troops?.troopsI ?? {} };
}

export function resolveMainCastleTroops(
  castleResources: Map<number, PlayerCastleInfo>,
  snapshotGameState: unknown | undefined
): MainCastleTroopSnapshot | null {
  const snapshotMain = mainCastleFromSnapshot(snapshotGameState);
  const mainAid = snapshotMain?.aid;
  const liveMain = mainCastleFromResourceMap(castleResources, mainAid);
  if (liveMain) return liveMain;
  return snapshotMain ?? null;
}

/** Unit ids with in-castle count > 0, sorted by stock descending. */
export function mainCastleAvailableUnitIds(troopsI: Record<string, number>): number[] {
  return Object.entries(troopsI)
    .map(([id, count]) => ({ id: Number(id), count: Number(count) }))
    .filter((row) => Number.isFinite(row.id) && row.id > 0 && row.count > 0)
    .sort((a, b) => b.count - a.count)
    .map((row) => row.id);
}

export function mainCastleStockQuantities(troopsI: Record<string, number>): Record<number, number> {
  const out: Record<number, number> = {};
  for (const [id, count] of Object.entries(troopsI)) {
    const unitId = Number(id);
    const qty = Number(count);
    if (Number.isFinite(unitId) && unitId > 0 && qty > 0) {
      out[unitId] = qty;
    }
  }
  return out;
}
