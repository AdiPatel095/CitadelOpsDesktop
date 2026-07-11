import type { GameStateV2 } from '../../api/Contracts';

export interface MainCastleTroopSnapshot {
  aid: number;
  troopsI: Record<string, number>;
}

export function resolveMainCastleTroops(state: GameStateV2 | null): MainCastleTroopSnapshot | null {
  if (!state) return null;
  const castles = Object.values(state.castles);
  const main = castles.find((castle) => castle.slotType === 1)
    ?? castles.find((castle) => castle.kingdomId === 0 && castle.focused)
    ?? castles.find((castle) => castle.kingdomId === 0)
    ?? null;
  if (!main) return null;
  return { aid: main.id, troopsI: main.units.stationed };
}

export function mainCastleAvailableUnitIds(troopsI: Record<string, number>): number[] {
  return Object.entries(troopsI)
    .map(([id, count]) => ({ id: Number(id), count: Number(count) }))
    .filter((row) => Number.isFinite(row.id) && row.id > 0 && row.count > 0)
    .sort((left, right) => right.count - left.count)
    .map((row) => row.id);
}

export function mainCastleStockQuantities(troopsI: Record<string, number>): Record<number, number> {
  const result: Record<number, number> = {};
  for (const [id, count] of Object.entries(troopsI)) {
    const unitId = Number(id);
    const quantity = Number(count);
    if (Number.isFinite(unitId) && unitId > 0 && quantity > 0) result[unitId] = quantity;
  }
  return result;
}
