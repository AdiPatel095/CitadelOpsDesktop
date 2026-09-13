import { useEffect, useMemo, useState } from 'react';
import { History } from 'lucide-react';
import { CitadelAPI } from '../../api/CitadelClient';
import type { WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';
import { Button, Card, CardContent, EmptyState, Select } from '../../components/ui';
import { formatEventEndLocal } from '../../worldIntelligence/components/WorldEventFinals';
import { featureEventFinals } from './FeatureEventScores';

const pageSize = 10;
const emptyHistory: WorldIntelligenceEventScoreObservationV1[] = [];

export function useFeatureEventHistory(worldId: string, playerId: number) {
  const scope = `${worldId}:${playerId}`;
  const [result, setResult] = useState<{
    scope: string; entries: WorldIntelligenceEventScoreObservationV1[]; loading: boolean; error: string;
  }>({ scope: '', entries: [], loading: false, error: '' });
  useEffect(() => {
    if (!worldId || playerId <= 0) return;
    let cancelled = false;
    let inFlight = false;
    setResult({ scope, entries: [], loading: true, error: '' });
    const refresh = async () => {
      if (inFlight) return;
      inFlight = true;
      try {
        const history = await CitadelAPI.getWorldIntelligencePlayerEventScores({ worldId, playerId, limit: 5_000 });
        if (history.playerId !== playerId || history.worldId?.toLowerCase() !== worldId.toLowerCase() || !Array.isArray(history.history)) throw new Error('Event history identity mismatch');
        if (!cancelled) setResult({ scope, entries: history.history, loading: false, error: '' });
      } catch {
        if (!cancelled) setResult((previous) => ({
          ...previous, scope, loading: false,
          error: 'Previous event scores are temporarily unavailable. Retrying automatically.',
        }));
      } finally {
        inFlight = false;
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 60_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [playerId, scope, worldId]);
  // Never show the previous account's rows while the new effect is starting.
  return result.scope === scope ? result : { entries: emptyHistory, loading: Boolean(worldId && playerId > 0), error: '' };
}

export function FeatureEventHistory({ entries, worldId, playerId, now, loading, error, eventIds }: {
  entries: WorldIntelligenceEventScoreObservationV1[];
  worldId: string;
  playerId: number;
  now: number;
  loading: boolean;
  error: string;
  eventIds?: readonly number[];
}) {
  const [page, setPage] = useState(0);
  const [eventFilter, setEventFilter] = useState('all');
  const finals = useMemo(() => featureEventFinals(entries, worldId, playerId, now)
    .filter((entry) => !eventIds || eventIds.includes(entry.eventId)), [entries, eventIds, now, playerId, worldId]);
  const eventOptions = [...new Map(finals.map((entry) => [String(entry.eventId), entry.eventName || entry.eventKey.replaceAll('-', ' ')])).entries()]
    .map(([value, label]) => ({ value, label })).sort((left, right) => left.label.localeCompare(right.label));
  const selectedEvent = eventOptions.some((option) => option.value === eventFilter) ? eventFilter : 'all';
  const filtered = finals.filter((entry) => selectedEvent === 'all' || String(entry.eventId) === selectedEvent);
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(page, pages - 1);
  const visible = filtered.slice(safePage * pageSize, (safePage + 1) * pageSize);
  return <Card><CardContent>
    <div className="mb-4">
      <div className="flex items-center gap-2 font-bold text-text-main"><History className="h-5 w-5 text-primary" /> Previous event scores</div>
      <p className="mt-1 text-xs text-text-muted">Final known account score for each collected, completed event run—not points attributed to an automation. Newest first; dates use this device’s local time.</p>
    </div>
    {eventOptions.length > 1 && <div className="mb-4 w-full sm:w-72"><Select ariaLabel="Filter previous scores by event" value={selectedEvent} onChange={(value) => { setEventFilter(value); setPage(0); }} options={[{ value: 'all', label: 'All previous events' }, ...eventOptions]} menuGrowToViewport /></div>}
    {error && <p role="status" className="mb-4 text-sm text-warning">{error}</p>}
    {loading ? <p role="status" className="text-sm text-text-muted">Loading previous scores…</p> : finals.length === 0 ? (
      <EmptyState size="sm" surface="plain" title="No previous scores recorded" description="Completed events appear here when a known score was collected for this account. Running events and unknown scores are not shown as finals." />
    ) : <>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead><tr className="border-b border-border-base text-left text-xs text-text-muted">
            <th scope="col" className="px-3 py-2">Event</th><th scope="col" className="px-3 py-2">Ended</th>
            <th scope="col" className="px-3 py-2 text-right">Final known score</th><th scope="col" className="px-3 py-2 text-right">Rank</th>
          </tr></thead>
          <tbody>{visible.map((entry) => <tr key={entry.occurrenceId} className="border-b border-border-base/50">
            <td className="px-3 py-3 font-semibold text-text-main">{entry.eventName || entry.eventKey.replaceAll('-', ' ')}</td>
            <td className="whitespace-nowrap px-3 py-3 text-text-muted">{formatEventEndLocal(entry.eventEndsAt)}</td>
            <td className="whitespace-nowrap px-3 py-3 text-right tabular-nums text-text-main">{entry.score?.toLocaleString()} <span className="text-xs text-text-muted">{entry.scoreUnit || 'points'}</span></td>
            <td className="px-3 py-3 text-right tabular-nums text-text-muted">{entry.rank > 0 ? `#${entry.rank.toLocaleString()}` : '—'}</td>
          </tr>)}</tbody>
        </table>
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-text-muted">
        <span>{filtered.length.toLocaleString()} completed runs · Page {safePage + 1} of {pages}</span>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" disabled={safePage === 0} onClick={() => setPage(safePage - 1)}>Previous</Button>
          <Button variant="secondary" size="sm" disabled={safePage + 1 >= pages} onClick={() => setPage(safePage + 1)}>Next</Button>
        </div>
      </div>
    </>}
  </CardContent></Card>;
}
