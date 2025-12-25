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
                : 'border-border-base/30'
                } transition-colors bg-bg-app/30`}
            onDragOver={(e) => {
                e.preventDefault();
                if (stats.length === 0) {
                    onDragOver(e, tier, 0);
                }
            }}
            onDrop={(e) => onDrop(e, tier)}
        >
            {/* Tier Header */}
            <div className={`px-3 py-2 border-b border-border-base/30 flex items-center gap-2`}>
                <span className={`w-5 h-5 rounded flex items-center justify-center text-xs font-bold bg-${style.bg}/20 text-${style.color}`}>
                    {tier}
                </span>
                <span className="text-xs font-medium text-text-muted">
                    {style.label}
                </span>
            </div>

            {/* Stats List */}
            <div className="p-2 min-h-[50px]">
                {stats.length === 0 ? (
                    <div className="text-center py-2 text-text-muted text-xs">
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
                                        rounded-global flex items-center gap-2 px-2.5 py-2 bg-bg-card border 
                                        transition-all duration-150 cursor-grab active:cursor-grabbing
                                        ${isDragging ? 'opacity-40 scale-95' : ''}
                                        ${isDropTarget
                                            ? `border-${style.color} shadow-md shadow-${style.bg}/20`
                                            : 'border-border-base/50 hover:border-text-muted'
                                        }
                                    `}
                                >
                                    <span className={`w-5 h-5 rounded flex items-center justify-center text-xs font-bold bg-${style.bg}/10 text-${style.color}`}>
                                        {index + 1}
                                    </span>
                                    <span className="text-sm text-text-muted flex-1 truncate">
                                        {statDisplayName[stat] || stat}
                                    </span>
                                    <button
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            onRemove(stat);
                                        }}
                                        className="p-0.5 rounded hover:bg-red-500/20 text-text-muted hover:text-red-400 transition-colors"
                                    >
                                        <Icons.X className="w-3.5 h-3.5" />
                                    </button>
                                    <Icons.GripVertical className="w-3.5 h-3.5 text-text-muted" />
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
            } else if (message.type === 'reconfigureError') {
                // Reset reconfiguring state when backend returns an error
                console.log('Received reconfigureError:', message.payload);
                setIsReconfiguring(false);
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);
        return () => FrontendWebsocket.removeMessageListener(handleMessage);
    }, []);

    return (
        <div className="glass-panel h-full flex flex-col relative">
            {/* Header with Add Stat Button */}
            <div className="p-4 border-b border-border-base">
                <div className="flex items-center justify-between gap-2">
                    <h3 className="text-lg font-semibold text-text-main flex items-center gap-2">
                        <Icons.Activity className="w-5 h-5 text-primary" />
                        Stat Priority
                        <button
                            onClick={() => setShowInfoModal(true)}
                            className="p-1 rounded-full hover:bg-primary/20 text-text-muted hover:text-primary transition-colors"
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
                                <div className="rounded-global absolute top-full right-0 mt-2 w-56 bg-bg-card border border-border-base shadow-xl max-h-64 overflow-y-auto z-50">
                                    {Object.entries(groupedAvailableStats).map(([groupName, stats]) => (
                                        <div key={groupName}>
                                            <div className="px-3 py-2 text-xs font-bold text-text-muted uppercase tracking-wider bg-bg-card/95 sticky top-0 backdrop-blur-sm">
                                                {statGroupDisplayName[groupName] || groupName}
                                            </div>
                                            {stats.map(stat => (
                                                <button
                                                    key={stat}
                                                    onClick={() => addStat(stat)}
                                                    className="w-full text-left px-3 py-2 text-sm text-text-muted hover:bg-primary/10 hover:text-primary transition-colors"
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
                <p className="text-xs text-text-muted mt-1">Drag between tiers to reorganize</p>
            </div>



            {/* Three-Tier Priority Lists - Stacked */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3"
                style={{
                    scrollbarWidth: 'thin',
                    scrollbarColor: 'var(--color-border-base) transparent'
                }}>
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
            <div className="p-4 border-t border-border-base">
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
                            : 'bg-bg-card-hover/50 text-text-muted border border-border-base/50 cursor-not-allowed'
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
                    <span className={`text-xs ${hasEnoughCredits ? 'text-text-muted' : 'text-red-400'}`}>
                        Cost: {RECONFIGURE_COST.toLocaleString()} credits{!hasEnoughCredits ? ` (You have ${credits.toLocaleString()})` : ''}
                    </span>
                </div>
            </div>

            {/* Info Tooltip Popover */}
            {showInfoModal && (
                <div
                    className="absolute top-12 right-4 z-50 w-[600px] bg-bg-card border border-border-base rounded-global shadow-2xl animate-fade-in"
                    style={{
                        animation: 'fadeIn 0.15s ease-out',
                    }}
                >
                    {/* Popover Arrow */}
                    <div className="absolute -top-2 right-[160px] w-4 h-4 bg-bg-card border-l border-t border-border-base rotate-45" />

                    {/* Popover Header */}
                    <div className="flex items-center justify-between p-5 border-b border-border-base relative">
                        <h4 className="text-lg font-semibold text-text-main flex items-center gap-2">
                            <Icons.Info className="w-5 h-5 text-primary" />
                            Understanding Stat Priority
                        </h4>
                        <button
                            onClick={() => setShowInfoModal(false)}
                            className="p-1 rounded hover:bg-red-500/20 text-text-muted hover:text-red-400 transition-colors"
                        >
                            <Icons.X className="w-6 h-6" />
                        </button>
                    </div>

                    {/* Popover Content */}
                    <div className="p-6 space-y-5 max-h-[700px] overflow-y-auto">
                        {/* Tier 1 Explanation */}
                        <div className="rounded-global border border-rose-500/30 bg-rose-500/5 p-5">
                            <div className="flex items-center gap-3 mb-2">
                                <span className="w-7 h-7 rounded flex items-center justify-center text-base font-bold bg-rose-500/20 text-rose-500">
                                    1
                                </span>
                                <span className="text-lg font-semibold text-rose-400">Max Stat</span>
                            </div>
                            <p className="text-base text-text-muted leading-relaxed">
                                These stats will be <span className="text-rose-400 font-medium">maximized to their limits</span> as much as possible.
                            </p>
                        </div>

                        {/* Tier 2 Explanation */}
                        <div className="rounded-global border border-primary/30 bg-primary/5 p-5">
                            <div className="flex items-center gap-3 mb-2">
                                <span className="w-7 h-7 rounded flex items-center justify-center text-base font-bold bg-primary/20 text-primary">
                                    2
                                </span>
                                <span className="text-lg font-semibold text-primary">Have in Random Slots</span>
                            </div>
                            <p className="text-base text-text-muted leading-relaxed mb-3">
                                The bot will try to have exactly <span className="text-primary font-medium">1 copy of each stat in random slots</span> from this list.
                            </p>
                            <p className="text-base text-text-muted leading-relaxed">
                                Relic 2.0 gear gets a total of <span className="text-text-main font-medium">9 random stats</span>. If you have less than 9 stats in this list, the bot will pick a combo to have 1 copy of all stats listed here in the random slots of the base equipment and then any leftover random stats will double up with (very likely) your highest priority stat.
                            </p>
                        </div>

                        {/* Best Practices */}
                        <div className="rounded-global border border-border-base bg-bg-app/50 p-5">
                            <div className="flex items-center gap-2 mb-3">
                                <span className="text-yellow-400 text-lg">★</span>
                                <span className="text-lg font-semibold text-text-main">Best Practices</span>
                            </div>
                            <ul className="text-base text-text-muted space-y-3">
                                <li className="flex items-start gap-3">
                                    <span className="text-primary mt-1.5">•</span>
                                    <span>Keep Tier 1 focused on <span className="text-text-main">1-2 stats at most</span>, and additionally stats that are exclusive to heroes such as Wave, Mead Strength, and so on.</span>
                                </li>
                                <li className="flex items-start gap-3">
                                    <span className="text-primary mt-1.5">•</span>
                                    <span>If making a full Commander or Castellan, try to list <span className="text-text-main">every stat in Tier 2</span> that you want exactly at least 1 occurrence of in the random stats of the base equipment pieces.</span>
                                </li>
                                <li className="flex items-start gap-3">
                                    <span className="text-primary mt-1.5">•</span>
                                    <span>You may <span className="text-text-main">drag to reorder</span> the priorities.</span>
                                </li>
                            </ul>
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
