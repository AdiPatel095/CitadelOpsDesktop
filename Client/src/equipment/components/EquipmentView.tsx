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

  const getSelectionItems = (): (CommStat | CastStat)[] => {
    if (equipmentMode === 'Commander') {
      return equipmentData.commStats.filter(c => c !== null && c.id !== 0);
    }
    if (equipmentMode === 'Castellan') {
      return equipmentData.castellanStats;
    }
    return [];
  };

  const selectionItems = getSelectionItems();
  const selectedItem = selectedIndex !== null ? selectionItems[selectedIndex] : null;

  useEffect(() => {
    if (selectionItems.length > 0 && selectedIndex === null) {
      setSelectedIndex(0);
    } else if (selectionItems.length === 0) {
      setSelectedIndex(null);
    }
  }, [selectionItems, selectedIndex]);

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
          selectionItems={selectionItems}
          selectedItem={selectedItem}
        />
      </div>

      {/* Right Panel - Stat Priority */}
      <div className="w-80 flex-shrink-0">
        <StatPriority
          equipmentMode={equipmentMode}
          credits={credits}
          hardwareID={hardwareID}
        />
      </div>
    </div>
  );
};

export default EquipmentView;
