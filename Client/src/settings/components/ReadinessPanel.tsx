import React from 'react';
import { CheckCircle2, CircleDashed, Clock3, ClipboardCheck, XCircle } from 'lucide-react';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { LocalizedText } from '../../i18n/LocalizedText';
import type { MessageKey } from '../../i18n/messages';
import type { CheckState, ReadinessCheck, ReadinessReport } from '../readiness/Readiness';

const STATE_TONE: Record<CheckState, string> = {
  valid: 'text-success',
  blocked: 'text-error',
  pending: 'text-warning',
  unavailable: 'text-text-muted',
};

const STATE_BADGE: Record<CheckState, 'success' | 'danger' | 'warning' | 'outline'> = {
  valid: 'success',
  blocked: 'danger',
  pending: 'warning',
  unavailable: 'outline',
};

const StateIcon: React.FC<{ state: CheckState }> = ({ state }) => {
  const className = `mt-0.5 h-4 w-4 shrink-0 ${STATE_TONE[state]}`;
  if (state === 'valid') return <CheckCircle2 className={className} aria-hidden="true" />;
  if (state === 'blocked') return <XCircle className={className} aria-hidden="true" />;
  if (state === 'pending') return <Clock3 className={className} aria-hidden="true" />;
  return <CircleDashed className={className} aria-hidden="true" />;
};

/** One readiness line: state, message and, where it helps, the in-context fix. */
export const ReadinessCheckLine: React.FC<{
  check: ReadinessCheck;
  slotLabelKey?: MessageKey;
  onFix?: (check: ReadinessCheck) => void;
}> = ({ check, slotLabelKey, onFix }) => (
  <li className="flex items-start gap-2 text-xs leading-relaxed text-text-main">
    <StateIcon state={check.state} />
    <span className="sr-only"><LocalizedText messageKey="readiness.state" params={{ state: check.state }} /></span>
    <span className="min-w-0 flex-1">
      {slotLabelKey ? <span className="font-bold text-text-muted"><LocalizedText messageKey={slotLabelKey} />{': '}</span> : null}
      <LocalizedText messageKey={check.messageKey} params={check.params} />
      {check.fix === 'assignment' && !onFix ? (
        <span className="block text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.readinessPanel.assign.commanders.under.commanders.features.repairing.assignments.a9c68853" /></span>
      ) : null}
      {check.fix === 'connection' ? (
        <span className="block text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.readinessPanel.this.updates.by.itself.once.the.game.7b60fd8c" /></span>
      ) : null}
    </span>
    {onFix && check.state !== 'valid' && (check.fix === 'settings' || check.fix === 'presets' || check.fix === 'assignment') ? (
      <Button variant="ghost" size="sm" className="shrink-0" onClick={() => onFix(check)}>
        <LocalizedText messageKey="ui.settings.components.readinessPanel.fix.21f1595b" />
      </Button>
    ) : null}
  </li>
);

export interface ReadinessPanelProps {
  report: ReadinessReport;
  /** Label for slot-specific checks (for example Nomad / Samurai). */
  slotLabelKeys?: Readonly<Record<string, MessageKey>>;
  onFix?: (check: ReadinessCheck) => void;
}

/**
 * Non-action readiness review. It never sends game actions, and saving the
 * module does not start the automation.
 */
export const ReadinessPanel: React.FC<ReadinessPanelProps> = ({ report, slotLabelKeys, onFix }) => (
  <section className="rounded-xl border border-border-base bg-bg-elevated/40 p-4" aria-labelledby={`readiness-${report.featureId}`}>
    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 id={`readiness-${report.featureId}`} className="flex items-center gap-2 text-sm font-black text-text-main">
        <ClipboardCheck className="h-4 w-4 text-primary" aria-hidden="true" /> <LocalizedText messageKey="ui.settings.components.readinessPanel.before.you.start.74e492d5" />
      </h3>
      <Badge variant={STATE_BADGE[report.overall]} className="normal-case tracking-normal">
        <LocalizedText messageKey="readiness.overall" params={{ state: report.overall }} />
      </Badge>
    </div>
    <ul className="space-y-2">
      {report.checks.map((check, index) => (
        <ReadinessCheckLine
          key={`${check.id}:${check.slot ?? ''}:${index}`}
          check={check}
          slotLabelKey={check.slot ? slotLabelKeys?.[check.slot] : undefined}
          onFix={onFix}
        />
      ))}
    </ul>
    <p className="mt-3 border-t border-border-base pt-3 text-[11px] text-text-muted">
      <LocalizedText messageKey="ui.settings.components.readinessPanel.checks.run.against.these.settings.and.current.b517e885" />
    </p>
  </section>
);
