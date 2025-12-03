import React, { useState } from 'react';
import './Header.css';
import logo from '../assets/citadel-ops-logo.svg';

type ControlStatus = 'In control' | 'Idle';

const Header: React.FC = () => {
  const [controlStatus, setControlStatus] = useState<ControlStatus>('Idle');

  const handleStart = () => {
    setControlStatus('In control');
    // TODO: Add logic to start the process
  };

  const handleStop = () => {
    setControlStatus('Idle');
    // TODO: Add logic to stop the process
  };

  return (
    <header className="game-header">
      <div className="header-left">
        <img src={logo} alt="Citadel Ops Logo" className="logo" />
        <h1 className="site-title">Citadel Ops Desktop</h1>
      </div>
      <div className="header-center">
        {/* <button className="control-button" onClick={handleStart}>Start</button>
        <button className="control-button" onClick={handleStop}>Stop</button>
        <div className={`status-indicator ${controlStatus.replace(' ', '-').toLowerCase()}`}></div>
        <span className="status-text">{controlStatus}</span> */}
      </div>
      <div className="header-right">
        {/* <button className="settings-button">Settings</button>
        <a href="https://example.com" target="_blank" rel="noopener noreferrer" className="profile-link">
          <div className="profile-picture-placeholder"></div>
          <span>Online Profile</span>
        </a> */}
      </div>
    </header>
  );
};

export default Header;
