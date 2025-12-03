import React from 'react';
import './EquipmentView.css';

const StatPriority: React.FC = () => {
    return (
        <div className="equipment-panel right-panel">
            <h3>Stat Priority</h3>
            <div className="stat-priority-list">
                <p>Stat priority settings will go here...</p>
            </div>
            <button className="reconfigure-btn">Reconfigure Loadout</button>
        </div>
    );
};

export default StatPriority;
