import React, { useEffect, useMemo, useRef, useState } from 'react';
import { CheckCircle2, ChevronDown, CircleDashed, Clock3, XCircle } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { useHostedRuntimePresence } from '../config/Deployment';
import { useAuth } from '../context/AuthContext';
import { useMetadata } from '../context/MetadataContext';
import { LocalizedText } from '../i18n/LocalizedText';
import { movementViewFromState } from '../Movement/types/MovementState';
import { ReadinessPanel } from '../settings/components/ReadinessPanel';
import type { SettingsFeatureId } from '../settings/disclosure/placement';
import { useEventDifficultyOptions } from '../settings/EventDifficultyOptions';
import { catalogInputsFor, evaluateFeatureReadiness, READINESS_DIFFICULTY_EVENTS } from '../settings/readiness/featureReadiness';
import { publishReadiness, savedSectionsDigest } from '../settings/readiness/latestReadiness';
import type { CheckState, ReadinessCheck } from '../settings/readiness/Readiness';
import { clearPendingRowFocus, requestSettingsFix, ROW_FOCUS_EVENT, takePendingRowFocus } from '../settings/readiness/settingsFixRequest';
import { normalizeFeatureSchedules } from '../settings/SchedulerTypes';
import { loadStormUnlockOffer, type StormUnlockOffer } from '../settings/StormCastleOptions';
import { Badge } from './ui/Badge';

const ICON: Record<CheckState, React.ComponentType<{ className?: string }>> = {
  valid: CheckCircle2, blocked: XCircle, pending: Clock3, unavailable: CircleDashed,
};
const TONE: Record<CheckState, string> = {
  valid: 'text-success', blocked: 'text-error', pending: 'text-warning', unavailable: 'text-text-muted',
};
const BADGE: Record<CheckState, 'success' | 'danger' | 'warning' | 'outline'> = {
  valid: 'success', blocked: 'danger', pending: 'warning', unavailable: 'outline',
};
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

/**
 * "Before you start" on the Automation page (CIT-20): a collapsed summary line per automation that expands
 * to the same readiness report the settings editor shows, built from the SAVED configuration and current
 * observations. It is a preview only: it never disables the switch (the game decides at Start), sends no
 * game action, and "Open settings" only opens the editor at the setting to fix.
 */
export const AutomationReadinessRow: React.FC<{
  featureId: SettingsFeatureId;
  onOpenSettings: () => void;
}> = ({ featureId, onOpenSettings }) => {
  const { state: liveState, configuration: liveConfiguration, getCatalog } = useCitadelAPI();
  const state = useThrottledValue(liveState, REFRESH_MS);
  const configuration = useThrottledValue(liveConfiguration, REFRESH_MS);
  const { troops, tools, resources, unitsLoading, unitsError } = useMetadata();
  const { gameLoggedIn } = useAuth();
  const presence = useHostedRuntimePresence();
  const [expanded, setExpanded] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
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
  const report = useMemo(() => evaluateFeatureReadiness(featureId, {
    sections, state, observation, troops, tools, resources,
    metadataReady: !unitsLoading && !unitsError,
    movement, gameLoggedIn, now: Date.now(),
    schedule: normalizeFeatureSchedules((sections?.scheduler as { featureSchedules?: unknown } | undefined)?.featureSchedules)[featureId],
    ...catalogInputsFor(featureId, {
      // A catalog that failed to load contributes nothing: the check reads "decided at launch" here and at Start.
      difficulties: difficultyEvents && !difficulties.error
        ? { optionsByEvent: difficulties.optionsByEvent, achievementsObserved, loading: difficulties.loading }
        : undefined,
      stormOffer: stormOffer.loaded ? stormOffer : undefined,
    }),
  }), [achievementsObserved, difficultyEvents, difficulties.error, difficulties.loading, difficulties.optionsByEvent, featureId, gameLoggedIn, movement, observation, resources, sections, state, stormOffer, tools, troops, unitsError, unitsLoading]);

  // One report per feature: the Start check reuses this one while the saved configuration is unchanged.
  const digest = useMemo(() => savedSectionsDigest(sections), [sections]);
  useEffect(() => { publishReadiness(featureId, report, digest); }, [digest, featureId, report]);

  // "Fix first" from the Start confirmation brings this row into view, expanded. Same pending mechanism as the
  // settings fix: a row that mounts after the request takes the pending record, a mounted one hears the event.
  useEffect(() => {
    const bringIntoView = () => {
      setExpanded(true);
      rootRef.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      toggleRef.current?.focus({ preventScroll: true });
    };
    if (takePendingRowFocus(featureId)) bringIntoView();
    const onFixFirst = (event: Event) => {
      if ((event as CustomEvent<{ featureId?: string }>).detail?.featureId !== featureId) return;
      clearPendingRowFocus(featureId);
      bringIntoView();
    };
    window.addEventListener(ROW_FOCUS_EVENT, onFixFirst);
    return () => window.removeEventListener(ROW_FOCUS_EVENT, onFixFirst);
  }, [featureId]);

  const fix = (check: ReadinessCheck) => requestSettingsFix(featureId, check, onOpenSettings);
  const Icon = ICON[report.overall];
  const blocked = report.checks.filter((check) => check.state === 'blocked').length;
  const waiting = report.checks.filter((check) => check.state === 'pending' || check.state === 'unavailable').length;
  const panelId = `automation-readiness-${featureId}`;
  return (
    <div ref={rootRef} className="mt-1.5 text-xs" data-automation-readiness={featureId} data-readiness-overall={report.overall}>
      <button
        ref={toggleRef}
        type="button"
        className="flex w-full min-w-0 flex-wrap items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-text-muted hover:text-text-main focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
        aria-expanded={expanded}
        aria-controls={panelId}
        onClick={() => setExpanded((current) => !current)}
      >
        <Icon className={`h-3.5 w-3.5 shrink-0 ${TONE[report.overall]}`} aria-hidden="true" />
        <span className="font-semibold text-text-main"><LocalizedText messageKey="featureReadiness.title" />:</span>
        <Badge variant={BADGE[report.overall]} className="normal-case tracking-normal">
          <LocalizedText messageKey="readiness.overall" params={{ state: report.overall }} />
        </Badge>
        {blocked + waiting > 0 ? (
          <span className="min-w-0 whitespace-normal break-words"><LocalizedText messageKey="featureReadiness.counts" params={{ blocked, waiting }} /></span>
        ) : null}
        <ChevronDown className={`ml-auto h-3.5 w-3.5 shrink-0 transition-transform ${expanded ? 'rotate-180' : ''}`} aria-hidden="true" />
        <span className="sr-only"><LocalizedText messageKey={expanded ? 'featureReadiness.collapse' : 'featureReadiness.expand'} /></span>
      </button>
      <div id={panelId} hidden={!expanded} className="mt-1.5 space-y-1.5">
        {expanded ? (
          <>
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="featureReadiness.notAction" /></p>
            <ReadinessPanel report={report} onFix={fix} />
          </>
        ) : null}
      </div>
    </div>
  );
};
