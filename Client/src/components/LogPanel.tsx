import React, { useState } from 'react';
import './LogPanel.css';

type ActiveTab = 'logs' | 'eventCounters';

const LogPanel: React.FC = () => {
  const [isOpen, setIsOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<ActiveTab>('logs');

  const togglePanel = (openState: boolean) => {
    setIsOpen(openState);
  };

  const setTab = (tab: ActiveTab) => {
    setActiveTab(tab);
    if (!isOpen) {
      setIsOpen(true);
    }
  };

  return (
    <div className={`log-panel ${isOpen ? 'open' : 'closed'}`}>
      <div className="log-panel-header">
        <div 
          className={`tab ${activeTab === 'logs' ? 'active' : ''}`} 
          onClick={() => setTab('logs')}
        >
          <h3>Logs</h3>
        </div>
        <div 
          className={`tab ${activeTab === 'eventCounters' ? 'active' : ''}`} 
          onClick={() => setTab('eventCounters')}
        >
          <h3>Event Counters</h3>
        </div>
      </div>
      {isOpen && (
        <button className="minimize-button" onClick={() => togglePanel(false)}>
          &#x2715; {/* A simple 'X' character for minimize */}
        </button>
      )}
      <div className="log-panel-content">
        {activeTab === 'logs' && (
          <div>
            <p>Log entry 1...</p>
            <p>Log entry 2...</p>
            <p>Log entry 3...</p>
          </div>
        )}
        {activeTab === 'eventCounters' && (
          <div>
            <p>Event Counter 1...</p>
            <p>Event Counter 2...</p>
            <p>Event Counter 3...</p>
          </div>
        )}
      </div>
    </div>
  );
};

export default LogPanel;
