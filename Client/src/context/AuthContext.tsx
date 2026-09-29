import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { StartConfirmDialog, type PendingStart } from '../components/StartConfirmDialog';
import { useHostedRuntimePresence } from '../config/Deployment';
import { movementViewFromState } from '../Movement/types/MovementState';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../settings/disclosure/placement';
import { evaluateFeatureReadiness, startRequiresConfirmation } from '../settings/readiness/featureReadiness';
import { clearEnabledSince, readEnabledSince, recordEnabledSince } from '../settings/readiness/firstResult';
import { accountKey } from '../settings/requirements/castleRequirements';
import { normalizeFeatureSchedules } from '../settings/SchedulerTypes';
import { useMetadata } from './MetadataContext';
import type { RecruitTroopsMode } from '../settings/RecruitTroopsClientState';
import type { AutoToolMode } from '../settings/AutoToolClientState';
import type { AutomationStateV2, StationingOperationV2 } from '../api/Contracts';
import {
	nextAutomationExpirationMs,
	parseAutomationEnabledControls,
	timedAutomationEnabledValue,
} from '../settings/AutomationEnabled';

export type GameConnectionState =
  | 'stopped'
  | 'starting'
  | 'connecting'
  | 'authenticating'
  | 'connected'
  | 'cooldown'
  | 'reconnecting'
  | 'suspended'
  | 'released'
  | 'disconnected'
  | 'error';

export type DashboardConnectionStatus = 'Disconnected' | 'Connecting' | 'Connected';

/** A rejected write of `automation.enabled`, kept until the next write of that feature succeeds (CIT-20). */
export interface AutomationWriteFailure {
  intent: 'start' | 'stop';
  message: string;
  at: number;
}

export interface AutoBirdCastleCycle {
 paused?: boolean;
 pausedUntilMs?: number;
 rescanRequested?: boolean;
	castleId: number;
	castleName: string;
	kingdomId: number;
	nextCycleAtMs: number;
	phase?: StationingOperationV2['phase'];
	statusDetail?: string;
	delayHours?: number;
	waitSeconds?: number;
	travelSeconds?: number;
}

