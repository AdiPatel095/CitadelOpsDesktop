import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import type { RecruitTroopsMode } from '../settings/RecruitTroopsClientState';
import type { AutoToolMode } from '../settings/AutoToolClientState';
import { Notifications } from '../components/Notifications';

export type GameConnectionState =
  | 'stopped'
  | 'starting'
  | 'connecting'
  | 'authenticating'
  | 'connected'
  | 'cooldown'
  | 'reconnecting'
  | 'disconnected'
  | 'error';

export type DashboardConnectionStatus = 'Disconnected' | 'Connecting' | 'Connected';

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
  autoHospitalEnabled: boolean;
  autoTCIEnabled: boolean;
  autoTCINextWakeUp: number;
  autoBirdEnabled: boolean;
  autoBirdNextWakeUp: number;
  autoStationEnabled: boolean;
  autoStationState: string;
  autoStationThreatCount: number;
  autoStationNextImpact: number;
  autoStationDetail: string;
  autoBeriWorldEnabled: boolean;
  autoBeriWorldNextWakeUp: number;
  versionUpdate: { newVersion: string; downloadUrl: string } | null;
  isVersionBannerDismissed: boolean;
  ignoredVersion: string | null;
  updateProgress: { stage: string; percent: number } | null;
  isUpdating: boolean;
  restartRequired: boolean;
  goMem: number;
	browserMem: number;
  dismissVersionBanner: () => void;
  ignoreVersion: (version: string) => void;
  triggerUpdate: (downloadUrl: string) => void;
  startGame: () => void;
  stopGame: () => void;
  toggleRecruitTroops: () => void;
  toggleAutoTool: () => void;
  toggleAutoSceatRes: () => void;
  toggleAutoHospital: () => void;
  toggleAutoTCI: () => void;
  toggleAutoBird: () => void;
  toggleAutoStation: () => void;
  toggleAutoBeriWorld: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { connectionStatus, state, catalogs, configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const [isVersionBannerDismissed, setIsVersionBannerDismissed] = useState(false);
  const [ignoredVersion, setIgnoredVersion] = useState<string | null>(() => localStorage.getItem('ignoredVersion'));
  const [updateProgress, setUpdateProgress] = useState<{ stage: string; percent: number } | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);

  const session = state?.session;
	const automationEnabled = isRecord(configuration?.sections['automation.enabled'])
		? configuration.sections['automation.enabled'] as Record<string, unknown>
		: {};
	const recruitTroopsEnabled = automationEnabled.recruit_troops === true;
	const autoToolEnabled = automationEnabled.auto_tool === true;
	const autoSceatResEnabled = automationEnabled.auto_sceat_resources === true;
	const autoHospitalEnabled = automationEnabled.auto_hospital === true;
	const autoTCIEnabled = automationEnabled.auto_tci === true;
	const autoBirdEnabled = automationEnabled.auto_bird === true;
	const autoStationEnabled = automationEnabled.auto_station === true;
	const autoBeriWorldEnabled = automationEnabled.auto_beri_world === true;
  const gameLoggedIn = connectionStatus === 'Connected' && session?.loggedIn === true && session.socketReady === true;
  const gameConnectionState = normalizeConnectionState(session?.status, gameLoggedIn);

  const submit = (name: string, argumentsValue: Record<string, unknown> = {}) => {
    void submitIntent(name, argumentsValue).catch((error) => {
      console.error(`Intent ${name} failed`, error);
    });
  };

  const toggle = (feature: string, enabled: boolean) => {
	void updateConfiguration('automation.enabled', { ...automationEnabled, [feature]: !enabled });
  };

  const triggerUpdate = (downloadUrl: string) => {
	void downloadUrl;
	setIsUpdating(false);
	setUpdateProgress(null);
	Notifications.warning('The 2.0 updater is not enabled; install releases through the normal release channel.');
  };

  const value = useMemo<AuthContextType>(() => ({
    gameLoggedIn,
    gameLoginCooldown: 0,
    gameLoginRetrySeconds: 0,
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
    autoHospitalEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp: 0,
    autoBirdEnabled,
    autoBirdNextWakeUp: 0,
    autoStationEnabled,
    autoStationState: autoStationEnabled ? 'active' : 'off',
    autoStationThreatCount: 0,
    autoStationNextImpact: 0,
    autoStationDetail: '',
    autoBeriWorldEnabled,
    autoBeriWorldNextWakeUp: 0,
    versionUpdate: null,
    isVersionBannerDismissed,
    ignoredVersion,
    updateProgress,
    isUpdating,
    restartRequired: false,
    goMem: 0,
		browserMem: 0,
    dismissVersionBanner: () => setIsVersionBannerDismissed(true),
    ignoreVersion: (version) => {
      localStorage.setItem('ignoredVersion', version);
      setIgnoredVersion(version);
      setIsVersionBannerDismissed(true);
    },
    triggerUpdate,
    startGame: () => submit('session.start'),
    stopGame: () => submit('session.stop'),
	toggleRecruitTroops: () => toggle('recruit_troops', recruitTroopsEnabled),
	toggleAutoTool: () => toggle('auto_tool', autoToolEnabled),
	toggleAutoSceatRes: () => toggle('auto_sceat_resources', autoSceatResEnabled),
	toggleAutoHospital: () => toggle('auto_hospital', autoHospitalEnabled),
	toggleAutoTCI: () => toggle('auto_tci', autoTCIEnabled),
	toggleAutoBird: () => toggle('auto_bird', autoBirdEnabled),
	toggleAutoStation: () => toggle('auto_station', autoStationEnabled),
	toggleAutoBeriWorld: () => toggle('auto_beri_world', autoBeriWorldEnabled),
  }), [
    autoBeriWorldEnabled,
    autoBirdEnabled,
    autoHospitalEnabled,
    autoSceatResEnabled,
    autoStationEnabled,
    autoTCIEnabled,
    autoToolEnabled,
    catalogs,
	configuration,
    connectionStatus,
    gameConnectionState,
    gameLoggedIn,
    ignoredVersion,
    isUpdating,
    isVersionBannerDismissed,
    recruitTroopsEnabled,
    session,
    submitIntent,
	updateConfiguration,
    updateProgress,
  ]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
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
