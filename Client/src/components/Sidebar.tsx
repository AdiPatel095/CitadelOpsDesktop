import React, { useState } from 'react';
import { Settings, ChevronDown, ChevronRight } from 'lucide-react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/navigation';
import { ThemeToggle } from './ThemeToggle';
import { useAuth } from '../context/AuthContext';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
  onOpenRecruitTroopsSettings: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({ currentView, onViewChange, onOpenRecruitTroopsSettings }) => {
  const { recruitTroopsEnabled, toggleRecruitTroops } = useAuth();

  // Collapse state
  const [expandedSections, setExpandedSections] = useState({
    gameFunctions: true,
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
        {/* Game Functions Section */}
        <div className="px-4 mb-4">
          <div
            className="flex items-center justify-between text-[10px] font-bold text-text-muted uppercase tracking-widest mb-2 px-3 cursor-pointer hover:text-text-main transition-colors"
            onClick={() => toggleSection('gameFunctions')}
          >
            <span>Game Functions</span>
            {expandedSections.gameFunctions ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          </div>
          {expandedSections.gameFunctions && (
            <div className="px-2 space-y-2">
              {/* Recruit Troops Controls */}
              <div className="flex gap-2">
                <button
                  onClick={toggleRecruitTroops}
                  className={`flex-1 px-3 py-2 rounded-global text-xs font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 shadow-lg active:scale-95 ${recruitTroopsEnabled
                    ? 'bg-amber-500 hover:bg-amber-600 text-white shadow-amber-500/20'
                    : 'bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/50 text-amber-500'
                    }`}
                >
                  <div className={`w-1.5 h-1.5 rounded-full ${recruitTroopsEnabled ? 'bg-white shadow-[0_0_8px] shadow-white/80' : 'bg-amber-400'}`} />
                  Recruit: {recruitTroopsEnabled ? 'ON' : 'OFF'}
                </button>
                <button
                  onClick={onOpenRecruitTroopsSettings}
                  className="px-3 py-2 rounded-global bg-bg-card-hover border border-border-light text-text-muted hover:text-text-main hover:bg-bg-input transition-all active:scale-95 shadow-sm"
                  title="Recruit Troops Settings"
                >
                  <Settings className="w-4 h-4" />
                </button>
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

      {/* System Section - At bottom */}
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
