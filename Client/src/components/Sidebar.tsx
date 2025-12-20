import React from 'react';
import { NAVIGATION_ITEMS, type ViewId } from '../config/navigation';
import { ThemeToggle } from './ThemeToggle';

interface SidebarProps {
  currentView: ViewId;
  onViewChange: (viewId: ViewId) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ currentView, onViewChange }) => {
  const mainItems = NAVIGATION_ITEMS.filter(item => item.section === 'main');
  const systemItems = NAVIGATION_ITEMS.filter(item => item.section === 'system');

  return (
    <aside className="w-64 bg-bg-card border-r border-border-base flex flex-col pt-20 pb-6 h-screen fixed left-0 top-0 z-40 transition-colors duration-300">
      <div className="px-4 mb-2">
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
