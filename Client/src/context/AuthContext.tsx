import React, { createContext, useContext, useState, ReactNode, useEffect } from 'react';
import { FrontendWebsocket } from '../websocket';

interface AuthContextType {
  isAuthenticated: boolean;
  login: () => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    const handleLoginStatus = (message: any) => {
      if (message.type === 'LOGIN_STATUS') {
        if (message.payload.status === 'success') {
          setIsAuthenticated(true);
        } else {
          setIsAuthenticated(false);
        }
      }
    };

    FrontendWebsocket.addMessageListener(handleLoginStatus);
    FrontendWebsocket.connect('ws://localhost:8080/ws');

    return () => {
      FrontendWebsocket.removeMessageListener(handleLoginStatus);
    };
  }, []);

  const login = () => {
    // For now, we'll just set isAuthenticated to true
    // In the future, this would involve a login request to the backend
    setIsAuthenticated(true);
  };

  const logout = () => {
    setIsAuthenticated(false);
  };

  return (
    <AuthContext.Provider value={{ isAuthenticated, login, logout }}>
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
