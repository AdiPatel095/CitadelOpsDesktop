import React, { useEffect, useMemo, useState } from 'react';
import { Settings, Trash2 } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import CastleFocusBadge from './CastleFocusBadge';
import CastleFocusSwitcher from './CastleFocusSwitcher';
import { Button, Badge } from './ui';

const showHeaderMemoryBadges =
  import.meta.env.DEV === true || import.meta.env.VITE_SHOW_HEADER_MEMORY === 'true';

function formatNextBirdIn(msLeft: number): string {
  if (msLeft <= 0) return 'due now';
  const totalM = Math.ceil(msLeft / 60000);
  const h = Math.floor(totalM / 60);
  const m = totalM % 60;
  if (h > 0 && m > 0) return `${h}h ${m}m`;
  if (h > 0) return `${h}h`;
  return `${Math.max(1, m)}m`;
}

interface HeaderProps {
  onOpenAutoBirdSettings: () => void;
}

const Header: React.FC<HeaderProps> = ({ onOpenAutoBirdSettings }) => {
  const {
    gameLoggedIn,
    gameLoginCooldown,
    startGame,
    stopGame,
    goMem,
    chromeMem,
    autoBirdEnabled,
    autoBirdNextWakeUp,
    toggleAutoBird,
    sendMessage,
  } = useAuth();
  const { theme } = useTheme();

  const [nowTick, setNowTick] = useState(() => Date.now());
  useEffect(() => {
    if (!autoBirdEnabled) return;
    const id = window.setInterval(() => setNowTick(Date.now()), 30000);
    return () => window.clearInterval(id);
  }, [autoBirdEnabled]);

  const autoBirdPill = useMemo(() => {
    if (!autoBirdEnabled) {
      return { on: false as const, text: 'Auto Bird off' };
    }
    if (!autoBirdNextWakeUp) {
      return { on: true as const, text: 'Next Bird: scheduling…' };
    }
    const left = autoBirdNextWakeUp - nowTick;
    return { on: true as const, text: `Next Bird in: ${formatNextBirdIn(left)}` };
  }, [autoBirdEnabled, autoBirdNextWakeUp, nowTick]);

  return (
    <header className="h-16 bg-bg-card/80 backdrop-blur-md border-b border-border-base flex min-w-0 items-center px-6 fixed top-0 left-0 right-0 z-50 transition-colors duration-300">
      <div className="flex min-w-0 w-full items-center justify-between gap-3">
        {/* Left: Logo, Title */}
        <div className="flex min-w-0 shrink-0 items-center gap-4">
          <img
            src={theme === 'light' ? '/logo-light.svg' : '/logo-dark.svg'}
            alt="Citadel Ops Logo"
            className="w-8 h-8 drop-shadow-[0_0_8px_rgba(52,211,153,0.3)] transition-all duration-300"
          />
          <div className="flex items-center gap-2">
            <span className="text-xl font-bold text-text-main tracking-tight">Citadel Ops</span>
            <span className="text-xs font-medium text-text-muted">Desktop</span>
          </div>
        </div>

        {/* Center: Status Indicators */}
        <div className="flex min-w-0 flex-1 items-center justify-center gap-3 overflow-x-auto px-1 py-4 sm:gap-4">
          <div className="flex items-center gap-2">
            <CastleFocusBadge />
            <CastleFocusSwitcher />
          </div>
          
          {showHeaderMemoryBadges && (
            <>
              <Badge className="bg-purple-500/10 text-purple-400 border border-purple-500/30 gap-2 px-3 py-1.5">
                <span className="text-[9px] font-bold text-purple-400/80 uppercase tracking-wider">APP RAM</span>
                <span className="font-mono">{goMem ? `${goMem} MB` : '--'}</span>
              </Badge>
              <Badge className="bg-orange-500/10 text-orange-400 border border-orange-500/30 gap-2 px-3 py-1.5">
                <span className="text-[9px] font-bold text-orange-400/80 uppercase tracking-wider">CHROME RAM</span>
                <span className="font-mono">{chromeMem ? `${chromeMem} MB` : '--'}</span>
              </Badge>
            </>
          )}

          {/* Bot Status */}
          <div className={`rounded-global flex items-center gap-3 px-4 py-1.5 border transition-all duration-300 ${
            gameLoggedIn
              ? 'bg-success/10 border-success/30 shadow-[0_0_15px_var(--color-success)]'
              : gameLoginCooldown > 0
                ? 'bg-warning/10 border-warning/30 shadow-[0_0_15px_var(--color-warning)]'
                : 'bg-error/10 border-error/30 shadow-[0_0_15px_var(--color-error)]'
            }`}>
            <div className={`w-2.5 h-2.5 rounded-full shadow-[0_0_10px] animate-pulse ${
              gameLoggedIn ? 'bg-success shadow-success/50' 
              : gameLoginCooldown > 0 ? 'bg-warning shadow-warning/50' 
              : 'bg-error shadow-error/50'
            }`} />
            <span className={`text-sm font-semibold ${
              gameLoggedIn ? 'text-success' 
              : gameLoginCooldown > 0 ? 'text-warning' 
              : 'text-error'
            }`}>
              {gameLoggedIn ? 'Connected' : gameLoginCooldown > 0 ? `Reconnecting (${gameLoginCooldown}s)` : 'Disconnected'}
            </span>
          </div>

          <div className="flex shrink-0 items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => toggleAutoBird()}
                className={`border-2 ${
                  autoBirdPill.on
                    ? '!border-success/40 !text-success hover:!bg-success/10 !shadow-[0_0_15px_rgba(16,185,129,0.1)]'
                    : '!border-error/40 !text-error hover:!bg-error/10 !shadow-[0_0_15px_rgba(239,68,68,0.1)]'
                }`}
                title={
                  gameLoggedIn
                    ? 'Click to turn Auto Bird on or off'
                    : 'Last known Auto Bird status while bot is disconnected; reconnect to refresh'
                }
              >
                <div className={`w-2 h-2 rounded-full ${autoBirdPill.on ? 'bg-success animate-pulse' : 'bg-error'}`} />
                {autoBirdPill.text}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  if (
                    !window.confirm(
                      'Clear the AutoBird sent-bird log? Reconciliation starts fresh; use this to manually reset AutoBird tracking.'
                    )
                  ) {
                    return;
                  }
                  sendMessage('clearAutoBirdSentBirds');
                }}
                className="text-text-muted hover:text-error hover:bg-error/10"
                title="Clear logged sent birds (reset AutoBird reconciliation)"
              >
                <Trash2 className="w-4 h-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onClick={onOpenAutoBirdSettings}
                className={autoBirdPill.on ? 'text-success hover:bg-success/10' : 'text-error hover:bg-error/10'}
                title="Auto Bird Settings"
              >
                <Settings className="w-4 h-4" />
              </Button>
            </div>
        </div>

        {/* Right: bot controls */}
        <div className="flex shrink-0 items-center gap-3">
          {!gameLoggedIn ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => startGame()}
              className="uppercase tracking-wider text-[11px]"
              leftIcon={<div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />}
            >
              Start Bot
            </Button>
          ) : (
            <Button
              variant="danger"
              size="sm"
              onClick={stopGame}
              className="uppercase tracking-wider text-[11px]"
              leftIcon={<div className="w-1.5 h-1.5 rounded-full bg-error shadow-[0_0_8px] shadow-error/80" />}
            >
              Stop Bot
            </Button>
          )}
        </div>
      </div>
    </header>
  );
};

export default Header;
