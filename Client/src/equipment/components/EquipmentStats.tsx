import React, { useMemo } from 'react';

import { statDisplayName, commanderStatGroups, castellanStatGroups, type CommStat, type CastStat } from '../models/equipment';

interface EquipmentStatsProps {
  equipmentMode: 'Commander' | 'Castellan';
  combatMode: 'PvP' | 'PvE';
  selectedItem: CommStat | CastStat | null;
}

const EquipmentStats: React.FC<EquipmentStatsProps> = ({ equipmentMode, combatMode, selectedItem }) => {
  const stats = selectedItem;
  const name = selectedItem?.name;

  const processedStats = useMemo(() => {
    if (!stats) return {};

    const newStats: { [key: string]: number } = {};
    const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;
    const allKeys = Object.values(statGroups).flat();

    for (const key of allKeys) {
      const isSpecialStat = ['glory', 'later', 'fire', 'early'].includes(key);
      let baseKey = key;
      if (isSpecialStat) {
        baseKey = combatMode === 'PvP' ? `CL${key.charAt(0).toUpperCase() + key.slice(1)}` : `NPC${key.charAt(0).toUpperCase() + key.slice(1)}`;
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

        // Skip adding CL/NPC stats to frontCbtStr/flankCbtStr since they go to limits now
        if (key === 'frontCbtStr' || key === 'flankCbtStr') {
          // Do nothing, these don't get CL/NPC additions anymore
        } else {
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
  }, [stats, combatMode, equipmentMode]);

  const renderStat = (statKey: string, value: number) => {
    const label = statDisplayName[statKey] || statKey;
    return (
      <div className="flex items-center justify-between py-1.5 px-2 rounded hover:bg-white/5 transition-colors" key={statKey}>
        <span className="text-sm text-gray-400">{label}</span>
        <span className="text-sm font-mono font-medium text-primary">{value.toFixed(2)}</span>
      </div>
    );
  };

  const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;

  return (
    <div className="p-4">
      {/* Header */}
      <div className="mb-4">
        <h3 className="text-lg font-semibold text-white">
          {name ? name : `Select a ${equipmentMode}`}
        </h3>
        {name && (
          <span className={`
            inline-flex items-center px-2 py-0.5 rounded-global text-xs font-medium mt-1
            ${combatMode === 'PvP'
              ? 'bg-red-500/10 text-red-400 border border-red-500/20'
              : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'
            }
          `}>
            {combatMode} Stats
          </span>
        )}
      </div>

      {/* Stats List */}
      <div className="space-y-4">
        {!stats && (
          <div className="text-center py-8 text-gray-500">
            <p>No stats available for this selection.</p>
          </div>
        )}

        {stats && Object.entries(statGroups).map(([groupName, statKeys]) => {
          const visibleStats = statKeys.filter(key => {
            const value = processedStats[key];
            return !(value === undefined || value === 0);
          });

          if (visibleStats.length === 0) return null;

          return (
            <div key={groupName} className="rounded-global bg-dark-bg/50 p-3 border border-dark-border/50">
              <h4 className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2">{groupName}</h4>
              <div className="space-y-0.5">
                {visibleStats.map(key => renderStat(key, processedStats[key]))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default EquipmentStats;
