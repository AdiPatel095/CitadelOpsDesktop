import { Lock, Unlock, Menu } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { useLocale } from '../i18n/LocaleContext';
import { HostedHeaderChromeSlot } from '../config/HostedChrome';
import { Button } from './ui';
import CastleFocusSwitcher from './CastleFocusSwitcher';
import { StatusCluster } from './header/StatusCluster';
import { useHeaderStatus } from './header/useHeaderStatus';
interface HeaderProps {
  onOpenAutoBirdSettings(): void; onOpenAutoStationSettings(): void;
  onOpenAutomationDuration(featureKey: string, featureLabel: string): void;
  onOpenNavigation(): void; navigationOpen: boolean;
}
export default function Header({ onOpenNavigation, navigationOpen, ...actions }: HeaderProps) {
  const { t } = useLocale(); const { theme } = useTheme();
  const { botLocked, toggleBotLock, dashboardConnectionStatus, startGame, reconnectGame, gameConnectionState } = useAuth();
  const surface = 'desktop';
  const data = useHeaderStatus(surface);
  return <header className="liquid-header"><div className="liquid-header-inner">
    <Button variant="ghost" iconOnly className="liquid-mobile-nav-trigger" onClick={onOpenNavigation} aria-label={t('ui.components.header.aria-label.open.workspace.navigation.9df22e36')} aria-expanded={navigationOpen} aria-controls="workspace-navigation"><Menu aria-hidden="true" /></Button>
    <div className="liquid-brand"><div className="liquid-brand-mark" data-brand-mark><img src={theme === 'light' ? '/logo-light.svg' : '/logo-dark.svg'} alt={t('ui.components.header.alt.citadel.ops.logo.ab367a3c')} width="28" height="28" /></div>
      <div className="liquid-brand-copy"><div className="text-body-lg font-semibold">CitadelOps</div><div className="text-caption">{t('navigation.commandCenter')}</div></div>
    </div>
    <div className="liquid-castle-focus-slot"><CastleFocusSwitcher /></div>
    <StatusCluster {...actions} surface={surface} data={data} />
    <div className="liquid-header-controls">
      {surface === 'desktop' && <Button variant="secondary" size="sm" className="header-lock" aria-label={botLocked ? t('bot.unlock') : t('bot.lock')} aria-pressed={botLocked} disabled={dashboardConnectionStatus !== 'Connected'} onClick={toggleBotLock} leftIcon={botLocked ? <Lock aria-hidden="true" /> : <Unlock aria-hidden="true" />}><span className="liquid-header-control-label">{botLocked ? t('bot.unlock') : t('bot.lock')}</span></Button>}
      {surface === 'desktop' && data.gameReconnectAvailable && <Button variant="secondary" size="sm" className="header-wide-action" disabled={dashboardConnectionStatus !== 'Connected'} onClick={() => void reconnectGame()}>{t('bot.reconnect')}</Button>}
      {surface === 'desktop' && !data.gameConnectionActive && <Button variant="primary" size="sm" className="header-wide-action" disabled={!data.connectionControlsReady} onClick={() => void startGame()}>{gameConnectionState === 'starting' ? t('bot.starting') : t('bot.start')}</Button>}
      <HostedHeaderChromeSlot />
    </div>
  </div></header>;
}
