import React from 'react';
import { Gauge } from 'lucide-react';
import { useLocale } from '../i18n/LocaleContext';
import { useCitadelAPI } from '../api/ApiContext';
import { useHostedRuntimePresence } from '../config/Deployment';

const DailyAttackTracker: React.FC = () => {
  const { t, number, messageLocale, locale } = useLocale();
  const { state } = useCitadelAPI();
  const presence = useHostedRuntimePresence();
  const dailyAttacks = state?.dailyAttacks;
  const observedAt = dailyAttacks?.observedAt;
  const observedAtMs = observedAt ? Date.parse(observedAt) : Number.NaN;
  const synced = Boolean(
    observedAt && !observedAt.startsWith('0001-01-01') && Number.isFinite(observedAtMs)
    && typeof dailyAttacks?.count === 'number' && Number.isFinite(dailyAttacks.count),
  );
  const count = synced ? Math.max(0, Math.trunc(dailyAttacks!.count)) : 0;
  const savedAt = presence.checkpointObservedAt ? Date.parse(presence.checkpointObservedAt) : Number.NaN;
  const saved = presence.mode === 'checkpoint';
  const known = synced && (!saved || Number.isFinite(savedAt));
  const savedTime = Number.isFinite(savedAt)
    ? new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(savedAt) : '';
  const formattedCount = known
    ? saved ? t('dailyAttacks.saved', { count, time: savedTime }) : number(count)
    : '—';
  const title = known
    ? saved ? t('dailyAttacks.saved', { count, time: savedTime }) : t('dailyAttacks.observed', { count, observedAt: observedAtMs })
    : t('attackCounts.unknownTitle');

  return (
    <div
      lang={messageLocale}
      className={`liquid-status-dock-item liquid-daily-attacks-dock ${known ? 'liquid-status-dock-item-primary' : 'liquid-status-dock-item-muted'}`}
      title={title}
      aria-label={known ? saved ? title : t('dailyAttacks.accessible', { count }) : title}
    >
      <span className="liquid-status-dock-icon" aria-hidden="true"><Gauge className="h-4 w-4" /></span>
      <span className="liquid-desktop-status-label">{saved ? formattedCount : t('dailyAttacks.label')}</span>
      {!saved ? <span className="liquid-daily-attacks-value font-mono tabular-nums">{formattedCount}</span> : null}
    </div>
  );
};

export default DailyAttackTracker;
