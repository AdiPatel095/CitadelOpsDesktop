import React from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';

const Header: React.FC = () => {
  const { credits, isLoading, gameLoggedIn, gameLoginCooldown, changeLoginDetails, autoBirdEnabled, nextWakeUp } = useAuth();
  const { theme } = useTheme();

  const [showConfirm, setShowConfirm] = React.useState(false);
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
      const seconds = Math.floor((diff % (1000 * 60)) / 1000); // Optional, user asked for hrs/mins

      setSleepTimer(`${hours}h ${minutes}m`);
    };

    updateTimer();
    const interval = setInterval(updateTimer, 1000);
    return () => clearInterval(interval);
  }, [autoBirdEnabled, nextWakeUp]);

  const handleChangeLogin = () => {
    changeLoginDetails();
    setShowConfirm(false);
  };

  return (
    <header className="h-16 bg-bg-card/80 backdrop-blur-md border-b border-border-base flex items-center px-6 fixed top-0 left-0 right-0 z-50 transition-colors duration-300">
      <div className="flex items-center justify-between w-full">
        {/* Left: Logo, Title, Credits */}
        <div className="flex items-center gap-4">
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
        <div className="flex items-center gap-4">
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

        {/* Right: Change Login Button */}
        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowConfirm(true)}
            className="px-3 py-1.5 rounded-global bg-bg-app border border-border-base hover:border-yellow-500/50 hover:bg-yellow-500/10 text-text-muted hover:text-yellow-500 text-[10px] font-medium transition-all uppercase tracking-wider flex items-center gap-1.5"
          >
            Change Login
          </button>

          {/* Confirmation Modal */}
          {showConfirm && createPortal(
            <div className="fixed inset-0 z-[100] flex items-center justify-center">
              {/* Backdrop */}
              <div
                className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                onClick={() => setShowConfirm(false)}
              />

              {/* Modal Content */}
              <div className="relative glass-panel p-6 max-w-sm w-full mx-4 animate-fade-in shadow-2xl bg-bg-card/95">
                {/* Icon */}
                <div className="flex justify-center mb-4">
                  <div className="w-16 h-16 rounded-full bg-yellow-500/20 flex items-center justify-center shadow-[0_0_15px_rgba(234,179,8,0.2)]">
                    <svg className="w-8 h-8 text-yellow-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                </div>

                <h3 className="text-xl font-bold text-text-main text-center mb-3">Change Login Details?</h3>

                <p className="text-text-muted text-center mb-6 leading-relaxed">
                  This will <span className="text-red-400 font-semibold">delete your current login session</span>.
                  You will need to click 'Start Bot' to log in again with new details.
                </p>

                <div className="flex gap-3">
                  <button
                    onClick={() => setShowConfirm(false)}
                    className="flex-1 px-4 py-2.5 rounded-global bg-bg-app border border-border-base hover:bg-bg-card-hover text-text-muted hover:text-text-main font-semibold transition-all duration-200"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleChangeLogin}
                    className="flex-1 px-4 py-2.5 rounded-global bg-yellow-500 hover:bg-yellow-600 text-bg-app font-bold transition-all shadow-lg shadow-yellow-500/20 hover:shadow-yellow-500/40 active:scale-95 duration-200"
                  >
                    Confirm
                  </button>
                </div>
              </div>
            </div>,
            document.body
          )}
        </div>
      </div>
    </header>
  );
};

export default Header;
