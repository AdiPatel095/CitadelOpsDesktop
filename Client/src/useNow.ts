import { useEffect, useState } from 'react';

/** A render-safe clock; callers choose the cadence of their existing time displays. */
export function useNow(refreshMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(timer);
  }, [refreshMs]);
  return now;
}
