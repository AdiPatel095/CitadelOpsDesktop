import {
  parseCastleFocusPayload,
  type CastleFocusState,
  type PlayerCastleOption,
} from '../types/CastleFocusState.ts';

/** JSON keys for GameState.Castle (matches Go `PlayerCastles` tags). */
export const CASTLE_SLOT_KEYS = [
  'mainCastle',
  'outpost1',
  'outpost2',
  'outpost3',
  'iceCastle',
  'desertCastle',
  'dungeonCastle',
  'stormCastle',
  'metropolis',
  'capital',
] as const;

function isRecord(v: unknown): v is Record<string, unknown> {
  return v != null && typeof v === 'object';
}

function castleNameForAid(castleRoot: Record<string, unknown> | undefined, aid: number): string | undefined {
  if (!castleRoot) return undefined;
  for (const k of CASTLE_SLOT_KEYS) {
    const slot = castleRoot[k];
    if (!isRecord(slot)) continue;
    const a = Math.trunc(Number(slot.aid));
    if (a === aid && typeof slot.castleName === 'string' && slot.castleName.trim()) {
      return slot.castleName.trim();
    }
  }
  return undefined;
}

function defaultKingdomIDForSlot(key: typeof CASTLE_SLOT_KEYS[number]): number {
  switch (key) {
    case 'iceCastle':
      return 2;
    case 'desertCastle':
      return 1;
    case 'dungeonCastle':
      return 3;
    case 'stormCastle':
      return 4;
    default:
      return 0;
  }
}

function castleSlotRank(castleRoot: Record<string, unknown> | undefined, aid: number): number {
  if (!castleRoot || aid <= 0) return 1000;
  const idx = CASTLE_SLOT_KEYS.findIndex((k) => {
    const slot = castleRoot[k];
    return isRecord(slot) && Math.trunc(Number(slot.aid)) === aid;
  });
  return idx >= 0 ? idx : 1000;
}

function slotMapCoords(slot: Record<string, unknown>, key: typeof CASTLE_SLOT_KEYS[number]) {
  let kingdomID = Math.trunc(Number(slot.mapKingdomID));
  if (!Number.isFinite(kingdomID) || kingdomID === 0) {
    kingdomID = defaultKingdomIDForSlot(key);
  }
  let mapX = Math.trunc(Number(slot.mapX)) || 0;
  let mapY = Math.trunc(Number(slot.mapY)) || 0;
  if ((mapX === 0 && mapY === 0) && isRecord(slot.troops)) {
    mapX = Math.trunc(Number(slot.troops.x)) || 0;
    mapY = Math.trunc(Number(slot.troops.y)) || 0;
    const troopKid = Math.trunc(Number(slot.troops.kingdomID));
    if (Number.isFinite(troopKid) && troopKid !== 0) {
      kingdomID = troopKid;
    }
  }
  return { kingdomID, mapX, mapY };
}

