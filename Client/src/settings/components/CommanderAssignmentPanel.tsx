import React, { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, Save, Users } from 'lucide-react';
import type { GameStateV2 } from '../../api/Contracts';
import { Notifications } from '../../components/Notifications';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Modal } from '../../components/ui/Modal';
import { ModalTitle } from '../../components/ui/ModalTitle';
import { Switch } from '../../components/ui/Switch';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import { COMMANDER_ACTIVITY_LABEL_KEYS } from '../../Movement/types/CommanderActivity';
import type { CommanderFeatureConfigurationV2, CommanderFeatureID } from '../../Movement/types/CommanderFeatureAssignments';
import type { MovementViewModel } from '../../Movement/types/MovementState';
import {
  saveCommanderAssignmentDraft,
  savedCommanderAssignments,
  setFeatureCommandersAll,
  toggleFeatureCommander,
  type CommanderAssignmentSession,
} from '../requirements/commanderAssignmentDraft';
import {
  COMMANDER_FEATURE_LABEL_KEYS,
  assignmentImpact,
  assignmentImpactIsEmpty,
  commanderRowStatus,
  evaluateCommanderEligibility,
} from '../requirements/commanderEligibility';
import { ReadinessCheckLine } from './ReadinessPanel';
import { browserFrames, moveFocusAfterDialog } from './dialogFocus';

export interface CommanderAssignmentPanelProps {
  id: string;
  featureId: CommanderFeatureID;
  draftSession: CommanderAssignmentSession;
  state: GameStateV2 | null;
  movement: MovementViewModel | null;
  gameLoggedIn: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  /** Extra runtime rule shown under the summary (for example Fortress's fastest-commander rule). */
  note?: React.ReactNode;
  onSaved?: () => void;
  disabled?: boolean;
}

const ACTIVITY_BADGE = {
  free: 'success',
  outbound: 'warning',
  busy: 'warning',
  posted: 'warning',
  returning: 'outline',
  syncing: 'secondary',
  unknown: 'danger',
} as const;

/**
 * Assignments for one commander feature, edited in the module's context. It
 * edits only this feature's key of the shared document, discloses the impact,
 * and saves separately after confirmation. It never enables an automation and
 * never assigns a commander on its own.
 */
