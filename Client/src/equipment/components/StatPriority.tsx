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
    equipmentMode: 'Commander' | 'Castellan';
}

type TierType = 1 | 2;

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
            case 1: return { color: 'rose-500', bg: 'rose-500', label: 'Max Stat' };
            case 2: return { color: 'primary', bg: 'primary', label: 'Have in Random Slots' };
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
    const [tier1Stats, setTier1Stats] = useState<string[]>([]);
    const [tier2Stats, setTier2Stats] = useState<string[]>([]);
    const [showAddDropdown, setShowAddDropdown] = useState(false);
    const [dragState, setDragState] = useState<DragState | null>(null);
    const [dropTarget, setDropTarget] = useState<{ tier: TierType; index: number } | null>(null);
    const [isReconfiguring, setIsReconfiguring] = useState(false);
    const [reconfigureError, setReconfigureError] = useState<string | null>(null);
    const dropdownRef = useRef<HTMLDivElement>(null);
    const [showComparisonModal, setShowComparisonModal] = useState(false);
    const [comparisonData, setComparisonData] = useState<ComparisonData | null>(null);
    const [showInfoModal, setShowInfoModal] = useState(false);

    const hasEnoughCredits = credits >= RECONFIGURE_COST;
    const totalStats = tier1Stats.length + tier2Stats.length;

    // Get available stats based on equipment mode
    const allStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        return Object.values(groups).flat();
    }, [equipmentMode]);

    // Stats not yet in any tier
    const availableStats = useMemo(() => {
        const usedStats = [...tier1Stats, ...tier2Stats];
        return allStats.filter(stat => !usedStats.includes(stat));
    }, [allStats, tier1Stats, tier2Stats]);

    // Group available stats by category for the dropdown
    const groupedAvailableStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        const usedStats = [...tier1Stats, ...tier2Stats];

        const result: { [key: string]: string[] } = {};
        for (const [groupName, stats] of Object.entries(groups)) {
            const available = stats.filter(stat => !usedStats.includes(stat));
            if (available.length > 0) {
                result[groupName] = available;
            }
        }
        return result;
    }, [equipmentMode, tier1Stats, tier2Stats]);

    // Helper to get tier state setters
    const getTierState = (tier: TierType) => {
        switch (tier) {
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
                targetIndex: selectedIndex,
                stats: [
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
        setTier1Stats([]);
        // Default to Core Stats in Tier 2
        const defaultStats = equipmentMode === 'Commander'
            ? commanderStatGroups.core
            : castellanStatGroups.core;
        setTier2Stats(defaultStats);
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
                    targetIndex: message.payload.targetIndex,
                    equipmentMode: equipmentMode
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
                        <button
                            onClick={() => setShowInfoModal(true)}
                            className="p-1 rounded-full hover:bg-primary/20 text-gray-400 hover:text-primary transition-colors"
                            title="Learn about stat priorities"
                        >
                            <Icons.Info className="w-4 h-4" />
                        </button>
                    </h3>

                    <div className="flex items-center gap-2">
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
            </div>



            {/* Three-Tier Priority Lists - Stacked */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3">
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

            {/* Info Modal */}
            {showInfoModal && (
                <div className="fixed inset-0 bg-black/80 backdrop-blur-sm flex items-center justify-center z-50">
                    <div className="bg-dark-bg border border-dark-border rounded-global max-w-md w-full mx-4 shadow-2xl">
                        {/* Modal Header */}
                        <div className="flex items-center justify-between p-4 border-b border-dark-border">
                            <h4 className="text-lg font-semibold text-white flex items-center gap-2">
                                <Icons.Info className="w-5 h-5 text-primary" />
                                Understanding Stat Priority
                            </h4>
                            <button
                                onClick={() => setShowInfoModal(false)}
                                className="p-1 rounded hover:bg-red-500/20 text-gray-400 hover:text-red-400 transition-colors"
                            >
                                <Icons.X className="w-5 h-5" />
                            </button>
                        </div>

                        {/* Modal Content */}
                        <div className="p-4 space-y-4">
                            {/* Tier 1 Explanation */}
                            <div className="rounded-global border border-rose-500/30 bg-rose-500/5 p-3">
                                <div className="flex items-center gap-2 mb-2">
                                    <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-rose-500/20 text-rose-500">
                                        1
                                    </span>
                                    <span className="font-semibold text-rose-400">Max Stat</span>
                                </div>
                                <p className="text-sm text-gray-300 leading-relaxed">
                                    Stats in this tier will be <span className="text-rose-400 font-medium">maximized to their ceiling values</span>.
                                    The optimizer will prioritize finding gear that pushes these stats as high as possible.
                                </p>
                                <p className="text-xs text-gray-500 mt-2 italic">
                                    Order matters — stats listed first receive higher priority.
                                </p>
                            </div>

                            {/* Tier 2 Explanation */}
                            <div className="rounded-global border border-primary/30 bg-primary/5 p-3">
                                <div className="flex items-center gap-2 mb-2">
                                    <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-primary/20 text-primary">
                                        2
                                    </span>
                                    <span className="font-semibold text-primary">Have in Random Slots</span>
                                </div>
                                <p className="text-sm text-gray-300 leading-relaxed">
                                    Stats in this tier will be <span className="text-primary font-medium">included when possible</span>,
                                    but won't be pushed to their maximum. The optimizer ensures these stats appear
                                    in your loadout without sacrificing Tier 1 priorities.
                                </p>
                                <p className="text-xs text-gray-500 mt-2 italic">
                                    Order matters — stats listed first are preferred.
                                </p>
                            </div>

                            {/* Best Practices */}
                            <div className="rounded-global border border-dark-border bg-dark-bg/50 p-3">
                                <div className="flex items-center gap-2 mb-2">
                                    <span className="text-yellow-400">★</span>
                                    <span className="font-semibold text-gray-200">Best Practices</span>
                                </div>
                                <ul className="text-sm text-gray-400 space-y-1.5">
                                    <li className="flex items-start gap-2">
                                        <span className="text-primary mt-0.5">•</span>
                                        <span>Keep Tier 1 focused — <span className="text-gray-300">1-3 stats work best</span></span>
                                    </li>
                                    <li className="flex items-start gap-2">
                                        <span className="text-primary mt-0.5">•</span>
                                        <span>Use Tier 2 for <span className="text-gray-300">secondary combat stats</span></span>
                                    </li>
                                    <li className="flex items-start gap-2">
                                        <span className="text-primary mt-0.5">•</span>
                                        <span>Drag stats to <span className="text-gray-300">reorder priorities</span> within tiers</span>
                                    </li>
                                </ul>
                            </div>
                        </div>

                        {/* Modal Footer */}
                        <div className="p-4 border-t border-dark-border">
                            <button
                                onClick={() => setShowInfoModal(false)}
                                className="w-full py-2 px-4 rounded-global bg-primary/20 hover:bg-primary/30 text-primary font-medium transition-colors"
                            >
                                Got it
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* Comparison Modal */}
            <ReconfigureComparisonModal
                isOpen={showComparisonModal}
                onClose={() => setShowComparisonModal(false)}
                currentLoadout={comparisonData?.currentLoadout ?? null}
                newLoadout={comparisonData?.newLoadout ?? null}
                targetIndex={comparisonData?.targetIndex ?? 0}
                combatMode={combatMode}
                equipmentMode={equipmentMode}
            />
        </div>
    );
};

export default StatPriority;

