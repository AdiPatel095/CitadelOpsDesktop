import React, { useState, useEffect } from 'react';

import { useEquipment } from '../context/EquipmentContext';
import { useAuth } from '../../context/AuthContext';
import EquipmentSelection, { type EquipmentMode, type CombatMode } from './EquipmentSelection';
import StatPriority from './StatPriority';
import { type CommStat, type CastStat } from '../models/equipment';

const EquipmentView: React.FC = () => {
  const { equipmentData } = useEquipment();
  const { credits, hardwareID } = useAuth();
  const [equipmentMode, setEquipmentMode] = useState<EquipmentMode>('Commander');
  const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);

  // Keep full arrays - filtering happens only during rendering
  const getFullArray = (): (CommStat | CastStat | null)[] => {
    if (equipmentMode === 'Commander') {
      return equipmentData.commStats;
    }
    if (equipmentMode === 'Castellan') {
      // Use castStats (index-based array) to preserve proper order (0-7)
      return equipmentData.castStats;
    }
    return [];
  };

  const fullArray = getFullArray();
  // selectedIndex is the actual 0-based array index
  const selectedItem = selectedIndex !== null ? fullArray[selectedIndex] : null;

  // Find first non-null item for default selection
  useEffect(() => {
    if (selectedIndex === null || fullArray[selectedIndex] === null) {
      const firstValidIndex = fullArray.findIndex(item => item !== null);
      if (firstValidIndex !== -1) {
        setSelectedIndex(firstValidIndex);
      } else {
        setSelectedIndex(null);
      }
    }
  }, [fullArray, selectedIndex]);

  return (
    <div className="flex gap-6 h-[calc(100vh-8rem)]">
      {/* Left Panel - Selection */}
      <div className="flex-1 min-w-0">
        <EquipmentSelection
          equipmentMode={equipmentMode}
          setEquipmentMode={setEquipmentMode}
          combatMode={combatMode}
          setCombatMode={setCombatMode}
          selectedIndex={selectedIndex}
          setSelectedIndex={setSelectedIndex}
          fullArray={fullArray}
          selectedItem={selectedItem}
        />
      </div>

      {/* Right Panel - Stat Priority */}
      <div className="w-80 flex-shrink-0">
        <StatPriority
          equipmentMode={equipmentMode}
          combatMode={combatMode}
          credits={credits}
          hardwareID={hardwareID}
          selectedIndex={selectedIndex}
        />
      </div>
    </div>
  );
};

export default EquipmentView;
