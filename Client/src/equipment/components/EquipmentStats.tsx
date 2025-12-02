import React, { useMemo } from 'react';
import { useEquipment } from '../context/EquipmentContext';
import './EquipmentStats.css';
import { statDisplayName, commanderStatGroups, castellanStatGroups, type CommStat, type CastStat } from '../models/equipment';

interface EquipmentStatsProps {
  equipmentMode: 'Commander' | 'Castellan';
  combatMode: 'PvP' | 'PvE';
  selectedName: string;
}

const EquipmentStats: React.FC<EquipmentStatsProps> = ({ equipmentMode, combatMode, selectedName }) => {
  const { equipmentData } = useEquipment();

  const { stats, name } = useMemo(() => {
    let stats: CommStat | CastStat | undefined;
    let name: string | undefined;

    if (equipmentMode === 'Commander') {
      stats = equipmentData.commStats.find(c => c.name === selectedName);
      if (stats) {
        name = stats.name;
      }
    } else { // Castellan
      if (equipmentData.castStats) {
        stats = Object.values(equipmentData.castStats).find(c => c.name === selectedName);
        if (stats) {
          name = stats.name;
        }
      }
    }
    return { stats, name };
  }, [equipmentData, equipmentMode, selectedName]);

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
        let isLimitStat = false;

        // Special handling for Front/Flank limits
        if (key === 'frontLimit') {
          suffix = 'Front';
          isLimitStat = true;
        } else if (key === 'flankLimit') {
          suffix = 'Flank';
          isLimitStat = true;
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
      <div className="stat-row" key={statKey}>
        <span className="stat-label">{label}</span>
        <span className="stat-value">{value.toFixed(2)}</span>
      </div>
    );
  };

  const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;

  return (
    <div className="stats-display-container">
      <h4>{name ? `${name} - ${combatMode} Stats` : `Select a ${equipmentMode}`}</h4>
      <div className="stats-list">
        {!stats && <p>No stats available for this selection.</p>}
        {stats && Object.entries(statGroups).map(([groupName, statKeys]) => (
          <div key={groupName} className="stat-group">
            <h5>{groupName}</h5>
            {statKeys.map(key => {
              const value = processedStats[key];
              if (value === undefined || value === 0) return null;
              return renderStat(key, value);
            }).filter(Boolean)}
          </div>
        ))}
      </div>
    </div>
  );
};

export default EquipmentStats;
