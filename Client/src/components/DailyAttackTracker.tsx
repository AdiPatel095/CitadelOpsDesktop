import React from 'react';
import { Gauge } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';

const DailyAttackTracker: React.FC = () => {
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
  const formattedCount = synced ? count.toLocaleString() : '--';
  const title = synced
    ? `The server's account-wide normal-attack count is ${count.toLocaleString()}. Last observed ${new Date(observedAtMs).toLocaleString()}. Advisor attacks are exempt.`
    : 'Waiting for the server daily attack counter.';

  return (
    <div
      className={`liquid-status-dock-item liquid-daily-attacks-dock ${synced ? 'liquid-status-dock-item-primary' : 'liquid-status-dock-item-muted'}`}
      title={title}
      aria-label={synced
        ? `Daily attacks: ${formattedCount}`
        : 'Daily attacks: waiting for server count'}
    >
      <span className="liquid-status-dock-icon" aria-hidden="true">
        <Gauge className="h-4 w-4" />
      </span>
      <span className="liquid-desktop-status-label">Daily attacks</span>
      <span className="liquid-daily-attacks-value font-mono tabular-nums">{formattedCount}</span>
    </div>
  );
};

export default DailyAttackTracker;
