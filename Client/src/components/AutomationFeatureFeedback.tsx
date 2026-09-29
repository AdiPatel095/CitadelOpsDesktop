import React, { useMemo } from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { LocalizedText } from '../i18n/LocalizedText';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../settings/disclosure/placement';
import { firstConfirmedResult } from '../settings/readiness/firstResult';
import type { AutomationPhase } from '../settings/readiness/runtimeState';
import { useAutomationDescription } from '../settings/readiness/useAutomationDescription';
import { accountKey } from '../settings/requirements/castleRequirements';
import { AutomationReadinessRow } from './AutomationReadinessRow';
import { FirstResultCard } from './FirstResultCard';
import { StopControl } from './StopControl';
import { Badge } from './ui/Badge';
import { Button } from './ui/Button';

const PHASE_BADGE: Record<AutomationPhase, 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'outline'> = {
  disabled: 'outline', stopped: 'outline', 'enabled-waiting': 'secondary', running: 'primary',
  blocked: 'warning', error: 'danger', locked: 'warning', completed: 'secondary', unknown: 'outline',
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
}> = ({ featureId, enabled, onOpenSettings, launchesByFeature, accountLabel, buildLaneActive }) => {
  const description = useAutomationDescription(featureId, { buildLaneActive });
  const { operations, state } = useCitadelAPI();
  const { automationEnabledSince, automationStates } = useAuth();
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const message = useLocalizedMessage(description.runtimeDetail?.descriptor, description.runtimeDetail?.text ?? '');
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
    <div className="mt-1 space-y-1.5" data-automation-feedback={featureId}>
      {showPhase ? (
        <div className="flex flex-wrap items-start gap-1.5 text-xs leading-relaxed text-text-muted" data-automation-phase={description.phase}>
          <Badge variant={PHASE_BADGE[description.phase]} className="normal-case tracking-normal">
            <LocalizedText messageKey="runtimeState.phase" params={{ phase: description.phase.replaceAll('-', '_') }} />
          </Badge>
          <span className="min-w-0 flex-1">
            <LocalizedText messageKey={description.messageKey} params={description.params} />
            {description.runtimeDetail && (description.phase === 'error' || description.phase === 'blocked') ? (
              <span className="block text-text-main" {...messageLanguageAttributes(message)}>{message.text}</span>
            ) : null}
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
      <AutomationReadinessRow featureId={featureId} onOpenSettings={onOpenSettings} />
    </div>
  );
};
