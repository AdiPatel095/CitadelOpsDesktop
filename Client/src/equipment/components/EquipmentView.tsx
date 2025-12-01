import React, { useState, useEffect } from 'react';
import './EquipmentView.css';
import EquipmentStats from './EquipmentStats';
import { useEquipment } from '../context/EquipmentContext';

type EquipmentMode = 'Commander' | 'Castellan';
type CombatMode = 'PvP' | 'PvE';

const EquipmentView: React.FC = () => {
  const { equipmentData } = useEquipment();
  const [equipmentMode, setEquipmentMode] = useState<EquipmentMode>('Commander');
  const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
  const [selectedName, setSelectedName] = useState<string>('');

  const getSelectionItems = () => {
    if (equipmentMode === 'Commander') {
      return []; // To be implemented later, as commActuals is not available
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
      {/* Left Half */}
      <div className="equipment-panel left-panel">
        <div className="controls-header">
          <div className="switch-container">
            <button 
              className={`switch-btn ${equipmentMode === 'Commander' ? 'active' : ''}`}
              onClick={() => setEquipmentMode('Commander')}
            >
              Commander
            </button>
            <button 
              className={`switch-btn ${equipmentMode === 'Castellan' ? 'active' : ''}`}
              onClick={() => setEquipmentMode('Castellan')}
            >
              Castellan
            </button>
          </div>
          <div className="switch-container">
            <button 
              className={`switch-btn ${combatMode === 'PvP' ? 'active' : ''}`}
              onClick={() => setCombatMode('PvP')}
            >
              PvP
            </button>
            <button 
              className={`switch-btn ${combatMode === 'PvE' ? 'active' : ''}`}
              onClick={() => setCombatMode('PvE')}
            >
              PvE
            </button>
          </div>
        </div>

        <div className="selection-container">
          <div className="selection-sidebar">
            {selectionItems.map(name => (
              <div 
                key={name} 
                className={`selection-item ${selectedName === name ? 'active' : ''}`}
                onClick={() => setSelectedName(name)}
              >
                {name}
              </div>
            ))}
          </div>
          <div className="stats-display">
            <EquipmentStats 
              equipmentMode={equipmentMode}
              combatMode={combatMode}
              selectedName={selectedName}
            />
          </div>
        </div>
      </div>

      {/* Right Half */}
      <div className="equipment-panel right-panel">
        <h3>Stat Priority</h3>
        <div className="stat-priority-list">
          <p>Stat priority settings will go here...</p>
        </div>
        <button className="reconfigure-btn">Reconfigure Loadout</button>
      </div>
    </div>
  );
};

export default EquipmentView;
