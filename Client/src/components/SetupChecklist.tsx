import React from 'react';
import { CheckCircle2, ChevronDown, Circle, CircleAlert, CircleDashed, Clock3, CircleHelp } from 'lucide-react';
import { LocalizedText } from '../i18n/LocalizedText';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import type { MessageKey } from '../i18n/messages';
import { requestConnectionRepair } from '../settings/connection/repairRequest';
import type { ReadinessCheck } from '../settings/readiness/Readiness';
import type { ChecklistStep, ChecklistStepId, ChecklistStepState } from '../settings/onboarding/checklist';
import type { AutomationGoal } from '../settings/onboarding/goals';
import { useGoalChecklist } from '../settings/onboarding/useChecklist';
import { Button } from './ui/Button';
import { StateLegend } from './StateLegend';

const STEP_NAME: Record<ChecklistStepId, MessageKey> = {
  connection: 'checklist.step.connection',
  choices: 'checklist.step.choices',
  readiness: 'checklist.step.readiness',
  activation: 'checklist.step.activation',
  'first-result': 'checklist.step.firstResult',
};

const TONE: Record<ChecklistStepState, string> = {
  done: 'text-primary', current: 'text-primary', todo: 'text-text-muted', blocked: 'text-error', waiting: 'text-warning', unknown: 'text-text-muted',
};

const StepIcon: React.FC<{ step: ChecklistStep }> = ({ step }) => {
  // Only a confirmed first result is green; a finished step in general is the primary color.
  const tone = step.id === 'first-result' && step.state === 'done' ? 'text-success' : TONE[step.state];
  const className = `mt-0.5 h-4 w-4 shrink-0 ${tone}`;
  if (step.state === 'done') return <CheckCircle2 className={className} aria-hidden="true" />;
  if (step.state === 'blocked') return <CircleAlert className={className} aria-hidden="true" />;
  if (step.state === 'waiting') return <Clock3 className={className} aria-hidden="true" />;
  if (step.state === 'unknown') return <CircleHelp className={className} aria-hidden="true" />;
  if (step.state === 'current') return <Circle className={className} aria-hidden="true" />;
  return <CircleDashed className={className} aria-hidden="true" />;
};

const StepRow: React.FC<{
  step: ChecklistStep;
  current: boolean;
  onOpenEditor: (check?: ReadinessCheck) => void;
  onGoToSwitch: () => void;
}> = ({ step, current, onOpenEditor, onGoToSwitch }) => {
  const reason = useLocalizedMessage(undefined, step.detail?.text ?? '');
  const next = step.nextStep;
  return (
    <li
      className="flex items-start gap-2 rounded-lg px-2 py-1.5 text-xs leading-relaxed"
      aria-current={current ? 'step' : undefined}
      data-checklist-step={step.id}
      data-step-state={step.state}
    >
      <StepIcon step={step} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-bold text-text-main"><LocalizedText messageKey={STEP_NAME[step.id]} /></span>
          <span className={`text-[10px] font-black uppercase tracking-wide ${TONE[step.state]}`}><LocalizedText messageKey="checklist.state" params={{ state: step.state }} /></span>
        </div>
        <p className="text-text-muted"><LocalizedText messageKey={step.messageKey} params={step.params} /></p>
        {step.detail?.text ? (
          <p className="text-text-main" {...messageLanguageAttributes(reason)}>{step.detail.text}</p>
        ) : step.detail ? (
          <p className="text-text-main"><LocalizedText messageKey={step.detail.messageKey} params={step.detail.params} /></p>
        ) : null}
        {step.evidence.at && step.state !== 'todo' ? (
          <p className="text-[10px] text-text-muted"><LocalizedText messageKey="checklist.evidenceAt" params={{ at: Date.parse(step.evidence.at) }} /></p>
        ) : null}
      </div>
      {next && next.kind !== 'wait' && step.state !== 'done' ? (
        <Button
          variant={current ? 'primary' : 'outline'}
          size="sm"
          className="shrink-0"
          onClick={() => {
            if (next.kind === 'repair-connection' || next.kind === 'account-center') requestConnectionRepair();
            else if (next.kind === 'open-editor') onOpenEditor(next.check);
            else if (next.kind === 'start') onGoToSwitch();
          }}
        >
          <LocalizedText messageKey={
            next.kind === 'repair-connection' ? 'checklist.action.repair'
              : next.kind === 'account-center' ? 'checklist.action.accountCenter'
                : next.kind === 'open-editor' ? 'checklist.action.openEditor'
                  : 'checklist.action.turnOn'
          } />
        </Button>
      ) : null}
    </li>
  );
};

