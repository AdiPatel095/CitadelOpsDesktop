import React, { useState, useEffect } from 'react';
import './EquipmentView.css';
import { useEquipment } from '../context/EquipmentContext';
import EquipmentSelection, { type EquipmentMode, type CombatMode } from './EquipmentSelection';
import StatPriority from './StatPriority';

const EquipmentView: React.FC = () => {
  const { equipmentData } = useEquipment();
  const [equipmentMode, setEquipmentMode] = useState<EquipmentMode>('Commander');
  const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
  const [selectedName, setSelectedName] = useState<string>('');

  const getSelectionItems = () => {
    if (equipmentMode === 'Commander') {
      return equipmentData.commStats.map(c => c.name).filter(Boolean);
    }
    if (equipmentData.castStats) {
      return Object.values(equipmentData.castStats).map(c => c.name).filter(Boolean);
    }
    return [];
  };

  const selectionItems = getSelectionItems();

  useEffect(() => {
    if (selectionItems.length > 0 && !selectionItems.includes(selectedName)) {
      setSelectedName(selectionItems[0]);
    } else if (selectionItems.length === 0) {
      setSelectedName('');
    }
  }, [selectionItems, selectedName]);

  return (
    <div className="equipment-view">
      <EquipmentSelection
        equipmentMode={equipmentMode}
        setEquipmentMode={setEquipmentMode}
        combatMode={combatMode}
        setCombatMode={setCombatMode}
        selectedName={selectedName}
        setSelectedName={setSelectedName}
        selectionItems={selectionItems}
      />
      <StatPriority />
    </div>
  );
};

export default EquipmentView;
