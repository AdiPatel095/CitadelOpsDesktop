import { useEffect, useMemo, useState } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { useAuth } from '../../context/AuthContext';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { normalizeFeatureSchedules, scheduleAllowsAt } from '../SchedulerTypes';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../disclosure/placement';
import { attributedReceipts, isActiveReceipt } from './automationAttribution';
import { describeFeatureState, featureStateLaneIds, type AutomationStateDescription } from './runtimeState';

/**
 * The current truthful description of one automation (CIT-20): the saved switch, the game's reported status
 * (per lane where the feature has lanes), the connection and the weekly schedule, through the pure
 * `describeFeatureState`. Read-only; `now` ticks every 30 seconds so timed runs and locks expire on screen.
 */
export function useAutomationDescription(featureId: SettingsFeatureId, options: { buildLaneActive?: boolean } = {}): AutomationStateDescription {
  const { automationStates, automationEnabledByKey, automationTimedUntilByKey, gameLoggedIn } = useAuth();
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
    const lanes = featureStateLaneIds(featureId).map((id) => ({
      id,
      runtime: automationStates[id],
      active: id === 'autoBeriWorldBuild' ? options.buildLaneActive !== false : true,
    }));
    return describeFeatureState(
      featureId,
      lanes,
      on ? { configured: true, enabled: true, ...(timedUntil ? { expiresAtMs: timedUntil } : {}) } : { configured: false, enabled: false },
      {
        connected: gameLoggedIn,
        presence: presence.mode ? { mode: presence.mode, checkpointObservedAt: presence.checkpointObservedAt } : undefined,
        schedule: schedule?.enabled ? { enabled: true, allowedNow: scheduleAllowsAt(schedule, new Date(now)) } : undefined,
        connectionSince: state?.session?.changedAt,
        inFlight,
        now,
      },
    ).overall;
  }, [automationStates, featureId, gameLoggedIn, now, on, operations, options.buildLaneActive, presence.checkpointObservedAt, presence.mode, schedule, state?.session?.changedAt, timedUntil]);
}
