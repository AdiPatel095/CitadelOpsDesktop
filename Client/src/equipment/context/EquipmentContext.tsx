import React, { createContext, useContext, useState, type ReactNode, useEffect, useMemo } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import {
    type CommStat,
    type CastStat,
} from '../models/equipment.ts';

export const CASTLE_LOCATIONS = [
    'mainCastle', 'outpost1', 'outpost2', 'outpost3',
    'iceCastle', 'desertCastle', 'dungeonCastle', 'stormCastle'
];

type CastStats = Record<string, CastStat>;
type LoadingStatus = Record<string, boolean>;

interface EquipmentData {
    commStats: CommStat[];
    castellanStats: CastStat[];
    castStats: CastStats;
}

interface EquipmentContextType {
    equipmentData: EquipmentData;
    isCastStatsLoading: LoadingStatus;
    isCommStatsLoading: boolean;
}

// Create a default, "empty" state for the context. This prevents consumers
// from ever receiving `undefined` and causing runtime errors.
const defaultEquipmentContext: EquipmentContextType = {
    equipmentData: {
        commStats: [],
        castellanStats: [],
        castStats: {},
    },
    isCastStatsLoading: {},
    isCommStatsLoading: true,
};

const EquipmentContext = createContext<EquipmentContextType>(defaultEquipmentContext);

export const useEquipment = () => {
    const context = useContext(EquipmentContext);
    // The check for undefined is no longer strictly necessary due to the default value,
    // but it's good practice to keep it as a safeguard.
    return context;
};

type WebsocketMessage = {
    type: string;
    payload: any;
    optionalData?: string;
};

interface EquipmentProviderProps {
    children: ReactNode;
}

export const EquipmentProvider: React.FC<EquipmentProviderProps> = ({ children }) => {
    const [commStats, setCommStats] = useState<CommStat[]>(Array(50).fill(null));
    const [castStats, setCastStats] = useState<CastStats>({});
    const [isCommStatsLoading, setIsCommStatsLoading] = useState(true);
    const [isCastStatsLoading, setIsCastStatsLoading] = useState<LoadingStatus>(
        CASTLE_LOCATIONS.reduce((acc, castleLocation) => {
            acc[castleLocation] = true;
            return acc;
        }, {} as LoadingStatus));
    const [commStatsLoadedCount, setCommStatsLoadedCount] = useState(0);

    useEffect(() => {
        const handleEquipmentUpdate = (message: WebsocketMessage) => {
            if (typeof message !== 'object' || message === null || !message.type) {
                return;
            }

            switch (message.type) {
                case 'commStatUpdate':
                    if (message.optionalData !== undefined && message.payload) {
                        const index = parseInt(message.optionalData, 10);

                        if (!isNaN(index)) {
                            const newStat = message.payload as CommStat;
                            setCommStats(prev => {
                                const newStats = [...prev];
                                newStats[index] = newStat;
                                return newStats;
                            });
                            setCommStatsLoadedCount(prev => prev + 1);
                        } else {
                            console.error(`[commStatUpdate] Invalid index received. optionalData: "${message.optionalData}"`);
                        }
                    } else {
                        console.warn('[commStatUpdate] Message missing optionalData or payload.');
                    }
                    break;

                case 'castStatUpdate':
                    if (message.optionalData && message.payload) {
                        const castleLocation = message.optionalData;
                        const newStat = message.payload as CastStat;
                        setCastStats(prev => {
                            return {
                                ...prev,
                                [castleLocation]: newStat,
                            };
                        });
                        setIsCastStatsLoading(prev => ({ ...prev, [castleLocation]: false }));
                    }
                    break;

                default:
                    // console.log('Unknown websocket message type:', message.type);
                    break;
            }
        };

        FrontendWebsocket.addMessageListener(handleEquipmentUpdate);

        return () => {
            FrontendWebsocket.removeMessageListener(handleEquipmentUpdate);
        };
    }, []); // Empty dependency array: This effect runs only once to set up the listener.

    // This effect correctly determines the loading state for commStats.
    useEffect(() => {
        // If we have received updates for all 50 commanders, loading is complete.
        setIsCommStatsLoading(commStatsLoadedCount < 50);
    }, [commStatsLoadedCount]);

    // Memoize a flattened array of castellan stats
    const castellanStats = useMemo(() => {
        return Object.values(castStats).filter(Boolean); // filter(Boolean) removes any null/undefined entries
    }, [castStats]);

    const value = useMemo(() => ({
        equipmentData: {
            commStats,
            castellanStats,
            castStats,
        },
        isCastStatsLoading,
        isCommStatsLoading,
    }), [commStats, castellanStats, castStats, isCastStatsLoading, isCommStatsLoading]);

    return (
        <EquipmentContext.Provider value={value}>
            {children}
        </EquipmentContext.Provider>
    );
};
