import React, { useState } from 'react';
import { LocalizedText } from '../i18n/LocalizedText';
import type { AutomationGoal } from '../settings/onboarding/goals';
import { Button } from './ui/Button';
import { Modal } from './ui/Modal';

/**
 * Goal picker (CIT-19): eight curated goals first, "Show all automations" for the rest, and a plain link that records no
 * goal. Choosing a goal only opens the checklist for that automation; it saves nothing, starts nothing and gates nothing.
 */
export const GoalPicker: React.FC<{
  /** The goals on offer (the hosted product leaves out features it does not show). */
  goals: readonly AutomationGoal[];
  onChoose: (goalId: string) => void;
  onClose: () => void;
}> = ({ goals, onChoose, onClose }) => {
  const [showAll, setShowAll] = useState(false);
  const curated = goals.filter((goal) => goal.curated);
  const more = goals.filter((goal) => !goal.curated);
  const item = (goal: AutomationGoal) => (
    <li key={goal.id}>
      <button
        type="button"
        className="flex w-full min-w-0 flex-col gap-0.5 rounded-lg border border-border-base bg-bg-card/60 px-3 py-2 text-left hover:border-primary/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
        onClick={() => onChoose(goal.id)}
        data-goal={goal.id}
      >
        <span className="text-sm font-bold text-text-main"><LocalizedText messageKey={goal.titleKey} /></span>
        <span className="text-xs text-text-muted"><LocalizedText messageKey={goal.outcomeKey} /></span>
      </button>
    </li>
  );
  return (
    <Modal
      isOpen
      onClose={onClose}
      maxWidth="2xl"
      title={<LocalizedText messageKey="goalPicker.title" />}
      footer={<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="goalPicker.close" /></Button>}
    >
      <div className="space-y-3" data-goal-picker>
        <p className="text-xs text-text-muted"><LocalizedText messageKey="goalPicker.intro" /></p>
        <h3 className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="goalPicker.curated" /></h3>
        <ul className="space-y-2">{curated.map(item)}</ul>
        {showAll ? (
          <>
            <h3 className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="goalPicker.more" /></h3>
            <ul className="space-y-2">{more.map(item)}</ul>
          </>
        ) : null}
        <div className="flex flex-wrap items-center gap-3 border-t border-border-base pt-3">
          <Button variant="outline" size="sm" onClick={() => setShowAll((current) => !current)} aria-expanded={showAll}>
            <LocalizedText messageKey={showAll ? 'goalPicker.showFewer' : 'goalPicker.showAll'} />
          </Button>
          <button type="button" className="text-xs font-semibold text-primary underline underline-offset-2" onClick={onClose} data-goal-none>
            <LocalizedText messageKey="goalPicker.somethingElse" />
          </button>
        </div>
      </div>
    </Modal>
  );
};
