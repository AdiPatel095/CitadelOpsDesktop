import React from 'react';
import { Gauge } from 'lucide-react';
import { useLocale } from '../i18n/LocaleContext';
import { useCitadelAPI } from '../api/ApiContext';

const DailyAttackTracker: React.FC = () => {
  const { t, number, messageLocale } = useLocale();
  const { state } = useCitadelAPI();
  const dailyAttacks = state?.dailyAttacks;
  const observedAt = dailyAttacks?.observedAt;
  const observedAtMs = observedAt ? Date.parse(observedAt) : Number.NaN;
  const synced = Boolean(
    observedAt &&
    !observedAt.startsWith('0001-01-01') &&
    Number.isFinite(observedAtMs),
  );
  const count = Math.max(0, Math.trunc(dailyAttacks?.count ?? 0));
  const formattedCount = synced ? number(count) : '--';
  const title = synced
    ? t('dailyAttacks.observed', { count, observedAt: observedAtMs })
    : t('dailyAttacks.waiting');

  return (
    <div
      lang={messageLocale}
      className={`liquid-status-dock-item liquid-daily-attacks-dock ${synced ? 'liquid-status-dock-item-primary' : 'liquid-status-dock-item-muted'}`}
      title={title}
      aria-label={synced
        ? t('dailyAttacks.accessible', { count })
        : t('dailyAttacks.accessibleWaiting')}
    >
      <span className="liquid-status-dock-icon" aria-hidden="true">
        <Gauge className="h-4 w-4" />
      </span>
      <span className="liquid-desktop-status-label">{t('dailyAttacks.label')}</span>
      <span className="liquid-daily-attacks-value font-mono tabular-nums">{formattedCount}</span>
    </div>
  );
};

export default DailyAttackTracker;
