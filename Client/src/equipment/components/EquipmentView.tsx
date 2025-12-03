import React, { useState, useEffect } from 'react';
import './EquipmentView.css';
import { useEquipment } from '../context/EquipmentContext';
import EquipmentSelection, { type EquipmentMode, type CombatMode } from './EquipmentSelection';
import StatPriority from './StatPriority';
import { type CommStat, type CastStat } from '../models/equipment';

const EquipmentView: React.FC = () => {
  const { equipmentData } = useEquipment();
  const [equipmentMode, setEquipmentMode] = useState<EquipmentMode>('Commander');
  const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);

  const getSelectionItems = (): (CommStat | CastStat)[] => {
    if (equipmentMode === 'Commander') {
      return equipmentData.commStats;
    }
    if (equipmentMode === 'Castellan' && equipmentData.castStats) {
      return Object.values(equipmentData.castStats)
        .filter(c => c.name && c.name.toLowerCase() === 'castellan');
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
    <div className="equipment-view">
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
      <StatPriority />
    </div>
  );
};

export default EquipmentView;
