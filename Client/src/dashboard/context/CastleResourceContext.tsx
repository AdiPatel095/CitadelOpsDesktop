import React, { createContext, useContext, useState, useEffect, type ReactNode, useCallback, useMemo } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import { useAuth } from '../../context/AuthContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { CASTLE_SLOT_KEYS } from '../../utils/castleSnapshotHydration.ts';
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
    const { gameLoggedIn } = useAuth();
    const { snapshot } = useLastKnownSnapshot();

    const requestCastleResource = useCallback((castleId: number) => {
        setIsCastleResourcesLoading(prev => ({ ...prev, [castleId]: true }));
        // Note: The backend still expects location strings for targeted requests,
        // but since we don't actively request resources individually right now,
        // we can just send the request. If needed, the backend can be updated to accept ID.
        FrontendWebsocket.sendMessage({ type: 'getCastleResourceUpdate', castleId });
    }, []);

    useEffect(() => {
        const handleCastleUpdate = (message: WebsocketMessage) => {
            if (message.type === 'castleResourceUpdate' && message.payload) {
                const castleInfo = message.payload as PlayerCastleInfo;
                const castleId = Math.trunc(Number(castleInfo.aid));
                if (!castleId || castleId <= 0) return;

                // Debug logging for Main Castle and Outpost 1
                if (castleInfo.castleName === 'Main Castle' || castleInfo.castleName === 'Outpost 1') {
                    console.log(`[CastleUpdate] Received data for ${castleInfo.castleName} (ID: ${castleId}):`, {
                        troopsMixed: castleInfo.troops?.troopsMixed,
                        wood: castleInfo.amount.wood_amount
                    });
                }

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

    /**
     * While disconnected, overlay persisted snapshot slots onto the map during render (not in useEffect).
     * Otherwise castleFocus can update to the picked aid one frame before effect runs, and CastleView shows
     * "No castle data" even though the snapshot already contains that castle.
     */
    const castleResourcesWithSnapshot = useMemo(() => {
        if (gameLoggedIn || !snapshot) return castleResources;
        const gs = snapshot.gameState;
        if (!gs || typeof gs !== 'object') return castleResources;
        const castleRoot = (gs as Record<string, unknown>).castle;
        if (!castleRoot || typeof castleRoot !== 'object') return castleResources;
        const next = new Map(castleResources);
        for (const k of CASTLE_SLOT_KEYS) {
            const raw = (castleRoot as Record<string, unknown>)[k];
            if (!raw || typeof raw !== 'object') continue;
            const info = raw as PlayerCastleInfo;
            const aid = Math.trunc(Number(info.aid));
            if (aid <= 0) continue;
            next.set(aid, info);
        }
        return next;
    }, [castleResources, snapshot, gameLoggedIn]);

    const getCastle = useCallback(
        (castleId: number) => castleResourcesWithSnapshot.get(castleId),
        [castleResourcesWithSnapshot]
    );

    const value = useMemo(
        () => ({
            castleResources: castleResourcesWithSnapshot,
            isCastleResourcesLoading,
            getCastle,
            requestCastleResource,
        }),
        [castleResourcesWithSnapshot, isCastleResourcesLoading, getCastle, requestCastleResource]
    );

    return (
        <CastleResourceContext.Provider value={value}>
            {children}
        </CastleResourceContext.Provider>
    );
};
