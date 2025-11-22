import React, { createContext, useState, useEffect, useContext, type ReactNode } from 'react';

interface AuthContextType {
  isLocked: boolean;
  wsStatus: string;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [isLocked, setIsLocked] = useState(true);
  const [wsStatus, setWsStatus] = useState('Disconnected');

  useEffect(() => {
    const socket = new WebSocket('ws://localhost:8081/ws');

    socket.onopen = () => setWsStatus('Connected');
    socket.onclose = () => setWsStatus('Disconnected');
    socket.onerror = (error) => console.error('WebSocket error:', error);

    socket.onmessage = (event) => {
      console.log('WebSocket message received:', event.data);
      try {
        const message = JSON.parse(event.data);
        if (message.type === 'LOGIN_STATUS') {
          const payload = message.payload;
          if (payload.status === 'success') {
            console.log('Login successful! Unlocking application.');
            setIsLocked(false); // Unlock the app on successful login
          }
        }
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };

    // The cleanup function will run when the component unmounts
    return () => socket.close();
  }, []); // The empty array is correct here, the logging will help us debug.

  return <AuthContext.Provider value={{ isLocked, wsStatus }}>{children}</AuthContext.Provider>;
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};