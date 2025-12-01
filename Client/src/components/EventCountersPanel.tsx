import React, { useState } from 'react';
import './EventCountersPanel.css';

const EventCountersPanel: React.FC = () => {
  const [isOpen, setIsOpen] = useState(false);

  const togglePanel = () => {
    setIsOpen(!isOpen);
  };

  return (
    <div className={`event-counters-panel ${isOpen ? 'open' : 'closed'}`}>
      <div className="panel-header" onClick={togglePanel}>
        <h3>Event Counters</h3>
        {isOpen && (
          <button className="minimize-button" onClick={(e) => { e.stopPropagation(); setIsOpen(false); }}>
            &#x2715;
          </button>
        )}
      </div>
      {isOpen && (
        <div className="panel-content">
          <p>Event Counter 1...</p>
          <p>Event Counter 2...</p>
          <p>Event Counter 3...</p>
        </div>
      )}
    </div>
  );
};

export default EventCountersPanel;
