import type { CastleBuildingRow } from './models/PlayerCastleInfo';

/**
 * WOD IDs from EmpireItems/buildings.json (upgrade chains) for JAA BG/BD `buildingID` checks.
 */

/** `"name": "Barracks"` (level 0 placeholder + main + relic + high tiers). */
export const WODS_BARRACKS = new Set([
  475, 160, 161, 162, 163, 164, 1741, 1742, 1743, 1744, 1748, 1749, 1750, 1751, 1938, 1939, 3112, 3113, 3114, 3115,
  3116,
]);

/**
 * Siege workshop offensive tools — JSON uses `"name": "Workshop"` (not the string SiegeWorkshop).
 * WODs: 165–167, 256.
 */
export const WODS_SIEGE_WORKSHOP = new Set([165, 166, 167, 256]);

/**
 * Defense workshop — JSON uses `"name": "Dworkshop"`.
 * WODs: 176–178, 257.
 */
export const WODS_DEFENSE_WORKSHOP = new Set([176, 177, 178, 257]);

export const WODS_REFINERY = new Set([
  254, 2172, 2173, 2174, 3060, 3061, 3062, 3063, 3175,
]);

export const WODS_TOOLSMITH = new Set([255, 2175, 2176, 2177, 3064, 3065, 3066, 3067]);

export const WODS_DRAGON_HOARD = new Set([3068, 3069, 3070, 3071, 3072]);

export const WODS_DRAGON_BREATH_FORGE = new Set([3093, 3094, 3095, 3096, 3097]);

function buildingWod(row: CastleBuildingRow): number {
  return Math.trunc(Number(row.buildingID));
}

function rowsMatchWodSet(rows: CastleBuildingRow[], set: ReadonlySet<number>): boolean {
  return rows.some((r) => {
    const id = buildingWod(r);
    return Number.isFinite(id) && id > 0 && set.has(id);
  });
}

function rowsMatchAnyWodSet(rows: CastleBuildingRow[], sets: ReadonlySet<number>[]): boolean {
  return sets.some((s) => rowsMatchWodSet(rows, s));
}

/**
 * Which queue strips to show for the Castle card, based on merged JAA BG+BD rows for the current focus.
 */
export function visibleCastleQueueIds(rows: CastleBuildingRow[]): Set<string> {
  const out = new Set<string>();
  if (!rows.length) return out;

  if (rowsMatchWodSet(rows, WODS_BARRACKS)) out.add('recruitment');
  if (rowsMatchAnyWodSet(rows, [WODS_SIEGE_WORKSHOP, WODS_DEFENSE_WORKSHOP])) out.add('tool');
  if (rowsMatchWodSet(rows, WODS_REFINERY)) out.add('refinery');
  if (rowsMatchWodSet(rows, WODS_TOOLSMITH)) out.add('toolsmith');
  if (rowsMatchWodSet(rows, WODS_DRAGON_HOARD)) out.add('dragon-hoard');
  if (rowsMatchWodSet(rows, WODS_DRAGON_BREATH_FORGE)) out.add('dragon-breath-forge');

  return out;
}
