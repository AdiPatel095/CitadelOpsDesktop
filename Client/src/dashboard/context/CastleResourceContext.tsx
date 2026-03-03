import React, { createContext, useContext, useState, useEffect, type ReactNode, useCallback, useMemo } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import { type PlayerCastleInfo } from '../models/PlayerCastleInfo.ts';

// We now index by CastleID (number) instead of string location keys
type CastleResourceMap = Map<number, PlayerCastleInfo>;
type LoadingStatus = Record<number, boolean>;

interface CastleResourceContextType {
    castleResources: CastleResourceMap;
    isCastleResourcesLoading: LoadingStatus;
    getCastle: (castleId: number) => PlayerCastleInfo | undefined;
    requestCastleResource: (castleId: number) => void;
}

const CastleResourceContext = createContext<CastleResourceContextType | undefined>(undefined);

export const useCastleResources = () => {
    const context = useContext(CastleResourceContext)
    if (!context) {
        throw new Error('useCastleResources must be used within a CastleResourceProvider');
    }
    return context;
};

interface WebsocketMessage {
    type: string;
    payload: unknown;
    optionalData?: string;
}

export const CastleResourceProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
    const [castleResources, setCastleResources] = useState<CastleResourceMap>(new Map());
    const [isCastleResourcesLoading, setIsCastleResourcesLoading] = useState<LoadingStatus>({});

    const requestCastleResource = useCallback((castleId: number) => {
        setIsCastleResourcesLoading(prev => ({ ...prev, [castleId]: true }));
        // Note: The backend still expects location strings for targeted requests,
        // but since we don't actively request resources individually right now,
        // we can just send the request. If needed, the backend can be updated to accept ID.
        FrontendWebsocket.sendMessage({ type: 'getCastleResourceUpdate', castleId });
    }, []);

    const getCastle = useCallback(
        (castleId: number) => castleResources.get(castleId),
        [castleResources]
    );

    useEffect(() => {
        const handleCastleUpdate = (message: WebsocketMessage) => {
            if (message.type === 'castleResourceUpdate' && message.payload) {
                const castleInfo = message.payload as PlayerCastleInfo;
                if (!castleInfo.aid) return; // Must have an ID

                // Debug logging for Main Castle and Outpost 1
                if (castleInfo.castleName === 'Main Castle' || castleInfo.castleName === 'Outpost 1') {
                    console.log(`[CastleUpdate] Received data for ${castleInfo.castleName} (ID: ${castleInfo.aid}):`, {
                        troopsMixed: castleInfo.troops?.troopsMixed,
                        wood: castleInfo.amount.wood_amount
                    });
                }

                const castleId = castleInfo.aid;

                setCastleResources(prevMap => {
                    const newMap = new Map(prevMap);
                    newMap.set(castleId, castleInfo);
                    return newMap;
                });

                setIsCastleResourcesLoading(prev => ({ ...prev, [castleId]: false }));
            }
        };

        FrontendWebsocket.addMessageListener(handleCastleUpdate);

        return () => {
            FrontendWebsocket.removeMessageListener(handleCastleUpdate);
        };
    }, []);

    const value = useMemo(() => ({ castleResources, isCastleResourcesLoading, getCastle, requestCastleResource }), [castleResources, isCastleResourcesLoading, getCastle, requestCastleResource]);

    return (
        <CastleResourceContext.Provider value={value}>
            {children}
        </CastleResourceContext.Provider>
    );
};
