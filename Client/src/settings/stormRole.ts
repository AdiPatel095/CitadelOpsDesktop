import type { GameStateV2 } from '../api/Contracts';
import type { CastleOptionV2 } from '../api/Selectors';

export const STORM_SETTINGS_KEY = 'storm';
type CastleIdentity = { id: number; kingdomId: number };
export function castleSettingsKey(castle: CastleIdentity): string {
  return castle.kingdomId === 4 ? STORM_SETTINGS_KEY : String(castle.id);
}
export function castleSettingsEntry<T>(entries: Readonly<Record<string, T>>, castle: CastleIdentity, stormLegacyKey?: string): T | undefined {
  if (castle.kingdomId !== 4 && String(castle.id) === stormLegacyKey) return undefined;
  const key = castleSettingsKey(castle);
  return Object.hasOwn(entries, key) ? entries[key] : entries[String(castle.id)];
}
export function castleForSettingsKey(key: string, state: GameStateV2 | null) {
  return key === STORM_SETTINGS_KEY
    ? Object.values(state?.castles ?? {}).find((castle) => castle.kingdomId === 4)
    : state?.castles[key];
}
export function parseStormLegacyKey(value: unknown): string | undefined {
  return typeof value === 'string' && /^[1-9]\d*$/.test(value) ? value : undefined;
}
export function stormLegacyKeyFor(state: GameStateV2 | null): string | undefined {
  const castle = castleForSettingsKey(STORM_SETTINGS_KEY, state);
  return castle ? String(castle.id) : undefined;
}
export function stormReserveConfigured(rows: ReadonlyArray<{ id: number; amount: number }> | undefined): boolean {
  return rows?.some((row) => row.id > 0 && row.amount > 0) === true;
}
interface StormKeyNormalization {
  stormLegacyKey?: string;
  dualWrite?: boolean;
}
// Pure draft transformation. Dual-write Save preserves every unmarked key;
// read/Towers normalization hides numeric duplicates without persisting it.
export function normalizeStormKeys<T>(entries: Readonly<Record<string, T>>, state: GameStateV2 | null, options: StormKeyNormalization = {}): Record<string, T> {
  const draft = { ...entries };
  const current = stormLegacyKeyFor(state);
  if (options.stormLegacyKey) delete draft[options.stormLegacyKey];
  if (current && Object.hasOwn(draft, current) && !Object.hasOwn(draft, STORM_SETTINGS_KEY)) draft.storm = draft[current];
  if (options.dualWrite) {
    const entry = draft.storm;
    if (entry !== undefined) {
      draft.storm = entry;
      if (current) draft[current] = entry;
    }
  } else {
    for (const castle of Object.values(state?.castles ?? {})) {
      if (castle.kingdomId === 4) delete draft[String(castle.id)];
    }
  }
  return draft;
}
export function legacyStormRepairKeys<T>(entries: Readonly<Record<string, T>>, state: GameStateV2 | null, stormLegacyKey?: string): string[] {
  if (!state || Object.keys(state.castles).length === 0 || Object.hasOwn(normalizeStormKeys(entries, state, { stormLegacyKey }), STORM_SETTINGS_KEY)) return [];
  return Object.keys(entries).filter((key) => key !== stormLegacyKey && /^\d+$/.test(key) && !state.castles[key]);
}
export function stormRepairDraft<T>(entries: Readonly<Record<string, T>>, key: string, state: GameStateV2 | null, stormLegacyKey?: string): Record<string, T> {
  if (!legacyStormRepairKeys(entries, state, stormLegacyKey).includes(key)) return { ...entries };
  const draft: Record<string, T> = { ...entries, storm: entries[key] };
  delete draft[key];
  return draft;
}
export function stormEditorCastles(castles: CastleOptionV2[], hasEntry: boolean): CastleOptionV2[] {
  if (castles.some((castle) => castle.kingdomId === 4) || !hasEntry) return castles;
  return [...castles, { id: 0, kingdomId: 4, name: 'Storm castle · used when you have one', type: 'Storm', x: 0, y: 0 }];
}
