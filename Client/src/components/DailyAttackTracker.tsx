import React from 'react';
import { Gauge } from 'lucide-react';
import { useLocale } from '../i18n/useLocale';
import { useCitadelAPI } from '../api/useCitadelAPI';
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
    ? saved ? t('copy.attacksSaved', { count, time: savedTime }) : t('copy.attacksToday', { count: number(count) })
    : '—';
  const title = known
    ? saved ? t('copy.offlineSavedTitle', { time: savedTime }) : t('dailyAttacks.observed', { count, observedAt: observedAtMs })
    : t('copy.countUnknownTitle');

  return (
    <div
      lang={messageLocale}
      className={`liquid-status-dock-item liquid-daily-attacks-dock ${known ? 'liquid-status-dock-item-primary' : 'liquid-status-dock-item-muted'}`}
      title={title}
      aria-label={known ? formattedCount : title}
    >
      <span className="liquid-status-dock-icon" aria-hidden="true"><Gauge className="h-4 w-4" /></span>
      <span className="liquid-desktop-status-label">{formattedCount}</span>
    </div>
  );
};

export default DailyAttackTracker;
