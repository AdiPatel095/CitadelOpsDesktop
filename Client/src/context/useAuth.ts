import { createContext, useContext } from 'react';
import type { RecruitTroopsMode } from '../settings/RecruitTroopsClientState';
import type { AutoToolMode } from '../settings/AutoToolClientState';
import type { AutomationStateV2, StationingOperationV2 } from '../api/Contracts';

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

export interface AuthContextType {
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

export const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth must be used within AuthProvider');
  return context;
}
