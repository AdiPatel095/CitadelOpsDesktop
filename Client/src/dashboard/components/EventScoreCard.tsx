import React, { useEffect, useMemo, useState } from 'react';
import { Trophy } from 'lucide-react';
import { CitadelAPI } from '../../api/CitadelClient';
import type { ScalableEventScoreV2 } from '../../api/Contracts';
import { Button, MetricTile, SectionCard } from '../../components/ui';

interface EventScoreCardProps {
  live: boolean;
	event?: ScalableEventScoreV2;
	onOpenRanking?: () => void;
	rankingLoading?: boolean;
}

const EventScoreCard: React.FC<EventScoreCardProps> = ({ live, event, onOpenRanking, rankingLoading = false }) => {
  const [translations, setTranslations] = useState<Record<string, string>>({});
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!event) return;
    const keys = [
      event.localizationKey,
      event.difficultyTypeName ? `dialog_difficultyScaling_${event.difficultyTypeName}` : undefined,
    ].filter((key): key is string => Boolean(key));
    if (keys.length === 0) return;
    let cancelled = false;
    void CitadelAPI.localize(keys).then((values) => {
      if (!cancelled) setTranslations(values);
    }).catch(() => undefined);
    return () => { cancelled = true; };
  }, [event?.difficultyTypeName, event?.localizationKey]);

  useEffect(() => {
    if (!event || (event.remainingSec ?? 0) <= 0) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [event]);

  const remainingSec = useMemo(() => {
    if (!event || (event.remainingSec ?? 0) <= 0) return 0;
    const observedAt = Date.parse(event.observedAt);
    const elapsed = Number.isFinite(observedAt) ? Math.max(0, Math.floor((now - observedAt) / 1000)) : 0;
    return Math.max(0, (event.remainingSec ?? 0) - elapsed);
  }, [event, now]);

  const eventName = event
    ? translations[event.localizationKey] || knownEventName(event.eventId) || event.name || `Event ${event.eventId}`
    : 'Active event';
  const difficulty = event ? difficultyLabel(event, translations) : '';
  const canOpenRanking = event?.eventId === 72 && (event.allianceLeagueId ?? 0) > 0;

  return (
    <SectionCard
      variant="glass"
      title={eventName}
      description="Live event score and reward progress"
      titleClassName="truncate text-primary"
      descriptionClassName="font-bold uppercase tracking-wider"
      className="flex min-h-0 flex-col"
      actions={(
        <div className="flex flex-wrap items-center justify-end gap-2">
          {event && onOpenRanking && canOpenRanking && (
            <Button
              type="button"
              variant="glass"
              size="sm"
              leftIcon={<Trophy className="h-3.5 w-3.5" />}
              isLoading={rankingLoading}
              disabled={!live}
              onClick={onOpenRanking}
              title="Open the live GGE Nomad alliance ranking"
            >
              Alliance ranking
            </Button>
          )}
          <span className={`rounded-full border px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider ${live ? 'border-success/30 bg-success/10 text-success' : 'border-border-light bg-bg-card/50 text-text-muted'}`}>
            {live ? 'Live' : 'Last known'}
          </span>
        </div>
      )}
    >
        {!event ? (
          <div className="rounded-global border border-dashed border-border-light bg-bg-card/35 px-4 py-7 text-center backdrop-blur-xl">
            <p className="text-sm font-medium text-text-main">No supported event is active.</p>
            <p className="mx-auto mt-2 max-w-xl text-xs text-text-muted">The page will switch automatically when a supported event begins.</p>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
              <MetricTile label="Your score" value={event.playerScore} tone="brand" className="border-border-light bg-bg-card/40 px-4 py-3 backdrop-blur-xl [&_.ui-metric-value]:truncate [&_.ui-metric-value]:text-xl" />
              <MetricTile label="Your rank" value={formatRank(event.playerRank)} className="border-border-light bg-bg-card/40 px-4 py-3 backdrop-blur-xl [&_.ui-metric-value]:truncate [&_.ui-metric-value]:text-xl" />
              <MetricTile label="Alliance score" value={event.allianceScore} className="border-border-light bg-bg-card/40 px-4 py-3 backdrop-blur-xl [&_.ui-metric-value]:truncate [&_.ui-metric-value]:text-xl" />
              <MetricTile label="Alliance rank" value={formatRank(event.allianceRank)} className="border-border-light bg-bg-card/40 px-4 py-3 backdrop-blur-xl [&_.ui-metric-value]:truncate [&_.ui-metric-value]:text-xl" />
              <MetricTile
                label="Reward pages"
                value={formatRewardPages(event.rewardPagesReached, event.rewardPagesTotal)}
                tone="success"
                caption={rewardPagesCaption(event)}
                className="col-span-2 border-border-light bg-bg-card/40 px-4 py-3 backdrop-blur-xl [&_.ui-metric-value]:truncate [&_.ui-metric-value]:text-xl md:col-span-1"
              />
            </div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-border-base/60 pt-3 text-xs text-text-muted">
              <span>Difficulty: <strong className="font-semibold text-text-main">{difficulty}</strong></span>
              {remainingSec > 0 && <span>Ends in: <strong className="font-mono font-semibold tabular-nums text-text-main">{formatRemaining(remainingSec)}</strong></span>}
              {(event.leagueId ?? 0) > 0 && <span>League: <strong className="font-semibold text-text-main">{event.leagueId}</strong></span>}
              <span>Updated: <strong className="font-semibold text-text-main">{formatObservedAt(event.observedAt)}</strong></span>
            </div>
          </div>
        )}
    </SectionCard>
  );
};

function knownEventName(eventID: number): string {
  switch (eventID) {
    case 71:
      return 'Foreign Lords Invasion';
    case 72:
      return 'Nomad Invasion';
    case 80:
      return 'Samurai Invasion';
    case 103:
      return 'Bloodcrow Invasion';
    default:
      return '';
  }
}

function difficultyLabel(event: { difficultyId?: number; difficultyTypeName?: string }, translations: Record<string, string>): string {
  if (!event.difficultyId || event.difficultyId < 0) return 'Not selected';
  const key = event.difficultyTypeName ? `dialog_difficultyScaling_${event.difficultyTypeName}` : '';
  if (key && translations[key]) return translations[key];
  if (!event.difficultyTypeName) return `Difficulty ${event.difficultyId}`;
  return event.difficultyTypeName
    .replace(/Plus$/, '+')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/^./, (value) => value.toUpperCase());
}

function formatRank(rank: number): string {
  return rank > 0 ? `#${rank.toLocaleString()}` : '—';
}

function formatRewardPages(reached = 0, total = 0): string {
  if (total <= 0) return '—';
  return `${Math.max(0, reached).toLocaleString()} / ${total.toLocaleString()}`;
}

function rewardPagesCaption(event: ScalableEventScoreV2): string {
  if ((event.rewardPagesTotal ?? 0) <= 0) {
    return (event.difficultyId ?? -1) < 0 ? 'Select a difficulty to load rewards' : 'Reward pages unavailable';
  }
  if ((event.nextRewardScore ?? 0) > 0) return `Next at ${event.nextRewardScore?.toLocaleString()} points`;
  return 'All selected-difficulty pages reached';
}

function formatObservedAt(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Unknown';
  return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' });
}

function formatRemaining(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  if (days > 0) return `${days}d ${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h ${minutes}m ${secs}s`;
  return `${minutes}m ${secs}s`;
}

export default EventScoreCard;
