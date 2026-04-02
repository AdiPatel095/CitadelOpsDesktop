import React from 'react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import CastleFocusBadge from './CastleFocusBadge';
interface HeaderProps {
  onOpenAutoBirdSettings: () => void;
}

const Header: React.FC<HeaderProps> = ({ onOpenAutoBirdSettings }) => {
  const { credits, isLoading, gameLoggedIn, gameLoginCooldown, startGame, stopGame, autoBirdEnabled, toggleAutoBird, nextWakeUp, goMem, chromeMem } = useAuth();
  const { theme } = useTheme();

  const [sleepTimer, setSleepTimer] = React.useState<string>('');

  React.useEffect(() => {
    if (!autoBirdEnabled || !nextWakeUp) {
      setSleepTimer('');
      return;
    }

    const updateTimer = () => {
      const now = Date.now();
      const diff = nextWakeUp - now;

      if (diff <= 0) {
        setSleepTimer('');
        return;
      }

      const hours = Math.floor(diff / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

      setSleepTimer(`${hours}h ${minutes}m`);
    };

    updateTimer();
    const interval = setInterval(updateTimer, 1000);
    return () => clearInterval(interval);
  }, [autoBirdEnabled, nextWakeUp]);

  return (
    <header className="h-16 bg-bg-card/80 backdrop-blur-md border-b border-border-base flex min-w-0 items-center px-6 fixed top-0 left-0 right-0 z-50 transition-colors duration-300">
      <div className="flex min-w-0 w-full items-center justify-between gap-3">
        {/* Left: Logo, Title, Credits */}
        <div className="flex min-w-0 shrink-0 items-center gap-4">
          {/* Theme-aware Logo */}
          <img
            src={theme === 'light' ? '/logo-light.svg' : '/logo-dark.svg'}
            alt="Citadel Ops Logo"
            className="w-8 h-8 drop-shadow-[0_0_8px_rgba(52,211,153,0.3)] transition-all duration-300"
          />
          <div className="flex items-center gap-2">
            <span className="text-xl font-bold text-text-main tracking-tight">Citadel Ops</span>
            <span className="text-xs font-medium text-text-muted">Desktop</span>
          </div>

          {/* Credits Display */}
          <div className="rounded-global flex items-center gap-2 px-3 py-1.5 bg-bg-app/50 border border-border-base transition-colors duration-300 ml-4">
            <div className="flex flex-col items-end mr-2">
              <span className="text-[10px] font-bold text-text-muted uppercase tracking-wider leading-none">Credits</span>
              <div className="flex items-center gap-1.5 leading-none mt-0.5">
                {isLoading ? (
                  <span className="text-sm font-mono font-medium text-text-muted">...</span>
                ) : (
                  <span className="text-sm font-mono font-medium text-primary">{credits.toLocaleString()}</span>
                )}
                <img src="/ops-coin.svg" alt="OPS" className="w-3.5 h-3.5" />
              </div>
            </div>
          </div>
        </div>

        {/* Center: Status Indicators */}
        <div className="flex min-w-0 flex-1 justify-center gap-3 overflow-x-auto px-1 sm:gap-4">
          <CastleFocusBadge />
          {/* Memory Status */}
          <div className="rounded-global flex items-center gap-2 px-3 py-1.5 border border-purple-500/30 bg-purple-500/10 transition-all duration-300">
            <span className="text-[9px] font-bold text-purple-400/80 uppercase tracking-wider">APP RAM</span>
            <span className="text-xs font-mono font-semibold text-purple-400">{goMem ? `${goMem} MB` : '--'}</span>
          </div>
          <div className="rounded-global flex items-center gap-2 px-3 py-1.5 border border-orange-500/30 bg-orange-500/10 transition-all duration-300">
            <span className="text-[9px] font-bold text-orange-400/80 uppercase tracking-wider">CHROME RAM</span>
            <span className="text-xs font-mono font-semibold text-orange-400">{chromeMem ? `${chromeMem} MB` : '--'}</span>
          </div>

          {/* AutoBird Status */}
          {autoBirdEnabled && (
            <div className={`rounded-global flex items-center gap-3 px-5 py-2 border-2 transition-all duration-300 ${sleepTimer
              ? 'bg-emerald-500/15 border-emerald-500/40 shadow-[0_0_20px_rgba(16,185,129,0.15)]'
              : 'bg-blue-500/15 border-blue-500/40 shadow-[0_0_20px_rgba(59,130,246,0.15)]'
              }`}>
              <div className={`w-3 h-3 rounded-full shadow-[0_0_12px] transition-colors duration-300 animate-pulse ${sleepTimer ? 'bg-emerald-500 shadow-emerald-500/80' : 'bg-blue-500 shadow-blue-500/80'
                }`} />
              <span className={`text-sm font-semibold transition-colors duration-300 ${sleepTimer ? 'text-emerald-400' : 'text-blue-400'
                }`}>
                {sleepTimer ? `Next Bird in: ${sleepTimer}` : 'Sending Birds'}
              </span>
            </div>
          )}

          {/* Bot Status */}
          <div className={`rounded-global flex items-center gap-3 px-5 py-2 border-2 transition-all duration-300 ${gameLoggedIn
            ? 'bg-emerald-500/15 border-emerald-500/40 shadow-[0_0_20px_rgba(16,185,129,0.15)]'
            : gameLoginCooldown > 0
              ? 'bg-yellow-500/15 border-yellow-500/40 shadow-[0_0_20px_rgba(234,179,8,0.15)]'
              : 'bg-red-500/15 border-red-500/40 shadow-[0_0_20px_rgba(239,68,68,0.15)]'
            }`}>
            <div className={`w-3 h-3 rounded-full shadow-[0_0_12px] transition-colors duration-300 animate-pulse ${gameLoggedIn
              ? 'bg-emerald-500 shadow-emerald-500/80'
              : gameLoginCooldown > 0
                ? 'bg-yellow-500 shadow-yellow-500/80'
                : 'bg-red-500 shadow-red-500/80'
              }`} />
            <span className={`text-sm font-semibold transition-colors duration-300 ${gameLoggedIn
              ? 'text-emerald-400'
              : gameLoginCooldown > 0
                ? 'text-yellow-400'
                : 'text-red-400'
              }`}>
              {gameLoggedIn
                ? 'Bot Connected'
                : gameLoginCooldown > 0
                  ? `Reconnecting (${gameLoginCooldown}s)`
                  : 'Bot Disconnected'}
            </span>
          </div>
        </div>

        {/* Right: bot controls (castle focus switcher lives in App global strip below header) */}
        <div className="flex shrink-0 items-center gap-3">
          {/* Start/Stop Bot Button */}
          {!gameLoggedIn ? (
            <button
              onClick={() => startGame()}
              className="px-4 py-2 rounded-global bg-emerald-500 hover:bg-emerald-600 text-white text-[11px] font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 shadow-[0_0_15px_rgba(16,185,129,0.2)] hover:shadow-[0_0_20px_rgba(16,185,129,0.4)] active:scale-95 border border-emerald-400/20 whitespace-nowrap"
            >
              <div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />
              Start Bot
            </button>
          ) : (
            <button
              onClick={stopGame}
              className="px-4 py-2 rounded-global bg-red-500 hover:bg-red-600 text-white text-[11px] font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 shadow-[0_0_15px_rgba(239,68,68,0.2)] hover:shadow-[0_0_20px_rgba(239,68,68,0.4)] active:scale-95 border border-red-400/20 whitespace-nowrap"
            >
              <div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />
              Stop Bot
            </button>
          )}

          {/* Auto Bird Controls */}
          <div className="flex items-center gap-2">
            <button
              onClick={toggleAutoBird}
              className={`px-4 py-2 rounded-global text-[11px] font-bold transition-all uppercase tracking-wider flex items-center justify-center gap-2 active:scale-95 whitespace-nowrap ${autoBirdEnabled
                ? 'bg-emerald-500/20 border-emerald-500/50 text-emerald-400 hover:bg-emerald-500/30 shadow-[0_0_15px_rgba(16,185,129,0.15)]'
                : 'bg-red-500/20 border border-red-500/50 text-red-400 hover:bg-red-500/30'
                }`}
            >
              <div className={`w-1.5 h-1.5 rounded-full ${autoBirdEnabled ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.8)]' : 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.8)]'}`} />
              Auto Bird: {autoBirdEnabled ? 'ON' : 'OFF'}
            </button>

            <button
              onClick={onOpenAutoBirdSettings}
              className="px-3 py-2 rounded-global bg-bg-card-hover border border-border-light text-text-muted hover:text-text-main hover:bg-bg-input transition-all active:scale-95 shadow-sm"
              title="Auto Bird Settings"
            >
              <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="3"></circle>
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
              </svg>
            </button>
          </div>
        </div>
      </div>
    </header>
  );
};

export default Header;
