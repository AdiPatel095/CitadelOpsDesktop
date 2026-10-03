import type { GameStateV2 } from '../api/Contracts';
import type { CastleOptionV2 } from '../api/Selectors';

export const STORM_SETTINGS_KEY = 'storm';
type CastleIdentity = { id: number; kingdomId: number };
export function castleSettingsKey(castle: CastleIdentity): string {
  return castle.kingdomId === 4 ? STORM_SETTINGS_KEY : String(castle.id);
}
export function castleSettingsEntry<T>(entries: Readonly<Record<string, T>>, castle: CastleIdentity): T | undefined {
  const key = castleSettingsKey(castle);
  return Object.hasOwn(entries, key) ? entries[key] : entries[String(castle.id)];
}
export function castleForSettingsKey(key: string, state: GameStateV2 | null) {
  return key === STORM_SETTINGS_KEY
    ? Object.values(state?.castles ?? {}).find((castle) => castle.kingdomId === 4)
    : state?.castles[key];
}
// Pure draft transformation. Only the editor's existing Save persists it.
export function normalizeStormKeys<T>(entries: Readonly<Record<string, T>>, state: GameStateV2 | null): Record<string, T> {
  const draft = { ...entries };
  for (const castle of Object.values(state?.castles ?? {}).sort((a, b) => a.id - b.id)) {
    if (castle.kingdomId !== 4 || !Object.hasOwn(draft, String(castle.id))) continue;
    if (!Object.hasOwn(draft, STORM_SETTINGS_KEY)) draft.storm = draft[String(castle.id)];
    delete draft[String(castle.id)];
  }
  return draft;
}
export function legacyStormRepairKeys<T>(entries: Readonly<Record<string, T>>, state: GameStateV2 | null): string[] {
  if (!state || Object.keys(state.castles).length === 0 || Object.hasOwn(normalizeStormKeys(entries, state), STORM_SETTINGS_KEY)) return [];
  return Object.keys(entries).filter((key) => /^\d+$/.test(key) && !state.castles[key]);
}
export function useLegacyAsStorm<T>(entries: Readonly<Record<string, T>>, key: string, state: GameStateV2 | null): Record<string, T> {
  if (!legacyStormRepairKeys(entries, state).includes(key)) return { ...entries };
  const draft: Record<string, T> = { ...entries, storm: entries[key] };
  delete draft[key];
  return draft;
}
export function stormEditorCastles(castles: CastleOptionV2[], hasEntry: boolean): CastleOptionV2[] {
  if (castles.some((castle) => castle.kingdomId === 4) || !hasEntry) return castles;
  return [...castles, { id: 0, kingdomId: 4, name: 'Storm castle · used when you have one', type: 'Storm', x: 0, y: 0 }];
}
