/** Simple global options for the Auto Beri World (Berimond troop transfer) loop. */
export interface AutoBeriWorldSettings {
  minTroopsToTransfer: number;
  beriCastleCID: number;
  beriMapX: number;
  beriMapY: number;
  /** Unit wodID sent in kut A:[[wid,amount]]. */
  transferTroopWID: number;
  /** Source castle instance id for kut SCID (main world). */
  kutSourceCastleSCID: number;
  /** kut wire CID field (often -1). */
  kutCastleCID: number;
  /** Seconds between full module checks (fuc, then kut/msk when eligible). */
  troopSpaceCheckIntervalSec: number;
}

const STORAGE_KEY = 'autoBeriWorldSettings';

export const DEFAULT_AUTO_BERI_WORLD_SETTINGS: AutoBeriWorldSettings = {
  minTroopsToTransfer: 1,
  beriCastleCID: 0,
  beriMapX: 0,
  beriMapY: 0,
  transferTroopWID: 0,
  kutSourceCastleSCID: 0,
  kutCastleCID: -1,
  troopSpaceCheckIntervalSec: 30,
};

function clampMinTroops(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.round(value));
}

function clampNonNegInt(value: number, fallback = 0): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.max(0, Math.round(value));
}

function clampKutCID(value: number): number {
  if (!Number.isFinite(value)) return -1;
  return Math.round(value);
}

function clampTroopSpaceCheckSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_BERI_WORLD_SETTINGS.troopSpaceCheckIntervalSec;
  return Math.min(3600, Math.max(5, Math.round(value)));
}

/** Normalizes an arbitrary payload (server or storage) into a valid settings object. */
export function parseAutoBeriWorldSettings(payload: unknown): AutoBeriWorldSettings {
  const obj = (payload ?? {}) as Record<string, unknown>;
  const kutCastleCID = obj.kutCastleCID;
  return {
    minTroopsToTransfer: clampMinTroops(Number(obj.minTroopsToTransfer)),
    beriCastleCID: clampNonNegInt(Number(obj.beriCastleCID)),
    beriMapX: clampNonNegInt(Number(obj.beriMapX)),
    beriMapY: clampNonNegInt(Number(obj.beriMapY)),
    transferTroopWID: clampNonNegInt(Number(obj.transferTroopWID)),
    kutSourceCastleSCID: clampNonNegInt(Number(obj.kutSourceCastleSCID)),
    kutCastleCID:
      kutCastleCID === undefined || kutCastleCID === null
        ? DEFAULT_AUTO_BERI_WORLD_SETTINGS.kutCastleCID
        : clampKutCID(Number(kutCastleCID)),
    troopSpaceCheckIntervalSec: clampTroopSpaceCheckSec(Number(obj.troopSpaceCheckIntervalSec)),
  };
}

/** Mirrors server AutoBeriWorld.json into localStorage so the sidebar toggle can send it while disconnected. */
export function applyAutoBeriWorldSettingsToLocalStorage(payload: unknown): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(parseAutoBeriWorldSettings(payload)));
}

/** Loads the cached settings from localStorage (defaults if missing/invalid). */
export function loadAutoBeriWorldSettingsFromStorage(): AutoBeriWorldSettings {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    return { ...DEFAULT_AUTO_BERI_WORLD_SETTINGS };
  }
  try {
    return parseAutoBeriWorldSettings(JSON.parse(raw));
  } catch {
    return { ...DEFAULT_AUTO_BERI_WORLD_SETTINGS };
  }
}

/** Writes settings to localStorage. */
export function persistAutoBeriWorldSettingsToLocalStorage(settings: AutoBeriWorldSettings): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(parseAutoBeriWorldSettings(settings)));
}
