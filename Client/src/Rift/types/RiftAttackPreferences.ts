import { parseHorseTravelBoostID, type HorseTravelBoostID } from '../../settings/HorseTravelBoost';

export const RIFT_ATTACK_PREFERENCES_SECTION = 'rift.attackPreferences';

export interface RiftAttackPreferencesV1 {
  version: 1;
  replayHorseTravelBoostId: HorseTravelBoostID;
  maidenHorseTravelBoostId: HorseTravelBoostID;
}

export function defaultRiftAttackPreferences(): RiftAttackPreferencesV1 {
  return { version: 1, replayHorseTravelBoostId: -1, maidenHorseTravelBoostId: -1 };
}

export function parseRiftAttackPreferences(value: unknown): RiftAttackPreferencesV1 {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return defaultRiftAttackPreferences();
  const raw = value as Record<string, unknown>;
  return {
    version: 1,
    replayHorseTravelBoostId: parseHorseTravelBoostID(raw.replayHorseTravelBoostId),
    maidenHorseTravelBoostId: parseHorseTravelBoostID(raw.maidenHorseTravelBoostId),
  };
}
