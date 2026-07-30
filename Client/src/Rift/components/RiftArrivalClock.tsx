import React, { useEffect, useMemo, useState } from 'react';
import { Clock, Minus, Plus } from 'lucide-react';
import { Button } from '../../components/ui';
import type { RiftCRALaunchEntry } from '../types/RiftCRALaunch';
import {
  arriveAtUnixFromOffset,
  formatLocalArrivalFromUnix,
  formatTravelDuration,
  isEarliestOffset,
  launchAtUnix,
  minArriveAtUnix,
  offsetMinutesFromScheduled,
  stepArrivalOffsetMinutes,
} from '../types/RiftArrivalTime';

interface RiftArrivalClockProps {
  entry: RiftCRALaunchEntry;
  offsetMinutes: number;
  onOffsetChange: (offsetMinutes: number) => void;
}

const RiftArrivalClock: React.FC<RiftArrivalClockProps> = ({ entry, offsetMinutes, onOffsetChange }) => {
  const [nowMs, setNowMs] = useState(() => Date.now());

  useEffect(() => {
    const id = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  const minUnix = useMemo(() => minArriveAtUnix(entry, nowMs), [entry, nowMs]);
  const arriveAtUnix = useMemo(
    () => arriveAtUnixFromOffset(entry, offsetMinutes, nowMs),
    [entry, offsetMinutes, nowMs]
  );
  const atEarliest = isEarliestOffset(offsetMinutes);

  useEffect(() => {
    if (entry.scheduledArriveAtUnix == null || entry.scheduledArriveAtUnix <= 0) return;
    onOffsetChange(offsetMinutesFromScheduled(entry, entry.scheduledArriveAtUnix, nowMs));
  }, [entry.scheduledArriveAtUnix, entry.id, nowMs, onOffsetChange]);

  if (minUnix == null || arriveAtUnix == null) {
    return (
      <span className="text-xs text-text-muted whitespace-nowrap" title="Complete a successful feather launch to unlock timing">
        No TT yet
      </span>
    );
  }

  const fireUnix = launchAtUnix(arriveAtUnix, entry.oneWayTTSeconds);

  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex items-center gap-1">
        <Button
          variant="outline"
          size="sm"
          className="h-8 w-8 p-0"
          disabled={atEarliest}
          onClick={() => onOffsetChange(stepArrivalOffsetMinutes(offsetMinutes, -1))}
          title={atEarliest ? 'Already at earliest feather arrival' : '−1 min from earliest'}
          aria-label="Decrease offset by one minute"
        >
          <Minus className="h-3.5 w-3.5" />
        </Button>
        <button
          type="button"
          disabled={atEarliest}
          onClick={() => onOffsetChange(0)}
          aria-label={atEarliest ? 'Earliest Rift arrival selected' : 'Reset to earliest Rift arrival'}
          className="flex min-w-[5.5rem] items-center justify-center gap-1 rounded-md border border-border-base bg-bg-card/60 px-2 py-1 text-sm text-text-main transition-colors enabled:hover:border-primary/50 enabled:hover:bg-primary/10 disabled:cursor-default"
          title={
            atEarliest
              ? `Earliest feather arrival (${formatLocalArrivalFromUnix(minUnix)}) · TT ${formatTravelDuration(entry.oneWayTTSeconds)}`
              : `Reset to earliest arrival · currently ${formatLocalArrivalFromUnix(arriveAtUnix)} (+${offsetMinutes}m) · TT ${formatTravelDuration(entry.oneWayTTSeconds)}${
                  fireUnix != null ? ` · launch ${formatLocalArrivalFromUnix(fireUnix)}` : ''
                }`
          }
        >
          <Clock className="h-3.5 w-3.5 text-text-muted shrink-0" />
          <span className="font-mono">{formatLocalArrivalFromUnix(arriveAtUnix)}</span>
        </button>
        <Button
          variant="outline"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={() => onOffsetChange(stepArrivalOffsetMinutes(offsetMinutes, 1))}
          title="+1 min after earliest (minute boundary)"
          aria-label="Increase offset by one minute"
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
      <span className="text-[10px] text-text-muted">
        {atEarliest ? 'earliest feather arrival' : `+${offsetMinutes}m · click time to reset`}
      </span>
    </div>
  );
};

export default RiftArrivalClock;
