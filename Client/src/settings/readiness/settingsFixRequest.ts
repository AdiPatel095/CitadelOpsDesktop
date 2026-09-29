import { fixTargetFor } from '../disclosure/fixTargets';
import type { SettingsFeatureId } from '../disclosure/placement';
import type { ReadinessCheck } from './Readiness';

/**
 * Routes a readiness fix from outside a settings editor (the Automation page) into the editor (CIT-20). The
 * request is a PENDING RECORD, not only an event: the editor's content mounts after its draft session loads, so a
 * listener may not exist yet when the request is made. `useSettingsDisclosure` consumes the pending record for its
 * feature on mount, and the event still reaches an editor that is already mounted. A record older than
 * `PENDING_REQUEST_TTL_MS` is ignored, so a request that was never consumed cannot steal focus later. It only moves
 * focus: nothing is saved, started or stopped.
 */
export const SETTINGS_FIX_EVENT = 'citadelops:settings-fix';
export const ROW_FOCUS_EVENT = 'citadelops:fix-before-start';
export const PENDING_REQUEST_TTL_MS = 10_000;

export interface SettingsFixRequest {
  featureId: SettingsFeatureId;
  section: string;
  control: string;
}

interface Pending<T> {
  value: T;
  requestedAt: number;
}

const pendingFixes = new Map<string, Pending<SettingsFixRequest>>();
const pendingRows = new Map<string, Pending<{ featureId: string }>>();

function fresh<T>(store: Map<string, Pending<T>>, key: string, now: number): T | null {
  const record = store.get(key);
  if (!record) return null;
  store.delete(key);
  return now - record.requestedAt <= PENDING_REQUEST_TTL_MS ? record.value : null;
}

export function requestSettingsFix(
  featureId: SettingsFeatureId,
  check: Pick<ReadinessCheck, 'id' | 'slot'>,
  openEditor: () => void,
  now: number = Date.now(),
): void {
  openEditor();
  const target = fixTargetFor(featureId, check);
  if (!target) return;
  const request: SettingsFixRequest = { featureId, section: target.section, control: target.control };
  pendingFixes.set(featureId, { value: request, requestedAt: now });
  // An editor that is already mounted hears the event and consumes the record; a late one takes it on mount.
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent<SettingsFixRequest>(SETTINGS_FIX_EVENT, { detail: request }));
  }
}

/** The pending fix for a feature, once (consumed), or null when there is none or it is older than the TTL. */
export function takePendingSettingsFix(featureId: SettingsFeatureId, now: number = Date.now()): SettingsFixRequest | null {
  return fresh(pendingFixes, featureId, now);
}

/** Drops a fix the event path already delivered, so the mount path cannot replay it. */
export function clearPendingSettingsFix(featureId: SettingsFeatureId): void {
  pendingFixes.delete(featureId);
}

/** "Fix first" from the Start confirmation: the same pending mechanism, aimed at the feature's readiness row. */
export function requestReadinessRowFocus(featureId: string, now: number = Date.now()): void {
  pendingRows.set(featureId, { value: { featureId }, requestedAt: now });
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent<{ featureId: string }>(ROW_FOCUS_EVENT, { detail: { featureId } }));
  }
}

export function takePendingRowFocus(featureId: string, now: number = Date.now()): boolean {
  return fresh(pendingRows, featureId, now) !== null;
}

export function clearPendingRowFocus(featureId: string): void {
  pendingRows.delete(featureId);
}

export function resetPendingRequestsForTests(): void {
  pendingFixes.clear();
  pendingRows.clear();
}
