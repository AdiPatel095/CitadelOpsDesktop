import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowRight, Copy } from 'lucide-react';
import { useAuth } from '../../context/useAuth';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Modal } from '../../components/ui/Modal';
import { ModalTitle } from '../../components/ui/ModalTitle';
import { useLocale } from '../../i18n/useLocale';
import { LocalizedText } from '../../i18n/LocalizedText';
import { automationDuration } from '../../i18n/automationDuration';
import {
  applyCastleCopy,
  buildCopySelection,
  canIncludeDestination,
  configuredSources,
  defaultCopyInput,
  previewCastleCopy,
  reasonAsCheckState,
  selectionSize,
  withAllCompatible,
  withDestination,
  withField,
  withKeptInclude,
  type CastleCandidate,
  type CastleCopyContext,
  type CastleCopyDescriptor,
  type CastleCopyDestinationPreview,
  type CastleCopyPreview,
  type CastleCopySelectionInput,
  type CastleCopyState,
} from '../copy/castleCopy';
import { makeReplay, type CastleCopyReplay } from '../copy/castleCopyReplay';
import type { MessageKey } from '../../i18n/messages';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { ReadinessCheckLine } from './ReadinessPanel';
import { browserFrames, moveFocusAfterDialog } from './dialogFocus';

const STATE_BADGE: Record<CastleCopyState, 'success' | 'warning' | 'outline' | 'danger'> = {
  compatible: 'success', unavailable: 'warning', unknown: 'outline', incompatible: 'danger',
};

interface CastleCopyBodyProps<Draft, T> {
  descriptor: CastleCopyDescriptor<Draft, T>;
  context: CastleCopyContext;
  featureLabel: string;
  preview: CastleCopyPreview;
  input: CastleCopySelectionInput;
  onInput: (update: (current: CastleCopySelectionInput) => CastleCopySelectionInput) => void;
  sources: readonly CastleCandidate[];
  sourceKey: string;
  onSource: (key: string) => void;
  /** The automation is running: says when saved settings are used. */
  running: boolean;
  /** Minutes to its next check, when the game reported one (0 = unknown). */
  minutes: number;
  /** A line shown first, for example why the dialog reopened for review. */
  noticeKey?: MessageKey;
}

