import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReadinessCheck } from '../readiness/Readiness';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import { fixTargetFor, type SettingsFixTarget } from './fixTargets';
import {
  advancedSectionIds,
  sectionPlacement,
  settingsSectionElementId,
  type SettingsFeatureId,
  type SettingsSectionPlacement,
} from './placement';

/**
 * Per-feature memory of which Advanced sections are expanded (CIT-17). A
 * per-browser viewing preference only: it never enters configuration, never
 * changes the draft and never saves, starts or stops anything. Every Advanced
 * section starts collapsed; storage failures (private windows, quota, blocked
 * site data) fall back to that default.
 */
export const SETTINGS_DISCLOSURE_STORAGE_PREFIX = 'citadelops.settings.disclosure.v1.';

type DisclosureStorage = Pick<Storage, 'getItem' | 'setItem'>;

function browserStorage(): DisclosureStorage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

/** Expanded Advanced section ids remembered for a feature; unknown or malformed entries are dropped. */
export function readSettingsDisclosure(
  featureId: SettingsFeatureId,
  storage: DisclosureStorage | null = browserStorage(),
): string[] {
  if (!storage) return [];
  try {
    const raw = storage.getItem(SETTINGS_DISCLOSURE_STORAGE_PREFIX + featureId);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as { version?: unknown; expanded?: unknown };
    if (parsed?.version !== 1 || !Array.isArray(parsed.expanded)) return [];
    const known = new Set(advancedSectionIds(featureId));
    return [...new Set(parsed.expanded.filter((id): id is string => typeof id === 'string' && known.has(id)))];
  } catch {
    return [];
  }
}

export function writeSettingsDisclosure(
  featureId: SettingsFeatureId,
  expanded: readonly string[],
  storage: DisclosureStorage | null = browserStorage(),
): void {
  if (!storage) return;
  try {
    storage.setItem(SETTINGS_DISCLOSURE_STORAGE_PREFIX + featureId, JSON.stringify({ version: 1, expanded: [...expanded].sort() }));
  } catch {
    // A viewing preference; losing it only means sections open collapsed next time.
  }
}

/** Pure toggle, exported for tests. */
export function toggleSettingsDisclosure(expanded: readonly string[], sectionId: string): string[] {
  return expanded.includes(sectionId) ? expanded.filter((id) => id !== sectionId) : [...expanded, sectionId];
}

export interface CollapsedFixTarget {
  target: SettingsFixTarget;
  placement: SettingsSectionPlacement;
}

export interface SettingsDisclosure {
  featureId: SettingsFeatureId;
  expandedIds: readonly string[];
  isExpanded: (sectionId: string) => boolean;
  toggle: (sectionId: string) => void;
  expand: (sectionId: string) => void;
  /** Expands the section when it is Advanced, then focuses the control once it is visible. */
  reveal: (sectionId: string, controlId?: string) => void;
  /** Routes a readiness Fix to its control through the fix-target table; false when unmapped. */
  fix: (check: ReadinessCheck) => boolean;
  /** The collapsed Advanced section that holds the control a non-valid check points at, if any. */
  collapsedTarget: (check: ReadinessCheck) => CollapsedFixTarget | null;
}

export function useSettingsDisclosure(featureId: SettingsFeatureId): SettingsDisclosure {
  const [expandedIds, setExpandedIds] = useState<string[]>(() => readSettingsDisclosure(featureId));
  const [focusRequest, setFocusRequest] = useState<{ controlId: string; sequence: number } | null>(null);

  useEffect(() => {
    // Focus after the commit that made the section visible.
    if (focusRequest) focusReadinessTarget(focusRequest.controlId);
  }, [focusRequest]);

  const update = useCallback((next: (current: string[]) => string[]) => {
    setExpandedIds((current) => {
      const updated = next(current);
      if (updated !== current) writeSettingsDisclosure(featureId, updated);
      return updated;
    });
  }, [featureId]);

  const expand = useCallback((sectionId: string) => {
    if (sectionPlacement(featureId, sectionId).tier !== 'advanced') return;
    update((current) => (current.includes(sectionId) ? current : [...current, sectionId]));
  }, [featureId, update]);

  const toggle = useCallback((sectionId: string) => {
    if (sectionPlacement(featureId, sectionId).tier !== 'advanced') return;
    update((current) => toggleSettingsDisclosure(current, sectionId));
  }, [featureId, update]);

  const reveal = useCallback((sectionId: string, controlId?: string) => {
    expand(sectionId);
    setFocusRequest((current) => ({
      controlId: controlId ?? settingsSectionElementId(featureId, sectionId),
      sequence: (current?.sequence ?? 0) + 1,
    }));
  }, [expand, featureId]);

  const fix = useCallback((check: ReadinessCheck) => {
    const target = fixTargetFor(featureId, check);
    if (!target) return false;
    reveal(target.section, target.control);
    return true;
  }, [featureId, reveal]);

  const collapsedTarget = useCallback((check: ReadinessCheck): CollapsedFixTarget | null => {
    if (check.state === 'valid') return null;
    const target = fixTargetFor(featureId, check);
    if (!target) return null;
    const placement = sectionPlacement(featureId, target.section);
    if (placement.tier !== 'advanced' || expandedIds.includes(target.section)) return null;
    return { target, placement };
  }, [expandedIds, featureId]);

  return useMemo(() => ({
    featureId,
    expandedIds,
    isExpanded: (sectionId: string) => expandedIds.includes(sectionId),
    toggle,
    expand,
    reveal,
    fix,
    collapsedTarget,
  }), [collapsedTarget, expand, expandedIds, featureId, fix, reveal, toggle]);
}
