import React, { useEffect, useMemo, useState } from 'react';
import { Bird, Lock, Menu, Radio, Settings, Shield, Trash2, Unlock } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import AutoBirdHoverPopover from './AutoBirdHoverPopover';
import CastleFocusSwitcher from './CastleFocusSwitcher';
import DailyAttackTracker from './DailyAttackTracker';
import { Notifications } from './Notifications';
import { Button } from './ui';

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
  onOpenNavigation: () => void;
  navigationOpen: boolean;
}

const Header: React.FC<HeaderProps> = ({
  onOpenAutoBirdSettings,
  onOpenAutoStationSettings,
  onOpenAutomationDuration,
  onOpenNavigation,
  navigationOpen,
}) => {
  const { state, submitIntent } = useCitadelAPI();
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
    reconnectGame,
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
	const backgroundConnection = state?.session.mode === 'background';
	const autoBirdStatus = automationStates.autoBird?.status ?? '';
	const hasAutoBirdCycles = autoBirdCastleCycles.some((cycle) => cycle.nextCycleAtMs > 0);
  const [clearingAutoBirdTracking, setClearingAutoBirdTracking] = useState(false);

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

  const clearAutoBirdTracking = async () => {
    if (clearingAutoBirdTracking) return;
    if (!window.confirm(
      'Clear all Auto Bird cycle tracking from CitadelOps memory? Settings, presets, Auto Station, and game movements will be kept. Auto Bird will rebuild tracking from current game state.',
    )) return;
    setClearingAutoBirdTracking(true);
    try {
      await submitIntent('auto_bird.clear_tracking', {}, { actor: 'ui:auto-bird' });
      Notifications.success('Auto Bird tracking reset requested.');
    } catch {
      // The API context already presents the server error.
    } finally {
      setClearingAutoBirdTracking(false);
    }
  };

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
			title: backgroundConnection
				? 'The direct background game connection is starting.'
				: `${gameBrowserName} is starting and loading the game client.`,
        };
      case 'reconnecting':
        return {
          tone: 'warning' as const,
          pulse: true,
          label: backgroundConnection ? 'Reconnecting game…' : 'Reloading game…',
          title: backgroundConnection
				? 'CitadelOps is reconnecting directly to the game server.'
				: 'The game tab is reloading to establish a fresh WebSocket.',
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
      case 'suspended':
        return {
          tone: 'error' as const,
          pulse: false,
          label: gameLoginRetrySeconds > 0
            ? `Account suspended (resumes in ${formatConnectionSeconds(gameLoginRetrySeconds)})`
            : 'Account suspended',
          title: gameConnectionDetail || 'The game reported the account as suspended; CitadelOps resumes automatically when the suspension ends.',
        };
      case 'released':
        return {
          tone: 'warning' as const,
          pulse: false,
          label: gameLoginRetrySeconds > 0
            ? `Session released (retry in ${formatConnectionSeconds(gameLoginRetrySeconds)})`
            : 'Session released',
          title: gameConnectionDetail || 'The game session was released until the retry time; use Reconnect to try now.',
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
			title: backgroundConnection
				? 'The direct background game connection is stopped.'
				: gameBrowserRunning
				? `The ${gameBrowserName} session is stopping.`
				: 'The game browser and WebSocket are stopped.',
        };
      default:
        return {
          tone: 'error' as const,
          pulse: gameLoginRetrySeconds > 0,
          label: gameLoginRetrySeconds > 0
            ? `Retrying in ${formatConnectionSeconds(gameLoginRetrySeconds)}`
            : 'Game disconnected',
          title: gameConnectionDetail || (gameLoginRetrySeconds > 0
			? backgroundConnection
				? 'CitadelOps will reconnect directly and retry the saved login automatically.'
				: 'CitadelOps will reload the game and retry the saved login automatically.'
            : 'No active game WebSocket is available.'),
        };
    }
  }, [
		backgroundConnection,
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

  const connectionIconClass = connectionPill.tone === 'success'
    ? 'liquid-header-connection-success'
    : connectionPill.tone === 'warning'
      ? 'liquid-header-connection-warning'
      : 'liquid-header-connection-danger';
  const desktopConnectionToneClass = connectionPill.tone === 'success'
    ? 'm3-status-chip-success text-success'
    : connectionPill.tone === 'warning'
      ? 'm3-status-chip-warning text-warning'
      : 'm3-status-chip-danger text-error';
  const desktopConnectionDotClass = connectionPill.tone === 'success'
    ? 'bg-success shadow-success/50'
    : connectionPill.tone === 'warning'
      ? 'bg-warning shadow-warning/50'
      : 'bg-error shadow-error/50';
  const gameConnectionActive = hasGameConnectionStatus && (
    gameConnectionState === 'connecting' ||
    gameConnectionState === 'authenticating' ||
    gameConnectionState === 'connected' ||
    gameConnectionState === 'cooldown' ||
    gameConnectionState === 'reconnecting' ||
    gameConnectionState === 'suspended' ||
    gameConnectionState === 'released'
  );
  // While the runtime is waiting to reconnect on its own (relog delay,
  // cooldown, suspension) or has released the session, the user can force an
  // early retry instead of waiting out the timer.
  const gameReconnectAvailable = hasGameConnectionStatus && (
    gameConnectionState === 'cooldown' ||
    gameConnectionState === 'reconnecting' ||
    gameConnectionState === 'suspended' ||
    gameConnectionState === 'released'
  );
  const connectionControlsReady =
    dashboardConnectionStatus === 'Connected' &&
    hasGameConnectionStatus &&
    gameConnectionState !== 'starting';
  return (
    <header className="liquid-header transition-colors duration-300">
      <div className="liquid-header-inner relative z-10">
        <button
          type="button"
          className="liquid-mobile-nav-trigger"
          onClick={onOpenNavigation}
          aria-label="Open workspace navigation"
          aria-expanded={navigationOpen}
          aria-controls="workspace-navigation"
        >
          <Menu className="h-5 w-5" />
        </button>

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
            <div className="text-[11px] font-medium leading-tight text-text-muted">Command center</div>
          </div>
          <span
            className={`liquid-header-connection ${connectionIconClass} ${connectionPill.pulse ? 'liquid-header-connection-pulse' : ''}`}
            role="status"
            aria-label={connectionPill.label}
            title={`${connectionPill.label}. ${connectionPill.title}`}
          >
            <Radio className="h-4 w-4" aria-hidden="true" />
          </span>
        </div>

        {/* Center: Status Indicators */}
        <div className="liquid-header-status-strip custom-scrollbar">
          <div className="liquid-castle-focus-slot flex min-w-0 items-center gap-2">
            <CastleFocusSwitcher />
          </div>
          <div className="liquid-status-dock" role="group" aria-label="Daily attacks and automation status">
            <DailyAttackTracker />

            <div
              className={`m3-status-chip liquid-desktop-connection-pill ${desktopConnectionToneClass}`}
              title={connectionPill.title}
              aria-live="polite"
            >
              <span className={`liquid-desktop-connection-dot ${connectionPill.pulse ? 'animate-pulse' : ''} ${desktopConnectionDotClass}`} aria-hidden="true" />
              <span className="liquid-desktop-status-text">{connectionPill.label}</span>
            </div>

            <div className={`liquid-status-dock-item liquid-status-dock-action-group liquid-header-automation-pill ${autoBirdPill.on ? 'liquid-status-dock-item-success' : 'liquid-status-dock-item-muted'}`}>
              <AutoBirdHoverPopover
                cycles={autoBirdCastleCycles}
                enabled={autoBirdEnabled}
                now={nowTick}
                hint={autoBirdInteractionHint}
              >
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => toggleAutoBird()}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    onOpenAutomationDuration('auto_bird', 'Auto Bird');
                  }}
                  className="liquid-status-dock-main liquid-status-dock-icon-button liquid-auto-bird-button"
                  aria-label={`${autoBirdPill.text}. Hover for every castle cycle.`}
                >
                  <span className="liquid-status-dock-icon liquid-mobile-status-icon" aria-hidden="true">
                    <Bird className="h-4 w-4" />
                    <span className={`liquid-status-dock-dot ${autoBirdPill.on ? 'bg-success animate-pulse' : 'bg-text-muted'}`} />
                  </span>
                  <span className={`liquid-desktop-status-dot ${autoBirdPill.on ? 'bg-success animate-pulse' : 'bg-text-muted'}`} aria-hidden="true" />
                  <span className="liquid-desktop-status-text">{autoBirdPill.text}</span>
                </Button>
              </AutoBirdHoverPopover>
              <span className="liquid-status-dock-utilities">
                <Button
                  variant="ghost"
                  size="icon"
                  disabled={clearingAutoBirdTracking}
                  onClick={() => void clearAutoBirdTracking()}
                  className="liquid-status-dock-utility text-text-muted hover:text-error"
                  title="Clear Auto Bird cycle tracking from CitadelOps memory"
                  aria-label="Clear Auto Bird cycle tracking"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onOpenAutoBirdSettings}
                  className="liquid-status-dock-utility"
                  title="Auto Bird Settings"
                  aria-label="Open Auto Bird settings"
                >
                  <Settings className="h-4 w-4" />
                </Button>
              </span>
            </div>

            <div className={`liquid-status-dock-item liquid-status-dock-action-group liquid-header-automation-pill ${
              autoStationPill.tone === 'on'
                ? 'liquid-status-dock-item-success'
                : autoStationPill.tone === 'warning'
                  ? 'liquid-status-dock-item-warning'
                  : autoStationPill.tone === 'error'
                    ? 'liquid-status-dock-item-danger'
                    : 'liquid-status-dock-item-muted'
            }`}>
              <Button
                variant="ghost"
                size="icon"
                onClick={toggleAutoStation}
                onContextMenu={(event) => {
                  event.preventDefault();
                  onOpenAutomationDuration('auto_station', 'Auto Station');
                }}
                className="liquid-status-dock-main liquid-status-dock-icon-button liquid-auto-bird-button"
                aria-label={autoStationPill.text}
                title={automationTimedUntilByKey.auto_station
                  ? `Timed until ${new Date(automationTimedUntilByKey.auto_station).toLocaleString()}. Right-click to change the duration.`
                  : `${autoStationDetail || 'Click to turn Auto Station on or off'}. Right-click to run it for a duration.`}
              >
                <span className="liquid-status-dock-icon liquid-mobile-status-icon" aria-hidden="true">
                  <Shield className="h-4 w-4" />
                  <span className={`liquid-status-dock-dot ${
                    autoStationPill.tone === 'on'
                      ? 'bg-success animate-pulse'
                      : autoStationPill.tone === 'warning'
                        ? 'bg-warning animate-pulse'
                        : autoStationPill.tone === 'error'
                          ? 'bg-error'
                          : 'bg-text-muted'
                  }`} />
                </span>
                <Shield className="liquid-desktop-status-icon h-4 w-4" aria-hidden="true" />
                <span className="liquid-desktop-status-text">{autoStationPill.text}</span>
              </Button>
              <span className="liquid-status-dock-utilities">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onOpenAutoStationSettings}
                  className="liquid-status-dock-utility"
                  title="Auto Station Settings"
                  aria-label="Open Auto Station settings"
                >
                  <Settings className="h-4 w-4" />
                </Button>
              </span>
            </div>
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
				<span className="liquid-header-control-label">{botLocked ? 'Unlock Bot' : 'Lock Bot'}</span>
			</Button>
          {gameReconnectAvailable && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => reconnectGame()}
              disabled={dashboardConnectionStatus !== 'Connected'}
              title={gameConnectionState === 'cooldown'
                ? 'Retry the game login now. The game may answer with another cooldown if its timer has not elapsed.'
                : gameConnectionState === 'suspended'
                  ? 'Retry the game login now. A suspended account will be refused until the suspension ends.'
                  : 'Reconnect to the game now instead of waiting for the retry timer'}
              className="uppercase text-[11px]"
            >
              <span className="liquid-header-control-label">Reconnect</span>
            </Button>
          )}
          {!gameConnectionActive && (
            <Button
              variant="primary"
              size="sm"
              onClick={() => startGame()}
              disabled={!connectionControlsReady}
              title={connectionControlsReady ? 'Start or retry the game connection' : 'Waiting for current connection status'}
              className="uppercase text-[11px]"
              leftIcon={<div className="w-1.5 h-1.5 rounded-full bg-white shadow-[0_0_8px] shadow-white/80" />}
            >
              <span className="liquid-header-control-label">
                {gameConnectionState === 'starting' ? 'Starting…' : 'Start Bot'}
              </span>
            </Button>
          )}
        </div>
      </div>
    </header>
  );
};

export default Header;
