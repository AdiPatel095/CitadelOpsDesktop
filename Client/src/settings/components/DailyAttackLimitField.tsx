import React from 'react';
import { Gauge } from 'lucide-react';
import type { DailyAttackStateV2 } from '../../api/Contracts';
import { Card, Input } from '../../components/ui';

interface DailyAttackLimitFieldProps {
  value: number;
  onChange: (value: number) => void;
  serverState?: DailyAttackStateV2;
}

export const DailyAttackLimitField: React.FC<DailyAttackLimitFieldProps> = ({ value, onChange, serverState }) => {
  const synced = Boolean(serverState?.observedAt && !serverState.observedAt.startsWith('0001-01-01'));
  return (
    <Card variant="solid" className="p-4">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-black text-text-main">
            <Gauge className="h-4 w-4 text-primary" /> Daily normal-attack limit
          </div>
          <p className="mt-1 text-xs text-text-muted">
            Stop this automation when the server&apos;s account-wide daily attack count reaches this value. It resumes automatically when the server count resets. Advisor attacks are exempt.
          </p>
          <p className="mt-2 text-[11px] text-text-muted">
            {synced
              ? `Server count ${serverState?.count.toLocaleString()} · game threshold ${serverState?.serverThreshold.toLocaleString()}`
              : 'Waiting for the server daily attack counter.'}
          </p>
        </div>
        <label className="block w-full shrink-0 sm:w-48">
          <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Attack count · 0 disables</span>
          <Input
            type="text"
            inputMode="numeric"
            autoComplete="off"
            value={value.toLocaleString()}
            onChange={(event) => {
              const digits = event.target.value.replace(/\D/g, '');
              onChange(digits ? Math.min(Number.MAX_SAFE_INTEGER, Number.parseInt(digits, 10)) : 0);
            }}
            className="font-mono"
          />
        </label>
      </div>
    </Card>
  );
};
