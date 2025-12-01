import React, { createContext, useContext, useState, useEffect } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';

interface PlayerGlobalResources {
  rubies: number;
  coins: number;
  relic_shard: number;
  sceat: number;
  ducat: number;
  const_token: number;
  upgr_token: number;
  affl_tix: number;
  plaster: number;
  drg_scale: number;
  drg_spl: number;
  min1: number;
  min5: number;
  min10: number;
  min30: number;
  hr1: number;
  hr5: number;
  hr24: number;
  might_pt: number;
  glory_pt: number;
  gallan_pt: number;
}

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
    console.log('ResourceContext: resources changed to', resources);
  }, [resources]);

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
