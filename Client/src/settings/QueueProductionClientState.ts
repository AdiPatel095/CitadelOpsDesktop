import { queueConfigurationUpdate } from './Configuration';

const CHECK_INTERVAL_SEC_PER_MIN = 60;

export type QueueProductionMode = 'global' | 'perCastle';

export interface QueueProductionItem {
  id: number;
  minId?: number;
  maxId?: number;
  amount?: number;
}

export interface QueueProductionCastleSettings {
  enabled: boolean;
  kingdomId?: number;
  items: QueueProductionItem[];
  cursor: number;
}

export interface QueueProductionClientSettingsV1 {
  version: 1;
  mode: QueueProductionMode;
  checkIntervalSec: number;
  recruitLevel10OnTitleLoss?: boolean;
  globalItems: QueueProductionItem[];
  castles: Record<string, QueueProductionCastleSettings>;
}

interface QueueProductionClientStateOptions {
  configurationSection: string;
  schedulePrefix: string;
  defaultCheckIntervalSec?: number;
  minCheckIntervalSec?: number;
  maxCheckIntervalSec?: number;
  supportsGloryTitleFallback?: boolean;
}

export interface QueueProductionCastleIdentity {
  id: number;
  kingdomId: number;
}

const STORM_KINGDOM_ID = 4;

function positiveInteger(value: unknown): number | null {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
}

export function queueProductionKnownStormCastleIDs(
  lastScannedAt: unknown,
  mapSourceCastleID: unknown,
  autoStormTarget: unknown,
): number[] {
  const ids = new Set<number>();
  const sourceCastleID = positiveInteger(mapSourceCastleID);
  if (sourceCastleID != null) ids.add(sourceCastleID);
  if (lastScannedAt && typeof lastScannedAt === 'object' && !Array.isArray(lastScannedAt)) {
    Object.keys(lastScannedAt as Record<string, unknown>).forEach((rawID) => {
      const castleID = positiveInteger(rawID);
      if (castleID != null) ids.add(castleID);
    });
  }
  if (autoStormTarget && typeof autoStormTarget === 'object' && !Array.isArray(autoStormTarget)) {
    const target = autoStormTarget as Record<string, unknown>;
    const castleID = positiveInteger(target.castleId);
    if (Number(target.kingdomId) === STORM_KINGDOM_ID && castleID != null) ids.add(castleID);
  }
  return Array.from(ids).sort((left, right) => left - right);
}

export function queueProductionCastleConfigurationKey(
  settings: QueueProductionClientSettingsV1,
  castle: QueueProductionCastleIdentity,
  liveCastles: QueueProductionCastleIdentity[],
  knownStormCastleIDs: number[] = [],
): string {
  const currentKey = String(castle.id);
  const exact = settings.castles[currentKey];
  if (exact && (exact.kingdomId == null || exact.kingdomId === castle.kingdomId)) return currentKey;
  if (castle.kingdomId !== STORM_KINGDOM_ID) return currentKey;

  const stableKeys = Object.entries(settings.castles)
    .filter(([key, candidate]) => key !== currentKey && positiveInteger(key) != null && candidate.kingdomId === STORM_KINGDOM_ID)
    .map(([key]) => key);
  if (stableKeys.length === 1) return stableKeys[0];
  if (stableKeys.length > 1) return currentKey;

  const liveIDs = new Set(liveCastles.map(({ id }) => id));
  const historicalIDs = new Set(knownStormCastleIDs);
  const legacyKeys = Object.entries(settings.castles)
    .filter(([key, candidate]) => {
      const configuredID = positiveInteger(key);
      return candidate.kingdomId == null && configuredID != null && !liveIDs.has(configuredID) && historicalIDs.has(configuredID);
    })
    .map(([key]) => key);
  return legacyKeys.length === 1 ? legacyKeys[0] : currentKey;
}

export function queueProductionLiveCastleForKey(
  settings: QueueProductionClientSettingsV1,
  configurationKey: string,
  liveCastles: QueueProductionCastleIdentity[],
  knownStormCastleIDs: number[] = [],
): QueueProductionCastleIdentity | undefined {
  return liveCastles.find((castle) => (
    queueProductionCastleConfigurationKey(settings, castle, liveCastles, knownStormCastleIDs) === configurationKey
  ));
}

