import React, { useEffect, useMemo, useState } from 'react';
import { Lock, Settings, Shield, Unlock } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import AutoBirdHoverPopover from './AutoBirdHoverPopover';
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

function formatConnectionSeconds(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

function formatStationImpact(msLeft: number): string {
  if (msLeft <= 0) return 'now';
  const seconds = Math.ceil(msLeft / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

interface HeaderProps {
  onOpenAutoBirdSettings: () => void;
  onOpenAutoStationSettings: () => void;
  onOpenAutomationDuration: (featureKey: string, featureLabel: string) => void;
}

const Header: React.FC<HeaderProps> = ({
  onOpenAutoBirdSettings,
  onOpenAutoStationSettings,
  onOpenAutomationDuration,
}) => {
  const {
    gameLoggedIn,
    gameLoginCooldown,
    gameLoginRetrySeconds,
    gameConnectionState,
    gameSocketConnected,
    gameBrowserRunning,
		gameBrowserName,
    gameConnectionDetail,
    dashboardConnectionStatus,
    hasGameConnectionStatus,
    startGame,
    stopGame,
    goMem,
		browserMem,
    autoBirdEnabled,
    autoBirdNextWakeUp,
		autoBirdNextCastleName,
		autoBirdCastleCycles,
    toggleAutoBird,
    autoStationEnabled,
    autoStationState,
    autoStationThreatCount,
    autoStationNextImpact,
    autoStationDetail,
		toggleAutoStation,
		botLocked,
		toggleBotLock,
		automationStates,
		automationTimedUntilByKey,
  } = useAuth();
  const { theme } = useTheme();
	const autoBirdStatus = automationStates.autoBird?.status ?? '';
	const hasAutoBirdCycles = autoBirdCastleCycles.some((cycle) => cycle.nextCycleAtMs > 0);

  const [nowTick, setNowTick] = useState(() => Date.now());
  useEffect(() => {
    if (!autoBirdEnabled && !hasAutoBirdCycles) return;
    const id = window.setInterval(() => setNowTick(Date.now()), 30000);
    return () => window.clearInterval(id);
  }, [autoBirdEnabled, hasAutoBirdCycles]);

  useEffect(() => {
    if (!autoStationEnabled || !autoStationNextImpact) return;
    const id = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [autoStationEnabled, autoStationNextImpact]);

  const autoBirdPill = useMemo(() => {
    if (!autoBirdEnabled) {
      return { on: false as const, text: 'Auto Bird off' };
    }
    if (!autoBirdNextWakeUp) {
			switch (autoBirdStatus) {
				case 'running':
					return { on: true as const, text: 'Auto Bird sending…' };
				case 'idle':
					return { on: true as const, text: 'Auto Bird monitoring' };
				case 'protected':
					return { on: true as const, text: 'Auto Bird paused' };
				case 'blocked':
				case 'error':
					return { on: true as const, text: 'Auto Bird needs attention' };
				default:
					return { on: true as const, text: 'Auto Bird checking…' };
			}
    }
    const left = autoBirdNextWakeUp - nowTick;
		const castle = autoBirdNextCastleName || 'Unknown castle';
		return { on: true as const, text: `Next Bird: ${castle} · ${formatNextBirdIn(left)}` };
	}, [autoBirdEnabled, autoBirdNextCastleName, autoBirdNextWakeUp, autoBirdStatus, nowTick]);

	const autoBirdInteractionHint = automationTimedUntilByKey.auto_bird
		? `Timed until ${new Date(automationTimedUntilByKey.auto_bird).toLocaleString()}. Click toggles Auto Bird; right-click changes the duration.`
		: gameLoggedIn
			? 'Click toggles Auto Bird; right-click runs it for a duration.'
			: 'Showing the last known cycles while disconnected. Right-click runs Auto Bird for a duration.';

  const autoStationPill = useMemo(() => {
    if (!autoStationEnabled) return { tone: 'off' as const, text: 'Auto Station off' };
    const impact = autoStationNextImpact > 0 ? formatStationImpact(autoStationNextImpact - nowTick) : '';
    switch (autoStationState) {
      case 'threat':
        return { tone: 'warning' as const, text: `${autoStationThreatCount} incoming · ${impact || 'checking'}` };
      case 'evacuating':
        return { tone: 'warning' as const, text: 'Auto Station evacuating…' };
      case 'protected':
        return {
          tone: 'on' as const,
          text: autoStationThreatCount > 0 ? `${autoStationThreatCount} incoming protected` : 'Troops protected',
        };
      case 'recalling':
        return { tone: 'on' as const, text: 'Auto Station recalling…' };
      case 'waiting':
        return { tone: 'warning' as const, text: 'Auto Station waiting' };
      case 'error':
        return { tone: 'error' as const, text: 'Auto Station error' };
      default:
        return { tone: 'on' as const, text: 'Auto Station armed' };
    }
  }, [autoStationEnabled, autoStationNextImpact, autoStationState, autoStationThreatCount, nowTick]);

  const connectionPill = useMemo(() => {
    if (dashboardConnectionStatus !== 'Connected') {
      return {
        tone: 'warning' as const,
        pulse: true,
        label: dashboardConnectionStatus === 'Connecting' ? 'Dashboard connecting…' : 'Dashboard reconnecting…',
        title: 'Game connection status is unavailable while the dashboard reconnects to CitadelOps.',
      };
    }
    if (!hasGameConnectionStatus) {
      return {
        tone: 'warning' as const,
        pulse: true,
        label: 'Checking game status…',
        title: 'Dashboard connected; waiting for the current game WebSocket status.',
      };
    }

    switch (gameConnectionState) {
      case 'connected':
        return {
          tone: gameLoggedIn ? 'success' as const : 'warning' as const,
          pulse: true,
          label: gameLoggedIn ? 'Game connected' : 'Checking game status…',
          title: gameSocketConnected
            ? 'Game WebSocket is open and the game login is confirmed.'
            : 'Game login was reported, but the WebSocket is not currently open.',
        };
      case 'starting':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: 'Starting game…',
			title: `${gameBrowserName} is starting and loading the game client.`,
        };
      case 'reconnecting':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: 'Reloading game…',
          title: 'The game tab is reloading to establish a fresh WebSocket.',
        };
      case 'connecting':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: 'Opening game socket…',
          title: 'The game WebSocket handshake is in progress.',
        };
      case 'authenticating':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: 'Authenticating game…',
          title: 'Game WebSocket is open; waiting for the game login to complete.',
        };
      case 'cooldown':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: gameLoginCooldown > 0
            ? `Login cooldown (${formatConnectionSeconds(gameLoginCooldown)})`
            : gameLoginRetrySeconds > 0
              ? `Retrying in ${formatConnectionSeconds(gameLoginRetrySeconds)}`
              : 'Retrying login…',
          title: gameConnectionDetail || 'The game server requested a login cooldown; CitadelOps will retry automatically.',
        };
      case 'error':
        return {
          tone: 'error' as const,
          pulse: false,
          label: 'Connection error',
          title: gameConnectionDetail || 'The game connection failed. Start the bot to retry.',
        };
      case 'stopped':
        return {
          tone: 'error' as const,
          pulse: false,
          label: 'Game stopped',
			title: gameBrowserRunning
				? `The ${gameBrowserName} session is stopping.`
				: 'The game browser and WebSocket are stopped.',
        };
      default:
        return {
          tone: 'error' as const,
          pulse: false,
          label: 'Game disconnected',
          title: gameConnectionDetail || 'No active game WebSocket is available.',
        };
    }
  }, [
    dashboardConnectionStatus,
    gameBrowserRunning,
		gameBrowserName,
    gameConnectionDetail,
    gameConnectionState,
    gameLoggedIn,
    gameLoginCooldown,
    gameLoginRetrySeconds,
    gameSocketConnected,
    hasGameConnectionStatus,
  ]);

  const connectionToneClass = connectionPill.tone === 'success'
    ? 'text-success shadow-[0_0_15px_var(--color-success)]'
    : connectionPill.tone === 'warning'
      ? 'text-warning shadow-[0_0_15px_var(--color-warning)]'
      : 'text-error shadow-[0_0_15px_var(--color-error)]';
  const connectionDotClass = connectionPill.tone === 'success'
    ? 'bg-success shadow-success/50'
    : connectionPill.tone === 'warning'
      ? 'bg-warning shadow-warning/50'
      : 'bg-error shadow-error/50';
  const gameConnectionActive = hasGameConnectionStatus && (
    gameConnectionState === 'connecting' ||
    gameConnectionState === 'authenticating' ||
    gameConnectionState === 'connected' ||
    gameConnectionState === 'cooldown' ||
    gameConnectionState === 'reconnecting'
  );
  const connectionControlsReady =
    dashboardConnectionStatus === 'Connected' &&
    hasGameConnectionStatus &&
    gameConnectionState !== 'starting';

  return (
    <header className="liquid-header transition-colors duration-300">
      <div className="liquid-header-inner relative z-10">
        {/* Left: Logo, Title */}
        <div className="liquid-brand">
          <div className="liquid-brand-mark">
            <img
              src={theme === 'light' ? '/logo-light.svg' : '/logo-dark.svg'}
              alt="Citadel Ops Logo"
              className="w-7 h-7 drop-shadow-[0_0_10px_var(--primary-glow)] transition-all duration-300"
            />
          </div>
          <div className="liquid-brand-copy">
            <div className="text-lg font-bold leading-tight text-text-main">Citadel Ops</div>
            <div className="text-[11px] font-medium leading-tight text-text-muted">Desktop</div>
          </div>
        </div>

        {/* Center: Status Indicators */}
        <div className="liquid-header-status-strip custom-scrollbar">
          <div className="flex min-w-0 items-center gap-2">
            <CastleFocusSwitcher />
          </div>
          
          {showHeaderMemoryBadges && (
            <>
              <Badge className="liquid-memory-badge bg-info/10 text-info border border-info/30 gap-2 px-3 py-1.5">
                <span className="text-[9px] font-bold text-info/80 uppercase tracking-wider">APP RAM</span>
                <span className="font-mono">{goMem ? `${goMem} MB` : '--'}</span>
              </Badge>
              <Badge className="liquid-memory-badge bg-warning/10 text-warning border border-warning/30 gap-2 px-3 py-1.5">
				<span className="text-[9px] font-bold text-warning/80 uppercase tracking-wider">BROWSER RAM</span>
				<span className="font-mono">{browserMem ? `${browserMem} MB` : '--'}</span>
              </Badge>
            </>
          )}

          {/* Bot Status */}
          <div
            className={`liquid-bot-status liquid-glass-edge flex shrink-0 items-center gap-3 rounded-full px-4 py-1.5 transition-all duration-300 ${connectionToneClass}`}
            title={connectionPill.title}
          >
            <div className={`w-2.5 h-2.5 rounded-full shadow-[0_0_10px] ${connectionPill.pulse ? 'animate-pulse' : ''} ${connectionDotClass}`} />
            <span className="liquid-bot-status-text text-sm font-semibold">
              {connectionPill.label}
            </span>
          </div>

          <div className="liquid-auto-bird-actions">
            <AutoBirdHoverPopover
				cycles={autoBirdCastleCycles}
				enabled={autoBirdEnabled}
				now={nowTick}
				hint={autoBirdInteractionHint}
			>
				<Button
					variant="outline"
					size="sm"
					onClick={() => toggleAutoBird()}
					onContextMenu={(event) => {
						event.preventDefault();
						onOpenAutomationDuration('auto_bird', 'Auto Bird');
					}}
					className={`liquid-auto-bird-button border-2 ${
						autoBirdPill.on
							? '!border-success/40 !text-success hover:!bg-success/10 !shadow-[0_0_15px_rgba(16,185,129,0.1)]'
							: '!border-error/40 !text-error hover:!bg-error/10 !shadow-[0_0_15px_rgba(239,68,68,0.1)]'
					}`}
					aria-label={`${autoBirdPill.text}. Hover for every castle cycle.`}
				>
					<div className={`w-2 h-2 rounded-full ${autoBirdPill.on ? 'bg-success animate-pulse' : 'bg-error'}`} />
					<span className="liquid-auto-bird-text">{autoBirdPill.text}</span>
				</Button>
			</AutoBirdHoverPopover>
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

          <div className="liquid-auto-bird-actions">
            <Button
              variant="outline"
              size="sm"
              onClick={toggleAutoStation}
              onContextMenu={(event) => {
                event.preventDefault();
                onOpenAutomationDuration('auto_station', 'Auto Station');
              }}
              className={`liquid-auto-bird-button border-2 ${
                autoStationPill.tone === 'on'
                  ? '!border-success/40 !text-success hover:!bg-success/10'
                  : autoStationPill.tone === 'warning'
                    ? '!border-warning/50 !text-warning hover:!bg-warning/10'
                    : '!border-error/40 !text-error hover:!bg-error/10'
              }`}
              title={automationTimedUntilByKey.auto_station
                ? `Timed until ${new Date(automationTimedUntilByKey.auto_station).toLocaleString()}. Right-click to change the duration.`
                : `${autoStationDetail || 'Click to turn Auto Station on or off'}. Right-click to run it for a duration.`}
            >
              <Shield className="h-4 w-4" />
              <span className="liquid-auto-bird-text">{autoStationPill.text}</span>
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={onOpenAutoStationSettings}
              className={autoStationEnabled ? 'text-success hover:bg-success/10' : 'text-error hover:bg-error/10'}
              title="Auto Station Settings"
            >
              <Settings className="h-4 w-4" />
            </Button>
          </div>

        </div>

        {/* Right: bot controls */}
        <div className="liquid-header-controls">
			<Button
				variant={botLocked ? 'danger' : 'outline'}
				size="sm"
				onClick={toggleBotLock}
				disabled={dashboardConnectionStatus !== 'Connected'}
				aria-pressed={botLocked}
				title={botLocked
					? 'Automation and scheduled game actions are locked. Click to resume them.'
					: 'Automation is allowed to control the game. Click to lock all automated actions.'}
				className="uppercase text-[11px]"
				leftIcon={botLocked ? <Lock className="h-3.5 w-3.5" /> : <Unlock className="h-3.5 w-3.5" />}
			>
				{botLocked ? 'Unlock Bot' : 'Lock Bot'}
			</Button>
          {!gameConnectionActive ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => startGame()}
              disabled={!connectionControlsReady}
              title={connectionControlsReady ? 'Start or retry the game connection' : 'Waiting for current connection status'}
              className="uppercase text-[11px]"
              leftIcon={<div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />}
            >
              {gameConnectionState === 'starting' ? 'Starting…' : 'Start Bot'}
            </Button>
          ) : (
            <Button
              variant="danger"
              size="sm"
              onClick={stopGame}
              disabled={!connectionControlsReady}
              className="uppercase text-[11px]"
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
