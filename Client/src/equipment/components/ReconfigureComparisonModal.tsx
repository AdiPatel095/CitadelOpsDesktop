import React, { useMemo } from 'react';
import ReactDOM from 'react-dom';
import { Icons } from '../../components/Icons';
import { type CommStat, statDisplayName, commanderStatGroups, statGroupDisplayName } from '../models/equipment';
import { FrontendWebsocket } from '../../websocket';

interface ReconfigureComparisonModalProps {
    isOpen: boolean;
    onClose: () => void;
    currentLoadout: CommStat | null;
    newLoadout: CommStat | null;
    targetIndex: number;
    combatMode: 'PvP' | 'PvE';
}

// Process stats to combine base stats with CL/NPC stats based on combat mode
const processStats = (stats: CommStat, combatMode: 'PvP' | 'PvE'): { [key: string]: number } => {
    const newStats: { [key: string]: number } = {};
    const allKeys = Object.values(commanderStatGroups).flat();

    for (const key of allKeys) {
        const isSpecialStat = ['glory', 'later', 'fire', 'early'].includes(key);
        let baseKey = key;
        if (isSpecialStat) {
            baseKey = combatMode === 'PvP'
                ? `CL${key.charAt(0).toUpperCase() + key.slice(1)}`
                : `NPC${key.charAt(0).toUpperCase() + key.slice(1)}`;
        }

        let finalValue = (stats as any)[baseKey] || 0;

        if (!isSpecialStat) {
            let suffix = key;

            // Special handling for Front/Flank limits
            if (key === 'frontLimit') {
                suffix = 'Front';
            } else if (key === 'flankLimit') {
                suffix = 'Flank';
            } else if (key.endsWith('CbtStr')) {
                suffix = key.replace('CbtStr', '');
            } else if (key.endsWith('Str')) {
                suffix = key.replace('Str', '');
            }

            const capitalizedSuffix = suffix.charAt(0).toUpperCase() + suffix.slice(1);

            // Skip adding CL/NPC stats to frontCbtStr/flankCbtStr
            if (key !== 'frontCbtStr' && key !== 'flankCbtStr') {
                if (combatMode === 'PvP') {
                    const clKey = `CL${capitalizedSuffix}`;
                    if ((stats as any)[clKey]) {
                        finalValue += (stats as any)[clKey];
                    }
                } else { // PvE
                    const npcKey = `NPC${capitalizedSuffix}`;
                    if ((stats as any)[npcKey]) {
                        finalValue += (stats as any)[npcKey];
                    }
                }
            }
        }

        newStats[key] = finalValue;
    }

    return newStats;
};

interface StatRowProps {
    statKey: string;
    currentValue: number;
    newValue: number;
}

const StatRow: React.FC<StatRowProps> = ({ statKey, currentValue, newValue }) => {
    const diff = newValue - currentValue;
    const hasChange = diff !== 0;

    return (
        <div className={`flex items-center py-1.5 px-2 rounded ${hasChange ? 'bg-dark-bg/30' : ''}`}>
            <span className="flex-1 text-sm text-gray-400 truncate">
                {statDisplayName[statKey] || statKey}
            </span>
            <span className="w-16 text-right text-sm text-gray-500">
                {currentValue.toFixed(1)}
            </span>
            <span className="w-8 text-center text-gray-600">→</span>
            <span className={`w-16 text-right text-sm font-medium ${diff > 0 ? 'text-green-400' : diff < 0 ? 'text-red-400' : 'text-gray-400'
                }`}>
                {newValue.toFixed(1)}
            </span>
            <span className={`w-16 text-right text-xs ${diff > 0 ? 'text-green-500' : diff < 0 ? 'text-red-500' : 'text-gray-600'
                }`}>
                {diff > 0 ? `+${diff.toFixed(1)}` : diff < 0 ? diff.toFixed(1) : '-'}
            </span>
        </div>
    );
};

interface StatSectionProps {
    title: string;
    stats: string[];
    currentProcessed: { [key: string]: number };
    newProcessed: { [key: string]: number };
}

