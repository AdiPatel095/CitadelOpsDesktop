import React from 'react';
import { Gauge } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { Badge } from './ui';

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
    <Badge
      variant={synced ? 'primary' : 'secondary'}
      className="shrink-0 gap-2 px-3 py-1.5"
      title={title}
      aria-label={synced
        ? `Daily attacks: ${formattedCount}`
        : 'Daily attacks: waiting for server count'}
    >
      <Gauge className="h-3.5 w-3.5" aria-hidden="true" />
      <span className="text-[9px] font-bold uppercase tracking-wider opacity-80">Daily attacks</span>
      <span className="font-mono text-xs tabular-nums">{formattedCount}</span>
    </Badge>
  );
};

export default DailyAttackTracker;
