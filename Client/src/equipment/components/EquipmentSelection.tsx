import React, { useState } from 'react';
import EquipmentStats from './EquipmentStats';
import GameButton from '../../components/GameButton';
import { FrontendWebsocket } from '../../websocket';

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

// Caution Modal Component
type SellItemType = 'Equipment' | 'Gems';

interface CautionModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConfirm: () => void;
    itemType: SellItemType;
}

const CautionModal: React.FC<CautionModalProps> = ({ isOpen, onClose, onConfirm, itemType }) => {
    if (!isOpen) return null;

    const isGems = itemType === 'Gems';
    const singularItem = isGems ? 'gem' : 'item';
    const action = isGems ? 'socket it to an equipment piece' : 'equip it to a commander or castellan';

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
            {/* Backdrop */}
            <div
                className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                onClick={onClose}
            />

            {/* Modal Content */}
            <div className="relative glass-panel p-6 max-w-md mx-4 animate-fade-in">
                {/* Caution Icon */}
                <div className="flex justify-center mb-4">
                    <div className="w-16 h-16 rounded-full bg-amber-500/20 flex items-center justify-center">
                        <svg
                            className="w-8 h-8 text-amber-500"
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                        >
                            <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2}
                                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                            />
                        </svg>
                    </div>
                </div>

                {/* Title */}
                <h3 className="text-xl font-bold text-white text-center mb-3">
                    Caution
                </h3>

                {/* Warning Message */}
                <p className="text-gray-300 text-center mb-4">
                    This will sell all <span className="text-amber-400 font-semibold">Non-Relic {itemType.toLowerCase()}</span> and is <span className="text-red-400 font-semibold">not reversible</span>.
                </p>

                {/* Cost Info */}
                <div className="bg-dark-bg/50 border border-dark-border rounded-global px-4 py-3 mb-4">
                    <div className="flex items-center justify-center gap-2 text-sm">
                        <span className="text-gray-400">Cost:</span>
                        <span className="text-primary font-semibold">1</span>
                        <img src="/ops-coin.svg" alt="credit" className="w-4 h-4" />
                        <span className="text-gray-400">per {singularItem} sold</span>
                    </div>
                </div>

                {/* Tip */}
                <div className="flex items-start gap-2 text-sm text-gray-400 mb-6">
                    <span className="text-primary mt-0.5">💡</span>
                    <span>
                        <span className="font-medium text-gray-300">Tip:</span> If you want to save a {singularItem}, manually {action} before cleaning the storage.
                    </span>
                </div>

                {/* Buttons */}
                <div className="flex gap-3">
                    <button
                        onClick={onClose}
                        className="flex-1 btn-ghost border border-dark-border"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={onConfirm}
                        className="flex-1 px-4 py-2 bg-amber-500 text-dark-bg font-semibold rounded-global hover:bg-amber-600 active:scale-95 transition-all duration-200"
                    >
                        Confirm
                    </button>
                </div>
            </div>
        </div>
    );
};

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
    const [showSellModal, setShowSellModal] = useState(false);
    const [sellType, setSellType] = useState<SellItemType>('Equipment');

    const openSellModal = (type: SellItemType) => {
        setSellType(type);
        setShowSellModal(true);
    };

    const handleSellConfirm = () => {
        if (sellType === 'Equipment') {
            FrontendWebsocket.sendMessage({ type: 'sellNonRelicEquipment' });
        } else {
            FrontendWebsocket.sendMessage({ type: 'sellNonRelicGems' });
        }
        setShowSellModal(false);
    };

    return (
        <div className="glass-panel h-full flex flex-col">
            {/* Caution Modal */}
            <CautionModal
                isOpen={showSellModal}
                onClose={() => setShowSellModal(false)}
                onConfirm={handleSellConfirm}
                itemType={sellType}
            />

            {/* Controls Header */}
            <div className="p-4 border-b border-dark-border flex flex-wrap items-center gap-3">
                {/* Equipment Mode Toggle */}
                <div className="toggle-container">
                    <button
                        className={`toggle-btn ${equipmentMode === 'Commander' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                        onClick={() => setEquipmentMode('Commander')}
                    >
                        Commander
                    </button>
                    <button
                        className={`toggle-btn ${equipmentMode === 'Castellan' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                        onClick={() => setEquipmentMode('Castellan')}
                    >
                        Castellan
                    </button>
                </div>

                {/* Combat Mode Toggle */}
                <div className="toggle-container">
                    <button
                        className={`toggle-btn ${combatMode === 'PvP' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                        onClick={() => setCombatMode('PvP')}
                    >
                        PvP
                    </button>
                    <button
                        className={`toggle-btn ${combatMode === 'PvE' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                        onClick={() => setCombatMode('PvE')}
                    >
                        PvE
                    </button>
                </div>

                {/* Sell Buttons - pushed to right */}
                <div className="ml-auto flex gap-2">
                    <GameButton
                        onClick={() => openSellModal('Gems')}
                        className="px-4 py-2 bg-amber-500/20 text-amber-400 border border-amber-500/30 font-medium text-sm rounded-global hover:bg-amber-500/30 hover:border-amber-500/50 active:scale-95 transition-all duration-200"
                    >
                        Sell Gems
                    </GameButton>
                    <GameButton
                        onClick={() => openSellModal('Equipment')}
                        className="px-4 py-2 bg-amber-500/20 text-amber-400 border border-amber-500/30 font-medium text-sm rounded-global hover:bg-amber-500/30 hover:border-amber-500/50 active:scale-95 transition-all duration-200"
                    >
                        Sell Equipment
                    </GameButton>
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
                                        rounded-global px-3 py-2.5 cursor-pointer transition-all duration-200
                                        ${selectedIndex === index
                                            ? 'bg-primary/10 text-primary border border-primary/30 shadow-[0_0_10px_rgba(52,211,153,0.1)]'
                                            : 'text-gray-300 hover:bg-white/5 hover:text-white border border-transparent'
                                        }
                                    `}
                                    onClick={() => setSelectedIndex(index)}
                                >
                                    <div className="flex items-center gap-2">
                                        <span className={`
                                            rounded-global w-6 h-6 flex items-center justify-center text-xs font-bold
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