interface AuthContextType {
  gameLoggedIn: boolean;
  gameLoginCooldown: number;
  gameLoginRetrySeconds: number;
  gameConnectionState: GameConnectionState;
  gameSocketConnected: boolean;
  gameBrowserRunning: boolean;
	gameBrowserName: string;
  gameConnectionDetail: string;
  dashboardConnectionStatus: DashboardConnectionStatus;
  hasGameConnectionStatus: boolean;
  isGameDataReady: boolean;
  recruitTroopsEnabled: boolean;
  autoRecruitMode: RecruitTroopsMode;
  autoToolEnabled: boolean;
  autoToolMode: AutoToolMode;
  autoSceatResEnabled: boolean;
  autoFoodBalanceEnabled: boolean;
  autoHospitalEnabled: boolean;
  autoTCIEnabled: boolean;
	autoTCINextWakeUp: number;
	autoTowerEnabled: boolean;
	autoFortressEnabled: boolean;
	autoInvasionEnabled: boolean;
	autoNomadEnabled: boolean;
	autoAdvisorEnabled: boolean;
	autoBoosterEnabled: boolean;
	autoBuyerEnabled: boolean;
	autoKhanEnabled: boolean;
	autoBeriWorldEnabled: boolean;
	autoStormEnabled: boolean;
	autoBirdEnabled: boolean;
  autoBirdNextWakeUp: number;
	autoBirdNextCastleName: string;
	autoBirdCastleCycles: AutoBirdCastleCycle[];
  autoStationEnabled: boolean;
  autoStationState: string;
  autoStationThreatCount: number;
  autoStationNextImpact: number;
  autoStationDetail: string;
  goMem: number;
  browserMem: number;
	botLocked: boolean;
	automationStates: Record<string, AutomationStateV2>;
	automationEnabledByKey: Record<string, boolean>;
	automationTimedUntilByKey: Record<string, number>;
	/** Failed Start/Stop writes by `automation.enabled` key; the switch itself always follows the saved value. */
	automationWriteFailures: Record<string, AutomationWriteFailure>;
	/** When this account turned each automation on (ISO), from configuration changes seen on this device. */
	automationEnabledSince: Record<string, string>;
  startGame: () => void;
  stopGame: () => void;
  /** Force a fresh game connection now, bypassing a scheduled retry, cooldown wait, or login park. */
  reconnectGame: () => void;
  toggleRecruitTroops: () => void;
  toggleAutoTool: () => void;
  toggleAutoSceatRes: () => void;
  toggleAutoFoodBalance: () => void;
  toggleAutoHospital: () => void;
	toggleAutoTCI: () => void;
	toggleAutoTower: () => void;
	toggleAutoFortress: () => void;
	toggleAutoInvasion: () => void;
	toggleAutoNomad: () => void;
	toggleAutoAdvisor: () => void;
	toggleAutoBooster: () => void;
	toggleAutoBuyer: () => void;
	toggleAutoKhan: () => void;
	toggleAutoBeriWorld: () => void;
	toggleAutoStorm: () => void;
	toggleAutoBird: () => void;
	toggleAutoStation: () => void;
	toggleBotLock: () => void;
	/**
	 * Writes `automation.enabled[feature]`. Turning on first previews the saved settings; blocked checks open a
	 * confirmation (the game's own guards stay authoritative), and declining leaves the switch off. The
	 * preview and confirmation submit nothing; the switch write is the only mutation.
	 */
	setAutomationEnabled: (feature: string, enabled: boolean) => Promise<void>;
	enableAutomationFor: (feature: string, durationMinutes: number) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { connectionStatus, state, catalogs, configuration, diagnostics, submitIntent, updateConfiguration, loadLatestConfiguration } = useCitadelAPI();
  const metadata = useMetadata();
  const hostedPresence = useHostedRuntimePresence();
  const session = state?.session;
	const automationEnabled = isRecord(configuration?.sections['automation.enabled'])
		? configuration.sections['automation.enabled'] as Record<string, unknown>
		: {};
	const schedulerConfiguration = isRecord(configuration?.sections.scheduler)
		? configuration.sections.scheduler as Record<string, unknown>
		: {};
	const [now, setNow] = useState(() => Date.now());
	const automationControls = parseAutomationEnabledControls(automationEnabled, now);
	const automationEnabledByKey = Object.fromEntries(
		Object.entries(automationControls).map(([feature, control]) => [feature, control.enabled]),
	);
	const automationTimedUntilByKey = Object.fromEntries(
		Object.entries(automationControls).flatMap(([feature, control]) => (
			control.enabled && control.expiresAtMs ? [[feature, control.expiresAtMs]] : []
		)),
	);
	const nextAutomationExpiry = nextAutomationExpirationMs(automationControls, now);
	const botLocked = schedulerConfiguration.botLocked === true;
	const recruitTroopsEnabled = automationEnabledByKey.recruit_troops === true;
	const autoToolEnabled = automationEnabledByKey.auto_tool === true;
	const autoSceatResEnabled = automationEnabledByKey.auto_sceat_resources === true;
	const autoFoodBalanceEnabled = automationEnabledByKey.auto_food_balance === true;
	const autoHospitalEnabled = automationEnabledByKey.auto_hospital === true;
	const autoTCIEnabled = automationEnabledByKey.auto_tci === true;
	const autoTowerEnabled = automationEnabledByKey.auto_towers === true;
	const autoFortressEnabled = automationEnabledByKey.auto_fortress === true;
	const autoInvasionEnabled = automationEnabledByKey.auto_invasion === true;
	const autoNomadEnabled = automationEnabledByKey.auto_nomad === true;
	const autoAdvisorEnabled = automationEnabledByKey.auto_advisor === true;
	const autoBoosterEnabled = automationEnabledByKey.auto_booster === true;
	const autoBuyerEnabled = automationEnabledByKey.auto_buyer === true;
	const autoKhanEnabled = automationEnabledByKey.auto_khan === true;
	const autoBeriWorldEnabled = automationEnabledByKey.auto_beri_world === true;
	const autoStormEnabled = automationEnabledByKey.auto_storm === true;
	const autoBirdEnabled = automationEnabledByKey.auto_bird === true;
	const autoStationEnabled = automationEnabledByKey.auto_station === true;
	const automationStates = state?.automations ?? {};
	const autoBirdState = automationStates.autoBird;
	const autoBirdNextCastleID = Math.trunc(automationMetricMillis(autoBirdState, 'nextBirdCastleId'));
	const autoBirdNextCastleName = autoBirdNextCastleID > 0
		? state?.castles[String(autoBirdNextCastleID)]?.name?.trim() || `Castle ${autoBirdNextCastleID}`
		: '';
	const autoBirdCastleCycles = useMemo<AutoBirdCastleCycle[]>(() => {
		return Object.values(state?.castles ?? {})
			.map((castle) => {
				const operation = state?.stationing?.[`autoBird:${castle.id}`];
 const control = state?.stationing?.[`autoBirdControl:${castle.id}`];
				const metricReturn = automationMetricMillis(autoBirdState, `birdReturnUnixMs.${castle.id}`);
				const recordedReturn = Date.parse(operation?.expectedReturnAt ?? '');
				return {
					castleId: castle.id,
 paused: control?.paused,
 pausedUntilMs: Date.parse(control?.pausedUntil ?? '') || 0,
 rescanRequested: control?.rescanRequested,
					castleName: castle.name?.trim() || `Castle ${castle.id}`,
					kingdomId: castle.kingdomId,
					nextCycleAtMs: metricReturn > 0
						? metricReturn
						: Number.isFinite(recordedReturn) ? recordedReturn : 0,
					phase: operation?.phase,
					statusDetail: operation?.statusDetail,
					delayHours: operation?.delayHours,
					waitSeconds: operation?.waitSeconds,
					travelSeconds: operation?.travelSeconds,
				};
			})
			.sort((left, right) => {
				const leftActive = left.nextCycleAtMs > 0;
				const rightActive = right.nextCycleAtMs > 0;
				if (leftActive !== rightActive) return leftActive ? -1 : 1;
				if (leftActive && left.nextCycleAtMs !== right.nextCycleAtMs) {
					return left.nextCycleAtMs - right.nextCycleAtMs;
				}
				return left.castleName.localeCompare(right.castleName);
			});
	}, [autoBirdState, state?.castles, state?.stationing]);
	const autoStationState = automationStates.autoStation;
  const gameLoggedIn = connectionStatus === 'Connected' && session?.loggedIn === true && session.socketReady === true;
  const gameConnectionState = normalizeConnectionState(session?.status, gameLoggedIn);
	useEffect(() => {
		if (session?.cooldownUntil == null && session?.retryAt == null) return;
		const interval = window.setInterval(() => setNow(Date.now()), 1000);
		return () => window.clearInterval(interval);
	}, [session?.cooldownUntil, session?.retryAt]);
	useEffect(() => {
		if (nextAutomationExpiry <= 0) return;
		const timeout = window.setTimeout(
			() => setNow(Date.now()),
			Math.max(1, nextAutomationExpiry - Date.now() + 25),
		);
		return () => window.clearTimeout(timeout);
	}, [nextAutomationExpiry]);
	const cooldownSeconds = secondsUntil(session?.cooldownUntil, now);
	const retrySeconds = cooldownSeconds > 0 ? 0 : secondsUntil(session?.retryAt, now);

