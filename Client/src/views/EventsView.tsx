import React, { useCallback, useEffect, useState } from 'react';
import StaleSessionBanner from '../components/StaleSessionBanner';
import EventScoreCard from '../dashboard/components/EventScoreCard';
import { useAuth } from '../context/AuthContext';
import { PillSelector } from '../components/ui';
import { useCitadelAPI } from '../api/ApiContext';
import EventActivityCard from '../events/components/EventActivityCard';
import EventRankingModal from '../events/components/EventRankingModal';
import { FeatureEventHistory, useFeatureEventHistory } from '../events/components/FeatureEventHistory';
import { featureEventIds, isFeatureEventRunning } from '../events/components/FeatureEventScores';
import AttackEconomyView, {
  attackEconomyFeatureDefinitions,
  type AttackEconomyFeatureID,
} from '../attackAnalytics/components/AttackEconomyView';

type EventsAnalyticsView = 'events' | AttackEconomyFeatureID;

const EventsView: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { state, submitIntent } = useCitadelAPI();
  const [now, setNow] = useState(() => Date.now());
  const worldId = state?.account.worldId ?? '';
  const playerId = state?.account.playerId === state?.player.id ? state?.player.id ?? 0 : 0;
  const history = useFeatureEventHistory(worldId, playerId);
  const [rankingOpen, setRankingOpen] = useState(false);
  const [rankingLoading, setRankingLoading] = useState(false);
  const [rankingError, setRankingError] = useState('');
  const [analyticsView, setAnalyticsView] = useState<EventsAnalyticsView>('events');
  const selectedAnalyticsView = analyticsView;
  const selectedEventIds = featureEventIds[selectedAnalyticsView];
  const liveEvents = Object.values(state?.eventScores.byEvent ?? {}).filter((score) => (
    isFeatureEventRunning(score, state?.eventScores.inventory, now, state?.session.changedAt)
    && (selectedAnalyticsView === 'events' || selectedEventIds?.includes(score.eventId))
  ));
  const event = liveEvents.find((score) => score.eventId === 72) ?? liveEvents[0];
  const ranking = event ? state?.eventScores.rankingByEvent?.[String(event.eventId)] : undefined;
  const analyticsOptions = [
    { value: 'events', label: 'Events' },
    ...attackEconomyFeatureDefinitions.map(({ id, label }) => ({ value: id, label })),
  ];

  useEffect(() => {
    setNow(Date.now());
  }, [state?.eventScores, history.entries]);

  useEffect(() => {
    const boundaries = [
      ...Object.values(state?.eventScores.byEvent ?? {}).map((score) => Date.parse(score.observedAt) + (score.remainingSec ?? 0) * 1000),
      ...Object.values(state?.eventScores.inventory?.activeByEvent ?? {}).map((entry) => Date.parse(entry.endsAt)),
      ...history.entries.map((entry) => Date.parse(entry.eventEndsAt)),
    ].filter((end) => Number.isFinite(end) && end > now);
    if (boundaries.length === 0) return;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(Math.max(Math.min(...boundaries) - Date.now() + 25, 25), 2_147_483_647));
    return () => window.clearTimeout(timer);
  }, [state?.eventScores, history.entries, now]);

  useEffect(() => {
    setRankingOpen(false);
    setRankingError('');
  }, [event?.eventId, selectedAnalyticsView]);

  const refreshRanking = useCallback(async () => {
    if (!event || !gameLoggedIn || event.eventId !== 72 || (event.allianceLeagueId ?? 0) <= 0 || rankingLoading) return;
    setRankingLoading(true);
    setRankingError('');
    try {
      const receipt = await submitIntent('event.ranking.refresh', { eventId: event.eventId }, { actor: 'ui:events-ranking' });
      if (receipt.status === 'failed' || receipt.status === 'cancelled' || receipt.status === 'indeterminate') {
        throw new Error(receipt.error || `Ranking refresh ${receipt.status.replaceAll('_', ' ')}.`);
      }
    } catch (error) {
      setRankingError(error instanceof Error ? error.message : 'Could not refresh the GGE event ranking.');
    } finally {
      setRankingLoading(false);
    }
  }, [event, gameLoggedIn, rankingLoading, submitIntent]);

  const openRanking = useCallback(() => {
    setRankingOpen(true);
    void refreshRanking();
  }, [refreshRanking]);

  return (
    <div className="flex flex-col gap-6">
      <PillSelector
        ariaLabel="Feature stats view"
        value={selectedAnalyticsView}
        onChange={(value) => setAnalyticsView(value as EventsAnalyticsView)}
        options={analyticsOptions}
        size="header"
        className="w-full"
      />
      <StaleSessionBanner />
      {liveEvents.map((liveEvent) => <React.Fragment key={liveEvent.eventId}>
        <EventScoreCard live={gameLoggedIn} event={liveEvent} onOpenRanking={liveEvent.eventId === event?.eventId ? openRanking : undefined} rankingLoading={rankingLoading} />
        {selectedAnalyticsView === 'events' && <EventActivityCard event={liveEvent} />}
      </React.Fragment>)}
      {(selectedAnalyticsView === 'events' || selectedEventIds) && <FeatureEventHistory
        key={`${worldId}:${playerId}:${selectedAnalyticsView}`}
        {...history} worldId={worldId} playerId={playerId} now={now} eventIds={selectedEventIds}
      />}
      {selectedAnalyticsView === 'events' ? (
        <>
          {isInvasionEvent(event?.eventId, event?.eventType, event?.name, event?.localizationKey) && (
            <AttackEconomyView
              selectedFeature="autoInvasion"
              showFeatureSelector={false}
              embedded
            />
          )}
        </>
      ) : (
        <AttackEconomyView
          key={selectedAnalyticsView}
          selectedFeature={selectedAnalyticsView}
          onFeatureChange={setAnalyticsView}
          showFeatureSelector={false}
          embedded
        />
      )}
      <EventRankingModal
        isOpen={rankingOpen && Boolean(event)}
        eventName={eventDisplayName(event?.eventId, event?.name)}
        ranking={ranking}
        allianceId={state?.player.allianceId}
        isRefreshing={rankingLoading}
        error={rankingError}
        onRefresh={() => void refreshRanking()}
        onClose={() => setRankingOpen(false)}
      />
    </div>
  );
};

function eventDisplayName(eventID?: number, name?: string): string {
  if (eventID === 71) return 'Foreign Lords Invasion';
  if (eventID === 72) return 'Nomad Invasion';
  if (eventID === 80) return 'Samurai Invasion';
  if (eventID === 103) return 'Bloodcrow Invasion';
  if (name?.trim()) return name.trim();
  return eventID ? `Event ${eventID}` : 'Event';
}

function isInvasionEvent(eventID?: number, ...identityParts: Array<string | undefined>): boolean {
  if (eventID === 71 || eventID === 103) return true;
  const identity = identityParts.join(' ').toLowerCase();
  return identity.includes('alien invasion') || identity.includes('bloodcrow') || identity.includes('foreign lord');
}

export default EventsView;
