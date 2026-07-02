import React, { useState, useEffect } from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { useEquipment } from '../context/EquipmentContext';
import EquipmentSelection, { type EquipmentMode, type CombatMode } from './EquipmentSelection';
import StatPriority from './StatPriority';
import { type CommStat, type CastStat } from '../models/Equipment';

const EquipmentView: React.FC = () => {
  const { equipmentData } = useEquipment();
  const [equipmentMode, setEquipmentMode] = useState<EquipmentMode>('Commander');
  const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);

  // Keep full arrays - filtering happens only during rendering
  const getFullArray = (): (CommStat | CastStat | null)[] => {
    if (equipmentMode === 'Commander') {
      return equipmentData.commStats;
    }
    if (equipmentMode === 'Castellan') {
      // Use castStats (index-based array) to preserve proper order (0-10)
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
    <div className="equipment-view-shell">
      <StaleSessionBanner />
      <div className="equipment-layout">
        {/* Left Panel - Selection */}
        <div className="equipment-main-panel">
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
        <div className="equipment-priority-panel">
          <StatPriority
            equipmentMode={equipmentMode}
            combatMode={combatMode}
            selectedIndex={selectedIndex}
          />
        </div>
      </div>
    </div>
  );
};

export default EquipmentView;