  const submit = (name: string, argumentsValue: Record<string, unknown> = {}) => {
    void submitIntent(name, argumentsValue).catch((error) => {
      console.error(`Intent ${name} failed`, error);
    });
  };

  const latest = useRef({ state, configuration, metadata, gameLoggedIn, hostedPresence });
  latest.current = { state, configuration, metadata, gameLoggedIn, hostedPresence };
  const [writeFailures, setWriteFailures] = useState<Record<string, AutomationWriteFailure>>({});
  const [pendingStart, setPendingStart] = useState<PendingStart | null>(null);

  const rememberFailure = useCallback((feature: string, intent: 'start' | 'stop', error: unknown) => {
    const message = error instanceof Error && error.message.trim() ? error.message : '';
    setWriteFailures((current) => ({ ...current, [feature]: { intent, message, at: Date.now() } }));
  }, []);
  const forgetFailure = useCallback((feature: string) => {
    setWriteFailures((current) => {
      if (!(feature in current)) return current;
      const rest = { ...current };
      delete rest[feature];
      return rest;
    });
  }, []);

  /**
   * Start-time revalidation (CIT-20): previews the LATEST saved configuration with the current observations.
   * Blocked checks ask for confirmation; unresolved ones (decided by the game, waiting for data) never block.
   * Submits nothing; the game's fail-closed guards decide what actually runs.
   */
  const confirmStart = useCallback(async (feature: string): Promise<boolean> => {
    const featureId = (Object.keys(AUTOMATION_ENABLED_KEYS) as SettingsFeatureId[]).find((id) => AUTOMATION_ENABLED_KEYS[id] === feature);
    if (!featureId) return true;
    let sections = latest.current.configuration?.sections;
    try {
      sections = (await loadLatestConfiguration()).sections;
    } catch {
      // Keep the configuration already loaded; the write below reports its own failure.
    }
    const current = latest.current;
    const session = current.state?.session ?? null;
    const report = evaluateFeatureReadiness(featureId, {
      sections,
      state: current.state,
      observation: {
        session, connected: current.gameLoggedIn,
        hostedPresence: current.hostedPresence?.mode ? { mode: current.hostedPresence.mode, checkpointObservedAt: current.hostedPresence.checkpointObservedAt } : undefined,
      },
      troops: current.metadata.troops, tools: current.metadata.tools,
      metadataReady: !current.metadata.unitsLoading && !current.metadata.unitsError,
      resources: current.metadata.resources,
      movement: movementViewFromState(current.state),
      gameLoggedIn: current.gameLoggedIn,
      schedule: normalizeFeatureSchedules(isRecord(sections?.scheduler) ? (sections?.scheduler as Record<string, unknown>).featureSchedules : undefined)[featureId],
    });
    if (!startRequiresConfirmation(report)) return true;
    return new Promise<boolean>((resolve) => {
      setPendingStart({ featureId, report, decide: (proceed) => { setPendingStart(null); resolve(proceed); } });
    });
  }, [loadLatestConfiguration]);

