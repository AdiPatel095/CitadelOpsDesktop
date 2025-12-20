import { createContext, useContext, useState, type ReactNode, useEffect } from 'react';
import { FrontendWebsocket } from '../websocket';

interface AuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  hardwareID: string | null;
  credits: number;
  gameLoggedIn: boolean;
  gameLoginCooldown: number;
  isGameDataReady: boolean;
  versionUpdate: { newVersion: string; downloadUrl: string } | null;
  isVersionBannerDismissed: boolean;
  updateProgress: { stage: string; percent: number } | null;
  isUpdating: boolean;
  restartRequired: boolean;
  dismissVersionBanner: () => void;
  triggerUpdate: (downloadUrl: string) => void;
  startGame: () => void;
  stopGame: () => void;
  changeLoginDetails: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [hardwareID, setHardwareID] = useState<string | null>(null);
  const [credits, setCredits] = useState(0);
  const [gameLoggedIn, setGameLoggedIn] = useState(false);
  const [gameLoginCooldown, setGameLoginCooldown] = useState(0);
  const [isGameDataReady, setIsGameDataReady] = useState(false);
  const [versionUpdate, setVersionUpdate] = useState<{ newVersion: string; downloadUrl: string } | null>(null);
  const [isVersionBannerDismissed, setIsVersionBannerDismissed] = useState(false);
  const [updateProgress, setUpdateProgress] = useState<{ stage: string; percent: number } | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [restartRequired, setRestartRequired] = useState(false);

  useEffect(() => {
    const handleMessage = (message: any) => {
      // console.log('AuthContext received message:', message);
      if (message.type === 'registrationStatus') {
        setIsAuthenticated(message.payload.registered);
        setHardwareID(message.payload.hardwareID);
        setCredits(message.payload.credits);
        setIsLoading(false);
      } else if (message.type === 'creditsUpdate') {
        console.log('Credits update received:', message.payload.credits);
        setCredits(message.payload.credits);
      } else if (message.type === 'gameLoginStatus') {
        console.log('Game login status received:', message.payload);
        setGameLoggedIn(message.payload.loggedIn);
        setGameLoginCooldown(message.payload.cooldown);
        if (message.payload.loggedIn) {
          setIsGameDataReady(true);
        } else {
          setIsGameDataReady(false);
        }
      } else if (message.type === 'versionUpdate') {
        console.log('Version update received:', message.payload);
        setVersionUpdate({
          newVersion: message.payload.newVersion,
          downloadUrl: message.payload.downloadUrl
        });
        // Reset dismissed state when a new version is detected
        setIsVersionBannerDismissed(false);
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

  const changeLoginDetails = () => {
    FrontendWebsocket.changeLoginDetails();
  };

  const dismissVersionBanner = () => {
    setIsVersionBannerDismissed(true);
  };

  const triggerUpdate = (downloadUrl: string) => {
    setIsUpdating(true);
    setUpdateProgress({ stage: 'Starting update...', percent: 0 });
    FrontendWebsocket.triggerUpdate(downloadUrl);
  };

  return (
    <AuthContext.Provider value={{
      isAuthenticated,
      isLoading,
      hardwareID,
      credits,
      gameLoggedIn,
      gameLoginCooldown,
      isGameDataReady,
      versionUpdate,
      isVersionBannerDismissed,
      updateProgress,
      isUpdating,
      restartRequired,
      dismissVersionBanner,
      triggerUpdate,
      startGame,
      stopGame,
      changeLoginDetails
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

