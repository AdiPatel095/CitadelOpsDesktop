import { describeFeaturePlayerStatus } from './automationPlayerStatusDisplay';
import { useEffect, useMemo, useState } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { useAuth } from '../../context/AuthContext';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { normalizeFeatureSchedules, scheduleAllowsAt } from '../SchedulerTypes';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../disclosure/placement';
import { attributedReceipts, isActiveReceipt } from './automationAttribution';

/**
 * The current truthful description of one automation (CIT-20): the saved switch, the game's reported status
 * (per lane where the feature has lanes), the connection and the weekly schedule, through the pure
 * `automationPlayerStatus`. Read-only; `now` ticks every 30 seconds so timed runs and locks expire on screen.
 */
export function useAutomationPlayerStatus(featureId: SettingsFeatureId, options: { buildLaneActive?: boolean } = {}) {
  const { automationStates, automationEnabledByKey, automationTimedUntilByKey, gameLoggedIn, botLocked } = useAuth();
  const { state, configuration, operations } = useCitadelAPI();
  const presence = useHostedRuntimePresence();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(interval);
  }, []);
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const on = automationEnabledByKey[enabledKey] === true;
  const timedUntil = automationTimedUntilByKey[enabledKey];
  const scheduleRaw = configuration?.sections?.scheduler as { featureSchedules?: unknown } | undefined;
  const schedule = normalizeFeatureSchedules(scheduleRaw?.featureSchedules)[featureId];
  return useMemo(() => {
    const inFlight = attributedReceipts(featureId, operations).filter((receipt) => isActiveReceipt(receipt)).length;
    const control = on ? { configured: true, enabled: true, ...(timedUntil ? { expiresAtMs: timedUntil } : {}) } : { configured: false, enabled: false };
    const context = {
      connected: gameLoggedIn,
      presence: presence.mode ? { mode: presence.mode, checkpointObservedAt: presence.checkpointObservedAt } : undefined,
      schedule: schedule?.enabled ? { enabled: true, allowedNow: scheduleAllowsAt(schedule, new Date(now)) } : undefined,
      connectionSince: state?.session?.changedAt, inFlight, now,
    };
    return describeFeaturePlayerStatus({ featureId, states: automationStates, control, context, desktopLocked: botLocked, buildLaneActive: options.buildLaneActive });
  }, [automationStates, botLocked, featureId, gameLoggedIn, now, on, operations, options.buildLaneActive, presence.checkpointObservedAt, presence.mode, schedule, state?.session?.changedAt, timedUntil]);
}
