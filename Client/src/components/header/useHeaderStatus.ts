import { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../../context/AuthContext';
import { useCitadelAPI } from '../../api/ApiContext';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { useAutomationDescriptions } from '../../settings/readiness/useAutomationDescription';
import { describeFeaturePlayerStatus } from '../../settings/readiness/automationPlayerStatusDisplay';
import { headerPlayerStatus } from '../playerStatusDisplay';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../../settings/disclosure/placement';
import { normalizeFeatureSchedules, scheduleAllowsAt } from '../../settings/SchedulerTypes';
import { attributedReceipts, isActiveReceipt } from '../../settings/readiness/automationAttribution';
import { parseAutoBeriWorldSettings } from '../../settings/AutoBeriWorldClientState';
import { configurationSection } from '../../settings/Configuration';
import { attentionEntries, prioritySignal } from './headerStatus';
import type { PlayerStatusDescription } from '../../settings/readiness/playerStatus';
export function useHeaderStatus(surface: 'desktop' | 'hosted', accountStatus?: PlayerStatusDescription) {
  const auth = useAuth();
  const { state, configuration, operations } = useCitadelAPI();
  const presence = useHostedRuntimePresence();
  const descriptions = useAutomationDescriptions();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);
  const features = useMemo(() => {
    const raw = configuration?.sections?.scheduler as { featureSchedules?: unknown } | undefined;
    const schedules = normalizeFeatureSchedules(raw?.featureSchedules);
    return (Object.keys(AUTOMATION_ENABLED_KEYS) as SettingsFeatureId[]).map(featureId => {
      const key = AUTOMATION_ENABLED_KEYS[featureId];
      const enabled = auth.automationEnabledByKey[key] === true;
      const schedule = schedules[featureId];
      const value = describeFeaturePlayerStatus({ featureId, states: auth.automationStates,
        control: { configured: enabled, enabled, expiresAtMs: auth.automationTimedUntilByKey[key] }, desktopLocked: false,
        buildLaneActive: featureId !== 'autoBeriWorld' || parseAutoBeriWorldSettings(configurationSection(configuration, 'automation.autoBeriWorld')).build.enabled,
        context: { connected: auth.gameLoggedIn, presence, now,
          schedule: schedule?.enabled ? { enabled: true, allowedNow: scheduleAllowsAt(schedule, new Date(now)) } : undefined,
          connectionSince: state?.session.changedAt, inFlight: attributedReceipts(featureId, operations).filter(isActiveReceipt).length } }).overall;
      return { featureId, ...value };
    });
  }, [auth.automationEnabledByKey, auth.automationStates, auth.automationTimedUntilByKey, auth.gameLoggedIn, configuration, now, operations, presence, state?.session.changedAt, descriptions]);
  const station = features.find(feature => feature.featureId === 'autoStation')!;
  const bird = features.find(feature => feature.featureId === 'autoBird')!;
  const connection = headerPlayerStatus({ surface, status: auth.gameConnectionState, loggedIn: auth.gameLoggedIn,
    dashboard: auth.dashboardConnectionStatus, started: auth.hasGameConnectionStatus || auth.gameBrowserRunning,
    loginFailure: state?.session.loginFailure, cooldownUntil: state?.session.cooldownUntil, retryAt: state?.session.retryAt, now,
    checkpoint: presence.mode === 'checkpoint', checkpointObservedAt: presence.checkpointObservedAt,
    detail: auth.gameConnectionDetail ? { text: auth.gameConnectionDetail } : undefined, accountStatus });
  const signal = prioritySignal({ now, station: { enabled: auth.autoStationEnabled, threatCount: auth.autoStationThreatCount, nextImpactAt: auth.autoStationNextImpact, status: station.status },
    bird: { enabled: auth.autoBirdEnabled, nextWakeAt: auth.autoBirdNextWakeUp, nextCastleName: auth.autoBirdNextCastleName }, features });
  const activeStates = ['connecting', 'authenticating', 'connected', 'cooldown', 'reconnecting', 'suspended', 'released'];
  return { now, connection, station, bird, signal, stationThreatCount: auth.autoStationThreatCount, stationNextImpact: auth.autoStationNextImpact, attention: attentionEntries(features), presence, dailyAttacks: state?.dailyAttacks,
    gameConnectionActive: auth.hasGameConnectionStatus && activeStates.includes(auth.gameConnectionState),
    gameReconnectAvailable: auth.hasGameConnectionStatus && ['cooldown', 'reconnecting', 'suspended', 'released'].includes(auth.gameConnectionState),
    connectionControlsReady: auth.dashboardConnectionStatus === 'Connected' && auth.hasGameConnectionStatus && auth.gameConnectionState !== 'starting' };
}
