import { createContext, useContext, useState, type ReactNode, useEffect } from 'react';
import { FrontendWebsocket } from '../websocket';
import { loadAutoBirdSettingsFromStorage } from '../settings/components/AutoBirdSettingsModal';

interface AuthContextType {
  gameLoggedIn: boolean;
  gameLoginCooldown: number;
  isGameDataReady: boolean;
  recruitTroopsEnabled: boolean;
  autoBirdEnabled: boolean;
  autoBirdNextWakeUp: number;
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
  toggleAutoBird: () => void;
  sendMessage: (type: string, payload?: any) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [gameLoggedIn, setGameLoggedIn] = useState(false);
  const [gameLoginCooldown, setGameLoginCooldown] = useState(0);
  const [isGameDataReady, setIsGameDataReady] = useState(false);
  const [recruitTroopsEnabled, setRecruitTroopsEnabled] = useState(false);
  const [autoBirdEnabled, setAutoBirdEnabled] = useState(false);
  const [autoBirdNextWakeUp, setAutoBirdNextWakeUp] = useState(0);
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

  useEffect(() => {
    const handleMessage = (message: any) => {
      // console.log('AuthContext received message:', message);
      if (message.type === 'gameLoginStatus') {
        console.log('Game login status received:', message.payload);
        setGameLoggedIn(message.payload.loggedIn);
        setGameLoginCooldown(message.payload.cooldown);
        if (message.payload.loggedIn) {
          setIsGameDataReady(true);
        } else {
          setIsGameDataReady(false);
        }
      } else if (message.type === 'memoryStats') {
        setGoMem(message.payload.goMem);
        setChromeMem(message.payload.chromeMem);
      } else if (message.type === 'recruitTroopsStatus') {
        setRecruitTroopsEnabled(message.payload.enabled);
      } else if (message.type === 'autoBirdStatus') {
        setAutoBirdEnabled(!!message.payload?.enabled);
        const nw = message.payload?.nextWakeUp;
        setAutoBirdNextWakeUp(typeof nw === 'number' ? nw : 0);
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

    FrontendWebsocket.addMessageListener(handleMessage);
    // Connect to WebSocket using the current page's host (supports dynamic port)
    const wsUrl = `ws://${window.location.host}/ws`;
    FrontendWebsocket.connect(wsUrl);

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
    };
  }, []);

  useEffect(() => {
    let interval: ReturnType<typeof setInterval>;

    if (gameLoginCooldown > 0) {
      interval = setInterval(() => {
        setGameLoginCooldown((prev) => (prev > 0 ? prev - 1 : 0));
      }, 1000);
    }

    return () => {
      if (interval) clearInterval(interval);
    };
  }, [gameLoginCooldown]);

  const startGame = () => {
    FrontendWebsocket.startGame();
  };

  const stopGame = () => {
    FrontendWebsocket.stopGame();
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
      },
    });
  };

  const toggleRecruitTroops = () => {
    const savedSettings = localStorage.getItem('recruitTroopsSettings');
    let settings = {};
    if (savedSettings) {
      try {
        settings = JSON.parse(savedSettings);
      } catch (e) {
        console.error("Failed to parse settings for recruit troops toggle", e);
      }
    }

    console.log("[RecruitTroops] Toggling. Sending settings payload:", { settings });

    FrontendWebsocket.sendMessage({
      type: 'toggleRecruitTroops',
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
      isGameDataReady,
      recruitTroopsEnabled,
      autoBirdEnabled,
      autoBirdNextWakeUp,
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
      toggleAutoBird,
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
