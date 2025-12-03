import React from 'react';
import EquipmentStats from './EquipmentStats';
import './EquipmentView.css';
import { type CommStat, type CastStat } from '../models/equipment';

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

interface EquipmentSelectionProps {
    equipmentMode: EquipmentMode;
    setEquipmentMode: (mode: EquipmentMode) => void;
    combatMode: CombatMode;
    setCombatMode: (mode: CombatMode) => void;
    selectedIndex: number | null;
    setSelectedIndex: (index: number) => void;
    selectionItems: (CommStat | CastStat)[];
    selectedItem: CommStat | CastStat | null;
}

const EquipmentSelection: React.FC<EquipmentSelectionProps> = ({
    equipmentMode,
    setEquipmentMode,
    combatMode,
    setCombatMode,
    selectedIndex,
    setSelectedIndex,
    selectionItems,
    selectedItem,
}) => {
    return (
        <div className="equipment-panel left-panel">
            <div className="controls-header">
                <div className="switch-container">
                    <button
                        className={`switch-btn ${equipmentMode === 'Commander' ? 'active' : ''}`}
                        onClick={() => setEquipmentMode('Commander')}
                    >
                        Commander
                    </button>
                    <button
                        className={`switch-btn ${equipmentMode === 'Castellan' ? 'active' : ''}`}
                        onClick={() => setEquipmentMode('Castellan')}
                    >
                        Castellan
                    </button>
                </div>
                <div className="switch-container">
                    <button
                        className={`switch-btn ${combatMode === 'PvP' ? 'active' : ''}`}
                        onClick={() => setCombatMode('PvP')}
                    >
                        PvP
                    </button>
                    <button
                        className={`switch-btn ${combatMode === 'PvE' ? 'active' : ''}`}
                        onClick={() => setCombatMode('PvE')}
                    >
                        PvE
                    </button>
                </div>
            </div>

            <div className="selection-container">
                <div className="selection-sidebar">
                    {selectionItems.map((item, index) => (
                        <div
                            key={index}
                            className={`selection-item ${selectedIndex === index ? 'active' : ''}`}
                            onClick={() => setSelectedIndex(index)}
                        >
                            {`${index}: ${item.name}`}
                        </div>
                    ))}
                </div>
                <div className="stats-display">
                    <EquipmentStats
                        equipmentMode={equipmentMode}
                        combatMode={combatMode}
                        selectedItem={selectedItem}
                    />
                </div>
            </div>
        </div>
    );
};

export default EquipmentSelection;
