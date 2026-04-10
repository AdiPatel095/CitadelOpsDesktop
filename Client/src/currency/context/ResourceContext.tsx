import React, { createContext, useContext, useState, useEffect } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import type { PlayerGlobalResources } from '../../types/playerGlobalResources.ts';
import { useAuth } from '../../context/AuthContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';

interface ResourceContextType {
  resources: PlayerGlobalResources | null;
}

const ResourceContext = createContext<ResourceContextType | undefined>(undefined);

export const useResources = () => {
  const context = useContext(ResourceContext);
  if (!context) {
    throw new Error('useResources must be used within a ResourceProvider');
  }
  return context;
};

export const ResourceProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [resources, setResources] = useState<PlayerGlobalResources | null>(null);
  const { gameLoggedIn } = useAuth();
  const { snapshot } = useLastKnownSnapshot();

  useEffect(() => {
    const handleResourceUpdate = (data: any) => {
      if (data.type === 'globalResourceUpdate') {
        setResources(data.payload as PlayerGlobalResources);
      }
    };

    FrontendWebsocket.addMessageListener(handleResourceUpdate);

    return () => {
      FrontendWebsocket.removeMessageListener(handleResourceUpdate);
    };
  }, []);

  useEffect(() => {
    if (!snapshot || gameLoggedIn) return;
    const gs = snapshot.gameState;
    if (!gs || typeof gs !== 'object') return;
    const gr = (gs as Record<string, unknown>).globalResources;
    if (!gr || typeof gr !== 'object') return;
    setResources((prev) => (prev == null ? (gr as PlayerGlobalResources) : prev));
  }, [snapshot, gameLoggedIn]);

  return (
    <ResourceContext.Provider value={{ resources }}>
      {children}
    </ResourceContext.Provider>
  );
};
