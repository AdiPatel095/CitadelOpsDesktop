import React, { Suspense, useEffect, useId, useMemo, useRef, useState } from 'react';
import { Copy, Edit3, Save, Sparkles, Swords } from 'lucide-react';
import type { CastleStateV2 } from '../../api/Contracts';
import {
  appCreatedPresetOwner,
  inlineSetupFromPreset,
  inlineSetupTroopCount,
  isOwnedBy,
  summarizeAttackSetupRef,
  type AttackSetupRef,
  type InlineAttackSetup,
} from '../../attackPresets/AppCreatedPresets';
import { attackPresetSelectOptions, appCreatedPresetBadge } from '../../attackPresets/AttackPresetOptionLabel';
import {
  attackPresetReferrers,
  attackPresetSlotDefinition,
  type AttackPresetReference,
} from '../../attackPresets/AttackPresetReferences';
import {
  attackPresetToolLimits,
  type AttackPresetDocument,
} from '../../attackPresets/AttackPresetTypes';
import type { AttackSetupDraft, AttackSetupInventory } from '../../components/AttackSetupModal';
import UnitImage from '../../components/UnitImage';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { ChoiceChipGroup } from '../../components/ui/ChoiceChipGroup';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { ModalTitle } from '../../components/ui/ModalTitle';
import { Select } from '../../components/ui/Select';
import { useMetadata } from '../../context/MetadataContext';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import type { ReadinessCheck } from '../readiness/Readiness';
import type { EventAttackRecommendation } from '../onboarding/EventAttackRecommendation';
import { validateUserPresetName } from '../AppCreatedPresetSave';
import { ReadinessCheckLine } from './ReadinessPanel';
import type { ObservationContext } from '../requirements/observationFreshness';
import { evaluateUnitStock, requestsFromComposition } from '../requirements/unitRequirements';
import { UnitStockList } from './UnitStockList';

const AttackSetupModal = React.lazy(() => import('../../components/AttackSetupModal'));

type FieldMode = 'saved' | 'inline';

export interface EventAttackSetupFieldProps {
  /** DOM id of the field container; readiness fixes scroll and focus here. */
  id?: string;
  label: React.ReactNode;
  section: string;
  slot: string;
  moduleLabel: string;
  slotLabel: string;
  value: AttackSetupRef;
  onChange: (ref: AttackSetupRef) => void;
  document: AttackPresetDocument;
  references: readonly AttackPresetReference[];
  sourceCastle: CastleStateV2 | null;
  /** Session, connection and hosted presence; stationed stock is shown only while current (CIT-15 D1). */
  observation: ObservationContext;
  eventId: number;
  recommendation: EventAttackRecommendation;
  recipePending: readonly string[];
  /** Explicit Save as preset: creates a normal user preset and returns its id. */
  onSaveAsPreset: (setup: InlineAttackSetup, name: string) => Promise<string>;
  readinessChecks?: readonly ReadinessCheck[];
  disabled?: boolean;
  /** Label of the stock shown in the editor (Storm: the Storm castle plus donors). */
  inventoryLabel?: string;
}

function modeFor(ref: AttackSetupRef): FieldMode | null {
  if (ref.source === 'preset') return 'saved';
  if (ref.source === 'inline') return 'inline';
  return null;
}

const LANES = ['L', 'M', 'R'] as const;

/**
 * One attack slot of an event module: reuse a saved preset or configure the
 * composition here. "Configure here" is persisted as this slot's app-created
 * preset when the module is saved; nothing here writes a user or shared preset
 * except the explicit "Save as preset" action.
 */
