import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Camera, Copy, Edit3, Save, Shield } from 'lucide-react';
import {
  inlineDefenseFromPreset,
  summarizeDefenseSetupRef,
  type DefenseSetupRef,
  type InlineDefenseSetup,
} from '../../defensePresets/AppCreatedDefensePresets';
import { appCreatedPresetBadge, attackPresetSelectOptions } from '../../attackPresets/AttackPresetOptionLabel';
import { presetReferrers, presetSlotDefinition, type PresetReference } from '../../attackPresets/AttackPresetReferences';
import {
  emptyDefensePresetDraft,
  type DefensePresetDocument,
  type DefensePresetDraft,
} from '../../defensePresets/DefensePresetTypes';
import DefensePresetEditor from '../../components/DefensePresetEditor';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { ChoiceChipGroup } from '../../components/ui/ChoiceChipGroup';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { ModalTitle } from '../../components/ui/ModalTitle';
import { Select } from '../../components/ui/Select';
import { useLocale } from '../../i18n/useLocale';
import { LocalizedText } from '../../i18n/LocalizedText';
import { isRecordOwnedBy, recordOwner } from '../../presets/AppCreatedRecords';
import type { ReadinessCheck } from '../readiness/Readiness';
import { DEFENSE_NO_TOOLS } from '../readiness/khanReadiness';
import type { KhanDefenseStarter } from '../onboarding/KhanDefenseStarter';
import { validateUserPresetName } from '../AppCreatedPresetSave';
import { ReadinessCheckLine } from './ReadinessPanel';

type FieldMode = 'saved' | 'inline';

export interface DefenseSetupFieldProps {
  /** DOM id of the field container; readiness fixes scroll and focus here. */
  id?: string;
  label: React.ReactNode;
  section: string;
  slot: string;
  moduleLabel: string;
  slotLabel: string;
  value: DefenseSetupRef;
  onChange: (ref: DefenseSetupRef) => void;
  document: DefensePresetDocument;
  references: readonly PresetReference[];
  /** Account-derived starter (current main-castle defense) or the reason it is unavailable. */
  starter: KhanDefenseStarter;
  /** Name of the castle the starter is captured from, for the preview. */
  starterCastleName?: string;
  /** Explicit Save as preset: creates a normal user defense preset and returns its id. */
  onSaveAsPreset: (setup: InlineDefenseSetup, name: string) => Promise<string>;
  readinessChecks?: readonly ReadinessCheck[];
  disabled?: boolean;
}

function modeFor(ref: DefenseSetupRef): FieldMode | null {
  if (ref.source === 'preset') return 'saved';
  if (ref.source === 'inline') return 'inline';
  return null;
}

/**
 * Khan main-castle defense (CIT-16): reuse a saved defense preset or configure
 * it here. "Configure here" is persisted as this slot's app-created defense
 * preset when the module is saved; only "Save as preset" creates a user preset.
 */
