import React from 'react';
import { useAuth } from '../context/AuthContext';

const Header: React.FC = () => {
  const { credits, isLoading, gameLoggedIn, gameLoginCooldown } = useAuth();

  return (
    <header className="h-16 bg-dark-card/80 backdrop-blur-md border-b border-dark-border flex items-center px-6 fixed top-0 left-0 right-0 z-50">
      <div className="flex items-center justify-between w-full">
        <div className="flex items-center gap-3">
          <img src="/logo.svg" alt="Citadel Ops Logo" className="w-8 h-8 drop-shadow-[0_0_8px_rgba(52,211,153,0.5)]" />
          <span className="text-xl font-bold bg-gradient-to-r from-white to-gray-400 bg-clip-text text-transparent">Citadel Ops</span>
          <span className="text-xs font-medium text-gray-500 ml-2">Desktop</span>
        </div>

        <div className="flex items-center gap-4">
          {/* Credits Display */}
          <div className="rounded-global flex items-center gap-2 px-3 py-1.5 bg-dark-bg/50 border border-dark-border">
            <div className="flex flex-col items-end mr-2">
              <span className="text-[10px] font-bold text-gray-500 uppercase tracking-wider leading-none">Credits</span>
              <div className="flex items-center gap-1.5 leading-none mt-0.5">
                {isLoading ? (
                  <span className="text-sm font-mono font-medium text-gray-500">...</span>
                ) : (
                  <span className="text-sm font-mono font-medium text-primary">{credits.toLocaleString()}</span>
                )}
                <img src="/ops-coin.svg" alt="OPS" className="w-3.5 h-3.5" />
              </div>
            </div>
          </div>

          {/* Device indicator */}
          {/* Device indicator */}
          <div className={`rounded-global flex items-center gap-2 px-3 py-1.5 border transition-colors duration-300 ${gameLoggedIn
            ? 'bg-emerald-500/10 border-emerald-500/20'
            : gameLoginCooldown > 0
              ? 'bg-yellow-500/10 border-yellow-500/20'
              : 'bg-red-500/10 border-red-500/20'
            }`}>
            <div className={`w-2 h-2 rounded-full shadow-[0_0_8px] transition-colors duration-300 ${gameLoggedIn
              ? 'bg-emerald-500 shadow-emerald-500/80'
              : gameLoginCooldown > 0
                ? 'bg-yellow-500 shadow-yellow-500/80'
                : 'bg-red-500 shadow-red-500/80'
              }`} />
            <span className={`text-xs font-medium transition-colors duration-300 ${gameLoggedIn
              ? 'text-emerald-500'
              : gameLoginCooldown > 0
                ? 'text-yellow-500'
                : 'text-red-500'
              }`}>
              {gameLoggedIn
                ? 'Connected'
                : gameLoginCooldown > 0
                  ? `Cooldown (${gameLoginCooldown}s)`
                  : 'Disconnected'}
            </span>
          </div>
        </div>
      </div>
    </header>
  );
};

export default Header;
