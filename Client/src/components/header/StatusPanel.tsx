import { useEffect, useState } from 'react';
import { Settings, Trash2 } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCitadelAPI } from '../../api/ApiContext';
import { CitadelAPI } from '../../api/CitadelClient';
import type { AttackLaunchRatesV2 } from '../../api/Contracts';
import { automationDuration } from '../../i18n/automationDuration';
import { useLocale } from '../../i18n/LocaleContext';
import { useLocalizedMessage } from '../../i18n/useLocalizedMessage';
import { HostedConnectionActionsSlot } from '../../config/HostedChrome';
import { requestView } from '../../config/viewNavigation';
import { focusReadinessTargetWhenReady } from '../../settings/readiness/focusReadinessTarget';
import { AUTOMATION_FEATURE_NAMES } from '../../settings/automationFeatureNames';
import { AUTOMATION_ENABLED_KEYS, featureIdForEnabledKey } from '../../settings/disclosure/placement';
import { StatusBadge } from '../ui/StatusBadge';
import { Button, Switch } from '../ui';
import { TimedRunButton } from '../automation/TimedRunButton';
import { AutomationFeatureFeedback } from '../AutomationFeatureFeedback';
import AutoBirdCycles from '../AutoBirdCycles';
import { Notifications } from '../Notifications';
import { attacksText, featureOrder } from './headerStatus';
import type { HeaderStatus } from './StatusCluster';
export interface StatusPanelActions {
  onOpenAutoBirdSettings(): void; onOpenAutoStationSettings(): void;
  onOpenAutomationDuration(featureKey: string, featureLabel: string): void;
}
export function StatusPanel({ surface, data, open, titleId, onBeforeDialog, onOpenAutoBirdSettings, onOpenAutoStationSettings, onOpenAutomationDuration }: StatusPanelActions & {
  surface: 'desktop' | 'hosted'; data: HeaderStatus; open: boolean; titleId: string; onBeforeDialog(): void;
}) {
  const locale = useLocale(); const { t, number } = locale;
  const { submitIntent } = useCitadelAPI();
  const { autoStationEnabled, autoBirdEnabled, toggleAutoStation, toggleAutoBird, automationTimedUntilByKey, autoBirdCastleCycles, dashboardConnectionStatus, startGame, reconnectGame } = useAuth();
  const [clearingAutoBirdTracking, setClearingAutoBirdTracking] = useState(false);
  const [rates, setRates] = useState<AttackLaunchRatesV2 | null>(null);
  useEffect(() => {
    if (!open) return;
    let active = true;
    void CitadelAPI.getAttackLaunchRates().then(value => { if (active) setRates(value); }).catch(() => { if (active) setRates(null); });
    return () => { active = false; };
  }, [open]);
  const dialog = (action: () => void) => { onBeforeDialog(); action(); };
  const stationName = AUTOMATION_FEATURE_NAMES.autoStation; const birdName = AUTOMATION_FEATURE_NAMES.autoBird;
  const clearAutoBirdTracking = async () => {
    if (clearingAutoBirdTracking) return;
    onBeforeDialog();
    if (!window.confirm('Clear all Auto Bird cycle tracking from CitadelOps memory? Settings, presets, Auto Station, and game movements will be kept. Auto Bird will rebuild tracking from current game state.')) return;
    setClearingAutoBirdTracking(true);
    try { await submitIntent('auto_bird.clear_tracking', {}, { actor: 'ui:auto-bird' }); Notifications.success('Auto Bird tracking reset requested.'); }
    catch { /* The API context already presents the server error. */ }
    finally { setClearingAutoBirdTracking(false); }
  };
  const attacks = attacksText(data, locale);
  const featureCounts = data.presence.mode === 'checkpoint' ? [] : Object.entries(rates?.dailySession?.launchesByFeature ?? {}).filter(([, count]) => Number.isFinite(count) && count > 0)
    .map(([key, count]) => ({ id: featureIdForEnabledKey(key) ?? key, count })).sort((a, b) => featureOrder(a.id, b.id));
  return <div className="header-status-panel" id={titleId + '-panel'}>
    <h2 id={titleId}>{t('header.status.title')}</h2>
    <section><h3>{t('header.panel.connection')}</h3><StatusBadge {...data.connection} indicator />
      <div className="header-panel-actions">
        {surface === 'desktop' && !data.gameConnectionActive && <Button variant="primary" size="sm" disabled={!data.connectionControlsReady} onClick={() => void startGame()}>{t('bot.start')}</Button>}
        {data.gameReconnectAvailable && <Button variant="secondary" size="sm" disabled={dashboardConnectionStatus !== 'Connected'} onClick={() => void reconnectGame()}>{t('bot.reconnect')}</Button>}
        <HostedConnectionActionsSlot />
      </div>
    </section>
    <section><h3>{stationName}</h3><StatusBadge {...data.station} indicator />
      {data.stationThreatCount > 0 && <p>{t('header.signal.incoming', { count: data.stationThreatCount, state: data.stationNextImpact > 0 ? 'known' : 'other', duration: automationDuration(Math.max(0, Math.ceil((data.stationNextImpact - data.now) / 60_000)), locale.locale) })}</p>}
      <div className="header-panel-actions"><Switch checked={autoStationEnabled} onChange={toggleAutoStation} ariaLabel={stationName} disabled={dashboardConnectionStatus !== 'Connected'} />
        <TimedRunButton featureName={stationName} expiresAt={automationTimedUntilByKey.auto_station} now={data.now} disabled={dashboardConnectionStatus !== 'Connected'} onOpen={() => dialog(() => onOpenAutomationDuration('auto_station', stationName))} />
        <Button variant="ghost" iconOnly aria-label={t('ui.components.header.aria-label.open.auto.station.settings.afad0824')} onClick={() => dialog(onOpenAutoStationSettings)}><Settings aria-hidden="true" /></Button>
      </div><AutomationFeatureFeedback featureId="autoStation" enabled={autoStationEnabled} onOpenSettings={() => dialog(onOpenAutoStationSettings)} compact />
    </section>
    <section><h3>{birdName}</h3><StatusBadge {...data.bird} indicator />
      <div className="header-panel-actions"><Switch checked={autoBirdEnabled} onChange={toggleAutoBird} ariaLabel={birdName} disabled={dashboardConnectionStatus !== 'Connected'} />
        <TimedRunButton featureName={birdName} expiresAt={automationTimedUntilByKey.auto_bird} now={data.now} disabled={dashboardConnectionStatus !== 'Connected'} onOpen={() => dialog(() => onOpenAutomationDuration('auto_bird', birdName))} />
        <Button variant="ghost" iconOnly aria-label={t('ui.components.header.aria-label.open.auto.bird.settings.787f04dc')} onClick={() => dialog(onOpenAutoBirdSettings)}><Settings aria-hidden="true" /></Button>
      </div>
      <AutoBirdCycles cycles={autoBirdCastleCycles} enabled={autoBirdEnabled} canControl={dashboardConnectionStatus === 'Connected'} now={data.now} onBeforeDialog={onBeforeDialog} />
      <Button variant="danger" size="sm" disabled={clearingAutoBirdTracking} leftIcon={<Trash2 aria-hidden="true" />} onClick={() => void clearAutoBirdTracking()}>{t('ui.components.header.aria-label.clear.auto.bird.cycle.tracking.4813b36e')}</Button>
      <AutomationFeatureFeedback featureId="autoBird" enabled={autoBirdEnabled} onOpenSettings={() => dialog(onOpenAutoBirdSettings)} compact />
    </section>
    {data.attention.length > 0 && <section><h3>{t('header.panel.attention')}</h3>{data.attention.map(entry => <AttentionEntry key={entry.featureId} entry={entry} onOpen={() => { onBeforeDialog(); requestView('automation'); focusReadinessTargetWhenReady('automation-switch-' + entry.featureId); }} />)}</section>}
    <section><h3>{t('header.panel.attacksToday')}</h3><p title={attacks.title}>{attacks.text}</p>
      {featureCounts.map(entry => <p key={entry.id}><span>{AUTOMATION_FEATURE_NAMES[entry.id as keyof typeof AUTOMATION_ENABLED_KEYS] ?? entry.id}</span> · {number(entry.count)}</p>)}
    </section>
  </div>;
}
function AttentionEntry({ entry, onOpen }: { entry: HeaderStatus['attention'][number]; onOpen(): void }) {
  const { t } = useLocale(); const reason = useLocalizedMessage(entry.reason, ''); const name = AUTOMATION_FEATURE_NAMES[entry.featureId];
  return <div className="header-attention-entry"><strong>{name}</strong><StatusBadge status={entry.status} reason={entry.reason} indicator /><Button variant="ghost" size="sm" title={reason.text} onClick={onOpen}>{t('header.panel.openFeature', { feature: name })}</Button></div>;
}
