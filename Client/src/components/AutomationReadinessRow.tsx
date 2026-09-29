import React, { useEffect, useMemo, useRef, useState } from 'react';
import { CheckCircle2, ChevronDown, CircleDashed, Clock3, XCircle } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { CitadelAPI } from '../api/CitadelClient';
import { useHostedRuntimePresence } from '../config/Deployment';
import { useAuth } from '../context/AuthContext';
import { useMetadata } from '../context/MetadataContext';
import { LocalizedText } from '../i18n/LocalizedText';
import { movementViewFromState } from '../Movement/types/MovementState';
import { ReadinessPanel } from '../settings/components/ReadinessPanel';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../settings/disclosure/placement';
import { useEventDifficultyOptions } from '../settings/EventDifficultyOptions';
import { evaluateFeatureReadiness } from '../settings/readiness/featureReadiness';
import type { CheckState, ReadinessCheck } from '../settings/readiness/Readiness';
import { requestSettingsFix } from '../settings/readiness/settingsFixRequest';
import { normalizeFeatureSchedules } from '../settings/SchedulerTypes';
import { parseStormCastleOptions } from '../settings/StormCastleOptions';
import { Badge } from './ui/Badge';

/** Events whose official difficulty catalogs the event-attack readiness reads. */
const DIFFICULTY_EVENTS: Readonly<Partial<Record<SettingsFeatureId, readonly number[]>>> = {
  autoNomad: [72, 80],
  autoInvasion: [71, 103],
};

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
  const { state: liveState, configuration: liveConfiguration } = useCitadelAPI();
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

  const difficultyEvents = DIFFICULTY_EVENTS[featureId];
  const completed = state?.player.achievements?.completed ?? NO_ACHIEVEMENTS;
  const difficulties = useEventDifficultyOptions(difficultyEvents !== undefined, difficultyEvents ?? [], completed);
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);

  const [stormOffer, setStormOffer] = useState<{ loaded: boolean; offeredIds: number[] }>({ loaded: false, offeredIds: [] });
  const playerLevel = state?.player.level;
  useEffect(() => {
    if (featureId !== 'autoStorm') return undefined;
    let cancelled = false;
    void CitadelAPI.getCatalog<Record<string, unknown>>('prebuiltcastles')
      .then((response) => {
        if (!cancelled) setStormOffer({ loaded: true, offeredIds: parseStormCastleOptions(response.items, playerLevel).map((option) => option.id) });
      })
      .catch(() => { if (!cancelled) setStormOffer({ loaded: false, offeredIds: [] }); });
    return () => { cancelled = true; };
  }, [featureId, playerLevel]);

  const sections = configuration?.sections;
  const report = useMemo(() => evaluateFeatureReadiness(featureId, {
    sections, state, observation, troops, tools, resources,
    metadataReady: !unitsLoading && !unitsError,
    movement, gameLoggedIn, now: Date.now(),
    schedule: normalizeFeatureSchedules((sections?.scheduler as { featureSchedules?: unknown } | undefined)?.featureSchedules)[featureId],
    ...(difficultyEvents ? { difficulties: { optionsByEvent: difficulties.optionsByEvent, achievementsObserved, loading: difficulties.loading } } : {}),
    ...(featureId === 'autoStorm' ? { stormUnlockOffer: stormOffer } : {}),
  }), [achievementsObserved, difficultyEvents, difficulties.loading, difficulties.optionsByEvent, featureId, gameLoggedIn, movement, observation, resources, sections, state, stormOffer, tools, troops, unitsError, unitsLoading]);

  // "Fix first" from the Start confirmation brings this row into view, expanded.
  useEffect(() => {
    const onFixFirst = (event: Event) => {
      const detail = (event as CustomEvent<{ enabledKey?: string }>).detail;
      if (detail?.enabledKey !== AUTOMATION_ENABLED_KEYS[featureId]) return;
      setExpanded(true);
      rootRef.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      toggleRef.current?.focus({ preventScroll: true });
    };
    window.addEventListener('citadelops:fix-before-start', onFixFirst);
    return () => window.removeEventListener('citadelops:fix-before-start', onFixFirst);
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
        className="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-text-muted hover:text-text-main focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
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
          <span className="min-w-0 truncate"><LocalizedText messageKey="featureReadiness.counts" params={{ blocked, waiting }} /></span>
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
