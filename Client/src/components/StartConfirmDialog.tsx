import React from 'react';
import { Button } from './ui/Button';
import { Modal } from './ui/Modal';
import { LocalizedText } from '../i18n/LocalizedText';
import { ReadinessCheckLine } from '../settings/components/ReadinessPanel';
import { blockedChecks, undecidedChecks } from '../settings/readiness/featureReadiness';
import type { ReadinessReport } from '../settings/readiness/Readiness';

/**
 * Start-time confirmation (CIT-20). Shown only when the SAVED settings have blocked checks. It lists what is
 * blocked and what the game decides, says the game stays authoritative, and offers "Fix first" (the default,
 * focused first) or "Start anyway". It submits nothing: the only mutation is the single `automation.enabled`
 * write the caller performs after "Start anyway".
 */
export interface PendingStart {
  featureId: string;
  report: ReadinessReport;
  decide: (proceed: boolean) => void;
}

export const StartConfirmDialog: React.FC<{ pending: PendingStart | null }> = ({ pending }) => {
  if (!pending) return null;
  const blocked = blockedChecks(pending.report);
  const undecided = undecidedChecks(pending.report);
  return (
    <Modal
      isOpen
      onClose={() => pending.decide(false)}
      maxWidth="md"
      title={<LocalizedText messageKey="startConfirm.title" />}
      footer={(
        <>
          <Button variant="primary" onClick={() => pending.decide(false)}>
            <LocalizedText messageKey="startConfirm.fixFirst" />
          </Button>
          <Button variant="outline" onClick={() => pending.decide(true)}>
            <LocalizedText messageKey="startConfirm.startAnyway" />
          </Button>
        </>
      )}
    >
      <div className="space-y-3" data-start-confirm={pending.featureId}>
        <p className="text-body text-text-main"><LocalizedText messageKey="startConfirm.intro" params={{ count: blocked.length }} /></p>
        <ul className="space-y-2">
          {blocked.map((check, index) => <ReadinessCheckLine key={`${check.id}:${check.slot ?? ''}:${index}`} check={check} />)}
        </ul>
        {undecided.length > 0 ? (
          <>
            <p className="text-body text-text-main"><LocalizedText messageKey="startConfirm.undecided" /></p>
            <ul className="space-y-2">
              {undecided.map((check, index) => <ReadinessCheckLine key={`${check.id}:${check.slot ?? ''}:${index}`} check={check} />)}
            </ul>
          </>
        ) : null}
        <p className="border-t border-border-base pt-3 text-caption text-text-muted"><LocalizedText messageKey="startConfirm.authority" /></p>
      </div>
    </Modal>
  );
};
