export interface RiftMaidenCommsSettings {
  unitWodID: number;
}

export const DEFAULT_MAIDEN_PROBE_UNIT_ID = 216;

export function parseRiftMaidenCommsSettings(raw: unknown): RiftMaidenCommsSettings {
  if (raw == null || typeof raw !== 'object') {
    return { unitWodID: DEFAULT_MAIDEN_PROBE_UNIT_ID };
  }
  const unitWodID = Number((raw as { unitWodID?: unknown }).unitWodID);
  if (!Number.isFinite(unitWodID) || unitWodID <= 0) {
    return { unitWodID: DEFAULT_MAIDEN_PROBE_UNIT_ID };
  }
  return { unitWodID: Math.trunc(unitWodID) };
}
