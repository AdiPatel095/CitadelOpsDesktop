import { createContext, useContext, useState, type ReactNode, useEffect } from 'react';
import { FrontendWebsocket, type FrontendWebsocketStatus } from '../Websocket';
import { loadPresetsFile } from '../settings/AutoBirdPresets';
import {
  applyAutoBirdClientStateToLocalStorage,
  buildAutoBirdClientState,
  loadAutoBirdSettingsFromStorage,
  parseAutoBirdClientState,
  persistAutoBirdClientState,
} from '../settings/AutoBirdClientState';
import { loadPresetsFile as loadAutoTCIPresetsFile } from '../settings/AutoTCIPresets';
import {
  applyAutoTCIClientStateToLocalStorage,
  applyAutoTCISettingsToLocalStorage,
  buildAutoTCIClientState,
  loadAutoTCISettingsFromStorage,
  parseAutoTCIClientState,
  persistAutoTCIClientState,
} from '../settings/AutoTCIClientState';
import {
  applyAutoBeriWorldSettingsToLocalStorage,
  loadAutoBeriWorldSettingsFromStorage,
} from '../settings/AutoBeriWorldClientState';
import {
  applyRecruitTroopsSettingsToLocalStorage,
  DEFAULT_RECRUIT_CHECK_INTERVAL_SEC,
  loadRecruitTroopsSettingsFromStorage,
  normalizeRecruitTroopsSettings,
  persistRecruitTroopsSettings,
  RECRUIT_TROOPS_SETTINGS_CHANGED_EVENT,
  type RecruitTroopsClientSettingsV1,
  type RecruitTroopsMode,
} from '../settings/RecruitTroopsClientState';
import {
  applyAutoToolSettingsToLocalStorage,
  AUTO_TOOL_SETTINGS_CHANGED_EVENT,
  DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC,
  loadAutoToolSettingsFromStorage,
  normalizeAutoToolSettings,
  persistAutoToolSettings,
  type AutoToolClientSettingsV1,
  type AutoToolMode,
} from '../settings/AutoToolClientState';
import {
  applyAutoHospitalSettingsToLocalStorage,
  DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC,
  loadAutoHospitalSettingsFromStorage,
  normalizeAutoHospitalSettings,
  persistAutoHospitalSettings,
  type AutoHospitalClientSettingsV1,
} from '../settings/AutoHospitalClientState';
import {
  applyAutoSceatResSettingsToLocalStorage,
  loadAutoSceatResSettingsFromStorage,
  normalizeAutoSceatResSettings,
} from '../settings/AutoSceatResClientState';
import {
  applyAutoStationClientStateToLocalStorage,
  loadAutoStationClientState,
  parseAutoStationClientState,
  persistAutoStationClientState,
} from '../settings/AutoStationClientState';

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

interface GameConnectionSnapshot {
  state: GameConnectionState;
  loggedIn: boolean;
  browserRunning: boolean;
  socketConnected: boolean;
  cooldownUntil: number;
  retryAt: number;
  revision: number;
  changedAt: number;
  detail: string;
}

const GAME_CONNECTION_STATES = new Set<GameConnectionState>([
  'stopped',
  'starting',
  'connecting',
  'authenticating',
  'connected',
  'cooldown',
  'reconnecting',
  'disconnected',
  'error',
]);

const INITIAL_GAME_CONNECTION: GameConnectionSnapshot = {
  state: 'disconnected',
  loggedIn: false,
  browserRunning: false,
  socketConnected: false,
  cooldownUntil: 0,
  retryAt: 0,
  revision: 0,
  changedAt: 0,
  detail: '',
};

function finiteNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function parseGameConnectionSnapshot(payload: any): GameConnectionSnapshot {
  const loggedIn = payload?.loggedIn === true;
  const legacyCooldown = Math.max(0, Math.floor(finiteNumber(payload?.cooldown)));
  const state = GAME_CONNECTION_STATES.has(payload?.state)
    ? payload.state as GameConnectionState
    : loggedIn
      ? 'connected'
      : legacyCooldown > 0
        ? 'cooldown'
        : 'disconnected';
  const cooldownUntil = finiteNumber(payload?.cooldownUntil) ||
    (legacyCooldown > 0 ? Date.now() + legacyCooldown * 1000 : 0);

  return {
    state,
    loggedIn,
    browserRunning: typeof payload?.browserRunning === 'boolean' ? payload.browserRunning : loggedIn,
    socketConnected: typeof payload?.socketConnected === 'boolean' ? payload.socketConnected : loggedIn,
    cooldownUntil,
    retryAt: finiteNumber(payload?.retryAt) || cooldownUntil,
    revision: Math.max(0, Math.floor(finiteNumber(payload?.revision))),
    changedAt: finiteNumber(payload?.changedAt),
    detail: typeof payload?.detail === 'string' ? payload.detail : '',
  };
}

interface AuthContextType {
  gameLoggedIn: boolean;
  gameLoginCooldown: number;
  gameLoginRetrySeconds: number;
  gameConnectionState: GameConnectionState;
  gameSocketConnected: boolean;
  gameBrowserRunning: boolean;
  gameConnectionDetail: string;
  dashboardConnectionStatus: FrontendWebsocketStatus;
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
  // Memory stats
  goMem: number;
  chromeMem: number;
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
  sendMessage: (type: string, payload?: any) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

function recruitSettingsHasData(settings: RecruitTroopsClientSettingsV1): boolean {
  return (
    settings.mode !== 'global' ||
    settings.checkIntervalSec !== DEFAULT_RECRUIT_CHECK_INTERVAL_SEC ||
    settings.globalItems.length > 0 ||
    Object.keys(settings.castles).length > 0
  );
}

function autoToolSettingsHasData(settings: AutoToolClientSettingsV1): boolean {
  return (
    settings.mode !== 'global' ||
    settings.checkIntervalSec !== DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC ||
    settings.globalItems.length > 0 ||
    Object.keys(settings.castles).length > 0
  );
}

function autoHospitalSettingsHasData(settings: AutoHospitalClientSettingsV1): boolean {
  return settings.checkIntervalSec !== DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC;
}

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [gameConnection, setGameConnection] = useState<GameConnectionSnapshot>(INITIAL_GAME_CONNECTION);
  const [dashboardConnectionStatus, setDashboardConnectionStatus] = useState<FrontendWebsocketStatus>(
    FrontendWebsocket.getStatus(),
  );
  const [hasGameConnectionStatus, setHasGameConnectionStatus] = useState(false);
  const [gameConnectionClock, setGameConnectionClock] = useState(() => Date.now());
  const [recruitTroopsEnabled, setRecruitTroopsEnabled] = useState(false);
  const [autoRecruitMode, setAutoRecruitMode] = useState<RecruitTroopsMode>(() => loadRecruitTroopsSettingsFromStorage().mode);
  const [autoToolEnabled, setAutoToolEnabled] = useState(false);
  const [autoToolMode, setAutoToolMode] = useState<AutoToolMode>(() => loadAutoToolSettingsFromStorage().mode);
  const [autoSceatResEnabled, setAutoSceatResEnabled] = useState(false);
  const [autoHospitalEnabled, setAutoHospitalEnabled] = useState(false);
  const [autoTCIEnabled, setAutoTCIEnabled] = useState(false);
  const [autoTCINextWakeUp, setAutoTCINextWakeUp] = useState(0);
  const [autoBirdEnabled, setAutoBirdEnabled] = useState(false);
  const [autoBirdNextWakeUp, setAutoBirdNextWakeUp] = useState(0);
  const [autoStationEnabled, setAutoStationEnabled] = useState(false);
  const [autoStationState, setAutoStationState] = useState('off');
  const [autoStationThreatCount, setAutoStationThreatCount] = useState(0);
  const [autoStationNextImpact, setAutoStationNextImpact] = useState(0);
  const [autoStationDetail, setAutoStationDetail] = useState('');
  const [autoBeriWorldEnabled, setAutoBeriWorldEnabled] = useState(false);
  const [autoBeriWorldNextWakeUp, setAutoBeriWorldNextWakeUp] = useState(0);
  const [versionUpdate, setVersionUpdate] = useState<{ newVersion: string; downloadUrl: string } | null>(null);
  const [isVersionBannerDismissed, setIsVersionBannerDismissed] = useState(false);
  const [ignoredVersion, setIgnoredVersion] = useState<string | null>(() => {
    return localStorage.getItem('ignoredVersion');
  });
  const [updateProgress, setUpdateProgress] = useState<{ stage: string; percent: number } | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [restartRequired, setRestartRequired] = useState(false);
  const [goMem, setGoMem] = useState(0);
  const [chromeMem, setChromeMem] = useState(0);

