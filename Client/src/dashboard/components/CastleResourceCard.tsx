import React, { useEffect, useMemo, useState } from 'react';
import type { ResourceBalanceV2 } from '../../api/Contracts';
import { SectionCard } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';

interface CastleResourceCardProps {
  title: string;
  resources: Record<string, ResourceBalanceV2>;
}

const MS_PER_HOUR = 3600 * 1000;
const EXTREME_HOURS = 24 * 365 * 10;
const SEC_PER_HOUR = 3600;
const SEC_PER_DAY = 86400;

function formatRemainingMs(ms: number): string {
  const totalSec = Math.max(0, Math.floor(ms / 1000));
  if (totalSec >= SEC_PER_HOUR) {
    const days = Math.floor(totalSec / SEC_PER_DAY);
    const rem = totalSec % SEC_PER_DAY;
    const hours = Math.floor(rem / SEC_PER_HOUR);
    const minutes = Math.floor((rem % SEC_PER_HOUR) / 60);
    if (days > 0) return [`${days}d`, hours > 0 ? `${hours}h` : '', minutes > 0 ? `${minutes}m` : ''].filter(Boolean).join(' ');
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }
  const minutes = Math.floor(totalSec / 60);
  const seconds = totalSec % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}

function ResourceDepletionTimer({ amount, netPerHour }: { amount: number; netPerHour: number }) {
  const [deadlineMs, setDeadlineMs] = useState<number | null>(null);
  const [extremeLong, setExtremeLong] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setExtremeLong(false);
    if (netPerHour >= -1e-9 || amount <= 0) {
      setDeadlineMs(null);
      return;
    }
    const hours = amount / -netPerHour;
    if (!Number.isFinite(hours) || hours <= 0) {
      setDeadlineMs(null);
      return;
    }
    if (hours > EXTREME_HOURS) {
      setDeadlineMs(null);
      setExtremeLong(true);
      return;
    }
    setDeadlineMs(Date.now() + hours * MS_PER_HOUR);
  }, [amount, netPerHour]);

  useEffect(() => {
    if (deadlineMs == null) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [deadlineMs]);

  if (extremeLong) return <p className="w-full text-right text-[10px] leading-tight text-text-muted tabular-nums">&gt;10y</p>;
  if (deadlineMs == null || deadlineMs <= now) return null;
  return <p className="w-full text-right text-[10px] leading-tight text-text-muted tabular-nums">{formatRemainingMs(deadlineMs - now)}</p>;
}

const CastleResourceCard: React.FC<CastleResourceCardProps> = ({ title, resources }) => {
  const { resources: definitions } = useMetadata();
  const rows = useMemo(() => Object.entries(resources)
    .map(([rawID, balance]) => ({ id: Number(rawID), balance, definition: definitions[Number(rawID)] }))
    .filter((row) => Number.isFinite(row.id) && row.id > 0)
    .sort((left, right) => right.id - left.id), [definitions, resources]);

  return (
    <SectionCard
      variant="solid"
      title={title}
      titleClassName="text-primary"
      className="flex min-h-0 flex-col"
      contentClassName="custom-scrollbar flex flex-col gap-2 overflow-y-auto"
    >
        {rows.map(({ id, balance, definition }) => {
          const amount = balance.amount ?? 0;
          const capacity = balance.capacity ?? 0;
          const production = balance.productionPerHour ?? 0;
          const percentage = capacity > 0 ? amount / capacity * 100 : 0;
          const internalName = typeof definition?.internalName === 'string' ? definition.internalName : '';
          const name = definition?.name || internalName || `Resource ${id}`;
						const icon = definition?.image;
          return (
            <div key={id} className="flex items-center gap-3 rounded-global border border-border-light bg-bg-card/45 p-2.5 shadow-sm transition-colors hover:border-primary/30 hover:bg-bg-card-hover/70">
              {icon ? <img src={icon} alt={name} className="h-8 w-8 shrink-0 object-contain drop-shadow-sm" /> : null}
              <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                <div className="flex items-center justify-between text-xs font-medium text-text-main">
                  <span className="mr-2 truncate" title={name}>{amount.toLocaleString()} / {capacity.toLocaleString()}</span>
                  <span className={`shrink-0 font-semibold ${production < 0 ? 'text-error' : 'text-success'}`}>
                    ({production > 0 ? '+' : ''}{production.toLocaleString()}/hr)
                  </span>
                </div>
                <ResourceDepletionTimer amount={amount} netPerHour={production} />
                <div className="h-1.5 w-full overflow-hidden rounded-full border border-border-base/50 bg-bg-app/55 shadow-inner">
                  <div className="h-full bg-primary transition-all duration-500 ease-out" style={{ width: `${Math.min(100, Math.max(0, percentage))}%` }} />
                </div>
              </div>
            </div>
          );
        })}
    </SectionCard>
  );
};

export default CastleResourceCard;
