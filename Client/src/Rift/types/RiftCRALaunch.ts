export interface RiftCRALaunchEntry {
  id: string;
  displayName?: string;
  savedAtUnix?: number;
  commanderID?: number;
  sourceX?: number;
  sourceY?: number;
  targetX?: number;
  targetY?: number;
  kingdomID?: number;
  attackValid?: number;
  waveCount?: number;
  useTravelFeather?: boolean;
  commanderBusy?: boolean;
  canResend?: boolean;
  /** Feather one-way TT (seconds) from last successful inbound cra ack. */
  oneWayTTSeconds?: number;
  minArriveAtUnix?: number;
  lastSuccessAtUnix?: number;
  scheduledArriveAtUnix?: number;
}

export interface RiftCRALaunchState {
  launches: RiftCRALaunchEntry[];
  busyCommanderIDs: number[];
  launchCount: number;
}

function parseEntry(raw: unknown): RiftCRALaunchEntry | null {
  if (!raw || typeof raw !== 'object') return null;
  const p = raw as Record<string, unknown>;
  const id = typeof p.id === 'string' ? p.id : '';
  if (!id) return null;

  const num = (key: string) => {
    const v = Number(p[key]);
    return Number.isFinite(v) ? Math.trunc(v) : undefined;
  };

  const displayName = typeof p.displayName === 'string' ? p.displayName : undefined;

  return {
    id,
    displayName,
    savedAtUnix: num('savedAtUnix'),
    commanderID: num('commanderID'),
    sourceX: num('sourceX'),
    sourceY: num('sourceY'),
    targetX: num('targetX'),
    targetY: num('targetY'),
    kingdomID: num('kingdomID'),
    attackValid: num('attackValid'),
    waveCount: num('waveCount'),
    useTravelFeather: typeof p.useTravelFeather === 'boolean' ? p.useTravelFeather : undefined,
    commanderBusy: typeof p.commanderBusy === 'boolean' ? p.commanderBusy : undefined,
    canResend: typeof p.canResend === 'boolean' ? p.canResend : undefined,
    oneWayTTSeconds: num('oneWayTTSeconds'),
    minArriveAtUnix: num('minArriveAtUnix'),
    lastSuccessAtUnix: num('lastSuccessAtUnix'),
    scheduledArriveAtUnix: num('scheduledArriveAtUnix'),
  };
}

export function parseRiftCRALaunchPayload(raw: unknown): RiftCRALaunchState {
  const empty: RiftCRALaunchState = { launches: [], busyCommanderIDs: [], launchCount: 0 };
  if (!raw || typeof raw !== 'object') return empty;
  const p = raw as Record<string, unknown>;

  const launches: RiftCRALaunchEntry[] = [];
  if (Array.isArray(p.launches)) {
    for (const item of p.launches) {
      const entry = parseEntry(item);
      if (entry) launches.push(entry);
    }
  }

  const busyCommanderIDs: number[] = [];
  if (Array.isArray(p.busyCommanderIDs)) {
    for (const item of p.busyCommanderIDs) {
      const v = Number(item);
      if (Number.isFinite(v)) busyCommanderIDs.push(Math.trunc(v));
    }
  }

  const launchCount = Number(p.launchCount);
  return {
    launches,
    busyCommanderIDs,
    launchCount: Number.isFinite(launchCount) ? Math.trunc(launchCount) : launches.length,
  };
}

export function formatSavedAt(unix: number | undefined): string {
  if (unix == null || !Number.isFinite(unix)) return '—';
  return new Date(unix * 1000).toLocaleString();
}

/** User label or a short fallback from commander / wave count. */
export function riftLaunchLabel(entry: RiftCRALaunchEntry): string {
  const custom = entry.displayName?.trim();
  if (custom) return custom;
  const waves = entry.waveCount ?? 0;
  return `LID ${entry.commanderID ?? '—'} · ${waves} wave${waves === 1 ? '' : 's'}`;
}
