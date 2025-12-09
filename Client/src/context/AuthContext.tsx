import { createContext, useContext, useState, type ReactNode, useEffect } from 'react';
import { FrontendWebsocket } from '../websocket';

interface AuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  hardwareID: string | null;
  credits: number;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [hardwareID, setHardwareID] = useState<string | null>(null);
  const [credits, setCredits] = useState(0);

  useEffect(() => {
    const handleMessage = (message: any) => {
      if (message.type === 'registrationStatus') {
        setIsAuthenticated(message.payload.registered);
        setHardwareID(message.payload.hardwareID);
        setCredits(message.payload.credits);
        setIsLoading(false);
      } else if (message.type === 'creditsUpdate') {
        setCredits(message.payload.credits);
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.connect('ws://localhost:8080/ws');

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
    };
  }, []);

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, hardwareID, credits }}>
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

