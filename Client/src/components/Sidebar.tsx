import React from 'react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/Navigation';
import { ThemeToggle } from './ThemeToggle';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onViewChange,
}) => {
  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');

  const openView = (viewId: ViewId) => {
    onViewChange(viewId);
  };

  const renderItem = (item: (typeof NAVIGATION_ITEMS)[number]) => (
    <button
      type="button"
      key={item.id}
      className={`liquid-nav-item group ${currentView === item.id ? 'liquid-nav-item-active' : ''}`}
      aria-current={currentView === item.id ? 'page' : undefined}
      onClick={() => openView(item.id)}
      title={item.label}
    >
      <span className="liquid-nav-icon">
        {item.icon}
      </span>
      <span className="liquid-nav-label">{item.label}</span>
      <span className="liquid-nav-active-indicator" aria-hidden="true" />
    </button>
  );

  return (
    <aside className="liquid-sidebar transition-colors duration-300" aria-label="Application navigation">
      <div className="liquid-sidebar-main-island">
        <div className="liquid-sidebar-toolbar">
          <div className="liquid-sidebar-toolbar-copy">
            <span>Command center</span>
            <strong>Workspace</strong>
          </div>
          <ThemeToggle className="liquid-sidebar-theme-toggle !p-0" />
        </div>
        <nav className="liquid-sidebar-scroll custom-scrollbar" aria-label="Primary">
          <div className="liquid-section-label">
            <span className="liquid-sidebar-section-title">Operations</span>
          </div>
          <div className="liquid-nav-list">
            {mainItems.map(renderItem)}
          </div>
        </nav>

        <div className="liquid-sidebar-system">
          <div className="liquid-section-label">
            <span className="liquid-sidebar-section-title">System</span>
          </div>
          <nav className="liquid-nav-list" aria-label="System">
            {systemItems.map(renderItem)}
          </nav>
        </div>
        <div className="liquid-sidebar-footer">
          <span className="liquid-sidebar-footer-dot" aria-hidden="true" />
          <span>Local desktop control</span>
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;
