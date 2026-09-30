import { useMemo } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { useAuth } from '../../context/AuthContext';
import { readSavedAt, useDraftLine, type DraftLine } from '../DraftRecovery';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { firstConfirmedResult, type FirstResult } from '../readiness/firstResult';
import { useAutomationDescription } from '../readiness/useAutomationDescription';
import { useFeatureReadiness } from '../readiness/useFeatureReadiness';
import { accountKey } from '../requirements/castleRequirements';
import { evaluateChecklist, type Checklist, type ConnectionEvidence } from './checklist';
import { scopeKey } from './accountScope';
import { GOAL_SAVED_SECTION, isSavedSection, type AutomationGoal } from './goals';
import { useConnectionEvidence } from './useConnectionEvidence';

/** The five things the legend keeps apart, each with its current value for one feature. */
export interface StateLegendValues {
  draft: DraftLine;
  saved: { exists: boolean; at?: string };
  connected: 'yes' | 'no' | 'waiting' | 'unknown';
  on: { on: boolean; timedUntil?: number };
  firstResult: { state: FirstResult['state']; at?: string };
}

export interface GoalChecklistState {
  checklist: Checklist;
  legend: StateLegendValues;
  connection: ConnectionEvidence;
}

/**
 * The checklist and the state legend for one goal, computed from observed state only (CIT-19): the game session, the
 * saved section, the readiness of the SAVED settings, the saved switch, the reported phase and the attributed receipts.
 * Read-only: no intent, no configuration write.
 */
export function useGoalChecklist(goal: AutomationGoal): GoalChecklistState {
  const featureId = goal.featureId;
  const section = GOAL_SAVED_SECTION[featureId];
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const { state, operations } = useCitadelAPI();
  const { automationEnabledByKey, automationTimedUntilByKey, automationWriteFailures, automationEnabledSince, automationStates } = useAuth();
  const connection = useConnectionEvidence();
  const { report, sections } = useFeatureReadiness(featureId);
  const description = useAutomationDescription(featureId);
  const key = scopeKey(state);
  const draft = useDraftLine(key, section);

  const firstResult = useMemo(() => firstConfirmedResult(featureId, {
    operations, runtime: automationStates[featureId], enabledSince: automationEnabledSince[enabledKey], accountKey: accountKey(state ?? null),
  }), [automationEnabledSince, automationStates, enabledKey, featureId, operations, state]);

  const exists = isSavedSection(sections?.[section]);
  const on = automationEnabledByKey[enabledKey] === true;
  const timedUntil = automationTimedUntilByKey[enabledKey];
  const failure = automationWriteFailures[enabledKey];
  const startFailed = failure?.intent === 'start';
  const startFailureMessage = startFailed ? failure.message : undefined;

  const checklist = useMemo(() => evaluateChecklist(goal, {
    connection,
    saved: { exists },
    report: sections ? report : null,
    enabled: { on, ...(timedUntil ? { timedUntil } : {}) },
    failedStart: startFailed ? { message: startFailureMessage } : null,
    phase: { phase: description.phase },
    firstResult,
  }), [connection, description.phase, exists, firstResult, goal, on, report, sections, startFailed, startFailureMessage, timedUntil]);

  const connectionStep = checklist.steps[0];
  const legend: StateLegendValues = {
    draft,
    saved: { exists, ...(exists ? { at: readSavedAt(key, section) } : {}) },
    connected: connectionStep.state === 'done' ? 'yes' : connectionStep.state === 'waiting' ? 'waiting' : connectionStep.state === 'unknown' ? 'unknown' : 'no',
    on: { on, ...(timedUntil ? { timedUntil } : {}) },
    firstResult: { state: firstResult.state, ...(firstResult.completedAt ? { at: firstResult.completedAt } : {}) },
  };
  return { checklist, legend, connection };
}