  const setAutomationEnabled = async (feature: string, enabled: boolean) => {
	if (enabled && !(await confirmStart(feature))) {
		window.dispatchEvent(new CustomEvent('citadelops:fix-before-start', { detail: { enabledKey: feature } }));
		return;
	}
	try {
		await updateConfiguration('automation.enabled', { ...automationEnabled, [feature]: enabled });
		forgetFailure(feature);
	} catch (error) {
		rememberFailure(feature, enabled ? 'start' : 'stop', error);
		throw error;
	}
  };

  const enableAutomationFor = async (feature: string, durationMinutes: number) => {
	if (!(await confirmStart(feature))) {
		window.dispatchEvent(new CustomEvent('citadelops:fix-before-start', { detail: { enabledKey: feature } }));
		return;
	}
	try {
		await updateConfiguration('automation.enabled', {
			...automationEnabled,
			[feature]: timedAutomationEnabledValue(durationMinutes),
		});
		forgetFailure(feature);
	} catch (error) {
		rememberFailure(feature, 'start', error);
		throw error;
	}
  };

  const toggle = (feature: string, enabled: boolean) => {
	// A failed write is kept in `automationWriteFailures` (and reported by the configuration client).
	void setAutomationEnabled(feature, !enabled).catch(() => undefined);
  };

