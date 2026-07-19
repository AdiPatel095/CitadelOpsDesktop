import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { configurationSection } from './Configuration';
import {
  normalizeFeatureSchedules,
  scheduleAllowsAt,
  type WeeklySchedule,
} from './SchedulerTypes';

export const AUTO_EQUIPMENT_CLEANUP_FEATURE_ID = 'autoEquipmentCleanup';

const enabledStorageKey = 'equipmentAutoSellNonRelicEquipment';
const intervalStorageKey = 'equipmentAutoSellNonRelicEquipmentIntervalMinutes';

export interface AutoEquipmentCleanupController {
  enabled: boolean;
  intervalMinutes: number;
  schedule?: WeeklySchedule;
  setEnabled: (enabled: boolean) => void;
  setIntervalMinutes: (minutes: number) => void;
}

export function useAutoEquipmentCleanup(): AutoEquipmentCleanupController {
  const { state, configuration, submitIntent } = useCitadelAPI();
  const [enabled, setEnabled] = useState(() => readBoolean(enabledStorageKey));
  const [intervalMinutes, setIntervalMinutes] = useState(() => readInterval());
  const runningRef = useRef(false);
  const schedule = useMemo(() => (
    normalizeFeatureSchedules(configurationSection(configuration, 'scheduler').featureSchedules)[AUTO_EQUIPMENT_CLEANUP_FEATURE_ID]
  ), [configuration?.sections.scheduler]);

  useEffect(() => {
    try {
      localStorage.setItem(enabledStorageKey, String(enabled));
      localStorage.setItem(intervalStorageKey, String(intervalMinutes));
    } catch {
      // Browser privacy settings may disable local storage; the live setting still works.
    }
  }, [enabled, intervalMinutes]);

  useEffect(() => {
    if (!enabled || !state?.session.loggedIn) return;

    const runCleanup = async () => {
      if (runningRef.current || (schedule && !scheduleAllowsAt(schedule))) return;
      runningRef.current = true;
      try {
        await submitIntent('equipment.refresh', {}, { actor: 'ui:auto-equipment-cleanup' });
        await submitIntent('equipment.sell', {
          category: 'non_relic_equipment',
          sellLookItems: false,
          sellPost2026: false,
        }, { actor: 'ui:auto-equipment-cleanup' });
        await submitIntent('equipment.refresh', {}, { actor: 'ui:auto-equipment-cleanup' });
        await submitIntent('equipment.sell', {
          category: 'non_relic_gems',
          sellPost2026: false,
        }, { actor: 'ui:auto-equipment-cleanup' });
      } catch {
        // Intent failures already publish a user-facing notification.
      } finally {
        runningRef.current = false;
      }
    };

    void runCleanup();
    const timer = window.setInterval(() => void runCleanup(), intervalMinutes * 60_000);
    return () => window.clearInterval(timer);
  }, [enabled, intervalMinutes, schedule, state?.session.loggedIn, submitIntent]);

  const updateIntervalMinutes = useCallback((minutes: number) => {
    setIntervalMinutes(clampInterval(minutes));
  }, []);

  return {
    enabled,
    intervalMinutes,
    schedule,
    setEnabled,
    setIntervalMinutes: updateIntervalMinutes,
  };
}

function clampInterval(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.max(1, Math.min(1440, Math.round(value)));
}

function readBoolean(key: string): boolean {
  try {
    return localStorage.getItem(key) === 'true';
  } catch {
    return false;
  }
}

function readInterval(): number {
  try {
    return clampInterval(Number(localStorage.getItem(intervalStorageKey) ?? 1));
  } catch {
    return 1;
  }
}
