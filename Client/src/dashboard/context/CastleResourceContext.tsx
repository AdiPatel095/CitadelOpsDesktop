import React, { createContext, useContext, useState, useEffect, type ReactNode, useCallback, useMemo } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import { type PlayerCastleInfo } from '../models/PlayerCastleInfo.ts';

export const CASTLE_LOCATIONS = [
    'mainCastle', 'outpost1', 'outpost2', 'outpost3',
    'iceCastle', 'desertCastle', 'dungeonCastle', 'stormCastle'
] as const;

type CastleLocation = typeof CASTLE_LOCATIONS[number];
type CastleResourceMap = Map<CastleLocation, PlayerCastleInfo>;
type LoadingStatus = Record<CastleLocation, boolean>;

interface CastleResourceContextType {
    castleResources: CastleResourceMap;
    isCastleResourcesLoading: LoadingStatus;
    getCastle: (location: CastleLocation) => PlayerCastleInfo | undefined;
    requestCastleResource: (castleLocation: CastleLocation) => void;
    requestAllCastleResources: () => void;
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
    const [isCastleResourcesLoading, setIsCastleResourcesLoading] = useState<LoadingStatus>(
        CASTLE_LOCATIONS.reduce((acc, castleLocation) => {
            acc[castleLocation] = true;
            return acc;
        }, {} as LoadingStatus)
    );

    const requestCastleResource = useCallback((castleLocation: CastleLocation) => {
        setIsCastleResourcesLoading(prev => ({ ...prev, [castleLocation]: true }));
        FrontendWebsocket.sendMessage({ type: 'getCastleResourceUpdate', castleLocation });
    }, []);

    const requestAllCastleResources = useCallback(() => {
        CASTLE_LOCATIONS.forEach(location => requestCastleResource(location));
    }, [requestCastleResource]);

    const getCastle = useCallback(
        (location: CastleLocation) => castleResources.get(location),
        [castleResources]
    );

    useEffect(() => {
        const handleCastleUpdate = (message: WebsocketMessage) => {
            if (message.type === 'castleResourceUpdate' && message.optionalData && CASTLE_LOCATIONS.includes(message.optionalData as CastleLocation)) {
                const castleLocation = message.optionalData as CastleLocation;
                const castleInfo = message.payload as PlayerCastleInfo;

                setCastleResources(prevMap => {
                    const newMap = new Map(prevMap);
                    newMap.set(castleLocation, castleInfo);
                    return newMap;
                });

                setIsCastleResourcesLoading(prev => ({ ...prev, [castleLocation]: false }));
            }
        };

        FrontendWebsocket.addMessageListener(handleCastleUpdate);
        // requestAllCastleResources();

        return () => {
            FrontendWebsocket.removeMessageListener(handleCastleUpdate);
        };
    }, [requestAllCastleResources]);

    const value = useMemo(() => ({ castleResources, isCastleResourcesLoading, getCastle, requestCastleResource, requestAllCastleResources }), [castleResources, isCastleResourcesLoading, getCastle, requestCastleResource, requestAllCastleResources]);

    return (
        <CastleResourceContext.Provider value={value}>
            {children}
        </CastleResourceContext.Provider>
    );
};
