import React, {type JSX} from 'react';
import './Sidebar.css';
import { type View } from '../App';
import { useResources } from '../currency/context/ResourceContext.tsx';

import CoinsIcon from '../../assets/Coins.png';
import RubyIcon from '../../assets/Ruby.png';

// Placeholder Icon
const PlaceholderIcon = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10"></circle>
    <line x1="12" y1="8" x2="12" y2="12"></line>
    <line x1="12" y1="16" x2="12" y2="16"></line>
  </svg>
);

const DashboardIcon = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
    <line x1="3" y1="9" x2="21" y2="9"></line>
    <line x1="9" y1="21" x2="9" y2="9"></line>
  </svg>
);

const CurrencyIcon = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="8"></circle>
    <line x1="12" y1="16" x2="12" y2="12"></line>
    <line x1="12" y1="8" x2="12" y2="8"></line>
  </svg>
);

// const TroopsIcon = () => (
//   <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
//     <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path>
//     <circle cx="9" cy="7" r="4"></circle>
//     <path d="M23 21v-2a4 4 0 0 0-3-3.87"></path>
//     <path d="M16 3.13a4 4 0 0 1 0 7.75"></path>
//   </svg>
// );
//
// const ScheduleIcon = () => (
//   <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
//     <rect x="3" y="4" width="18" height="18" rx="2" ry="2"></rect>
//     <line x1="16" y1="2" x2="16" y2="6"></line>
//     <line x1="8" y1="2" x2="8" y2="6"></line>
//     <line x1="3" y1="10" x2="21" y2="10"></line>
//   </svg>
// );

const EquipmentIcon = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"></path>
  </svg>
);

interface SidebarProps {
  isCollapsed: boolean;
  activeView: View;
  setActiveView: (view: View) => void;
  toggleSidebar: () => void;
}

const formatValue = (value: number | string) => {
  if (typeof value === 'number') {
    return value.toLocaleString();
  }
  return value;
};

const Sidebar: React.FC<SidebarProps> = ({ isCollapsed, activeView, setActiveView, toggleSidebar }) => {
  const { resources } = useResources();

  const views: { name: View; icon: JSX.Element }[] = [
    { name: 'Dashboard', icon: <DashboardIcon /> },
    { name: 'Currency', icon: <CurrencyIcon /> },
    { name: 'Equipment', icon: <EquipmentIcon /> },
    // { name: 'Troops', icon: <TroopsIcon /> },
    // { name: 'Schedule', icon: <ScheduleIcon /> },
  ];

  const resourceItems = [
    { name: 'Coins', value: resources?.coins ?? 'N/A', icon: CoinsIcon },
    { name: 'Rubies', value: resources?.rubies ?? 'N/A', icon: RubyIcon },
    { name: 'Feathers', value: 'N/A', icon: null },
    { name: 'Total Attack Count', value: 'N/A', icon: null },
  ];

  return (
    <nav className={`sidebar ${isCollapsed ? 'collapsed' : ''}`}>
      <div>
        <ul className="sidebar-nav">
          {views.map(({ name, icon }) => (
            <li 
              key={name} 
              className={`sidebar-item ${activeView === name ? 'active' : ''}`}
              onClick={() => setActiveView(name)}
            >
              <a href="#" className="sidebar-link">
                {icon}
                <span className="link-text">{name}</span>
              </a>
            </li>
          ))}
        </ul>
        <div className="resource-list">
          <hr className="divider" />
          {resourceItems.map(({ name, value, icon }) => (
            <div key={name} className="resource-item">
              <div className="resource-icon">
                {icon ? <img src={icon} alt={name} /> : <PlaceholderIcon />}
              </div>
              <div className="resource-details">
                <span className="resource-name">{name}</span>
                <span className="resource-value">{formatValue(value)}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
      <button onClick={toggleSidebar} className="sidebar-toggle">
        {isCollapsed ? '>' : '<'}
      </button>
    </nav>
  );
};

export default Sidebar;
