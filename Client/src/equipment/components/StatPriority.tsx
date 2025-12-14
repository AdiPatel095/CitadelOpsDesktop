import React, { useState, useMemo, useRef, useEffect } from 'react';
import { Icons } from '../../components/Icons';
import GameButton from '../../components/GameButton';
import { type CommStat, statDisplayName, commanderStatGroups, castellanStatGroups, statGroupDisplayName } from '../models/equipment';
import { FrontendWebsocket } from '../../websocket';
import ReconfigureComparisonModal from './ReconfigureComparisonModal';

const RECONFIGURE_COST = 10000;

interface StatPriorityProps {
    equipmentMode: 'Commander' | 'Castellan';
    combatMode: 'PvP' | 'PvE';
    credits: number;
    hardwareID: string | null;
    selectedIndex: number | null;
}

interface ComparisonData {
    currentLoadout: CommStat;
    newLoadout: CommStat;
    targetIndex: number;
}

type TierType = 0 | 1 | 2;

interface DragState {
    stat: string;
    fromTier: TierType;
    fromIndex: number;
}

interface TierListProps {
    tier: TierType;
    stats: string[];
    dragState: DragState | null;
    dropTarget: { tier: TierType; index: number } | null;
    onDragStart: (e: React.DragEvent, stat: string, tier: TierType, index: number) => void;
    onDragOver: (e: React.DragEvent, tier: TierType, index: number) => void;
    onDragEnd: () => void;
    onDrop: (e: React.DragEvent, tier: TierType) => void;
    onRemove: (stat: string) => void;
}