  // Turn-on times per account: a configuration change that flips a switch on records when; off clears it.
  const accountId = accountKey(state ?? null);
  const [enabledSince, setEnabledSince] = useState<Record<string, string>>({});
  const previousEnabled = useRef<{ account: string; enabled: Record<string, boolean> } | null>(null);
  const configurationUpdatedAt = configuration?.updatedAt;
  useEffect(() => {
	if (!accountId || !configuration) return;
	const storage = typeof window === 'undefined' ? undefined : (() => { try { return window.localStorage; } catch { return undefined; } })();
	const previous = previousEnabled.current;
	if (!previous || previous.account !== accountId) {
		previousEnabled.current = { account: accountId, enabled: { ...automationEnabledByKey } };
		const stored = readEnabledSince(storage, accountId);
		// A stored time for a switch that is off is stale: drop it.
		let next = stored;
		for (const key of Object.keys(stored)) if (automationEnabledByKey[key] !== true) next = clearEnabledSince(storage, accountId, key);
		setEnabledSince(next);
		return;
	}
	let next: Record<string, string> | undefined;
	for (const key of new Set([...Object.keys(previous.enabled), ...Object.keys(automationEnabledByKey)])) {
		const was = previous.enabled[key] === true;
		const is = automationEnabledByKey[key] === true;
		if (!was && is) next = recordEnabledSince(storage, accountId, key, configurationUpdatedAt || new Date().toISOString());
		if (was && !is) next = clearEnabledSince(storage, accountId, key);
	}
	previous.enabled = { ...automationEnabledByKey };
	if (next) setEnabledSince(next);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- only switch changes and account changes matter
  }, [accountId, automationEnabledByKey]);

