import React from 'react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/navigation';
import { ThemeToggle } from './ThemeToggle';
import { useAuth } from '../context/AuthContext';
import { Settings } from 'lucide-react';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
  onOpenAutoBirdSettings: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({ currentView, onViewChange, onOpenAutoBirdSettings }) => {
  const { gameLoggedIn, startGame, stopGame, autoBirdEnabled, toggleAutoBird } = useAuth();
  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');

  return (
    <aside className="w-64 bg-bg-card border-r border-border-base flex flex-col pt-20 pb-6 h-screen fixed left-0 top-0 z-40 transition-colors duration-300">
      {/* Bot Controls Section - Smaller */}
      <div className="px-4 mb-4">
        <div className="text-[10px] font-bold text-text-muted uppercase tracking-widest mb-2 px-3">Bot Controls</div>
        <div className="px-2 space-y-2">
          {/* Start/Stop Bot Button */}
          {!gameLoggedIn ? (
            <button
              onClick={startGame}
              className="w-full px-3 py-2 rounded-global bg-emerald-500 hover:bg-emerald-600 text-white text-xs font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 shadow-lg shadow-emerald-500/20 active:scale-95"
            >
              <div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />
              Start Bot
            </button>
          ) : (
            <button
              onClick={stopGame}
              className="w-full px-3 py-2 rounded-global bg-red-500 hover:bg-red-600 text-white text-xs font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 shadow-lg shadow-red-500/20 active:scale-95"
            >
              <div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />
              Stop Bot
            </button>
          )}

          {/* Auto Bird Controls */}
          <div className="flex gap-2">
            <button
              onClick={toggleAutoBird}
              className={`flex-1 px-3 py-2 rounded-global text-xs font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 active:scale-95 ${autoBirdEnabled
                ? 'bg-emerald-500/20 border border-emerald-500/50 text-emerald-400 hover:bg-emerald-500/30'
                : 'bg-red-500/20 border border-red-500/50 text-red-400 hover:bg-red-500/30'
                }`}
            >
              <div className={`w-1.5 h-1.5 rounded-full ${autoBirdEnabled ? 'bg-emerald-400' : 'bg-red-400'}`} />
              Auto Bird: {autoBirdEnabled ? 'ON' : 'OFF'}
            </button>

            <button
              onClick={onOpenAutoBirdSettings}
              className="px-3 py-2 rounded-global bg-bg-card-hover border border-border-light text-text-muted hover:text-text-main hover:bg-bg-input transition-all active:scale-95"
              title="Auto Bird Settings"
            >
              <Settings className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* Main Navigation - Vertically centered */}
      <div className="flex-1 flex flex-col justify-center px-4">
        <div className="text-xs font-bold text-text-muted uppercase tracking-widest mb-3 px-3">Main Menu</div>
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
      </div>

      {/* System Section - At bottom */}
      <div className="mt-auto px-4">
        <div className="text-xs font-bold text-text-muted uppercase tracking-widest mb-3 px-3">System</div>
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
