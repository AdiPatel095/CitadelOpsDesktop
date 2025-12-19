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
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.connect('ws://localhost:8080/ws');

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

  return (
    <AuthContext.Provider value={{
      isAuthenticated,
      isLoading,
      hardwareID,
      credits,
      gameLoggedIn,
      gameLoginCooldown,
      isGameDataReady,
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

