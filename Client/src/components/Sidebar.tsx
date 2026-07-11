import React, { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
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
  const isSystemView = systemItems.some(item => item.id === currentView);
  const [systemMenuOpen, setSystemMenuOpen] = useState(isSystemView);

  const [expandedSections, setExpandedSections] = useState({
    mainMenu: true
  });

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections(prev => ({
      ...prev,
      [section]: !prev[section]
    }));
  };

  const systemMenuExpanded = isSystemView || systemMenuOpen;

  const openView = (viewId: ViewId) => {
    onViewChange(viewId);
    if (systemItems.some(item => item.id === viewId)) setSystemMenuOpen(true);
  };

  const handleNavigationKey = (event: React.KeyboardEvent, viewId: ViewId) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    openView(viewId);
  };

  return (
    <aside className="liquid-sidebar transition-colors duration-300">
      <div className="liquid-sidebar-main-island">
        <div className="liquid-sidebar-scroll custom-scrollbar">
          {/* Main Navigation */}
          <div className="mb-4">
            <div
              className="liquid-section-label mb-2"
              onClick={() => toggleSection('mainMenu')}
            >
              <span className="liquid-sidebar-section-title">Main Menu</span>
              {expandedSections.mainMenu ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
            </div>
            {expandedSections.mainMenu && (
              <div className="space-y-1">
                {mainItems.map((item) => (
                  <div
                    key={item.id}
                    className={`liquid-nav-item group ${currentView === item.id ? 'liquid-nav-item-active' : ''}`}
                    role="button"
                    tabIndex={0}
                    onClick={() => openView(item.id)}
                    onKeyDown={(event) => handleNavigationKey(event, item.id)}
                    title={item.label}
                  >
                    <span className={`liquid-nav-icon transition-colors duration-200 ${currentView === item.id ? 'text-primary' : 'text-text-muted group-hover:text-text-main'}`}>
                      {item.icon}
                    </span>
                    <span className="liquid-nav-label text-sm font-medium">{item.label}</span>
                    {currentView === item.id && (
                      <div className="ml-auto w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_8px_rgba(52,211,153,0.8)]" />
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={`liquid-sidebar-system-island ${systemMenuExpanded ? 'liquid-sidebar-system-island-active' : ''}`}>
        <div
          className="liquid-section-label liquid-system-section-label"
          role="button"
          tabIndex={0}
          aria-expanded={systemMenuExpanded}
          onClick={() => setSystemMenuOpen(open => !open)}
          onKeyDown={(event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return;
            event.preventDefault();
            setSystemMenuOpen(open => !open);
          }}
        >
          <span className="liquid-sidebar-section-title">System</span>
          <ChevronDown className="liquid-system-chevron w-3 h-3" />
        </div>

        <div className="liquid-system-items">
          {systemItems.map((item) => (
            <div
              key={item.id}
              className={`liquid-nav-item group ${currentView === item.id ? 'liquid-nav-item-active' : ''}`}
              role="button"
              tabIndex={0}
              onClick={() => openView(item.id)}
              onKeyDown={(event) => handleNavigationKey(event, item.id)}
              title={item.label}
            >
              <span className={`liquid-nav-icon transition-colors duration-200 ${currentView === item.id ? 'text-primary' : 'text-text-muted group-hover:text-text-main'}`}>
                {item.icon}
              </span>
              <span className="liquid-nav-label text-sm font-medium">{item.label}</span>
              {currentView === item.id && (
                <div className="ml-auto w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_8px_rgba(52,211,153,0.8)]" />
              )}
            </div>
          ))}
        </div>
      </div>

      <ThemeToggle className="liquid-sidebar-theme-toggle !p-0" />
    </aside>
  );
};

export default Sidebar;
