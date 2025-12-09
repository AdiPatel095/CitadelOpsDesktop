import React from 'react';
import EquipmentStats from './EquipmentStats';

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
        <div className="glass-panel h-full flex flex-col">
            {/* Controls Header */}
            <div className="p-4 border-b border-dark-border flex flex-wrap gap-3">
                {/* Equipment Mode Toggle */}
                <div className="flex bg-dark-bg rounded-lg p-1 gap-1">
                    <button
                        className={`px-4 py-2 rounded-md text-sm font-medium transition-all duration-200 ${equipmentMode === 'Commander'
                                ? 'bg-primary text-dark-bg shadow-glow'
                                : 'text-gray-400 hover:text-white hover:bg-white/5'
                            }`}
                        onClick={() => setEquipmentMode('Commander')}
                    >
                        Commander
                    </button>
                    <button
                        className={`px-4 py-2 rounded-md text-sm font-medium transition-all duration-200 ${equipmentMode === 'Castellan'
                                ? 'bg-primary text-dark-bg shadow-glow'
                                : 'text-gray-400 hover:text-white hover:bg-white/5'
                            }`}
                        onClick={() => setEquipmentMode('Castellan')}
                    >
                        Castellan
                    </button>
                </div>

                {/* Combat Mode Toggle */}
                <div className="flex bg-dark-bg rounded-lg p-1 gap-1">
                    <button
                        className={`px-4 py-2 rounded-md text-sm font-medium transition-all duration-200 ${combatMode === 'PvP'
                                ? 'bg-primary text-dark-bg shadow-glow'
                                : 'text-gray-400 hover:text-white hover:bg-white/5'
                            }`}
                        onClick={() => setCombatMode('PvP')}
                    >
                        PvP
                    </button>
                    <button
                        className={`px-4 py-2 rounded-md text-sm font-medium transition-all duration-200 ${combatMode === 'PvE'
                                ? 'bg-primary text-dark-bg shadow-glow'
                                : 'text-gray-400 hover:text-white hover:bg-white/5'
                            }`}
                        onClick={() => setCombatMode('PvE')}
                    >
                        PvE
                    </button>
                </div>
            </div>

            {/* Selection Container */}
            <div className="flex flex-1 min-h-0">
                {/* Selection Sidebar */}
                <div className="w-56 border-r border-dark-border overflow-y-auto">
                    <div className="p-2 space-y-1">
                        {selectionItems.length === 0 ? (
                            <div className="px-3 py-4 text-center text-gray-500 text-sm">
                                No {equipmentMode.toLowerCase()}s available
                            </div>
                        ) : (
                            selectionItems.map((item, index) => (
                                <div
                                    key={index}
                                    className={`
                                        px-3 py-2.5 rounded-lg cursor-pointer transition-all duration-200
                                        ${selectedIndex === index
                                            ? 'bg-primary/10 text-primary border border-primary/30 shadow-[0_0_10px_rgba(52,211,153,0.1)]'
                                            : 'text-gray-300 hover:bg-white/5 hover:text-white border border-transparent'
                                        }
                                    `}
                                    onClick={() => setSelectedIndex(index)}
                                >
                                    <div className="flex items-center gap-2">
                                        <span className={`
                                            w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold
                                            ${selectedIndex === index
                                                ? 'bg-primary text-dark-bg'
                                                : 'bg-dark-border text-gray-400'
                                            }
                                        `}>
                                            {index + 1}
                                        </span>
                                        <span className="text-sm font-medium truncate">
                                            {item ? item.name : 'Loading...'}
                                        </span>
                                    </div>
                                </div>
                            ))
                        )}
                    </div>
                </div>

                {/* Stats Display */}
                <div className="flex-1 overflow-y-auto">
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
