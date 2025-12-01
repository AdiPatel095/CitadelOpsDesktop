import React, { createContext, useContext, useState, useEffect } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import { type PlayerCastleInfo } from '../models/PlayerCastleInfo.ts';

const CastleResourceContext = createContext<{ castles: PlayerCastleInfo | null }>({ castles: null });

export const useCastleResources = () => useContext(CastleResourceContext);

export const CastleResourceProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [castles, setCastles] = useState<PlayerCastleInfo | null>(null);

  useEffect(() => {
    console.log('CastleResourceContext: castles changed to', castles);
  }, [castles]);

  useEffect(() => {
    const handleCastleUpdate = (data: any) => {
      if (data.type === 'castleResourceUpdate') {
        setCastles(data.payload as PlayerCastleInfo);
      }
    };

    FrontendWebsocket.addMessageListener(handleCastleUpdate);

    return () => {
      FrontendWebsocket.removeMessageListener(handleCastleUpdate);
    };
  }, []);

  return (
    <CastleResourceContext.Provider value={{ castles }}>
      {children}
    </CastleResourceContext.Provider>
  );
};
