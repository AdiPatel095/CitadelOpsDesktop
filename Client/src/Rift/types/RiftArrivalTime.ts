import type { RiftCRALaunchEntry } from './RiftCRALaunch';

/** Snap unix seconds up to the next whole-minute boundary (…:00). */
export function roundUpToUnixMinute(unixSeconds: number): number {
  if (unixSeconds <= 0) return 0;
  return Math.ceil(unixSeconds / 60) * 60;
}

export function formatLocalArrivalFromUnix(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function formatTravelDuration(seconds: number | undefined): string {
  if (seconds == null || seconds <= 0) return '—';
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

/** Earliest feather arrival as unix seconds on a minute boundary. */
export function minArriveAtUnix(entry: RiftCRALaunchEntry, nowMs = Date.now()): number | null {
  const tt = entry.oneWayTTSeconds;
  if (tt == null || tt <= 0) return null;
  const serverMin = entry.minArriveAtUnix ?? 0;
  const localMin = Math.floor(nowMs / 1000) + tt;
  return roundUpToUnixMinute(Math.max(serverMin, localMin));
}

/** Minutes after earliest feather arrival; 0 = resend ASAP. */
export function stepArrivalOffsetMinutes(currentOffset: number, deltaMinutes: number): number {
  return Math.max(0, currentOffset + deltaMinutes);
}

export function isEarliestOffset(offsetMinutes: number): boolean {
  return offsetMinutes <= 0;
}

/** Scheduled arrival unix: earliest minute top + whole-minute offset (always …:00). */
export function arriveAtUnixFromOffset(
  entry: RiftCRALaunchEntry,
  offsetMinutes: number,
  nowMs = Date.now()
): number | null {
  const min = minArriveAtUnix(entry, nowMs);
  if (min == null) return null;
  if (offsetMinutes <= 0) return min;
  return min + offsetMinutes * 60;
}

export function offsetMinutesFromScheduled(
  entry: RiftCRALaunchEntry,
  scheduledArriveAtUnix: number,
  nowMs = Date.now()
): number {
  const min = minArriveAtUnix(entry, nowMs);
  if (min == null || scheduledArriveAtUnix <= min) return 0;
  return Math.max(0, Math.round((scheduledArriveAtUnix - min) / 60));
}

export function launchAtUnix(arriveAtUnix: number, oneWayTTSeconds: number | undefined): number | null {
  if (oneWayTTSeconds == null || oneWayTTSeconds <= 0) return null;
  return arriveAtUnix - oneWayTTSeconds;
}