const TierList: React.FC<TierListProps> = ({
    tier,
    stats,
    dragState,
    dropTarget,
    onDragStart,
    onDragOver,
    onDragEnd,
    onDrop,
    onRemove,
}) => {
    const getTierStyle = () => {
        switch (tier) {
            case 0: return { color: 'rose-500', bg: 'rose-500', label: 'Max Stat' };
            case 1: return { color: 'primary', bg: 'primary', label: 'Ultimate Stat' };
            case 2: return { color: 'amber-500', bg: 'amber-500', label: 'Optimize Stat' };
        }
    };
    const style = getTierStyle();

    return (
        <div
            className={`rounded-global border ${dropTarget?.tier === tier && stats.length === 0
                ? `border-${style.color} bg-${style.bg}/5`
                : 'border-dark-border/30'
                } transition-colors`}
            onDragOver={(e) => {
                e.preventDefault();
                if (stats.length === 0) {
                    onDragOver(e, tier, 0);
                }
            }}
            onDrop={(e) => onDrop(e, tier)}
        >
            {/* Tier Header */}
            <div className={`px-3 py-2 border-b border-dark-border/30 flex items-center gap-2`}>
                <span className={`w-5 h-5 rounded flex items-center justify-center text-xs font-bold bg-${style.bg}/20 text-${style.color}`}>
                    {tier}
                </span>
                <span className="text-xs font-medium text-gray-400">
                    {style.label}
                </span>
            </div>

            {/* Stats List */}
            <div className="p-2 min-h-[50px]">
                {stats.length === 0 ? (
                    <div className="text-center py-2 text-gray-600 text-xs">
                        Drop stats here
                    </div>
                ) : (
                    <div className="space-y-1.5">
                        {stats.map((stat, index) => {
                            const isDragging = dragState?.stat === stat;
                            const isDropTarget = dropTarget?.tier === tier && dropTarget?.index === index;

                            return (
                                <div
                                    key={stat}
                                    draggable
                                    onDragStart={(e) => onDragStart(e, stat, tier, index)}
                                    onDragOver={(e) => {
                                        e.preventDefault();
                                        onDragOver(e, tier, index);
                                    }}
                                    onDragEnd={onDragEnd}
                                    className={`
                                        rounded-global flex items-center gap-2 px-2.5 py-2 bg-dark-bg/50 border 
                                        transition-all duration-150 cursor-grab active:cursor-grabbing
                                        ${isDragging ? 'opacity-40 scale-95' : ''}
                                        ${isDropTarget
                                            ? `border-${style.color} shadow-md shadow-${style.bg}/20`
                                            : 'border-dark-border/50 hover:border-gray-600'
                                        }
                                    `}
                                >
                                    <span className={`w-5 h-5 rounded flex items-center justify-center text-xs font-bold bg-${style.bg}/10 text-${style.color}`}>
                                        {index + 1}
                                    </span>
                                    <span className="text-sm text-gray-300 flex-1 truncate">
                                        {statDisplayName[stat] || stat}
                                    </span>
                                    <button
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            onRemove(stat);
                                        }}
                                        className="p-0.5 rounded hover:bg-red-500/20 text-gray-600 hover:text-red-400 transition-colors"
                                    >
                                        <Icons.X className="w-3.5 h-3.5" />
                                    </button>
                                    <Icons.GripVertical className="w-3.5 h-3.5 text-gray-600" />
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>
        </div>
    );
};

const StatPriority: React.FC<StatPriorityProps> = ({
    equipmentMode,
    combatMode,
    credits,
    hardwareID,
    selectedIndex
}) => {
    const [tier0Stats, setTier0Stats] = useState<string[]>([]);
    const [tier1Stats, setTier1Stats] = useState<string[]>([]);
    const [tier2Stats, setTier2Stats] = useState<string[]>([]);
    const [showAddDropdown, setShowAddDropdown] = useState(false);
    const [dragState, setDragState] = useState<DragState | null>(null);
    const [dropTarget, setDropTarget] = useState<{ tier: TierType; index: number } | null>(null);
    const [isReconfiguring, setIsReconfiguring] = useState(false);
    const [reconfigureError, setReconfigureError] = useState<string | null>(null);
    const [showSettings, setShowSettings] = useState(false);
    const [interTierMultiplier, setInterTierMultiplier] = useState<number>(2);
    const [intraTierMultiplier, setIntraTierMultiplier] = useState<number>(5);
    const dropdownRef = useRef<HTMLDivElement>(null);
    const [showComparisonModal, setShowComparisonModal] = useState(false);
    const [comparisonData, setComparisonData] = useState<ComparisonData | null>(null);

    const hasEnoughCredits = credits >= RECONFIGURE_COST;
    const totalStats = tier0Stats.length + tier1Stats.length + tier2Stats.length;

    // Get available stats based on equipment mode
    const allStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        return Object.values(groups).flat();
    }, [equipmentMode]);

    // Stats not yet in any tier
    const availableStats = useMemo(() => {
        const usedStats = [...tier0Stats, ...tier1Stats, ...tier2Stats];
        return allStats.filter(stat => !usedStats.includes(stat));
    }, [allStats, tier0Stats, tier1Stats, tier2Stats]);

    // Group available stats by category for the dropdown
    const groupedAvailableStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        const usedStats = [...tier0Stats, ...tier1Stats, ...tier2Stats];

        const result: { [key: string]: string[] } = {};
        for (const [groupName, stats] of Object.entries(groups)) {
            const available = stats.filter(stat => !usedStats.includes(stat));
            if (available.length > 0) {
                result[groupName] = available;
            }
        }
        return result;
    }, [equipmentMode, tier0Stats, tier1Stats, tier2Stats]);

    // Helper to get tier state setters
    const getTierState = (tier: TierType) => {
        switch (tier) {
            case 0: return { stats: tier0Stats, setStats: setTier0Stats };
            case 1: return { stats: tier1Stats, setStats: setTier1Stats };
            case 2: return { stats: tier2Stats, setStats: setTier2Stats };
        }
    };

    // Drag handlers
    const handleDragStart = (e: React.DragEvent, stat: string, tier: TierType, index: number) => {
        setDragState({ stat, fromTier: tier, fromIndex: index });
        e.dataTransfer.effectAllowed = 'move';
    };

    const handleDragOver = (_e: React.DragEvent, tier: TierType, index: number) => {
        if (dragState) {
            setDropTarget({ tier, index });
        }
    };

    const handleDragEnd = () => {
        if (dragState && dropTarget) {
            const { stat, fromTier, fromIndex } = dragState;
            const { tier: toTier, index: toIndex } = dropTarget;

            if (fromTier === toTier) {
                // Reorder within same tier
                const { stats, setStats } = getTierState(fromTier);
                const newStats = [...stats];
                if (fromIndex !== toIndex) {
                    newStats.splice(fromIndex, 1);
                    newStats.splice(toIndex, 0, stat);
                    setStats(newStats);
                }
            } else {
                // Move between tiers
                const fromState = getTierState(fromTier);
                const toState = getTierState(toTier);

                fromState.setStats(fromState.stats.filter(s => s !== stat));
                const newToStats = [...toState.stats];
                newToStats.splice(toIndex, 0, stat);
                toState.setStats(newToStats);
            }
        }
        setDragState(null);
        setDropTarget(null);
    };

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const handleDrop = (_e: React.DragEvent, _tier: TierType) => {
        // Drop is handled in dragEnd
    };

    // Add/Remove handlers - always adds to Tier 1, user can drag to other tiers
    const addStat = (stat: string) => {
        setTier1Stats([...tier1Stats, stat]);
        // setShowAddDropdown(false); // Keep menu open
    };

    const removeStat = (stat: string) => {
        setTier0Stats(tier0Stats.filter(s => s !== stat));
        setTier1Stats(tier1Stats.filter(s => s !== stat));
        setTier2Stats(tier2Stats.filter(s => s !== stat));
    };

    // Reconfigure handler - builds JSON payload
    const handleReconfigure = async () => {
        if (!hasEnoughCredits || !hardwareID || totalStats === 0 || selectedIndex === null) return;

        setIsReconfiguring(true);
        setReconfigureError(null);

        try {
            // Build structured payload
            const reconfigurePayload = {
                equipmentMode: equipmentMode,
                combatMode: combatMode,
                interTierMultiplier: interTierMultiplier,
                intraTierMultiplier: intraTierMultiplier,
                targetIndex: selectedIndex,
                stats: [
                    ...tier0Stats.map((stat, index) => ({ stat, tier: 0, position: index })),
                    ...tier1Stats.map((stat, index) => ({ stat, tier: 1, position: index })),
                    ...tier2Stats.map((stat, index) => ({ stat, tier: 2, position: index }))
                ]
            };

            console.log('Reconfigure Payload:', JSON.stringify(reconfigurePayload, null, 2));

            FrontendWebsocket.sendReconfigureLoadout({
                hardwareID,
                ...reconfigurePayload
            });

            // The WebSocket listener will handle the comparison response
            // and show the modal when reconfigureComparison is received
        } catch (error) {
            setReconfigureError('An unexpected error occurred');
            setIsReconfiguring(false);
        }
    };

    // Close dropdown when clicking outside
    React.useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setShowAddDropdown(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    // Reset priority stats when equipment mode changes
    React.useEffect(() => {
        setTier0Stats([]);
        // Default to Core Stats in Tier 1
        const defaultStats = equipmentMode === 'Commander'
            ? commanderStatGroups.core
            : castellanStatGroups.core;
        setTier1Stats(defaultStats);
        setTier2Stats([]);
        setReconfigureError(null);
    }, [equipmentMode]);

    // Listen for reconfigureComparison message from backend
    useEffect(() => {
        const handleMessage = (message: any) => {
            if (message.type === 'reconfigureComparison' && message.payload) {
                console.log('Received reconfigureComparison:', message.payload);
                setComparisonData({
                    currentLoadout: message.payload.currentLoadout,
                    newLoadout: message.payload.newLoadout,
                    targetIndex: message.payload.targetIndex
                });
                setShowComparisonModal(true);
                setIsReconfiguring(false);
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);
        return () => FrontendWebsocket.removeMessageListener(handleMessage);
    }, []);

    return (
        <div className="glass-panel h-full flex flex-col">
            {/* Header with Add Stat Button */}
            <div className="p-4 border-b border-dark-border">
                <div className="flex items-center justify-between gap-2">
                    <h3 className="text-lg font-semibold text-white flex items-center gap-2">
                        <Icons.Activity className="w-5 h-5 text-primary" />
                        Stat Priority
                    </h3>

                    <div className="flex items-center gap-2">
                        {/* Settings Button */}
                        <button
                            onClick={() => setShowSettings(!showSettings)}
                            className={`p-2 rounded-global transition-colors ${showSettings ? 'bg-primary/20 text-primary' : 'bg-dark-bg/50 text-gray-400 hover:text-white hover:bg-dark-bg'}`}
                            title="Configure Multipliers"
                        >
                            <Icons.Settings className="w-4 h-4" />
                        </button>

                        {/* Add Stat Button */}
                        <div className="relative" ref={dropdownRef}>
                            <button
                                onClick={() => setShowAddDropdown(!showAddDropdown)}
                                disabled={availableStats.length === 0}
                                className="rounded-global p-2 bg-primary/10 hover:bg-primary/20 text-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                                title="Add Stat"
                            >
                                <Icons.Plus className="w-4 h-4" />
                            </button>

                            {/* Dropdown */}
                            {showAddDropdown && availableStats.length > 0 && (
                                <div className="rounded-global absolute top-full right-0 mt-2 w-56 bg-dark-bg border border-dark-border shadow-xl max-h-64 overflow-y-auto z-50">
                                    {Object.entries(groupedAvailableStats).map(([groupName, stats]) => (
                                        <div key={groupName}>
                                            <div className="px-3 py-2 text-xs font-bold text-gray-500 uppercase tracking-wider bg-dark-bg/80 sticky top-0">
                                                {statGroupDisplayName[groupName] || groupName}
                                            </div>
                                            {stats.map(stat => (
                                                <button
                                                    key={stat}
                                                    onClick={() => addStat(stat)}
                                                    className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-primary/10 hover:text-primary transition-colors"
                                                >
                                                    {statDisplayName[stat] || stat}
                                                </button>
                                            ))}
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>
                </div>
                <p className="text-xs text-gray-500 mt-1">Drag between tiers to reorganize</p>

                {/* Settings Modal/Panel */}
                {showSettings && (
                    <div className="mt-3 p-3 bg-dark-bg/50 rounded-global border border-dark-border/50 space-y-3 animate-in fade-in slide-in-from-top-2 duration-200">
                        <div>
                            <div className="flex items-center justify-between mb-1">
                                <span className="text-sm text-gray-300">Inter-Tier Multiplier</span>
                                <input
                                    type="number"
                                    min="1"
                                    step="0.1"
                                    value={interTierMultiplier}
                                    onChange={(e) => setInterTierMultiplier(Math.max(1, parseFloat(e.target.value) || 1))}
                                    className="rounded-global w-16 px-2 py-1 bg-dark-bg border border-dark-border text-white text-sm text-center focus:outline-none focus:border-primary/50 transition-colors"
                                />
                            </div>
                            <p className="text-xs text-gray-500 leading-relaxed">
                                Multiplier within the same tier (priority weight difference).
                            </p>
                        </div>

                        <div className="border-t border-dark-border/30 pt-3">
                            <div className="flex items-center justify-between mb-1">
                                <span className="text-sm text-gray-300">Intra-Tier Multiplier</span>
                                <input
                                    type="number"
                                    min="1"
                                    step="0.1"
                                    value={intraTierMultiplier}
                                    onChange={(e) => setIntraTierMultiplier(Math.max(1, parseFloat(e.target.value) || 1))}
                                    className="rounded-global w-16 px-2 py-1 bg-dark-bg border border-dark-border text-white text-sm text-center focus:outline-none focus:border-primary/50 transition-colors"
                                />
                            </div>
                            <p className="text-xs text-gray-500 leading-relaxed">
                                Multiplier between different tiers (how much stronger a higher tier is).
                            </p>
                        </div>
                    </div>
                )}
            </div>

            {/* Three-Tier Priority Lists - Stacked */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3">
                <TierList
                    tier={0}
                    stats={tier0Stats}
                    dragState={dragState}
                    dropTarget={dropTarget}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnd={handleDragEnd}
                    onDrop={handleDrop}
                    onRemove={removeStat}
                />
                <TierList
                    tier={1}
                    stats={tier1Stats}
                    dragState={dragState}
                    dropTarget={dropTarget}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnd={handleDragEnd}
                    onDrop={handleDrop}
                    onRemove={removeStat}
                />
                <TierList
                    tier={2}
                    stats={tier2Stats}
                    dragState={dragState}
                    dropTarget={dropTarget}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnd={handleDragEnd}
                    onDrop={handleDrop}
                    onRemove={removeStat}
                />
            </div>



            {/* Reconfigure Button */}
            <div className="p-4 border-t border-dark-border">
                {reconfigureError && (
                    <div className="rounded-global mb-3 p-2 bg-red-500/10 border border-red-500/30">
                        <p className="text-xs text-red-400">{reconfigureError}</p>
                    </div>
                )}
                <GameButton
                    onClick={handleReconfigure}
                    disabled={!hasEnoughCredits || isReconfiguring || totalStats === 0}
                    className={`
                        rounded-global w-full flex items-center justify-center gap-2 py-3 px-4 font-medium transition-all duration-200
                        ${hasEnoughCredits && totalStats > 0
                            ? 'bg-primary/20 hover:bg-primary/30 text-primary border border-primary/30 hover:border-primary/50'
                            : 'bg-gray-800/50 text-gray-500 border border-gray-700/50 cursor-not-allowed'
                        }
                    `}
                >
                    {!hasEnoughCredits ? (
                        <>
                            <Icons.Lock className="w-4 h-4" />
                            <span>Reconfigure {equipmentMode}</span>
                        </>
                    ) : isReconfiguring ? (
                        <>
                            <Icons.RefreshCw className="w-4 h-4 animate-spin" />
                            <span>Reconfiguring...</span>
                        </>
                    ) : (
                        <>
                            <Icons.RefreshCw className="w-4 h-4" />
                            <span>Reconfigure {equipmentMode}</span>
                        </>
                    )}
                </GameButton>
                <div className="mt-2 text-center">
                    <span className={`text-xs ${hasEnoughCredits ? 'text-gray-500' : 'text-red-400'}`}>
                        Cost: {RECONFIGURE_COST.toLocaleString()} credits
                        {!hasEnoughCredits && ` (You have ${credits.toLocaleString()})`}
                    </span>
                </div>
            </div>

            {/* Comparison Modal */}
            <ReconfigureComparisonModal
                isOpen={showComparisonModal}
                onClose={() => setShowComparisonModal(false)}
                currentLoadout={comparisonData?.currentLoadout ?? null}
                newLoadout={comparisonData?.newLoadout ?? null}
                targetIndex={comparisonData?.targetIndex ?? 0}
                combatMode={combatMode}
            />
        </div>
    );
};

export default StatPriority;
