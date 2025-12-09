import React, { useState, useMemo, useRef } from 'react';
import { Icons } from '../../components/Icons';
import { statDisplayName, commanderStatGroups, castellanStatGroups } from '../models/equipment';
import { LicenseService } from '../../services/LicenseService';

const RECONFIGURE_COST = 10000;

interface StatPriorityProps {
    equipmentMode: 'Commander' | 'Castellan';
    credits: number;
    hardwareID: string | null;
}

interface DraggableItemProps {
    stat: string;
    index: number;
    onDragStart: (e: React.DragEvent, index: number) => void;
    onDragOver: (e: React.DragEvent, index: number) => void;
    onDragEnd: () => void;
    onRemove: (stat: string) => void;
    isDragging: boolean;
    dragOverIndex: number | null;
}

const DraggableItem: React.FC<DraggableItemProps> = ({
    stat,
    index,
    onDragStart,
    onDragOver,
    onDragEnd,
    onRemove,
    isDragging,
    dragOverIndex,
}) => {
    const isDropTarget = dragOverIndex === index;

    return (
        <div
            draggable
            onDragStart={(e) => onDragStart(e, index)}
            onDragOver={(e) => onDragOver(e, index)}
            onDragEnd={onDragEnd}
            className={`
        flex items-center gap-3 p-3 bg-dark-bg/50 border rounded-lg 
        transition-all duration-200 cursor-grab active:cursor-grabbing
        ${isDragging ? 'opacity-50 scale-95' : ''}
        ${isDropTarget ? 'border-primary shadow-lg shadow-primary/20 translate-y-1' : 'border-dark-border/50 hover:border-primary/30'}
      `}
        >
            <span className="w-6 h-6 rounded-full bg-primary/10 flex items-center justify-center text-xs font-bold text-primary">
                {index + 1}
            </span>
            <span className="text-sm font-medium text-gray-300 flex-1">
                {statDisplayName[stat] || stat}
            </span>
            <div className="flex items-center gap-2">
                <button
                    onClick={(e) => {
                        e.stopPropagation();
                        onRemove(stat);
                    }}
                    className="p-1 rounded hover:bg-red-500/20 text-gray-500 hover:text-red-400 transition-colors"
                >
                    <Icons.X className="w-4 h-4" />
                </button>
                <Icons.GripVertical className="w-4 h-4 text-gray-500" />
            </div>
        </div>
    );
};

