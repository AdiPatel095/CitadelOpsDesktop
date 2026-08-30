import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
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

export interface AutoBirdCastleCycle {
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
	autoInvasionEnabled: boolean;
	autoNomadEnabled: boolean;
	autoAdvisorEnabled: boolean;
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
	toggleAutoInvasion: () => void;
	toggleAutoNomad: () => void;
	toggleAutoAdvisor: () => void;
	toggleAutoBuyer: () => void;
	toggleAutoKhan: () => void;
	toggleAutoBeriWorld: () => void;
	toggleAutoStorm: () => void;
	toggleAutoBird: () => void;
	toggleAutoStation: () => void;
	toggleBotLock: () => void;
	setAutomationEnabled: (feature: string, enabled: boolean) => Promise<void>;
	enableAutomationFor: (feature: string, durationMinutes: number) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { connectionStatus, state, catalogs, configuration, diagnostics, submitIntent, updateConfiguration } = useCitadelAPI();
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
	const autoInvasionEnabled = automationEnabledByKey.auto_invasion === true;
	const autoNomadEnabled = automationEnabledByKey.auto_nomad === true;
	const autoAdvisorEnabled = automationEnabledByKey.auto_advisor === true;
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
				const metricReturn = automationMetricMillis(autoBirdState, `birdReturnUnixMs.${castle.id}`);
				const recordedReturn = Date.parse(operation?.expectedReturnAt ?? '');
				return {
					castleId: castle.id,
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

  const setAutomationEnabled = async (feature: string, enabled: boolean) => {
	await updateConfiguration('automation.enabled', { ...automationEnabled, [feature]: enabled });
  };

  const enableAutomationFor = async (feature: string, durationMinutes: number) => {
	await updateConfiguration('automation.enabled', {
		...automationEnabled,
		[feature]: timedAutomationEnabledValue(durationMinutes),
	});
  };

  const toggle = (feature: string, enabled: boolean) => {
	void setAutomationEnabled(feature, !enabled);
  };

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
		autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
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
		toggleAutoInvasion: () => toggle('auto_invasion', autoInvasionEnabled),
		toggleAutoNomad: () => toggle('auto_nomad', autoNomadEnabled),
		toggleAutoAdvisor: () => toggle('auto_advisor', autoAdvisorEnabled),
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
		autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
		autoBuyerEnabled,
		autoKhanEnabled,
		autoBeriWorldEnabled,
		autoStormEnabled,
    autoToolEnabled,
	automationStates,
	automationEnabledByKey,
	automationTimedUntilByKey,
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

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
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
