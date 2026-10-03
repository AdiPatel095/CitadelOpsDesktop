import { useEffect, useState } from 'react';
import { useLocale as useStaticLocale } from '../../i18n/useLocale';
import { CitadelAPI } from '../../api/CitadelClient';
import type { WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';
import { canonicalEventWorldID, featureHistoryMatchesScope } from './FeatureEventWorld';

const emptyHistory: WorldIntelligenceEventScoreObservationV1[] = [];

export function useFeatureEventHistory(worldId: string, playerId: number) {
  const {t}=useStaticLocale();
  worldId = canonicalEventWorldID(worldId);
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
        if (!featureHistoryMatchesScope(history, worldId, playerId)) throw new Error('Event history identity mismatch');
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
  return result.scope === scope ? {...result,error:result.error ? t('events.historyUnavailable') : ''} : { entries: emptyHistory, loading: Boolean(worldId && playerId > 0), error: '' };
}
