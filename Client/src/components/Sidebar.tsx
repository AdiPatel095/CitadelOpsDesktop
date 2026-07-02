import React, { useState } from 'react';
import { Settings, ChevronDown, ChevronRight } from 'lucide-react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/Navigation';
import { ThemeToggle } from './ThemeToggle';
import { Button } from './ui';
import { useAuth } from '../context/AuthContext';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
  onOpenRecruitTroopsSettings: () => void;
  onOpenAutoToolSettings: () => void;
  onOpenAutoTCISettings: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onViewChange,
  onOpenRecruitTroopsSettings,
  onOpenAutoToolSettings,
  onOpenAutoTCISettings,
}) => {
  const {
    autoTCIEnabled,
    toggleAutoTCI,
    recruitTroopsEnabled,
    toggleRecruitTroops,
    autoToolEnabled,
    toggleAutoTool,
    gameLoggedIn
  } = useAuth();

  const featureRowClass = 'liquid-feature-row';
  const featureToggleClass = 'liquid-feature-toggle';
  const featureIconClass = 'liquid-feature-icon';

  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');
  const isSystemView = systemItems.some(item => item.id === currentView);

  const [expandedSections, setExpandedSections] = useState({
    features: true,
    mainMenu: true
  });

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections(prev => ({
      ...prev,
      [section]: !prev[section]
    }));
  };

  return (
    <aside className="liquid-sidebar transition-colors duration-300">
      <div className="liquid-sidebar-main-island">
        <div className="liquid-sidebar-scroll custom-scrollbar">
          {/* Features */}
          <div className="mb-4">
            <div
              className="liquid-section-label mb-2"
              onClick={() => toggleSection('features')}
            >
              <span className="liquid-sidebar-section-title">Features</span>
              {expandedSections.features ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
            </div>
            {expandedSections.features && (
              <div className="px-2 space-y-2">
                <div className={featureRowClass}>
                  <Button
                    variant="outline"
                    size="sm"
                    className={`${featureToggleClass} ${
                      autoTCIEnabled
                        ? '!border-success/40 !text-success hover:!bg-success/10'
                        : '!border-error/40 !text-error hover:!bg-error/10'
                    }`}
                    onClick={() => toggleAutoTCI()}
                    title={
                      gameLoggedIn
                        ? 'Toggle Auto TCI (temporary construction items)'
                        : 'Last known Auto TCI status while bot is disconnected'
                    }
                    leftIcon={
                      <div className={`w-1.5 h-1.5 rounded-full ${autoTCIEnabled ? 'bg-success animate-pulse' : 'bg-error'}`} />
                    }
                  >
                    <span className="liquid-feature-toggle-label">Auto TCI</span>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className={`${featureIconClass} ${autoTCIEnabled ? 'text-warning hover:bg-warning/10' : 'text-text-muted hover:bg-bg-card-hover'}`}
                    onClick={onOpenAutoTCISettings}
                    title="Auto TCI settings"
                  >
                    <Settings className="w-4 h-4" />
                  </Button>
                </div>
                <div className={featureRowClass}>
                  <Button
                    variant="outline"
                    size="sm"
                    className={`${featureToggleClass} ${
                      recruitTroopsEnabled
                        ? '!border-success/40 !text-success hover:!bg-success/10'
                        : '!border-error/40 !text-error hover:!bg-error/10'
                    }`}
                    onClick={() => toggleRecruitTroops()}
                    title={
                      gameLoggedIn
                        ? 'Toggle Auto Recruit'
                        : 'Last known Auto Recruit status while bot is disconnected'
                    }
                    leftIcon={
                      <div className={`w-1.5 h-1.5 rounded-full ${recruitTroopsEnabled ? 'bg-success animate-pulse' : 'bg-error'}`} />
                    }
                  >
                    <span className="liquid-feature-toggle-label">Auto Recruit</span>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className={`${featureIconClass} ${recruitTroopsEnabled ? 'text-warning hover:bg-warning/10' : 'text-text-muted hover:bg-bg-card-hover'}`}
                    onClick={onOpenRecruitTroopsSettings}
                    title="Auto Recruit settings"
                  >
                    <Settings className="w-4 h-4" />
                  </Button>
                </div>
                <div className={featureRowClass}>
                  <Button
                    variant="outline"
                    size="sm"
                    className={`${featureToggleClass} ${
                      autoToolEnabled
                        ? '!border-success/40 !text-success hover:!bg-success/10'
                        : '!border-error/40 !text-error hover:!bg-error/10'
                    }`}
                    onClick={() => toggleAutoTool()}
                    title={
                      gameLoggedIn
                        ? 'Toggle Auto Tool'
                        : 'Last known Auto Tool status while bot is disconnected'
                    }
                    leftIcon={
                      <div className={`w-1.5 h-1.5 rounded-full ${autoToolEnabled ? 'bg-success animate-pulse' : 'bg-error'}`} />
                    }
                  >
                    <span className="liquid-feature-toggle-label">Auto Tool</span>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className={`${featureIconClass} ${autoToolEnabled ? 'text-warning hover:bg-warning/10' : 'text-text-muted hover:bg-bg-card-hover'}`}
                    onClick={onOpenAutoToolSettings}
                    title="Auto Tool settings"
                  >
                    <Settings className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            )}
          </div>

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
                    onClick={() => onViewChange(item.id)}
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

      <div className={`liquid-sidebar-system-island ${isSystemView ? 'liquid-sidebar-system-island-active' : ''}`}>
        <div
          className="liquid-section-label liquid-system-section-label"
        >
          <span className="liquid-sidebar-section-title">System</span>
          <ChevronDown className="liquid-system-chevron w-3 h-3" />
        </div>

        <div className="liquid-system-items">
          {systemItems.map((item) => (
            <div
              key={item.id}
              className={`liquid-nav-item group ${currentView === item.id ? 'liquid-nav-item-active' : ''}`}
              onClick={() => onViewChange(item.id)}
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
