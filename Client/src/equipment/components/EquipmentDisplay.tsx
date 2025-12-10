import React from 'react';
import { useEquipment } from '../context/EquipmentContext.tsx';

const EquipmentDisplay: React.FC = () => {
    const { equipmentData } = useEquipment();

    return (
        <div>
            <h2>Equipment Data</h2>
            <pre>{JSON.stringify(equipmentData, null, 2)}</pre>
        </div>
    );
};

export default EquipmentDisplay;
