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
  onOpenAutoTCISettings: () => void;
  onOpenAutoBeriWorldSettings: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onViewChange,
  onOpenRecruitTroopsSettings,
  onOpenAutoTCISettings,
  onOpenAutoBeriWorldSettings,
}) => {
  const { autoTCIEnabled, toggleAutoTCI, autoBeriWorldEnabled, toggleAutoBeriWorld, gameLoggedIn } = useAuth();

  const [expandedSections, setExpandedSections] = useState({
    features: true,
    mainMenu: true,
    system: true
  });

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections(prev => ({
      ...prev,
      [section]: !prev[section]
    }));
  };

  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');

  return (
    <aside className="w-64 bg-bg-card border-r border-border-base flex flex-col pt-20 pb-6 h-screen fixed left-0 top-0 z-40 transition-colors duration-300">
      <div className="flex-1 overflow-y-auto hidden-scrollbar">
        {/* Features */}
        <div className="px-4 mb-4">
          <div
            className="flex items-center justify-between text-[10px] font-bold text-text-muted uppercase tracking-widest mb-2 px-3 cursor-pointer hover:text-text-main transition-colors"
            onClick={() => toggleSection('features')}
          >
            <span>Features</span>
            {expandedSections.features ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          </div>
          {expandedSections.features && (
            <div className="px-2 space-y-2">
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className={`flex-1 h-9 border-2 text-xs uppercase tracking-wider ${
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
                  Auto TCI {autoTCIEnabled ? 'on' : 'off'}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className={`h-9 w-9 shrink-0 ${autoTCIEnabled ? 'text-amber-500 hover:bg-amber-500/10' : 'text-text-muted hover:bg-bg-card-hover'}`}
                  onClick={onOpenAutoTCISettings}
                  title="Auto TCI settings"
                >
                  <Settings className="w-4 h-4" />
                </Button>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className={`flex-1 h-9 border-2 text-xs uppercase tracking-wider ${
                    autoBeriWorldEnabled
                      ? '!border-success/40 !text-success hover:!bg-success/10'
                      : '!border-error/40 !text-error hover:!bg-error/10'
                  }`}
                  onClick={() => toggleAutoBeriWorld()}
                  title={
                    gameLoggedIn
                      ? 'Toggle Auto Beri World (Berimond troop transfer)'
                      : 'Last known Auto Beri World status while bot is disconnected'
                  }
                  leftIcon={
                    <div className={`w-1.5 h-1.5 rounded-full ${autoBeriWorldEnabled ? 'bg-success animate-pulse' : 'bg-error'}`} />
                  }
                >
                  Auto Beri {autoBeriWorldEnabled ? 'on' : 'off'}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className={`h-9 w-9 shrink-0 ${autoBeriWorldEnabled ? 'text-amber-500 hover:bg-amber-500/10' : 'text-text-muted hover:bg-bg-card-hover'}`}
                  onClick={onOpenAutoBeriWorldSettings}
                  title="Auto Beri World settings"
                >
                  <Settings className="w-4 h-4" />
                </Button>
              </div>
              <div className="flex gap-2">
                <Button
                  disabled
                  variant="secondary"
                  className="flex-1 text-xs uppercase tracking-wider h-9"
                  leftIcon={<div className="w-1.5 h-1.5 rounded-full bg-text-muted/30" />}
                >
                  Recruit: SOON
                </Button>
                <Button
                  disabled
                  variant="secondary"
                  size="icon"
                  className="h-9 w-9"
                  title="Recruit Troops Settings (Coming Soon)"
                >
                  <Settings className="w-4 h-4" />
                </Button>
              </div>
            </div>
          )}
        </div>

        {/* Main Navigation */}
        <div className="px-4 mb-4">
          <div
            className="flex items-center justify-between text-[10px] font-bold text-text-muted uppercase tracking-widest mb-2 px-3 cursor-pointer hover:text-text-main transition-colors"
            onClick={() => toggleSection('mainMenu')}
          >
            <span>Main Menu</span>
            {expandedSections.mainMenu ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          </div>
          {expandedSections.mainMenu && (
            <div className="space-y-1">
              {mainItems.map((item) => (
                <div
                  key={item.id}
                  className={`
                    rounded-global flex items-center gap-3 px-3 py-2.5 cursor-pointer transition-all duration-200 group
                    ${currentView === item.id
                      ? 'bg-primary/10 text-primary shadow-[0_0_15px_rgba(52,211,153,0.1)]'
                      : 'text-text-muted hover:text-text-main hover:bg-bg-card-hover'}
                  `}
                  onClick={() => onViewChange(item.id)}
                >
                  <span className={`transition-colors duration-200 ${currentView === item.id ? 'text-primary' : 'text-text-muted group-hover:text-text-main'}`}>
                    {item.icon}
                  </span>
                  <span className="text-sm font-medium">{item.label}</span>
                  {currentView === item.id && (
                    <div className="ml-auto w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_8px_rgba(52,211,153,0.8)]" />
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* System Section */}
      <div className="mt-auto px-4 pt-4 border-t border-border-base bg-bg-card">
        <div
          className="flex items-center justify-between text-[10px] font-bold text-text-muted uppercase tracking-widest mb-2 px-3 cursor-pointer hover:text-text-main transition-colors"
          onClick={() => toggleSection('system')}
        >
          <span>System</span>
          {expandedSections.system ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
        </div>
        {expandedSections.system && (
          <div className="space-y-1 mb-4">
            {systemItems.map((item) => (
              <div
                key={item.id}
                className={`
                  rounded-global flex items-center gap-3 px-3 py-2.5 cursor-pointer transition-all duration-200 group
                  ${currentView === item.id
                    ? 'bg-primary/10 text-primary shadow-[0_0_15px_rgba(52,211,153,0.1)]'
                    : 'text-text-muted hover:text-text-main hover:bg-bg-card-hover'}
                `}
                onClick={() => onViewChange(item.id)}
              >
                <span className={`transition-colors duration-200 ${currentView === item.id ? 'text-primary' : 'text-text-muted group-hover:text-text-main'}`}>
                  {item.icon}
                </span>
                <span className="text-sm font-medium">{item.label}</span>
                {currentView === item.id && (
                  <div className="ml-auto w-1.5 h-1.5 rounded-full bg-primary shadow-[0_0_8px_rgba(52,211,153,0.8)]" />
                )}
              </div>
            ))}
          </div>
        )}

        {/* Theme Toggle Footer */}
        <div className="pt-4 border-t border-border-base flex items-center justify-between px-2">
          <span className="text-[10px] font-bold text-text-muted uppercase tracking-wider">Appearance</span>
          <ThemeToggle />
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;