export const DefenseSetupField: React.FC<DefenseSetupFieldProps> = ({
  id,
  label,
  section,
  slot,
  moduleLabel,
  slotLabel,
  value,
  onChange,
  document,
  references,
  starter,
  starterCastleName,
  onSaveAsPreset,
  readinessChecks = [],
  disabled = false,
}) => {
  const { t: localizeStatic, locale } = useLocale();
  const fieldId = useId();
  const [mode, setMode] = useState<FieldMode | null>(() => modeFor(value));
  const [editing, setEditing] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [saveAsOpen, setSaveAsOpen] = useState(false);
  const [saveAsName, setSaveAsName] = useState('');
  const [saveAsError, setSaveAsError] = useState('');
  const [saveAsBusy, setSaveAsBusy] = useState(false);
  const [saveAsFailed, setSaveAsFailed] = useState(false);
  const saveAsInput = useRef<HTMLInputElement>(null);
  const fieldRef = useRef<HTMLDivElement>(null);
  const starterTrigger = useRef<HTMLButtonElement>(null);
  const saveAsTrigger = useRef<HTMLButtonElement>(null);
  const returnFocus = (trigger: React.RefObject<HTMLButtonElement | null>) => {
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
      const target = trigger.current?.isConnected && !trigger.current.disabled ? trigger.current : fieldRef.current;
      target?.focus();
    }));
  };
  const closePreview = () => {
    setPreviewOpen(false);
    returnFocus(starterTrigger);
  };
  const closeSaveAs = () => {
    setSaveAsOpen(false);
    returnFocus(saveAsTrigger);
  };

  useEffect(() => {
    const next = modeFor(value);
    if (next) setMode(next);
  }, [value]);

  useEffect(() => {
    if (!saveAsOpen) return;
    let inner = 0;
    const outer = window.requestAnimationFrame(() => {
      inner = window.requestAnimationFrame(() => saveAsInput.current?.focus());
    });
    return () => {
      window.cancelAnimationFrame(outer);
      window.cancelAnimationFrame(inner);
    };
  }, [saveAsOpen]);

  const summary = useMemo(() => summarizeDefenseSetupRef(value, document), [document, value]);
  const selectedPreset = value.source === 'preset' && !value.missing
    ? document.presets.find((preset) => preset.id === value.presetId)
    : undefined;
  const pickerOptions = useMemo(
    () => attackPresetSelectOptions(document.presets.filter((preset) => !isRecordOwnedBy(preset, section, slot))),
    [document.presets, section, slot],
  );
  const listFormat = useMemo(() => new Intl.ListFormat(locale, { type: 'conjunction' }), [locale]);
  const describeReferences = (items: readonly PresetReference[]) => listFormat.format(items.map((reference) => (
    `${localizeStatic(reference.moduleLabelKey)} · ${localizeStatic(reference.slotLabelKey)}`
  )));
  const otherReferrers = value.source === 'preset'
    ? presetReferrers(references, value.presetId).filter((reference) => reference.section !== section || reference.slot !== slot)
    : [];
  const owner = selectedPreset ? recordOwner(selectedPreset) : null;
  const ownerDefinition = owner ? presetSlotDefinition('defense.presets', owner.section, owner.slot) : undefined;
  const inlineSetup = value.source === 'inline' ? value.setup : null;
  const inlineId = value.source === 'inline' ? value.presetId : '';
  const generatedName = summary.name || localizeStatic('attackPresets.appCreatedName', { module: moduleLabel, slot: slotLabel });
  const starterSummary = useMemo(
    () => starter.setup ? summarizeDefenseSetupRef({ source: 'inline', presetId: '', setup: starter.setup, missing: false }, document).summary : null,
    [document, starter.setup],
  );

  const applyInline = (setup: InlineDefenseSetup) => {
    onChange({ source: 'inline', presetId: inlineId, setup, missing: false });
    setMode('inline');
  };

  const choosePreset = (presetId: string) => {
    const preset = document.presets.find((candidate) => candidate.id === presetId);
    if (!preset) return;
    const presetOwner = recordOwner(preset);
    onChange(presetOwner
      ? { source: 'preset', presetId, missing: false, appCreatedBy: presetOwner }
      : { source: 'preset', presetId, missing: false });
  };

  const saveDefense = (draft: DefensePresetDraft) => {
    applyInline(inlineDefenseFromPreset(draft));
    setEditing(false);
  };

  const submitSaveAs = async () => {
    if (!inlineSetup || saveAsBusy) return;
    const name = saveAsName.trim();
    const problem = validateUserPresetName(name, document.presets);
    if (problem === 'empty') {
      setSaveAsError(localizeStatic('ui.settings.components.eventAttackSetupField.enter.a.name.for.the.preset.9706f768'));
      return;
    }
    if (problem === 'duplicate') {
      setSaveAsError(localizeStatic('eventAttackSetup.duplicateName', { name }));
      return;
    }
    setSaveAsBusy(true);
    setSaveAsError('');
    setSaveAsFailed(false);
    try {
      const presetId = await onSaveAsPreset(inlineSetup, name);
      onChange({ source: 'preset', presetId, missing: false });
      setMode('saved');
      closeSaveAs();
    } catch (error) {
      setSaveAsFailed(true);
      setSaveAsError(error instanceof Error && error.message ? error.message : localizeStatic('ui.settings.components.eventAttackSetupField.could.not.save.the.preset.your.setup.d9f51983'));
    } finally {
      setSaveAsBusy(false);
    }
  };

  const showSaved = mode !== 'inline';
  const showInline = mode !== 'saved';
  const summaryBadge = summary.summary ? (
    <Badge variant="outline" className="normal-case tracking-normal">
      <LocalizedText messageKey="defenseSetup.summary" params={{ tools: summary.summary.toolAmount, types: summary.summary.toolTypes.length }} />
    </Badge>
  ) : null;
  const toollessNote = summary.summary && summary.summary.toolAmount === 0 ? (
    <p className="text-[11px] font-semibold text-warning"><LocalizedText messageKey={DEFENSE_NO_TOOLS} /></p>
  ) : null;

  return (
    <div id={id} ref={fieldRef} tabIndex={-1} className="min-w-0 space-y-3 rounded-xl border border-border-base bg-bg-app/35 p-3 outline-none focus-visible:ring-2 focus-visible:ring-primary/40">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
          <Shield className="h-3.5 w-3.5" aria-hidden="true" /> {label}
        </span>
        <ChoiceChipGroup<FieldMode>
          size="sm"
          ariaLabel={localizeStatic('ui.settings.components.defenseSetupField.aria-label.how.to.set.up.this.defense.8023a726')}
          options={[
            { value: 'saved', label: <LocalizedText messageKey="ui.settings.components.defenseSetupField.saved.defense.preset.d8fcb706" /> },
            { value: 'inline', label: <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.configure.here.51d9eba5" /> },
          ]}
          selected={mode ? [mode] : []}
          onToggle={(next) => setMode((current) => current === next ? null : next)}
          disabled={disabled}
        />
      </div>

      {showSaved ? (
        <div className="space-y-2">
          <Select
            value={value.source === 'preset' && !value.missing ? value.presetId : ''}
            onChange={choosePreset}
            options={pickerOptions}
            placeholder={pickerOptions.length > 0
              ? <LocalizedText messageKey="ui.settings.components.defenseSetupField.choose.a.saved.defense.preset.2033e814" />
              : <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.no.saved.presets.yet.f4a93a38" />}
            ariaLabel={localizeStatic('ui.settings.components.defenseSetupField.aria-label.saved.defense.preset.d8fcb706')}
            disabled={disabled}
            searchable
            menuGrowToViewport
          />
          {value.source === 'preset' && value.missing ? (
            <p role="alert" className="text-xs text-error"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.the.selected.preset.does.not.exist.anymore.9c6d37cc" /></p>
          ) : null}
          {selectedPreset ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold text-text-main">{selectedPreset.name}</span>
              {selectedPreset.app ? appCreatedPresetBadge() : null}
              {summaryBadge}
            </div>
          ) : null}
          {selectedPreset ? toollessNote : null}
          {owner && ownerDefinition ? (
            <p className="text-[11px] text-warning">
              <LocalizedText messageKey="attackPresets.createdByOther" params={{ module: `${localizeStatic(ownerDefinition.moduleLabelKey)} · ${localizeStatic(ownerDefinition.slotLabelKey)}` }} />
            </p>
          ) : null}
          {otherReferrers.length > 0 ? (
            <p className="text-[11px] text-text-muted">
              <LocalizedText messageKey="eventAttackSetup.alsoUsedBy" params={{ referrers: describeReferences(otherReferrers) }} />
            </p>
          ) : null}
          {selectedPreset ? (
            <Button
              variant="outline"
              size="sm"
              disabled={disabled}
              leftIcon={<Copy className="h-4 w-4" />}
              onClick={() => applyInline(inlineDefenseFromPreset(selectedPreset))}
            >
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.customize.this.preset.998581b3" />
            </Button>
          ) : null}
          {mode === 'saved' && value.source === 'inline' ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.defenseSetupField.this.defense.still.uses.the.setup.configured.22a9bc7d" /></p>
          ) : null}
        </div>
      ) : null}

      {showInline ? (
        <div className="space-y-2 border-t border-border-base pt-2">
          {inlineSetup ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold text-text-main">{generatedName}</span>
              {appCreatedPresetBadge()}
              {summaryBadge}
            </div>
          ) : null}
          {inlineSetup ? toollessNote : null}
          {inlineSetup ? null : value.source === 'preset' ? (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.defenseSetupField.this.defense.uses.the.saved.preset.until.c319bac8" /></p>
          ) : (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.defenseSetupField.no.defense.is.configured.here.yet.edit.6eac4105" /></p>
          )}
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" disabled={disabled} leftIcon={<Edit3 className="h-4 w-4" />} onClick={() => setEditing(true)}>
              <LocalizedText messageKey="ui.settings.components.defenseSetupField.edit.defense.18cde234" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || starter.setup == null}
              ref={starterTrigger}
              leftIcon={<Camera className="h-4 w-4" />}
              onClick={() => setPreviewOpen(true)}
            >
              <LocalizedText messageKey="ui.settings.components.defenseSetupField.use.current.main.castle.defense.9ef0789a" />
            </Button>
            {inlineSetup ? (
              <Button variant="ghost" size="sm" disabled={disabled} ref={saveAsTrigger} leftIcon={<Save className="h-4 w-4" />} onClick={() => {
                setSaveAsName('');
                setSaveAsError('');
                setSaveAsFailed(false);
                setSaveAsOpen(true);
              }}>
                <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.save.as.preset.da93e96d" />
              </Button>
            ) : null}
          </div>
          {starter.reason ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey={starter.reason} /></p>
          ) : null}
          {inlineSetup ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.defenseSetupField.saved.as.a.defense.preset.marked.created.a3d9482e" /></p>
          ) : null}
        </div>
      ) : null}

      {readinessChecks.length > 0 ? (
        <ul className="space-y-1.5 border-t border-border-base pt-2">
          {readinessChecks.map((check, index) => <ReadinessCheckLine key={`${check.id}:${index}`} check={check} />)}
        </ul>
      ) : null}

      {editing ? (
        <DefensePresetEditor
          initialDraft={inlineSetup ? { name: generatedName, ...inlineSetup } : { ...emptyDefensePresetDraft(), name: generatedName }}
          saving={false}
          nameField="hidden"
          saveLabel={<LocalizedText messageKey="ui.settings.components.defenseSetupField.apply.defense.9f2f4f5e" />}
          onClose={() => setEditing(false)}
          onSave={saveDefense}
        />
      ) : null}

      <Modal
        isOpen={previewOpen}
        onClose={closePreview}
        maxWidth="lg"
        title={(
          <ModalTitle icon={<Camera className="h-5 w-5" />} description={localizeStatic('ui.settings.components.defenseSetupField.description.review.the.captured.defense.before.it.replaces.33146c09')}>
            <LocalizedText messageKey="ui.settings.components.defenseSetupField.current.main.castle.defense.d13f806b" />
          </ModalTitle>
        )}
        footer={(
          <div className="flex w-full flex-wrap items-center justify-end gap-2">
            <Button variant="ghost" onClick={closePreview}><LocalizedText messageKey="game.cancel" /></Button>
            <Button
              disabled={starter.setup == null}
              onClick={() => {
                if (starter.setup) applyInline(starter.setup);
                closePreview();
              }}
            >
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.apply.to.this.setup.d00f0343" />
            </Button>
          </div>
        )}
      >
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            {starterCastleName ? <span className="text-sm font-bold text-text-main">{starterCastleName}</span> : null}
            <Badge variant="outline" className="normal-case tracking-normal">{label}</Badge>
            {starter.pendingReviews.length > 0 ? (
              <Badge variant="warning" className="normal-case tracking-normal"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.starter.values.pending.product.review.9369cca7" /></Badge>
            ) : null}
          </div>
          {value.source !== 'none' ? (
            <p className="rounded-global border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-text-main">
              <LocalizedText messageKey="ui.settings.components.defenseSetupField.applying.replaces.this.defense.s.current.setup.f63ccd88" />
            </p>
          ) : null}
          {starter.setup && starterSummary ? (
            <div className="grid grid-cols-3 gap-2 text-center text-xs">
              {(['left', 'middle', 'right'] as const).map((side) => (
                <div key={side} className="rounded-global border border-border-base bg-bg-app/35 p-2">
                  <div className="text-[10px] font-black uppercase tracking-wider text-text-muted">
                    {side === 'left'
                      ? <LocalizedText messageKey="ui.settings.components.defenseSetupField.left.wall.09cae679" />
                      : side === 'middle'
                        ? <LocalizedText messageKey="ui.settings.components.defenseSetupField.front.wall.8b3e3f72" />
                        : <LocalizedText messageKey="ui.settings.components.defenseSetupField.right.wall.d66eaf17" />}
                  </div>
                  <div className="font-mono tabular-nums text-text-main">{starter.setup?.wall[side].unitPercent}%</div>
                </div>
              ))}
              <div className="col-span-3 text-left text-xs text-text-muted">
                <LocalizedText messageKey="defenseSetup.summary" params={{ tools: starterSummary.toolAmount, types: starterSummary.toolTypes.length }} />
                {starterSummary.toolAmount === 0 ? (
                  <span className="mt-1 block font-semibold text-warning"><LocalizedText messageKey={DEFENSE_NO_TOOLS} /></span>
                ) : null}
              </div>
            </div>
          ) : null}
          <p className="text-[11px] text-text-muted">
            <LocalizedText messageKey="ui.settings.components.defenseSetupField.auto.khan.applies.this.defense.to.the.e4380389" />
          </p>
        </div>
      </Modal>

      <Modal
        isOpen={saveAsOpen}
        onClose={() => { if (!saveAsBusy) closeSaveAs(); }}
        maxWidth="md"
        title={(
          <ModalTitle icon={<Save className="h-5 w-5" />} description={localizeStatic('ui.settings.components.defenseSetupField.description.creates.a.normal.reusable.defense.preset.from.148099f6')}>
            <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.save.as.preset.853c8501" />
          </ModalTitle>
        )}
        footer={(
          <div className="flex w-full items-center justify-end gap-2">
            <Button variant="ghost" disabled={saveAsBusy} onClick={closeSaveAs}><LocalizedText messageKey="game.cancel" /></Button>
            <Button isLoading={saveAsBusy} disabled={saveAsBusy} leftIcon={<Save className="h-4 w-4" />} onClick={() => void submitSaveAs()}>
              {saveAsFailed && !saveAsBusy ? <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.try.again.d8b8392e" /> : <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.save.preset.362a1376" />}
            </Button>
          </div>
        )}
      >
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            void submitSaveAs();
          }}
        >
          <label className="block">
            <span className="mb-1.5 block text-[11px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="common.presetName" /></span>
            <Input
              value={saveAsName}
              ref={saveAsInput}
              maxLength={80}
              aria-invalid={saveAsError ? true : undefined}
              aria-describedby={saveAsError ? `${fieldId}-save-error` : undefined}
              onChange={(event) => {
                setSaveAsName(event.target.value);
                if (saveAsError) setSaveAsError('');
              }}
            />
          </label>
          {saveAsError ? <p id={`${fieldId}-save-error`} role="alert" className="text-xs font-semibold text-error">{saveAsError}</p> : null}
        </form>
      </Modal>
    </div>
  );
};
