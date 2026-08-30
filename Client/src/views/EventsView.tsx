import React, { useCallback, useEffect, useMemo, useState } from 'react';
import StaleSessionBanner from '../components/StaleSessionBanner';
import EventScoreCard from '../dashboard/components/EventScoreCard';
import { useAuth } from '../context/AuthContext';
import { PillSelector } from '../components/ui';
import { useCitadelAPI } from '../api/ApiContext';
import EventActivityCard from '../events/components/EventActivityCard';
import EventRankingModal from '../events/components/EventRankingModal';
import AttackEconomyView, { type AttackEconomyFeatureID } from '../attackAnalytics/components/AttackEconomyView';

type EventsAnalyticsView = 'events' | AttackEconomyFeatureID;
const berimondBattleEventID = 3;

const EventsView: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { state, submitIntent } = useCitadelAPI();
  const activeEventID = state?.eventScores.activeEventId ?? 0;
  const event = activeEventID > 0 ? state?.eventScores.byEvent[String(activeEventID)] : undefined;
  const ranking = activeEventID > 0 ? state?.eventScores.rankingByEvent?.[String(activeEventID)] : undefined;
  const [rankingOpen, setRankingOpen] = useState(false);
  const [rankingLoading, setRankingLoading] = useState(false);
  const [rankingError, setRankingError] = useState('');
  const [analyticsView, setAnalyticsView] = useState<EventsAnalyticsView>('events');
  const beriAvailability = state?.eventScores.inventory?.activeByEvent?.[String(berimondBattleEventID)];
  const [availabilityNow, setAvailabilityNow] = useState(() => Date.now());
  const autoBeriAvailable = isAuthoritativeEventActive(
    state?.eventScores.inventory?.observedAt,
    beriAvailability,
    berimondBattleEventID,
    availabilityNow,
  );
  const selectedAnalyticsView = analyticsView === 'autoBeriWorld' && !autoBeriAvailable
    ? 'events'
    : analyticsView;
  const analyticsOptions = useMemo(() => [
    { value: 'events', label: 'Active Event' },
    { value: 'autoTowers', label: 'Auto Towers' },
    { value: 'autoStorm', label: 'Auto Storm' },
    ...(autoBeriAvailable ? [{ value: 'autoBeriWorld', label: 'Auto Beri' }] : []),
  ], [autoBeriAvailable]);

  useEffect(() => {
    setRankingOpen(false);
    setRankingError('');
  }, [activeEventID]);

  useEffect(() => {
    const now = Date.now();
    setAvailabilityNow(now);
    if (!beriAvailability?.endsAt) return undefined;
    const endsAt = Date.parse(beriAvailability.endsAt);
    if (!Number.isFinite(endsAt) || endsAt <= now) return undefined;
    const timer = window.setTimeout(
      () => setAvailabilityNow(Date.now()),
      Math.min(endsAt - now + 50, 2_147_483_647),
    );
    return () => window.clearTimeout(timer);
  }, [beriAvailability?.endsAt, state?.eventScores.inventory?.observedAt]);

  useEffect(() => {
    if (!autoBeriAvailable && analyticsView === 'autoBeriWorld') setAnalyticsView('events');
  }, [analyticsView, autoBeriAvailable]);

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
      {selectedAnalyticsView === 'events' ? (
        <>
          <StaleSessionBanner />
          <EventScoreCard key={activeEventID} live={gameLoggedIn} event={event} onOpenRanking={openRanking} rankingLoading={rankingLoading} />
          <EventActivityCard key={activeEventID} event={event} />
          {isInvasionEvent(event?.eventId, event?.eventType, event?.name, event?.localizationKey) && (
            <AttackEconomyView
              selectedFeature="autoInvasion"
              showFeatureSelector={false}
              embedded
            />
          )}
          <EventRankingModal
            isOpen={rankingOpen}
            eventName={eventDisplayName(event?.eventId, event?.name)}
            ranking={ranking}
            allianceId={state?.player.allianceId}
            isRefreshing={rankingLoading}
            error={rankingError}
            onRefresh={() => void refreshRanking()}
            onClose={() => setRankingOpen(false)}
          />
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

function isAuthoritativeEventActive(
  observedAt: string | undefined,
  availability: { eventId: number; endsAt: string } | undefined,
  eventID: number,
  now: number,
): boolean {
  if (!observedAt || !availability || availability.eventId !== eventID) return false;
  const inventoryObservedAt = Date.parse(observedAt);
  const endsAt = Date.parse(availability.endsAt);
  return Number.isFinite(inventoryObservedAt) && inventoryObservedAt > 0
    && Number.isFinite(endsAt) && now < endsAt;
}

export default EventsView;