export interface SetupChecklistProps {
  goal: AutomationGoal;
  collapsed: boolean;
  onSetCollapsed: (collapsed: boolean) => void;
  /** Opens the feature's editor, focusing the control a blocked check points at when there is one. */
  onOpenEditor: (check?: ReadinessCheck) => void;
  /** Scrolls to and focuses the feature row's switch; the switch itself stays the one place a start is confirmed. */
  onGoToSwitch: () => void;
  onDone: () => void;
  onChooseAnother: () => void;
}

/**
 * The resumable goal checklist (CIT-19): five steps computed from observed state, one action per step, and the state
 * legend that keeps draft, saved, connected, on and first result apart. It has no switch and writes nothing: turning
 * the automation on is the row's own switch (and the CIT-20 Start confirmation), reached from "Go to the switch".
 */
export const SetupChecklist: React.FC<SetupChecklistProps> = ({ goal, collapsed, onSetCollapsed, onOpenEditor, onGoToSwitch, onDone, onChooseAnother }) => {
  const { checklist, legend } = useGoalChecklist(goal);
  return (
    <section
      id="setup-checklist"
      tabIndex={-1}
      className="rounded-2xl border border-primary/30 bg-bg-card/70 p-4 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      aria-labelledby={`goal-title-${goal.id}`}
      data-setup-checklist={goal.featureId}
      data-checklist-complete={checklist.complete ? 'true' : 'false'}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="checklist.eyebrow" /></div>
          <h2 id={`goal-title-${goal.id}`} className="text-base font-black text-text-main"><LocalizedText messageKey={goal.titleKey} /></h2>
          <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey={goal.outcomeKey} /></p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="ghost" size="sm" onClick={() => onSetCollapsed(!collapsed)} aria-expanded={!collapsed} aria-controls={`goal-steps-${goal.id}`} rightIcon={<ChevronDown className={`h-4 w-4 transition-transform ${collapsed ? '' : 'rotate-180'}`} aria-hidden="true" />}>
            <LocalizedText messageKey={collapsed ? 'checklist.expand' : 'checklist.collapse'} />
          </Button>
        </div>
      </div>
      <StateLegend values={legend} className="mt-3" />
      <div id={`goal-steps-${goal.id}`} hidden={collapsed} className="mt-3 space-y-3">
        <ol className="space-y-1" aria-label={undefined}>
          {checklist.steps.map((step) => (
            <StepRow key={step.id} step={step} current={checklist.current === step.id} onOpenEditor={onOpenEditor} onGoToSwitch={onGoToSwitch} />
          ))}
        </ol>
        {checklist.complete ? (
          <p className="text-xs font-semibold text-text-main" data-checklist-all-done><LocalizedText messageKey="checklist.allDone" /></p>
        ) : null}
        <p className="text-[11px] text-text-muted"><LocalizedText messageKey="checklist.notComplete" /></p>
        <div className="flex flex-wrap gap-2 border-t border-border-base pt-3">
          <Button variant="outline" size="sm" onClick={() => onOpenEditor()}><LocalizedText messageKey="checklist.action.openEditor" /></Button>
          <Button variant="outline" size="sm" onClick={onChooseAnother}><LocalizedText messageKey="checklist.chooseAnother" /></Button>
          <Button variant="ghost" size="sm" onClick={onDone}><LocalizedText messageKey="checklist.doneWithGoal" /></Button>
        </div>
      </div>
    </section>
  );
};
