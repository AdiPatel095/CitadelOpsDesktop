import React, { useMemo, useState } from 'react';
import { CircleStop, Info } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { LocalizedText } from '../i18n/LocalizedText';
import { describeStopSemantics } from '../settings/readiness/stopSemantics';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../settings/disclosure/placement';
import { useAutomationPlayerStatus } from '../settings/readiness/useAutomationPlayerStatus';
import { StatusBadge } from './ui/StatusBadge';
import { Button } from './ui/Button';

/**
 * Stop and its plain-language semantics (CIT-20). Stop writes only `automation.enabled[key] = false`; the
 * state shown always follows the saved value, never the click. A failed write stays visible with the reason
 * and a Retry, and the automation keeps being described as on until a write succeeds. The text says what the
 * code does (no new decisions, the running action is asked to stop at its next step, steps the game already
 * accepted are not recalled) and promises no recall, undo or instant cancellation.
 */
export interface StopControlProps {
  /** The `automation.enabled` key (for example `auto_towers`). */
  enabledKey: string;
  featureId: string;
  /** `button`: Stop button, failure and semantics. `notice`: only the failure/Retry and the semantics (the row switch is the Stop). */
  variant?: 'button' | 'notice';
  className?: string;
}

export const StopControl: React.FC<StopControlProps> = ({ enabledKey, featureId, variant = 'button', className = '' }) => {
  const { automationEnabledByKey, automationTimedUntilByKey, automationWriteFailures, setAutomationEnabled, gameLoggedIn } = useAuth();
  const { operations } = useCitadelAPI();
  const [busy, setBusy] = useState(false);
  const on = automationEnabledByKey[enabledKey] === true;
  const timedUntil = automationTimedUntilByKey[enabledKey];
  const failure = automationWriteFailures[enabledKey];
  const semantics = useMemo(() => describeStopSemantics(featureId, {
    enabled: on ? { configured: true, enabled: true, ...(timedUntil ? { expiresAtMs: timedUntil } : {}) } : undefined,
    operations,
    connected: gameLoggedIn,
    now: Date.now(),
  }), [featureId, gameLoggedIn, on, operations, timedUntil]);

  const write = async (enabled: boolean) => {
    if (busy) return;
    setBusy(true);
    try {
      await setAutomationEnabled(enabledKey, enabled);
    } catch {
      // Kept in `automationWriteFailures` and shown below; the switch state does not change.
    } finally {
      setBusy(false);
    }
  };

  const showSemantics = on || semantics.inFlight.length > 0;
  return (
    <div className={`space-y-1.5 ${className}`} data-stop-control={featureId}>
      {variant === 'button' && on ? (
        <Button variant="outline" size="sm" onClick={() => { void write(false); }} isLoading={busy} leftIcon={<CircleStop className="h-4 w-4" />}>
          <LocalizedText messageKey="stopSemantics.stop" />
        </Button>
      ) : null}
      {failure ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-caption font-semibold text-error">
          <span>
            <LocalizedText messageKey={failure.intent === 'stop' ? 'stopSemantics.failed' : 'stopSemantics.failedStart'} />
            {failure.message ? ` ${failure.message}` : ''}
          </span>
          <Button variant="outline" size="sm" onClick={() => { void write(failure.intent === 'start'); }} isLoading={busy}>
            <LocalizedText messageKey={failure.intent === 'stop' ? 'stopSemantics.retry' : 'startConfirm.startAnyway'} />
          </Button>
        </div>
      ) : null}
      {showSemantics ? (
        <details className="text-caption text-text-muted">
          <summary className="flex cursor-pointer items-center gap-1 font-semibold text-text-main">
            <Info className="h-3 w-3" aria-hidden="true" /> <LocalizedText messageKey="stopSemantics.title" />
          </summary>
          <ul className="mt-1 list-disc space-y-1 pl-4">
            {semantics.lines.map((line) => (
              <li key={line.key}><LocalizedText messageKey={line.key} params={line.params} /></li>
            ))}
          </ul>
          {semantics.inFlight.length > 0 ? (
            <ul className="mt-1 list-none space-y-0.5 pl-4 font-mono text-caption">
              {semantics.inFlight.map((operation) => <li key={operation.id}>{operation.summary ?? operation.intent}</li>)}
            </ul>
          ) : null}
        </details>
      ) : null}
    </div>
  );
};

/** Footer of a settings editor: the current phase and Stop, reachable without scrolling. Never starts anything. */
export const StopFooter: React.FC<{ featureId: SettingsFeatureId }> = ({ featureId }) => {
  const player = useAutomationPlayerStatus(featureId).overall;
  return (
    <div className="mr-auto flex min-w-0 flex-wrap items-start gap-2" data-settings-stop-footer={featureId}>
      <StatusBadge {...player} />
      <StopControl enabledKey={AUTOMATION_ENABLED_KEYS[featureId]} featureId={featureId} />
    </div>
  );
};
