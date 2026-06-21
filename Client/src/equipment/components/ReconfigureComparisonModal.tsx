import React, { useMemo } from 'react';
import { Icons } from '../../components/Icons';
import {
    type CommStat,
    type CastStat,
    displayStatName,
    commanderStatGroups,
    castellanStatGroups,
    statGroupDisplayName,
    processEquipmentStats,
    formatEquipmentStatValue,
} from '../models/Equipment';
import { FrontendWebsocket } from '../../Websocket';
import { Modal, Button, Badge } from '../../components/ui';

interface ReconfigureComparisonModalProps {
    isOpen: boolean;
    onClose: () => void;
    currentLoadout: CommStat | CastStat | null;
    newLoadout: CommStat | CastStat | null;
    targetIndex: number;
    combatMode: 'PvP' | 'PvE';
    equipmentMode: 'Commander' | 'Castellan';
}

const processStats = (stats: CommStat | CastStat, combatMode: 'PvP' | 'PvE', equipmentMode: 'Commander' | 'Castellan'): { [key: string]: number } => {
    const processed = processEquipmentStats(stats, combatMode, equipmentMode);
    return Object.fromEntries(
        Object.entries(processed).map(([key, stat]) => [key, stat.value])
    );
};

interface StatRowProps {
    statKey: string;
    currentValue: number;
    newValue: number;
    equipmentMode: 'Commander' | 'Castellan';
}

const StatRow: React.FC<StatRowProps> = ({ statKey, currentValue, newValue, equipmentMode }) => {
    const diff = newValue - currentValue;
    const hasChange = diff !== 0;

    return (
        <div className={`flex items-center py-2 px-3 rounded-lg border ${hasChange ? 'bg-bg-app/80 border-border-base' : 'border-transparent'}`}>
            <span className="flex-1 text-sm text-text-muted truncate font-medium">
                {displayStatName(statKey, { equipmentMode })}
            </span>
            <span className="w-16 text-right text-sm text-text-muted opacity-75 font-mono">
                {formatEquipmentStatValue(statKey, currentValue)}
            </span>
            <span className="w-8 text-center text-text-muted opacity-50">→</span>
            <span className={`w-16 text-right text-sm font-bold font-mono ${diff > 0 ? 'text-success' : diff < 0 ? 'text-error' : 'text-text-main'
                }`}>
                {formatEquipmentStatValue(statKey, newValue)}
            </span>
            <span className={`w-16 text-right text-xs font-mono font-medium ${diff > 0 ? 'text-success' : diff < 0 ? 'text-error' : 'text-text-muted'
                }`}>
                {diff !== 0 ? formatEquipmentStatValue(statKey, diff) : '-'}
            </span>
        </div>
    );
};

interface StatSectionProps {
    title: string;
    stats: string[];
    equipmentMode: 'Commander' | 'Castellan';
    currentProcessed: { [key: string]: number };
    newProcessed: { [key: string]: number };
}

const StatSection: React.FC<StatSectionProps> = ({ title, stats, equipmentMode, currentProcessed, newProcessed }) => {
    const hasAnyChange = stats.some(stat => {
        const current = currentProcessed[stat] || 0;
        const next = newProcessed[stat] || 0;
        return current !== next;
    });

    if (!hasAnyChange) return null; // We can optionally hide unchanged sections entirely

    return (
        <div className="mb-4 bg-bg-card-hover/40 p-3 rounded-global border border-border-base">
            <h4 className={`text-xs font-bold uppercase tracking-wider mb-2 ${hasAnyChange ? 'text-primary' : 'text-text-muted'}`}>
                {title}
            </h4>
            <div className="space-y-1">
                {stats.map(stat => (
                    <StatRow
                        key={stat}
                        statKey={stat}
                        currentValue={currentProcessed[stat] || 0}
                        newValue={newProcessed[stat] || 0}
                        equipmentMode={equipmentMode}
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
    combatMode,
    equipmentMode
}) => {
    const currentProcessed = useMemo(() => {
        if (!currentLoadout) return {};
        return processStats(currentLoadout, combatMode, equipmentMode);
    }, [currentLoadout, combatMode, equipmentMode]);

    const newProcessed = useMemo(() => {
        if (!newLoadout) return {};
        return processStats(newLoadout, combatMode, equipmentMode);
    }, [newLoadout, combatMode, equipmentMode]);

    const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;

    if (!isOpen || !currentLoadout || !newLoadout) return null;

    const handleConfirm = () => {
        FrontendWebsocket.sendConfirmReconfigure(targetIndex, currentLoadout, newLoadout, equipmentMode);
        onClose();
    };

    return (
        <Modal
            isOpen={isOpen}
            onClose={onClose}
            maxWidth="2xl"
            title={
                <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-primary/20 flex items-center justify-center shrink-0">
                        <Icons.RefreshCw className="w-5 h-5 text-primary" />
                    </div>
                    <div className="flex flex-col">
                        <span>Loadout Comparison</span>
                        <div className="flex items-center gap-2 mt-1">
                            <span className="text-xs text-text-muted font-medium uppercase tracking-wider">
                                {currentLoadout.name || `${equipmentMode} ${targetIndex + 1}`}
                            </span>
                            <Badge variant={combatMode === 'PvP' ? 'danger' : 'info'}>
                                {combatMode}
                            </Badge>
                        </div>
                    </div>
                </div>
            }
            footer={
                <>
                    <Button variant="ghost" onClick={onClose} className="flex-1">
                        Cancel
                    </Button>
                    <Button variant="primary" onClick={handleConfirm} className="flex-1" leftIcon={<Icons.Check className="w-4 h-4" />}>
                        Confirm Reconfiguration
                    </Button>
                </>
            }
        >
            <div className="flex items-center px-6 py-2 bg-bg-card border-b border-border-base text-[10px] font-bold uppercase tracking-wider text-text-muted sticky top-0 z-10 -mx-6 -mt-6 mb-4">
                <span className="flex-1">Stat</span>
                <span className="w-16 text-right">Current</span>
                <span className="w-8"></span>
                <span className="w-16 text-right">New</span>
                <span className="w-16 text-right">Change</span>
            </div>

            <div className="flex flex-col">
                {Object.entries(statGroups).map(([groupName, statKeys]) => (
                    <StatSection
                        key={groupName}
                        title={statGroupDisplayName[groupName] || groupName}
                        stats={statKeys}
                        equipmentMode={equipmentMode}
                        currentProcessed={currentProcessed}
                        newProcessed={newProcessed}
                    />
                ))}
            </div>
        </Modal>
    );
};

export default ReconfigureComparisonModal;
