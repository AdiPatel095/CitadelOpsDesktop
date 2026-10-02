import { useHostedRuntimePresence } from '../config/Deployment';
import { ConnectionStatus } from './ConnectionStatus';
import { headerPlayerStatus } from './playerStatusDisplay';
import { useLocale as useStaticLocale } from "../i18n/LocaleContext";
import { LocalizedText } from "../i18n/LocalizedText";
import { useLocale } from '../i18n/LocaleContext';
import React, { useEffect, useMemo, useState } from 'react';
import { Bird, Lock, Menu, Settings, Shield, Trash2, Unlock } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import AutoBirdHoverPopover from './AutoBirdHoverPopover';
import AutoStationHoverPopover from './AutoStationHoverPopover';
import { stationHeaderPill } from './stationHeaderPill';
import { AutomationFeatureFeedback } from './AutomationFeatureFeedback';
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
  const { t: localizeStatic } = useStaticLocale();
  const { t, messageLocale, locale } = useLocale();
  const { state, submitIntent } = useCitadelAPI();
  const {
    gameLoggedIn,
    gameConnectionState,
    gameBrowserRunning,
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
	const autoBirdStatus = automationStates.autoBird?.status ?? '';
	const hasAutoBirdCycles = autoBirdCastleCycles.some((cycle) => cycle.nextCycleAtMs > 0 || !!cycle.pausedUntilMs);
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
		? `Timed until ${new Date(automationTimedUntilByKey.auto_bird).toLocaleString(locale)}. Click toggles Auto Bird; right-click changes the duration.`
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

  const autoStationPill = useMemo(() => stationHeaderPill({
    enabled: autoStationEnabled,
    status: autoStationState,
    threatCount: autoStationThreatCount,
    nextImpact: autoStationNextImpact,
    now: nowTick,
    stationName: t('automationPopover.station.title'),
    blockedLabel: t('runtimeState.phase', { phase: 'blocked' }),
  }), [autoStationEnabled, autoStationNextImpact, autoStationState, autoStationThreatCount, nowTick, t]);

  const presence = useHostedRuntimePresence();
  const connectionValue = headerPlayerStatus({
    surface: 'desktop', status: gameConnectionState, loggedIn: gameLoggedIn,
    dashboard: dashboardConnectionStatus, started: hasGameConnectionStatus || gameBrowserRunning,
    loginFailure: state?.session.loginFailure, cooldownUntil: state?.session.cooldownUntil,
    retryAt: state?.session.retryAt, now: nowTick,
    checkpoint: presence.mode === 'checkpoint', checkpointObservedAt: presence.checkpointObservedAt,
    detail: gameConnectionDetail ? { text: gameConnectionDetail } : undefined,
  });


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
          aria-label={localizeStatic("ui.components.header.aria-label.open.workspace.navigation.9df22e36")}
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
              alt={localizeStatic("ui.components.header.alt.citadel.ops.logo.ab367a3c")}
              className="w-7 h-7 drop-shadow-[0_0_10px_var(--primary-glow)] transition-all duration-300"
            />
          </div>
          <div className="liquid-brand-copy">
            <div className="text-lg font-bold leading-tight text-text-main">CitadelOps</div>
            <div className="text-[11px] font-medium leading-tight text-text-muted"><LocalizedText messageKey="navigation.commandCenter" /></div>
          </div>
          <span className="liquid-header-connection"><ConnectionStatus value={connectionValue} /></span>
        </div>

        {/* Center: Status Indicators */}
        <div className="liquid-header-status-strip custom-scrollbar">
          <div className="liquid-castle-focus-slot flex min-w-0 items-center gap-2">
            <CastleFocusSwitcher />
          </div>
          <div className="liquid-status-dock" role="group" aria-label={localizeStatic("ui.components.header.aria-label.daily.attacks.and.automation.status.1f099931")}>
            <DailyAttackTracker />

            <div className="liquid-desktop-connection-pill"><ConnectionStatus value={connectionValue} /></div>

            <div className={`liquid-status-dock-item liquid-status-dock-action-group liquid-header-automation-pill ${autoBirdPill.on ? 'liquid-status-dock-item-success' : 'liquid-status-dock-item-muted'}`}>
              <AutoBirdHoverPopover
                cycles={autoBirdCastleCycles}
                canControl={dashboardConnectionStatus === 'Connected'}
                enabled={autoBirdEnabled}
                now={nowTick}
                hint={autoBirdInteractionHint}
                feedback={<AutomationFeatureFeedback featureId="autoBird" enabled={autoBirdEnabled} onOpenSettings={onOpenAutoBirdSettings} compact />}
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
                  title={localizeStatic("ui.components.header.title.clear.auto.bird.cycle.tracking.from.citadelops.cddc50b5")}
                  aria-label={localizeStatic("ui.components.header.aria-label.clear.auto.bird.cycle.tracking.4813b36e")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onOpenAutoBirdSettings}
                  className="liquid-status-dock-utility"
                  title={localizeStatic("ui.components.header.title.auto.bird.settings.158a0a4f")}
                  aria-label={localizeStatic("ui.components.header.aria-label.open.auto.bird.settings.787f04dc")}
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
              <AutoStationHoverPopover feedback={<AutomationFeatureFeedback featureId="autoStation" enabled={autoStationEnabled} onOpenSettings={onOpenAutoStationSettings} compact />}>
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
                  ? `Timed until ${new Date(automationTimedUntilByKey.auto_station).toLocaleString(locale)}. Right-click to change the duration.`
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
              </AutoStationHoverPopover>
              <span className="liquid-status-dock-utilities">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onOpenAutoStationSettings}
                  className="liquid-status-dock-utility"
                  title={localizeStatic("ui.components.header.title.auto.station.settings.eb56c8a6")}
                  aria-label={localizeStatic("ui.components.header.aria-label.open.auto.station.settings.afad0824")}
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
				<span lang={messageLocale} className="liquid-header-control-label">{botLocked ? t('bot.unlock') : t('bot.lock')}</span>
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
              <span lang={messageLocale} className="liquid-header-control-label">{t('bot.reconnect')}</span>
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
              <span lang={messageLocale} className="liquid-header-control-label">
                {gameConnectionState === 'starting' ? t('bot.starting') : t('bot.start')}
              </span>
            </Button>
          )}
        </div>
      </div>
    </header>
  );
};

export default Header;
