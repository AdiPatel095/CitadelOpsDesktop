import { useCallback, useEffect, useState } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { configurationSection } from './Configuration';
import {
  normalizeFeatureSchedules,
  type WeeklySchedule,
} from './SchedulerTypes';
import { useAuth } from '../context/AuthContext';

export const AUTO_EQUIPMENT_CLEANUP_FEATURE_ID = 'autoEquipmentCleanup';
export const AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY = 'auto_equipment_cleanup';
export const AUTO_EQUIPMENT_CLEANUP_SECTION = 'automation.autoEquipmentCleanup';

export interface AutoEquipmentCleanupController {
  enabled: boolean;
  intervalMinutes: number;
  schedule?: WeeklySchedule;
  setEnabled: (enabled: boolean) => void;
  setIntervalMinutes: (minutes: number) => void;
}

export function useAutoEquipmentCleanup(): AutoEquipmentCleanupController {
  const { configuration, updateConfiguration } = useCitadelAPI();
  const { automationEnabledByKey, setAutomationEnabled } = useAuth();
  const configuredIntervalMinutes = intervalFromConfiguration(
    configurationSection(configuration, AUTO_EQUIPMENT_CLEANUP_SECTION),
  );
  const [intervalMinutes, setIntervalMinutes] = useState(configuredIntervalMinutes);
  const enabled = automationEnabledByKey[AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY] === true;
  const schedule = normalizeFeatureSchedules(
    configurationSection(configuration, 'scheduler').featureSchedules,
  )[AUTO_EQUIPMENT_CLEANUP_FEATURE_ID];

  useEffect(() => {
    setIntervalMinutes(configuredIntervalMinutes);
  }, [configuredIntervalMinutes]);

  const updateIntervalMinutes = useCallback((minutes: number) => {
    const normalized = clampInterval(minutes);
    setIntervalMinutes(normalized);
    void updateConfiguration(AUTO_EQUIPMENT_CLEANUP_SECTION, {
      version: 1,
      checkIntervalSec: normalized * 60,
    }).catch(() => setIntervalMinutes(configuredIntervalMinutes));
  }, [configuredIntervalMinutes, updateConfiguration]);

  const updateEnabled = useCallback((value: boolean) => {
    void setAutomationEnabled(AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY, value);
  }, [setAutomationEnabled]);

  return {
    enabled,
    intervalMinutes,
    schedule,
    setEnabled: updateEnabled,
    setIntervalMinutes: updateIntervalMinutes,
  };
}

function clampInterval(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.max(1, Math.min(1440, Math.round(value)));
}

function intervalFromConfiguration(section: Record<string, unknown>): number {
  const seconds = Number(section.checkIntervalSec ?? 60);
  return clampInterval(seconds / 60);
}
