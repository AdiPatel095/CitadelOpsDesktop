import { useState } from 'react';
import type { AutomationStateV2 } from '../api/Contracts';
import { useCitadelAPI } from '../api/ApiContext';
import { Button } from './ui';

// Go's zero time is serialized for value timestamps; it is not an expiry/clear.
function timestamp(value?: string): number {
  return value && !value.startsWith('0001-') ? Date.parse(value) : 0;
}

export function AutomationSafetyPanel({ states, now }: {
  states: Record<string, AutomationStateV2>;
  now: number;
}) {
  const { submitIntent, refreshState } = useCitadelAPI();
  const [reviews, setReviews] = useState<Record<string, string>>({});
  const [pending, setPending] = useState<string>();
  const [error, setError] = useState('');
  const locked = Object.entries(states).filter(([, state]) => {
    const lock = state.safetyLock;
    return lock?.operationId && !timestamp(lock.clearedAt)
      && (!timestamp(lock.until) || timestamp(lock.until) > now);
  });
  if (locked.length === 0) return null;

  async function clear(lane: string, operationId: string) {
    const key = `${lane}:${operationId}`;
    setPending(key);
    setError('');
    try {
      const receipt = await submitIntent('automation.safety.clear', {
        lane, operationId, review: reviews[key]?.trim(),
      });
      if (receipt.status !== 'succeeded') {
        throw new Error(receipt.error || 'The safety lock was not cleared.');
      }
      await refreshState();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to clear the safety lock.');
    } finally {
      setPending(undefined);
    }
  }

  return (
    <section className="rounded-xl border border-amber-500/50 bg-amber-500/5 p-4" aria-label="Automation safety locks">
      <h2 className="font-semibold text-text-main">Automation safety locks</h2>
      <p className="mt-1 text-sm text-text-muted">These lanes have stopped after a game rejection. Review the operation and game state before allowing another attempt. Clearing a lock does not mark the error as safe.</p>
      {locked.map(([lane, state]) => {
        const lock = state.safetyLock!;
        const key = `${lane}:${lock.operationId}`;
        return (
          <div key={key} className="mt-4 space-y-2 border-t border-amber-500/20 pt-3">
            <p className="font-medium text-text-main">{lane} — {lock.opcode.toUpperCase()} {lock.code}</p>
            <p className="break-all text-sm text-text-muted">Operation {lock.operationId} · {lock.intent} · {new Date(lock.observedAt).toLocaleString()}</p>
            <p className="text-sm text-text-muted">{timestamp(lock.until) ? `MSD cooldown ends ${new Date(lock.until!).toLocaleString()}.` : 'Held until explicitly reviewed and cleared.'}</p>
            {!timestamp(lock.until) && <><label className="block text-sm text-text-main">
              Review and reason to resume
              <textarea className="mt-1 block w-full rounded border border-amber-500/30 bg-transparent p-2" maxLength={1000} rows={2}
                value={reviews[key] ?? ''} disabled={pending !== undefined}
                onChange={(event) => setReviews((current) => ({ ...current, [key]: event.target.value }))} />
            </label>
            <Button disabled={pending !== undefined || !reviews[key]?.trim()} onClick={() => void clear(lane, lock.operationId)}>
              {pending === key ? 'Clearing…' : 'Clear reviewed lock and allow lane to resume'}
            </Button>
            </>}
          </div>
        );
      })}
      {error && <p role="alert" className="mt-3 text-sm text-red-400">{error}</p>}
    </section>
  );
}