export function applyQueueProductionCastleIdentityMetadata(
  settings: QueueProductionClientSettingsV1,
  liveCastles: QueueProductionCastleIdentity[],
  knownStormCastleIDs: number[] = [],
): QueueProductionClientSettingsV1 {
  const castles = { ...settings.castles };
  liveCastles.forEach((castle) => {
    const key = queueProductionCastleConfigurationKey(settings, castle, liveCastles, knownStormCastleIDs);
    const configured = castles[key];
    if (configured) castles[key] = { ...configured, kingdomId: castle.kingdomId };
  });
  return { ...settings, castles };
}

export function createQueueProductionClientState({
  configurationSection,
  schedulePrefix,
  defaultCheckIntervalSec = 300,
  minCheckIntervalSec = 30,
  maxCheckIntervalSec = 86_400,
  supportsGloryTitleFallback = false,
}: QueueProductionClientStateOptions) {
  const minCheckIntervalMin = Math.ceil(minCheckIntervalSec / CHECK_INTERVAL_SEC_PER_MIN);
  const maxCheckIntervalMin = Math.floor(maxCheckIntervalSec / CHECK_INTERVAL_SEC_PER_MIN);
  const defaultCheckIntervalMin = Math.round(defaultCheckIntervalSec / CHECK_INTERVAL_SEC_PER_MIN);

  const castleScheduleID = (castleID: number | string) => `${schedulePrefix}:${castleID}`;

  const clampCheckIntervalSec = (value: number) => {
    if (!Number.isFinite(value)) return defaultCheckIntervalSec;
    return Math.min(maxCheckIntervalSec, Math.max(minCheckIntervalSec, Math.round(value)));
  };

  const checkIntervalSecToMinutes = (value: number) => Math.min(
    maxCheckIntervalMin,
    Math.max(
      minCheckIntervalMin,
      Math.round(clampCheckIntervalSec(value) / CHECK_INTERVAL_SEC_PER_MIN),
    ),
  );

  const checkIntervalMinutesToSec = (value: number) => (
    Number.isFinite(value)
      ? clampCheckIntervalSec(Math.round(value) * CHECK_INTERVAL_SEC_PER_MIN)
      : defaultCheckIntervalSec
  );

  const normalizeItem = (raw: unknown): QueueProductionItem | null => {
    if (!raw || typeof raw !== 'object') return null;
    const item = raw as Partial<QueueProductionItem>;
    const id = Number(item.id);
    const amount = item.amount == null ? 0 : Number(item.amount);
    if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(amount) || amount < 0) return null;
    const normalized: QueueProductionItem = { id: Math.floor(id), amount: Math.floor(amount) };
    const firstRangeID = Number(item.minId);
    const lastRangeID = Number(item.maxId);
    const rangeIDs = [firstRangeID, lastRangeID]
      .filter((value) => Number.isFinite(value) && value > 0)
      .map((value) => Math.floor(value));
    if (rangeIDs.length > 0) {
      normalized.minId = Math.min(...rangeIDs);
      normalized.maxId = Math.max(...rangeIDs);
    }
    return normalized;
  };

  const normalizeItems = (raw: unknown): QueueProductionItem[] => {
    let items: QueueProductionItem[];
    if (Array.isArray(raw)) {
      items = raw.map(normalizeItem).filter((item): item is QueueProductionItem => item != null);
    } else if (!raw || typeof raw !== 'object') {
      items = [];
    } else {
      items = Object.entries(raw as Record<string, unknown>)
        .map(([itemID, amount]) => normalizeItem({ id: itemID, amount }))
        .filter((item): item is QueueProductionItem => item != null);
    }
    const seen = new Set<number>();
    return items.filter((item) => {
      if (seen.has(item.id)) return false;
      seen.add(item.id);
      return true;
    });
  };

  const normalizeCursor = (raw: unknown, itemCount: number) => {
    const cursor = Number(raw);
    if (!Number.isFinite(cursor) || cursor < 0 || itemCount <= 0) return 0;
    return Math.floor(cursor) % itemCount;
  };

  const targetsMapToCastles = (
    raw: unknown,
    enabledRaw?: unknown,
  ): Record<string, QueueProductionCastleSettings> => {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
    const enabledMap = enabledRaw && typeof enabledRaw === 'object' && !Array.isArray(enabledRaw)
      ? enabledRaw as Record<string, unknown>
      : {};
    const castles: Record<string, QueueProductionCastleSettings> = {};
    Object.entries(raw as Record<string, unknown>).forEach(([castleID, targets]) => {
      const items = normalizeItems(targets);
      castles[castleID] = {
        enabled: typeof enabledMap[castleID] === 'boolean' ? !!enabledMap[castleID] : items.length > 0,
        items,
        cursor: 0,
      };
    });
    Object.entries(enabledMap).forEach(([castleID, enabled]) => {
      if (!castles[castleID]) castles[castleID] = { enabled: !!enabled, items: [], cursor: 0 };
    });
    return castles;
  };

  const defaultSettings = (): QueueProductionClientSettingsV1 => ({
    version: 1,
    mode: 'global',
    checkIntervalSec: defaultCheckIntervalSec,
    ...(supportsGloryTitleFallback ? { recruitLevel10OnTitleLoss: false } : {}),
    globalItems: [],
    castles: {},
  });

  const normalizeSettings = (raw: unknown): QueueProductionClientSettingsV1 => {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return defaultSettings();
    const payload = raw as Record<string, unknown>;
    const hasModernShape = [
      'mode',
      'checkIntervalSec',
      'globalItems',
      'globalTargets',
      'enabledCastles',
      'castles',
      'targets',
    ].some((key) => key in payload);
    if (!hasModernShape) {
      return {
        version: 1,
        mode: 'perCastle',
        checkIntervalSec: defaultCheckIntervalSec,
        ...(supportsGloryTitleFallback ? { recruitLevel10OnTitleLoss: false } : {}),
        globalItems: [],
        castles: targetsMapToCastles(payload),
      };
    }

    const explicitCastles = payload.castles && typeof payload.castles === 'object' && !Array.isArray(payload.castles)
      ? Object.fromEntries(
          Object.entries(payload.castles as Record<string, unknown>).map(([castleID, value]) => {
            const castle = value && typeof value === 'object' ? value as Record<string, unknown> : {};
            const items = normalizeItems(castle.items);
            const kingdomID = castle.kingdomId == null ? Number.NaN : Number(castle.kingdomId);
            return [castleID, {
              enabled: typeof castle.enabled === 'boolean' ? castle.enabled : items.length > 0,
              ...(Number.isInteger(kingdomID) && kingdomID >= 0 ? { kingdomId: kingdomID } : {}),
              items,
              cursor: normalizeCursor(castle.cursor, items.length),
            }];
          }),
        )
      : {};

    return {
      version: 1,
      mode: payload.mode === 'perCastle' ? 'perCastle' : 'global',
      checkIntervalSec: clampCheckIntervalSec(Number(payload.checkIntervalSec)),
      ...(supportsGloryTitleFallback
        ? { recruitLevel10OnTitleLoss: payload.recruitLevel10OnTitleLoss === true }
        : {}),
      globalItems: normalizeItems(payload.globalItems ?? payload.globalTargets),
      castles: {
        ...targetsMapToCastles(payload.targets, payload.enabledCastles),
        ...explicitCastles,
      },
    };
  };

  const persistSettings = (settings: QueueProductionClientSettingsV1) => (
    queueConfigurationUpdate(configurationSection, normalizeSettings(settings))
  );

  return {
    defaultCheckIntervalSec,
    minCheckIntervalSec,
    maxCheckIntervalSec,
    minCheckIntervalMin,
    maxCheckIntervalMin,
    defaultCheckIntervalMin,
    castleScheduleID,
    clampCheckIntervalSec,
    checkIntervalSecToMinutes,
    checkIntervalMinutesToSec,
    defaultSettings,
    normalizeSettings,
    persistSettings,
  };
}