  const gameLoggedIn =
    dashboardConnectionStatus === 'Connected' &&
    hasGameConnectionStatus &&
    gameConnection.state === 'connected' &&
    gameConnection.loggedIn &&
    gameConnection.socketConnected;
  const gameLoginCooldown = gameConnection.cooldownUntil > gameConnectionClock
    ? Math.ceil((gameConnection.cooldownUntil - gameConnectionClock) / 1000)
    : 0;
  const gameLoginRetrySeconds = gameConnection.retryAt > gameConnectionClock
    ? Math.ceil((gameConnection.retryAt - gameConnectionClock) / 1000)
    : 0;
  const isGameDataReady = gameLoggedIn;

  useEffect(() => {
    let previousDashboardStatus = FrontendWebsocket.getStatus();
    const handleMessage = (message: any) => {
      // console.log('AuthContext received message:', message);
      if (message.type === 'gameLoginStatus') {
        console.log('Game login status received:', message.payload);
        const next = parseGameConnectionSnapshot(message.payload);
        setGameConnectionClock(Date.now());
        setGameConnection((current) => {
          if (next.revision > 0 && current.revision > next.revision) {
            return current;
          }
          return next;
        });
        setHasGameConnectionStatus(true);
      } else if (message.type === 'memoryStats') {
        setGoMem(message.payload.goMem);
        setChromeMem(message.payload.chromeMem);
      } else if (message.type === 'recruitTroopsStatus') {
        setRecruitTroopsEnabled(message.payload.enabled);
      } else if (message.type === 'recruitTroopsSettings') {
        let settings = normalizeRecruitTroopsSettings(message.payload);
        const localSettings = loadRecruitTroopsSettingsFromStorage();
        if (!recruitSettingsHasData(settings) && recruitSettingsHasData(localSettings)) {
          settings = localSettings;
          persistRecruitTroopsSettings(settings);
        }
        applyRecruitTroopsSettingsToLocalStorage(settings);
        setAutoRecruitMode(settings.mode);
      } else if (message.type === 'autoToolStatus') {
        setAutoToolEnabled(message.payload.enabled);
      } else if (message.type === 'autoSceatResStatus') {
        setAutoSceatResEnabled(message.payload?.enabled === true);
      } else if (message.type === 'autoSceatResSettings') {
        applyAutoSceatResSettingsToLocalStorage(normalizeAutoSceatResSettings(message.payload));
      } else if (message.type === 'autoHospitalStatus') {
        setAutoHospitalEnabled(message.payload.enabled);
      } else if (message.type === 'autoHospitalSettings') {
        let settings = normalizeAutoHospitalSettings(message.payload);
        const localSettings = loadAutoHospitalSettingsFromStorage();
        if (!autoHospitalSettingsHasData(settings) && autoHospitalSettingsHasData(localSettings)) {
          settings = localSettings;
          persistAutoHospitalSettings(settings);
        }
        applyAutoHospitalSettingsToLocalStorage(settings);
      } else if (message.type === 'autoToolSettings') {
        let settings = normalizeAutoToolSettings(message.payload);
        const localSettings = loadAutoToolSettingsFromStorage();
        if (!autoToolSettingsHasData(settings) && autoToolSettingsHasData(localSettings)) {
          settings = localSettings;
          persistAutoToolSettings(settings);
        }
        applyAutoToolSettingsToLocalStorage(settings);
        setAutoToolMode(settings.mode);
      } else if (message.type === 'autoTCIStatus') {
        setAutoTCIEnabled(!!message.payload?.enabled);
        const nw = message.payload?.nextWakeUp;
        setAutoTCINextWakeUp(typeof nw === 'number' ? nw : 0);
      } else if (message.type === 'autoBirdStatus') {
        setAutoBirdEnabled(!!message.payload?.enabled);
        const nw = message.payload?.nextWakeUp;
        setAutoBirdNextWakeUp(typeof nw === 'number' ? nw : 0);
      } else if (message.type === 'autoStationStatus') {
        setAutoStationEnabled(message.payload?.enabled === true);
        setAutoStationState(typeof message.payload?.state === 'string' ? message.payload.state : 'off');
        setAutoStationThreatCount(Math.max(0, Math.floor(finiteNumber(message.payload?.threatCount))));
        setAutoStationNextImpact(Math.max(0, finiteNumber(message.payload?.nextImpactUnixMs)));
        setAutoStationDetail(typeof message.payload?.detail === 'string' ? message.payload.detail : '');
      } else if (message.type === 'autoBeriWorldStatus') {
        setAutoBeriWorldEnabled(!!message.payload?.enabled);
        const nw = message.payload?.nextWakeUp;
        setAutoBeriWorldNextWakeUp(typeof nw === 'number' ? nw : 0);
      } else if (message.type === 'autoBeriWorldSettings') {
        if (message.payload && typeof message.payload === 'object' && !Array.isArray(message.payload)) {
          applyAutoBeriWorldSettingsToLocalStorage(message.payload);
        }
      } else if (message.type === 'autoBirdClientState') {
        let state = parseAutoBirdClientState(message.payload);
        if (state.presets.presets.length === 0) {
          const localPresets = loadPresetsFile();
          const localIgnore = loadAutoBirdSettingsFromStorage();
          const hadLocalData =
            localPresets.presets.length > 0 ||
            Object.keys(localIgnore.settings).length > 0 ||
            localIgnore.minSend > 0 ||
            localIgnore.minDelay !== 6 ||
            localIgnore.maxDelay !== 12;
          if (hadLocalData) {
            state = buildAutoBirdClientState(localIgnore, localPresets);
            persistAutoBirdClientState(state);
          }
        }
        applyAutoBirdClientStateToLocalStorage(state);
      } else if (message.type === 'autoStationClientState') {
        const state = parseAutoStationClientState(message.payload);
        applyAutoStationClientStateToLocalStorage(state);
      } else if (message.type === 'autoTCIClientState') {
        let state = parseAutoTCIClientState(message.payload);
        if (state.presets.presets.length === 0) {
          const localPresets = loadAutoTCIPresetsFile();
          const localTargets = loadAutoTCISettingsFromStorage();
          const hadLocalData =
            localPresets.presets.length > 0 || Object.keys(localTargets).length > 0;
          if (hadLocalData) {
            state = buildAutoTCIClientState(localTargets, localPresets);
            persistAutoTCIClientState(state);
          }
        }
        applyAutoTCIClientStateToLocalStorage(state);
      } else if (message.type === 'autoTCISettings') {
        if (message.payload && typeof message.payload === 'object' && !Array.isArray(message.payload)) {
          applyAutoTCISettingsToLocalStorage(message.payload as Record<string, Record<string, number>>);
        }
      } else if (message.type === 'versionUpdate') {
        console.log('Version update received:', message.payload);
        const currentIgnoredVersion = localStorage.getItem('ignoredVersion');
        setVersionUpdate({
          newVersion: message.payload.newVersion,
          downloadUrl: message.payload.downloadUrl
        });
        // Only show popup if this version is not ignored
        if (message.payload.newVersion !== currentIgnoredVersion) {
          setIsVersionBannerDismissed(false);
        } else {
          console.log('Version update ignored by user:', message.payload.newVersion);
          setIsVersionBannerDismissed(true);
        }
      } else if (message.type === 'updateProgress') {
        console.log('Update progress:', message.payload);
        setUpdateProgress({
          stage: message.payload.stage,
          percent: message.payload.percent
        });
      } else if (message.type === 'updateComplete') {
        console.log('Update complete - restart required');
        setIsUpdating(false);
        setRestartRequired(true);
      } else if (message.type === 'updateError') {
        console.log('Update error:', message.payload);
        setUpdateProgress(null);
        setIsUpdating(false);
      }
    };

    const handleDashboardStatus = (status: FrontendWebsocketStatus) => {
      const changed = status !== previousDashboardStatus;
      previousDashboardStatus = status;
      setDashboardConnectionStatus(status);
      if (status !== 'Connected' || changed) {
        setHasGameConnectionStatus(false);
      }
      if (status !== 'Connected') {
        setGameConnection((current) => ({ ...current, revision: 0 }));
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.addStatusListener(handleDashboardStatus);
    // Connect to WebSocket using the current page's host (supports dynamic port)
    const wsUrl = `ws://${window.location.host}/ws`;
    FrontendWebsocket.connect(wsUrl);

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
      FrontendWebsocket.removeStatusListener(handleDashboardStatus);
    };
  }, []);

  useEffect(() => {
    const handleRecruitSettingsChanged = (event: Event) => {
      const customEvent = event as CustomEvent<RecruitTroopsClientSettingsV1>;
      setAutoRecruitMode(normalizeRecruitTroopsSettings(customEvent.detail).mode);
    };

    window.addEventListener(RECRUIT_TROOPS_SETTINGS_CHANGED_EVENT, handleRecruitSettingsChanged);
    return () => window.removeEventListener(RECRUIT_TROOPS_SETTINGS_CHANGED_EVENT, handleRecruitSettingsChanged);
  }, []);

  useEffect(() => {
    const handleAutoToolSettingsChanged = (event: Event) => {
      const customEvent = event as CustomEvent<AutoToolClientSettingsV1>;
      setAutoToolMode(normalizeAutoToolSettings(customEvent.detail).mode);
    };

    window.addEventListener(AUTO_TOOL_SETTINGS_CHANGED_EVENT, handleAutoToolSettingsChanged);
    return () => window.removeEventListener(AUTO_TOOL_SETTINGS_CHANGED_EVENT, handleAutoToolSettingsChanged);
  }, []);

  useEffect(() => {
    const deadline = Math.max(gameConnection.cooldownUntil, gameConnection.retryAt);
    setGameConnectionClock(Date.now());
    if (deadline <= Date.now()) return;

    const interval = window.setInterval(() => {
      const now = Date.now();
      setGameConnectionClock(now);
      if (now >= deadline) {
        window.clearInterval(interval);
      }
    }, 1000);
    return () => window.clearInterval(interval);
  }, [gameConnection.cooldownUntil, gameConnection.retryAt]);

  const startGame = () => {
    FrontendWebsocket.startGame();
  };

  const stopGame = () => {
    FrontendWebsocket.stopGame();
  };

  const toggleAutoTCI = () => {
    if (autoTCIEnabled) {
      setAutoTCIEnabled(false);
      setAutoTCINextWakeUp(0);
      FrontendWebsocket.sendMessage({ type: 'toggleAutoTCI' });
      return;
    }
    setAutoTCIEnabled(true);
    setAutoTCINextWakeUp(0);
    const settings = loadAutoTCISettingsFromStorage();
    FrontendWebsocket.sendMessage({ type: 'toggleAutoTCI', payload: { settings } });
  };

  const toggleAutoBird = () => {
    if (autoBirdEnabled) {
      FrontendWebsocket.sendMessage({ type: 'toggleAutoBird' });
      return;
    }
    const s = loadAutoBirdSettingsFromStorage();
    FrontendWebsocket.sendMessage({
      type: 'toggleAutoBird',
      payload: {
        settings: s.settings,
        minDelay: s.minDelay,
        maxDelay: s.maxDelay,
        minSend: s.minSend,
        minRPTDays: s.minRPTDays,
      },
    });
  };

  const toggleAutoStation = () => {
    if (!autoStationEnabled) {
      persistAutoStationClientState(loadAutoStationClientState());
    }
    FrontendWebsocket.sendMessage({ type: 'toggleAutoStation' });
  };

  const toggleAutoBeriWorld = () => {
    if (autoBeriWorldEnabled) {
      FrontendWebsocket.sendMessage({ type: 'toggleAutoBeriWorld' });
      return;
    }
    const s = loadAutoBeriWorldSettingsFromStorage();
    FrontendWebsocket.sendMessage({ type: 'toggleAutoBeriWorld', payload: s });
  };

  const toggleRecruitTroops = () => {
    const settings = loadRecruitTroopsSettingsFromStorage();

    console.log("[RecruitTroops] Toggling. Sending settings payload:", { settings });

    FrontendWebsocket.sendMessage({
      type: 'toggleRecruitTroops',
      payload: { settings }
    });
  };

  const toggleAutoTool = () => {
    const settings = loadAutoToolSettingsFromStorage();

    console.log("[AutoTool] Toggling. Sending settings payload:", { settings });

    FrontendWebsocket.sendMessage({
      type: 'toggleAutoTool',
      payload: { settings }
    });
  };

  const toggleAutoSceatRes = () => {
    const settings = loadAutoSceatResSettingsFromStorage();
    FrontendWebsocket.sendMessage({
      type: 'toggleAutoSceatRes',
      payload: { settings },
    });
  };

  const toggleAutoHospital = () => {
    const settings = loadAutoHospitalSettingsFromStorage();

    FrontendWebsocket.sendMessage({
      type: 'toggleAutoHospital',
      payload: { settings }
    });
  };

  const dismissVersionBanner = () => {
    setIsVersionBannerDismissed(true);
  };

  const ignoreVersion = (version: string) => {
    localStorage.setItem('ignoredVersion', version);
    setIgnoredVersion(version);
    setIsVersionBannerDismissed(true);
    console.log('User ignored version:', version);
  };

  const triggerUpdate = (downloadUrl: string) => {
    setIsUpdating(true);
    setUpdateProgress({ stage: 'Starting update...', percent: 0 });
    FrontendWebsocket.triggerUpdate(downloadUrl);
  };

  const sendMessage = (type: string, payload?: any) => {
    FrontendWebsocket.sendMessage({ type, payload });
  };

  return (
    <AuthContext.Provider value={{
      gameLoggedIn,
      gameLoginCooldown,
      gameLoginRetrySeconds,
      gameConnectionState: gameConnection.state,
      gameSocketConnected: gameConnection.socketConnected,
      gameBrowserRunning: gameConnection.browserRunning,
      gameConnectionDetail: gameConnection.detail,
      dashboardConnectionStatus,
      hasGameConnectionStatus,
      isGameDataReady,
      recruitTroopsEnabled,
      autoRecruitMode,
      autoToolEnabled,
      autoToolMode,
      autoSceatResEnabled,
      autoHospitalEnabled,
      autoTCIEnabled,
      autoTCINextWakeUp,
      autoBirdEnabled,
      autoBirdNextWakeUp,
      autoStationEnabled,
      autoStationState,
      autoStationThreatCount,
      autoStationNextImpact,
      autoStationDetail,
      autoBeriWorldEnabled,
      autoBeriWorldNextWakeUp,
      versionUpdate,
      isVersionBannerDismissed,
      ignoredVersion,
      updateProgress,
      isUpdating,
      restartRequired,
      goMem,
      chromeMem,
      dismissVersionBanner,
      ignoreVersion,
      triggerUpdate,
      startGame,
      stopGame,
      toggleRecruitTroops,
      toggleAutoTool,
      toggleAutoSceatRes,
      toggleAutoHospital,
      toggleAutoTCI,
      toggleAutoBird,
      toggleAutoStation,
      toggleAutoBeriWorld,
      sendMessage
    }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