export const EventAttackSetupField: React.FC<EventAttackSetupFieldProps> = ({
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
  sourceCastle,
  observation,
  eventId,
  recommendation,
  recipePending,
  onSaveAsPreset,
  readinessChecks = [],
  disabled = false,
  inventoryLabel,
}) => {
  const { t: localizeStatic, locale } = useLocale();
  const { troops, tools, getTroop, unitsLoading, unitsError } = useMetadata();
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
  const recommendTrigger = useRef<HTMLButtonElement>(null);
  const saveAsTrigger = useRef<HTMLButtonElement>(null);
  // Dialogs return focus to the control that opened them (after the dialog's own cleanup), or to the field.
  const returnFocus = (trigger: React.RefObject<HTMLButtonElement | null>) => {
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
      const target = trigger.current?.isConnected && !trigger.current.disabled ? trigger.current : fieldRef.current;
      target?.focus();
    }));
  };
  const closePreview = () => {
    setPreviewOpen(false);
    returnFocus(recommendTrigger);
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
    // The dialog focuses its first control on open; move focus to the name field after that.
    let inner = 0;
    const outer = window.requestAnimationFrame(() => {
      inner = window.requestAnimationFrame(() => saveAsInput.current?.focus());
    });
    return () => {
      window.cancelAnimationFrame(outer);
      window.cancelAnimationFrame(inner);
    };
  }, [saveAsOpen]);

  const summary = useMemo(() => summarizeAttackSetupRef(value, document), [document, value]);
  const selectedPreset = value.source === 'preset' && !value.missing
    ? document.presets.find((preset) => preset.id === value.presetId)
    : undefined;
  const pickerPresets = useMemo(
    () => document.presets.filter((preset) => !isOwnedBy(preset, section, slot)),
    [document.presets, section, slot],
  );
  const pickerOptions = useMemo(() => attackPresetSelectOptions(pickerPresets), [pickerPresets]);
  const listFormat = useMemo(() => new Intl.ListFormat(locale, { type: 'conjunction' }), [locale]);
  const describeReferences = (items: readonly AttackPresetReference[]) => listFormat.format(items.map((reference) => (
    `${localizeStatic(reference.moduleLabelKey)} · ${localizeStatic(reference.slotLabelKey)}`
  )));
  const otherReferrers = value.source === 'preset'
    ? attackPresetReferrers(references, value.presetId).filter((reference) => reference.section !== section || reference.slot !== slot)
    : [];
  const owner = selectedPreset ? appCreatedPresetOwner(selectedPreset) : null;
  const ownerDefinition = owner ? attackPresetSlotDefinition(owner.section, owner.slot) : undefined;
  const inlineSetup = value.source === 'inline' ? value.setup : null;
  const stock = useMemo(() => {
    const composition = inlineSetup ?? (value.source === 'preset' && !value.missing
      ? document.presets.find((preset) => preset.id === value.presetId)
      : undefined);
    if (!composition || !sourceCastle) return null;
    return evaluateUnitStock({
      castle: sourceCastle,
      observation,
      requests: requestsFromComposition({ name: '', ...composition }),
      troops,
      tools,
      metadataReady: !unitsLoading && !unitsError,
      useTroopFamilies: composition.useTroopFamilies,
      // Berimond: transfers and the armorer lane refill the camp before launch.
      decidedAtLaunch: section === 'automation.autoBeriWorld' ? 'stock' : 'quantity',
    });
  }, [document.presets, inlineSetup, observation, section, sourceCastle, tools, troops, unitsError, unitsLoading, value]);
  const inlineTroops = inlineSetup ? inlineSetupTroopCount(inlineSetup) : 0;
  const generatedName = summary.name || localizeStatic('attackPresets.appCreatedName', { module: moduleLabel, slot: slotLabel });
  const targetType = inlineSetup?.targetType ?? 'pve';
  // Hall flank bonuses are resolved on the Attack Presets page; here PvP uses the non-legendary base.
  const toolLimits = attackPresetToolLimits(targetType, { legendary: false, pvpFlankBonus: 0 });
  const inventory = useMemo<AttackSetupInventory | undefined>(() => {
    if (!sourceCastle) return undefined;
    const troopStock: Record<number, number> = {};
    const toolStock: Record<number, number> = {};
    for (const [rawId, rawCount] of Object.entries(sourceCastle.units?.stationed ?? {})) {
      const itemId = Number(rawId);
      const count = Math.max(0, Math.trunc(Number(rawCount) || 0));
      if (!Number.isInteger(itemId) || itemId <= 0 || count <= 0) continue;
      if (tools[itemId]) toolStock[itemId] = count;
      else if (troops[itemId]) troopStock[itemId] = count;
    }
    return {
      label: inventoryLabel ?? localizeStatic('eventAttackSetup.inventoryLabel', { castle: sourceCastle.name?.trim() || `#${sourceCastle.id}` }),
      troopStock,
      toolStock,
    };
  }, [inventoryLabel, localizeStatic, sourceCastle, tools, troops]);

  const inlineId = value.source === 'inline' ? value.presetId : '';
  // A recommendation resolved for another castle or event is never applied.
  const recommendedSetup = recommendation.resolvedFor.eventId === eventId
    && recommendation.resolvedFor.sourceCastleId === (sourceCastle?.id ?? 0)
    ? recommendation.setup
    : null;
  const applyInline = (setup: InlineAttackSetup) => {
    onChange({ source: 'inline', presetId: inlineId, setup, missing: false });
    setMode('inline');
  };

  const choosePreset = (presetId: string) => {
    const preset = document.presets.find((candidate) => candidate.id === presetId);
    if (!preset) return;
    const presetOwner = appCreatedPresetOwner(preset);
    onChange(presetOwner
      ? { source: 'preset', presetId, missing: false, appCreatedBy: presetOwner }
      : { source: 'preset', presetId, missing: false });
  };

  const saveComposition = (draft: AttackSetupDraft) => {
    applyInline({
      targetType,
      useTroopFamilies: Boolean(draft.useTroopFamilies),
      waves: draft.waves,
      courtyardSupport: draft.courtyardSupport,
    });
    setEditing(false);
  };

  const openSaveAs = () => {
    setSaveAsName('');
    setSaveAsError('');
    setSaveAsFailed(false);
    setSaveAsOpen(true);
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
  const summaryBadges = summary.summary ? (
    <Badge variant="outline" className="normal-case tracking-normal">
      <LocalizedText
        messageKey="eventAttackSetup.summary"
        params={{ waves: summary.summary.waves, troops: summary.summary.troops, tools: summary.summary.tools }}
      />
    </Badge>
  ) : null;

  return (
    <div id={id} ref={fieldRef} tabIndex={-1} className="min-w-0 space-y-3 rounded-xl border border-border-base bg-bg-app/35 p-3 outline-none focus-visible:ring-2 focus-visible:ring-primary/40">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span id={`${fieldId}-label`} className="flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
          <Swords className="h-3.5 w-3.5" aria-hidden="true" /> {label}
        </span>
        <ChoiceChipGroup<FieldMode>
          size="sm"
          ariaLabel={localizeStatic('ui.settings.components.eventAttackSetupField.aria-label.how.to.set.up.this.attack.e142a7b5')}
          options={[
            { value: 'saved', label: <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.saved.preset.50edcc5b" /> },
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
              ? <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.choose.a.saved.preset.80bee52d" />
              : <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.no.saved.presets.yet.f4a93a38" />}
            ariaLabel={localizeStatic('ui.settings.components.eventAttackSetupField.aria-label.saved.attack.preset.2c6c0932')}
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
              {summaryBadges}
            </div>
          ) : null}
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
              onClick={() => applyInline(inlineSetupFromPreset(selectedPreset))}
            >
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.customize.this.preset.998581b3" />
            </Button>
          ) : null}
          {mode === 'saved' && value.source === 'inline' ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.this.attack.still.uses.the.setup.configured.643b868e" /></p>
          ) : null}
        </div>
      ) : null}

      {showInline ? (
        <div className="space-y-2 border-t border-border-base pt-2">
          {inlineSetup ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold text-text-main">{generatedName}</span>
              {appCreatedPresetBadge()}
              {summaryBadges}
            </div>
          ) : value.source === 'preset' ? (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.this.attack.uses.the.saved.preset.until.af2cf4a8" /></p>
          ) : (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.no.setup.is.configured.here.yet.start.06ce2916" /></p>
          )}
          {inlineSetup && inlineTroops === 0 ? (
            <p role="alert" className="text-xs text-error"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.add.at.least.one.troop.before.saving.798500e7" /></p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" disabled={disabled} leftIcon={<Edit3 className="h-4 w-4" />} onClick={() => setEditing(true)}>
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.edit.composition.cf7f4829" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || recommendedSetup == null}
              ref={recommendTrigger}
              leftIcon={<Sparkles className="h-4 w-4" />}
              onClick={() => setPreviewOpen(true)}
            >
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.use.recommended.starting.setup.51456aa3" />
            </Button>
            {inlineSetup && inlineTroops > 0 ? (
              <Button variant="ghost" size="sm" disabled={disabled} ref={saveAsTrigger} leftIcon={<Save className="h-4 w-4" />} onClick={openSaveAs}>
                <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.save.as.preset.da93e96d" />
              </Button>
            ) : null}
          </div>
          {recommendedSetup == null && recommendation.requirements.length > 0 ? (
            <ul className="space-y-1 text-[11px] text-text-muted">
              {recommendation.requirements.map((requirement) => (
                <li key={requirement.id}><LocalizedText messageKey={requirement.messageKey} /></li>
              ))}
            </ul>
          ) : null}
          {inlineSetup ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.saved.as.a.preset.marked.created.by.a742674a" /></p>
          ) : null}
          {targetType === 'pvp' ? (
            <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.pvp.tool.limits.use.the.non.legendary.c8d16933" /></p>
          ) : null}
        </div>
      ) : null}

      {stock && stock.lines.length > 0 ? (
        <div className="border-t border-border-base pt-2">
          <UnitStockList
            lines={stock.lines}
            note={<LocalizedText messageKey="ui.settings.components.eventAttackSetupField.stationed.stock.in.the.source.castle.the.ff087c1c" />}
          />
        </div>
      ) : null}
      {readinessChecks.length > 0 ? (
        <ul className="space-y-1.5 border-t border-border-base pt-2">
          {readinessChecks.map((check, index) => <ReadinessCheckLine key={`${check.id}:${index}`} check={check} />)}
        </ul>
      ) : null}

      {editing ? (
        <Suspense fallback={null}>
          <AttackSetupModal
            isOpen
            initialDraft={inlineSetup ? { name: generatedName, ...inlineSetup } : { name: generatedName, waves: [], courtyardSupport: { troops: [], tools: [] } }}
            inventory={inventory}
            inventoryPolicy="advisory"
            targetType={targetType}
            toolLimits={toolLimits}
            allowTroopFamilyMode
            nameField="hidden"
            saveLabel={<LocalizedText messageKey="ui.settings.components.eventAttackSetupField.apply.composition.b3b4946b" />}
            onClose={() => setEditing(false)}
            onSave={saveComposition}
          />
        </Suspense>
      ) : null}

      <Modal
        isOpen={previewOpen}
        onClose={closePreview}
        maxWidth="2xl"
        title={(
          <ModalTitle icon={<Sparkles className="h-5 w-5" />} description={localizeStatic('ui.settings.components.eventAttackSetupField.description.review.the.composition.before.it.replaces.anything.50789484')}>
            <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.recommended.starting.setup.cda7931f" />
          </ModalTitle>
        )}
        footer={(
          <div className="flex w-full flex-wrap items-center justify-end gap-2">
            <Button variant="ghost" onClick={closePreview}><LocalizedText messageKey="game.cancel" /></Button>
            <Button
              disabled={recommendedSetup == null}
              onClick={() => {
                if (recommendedSetup) applyInline(recommendedSetup);
                closePreview();
              }}
            >
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.apply.to.this.setup.d00f0343" />
            </Button>
          </div>
        )}
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-bold text-text-main">{sourceCastle?.name?.trim() || (sourceCastle ? `#${sourceCastle.id}` : '')}</span>
            <Badge variant="outline" className="normal-case tracking-normal">{label}</Badge>
            {recipePending.length > 0 ? (
              <Badge variant="warning" className="normal-case tracking-normal"><LocalizedText messageKey="ui.settings.components.eventAttackSetupField.starter.values.pending.product.review.9369cca7" /></Badge>
            ) : null}
          </div>
          {value.source !== 'none' ? (
            <p className="rounded-global border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-text-main">
              <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.applying.replaces.this.attack.s.current.setup.3028457b" />
            </p>
          ) : null}
          {recommendedSetup ? recommendedSetup.waves.map((wave, waveIndex) => (
            <div key={waveIndex} className="grid gap-2 sm:grid-cols-3">
              {LANES.map((laneKey) => (
                <div key={laneKey} className="min-w-0 rounded-global border border-border-base bg-bg-app/35 p-2">
                  <div className="mb-1 text-[10px] font-black uppercase tracking-wider text-text-muted">
                    {laneKey === 'L'
                      ? <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.left.flank.95721338" />
                      : laneKey === 'M'
                        ? <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.center.front.f228c703" />
                        : <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.right.flank.51130f03" />}
                  </div>
                  <ul className="space-y-1">
                    {wave[laneKey].troops.filter((entry) => entry.itemId != null && entry.quantity > 0).map((entry, index) => (
                      <li key={`${entry.itemId}:${index}`} className="flex items-center gap-2 text-xs text-text-main">
                        <UnitImage unitId={entry.itemId ?? 0} size={24} />
                        <span className="min-w-0 flex-1 truncate">{getTroop(entry.itemId ?? 0)?.name ?? `#${entry.itemId}`}</span>
                        <span className="font-mono tabular-nums">{entry.quantity.toLocaleString(locale)}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )) : null}
          <ul className="list-disc space-y-1 pl-5 text-xs text-text-muted">
            {recommendation.notes.map((note) => <li key={note}><LocalizedText messageKey={note} /></li>)}
          </ul>
          <p className="text-[11px] text-text-muted">
            <LocalizedText messageKey="ui.settings.components.eventAttackSetupField.commanders.tools.travel.and.spending.choices.are.011e83cf" />
          </p>
        </div>
      </Modal>

      <Modal
        isOpen={saveAsOpen}
        onClose={() => { if (!saveAsBusy) closeSaveAs(); }}
        maxWidth="md"
        title={(
          <ModalTitle icon={<Save className="h-5 w-5" />} description={localizeStatic('ui.settings.components.eventAttackSetupField.description.creates.a.normal.reusable.attack.preset.from.b8966892')}>
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