/** The dialog content, separate from the modal frame so it renders and tests without a portal. */
export function CastleCopyBody<Draft, T>({ descriptor, context, featureLabel, preview, input, onInput, sources, sourceKey, onSource, running, minutes, noticeKey }: CastleCopyBodyProps<Draft, T>) {
  const { locale } = useLocale();
  const source = context.candidates.find((candidate) => candidate.key === sourceKey);
  const consequential = descriptor.fields.find((field) => field.consequential);
  const setInput = onInput;
  const renderDestination = (destination: CastleCopyDestinationPreview) => {
    const checkable = canIncludeDestination(destination);
    const checked = input.destinations.has(destination.key);
    const inputId = `castle-copy-destination-${destination.key}`;
    return (
      <li key={destination.key} className="rounded-global border border-border-base bg-bg-card/40 p-3" data-castle-copy-destination={destination.key} data-castle-copy-state={destination.state}>
        <div className="flex flex-wrap items-center gap-2">
          <input
            id={inputId}
            type="checkbox"
            checked={checked && checkable}
            disabled={!checkable}
            onChange={(event) => setInput((current) => withDestination(current, destination.key, event.target.checked))}
            className="h-4 w-4"
          />
          <label htmlFor={inputId} className="min-w-0 whitespace-normal break-words text-sm font-bold text-text-main">{destination.castle.name}</label>
          <Badge variant={STATE_BADGE[destination.state]} className="normal-case tracking-normal">
            <LocalizedText messageKey="castleCopy.state" params={{ state: destination.state }} />
          </Badge>
        </div>
        {destination.reasons.length > 0 ? (
          <ul className="mt-2 space-y-1.5">
            {destination.reasons.map((reason) => (
              <ReadinessCheckLine key={reason.id} check={{ id: reason.id, state: reasonAsCheckState(reason.state), messageKey: reason.messageKey, params: reason.params, fix: reason.fix }} />
            ))}
          </ul>
        ) : null}
        {destination.changes.length > 0 ? (
          <table className="mt-2 w-full text-left text-xs">
            <thead className="text-[10px] uppercase tracking-wider text-text-muted">
              <tr>
                <th scope="col" className="py-1 pr-2 font-bold"><LocalizedText messageKey="castleCopy.fields" /></th>
                <th scope="col" className="py-1 font-bold"><LocalizedText messageKey="castleCopy.toColumn" /></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border-base">
              {destination.changes.map((change) => {
                const field = descriptor.fields.find((entry) => entry.id === change.fieldId)!;
                const on = input.fields.has(change.fieldId);
                return (
                  <tr key={change.fieldId} className={on && change.kind !== 'same' ? '' : 'text-text-muted'}>
                    <th scope="row" className="py-1 pr-2 align-top font-semibold"><LocalizedText messageKey={field.labelKey} /></th>
                    <td className="py-1">
                      {change.kind === 'same' ? (
                        <LocalizedText messageKey="castleCopy.same" />
                      ) : (
                        <>
                          <bdi className="text-text-muted"><LocalizedText {...describeProps(field.describe(change.from, context))} /></bdi>
                          <ArrowRight className="mx-1 inline h-3.5 w-3.5 shrink-0 align-text-bottom rtl:-scale-x-100" aria-hidden="true" />
                          <bdi className="font-semibold text-text-main"><LocalizedText {...describeProps(field.describe(change.to, context))} /></bdi>
                          {change.kind === 'kept-difference' ? (
                            <span className="ml-2 inline-flex items-center gap-1 text-warning">
                              <LocalizedText messageKey="castleCopy.kept" />
                              <label className="inline-flex items-center gap-1 font-semibold">
                                <input
                                  type="checkbox"
                                  checked={input.keptIncludes[destination.key]?.has(change.fieldId) === true}
                                  onChange={(event) => setInput((current) => withKeptInclude(current, destination.key, change.fieldId, event.target.checked))}
                                  className="h-3.5 w-3.5"
                                />
                                <LocalizedText messageKey="castleCopy.include" />
                              </label>
                            </span>
                          ) : null}
                          {field.noteKey && on && (checked || change.kind === 'set') ? (
                            <span className="ml-2 text-text-muted"><LocalizedText messageKey={field.noteKey} /></span>
                          ) : null}
                        </>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : null}
      </li>
    );
  };

  return (
      <div className="space-y-4 text-sm" data-castle-copy={descriptor.featureId}>
        {noticeKey ? (
          <p role="status" className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-xs font-semibold text-text-main" data-castle-copy-notice><LocalizedText messageKey={noticeKey} /></p>
        ) : null}
        <label className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="castleCopy.source" /></span>
          <select
            className="rounded-global border border-border-base bg-bg-input px-2 py-1 text-sm text-text-main"
            value={sourceKey}
            onChange={(event) => onSource(event.target.value)}
          >
            {sources.map((candidate) => <option key={candidate.key} value={candidate.key}>{candidate.name}</option>)}
          </select>
        </label>

        {!preview.sourceConfigured ? (
          <p className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-text-main"><LocalizedText messageKey="castleCopy.nothingToCopy" params={{ castle: source?.name ?? '' }} /></p>
        ) : (
          <>
            <fieldset className="space-y-1.5">
              <legend className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="castleCopy.fields" /></legend>
              {descriptor.fields.map((field) => (
                <label key={field.id} className="flex items-center gap-2 text-xs text-text-main">
                  <input
                    type="checkbox"
                    checked={input.fields.has(field.id)}
                    onChange={(event) => setInput((current) => withField(current, field.id, event.target.checked))}
                    className="h-4 w-4"
                  />
                  {field.consequential && descriptor.enabledIncludeKey
                    ? <LocalizedText messageKey={descriptor.enabledIncludeKey} params={{ feature: featureLabel }} />
                    : <LocalizedText messageKey={field.labelKey} />}
                </label>
              ))}
              {consequential ? <p className="pl-6 text-[11px] text-text-muted"><LocalizedText messageKey="castleCopy.include.helper" /></p> : null}
            </fieldset>

            <div className="space-y-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="castleCopy.destinations" /></h3>
                <Button variant="ghost" size="sm" onClick={() => setInput((current) => withAllCompatible(current, preview))}>
                  <LocalizedText messageKey="castleCopy.selectAllCompatible" />
                </Button>
              </div>
              {preview.destinations.length === 0 ? (
                <p className="text-xs text-text-muted"><LocalizedText messageKey="castleCopy.noDestinations" /></p>
              ) : (
                <ul className="space-y-2">{preview.destinations.map(renderDestination)}</ul>
              )}
            </div>
          </>
        )}

        <div className="space-y-1 border-t border-border-base pt-3 text-[11px] leading-relaxed text-text-muted">
          <p><LocalizedText messageKey={preview.unsupported} /></p>
          <p className="font-semibold text-text-main"><LocalizedText messageKey="castleCopy.draftOnly" /></p>
          {running ? (
            minutes > 0
              ? <p><LocalizedText messageKey="castleCopy.timing.runningIn" params={{ feature: featureLabel, duration: automationDuration(minutes, locale) }} /></p>
              : <p><LocalizedText messageKey="castleCopy.timing.running" params={{ feature: featureLabel }} /></p>
          ) : null}
        </div>
      </div>
  );
}

interface CastleCopyDialogProps<Draft, T> {
  descriptor: CastleCopyDescriptor<Draft, T>;
  draft: Draft;
  sourceKey: string;
  context: CastleCopyContext;
  /** Player-facing feature name, for example "Auto Towers". */
  featureLabel: string;
  /** The new draft and the record of what was applied, so a save conflict can re-apply the reviewed copy. */
  onApply: (next: Draft, replay: CastleCopyReplay) => void;
  onClose: () => void;
  /** Choices to pre-fill instead of the defaults (a copy re-checked against newer settings). */
  initialInput?: CastleCopySelectionInput;
  /** A line shown first in the dialog. */
  noticeKey?: MessageKey;
}

/**
 * Preview and choose what to copy from one castle to others (CIT-21). It reads the editor's DRAFT and returns a new
 * draft on "Apply to draft"; Cancel changes nothing. Nothing is written or started here: the editor's own Save
 * persists the section once, and saving never starts an automation.
 */
export function CastleCopyDialog<Draft, T>({ descriptor, draft, sourceKey: initialSource, context, featureLabel, onApply, onClose, initialInput, noticeKey }: CastleCopyDialogProps<Draft, T>) {
  const { automationEnabledByKey, automationStates } = useAuth();
  const [sourceKey, setSourceKey] = useState(initialSource);
  const allKeys = useMemo(() => context.candidates.map((candidate) => candidate.key), [context.candidates]);
  const preview = useMemo(
    () => previewCastleCopy(descriptor, draft, sourceKey, allKeys, context),
    [allKeys, context, descriptor, draft, sourceKey],
  );
  const [input, setInput] = useState(() => initialInput ?? defaultCopyInput(descriptor, preview));
  const sourceRef = useRef(initialSource);
  useEffect(() => {
    // A different source castle has different destinations and defaults; nothing else resets the choices.
    if (sourceRef.current === sourceKey) return;
    sourceRef.current = sourceKey;
    setInput(defaultCopyInput(descriptor, preview));
  }, [descriptor, preview, sourceKey]);

  const selection = useMemo(() => buildCopySelection(preview, input), [input, preview]);
  const sources = useMemo(() => {
    const configured = configuredSources(descriptor, draft, context);
    return configured.some((candidate) => candidate.key === sourceKey)
      ? configured
      : [...configured, ...context.candidates.filter((candidate) => candidate.key === sourceKey)];
  }, [context, descriptor, draft, sourceKey]);
  const source = context.candidates.find((candidate) => candidate.key === sourceKey);
  const enabledKey = AUTOMATION_ENABLED_KEYS[descriptor.featureId];
  const running = automationEnabledByKey[enabledKey] === true;
  const nextCheck = Date.parse(automationStates[descriptor.featureId]?.nextCheckAt ?? '');
  const minutes = Number.isFinite(nextCheck) && nextCheck > Date.now() ? Math.max(1, Math.ceil((nextCheck - Date.now()) / 60_000)) : 0;
  const size = selectionSize(selection);

  const apply = () => {
    onApply(applyCastleCopy(descriptor, draft, preview, selection), makeReplay(sourceKey, input, preview, selection));
    onClose();
  };

  return (
    <Modal
      isOpen
      onClose={onClose}
      maxWidth="3xl"
      title={<ModalTitle className="castle-copy-title" icon={<Copy className="h-5 w-5" />}><LocalizedText messageKey="castleCopy.title" params={{ feature: featureLabel, castle: source?.name ?? '' }} /></ModalTitle>}
      footer={(
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-text-muted"><LocalizedText messageKey="castleCopy.selected" params={{ count: size }} /></span>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}><LocalizedText messageKey="castleCopy.cancel" /></Button>
            <Button variant="primary" disabled={size === 0} onClick={apply}><LocalizedText messageKey="castleCopy.apply" /></Button>
          </div>
        </div>
      )}
    >
      <CastleCopyBody
        descriptor={descriptor}
        context={context}
        featureLabel={featureLabel}
        preview={preview}
        input={input}
        onInput={setInput}
        sources={sources}
        sourceKey={sourceKey}
        onSource={setSourceKey}
        running={running}
        minutes={minutes}
        noticeKey={noticeKey}
      />
    </Modal>
  );
}

function describeProps(value: { messageKey: import('../../i18n/messages').MessageKey; params?: import('../../i18n/messages').MessageParameters }) {
  return { messageKey: value.messageKey, params: value.params };
}

interface CastleCopyButtonProps<Draft, T> extends Omit<CastleCopyDialogProps<Draft, T>, 'onClose'> {
  disabled?: boolean;
  className?: string;
}

/** The explicit "Copy to other castles…" control on a castle's card or row. Nothing opens it but a click. */
export function CastleCopyButton<Draft, T>({ disabled, className, ...dialog }: CastleCopyButtonProps<Draft, T>) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const returnTarget = useRef<HTMLElement | null>(null);
  useEffect(() => {
    const target = returnTarget.current;
    if (open || !target) return undefined;
    returnTarget.current = null;
    // The dialog opens inside a settings modal: return focus to the Copy button once it closes.
    return moveFocusAfterDialog(() => (target.isConnected ? target : trigger.current), browserFrames);
  }, [open]);
  return (
    <>
      <Button
        ref={trigger}
        variant="ghost"
        size="sm"
        className={className}
        disabled={disabled}
        leftIcon={<Copy className="h-3.5 w-3.5" />}
        onClick={(event) => { returnTarget.current = event.currentTarget; setOpen(true); }}
      >
        <LocalizedText messageKey="castleCopy.button" />
      </Button>
      {open ? <CastleCopyDialog {...dialog} onClose={() => setOpen(false)} /> : null}
    </>
  );
}
