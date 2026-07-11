import React, { useState, useMemo, useRef, useEffect } from 'react';
import { Icons } from '../../components/Icons';
import { type CommStat, displayStatName, commanderStatGroups, castellanStatGroups, statGroupDisplayName } from '../models/Equipment';
import { FrontendWebsocket } from '../../Websocket';
import ReconfigureComparisonModal from './ReconfigureComparisonModal';
import { Button, Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

interface StatPriorityProps {
    equipmentMode: 'Commander' | 'Castellan';
    combatMode: 'PvP' | 'PvE';
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
    equipmentMode: 'Commander' | 'Castellan';
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
    equipmentMode,
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
            case 1: return { color: 'error', bg: 'error', label: 'Max Stat' };
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
            <div className={`px-3 py-2 border-b border-border-base/30 flex items-center gap-2`}>
                <span className={`w-5 h-5 rounded flex items-center justify-center text-xs font-bold bg-${style.bg}/20 text-${style.color}`}>
                    {tier}
                </span>
                <span className="text-xs font-medium text-text-muted">
                    {style.label}
                </span>
            </div>

            <div className="p-2 min-h-[50px]">
                {stats.length === 0 ? (
                    <div className="text-center py-2 text-text-muted text-xs font-medium">
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
                                    <span className={`w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold bg-${style.bg}/10 text-${style.color}`}>
                                        {index + 1}
                                    </span>
                                    <span className="text-sm text-text-muted flex-1 truncate">
                                        {displayStatName(stat, { equipmentMode })}
                                    </span>
                                    <button
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            onRemove(stat);
                                        }}
                                        className="p-1 rounded hover:bg-error/20 text-text-muted hover:text-error transition-colors"
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
    selectedIndex
}) => {
    const [tier1Stats, setTier1Stats] = useState<string[]>([]);
    const [tier2Stats, setTier2Stats] = useState<string[]>([]);
    const [showAddDropdown, setShowAddDropdown] = useState(false);
    const [dragState, setDragState] = useState<DragState | null>(null);
    const [dropTarget, setDropTarget] = useState<{ tier: TierType; index: number } | null>(null);
    const [isReconfiguring, setIsReconfiguring] = useState(false);
    const dropdownRef = useRef<HTMLDivElement>(null);
    const [showComparisonModal, setShowComparisonModal] = useState(false);
    const [comparisonData, setComparisonData] = useState<ComparisonData | null>(null);
    const [showInfoModal, setShowInfoModal] = useState(false);

    const totalStats = tier1Stats.length + tier2Stats.length;

    const allStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        return Object.values(groups).flat();
    }, [equipmentMode]);

    const availableStats = useMemo(() => {
        const usedStats = [...tier1Stats, ...tier2Stats];
        return allStats.filter(stat => !usedStats.includes(stat));
    }, [allStats, tier1Stats, tier2Stats]);

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

    const getTierState = (tier: TierType) => {
        switch (tier) {
            case 1: return { stats: tier1Stats, setStats: setTier1Stats };
            case 2: return { stats: tier2Stats, setStats: setTier2Stats };
        }
    };

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
                const { stats, setStats } = getTierState(fromTier);
                const newStats = [...stats];
                if (fromIndex !== toIndex) {
                    newStats.splice(fromIndex, 1);
                    newStats.splice(toIndex, 0, stat);
                    setStats(newStats);
                }
            } else {
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

    const handleDrop = (_e: React.DragEvent, _tier: TierType) => {};

    const addStat = (stat: string) => {
        setTier1Stats([...tier1Stats, stat]);
    };

    const removeStat = (stat: string) => {
        setTier1Stats(tier1Stats.filter(s => s !== stat));
        setTier2Stats(tier2Stats.filter(s => s !== stat));
    };

    const handleReconfigure = async () => {
        if (totalStats === 0 || selectedIndex === null) return;

        setIsReconfiguring(true);
        try {
            const reconfigurePayload = {
                equipmentMode: equipmentMode,
                combatMode: combatMode,
                targetIndex: selectedIndex,
                stats: [
                    ...tier1Stats.map((stat, index) => ({ stat, tier: 1, position: index })),
                    ...tier2Stats.map((stat, index) => ({ stat, tier: 2, position: index }))
                ]
            };

            FrontendWebsocket.sendReconfigureLoadout(reconfigurePayload);
        } catch (error) {
            FrontendWebsocket.showAlert('red', 'An unexpected error occurred');
            setIsReconfiguring(false);
        }
    };

    React.useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setShowAddDropdown(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    React.useEffect(() => {
        setTier1Stats([]);
        const defaultStats = equipmentMode === 'Commander'
            ? commanderStatGroups.core
            : castellanStatGroups.core;
        setTier2Stats(defaultStats);
    }, [equipmentMode]);

    useEffect(() => {
        const handleMessage = (message: any) => {
            if (message.type === 'reconfigureComparison' && message.payload) {
                setComparisonData({
                    currentLoadout: message.payload.currentLoadout,
                    newLoadout: message.payload.newLoadout,
                    targetIndex: message.payload.targetIndex,
                    equipmentMode: equipmentMode
                });
                setShowComparisonModal(true);
                setIsReconfiguring(false);
            } else if (message.type === 'reconfigureError') {
                setIsReconfiguring(false);
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);
        return () => FrontendWebsocket.removeMessageListener(handleMessage);
    }, []);

    return (
        <Card className="liquid-prominent-header-card h-full flex flex-col relative min-h-0">
            <CardHeader className="liquid-card-header-prominent flex items-center justify-between gap-2">
                <div className="flex flex-col">
                  <h3 className="text-base font-semibold text-text-main flex items-center gap-2">
                      <Icons.Activity className="w-4 h-4 text-primary" />
                      Stat Priority
                      <button
                          onClick={() => setShowInfoModal(true)}
                          className="p-1 rounded-full hover:bg-primary/20 text-text-muted hover:text-primary transition-colors"
                          title="Learn about stat priorities"
                      >
                          <Icons.Info className="w-3.5 h-3.5" />
                      </button>
                  </h3>
                  <p className="text-xs text-text-muted mt-0.5">Drag between tiers to reorder</p>
                </div>

                <div className="flex items-center gap-2">
                    <div className="relative" ref={dropdownRef}>
                        <Button
                            size="icon"
                            variant="outline"
                            onClick={() => setShowAddDropdown(!showAddDropdown)}
                            disabled={availableStats.length === 0}
                            title="Add Stat"
                            className="w-8 h-8 rounded-full"
                        >
                            <Icons.Plus className="w-4 h-4" />
                        </Button>

                        {showAddDropdown && availableStats.length > 0 && (
                            <div className="rounded-global absolute top-full right-0 mt-2 w-56 bg-bg-card border border-border-base shadow-xl max-h-64 overflow-y-auto z-50">
                                {Object.entries(groupedAvailableStats).map(([groupName, stats]) => (
                                    <div key={groupName}>
                                        <div className="px-3 py-2 text-[10px] font-bold text-text-muted uppercase tracking-wider bg-bg-card/95 sticky top-0 backdrop-blur-sm border-b border-border-base/50">
                                            {statGroupDisplayName[groupName] || groupName}
                                        </div>
                                        {stats.map(stat => (
                                            <button
                                                key={stat}
                                                onClick={() => addStat(stat)}
                                                className="w-full text-left px-3 py-2.5 text-sm font-medium text-text-main hover:bg-primary/10 hover:text-primary transition-colors"
                                            >
                                                {displayStatName(stat, { equipmentMode })}
                                            </button>
                                        ))}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            </CardHeader>

            <CardContent className="liquid-prominent-header-content flex-1 overflow-y-auto custom-scrollbar p-3 space-y-3">
                <TierList
                    tier={1}
                    stats={tier1Stats}
                    equipmentMode={equipmentMode}
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
                    equipmentMode={equipmentMode}
                    dragState={dragState}
                    dropTarget={dropTarget}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnd={handleDragEnd}
                    onDrop={handleDrop}
                    onRemove={removeStat}
                />
            </CardContent>

            <div className="p-3 border-t border-border-base bg-bg-card-hover/50 rounded-b-[calc(var(--radius-global)-1px)] shrink-0">
                <Button
                    onClick={handleReconfigure}
                    disabled={isReconfiguring || totalStats === 0}
                    className="w-full"
                    leftIcon={<Icons.RefreshCw className={`w-4 h-4 ${isReconfiguring ? 'animate-spin' : ''}`} />}
                >
                    {isReconfiguring ? 'Reconfiguring...' : `Reconfigure ${equipmentMode}`}
                </Button>
            </div>

            {showInfoModal && (
                <div className="absolute top-12 right-4 z-50 w-[500px] max-w-[90vw] bg-bg-card border border-border-base rounded-global shadow-2xl animate-fade-in flex flex-col max-h-[80vh]">
                    <div className="absolute -top-2 right-6 w-4 h-4 bg-bg-card border-l border-t border-border-base rotate-45" />

                    <div className="flex items-center justify-between p-4 border-b border-border-base shrink-0">
                        <h4 className="text-base font-semibold text-text-main flex items-center gap-2">
                            <Icons.Info className="w-4 h-4 text-primary" />
                            Understanding Stat Priority
                        </h4>
                        <button
                            onClick={() => setShowInfoModal(false)}
                            className="p-1 rounded hover:bg-error/20 text-text-muted hover:text-error transition-colors"
                        >
                            <Icons.X className="w-5 h-5" />
                        </button>
                    </div>

                    <div className="p-5 space-y-5 overflow-y-auto custom-scrollbar flex-1">
                        <div className="rounded-global border border-error/30 bg-error/5 p-4">
                            <div className="flex items-center gap-2 mb-2">
                                <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-error/20 text-error">1</span>
                                <span className="text-sm font-semibold text-error">Max Stat</span>
                            </div>
                            <p className="text-sm text-text-muted leading-relaxed">
                                These stats will be <span className="text-error font-medium">maximized to their limits</span> as much as possible.
                            </p>
                        </div>

                        <div className="rounded-global border border-primary/30 bg-primary/5 p-4">
                            <div className="flex items-center gap-2 mb-2">
                                <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-primary/20 text-primary">2</span>
                                <span className="text-sm font-semibold text-primary">Have in Random Slots</span>
                            </div>
                            <p className="text-sm text-text-muted leading-relaxed mb-2">
                                The bot will try to have exactly <span className="text-primary font-medium">1 copy of each stat in random slots</span> from this list.
                            </p>
                            <p className="text-sm text-text-muted leading-relaxed">
                                Relic 2.0 gear gets a total of <span className="text-text-main font-medium">9 random stats</span>. If you have less than 9 stats in this list, the bot will pick a combo to have 1 copy of all stats listed here in the random slots of the base equipment and then any leftover random stats will double up with (very likely) your highest priority stat.
                            </p>
                        </div>

                        <div className="rounded-global border border-border-base bg-bg-app/50 p-4">
                            <div className="flex items-center gap-2 mb-2">
                                <span className="text-warning text-base">★</span>
                                <span className="text-sm font-semibold text-text-main">Best Practices</span>
                            </div>
                            <ul className="text-sm text-text-muted space-y-2">
                                <li className="flex items-start gap-2">
                                    <span className="text-primary font-bold mt-0.5">•</span>
                                    <span>Keep Tier 1 focused on <span className="text-text-main font-medium">1-2 stats at most</span>, and additionally stats that are exclusive to heroes such as Wave, Mead Strength, and so on.</span>
                                </li>
                                <li className="flex items-start gap-2">
                                    <span className="text-primary font-bold mt-0.5">•</span>
                                    <span>If making a full Commander or Castellan, try to list <span className="text-text-main font-medium">every stat in Tier 2</span> that you want exactly at least 1 occurrence of in the random stats of the base equipment pieces.</span>
                                </li>
                                <li className="flex items-start gap-2">
                                    <span className="text-primary font-bold mt-0.5">•</span>
                                    <span>You may <span className="text-text-main font-medium">drag to reorder</span> the priorities.</span>
                                </li>
                            </ul>
                        </div>
                    </div>
                </div>
            )}

            <ReconfigureComparisonModal
                isOpen={showComparisonModal}
                onClose={() => setShowComparisonModal(false)}
                currentLoadout={comparisonData?.currentLoadout ?? null}
                newLoadout={comparisonData?.newLoadout ?? null}
                targetIndex={comparisonData?.targetIndex ?? 0}
                combatMode={combatMode}
                equipmentMode={equipmentMode}
            />
        </Card>
    );
};

export default StatPriority;
