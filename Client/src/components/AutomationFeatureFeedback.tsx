import React, { useMemo } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { LocalizedText } from '../i18n/LocalizedText';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../settings/disclosure/placement';
import { firstConfirmedResult } from '../settings/readiness/firstResult';
import { useAutomationDescription } from '../settings/readiness/useAutomationDescription';
import { accountKey } from '../settings/requirements/castleRequirements';
import { AutomationReadinessRow } from './AutomationReadinessRow';
import { FirstResultCard } from './FirstResultCard';
import { StopControl } from './StopControl';
import { scopeKey } from '../settings/onboarding/accountScope';
import { goalById } from '../settings/onboarding/goals';
import { useGoal } from '../settings/onboarding/goalStore';
import { useGoalChecklist } from '../settings/onboarding/useChecklist';
import { StateLegend } from './StateLegend';
import { StatusBadge } from './ui/StatusBadge';
import { useAutomationPlayerStatus } from '../settings/readiness/useAutomationPlayerStatus';
import { Button } from './ui/Button';

const GoalLegendInner: React.FC<{ goal: NonNullable<ReturnType<typeof goalById>> }> = ({ goal }) => {
  const { legend } = useGoalChecklist(goal);
  return <StateLegend values={legend} compact />;
};

/** The compact state legend, only under the automation the player chose as their goal (CIT-19). */
const GoalLegendLine: React.FC<{ featureId: SettingsFeatureId }> = ({ featureId }) => {
  const { state } = useCitadelAPI();
  const { goal } = useGoal(scopeKey(state));
  const active = goalById(goal?.goalId);
  return active && active.featureId === featureId ? <GoalLegendInner goal={active} /> : null;
};

/**
 * Everything the Automation page says about one automation beyond the game's own status lines (CIT-20): the
 * phase and what it means (never invented: the game's detail stays verbatim in the status lines above), the
 * next step, a failed Start/Stop with Retry, the first confirmed result, and "Before you start".
 */
export const AutomationFeatureFeedback: React.FC<{
  featureId: SettingsFeatureId;
  enabled: boolean;
  onOpenSettings: () => void;
  /** `AttackLaunchRatesV2.launchesByFeature`: shown only as context, never as a confirmation. */
  launchesByFeature?: Record<string, number> | null;
  /** Player-facing account name; hosted shows "(this account)" beside a result. */
  accountLabel?: string;
  /** Berimond's builder lane only counts while its build switch is on. */
  buildLaneActive?: boolean;
  /**
   * Header popovers: phase, next step, a failed Start/Stop and the first result, without "Before you start"
   * (that stays on the Automation page row, which is the one place a start is confirmed).
   */
  compact?: boolean;
  /** The status panel already owns the feature status and reason. */
  hideStatus?: boolean;
}> = ({ featureId, enabled, onOpenSettings, launchesByFeature, accountLabel, buildLaneActive, compact = false, hideStatus = false }) => {
  const description = useAutomationDescription(featureId, { buildLaneActive });
  const player = useAutomationPlayerStatus(featureId, { buildLaneActive }).overall;
  const { operations, state } = useCitadelAPI();
  const { automationEnabledSince, automationStates } = useAuth();
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const result = useMemo(() => firstConfirmedResult(featureId, {
    operations,
    runtime: automationStates[featureId],
    launchRates: launchesByFeature ? { launchesByFeature } : null,
    enabledSince: automationEnabledSince[enabledKey],
    accountKey: accountKey(state ?? null),
    accountLabel,
  }), [accountLabel, automationEnabledSince, automationStates, enabledKey, featureId, launchesByFeature, operations, state]);
  const showPhase = enabled || description.phase === 'stopped';
  return (
    <div className={compact ? 'space-y-1.5' : 'mt-1 space-y-1.5'} data-automation-feedback={featureId} data-feedback-compact={compact ? 'true' : undefined}>
      {showPhase ? (
        <div className="flex flex-wrap items-start gap-1.5 text-caption text-text-muted" data-automation-phase={description.phase}>
          {compact && !hideStatus ? <StatusBadge {...player} /> : null}
          <span className="min-w-0 flex-1">
            {description.nextStep === 'clear-lock' ? (
              <Button variant="ghost" size="sm" className="ml-1" onClick={() => document.getElementById('automation-safety-panel')?.scrollIntoView({ block: 'center', behavior: 'smooth' })}>
                <LocalizedText messageKey="runtimeState.goToLock" />
              </Button>
            ) : null}
            {description.nextStep === 'settings' ? (
              <Button variant="ghost" size="sm" className="ml-1" onClick={onOpenSettings}>
                <LocalizedText messageKey="featureReadiness.openSettings" />
              </Button>
            ) : null}
          </span>
        </div>
      ) : null}
      <StopControl enabledKey={enabledKey} featureId={featureId} variant="notice" />
      {enabled ? <FirstResultCard result={result} accountLabel={accountLabel} /> : null}
      {compact ? null : <AutomationReadinessRow featureId={featureId} onOpenSettings={onOpenSettings} />}
      {compact ? null : <GoalLegendLine featureId={featureId} />}
    </div>
  );
};
