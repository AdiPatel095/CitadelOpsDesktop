import React, { createContext, useContext, useState, type ReactNode, useEffect } from 'react';
import { FrontendWebsocket } from '../../websocket.ts';
import {
    type CommStat,
    type CastStatArray,
    type EquipmentData
} from '../models/equipment.ts';

interface EquipmentContextType {
    equipmentData: EquipmentData;
    setEquipmentData: (data: EquipmentData) => void;
}

const EquipmentContext = createContext<EquipmentContextType | undefined>(undefined);

export const useEquipment = () => {
    const context = useContext(EquipmentContext);
    if (!context) {
        throw new Error('useEquipment must be used within an EquipmentProvider');
    }
    return context;
};

interface EquipmentProviderProps {
    children: ReactNode;
}

export const EquipmentProvider: React.FC<EquipmentProviderProps> = ({ children }) => {
    const [equipmentData, setEquipmentData] = useState<EquipmentData>({
        commStats: [],
        castStats: null,
    });

    useEffect(() => {
        console.log('EquipmentContext: equipmentData changed to', equipmentData);
    }, [equipmentData]);

    useEffect(() => {
        const handleEquipmentUpdate = (data: any) => {
            switch (data.type) {
                case 'commStatUpdate':
                    setEquipmentData(prevData => ({ ...prevData, commStats: data.payload as CommStat[] }));
                    break;
                case 'castStatUpdate':
                    setEquipmentData(prevData => ({ ...prevData, castStats: data.payload as CastStatArray }));
                    break;
                default:
                    break;
            }
        };

        FrontendWebsocket.addMessageListener(handleEquipmentUpdate);

        return () => {
            FrontendWebsocket.removeMessageListener(handleEquipmentUpdate);
        };
    }, []);

    const value = {
        equipmentData,
        setEquipmentData,
    };

    return (
        <EquipmentContext.Provider value={value}>
            {children}
        </EquipmentContext.Provider>
    );
};
