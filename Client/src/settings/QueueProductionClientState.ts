import { queueConfigurationUpdate } from './Configuration';

const CHECK_INTERVAL_SEC_PER_MIN = 60;

export type QueueProductionMode = 'global' | 'perCastle';

export interface QueueProductionItem {
  id: number;
  amount?: number;
}

export interface QueueProductionCastleSettings {
  enabled: boolean;
  items: QueueProductionItem[];
  cursor: number;
}

export interface QueueProductionClientSettingsV1 {
  version: 1;
  mode: QueueProductionMode;
  checkIntervalSec: number;
  globalItems: QueueProductionItem[];
  castles: Record<string, QueueProductionCastleSettings>;
}

interface QueueProductionClientStateOptions {
  configurationSection: string;
  schedulePrefix: string;
  defaultCheckIntervalSec?: number;
  minCheckIntervalSec?: number;
  maxCheckIntervalSec?: number;
}

export function createQueueProductionClientState({
  configurationSection,
  schedulePrefix,
  defaultCheckIntervalSec = 300,
  minCheckIntervalSec = 30,
  maxCheckIntervalSec = 86_400,
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
    return { id: Math.floor(id), amount: Math.floor(amount) };
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
        globalItems: [],
        castles: targetsMapToCastles(payload),
      };
    }

    const explicitCastles = payload.castles && typeof payload.castles === 'object' && !Array.isArray(payload.castles)
      ? Object.fromEntries(
          Object.entries(payload.castles as Record<string, unknown>).map(([castleID, value]) => {
            const castle = value && typeof value === 'object' ? value as Record<string, unknown> : {};
            const items = normalizeItems(castle.items);
            return [castleID, {
              enabled: typeof castle.enabled === 'boolean' ? castle.enabled : items.length > 0,
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
