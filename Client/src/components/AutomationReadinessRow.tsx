import React, { useEffect, useRef, useState } from 'react';
import { CheckCircle2, ChevronDown, CircleDashed, Clock3, XCircle } from 'lucide-react';
import { LocalizedText } from '../i18n/LocalizedText';
import { ReadinessPanel } from '../settings/components/ReadinessPanel';
import type { SettingsFeatureId } from '../settings/disclosure/placement';
import type { CheckState, ReadinessCheck } from '../settings/readiness/Readiness';
import { clearPendingRowFocus, requestSettingsFix, ROW_FOCUS_EVENT, takePendingRowFocus } from '../settings/readiness/settingsFixRequest';
import { useFeatureReadiness } from '../settings/readiness/useFeatureReadiness';
import { useCitadelAPI } from '../api/useCitadelAPI';
import { scopeKey } from '../settings/onboarding/accountScope';
import { GOAL_SAVED_SECTION, isSavedSection } from '../settings/onboarding/goals';
import { useGoal } from '../settings/onboarding/goalStore';
import { Badge } from './ui/Badge';
import { Button } from './ui/Button';

const ICON: Record<CheckState, React.ComponentType<{ className?: string }>> = {
  valid: CheckCircle2, blocked: XCircle, pending: Clock3, unavailable: CircleDashed,
};
const TONE: Record<CheckState, string> = {
  valid: 'text-success', blocked: 'text-error', pending: 'text-warning', unavailable: 'text-text-muted',
};
const BADGE: Record<CheckState, 'success' | 'danger' | 'warning' | 'outline'> = {
  valid: 'success', blocked: 'danger', pending: 'warning', unavailable: 'outline',
};
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
  const { report, sections } = useFeatureReadiness(featureId, { publish: true });
  const { state: gameState } = useCitadelAPI();
  const goalApi = useGoal(scopeKey(gameState));
  // Entry point 3: nothing is saved for this automation yet, and it is not already the chosen goal.
  const offerGuide = sections !== undefined && !isSavedSection(sections[GOAL_SAVED_SECTION[featureId]]) && goalApi.goal?.goalId !== featureId;
  const [expanded, setExpanded] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);

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
      {offerGuide ? (
        <div className="mt-1">
          <Button variant="ghost" size="sm" onClick={() => goalApi.choose(featureId)} data-guide-me={featureId}>
            <LocalizedText messageKey="goalEntry.guideMe" />
          </Button>
        </div>
      ) : null}
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