const StatSection: React.FC<StatSectionProps> = ({ title, stats, currentProcessed, newProcessed }) => {
    const hasAnyChange = stats.some(stat => {
        const current = currentProcessed[stat] || 0;
        const next = newProcessed[stat] || 0;
        return current !== next;
    });

    return (
        <div className="mb-4">
            <h4 className={`text-sm font-semibold mb-2 ${hasAnyChange ? 'text-primary' : 'text-gray-500'}`}>
                {title}
                {hasAnyChange && <span className="ml-2 text-xs text-primary/60">• Changes</span>}
            </h4>
            <div className="space-y-0.5">
                {stats.map(stat => (
                    <StatRow
                        key={stat}
                        statKey={stat}
                        currentValue={currentProcessed[stat] || 0}
                        newValue={newProcessed[stat] || 0}
                    />
                ))}
            </div>
        </div>
    );
};

const ReconfigureComparisonModal: React.FC<ReconfigureComparisonModalProps> = ({
    isOpen,
    onClose,
    currentLoadout,
    newLoadout,
    targetIndex,
    combatMode
}) => {
    // Process stats with useMemo to combine base + CL/NPC based on combat mode
    const currentProcessed = useMemo(() => {
        if (!currentLoadout) return {};
        return processStats(currentLoadout, combatMode);
    }, [currentLoadout, combatMode]);

    const newProcessed = useMemo(() => {
        if (!newLoadout) return {};
        return processStats(newLoadout, combatMode);
    }, [newLoadout, combatMode]);

    if (!isOpen || !currentLoadout || !newLoadout) return null;

    const handleConfirm = () => {
        FrontendWebsocket.sendConfirmReconfigure(targetIndex, currentLoadout, newLoadout);
        onClose();
    };

    // Use portal to render modal at document body level
    return ReactDOM.createPortal(
        <div className="fixed inset-0 z-[9999] flex items-center justify-center">
            {/* Backdrop */}
            <div
                className="absolute inset-0 bg-black/70 backdrop-blur-sm"
                onClick={onClose}
            />

            {/* Modal */}
            <div className="relative glass-panel max-w-2xl w-full mx-4 max-h-[85vh] flex flex-col animate-fade-in">
                {/* Header */}
                <div className="flex items-center justify-between p-4 border-b border-dark-border">
                    <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-full bg-primary/20 flex items-center justify-center">
                            <Icons.RefreshCw className="w-5 h-5 text-primary" />
                        </div>
                        <div>
                            <h3 className="text-lg font-bold text-white">Loadout Comparison</h3>
                            <p className="text-sm text-gray-400">
                                {currentLoadout.name || `Commander ${targetIndex + 1}`}
                                <span className={`ml-2 px-1.5 py-0.5 text-xs rounded ${combatMode === 'PvP'
                                    ? 'bg-red-500/20 text-red-400'
                                    : 'bg-blue-500/20 text-blue-400'
                                    }`}>
                                    {combatMode}
                                </span>
                            </p>
                        </div>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-2 rounded-global hover:bg-dark-bg/50 text-gray-400 hover:text-white transition-colors"
                    >
                        <Icons.X className="w-5 h-5" />
                    </button>
                </div>

                {/* Column Headers */}
                <div className="flex items-center px-6 py-2 bg-dark-bg/50 border-b border-dark-border/50 text-xs font-medium uppercase tracking-wider text-gray-500">
                    <span className="flex-1">Stat</span>
                    <span className="w-16 text-right">Current</span>
                    <span className="w-8"></span>
                    <span className="w-16 text-right">New</span>
                    <span className="w-16 text-right">Change</span>
                </div>

                {/* Scrollable Content - Uses commanderStatGroups with section titles */}
                <div className="flex-1 overflow-y-auto p-4">
                    {Object.entries(commanderStatGroups).map(([groupName, statKeys]) => (
                        <StatSection
                            key={groupName}
                            title={statGroupDisplayName[groupName] || groupName}
                            stats={statKeys}
                            currentProcessed={currentProcessed}
                            newProcessed={newProcessed}
                        />
                    ))}
                </div>

                {/* Footer with Buttons */}
                <div className="flex gap-3 p-4 border-t border-dark-border">
                    <button
                        onClick={onClose}
                        className="flex-1 btn-ghost border border-dark-border"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleConfirm}
                        className="flex-1 px-4 py-2 font-semibold rounded-global bg-primary text-dark-bg hover:bg-primary/80 active:scale-95 transition-all duration-200 flex items-center justify-center gap-2"
                    >
                        <Icons.Check className="w-4 h-4" />
                        Confirm Reconfiguration
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default ReconfigureComparisonModal;