  const value = useMemo<AuthContextType>(() => ({
    gameLoggedIn,
    gameLoginCooldown: cooldownSeconds,
    gameLoginRetrySeconds: retrySeconds,
    gameConnectionState,
    gameSocketConnected: session?.socketReady === true,
    gameBrowserRunning: session != null && session.status !== 'stopped' && session.status !== 'unavailable',
		gameBrowserName: session?.browserName ?? 'game browser',
    gameConnectionDetail: session?.detail ?? '',
    dashboardConnectionStatus: connectionStatus,
    hasGameConnectionStatus: session != null,
    isGameDataReady: catalogs != null,
    recruitTroopsEnabled,
    autoRecruitMode: 'global' as RecruitTroopsMode,
    autoToolEnabled,
    autoToolMode: 'global' as AutoToolMode,
    autoSceatResEnabled,
    autoFoodBalanceEnabled,
    autoHospitalEnabled,
    autoTCIEnabled,
		autoTCINextWakeUp: automationWakeMillis(automationStates.autoTCI),
		autoTowerEnabled,
		autoFortressEnabled,
		autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
		autoBoosterEnabled,
		autoBuyerEnabled,
		autoKhanEnabled,
		autoBeriWorldEnabled,
		autoStormEnabled,
		autoBirdEnabled,
		autoBirdNextWakeUp: automationMetricMillis(autoBirdState, 'nextBirdUnixMs'),
		autoBirdNextCastleName,
		autoBirdCastleCycles,
    autoStationEnabled,
    autoStationState: autoStationEnabled ? (autoStationState?.status ?? 'waiting') : 'off',
    autoStationThreatCount: Math.max(0, Math.trunc(autoStationState?.metrics?.threatCount ?? 0)),
    autoStationNextImpact: Math.max(0, autoStationState?.metrics?.nextImpactUnixMs ?? 0),
    autoStationDetail: autoStationState?.detail ?? autoStationState?.lastError ?? '',
    goMem: Math.max(0, Math.trunc(diagnostics?.applicationMemoryMb ?? 0)),
	browserMem: Math.max(0, Math.trunc(diagnostics?.browserMemoryMb ?? 0)),
	botLocked,
	automationStates,
	automationEnabledByKey,
	automationTimedUntilByKey,
	automationWriteFailures: writeFailures,
	automationEnabledSince: enabledSince,
    startGame: () => submit('session.start'),
    stopGame: () => submit('session.stop'),
    reconnectGame: () => submit('session.reconnect'),
	toggleRecruitTroops: () => toggle('recruit_troops', recruitTroopsEnabled),
	toggleAutoTool: () => toggle('auto_tool', autoToolEnabled),
	toggleAutoSceatRes: () => toggle('auto_sceat_resources', autoSceatResEnabled),
	toggleAutoFoodBalance: () => toggle('auto_food_balance', autoFoodBalanceEnabled),
	toggleAutoHospital: () => toggle('auto_hospital', autoHospitalEnabled),
	toggleAutoTCI: () => toggle('auto_tci', autoTCIEnabled),
		toggleAutoTower: () => toggle('auto_towers', autoTowerEnabled),
		toggleAutoFortress: () => toggle('auto_fortress', autoFortressEnabled),
		toggleAutoInvasion: () => toggle('auto_invasion', autoInvasionEnabled),
		toggleAutoNomad: () => toggle('auto_nomad', autoNomadEnabled),
		toggleAutoAdvisor: () => toggle('auto_advisor', autoAdvisorEnabled),
		toggleAutoBooster: () => toggle('auto_booster', autoBoosterEnabled),
		toggleAutoBuyer: () => toggle('auto_buyer', autoBuyerEnabled),
		toggleAutoKhan: () => toggle('auto_khan', autoKhanEnabled),
		toggleAutoBeriWorld: () => toggle('auto_beri_world', autoBeriWorldEnabled),
		toggleAutoStorm: () => toggle('auto_storm', autoStormEnabled),
		toggleAutoBird: () => toggle('auto_bird', autoBirdEnabled),
	toggleAutoStation: () => toggle('auto_station', autoStationEnabled),
	toggleBotLock: () => {
		void updateConfiguration('scheduler', { ...schedulerConfiguration, botLocked: !botLocked });
	},
	setAutomationEnabled,
	enableAutomationFor,
  }), [
    autoBirdEnabled,
	autoBirdState,
	autoBirdNextCastleName,
	autoBirdCastleCycles,
	botLocked,
    autoHospitalEnabled,
    autoSceatResEnabled,
	autoFoodBalanceEnabled,
    autoStationEnabled,
	autoStationState,
		autoTCIEnabled,
		autoTowerEnabled,
		autoFortressEnabled,
		autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
		autoBoosterEnabled,
		autoBuyerEnabled,
		autoKhanEnabled,
		autoBeriWorldEnabled,
		autoStormEnabled,
    autoToolEnabled,
	automationStates,
	automationEnabledByKey,
	automationTimedUntilByKey,
	writeFailures,
	enabledSince,
    catalogs,
	configuration,
    connectionStatus,
	diagnostics,
	cooldownSeconds,
    gameConnectionState,
    gameLoggedIn,
    recruitTroopsEnabled,
	retrySeconds,
    session,
    submitIntent,
	updateConfiguration,
  ]);

  return (
    <AuthContext.Provider value={value}>
      {children}
      <StartConfirmDialog pending={pendingStart} />
    </AuthContext.Provider>
  );
}

function automationWakeMillis(state: AutomationStateV2 | undefined): number {
	if (!state?.nextCheckAt) return 0;
	const value = Date.parse(state.nextCheckAt);
	return Number.isFinite(value) ? value : 0;
}

function automationMetricMillis(state: AutomationStateV2 | undefined, key: string): number {
	const value = state?.metrics?.[key];
	return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0;
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth must be used within AuthProvider');
  return context;
}

function normalizeConnectionState(status: string | undefined, loggedIn: boolean): GameConnectionState {
  if (loggedIn) return 'connected';
  switch (status) {
    case 'stopped':
    case 'starting':
    case 'connecting':
    case 'authenticating':
    case 'cooldown':
    case 'reconnecting':
    case 'suspended':
    case 'released':
    case 'disconnected':
      return status;
    case 'connected':
      return 'connected';
    case 'unavailable':
    case 'error':
      return 'error';
    default:
      return 'disconnected';
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function secondsUntil(value: string | undefined, now: number): number {
	if (!value) return 0;
	const target = Date.parse(value);
	return Number.isFinite(target) ? Math.max(0, Math.ceil((target - now) / 1000)) : 0;
}
