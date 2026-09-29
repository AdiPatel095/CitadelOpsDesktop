import { fixTargetFor } from '../disclosure/fixTargets';
import type { SettingsFeatureId } from '../disclosure/placement';
import type { ReadinessCheck } from './Readiness';

/**
 * Routes a readiness fix from outside a settings editor (the Automation page) into the editor (CIT-20). The
 * caller opens the editor, then this announces the target; the editor's `useSettingsDisclosure` reveals the
 * section (expanding a collapsed Advanced group) and focuses the control once it is mounted. It only moves
 * focus: nothing is saved, started or stopped.
 */
export const SETTINGS_FIX_EVENT = 'citadelops:settings-fix';

export interface SettingsFixRequest {
  featureId: SettingsFeatureId;
  section: string;
  control: string;
}

export function requestSettingsFix(
  featureId: SettingsFeatureId,
  check: Pick<ReadinessCheck, 'id' | 'slot'>,
  openEditor: () => void,
): void {
  openEditor();
  const target = fixTargetFor(featureId, check);
  if (!target || typeof window === 'undefined') return;
  // Two frames: the editor mounts, then its draft session finishes loading.
  window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
    window.dispatchEvent(new CustomEvent<SettingsFixRequest>(SETTINGS_FIX_EVENT, {
      detail: { featureId, section: target.section, control: target.control },
    }));
  }));
}
