import React, { createContext, useContext, useState, type ReactNode, useEffect, useMemo } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import {
    type CommStat,
    type CastStat,
} from '../models/equipment.ts';
import { useAuth } from '../../context/AuthContext';

// Castle index mapping (0-10) — matches server Models.NumPlayerCastleSlots / castStatUpdate optionalData
// 0 Main … 7 Storm, 8 BeriWorld, 9 Metropolis, 10 Capital
export const NUM_CASTLE_SLOTS = 11;
export const CASTLE_LOCATIONS = [
    'MainCastle', 'Outpost1', 'Outpost2', 'Outpost3',
    'IceCastle', 'DesertCastle', 'DungeonCastle', 'StormCastle', 'BeriWorldCastle',
    'MetropolisCastle', 'CapitalCastle',
];

interface EquipmentData {
    commStats: CommStat[];
    castellanStats: CastStat[];
    castStats: (CastStat | null)[];
}

interface EquipmentContextType {
    equipmentData: EquipmentData;
    isCastStatsLoading: boolean[];
    isCommStatsLoading: boolean;
}

// Create a default, "empty" state for the context. This prevents consumers
// from ever receiving `undefined` and causing runtime errors.
const defaultEquipmentContext: EquipmentContextType = {
    equipmentData: {
        commStats: [],
        castellanStats: [],
        castStats: Array(NUM_CASTLE_SLOTS).fill(null),
    },
    isCastStatsLoading: Array(NUM_CASTLE_SLOTS).fill(true),
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
    const [castStats, setCastStats] = useState<(CastStat | null)[]>(Array(NUM_CASTLE_SLOTS).fill(null));
    const [isCommStatsLoading, setIsCommStatsLoading] = useState(true);
    const [isCastStatsLoading, setIsCastStatsLoading] = useState<boolean[]>(Array(NUM_CASTLE_SLOTS).fill(true));
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
                    if (message.optionalData !== undefined && message.payload) {
                        const index = parseInt(message.optionalData, 10);

                        if (!isNaN(index) && index >= 0 && index < NUM_CASTLE_SLOTS) {
                            const newStat = message.payload as CastStat;
                            setCastStats(prev => {
                                const newStats = [...prev];
                                newStats[index] = newStat;
                                return newStats;
                            });
                            setIsCastStatsLoading(prev => {
                                const newLoading = [...prev];
                                newLoading[index] = false;
                                return newLoading;
                            });
                        } else {
                            console.error(`[castStatUpdate] Invalid index received. optionalData: "${message.optionalData}"`);
                        }
                    }
                    break;

                case 'refreshEquipment':
                    // Backend notifies us to refresh all equipment data
                    console.log('Received refreshEquipment notification from backend');
                    FrontendWebsocket.refreshEquipment();
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

    const { isGameDataReady } = useAuth();

    useEffect(() => {
        if (!isGameDataReady) return;

        // Fetch data 3 seconds after connection
        const timeout = setTimeout(() => {
            console.log('Initial equipment data fetch...');
            FrontendWebsocket.refreshEquipment();
        }, 3000);

        const interval = setInterval(() => {
            console.log('Refreshing equipment data...');
            FrontendWebsocket.refreshEquipment();
        }, 15000);

        return () => {
            clearTimeout(timeout);
            clearInterval(interval);
        };
    }, [isGameDataReady]);

    // This effect correctly determines the loading state for commStats.
    useEffect(() => {
        // If we have received updates for all 50 commanders, loading is complete.
        setIsCommStatsLoading(commStatsLoadedCount < 50);
    }, [commStatsLoadedCount]);

    // Memoize a flattened array of castellan stats (filter out nulls)
    const castellanStats = useMemo(() => {
        return castStats.filter((stat): stat is CastStat => stat !== null);
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
