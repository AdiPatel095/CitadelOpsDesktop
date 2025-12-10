import React, { useState } from 'react';


const LogsPanel: React.FC = () => {
  const [isOpen, setIsOpen] = useState(false);

  const togglePanel = () => {
    setIsOpen(!isOpen);
  };

  return (
    <div className={`logs-panel ${isOpen ? 'open' : 'closed'}`}>
      <div className="panel-header" onClick={togglePanel}>
        <h3>Logs</h3>
        {isOpen && (
          <button className="minimize-button" onClick={(e) => { e.stopPropagation(); setIsOpen(false); }}>
            &#x2715;
          </button>
        )}
      </div>
      {isOpen && (
        <div className="panel-content">
          <p>Log entry 1...</p>
          <p>Log entry 2...</p>
          <p>Log entry 3...</p>
        </div>
      )}
    </div>
  );
};

export default LogsPanel;