const StatPriority: React.FC<StatPriorityProps> = ({
    equipmentMode,
    credits,
    hardwareID
}) => {
    const [priorityStats, setPriorityStats] = useState<string[]>([]);
    const [showAddDropdown, setShowAddDropdown] = useState(false);
    const [draggedIndex, setDraggedIndex] = useState<number | null>(null);
    const [dragOverIndex, setDragOverIndex] = useState<number | null>(null);
    const [isReconfiguring, setIsReconfiguring] = useState(false);
    const [reconfigureError, setReconfigureError] = useState<string | null>(null);
    const dropdownRef = useRef<HTMLDivElement>(null);

    const hasEnoughCredits = credits >= RECONFIGURE_COST;

    // Get available stats based on equipment mode
    const allStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;
        return Object.values(groups).flat();
    }, [equipmentMode]);

    // Stats not yet in the priority list
    const availableStats = useMemo(() => {
        return allStats.filter(stat => !priorityStats.includes(stat));
    }, [allStats, priorityStats]);

    // Group available stats by category for the dropdown
    const groupedAvailableStats = useMemo(() => {
        const groups = equipmentMode === 'Commander'
            ? commanderStatGroups
            : castellanStatGroups;

        const result: { [key: string]: string[] } = {};
        for (const [groupName, stats] of Object.entries(groups)) {
            const available = stats.filter(stat => !priorityStats.includes(stat));
            if (available.length > 0) {
                result[groupName] = available;
            }
        }
        return result;
    }, [equipmentMode, priorityStats]);

    // Drag handlers
    const handleDragStart = (e: React.DragEvent, index: number) => {
        setDraggedIndex(index);
        e.dataTransfer.effectAllowed = 'move';
    };

    const handleDragOver = (e: React.DragEvent, index: number) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        if (draggedIndex !== null && draggedIndex !== index) {
            setDragOverIndex(index);
        }
    };

    const handleDragEnd = () => {
        if (draggedIndex !== null && dragOverIndex !== null && draggedIndex !== dragOverIndex) {
            const newStats = [...priorityStats];
            const [removed] = newStats.splice(draggedIndex, 1);
            newStats.splice(dragOverIndex, 0, removed);
            setPriorityStats(newStats);
        }
        setDraggedIndex(null);
        setDragOverIndex(null);
    };

    // Add/Remove handlers
    const addStat = (stat: string) => {
        setPriorityStats([...priorityStats, stat]);
        setShowAddDropdown(false);
    };

    const removeStat = (stat: string) => {
        setPriorityStats(priorityStats.filter(s => s !== stat));
    };

    // Reconfigure handler
    const handleReconfigure = async () => {
        if (!hasEnoughCredits || !hardwareID || priorityStats.length === 0) return;

        setIsReconfiguring(true);
        setReconfigureError(null);

        try {
            const response = await LicenseService.reconfigureLoadout(
                hardwareID,
                equipmentMode,
                priorityStats
            );

            if (!response.success) {
                setReconfigureError(response.message || 'Failed to reconfigure');
            }
            // Credits will be updated automatically via WebSocket
        } catch (error) {
            setReconfigureError('An unexpected error occurred');
        } finally {
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
        setPriorityStats([]);
        setReconfigureError(null);
    }, [equipmentMode]);

    return (
        <div className="glass-panel h-full flex flex-col">
            {/* Header with Add Stat Button */}
            <div className="p-4 border-b border-dark-border">
                <div className="flex items-center justify-between gap-2">
                    <h3 className="text-lg font-semibold text-white flex items-center gap-2">
                        <Icons.Activity className="w-5 h-5 text-primary" />
                        Stat Priority
                    </h3>
                    {/* Add Stat Button - Now in Header */}
                    <div className="relative" ref={dropdownRef}>
                        <button
                            onClick={() => setShowAddDropdown(!showAddDropdown)}
                            disabled={availableStats.length === 0}
                            className="p-2 rounded-lg bg-primary/10 hover:bg-primary/20 text-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            title="Add Stat"
                        >
                            <Icons.Plus className="w-4 h-4" />
                        </button>

                        {/* Dropdown */}
                        {showAddDropdown && availableStats.length > 0 && (
                            <div className="absolute top-full right-0 mt-2 w-56 bg-dark-bg border border-dark-border rounded-lg shadow-xl max-h-64 overflow-y-auto z-50">
                                {Object.entries(groupedAvailableStats).map(([groupName, stats]) => (
                                    <div key={groupName}>
                                        <div className="px-3 py-2 text-xs font-bold text-gray-500 uppercase tracking-wider bg-dark-bg/80 sticky top-0">
                                            {groupName}
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
                <p className="text-xs text-gray-500 mt-1">Drag to reorder priorities</p>
            </div>

            {/* Priority List */}
            <div className="flex-1 overflow-y-auto p-4">
                <div className="space-y-2">
                    {priorityStats.length === 0 ? (
                        <div className="text-center py-8 text-gray-500">
                            <Icons.List className="w-8 h-8 mx-auto mb-2 opacity-50" />
                            <p className="text-sm">No stats prioritized yet</p>
                            <p className="text-xs mt-1">Add stats to set your priority order</p>
                        </div>
                    ) : (
                        priorityStats.map((stat, index) => (
                            <DraggableItem
                                key={stat}
                                stat={stat}
                                index={index}
                                onDragStart={handleDragStart}
                                onDragOver={handleDragOver}
                                onDragEnd={handleDragEnd}
                                onRemove={removeStat}
                                isDragging={draggedIndex === index}
                                dragOverIndex={dragOverIndex}
                            />
                        ))
                    )}
                </div>
            </div>

            {/* Reconfigure Button */}
            <div className="p-4 border-t border-dark-border">
                {reconfigureError && (
                    <div className="mb-3 p-2 bg-red-500/10 border border-red-500/30 rounded-lg">
                        <p className="text-xs text-red-400">{reconfigureError}</p>
                    </div>
                )}
                <button
                    onClick={handleReconfigure}
                    disabled={!hasEnoughCredits || isReconfiguring || priorityStats.length === 0}
                    className={`
                        w-full flex items-center justify-center gap-2 py-3 px-4 rounded-lg font-medium transition-all duration-200
                        ${hasEnoughCredits && priorityStats.length > 0
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
                </button>
                <div className="mt-2 text-center">
                    <span className={`text-xs ${hasEnoughCredits ? 'text-gray-500' : 'text-red-400'}`}>
                        Cost: {RECONFIGURE_COST.toLocaleString()} credits
                        {!hasEnoughCredits && ` (You have ${credits.toLocaleString()})`}
                    </span>
                </div>
            </div>
        </div>
    );
};

export default StatPriority;