/** Build player castle dropdown options from persisted `gameState` (alliance locations + castle names). */
export function playerCastleOptionsFromGameStateSnapshot(gameState: unknown): PlayerCastleOption[] {
  if (!isRecord(gameState)) return [];
  const alliance = isRecord(gameState.alliance) ? gameState.alliance : undefined;
  const rawLocs = alliance?.playerCastleLocations;
  const locs = Array.isArray(rawLocs) ? rawLocs : [];
  const castle = isRecord(gameState.castle) ? gameState.castle : undefined;
  const out: PlayerCastleOption[] = [];
  const seenAid = new Set<number>();
  for (const loc of locs) {
    if (!isRecord(loc)) continue;
    const aid = Math.trunc(Number(loc.castleID));
    if (aid <= 0) continue;
    const nm = castleNameForAid(castle, aid) || `Castle ${aid}`;
    seenAid.add(aid);
    out.push({
      aid,
      kingdomID: Math.trunc(Number(loc.kingdomID)) || 0,
      name: nm,
      mapX: Math.trunc(Number(loc.x)) || 0,
      mapY: Math.trunc(Number(loc.y)) || 0,
    });
  }
  if (castle) {
    for (const key of CASTLE_SLOT_KEYS) {
      const slot = castle[key];
      if (!isRecord(slot)) continue;
      const aid = Math.trunc(Number(slot.aid));
      if (aid <= 0 || seenAid.has(aid)) continue;
      const { kingdomID, mapX, mapY } = slotMapCoords(slot, key);
      out.push({
        aid,
        kingdomID,
        name: typeof slot.castleName === 'string' && slot.castleName.trim() ? slot.castleName.trim() : `Castle ${aid}`,
        mapX,
        mapY,
      });
      seenAid.add(aid);
    }
  }
  return out.sort((a, b) => {
    const rankA = castleSlotRank(castle, a.aid);
    const rankB = castleSlotRank(castle, b.aid);
    if (rankA !== rankB) return rankA - rankB;
    if (a.kingdomID !== b.kingdomID) return a.kingdomID - b.kingdomID;
    return a.aid - b.aid;
  });
}

function findCastleSlotByAid(
  castleRoot: Record<string, unknown> | undefined,
  aid: number
): Record<string, unknown> | undefined {
  if (!castleRoot) return undefined;
  for (const k of CASTLE_SLOT_KEYS) {
    const slot = castleRoot[k];
    if (!isRecord(slot)) continue;
    const a = Math.trunc(Number(slot.aid));
    if (a === aid) return slot;
  }
  return undefined;
}

/**
 * Build a {@link CastleFocusState} from persisted snapshot JSON for a given castle (offline / stale viewing).
 */
export function buildCastleFocusFromSnapshot(
  snapshot: Record<string, unknown> | null | undefined,
  aid: number,
  kingdomID: number
): CastleFocusState | null {
  if (!snapshot || !isRecord(snapshot.gameState)) return null;
  const gs = snapshot.gameState as Record<string, unknown>;
  const castleRoot = isRecord(gs.castle) ? gs.castle : undefined;
  const castle = findCastleSlotByAid(castleRoot, aid);
  const cf = isRecord(gs.castleFocus) ? gs.castleFocus : undefined;

  const slotProd = castle && (castle.slotProductionByLid ?? castle.slotProductionByLID);

  const playerCastles = playerCastleOptionsFromGameStateSnapshot(gs);

  const raw: Record<string, unknown> = {
    aid,
    kingdomID: kingdomID || (cf ? Math.trunc(Number(cf.kingdomID)) || 0 : 0),
    mapPX: cf != null && cf.mapPX != null ? Number(cf.mapPX) : undefined,
    mapPY: cf != null && cf.mapPY != null ? Number(cf.mapPY) : undefined,
    castleName: typeof castle?.castleName === 'string' ? castle.castleName : undefined,
    bgRows: castle?.bgRows,
    bdRows: castle?.bdRows,
    slotProductionByLid: slotProd,
    craftingQueues: castle?.craftingQueues,
    playerCastles,
  };

  return parseCastleFocusPayload(raw);
}

/** Focus entry from `gameState.castleFocus` in a snapshot file. */
export function buildCastleFocusFromStoredSnapshotFocus(
  snapshot: Record<string, unknown> | null | undefined
): CastleFocusState | null {
  if (!snapshot || !isRecord(snapshot.gameState)) return null;
  const gs = snapshot.gameState as Record<string, unknown>;
  const cf = isRecord(gs.castleFocus) ? gs.castleFocus : undefined;
  const aid = cf ? Math.trunc(Number(cf.castleAID)) : 0;
  const kid = cf ? Math.trunc(Number(cf.kingdomID)) || 0 : 0;
  if (aid <= 0) return null;
  return buildCastleFocusFromSnapshot(snapshot, aid, kid);
}