export const CommanderAssignmentPanel: React.FC<CommanderAssignmentPanelProps> = ({
  id,
  featureId,
  draftSession,
  state,
  movement,
  gameLoggedIn,
  expanded,
  onExpandedChange,
  note,
  onSaved,
  disabled = false,
}) => {
  const { t: localizeStatic, locale } = useLocale();
  const saved = useMemo(() => savedCommanderAssignments(draftSession.sections), [draftSession.sections]);
  const [pending, setPending] = useState<CommanderFeatureConfigurationV2 | null>(null);
  const [confirming, setConfirming] = useState(false);
  const reviewTrigger = useRef<HTMLButtonElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  // Set synchronously when the dialog opens, so the first close already knows where focus returns.
  const returnTarget = useRef<HTMLElement | null>(null);
  // The confirm dialog opens inside the settings modal: move focus into it after the dialog's own
  // open frame, and back to the trigger after it closes (retrying while the content is briefly inert).
  useEffect(() => {
    if (confirming) return moveFocusAfterDialog(() => cancelButton.current, browserFrames);
    const target = returnTarget.current;
    if (!target) return undefined;
    returnTarget.current = null;
    return moveFocusAfterDialog(() => (target.isConnected ? target : reviewTrigger.current), browserFrames);
  }, [confirming]);
  const openConfirm = (trigger: HTMLElement | null) => {
    returnTarget.current = trigger ?? reviewTrigger.current;
    setSaveError('');
    setConfirming(true);
  };
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  // Activity (for example "posted" after arrival) depends on the clock; re-evaluate periodically.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(timer);
  }, []);
  const effective = pending ?? saved;
  const observedIds = useMemo(() => Object.values(state?.commanders ?? {}).map((commander) => commander.id), [state?.commanders]);
  const report = evaluateCommanderEligibility({ featureId, state, assignments: effective, movement, gameLoggedIn, now });
  const impact = pending ? assignmentImpact(saved, pending, observedIds) : null;
  const dirty = impact != null && !assignmentImpactIsEmpty(impact);
  const listFormat = useMemo(() => new Intl.ListFormat(locale, { type: 'conjunction' }), [locale]);
  const featureList = (features: readonly CommanderFeatureID[]) => listFormat.format(features.map((feature) => localizeStatic(COMMANDER_FEATURE_LABEL_KEYS[feature])));
  const commanderName = (commanderId: number) => report.rows.find((row) => row.commanderId === commanderId)?.name || `#${commanderId}`;
  const headingId = `${id}-heading`;

  const confirmSave = async () => {
    if (!pending || saving) return;
    setSaving(true);
    setSaveError('');
    const result = await saveCommanderAssignmentDraft(draftSession, pending);
    setSaving(false);
    if (result.ok) {
      setPending(null);
      setConfirming(false);
      Notifications.success(localizeStatic('ui.settings.components.commanderAssignmentPanel.commander.assignments.saved.03792114'));
      onSaved?.();
      return;
    }
    setSaveError(result.error instanceof Error && result.error.message
      ? result.error.message
      : localizeStatic('ui.settings.components.commanderAssignmentPanel.could.not.save.commander.assignments.your.changes.3d32e432'));
  };

  return (
    <section id={id} className="rounded-xl border border-border-base bg-bg-elevated/40 p-4" aria-labelledby={headingId}>
      <button data-button-pattern="disclosure"
        type="button"
        className="flex w-full items-center justify-between gap-3 text-left"
        aria-expanded={expanded}
        aria-controls={`${id}-body`}
        onClick={() => onExpandedChange(!expanded)}
      >
        <h3 id={headingId} tabIndex={-1} className="flex items-center gap-2 text-title-sm font-bold text-text-main outline-none">
          <Users className="h-4 w-4 text-primary" aria-hidden="true" /> <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.commanders.for.this.automation.3d1d017d" />
        </h3>
        {expanded ? <ChevronDown className="h-4 w-4 text-text-muted" aria-hidden="true" /> : <ChevronRight className="h-4 w-4 text-text-muted" aria-hidden="true" />}
      </button>
      <ul className="mt-2 space-y-1.5">
        <ReadinessCheckLine check={report.assignment} />
        <ReadinessCheckLine check={report.activity} />
      </ul>
      {note ? <p className="mt-1.5 text-caption text-text-muted">{note}</p> : null}

      {expanded ? (
        <div id={`${id}-body`} className="mt-3 space-y-3 border-t border-border-base pt-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-caption text-text-muted">
              {report.implicitAll
                ? <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.all.commanders.allowed.default.turning.one.off.c4ac1b89" />
                : <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.only.the.commanders.switched.on.below.may.9606a88a" />}
            </span>
            <div className="flex gap-2">
              <Button variant="secondary" size="sm" disabled={disabled || saving} onClick={() => setPending(setFeatureCommandersAll(effective, featureId, true))}>
                <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.allow.all.56ac845a" />
              </Button>
              <Button variant="secondary" size="sm" disabled={disabled || saving} onClick={() => setPending(setFeatureCommandersAll(effective, featureId, false))}>
                <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.allow.none.6e08056b" />
              </Button>
            </div>
          </div>
          {report.rows.length === 0 ? (
            <p className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.no.commanders.are.observed.in.this.account.8d3e969e" /></p>
          ) : (
            <ul className="divide-y divide-border-base rounded-global border border-border-base">
              {report.rows.map((row) => {
                const status = commanderRowStatus(row);
                const stateId = `${id}-commander-${row.commanderId}-state`;
                return (
                  <li key={row.commanderId} className="flex flex-wrap items-center gap-2 px-3 py-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-caption font-semibold text-text-main">{row.name || `#${row.commanderId}`}</div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span id={stateId}>
                          {status.kind === 'activity' ? (
                            <Badge variant={ACTIVITY_BADGE[row.activity]} className="normal-case">
                              <LocalizedText messageKey={COMMANDER_ACTIVITY_LABEL_KEYS[row.activity]} />
                            </Badge>
                          ) : (
                            <span className="text-caption text-text-muted"><LocalizedText messageKey="commanderAssignment.offForThisAutomation" /></span>
                          )}
                        </span>
                        {row.meetsRequirements
                          ? null
                          : <Badge variant="danger" className="normal-case"><LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.does.not.meet.the.requirement.ad82a9a0" /></Badge>}
                        {row.otherFeatures.length > 0 ? (
                          <span className="text-caption text-text-muted">
                            <LocalizedText messageKey="commanderAssignment.alsoAssigned" params={{ features: featureList(row.otherFeatures) }} />
                          </span>
                        ) : null}
                      </div>
                    </div>
                    <Switch
                      checked={row.assigned}
                      disabled={disabled || saving}
                      onChange={(assigned) => setPending(toggleFeatureCommander(effective, featureId, row.commanderId, assigned, observedIds))}
                      ariaLabel={localizeStatic('commanderAssignment.toggle', { commander: row.name || `#${row.commanderId}` })}
                      ariaDescribedBy={stateId}
                    />
                  </li>
                );
              })}
            </ul>
          )}
          <p className="text-caption text-text-muted">
            <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.equipment.requirements.are.edited.under.commanders.assigning.1f4fcd46" />
          </p>
          {saveError && !confirming ? <p role="alert" className="text-caption font-semibold text-error">{saveError}</p> : null}
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="ghost" size="sm" disabled={!pending || saving} onClick={() => { setPending(null); setSaveError(''); }}>
              <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.discard.assignment.changes.c07d4258" />
            </Button>
            <Button variant="secondary" size="sm" ref={reviewTrigger} disabled={disabled || !dirty || saving} leftIcon={<Save className="h-4 w-4" />} onClick={(event) => openConfirm(event.currentTarget)}>
              <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.review.and.save.assignments.18f33af7" />
            </Button>
          </div>
        </div>
      ) : null}

      <Modal
        isOpen={confirming}
        onClose={() => { if (!saving) setConfirming(false); }}
        maxWidth="lg"
        title={(
          <ModalTitle icon={<Users className="h-5 w-5" />} description={localizeStatic('ui.settings.components.commanderAssignmentPanel.description.commander.assignments.are.shared.by.every.automation.0b5d8839')}>
            <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.save.commander.assignments.8a752dd8" />
          </ModalTitle>
        )}
        footer={(
          <div className="flex w-full items-center justify-end gap-2">
            <Button variant="ghost" ref={cancelButton} disabled={saving} onClick={() => setConfirming(false)}><LocalizedText messageKey="game.cancel" /></Button>
            <Button variant="secondary" isLoading={saving} disabled={saving} leftIcon={<Save className="h-4 w-4" />} onClick={() => void confirmSave()}>
              {saveError ? <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.try.again.d8b8392e" /> : <LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.save.assignments.79af590f" />}
            </Button>
          </div>
        )}
      >
        {impact ? (
          <div className="space-y-2 text-caption text-text-main">
            <ul className="list-disc space-y-1 pl-5">
              {impact.added.map((change) => (
                <li key={`add:${change.featureId}:${change.commanderId}`}>
                  <LocalizedText messageKey="commanderAssignment.impactAdded" params={{ commander: commanderName(change.commanderId), feature: localizeStatic(COMMANDER_FEATURE_LABEL_KEYS[change.featureId]) }} />
                </li>
              ))}
              {impact.removed.map((change) => (
                <li key={`remove:${change.featureId}:${change.commanderId}`}>
                  <LocalizedText messageKey="commanderAssignment.impactRemoved" params={{ commander: commanderName(change.commanderId), feature: localizeStatic(COMMANDER_FEATURE_LABEL_KEYS[change.featureId]) }} />
                </li>
              ))}
            </ul>
            {impact.featuresLeftEmpty.map((feature) => (
              <p key={`empty:${feature}`} className="rounded-global border border-warning/40 bg-warning/10 px-3 py-2">
                <LocalizedText messageKey="commanderAssignment.impactEmpty" params={{ feature: localizeStatic(COMMANDER_FEATURE_LABEL_KEYS[feature]) }} />
              </p>
            ))}
            {impact.commandersNowShared.map((entry) => (
              <p key={`shared:${entry.commanderId}`} className="text-text-muted">
                <LocalizedText messageKey="commanderAssignment.impactShared" params={{ commander: commanderName(entry.commanderId), features: featureList(entry.features) }} />
              </p>
            ))}
            <p className="text-text-muted"><LocalizedText messageKey="ui.settings.components.commanderAssignmentPanel.saving.assignments.does.not.start.or.enable.6ec1e604" /></p>
            {saveError ? <p role="alert" className="font-semibold text-error">{saveError}</p> : null}
          </div>
        ) : null}
      </Modal>
    </section>
  );
};
