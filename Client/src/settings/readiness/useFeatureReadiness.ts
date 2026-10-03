import { useNow } from '../../useNow';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useCitadelAPI } from '../../api/useCitadelAPI';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { useAuth } from '../../context/useAuth';
import { useMetadata } from '../../context/useMetadata';
import { movementViewFromState } from '../../Movement/types/MovementState';
import type { SettingsFeatureId } from '../disclosure/placement';
import { useEventDifficultyOptions } from '../EventDifficultyOptions';
import { normalizeFeatureSchedules } from '../SchedulerTypes';
import { loadStormUnlockOffer, type StormUnlockOffer } from '../StormCastleOptions';
import { catalogInputsFor, evaluateFeatureReadiness, READINESS_DIFFICULTY_EVENTS } from './featureReadiness';
import { publishReadiness, savedSectionsDigest } from './latestReadiness';
import type { ReadinessReport } from './Readiness';

const NO_ACHIEVEMENTS: Record<string, boolean> = {};
const REFRESH_MS = 3000;

/** The latest value, refreshed at most once per `ms`: the report does not need to recompute on every state patch. */
function useThrottledValue<T>(value: T, ms: number): T {
  const [shown, setShown] = useState(value);
  const lastAt = useRef(0);
  useEffect(() => {
    if (Object.is(shown, value)) return undefined;
    const wait = Math.max(0, lastAt.current + ms - Date.now());
    const timer = window.setTimeout(() => { lastAt.current = Date.now(); setShown(value); }, wait);
    return () => window.clearTimeout(timer);
  }, [ms, shown, value]);
  return shown;
}

export interface FeatureReadiness {
  report: ReadinessReport;
  /** The SAVED configuration sections the report was built from. */
  sections: Record<string, unknown> | undefined;
  digest: string;
}

/**
 * The readiness report of one automation's SAVED settings, built the same way for every surface (CIT-20): the
 * Automation row, the goal checklist (CIT-19) and, through the published report, the Start check. Read-only.
 * Only the Automation row publishes (`publish`), so one report per feature is what Start reuses.
 */
export function useFeatureReadiness(featureId: SettingsFeatureId, options: { publish?: boolean } = {}): FeatureReadiness {
  const { state: liveState, configuration: liveConfiguration, getCatalog } = useCitadelAPI();
  const state = useThrottledValue(liveState, REFRESH_MS);
  const configuration = useThrottledValue(liveConfiguration, REFRESH_MS);
  const { troops, tools, resources, unitsLoading, unitsError } = useMetadata();
  const { gameLoggedIn } = useAuth();
  const presence = useHostedRuntimePresence();
  const movement = useMemo(() => movementViewFromState(state), [state]);
  const session = state?.session ?? null;
  const observation = useMemo(() => ({
    session, connected: gameLoggedIn,
    hostedPresence: presence.mode ? { mode: presence.mode, checkpointObservedAt: presence.checkpointObservedAt } : undefined,
  }), [gameLoggedIn, presence.checkpointObservedAt, presence.mode, session]);

  const difficultyEvents = READINESS_DIFFICULTY_EVENTS[featureId];
  const completed = state?.player.achievements?.completed ?? NO_ACHIEVEMENTS;
  const difficulties = useEventDifficultyOptions(difficultyEvents !== undefined, difficultyEvents ?? [], completed);
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);

  const [stormOffer, setStormOffer] = useState<StormUnlockOffer>({ loaded: false, offeredIds: [] });
  const playerLevel = state?.player.level;
  useEffect(() => {
    if (featureId !== 'autoStorm') return undefined;
    let cancelled = false;
    void loadStormUnlockOffer(getCatalog, playerLevel).then((offer) => { if (!cancelled) setStormOffer(offer); });
    return () => { cancelled = true; };
  }, [featureId, getCatalog, playerLevel]);

  const sections = configuration?.sections;
  const now = useNow(REFRESH_MS);
  const report = useMemo(() => evaluateFeatureReadiness(featureId, {
    sections, state, observation, troops, tools, resources,
    metadataReady: !unitsLoading && !unitsError,
    movement, gameLoggedIn, now,
    schedule: normalizeFeatureSchedules((sections?.scheduler as { featureSchedules?: unknown } | undefined)?.featureSchedules)[featureId],
    ...catalogInputsFor(featureId, {
      // A catalog that failed to load contributes nothing: the check reads "decided at launch" here and at Start.
      difficulties: difficultyEvents && !difficulties.error
        ? { optionsByEvent: difficulties.optionsByEvent, achievementsObserved, loading: difficulties.loading }
        : undefined,
      stormOffer: stormOffer.loaded ? stormOffer : undefined,
    }),
  }), [achievementsObserved, difficultyEvents, difficulties.error, difficulties.loading, difficulties.optionsByEvent, featureId, gameLoggedIn, movement, now, observation, resources, sections, state, stormOffer, tools, troops, unitsError, unitsLoading]);

  const digest = useMemo(() => savedSectionsDigest(sections), [sections]);
  const publish = options.publish === true;
  useEffect(() => { if (publish) publishReadiness(featureId, report, digest); }, [digest, featureId, publish, report]);
  return { report, sections, digest };
}
