import React from 'react';
import { ChevronDown, X } from 'lucide-react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/Navigation';
import { ThemeToggle } from './ThemeToggle';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
  open: boolean;
  onClose: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onViewChange,
  open,
  onClose,
}) => {
  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');
  const systemHasActiveView = systemItems.some(item => item.id === currentView);

  const openView = (viewId: ViewId) => {
    onViewChange(viewId);
    onClose();
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
    <>
    <button
      type="button"
      className={`liquid-sidebar-scrim ${open ? 'liquid-sidebar-scrim-visible' : ''}`}
      onClick={onClose}
      aria-label="Close workspace navigation"
      tabIndex={open ? 0 : -1}
    />
    <aside
      id="workspace-navigation"
      className={`liquid-sidebar transition-colors duration-300 ${open ? 'liquid-sidebar-mobile-open' : ''}`}
      aria-label="Application navigation"
      aria-hidden={!open ? undefined : false}
    >
      <div className="liquid-sidebar-main-island">
        <div className="liquid-sidebar-toolbar">
          <div className="liquid-sidebar-toolbar-copy">
            <span>Command center</span>
            <strong>Workspace</strong>
          </div>
          <button
            type="button"
            className="liquid-sidebar-mobile-close"
            onClick={onClose}
            aria-label="Close workspace navigation"
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <nav className="liquid-sidebar-scroll custom-scrollbar" aria-label="Primary">
          <div className="liquid-section-label">
            <span className="liquid-sidebar-section-title">Operations</span>
          </div>
          <div className="liquid-nav-list">
            {mainItems.map(renderItem)}
          </div>
        </nav>

      </div>

      <div className="liquid-sidebar-system-row">
        <div className={`liquid-sidebar-system-island ${systemHasActiveView ? 'liquid-sidebar-system-island-active' : ''}`}>
          <div className="liquid-system-section-label liquid-section-label">
            <span className="liquid-sidebar-section-title">System</span>
            <ChevronDown className="liquid-system-chevron h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          </div>
          <nav className="liquid-system-items liquid-nav-list" aria-label="System">
            {systemItems.map(renderItem)}
          </nav>
        </div>
        <ThemeToggle className="liquid-sidebar-system-toggle" />
      </div>
    </aside>
    </>
  );
};

export default Sidebar;
