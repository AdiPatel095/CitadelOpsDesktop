import React, { useState } from 'react';
import EquipmentStats from './EquipmentStats';
import GameButton from '../../components/GameButton';
import { FrontendWebsocket } from '../../websocket';

import { Icons } from '../../components/Icons';

import img1Star from '../../assets/1Star.png';
import img4Star from '../../assets/4Star.png';
import img7Star from '../../assets/7Star.png';
import img12Star from '../../assets/12Star.png';

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
    fullArray: (CommStat | CastStat | null)[];  // Full array with nulls
    selectedItem: CommStat | CastStat | null;
}

// Caution Modal Component
type SellItemType = 'Equipment' | 'Gems';
type RelicTab = 'Non Relic' | 'Relic 1.0' | 'Relic 2.0';

interface SellConfig {
    relicType: RelicTab;
    sellLookItems?: boolean;
    sellRift?: boolean;
    sellRiftGems?: boolean;
    keepStars?: number;
}

interface CautionModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConfirm: (config: SellConfig) => void;
    itemType: SellItemType;
}

const CautionModal: React.FC<CautionModalProps> = ({ isOpen, onClose, onConfirm, itemType }) => {
    // Local state
    const [relicTab, setRelicTab] = useState<RelicTab>('Non Relic');
    const [sellLookItems, setSellLookItems] = useState(false);
    const [sellRift, setSellRift] = useState(false);
    const [sellRiftGems, setSellRiftGems] = useState(false);
    const [keepStars, setKeepStars] = useState(0);
    const [showInfoModal, setShowInfoModal] = useState(false);

    // Reset state when modal opens
    React.useEffect(() => {
        if (isOpen) {
            setRelicTab('Non Relic');
            setSellLookItems(false);
            setSellRift(false);
            setSellRiftGems(false);
            setKeepStars(12); // Default to a reasonable mid-range value (12)
            setShowInfoModal(false);
        }
    }, [isOpen]);

    if (!isOpen) return null;

    const isGems = itemType === 'Gems';
    const singularItem = isGems ? 'gem' : 'item';
    const action = isGems ? 'socket it to an equipment piece' : 'equip it to a commander or castellan';

    const handleConfirm = () => {
        onConfirm({
            relicType: relicTab,
            sellLookItems,
            sellRift,
            sellRiftGems,
            keepStars
        });
    };

    const renderCautionMessage = (message: React.ReactNode, subMessage?: React.ReactNode) => (
        <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-global text-center mb-6">
            <p className="text-text-main text-sm">
                {message}
            </p>
            {subMessage && (
                <p className="text-xs text-text-muted mt-2">
                    {subMessage}
                </p>
            )}
        </div>
    );

    const renderContent = () => {
        switch (relicTab) {
            case 'Non Relic':
                if (isGems) {
                    return (
                        <>
                            {renderCautionMessage(
                                <>This will sell all <span className="font-bold text-amber-500">Non-Relic Gems</span>.</>,
                                <>This action <span className="font-bold text-red-500">CANNOT</span> be reversed.</>
                            )}
                            <div className="space-y-3 mb-6">
                                {/* Sell Rift Gems */}
                                <div
                                    className="flex items-center justify-between p-3 rounded-global bg-bg-app/50 border border-border-base cursor-pointer hover:bg-bg-card-hover transition-colors"
                                    onClick={() => setSellRiftGems(!sellRiftGems)}
                                >
                                    <span className="text-sm font-medium text-text-muted">Sell Rift Gems</span>
                                    <div className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-200 ${sellRiftGems ? 'bg-primary' : 'bg-gray-600'}`}>
                                        <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition duration-200 ${sellRiftGems ? 'translate-x-6' : 'translate-x-1'}`} />
                                    </div>
                                </div>
                            </div>
                        </>
                    );
                }
                return (
                    <>
                        {renderCautionMessage(
                            <>This will sell all <span className="font-bold text-amber-500">Non-Relic Equipment</span>.</>,
                            <>This action <span className="font-bold text-red-500">CANNOT</span> be reversed.</>
                        )}
                        <div className="space-y-3 mb-6">
                            {/* Sell Rift Equipment */}
                            <div
                                className="flex items-center justify-between p-3 rounded-global bg-bg-app/50 border border-border-base cursor-pointer hover:bg-bg-card-hover transition-colors"
                                onClick={() => setSellRift(!sellRift)}
                            >
                                <span className="text-sm font-medium text-text-muted">Sell Rift Gear</span>
                                <div className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-200 ${sellRift ? 'bg-primary' : 'bg-gray-600'}`}>
                                    <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition duration-200 ${sellRift ? 'translate-x-6' : 'translate-x-1'}`} />
                                </div>
                            </div>

                            {/* Sell Look Items */}
                            <div
                                className="flex items-center justify-between p-3 rounded-global bg-bg-app/50 border border-border-base cursor-pointer hover:bg-bg-card-hover transition-colors"
                                onClick={() => setSellLookItems(!sellLookItems)}
                            >
                                <span className="text-sm font-medium text-text-muted">Sell Look Items</span>
                                <div className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-200 ${sellLookItems ? 'bg-primary' : 'bg-gray-600'}`}>
                                    <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition duration-200 ${sellLookItems ? 'translate-x-6' : 'translate-x-1'}`} />
                                </div>
                            </div>
                        </div>
                    </>
                );
            case 'Relic 1.0':
                return renderCautionMessage(
                    <>This will sell <span className="font-bold text-red-500">ALL Relic 1.0</span> {isGems ? 'gems' : 'equipment'}.</>,
                    <>This action <span className="font-bold text-red-500">CANNOT</span> be reversed.</>
                );
            case 'Relic 2.0':
                return (
                    <div className="space-y-4 mb-6">
                        {renderCautionMessage(
                            <>This will sell {isGems ? 'gems' : 'items'} below <span className="font-bold text-white">{keepStars}</span> stars.</>,
                            <>This action <span className="font-bold text-red-500">CANNOT</span> be reversed.</>
                        )}

                        <div className="bg-bg-app/50 border border-border-base rounded-global p-4 relative">
                            <div className="flex items-center justify-between mb-2">
                                <label className="text-sm font-medium text-text-main">Keep Total Stars & Above</label>
                                <div className="flex items-center gap-2">
                                    <button
                                        onClick={() => setShowInfoModal(true)}
                                        className="p-1 rounded-full hover:bg-primary/20 text-text-muted hover:text-primary transition-colors"
                                        title="View Star Rating Guide"
                                    >
                                        <Icons.Info className="w-4 h-4" />
                                    </button>
                                </div>
                            </div>

                            <div className="flex items-center gap-2">
                                <input
                                    type="range"
                                    min="4"
                                    max="42"
                                    step="1"
                                    value={keepStars}
                                    onChange={(e) => setKeepStars(parseInt(e.target.value))}
                                    className="flex-1 accent-primary h-2 bg-bg-card rounded-lg appearance-none cursor-pointer"
                                />
                                <span className="w-8 text-center font-bold text-primary">{keepStars}</span>
                            </div>
                            <div className="flex justify-between text-xs text-text-muted mt-1 px-1">
                                <span>4</span>
                                <span>42</span>
                            </div>
                        </div>

                        <p className="text-xs text-text-muted text-center pt-2">
                            Selling {isGems ? 'gems' : 'items'} with less than <span className="text-white font-mono">{keepStars}</span> total stars.
                        </p>

                    </div>
                );
        }
    };

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
            {/* Backdrop */}
            <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />

            {/* Info Tooltip Popover - Positioned Relative to Modal Parent, Outside the Glass Panel */}
            {showInfoModal && (
                <div
                    className="absolute z-[60] left-[calc(50%+15rem)] top-1/2 -translate-y-1/2 w-96 bg-bg-card border border-border-base rounded-global shadow-2xl animate-fade-in"
                    style={{
                        animation: 'fadeIn 0.15s ease-out',
                    }}
                >
                    {/* Popover Arrow pointing Left */}
                    <div className="absolute top-1/2 -left-2 -translate-y-1/2 w-4 h-4 bg-bg-card border-l border-b border-border-base rotate-45" />

                    {/* Popover Header */}
                    <div className="flex items-center justify-between p-4 border-b border-border-base relative">
                        <h4 className="text-base font-semibold text-text-main flex items-center gap-2">
                            <Icons.Info className="w-4 h-4 text-primary" />
                            Star Rating Guide
                        </h4>
                        <button
                            onClick={() => setShowInfoModal(false)}
                            className="p-1 rounded hover:bg-red-500/20 text-text-muted hover:text-red-400 transition-colors"
                        >
                            <Icons.X className="w-5 h-5" />
                        </button>
                    </div>

                    {/* Popover Content */}
                    <div className="p-4 space-y-4">

                        {/* Individual Stats Card */}
                        <div className="rounded-global border border-primary/30 bg-primary/5 p-3">
                            <div className="flex items-center gap-2 mb-2">
                                <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-primary/20 text-primary">1</span>
                                <span className="text-sm font-semibold text-primary">Individual Stats (1-7 Stars)</span>
                            </div>
                            <div className="bg-black/40 p-2 rounded border border-border-base/30 flex justify-between items-end gap-1">
                                <div className="text-center">
                                    <img src={img1Star} alt="1 Star" className="h-6 w-auto mx-auto mb-1" />
                                    <span className="text-[10px] text-text-muted">1 Star</span>
                                </div>
                                <div className="text-center">
                                    <img src={img4Star} alt="4 Stars" className="h-6 w-auto mx-auto mb-1" />
                                    <span className="text-[10px] text-text-muted">4 Stars</span>
                                </div>
                                <div className="text-center">
                                    <img src={img7Star} alt="7 Stars" className="h-6 w-auto mx-auto mb-1" />
                                    <span className="text-[10px] text-text-muted">7 Stars (Max)</span>
                                </div>
                            </div>
                            <p className="text-xs text-text-muted mt-2 leading-relaxed">
                                Each stat on a Relic 2.0 item can have between <span className="text-text-main font-medium">1 and 7 stars</span>.
                            </p>
                        </div>

                        {/* Total Stars Card */}
                        <div className="rounded-global border border-amber-500/30 bg-amber-500/5 p-3">
                            <div className="flex items-center gap-2 mb-2">
                                <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-amber-500/20 text-amber-500">2</span>
                                <span className="text-sm font-semibold text-amber-500">Safety Threshold</span>
                            </div>
                            <div className="bg-black/40 p-2 rounded border border-border-base/30 flex justify-center mb-2">
                                <img src={img12Star} alt="12 Star Example" className="max-w-full h-auto rounded border border-border-base/20" />
                            </div>
                            <p className="text-xs text-text-muted leading-relaxed">
                                The <span className="text-text-main font-medium">Default Setting (12)</span> is designed to keep decent items like this one or better (12 from top 3+4+2+3).
                            </p>
                        </div>
                    </div>
                </div>
            )}

            {/* Modal Content */}
            <div className="relative glass-panel p-6 max-w-md w-full mx-4 animate-fade-in bg-bg-card">
                {/* Header with Tabs for Equipment and Gems */}
                <div className="flex justify-center mb-6 border-b border-border-base">
                    {(['Non Relic', 'Relic 1.0', 'Relic 2.0'] as RelicTab[]).map((tab) => (
                        <button
                            key={tab}
                            onClick={() => setRelicTab(tab)}
                            className={`
                                pb-2 px-4 text-sm font-medium transition-colors relative
                                ${relicTab === tab ? 'text-primary' : 'text-text-muted hover:text-text-main'}
                            `}
                        >
                            {tab}
                            {relicTab === tab && (
                                <div className="absolute bottom-0 left-0 w-full h-0.5 bg-primary rounded-t-full" />
                            )}
                        </button>
                    ))}
                </div>

                {!isGems ? (
                    <h3 className="text-xl font-bold text-text-main text-center mb-3">
                        Sell {relicTab} Equipment
                    </h3>
                ) : (
                    <h3 className="text-xl font-bold text-text-main text-center mb-3">
                        Sell {relicTab} Gems
                    </h3>
                )}

                {/* Dynamic Content */}
                {renderContent()}

                {/* Common Footer Info */}
                <div className="bg-bg-app/50 border border-border-base rounded-global px-4 py-3 mb-4">
                    <div className="flex items-center justify-center gap-2 text-sm">
                        <span className="text-text-muted">Cost:</span>
                        <span className="text-primary font-semibold">1</span>
                        <img src="/ops-coin.svg" alt="credit" className="w-4 h-4" />
                        <span className="text-text-muted">per {singularItem} sold</span>
                    </div>
                </div>

                <div className="flex items-start gap-2 text-sm text-text-muted mb-6">
                    <span className="text-primary mt-0.5">💡</span>
                    <span>
                        <span className="font-medium text-text-main">Tip:</span> If you want to save a {singularItem}, manually {action} before cleaning the storage.
                    </span>
                </div>

                {/* Buttons */}
                <div className="flex gap-3">
                    <button onClick={onClose} className="flex-1 btn-ghost border border-border-base">Cancel</button>
                    <button onClick={handleConfirm} className="flex-1 px-4 py-2 bg-amber-500 text-bg-app font-semibold rounded-global hover:bg-amber-600 active:scale-95 transition-all duration-200">
                        Confirm Sell
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
    fullArray,
    selectedItem,
}) => {
    const [showSellModal, setShowSellModal] = useState(false);
    const [sellType, setSellType] = useState<SellItemType>('Equipment');

    const openSellModal = (type: SellItemType) => {
        setSellType(type);
        setShowSellModal(true);
    };

    const handleSellConfirm = (config: SellConfig) => {
        if (sellType === 'Equipment') {
            // Check based on Relic Tab
            if (config.relicType === 'Non Relic') {
                FrontendWebsocket.sendMessage({
                    type: 'sellNonRelicEquipment',
                    payload: {
                        sellLookItems: !!config.sellLookItems,
                        sellRift: !!config.sellRift
                    }
                });
            } else if (config.relicType === 'Relic 1.0') {
                FrontendWebsocket.sendMessage({
                    type: 'sellRelic1Equipment',
                    payload: {}
                });
            } else if (config.relicType === 'Relic 2.0') {
                FrontendWebsocket.sendMessage({
                    type: 'sellRelic2Equipment',
                    payload: {
                        keepStars: config.keepStars || 0
                    }
                });
            }
        } else {
            if (config.relicType === 'Non Relic') {
                FrontendWebsocket.sendMessage({
                    type: 'sellNonRelicGems',
                    payload: {
                        sellRiftGems: !!config.sellRiftGems
                    }
                });
            } else if (config.relicType === 'Relic 1.0') {
                FrontendWebsocket.sendMessage({
                    type: 'sellRelic1Gems',
                    payload: {}
                });
            } else if (config.relicType === 'Relic 2.0') {
                FrontendWebsocket.sendMessage({
                    type: 'sellRelic2Gems',
                    payload: {
                        keepStars: config.keepStars || 0
                    }
                });
            }
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
            <div className="p-4 border-b border-border-base flex flex-wrap items-center gap-3">
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
                        className="px-4 py-2 bg-amber-500/10 text-amber-500 border border-amber-500/30 font-medium text-sm rounded-global hover:bg-amber-500/20 hover:border-amber-500/50 active:scale-95 transition-all duration-200"
                    >
                        Sell Gems
                    </GameButton>
                    <GameButton
                        onClick={() => openSellModal('Equipment')}
                        className="px-4 py-2 bg-amber-500/10 text-amber-500 border border-amber-500/30 font-medium text-sm rounded-global hover:bg-amber-500/20 hover:border-amber-500/50 active:scale-95 transition-all duration-200"
                    >
                        Sell Equipment
                    </GameButton>
                </div>
            </div>

            {/* Selection Container */}
            <div className="flex flex-1 min-h-0">
                {/* Selection Sidebar */}
                <div className="w-56 border-r border-border-base overflow-y-auto"
                    style={{
                        scrollbarWidth: 'thin',
                        scrollbarColor: 'var(--color-border-base) transparent'
                    }}>
                    <div className="p-2 space-y-1">
                        {fullArray.every(item => item === null) ? (
                            <div className="px-3 py-4 text-center text-text-muted text-sm">
                                No {equipmentMode.toLowerCase()}s available
                            </div>
                        ) : (
                            fullArray.map((item, index) => {
                                // Skip null items during rendering
                                if (item === null) return null;
                                return (
                                    <div
                                        key={index}
                                        className={`
                                            rounded-global px-3 py-2.5 cursor-pointer transition-all duration-200
                                            ${selectedIndex === index
                                                ? 'bg-primary/10 text-primary border border-primary/30 shadow-[0_0_10px_rgba(52,211,153,0.1)]'
                                                : 'text-text-muted hover:bg-bg-card-hover hover:text-text-main border border-transparent'
                                            }
                                        `}
                                        onClick={() => setSelectedIndex(index)}
                                    >
                                        <div className="flex items-center gap-2">
                                            <span className={`
                                                rounded-global w-6 h-6 flex items-center justify-center text-xs font-bold
                                                ${selectedIndex === index
                                                    ? 'bg-primary text-bg-app'
                                                    : 'bg-border-base text-text-muted'
                                                }
                                            `}>
                                                {index + 1}
                                            </span>
                                            <span className="text-sm font-medium truncate">
                                                {item.name}
                                            </span>
                                        </div>
                                    </div>
                                );
                            })
                        )}
                    </div>
                </div>

                {/* Stats Display */}
                <div className="flex-1 overflow-y-auto"
                    style={{
                        scrollbarWidth: 'thin',
                        scrollbarColor: 'var(--color-border-base) transparent'
                    }}>
                    <EquipmentStats
                        equipmentMode={equipmentMode}
                        combatMode={combatMode}
                        selectedItem={selectedItem}
                        selectedIndex={selectedIndex}
                    />
                </div>
            </div>
        </div>
    );
};

export default EquipmentSelection;
