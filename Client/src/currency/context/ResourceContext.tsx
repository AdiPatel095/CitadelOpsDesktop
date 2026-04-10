import React, { createContext, useContext, useState, useEffect } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import type { PlayerGlobalResources } from '../../types/playerGlobalResources.ts';

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

  return (
    <ResourceContext.Provider value={{ resources }}>
      {children}
    </ResourceContext.Provider>
  );
};
