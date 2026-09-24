import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Bird, BookOpen, CalendarDays, LockKeyhole, Plus } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import type { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import {
  applyPresetToStoredShape,
  emptyPresetsFile,
  snapshotFromForm,
  type AutoBirdPreset,
} from '../AutoBirdPresets';
import {
  buildAutoBirdClientState,
  defaultAutoBirdSettings,
  parseAutoBirdClientState,
  persistAutoBirdClientState,
  type AutoBirdStoredSettings,
} from '../AutoBirdClientState';
import {
  AddSlot,
  Button,
  Card,
  Input,
  NamedPresetControls,
  QuantityAssetTile,
  SettingsModal,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import { useAuth } from '../../context/AuthContext';
import { AUTO_FORTRESS_DIREWOLF_ID } from '../AutoFortressClientState';
import { AutoBirdGuideModal } from './AutoBirdGuideModal';
import { useGuideLocale } from '../../config/useGuideLocale';
import {
  autoFortressReservesDirewolves,
  mergeAutoBirdPickerItems,
  visibleAutoBirdReserveItems,
} from '../AutoBirdFortressReserve';

interface AutoBirdSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

function clampDelayHours(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.min(12, Math.max(1, value));
}

function clampMinRPTDays(value: number): number {
  if (!Number.isFinite(value)) return 3;
  return Math.min(30, Math.max(0, value));
}

export const AutoBirdSettingsModal: React.FC<AutoBirdSettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule }) => {
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const { t: localizeStatic } = useStaticLocale();
  const { state, configuration } = useCitadelAPI();
  const { autoFortressEnabled } = useAuth();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<Record<string, { id: number; amount: number }[]>>({});
  const [minDelay, setMinDelay] = useState(6);
  const [maxDelay, setMaxDelay] = useState(12);
  const [minSend, setMinSend] = useState(0);
  const [minRPTDays, setMinRPTDays] = useState(3);
  const [presetsState, setPresetsState] = useState(() => emptyPresetsFile());
  const [presetDropdownId, setPresetDropdownId] = useState('');
  const [appliedPresetId, setAppliedPresetId] = useState<string | null>(null);
  const [activePresetId, setActivePresetId] = useState<string | null>(null);
  const [presetName, setPresetName] = useState('');
  const [presetError, setPresetError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  useEffect(() => { if (!isOpen) setIsGuideOpen(false); }, [isOpen]);
  const loadedConfigurationSignature = useRef<string | null>(null);

  const currentIgnoreSettings = useCallback((): AutoBirdStoredSettings => {
    let maxD = clampDelayHours(maxDelay);
    const minD = clampDelayHours(minDelay);
    if (maxD < minD) maxD = minD;
    return {
      settings,
      minDelay: minD,
      maxDelay: maxD,
      minSend: Math.max(0, minSend),
      minRPTDays: clampMinRPTDays(minRPTDays),
    };
  }, [settings, minDelay, maxDelay, minSend, minRPTDays]);

  const hydrateFromConfiguration = useCallback(() => {
    const s = parseAutoBirdClientState(configuration?.sections['automation.autoBird']).ignoreSettings;
    setSettings(s.settings);
    setMinDelay(clampDelayHours(s.minDelay));
    setMaxDelay(clampDelayHours(s.maxDelay));
    setMinSend(s.minSend);
    setMinRPTDays(clampMinRPTDays(s.minRPTDays));
  }, [configuration?.sections]);

  const applyFullClientState = useCallback((state: ReturnType<typeof parseAutoBirdClientState>) => {
    const activePreset = state.activePresetId
      ? state.presets.presets.find((preset) => preset.id === state.activePresetId)
      : undefined;
    const ig = activePreset ? applyPresetToStoredShape(activePreset) : state.ignoreSettings;
    setSettings(ig.settings);
    setMinDelay(clampDelayHours(ig.minDelay));
    setMaxDelay(clampDelayHours(ig.maxDelay));
    setMinSend(ig.minSend);
    setMinRPTDays(clampMinRPTDays(ig.minRPTDays));
    setPresetsState(state.presets);
    setActivePresetId(state.activePresetId);
    setAppliedPresetId(activePreset?.id ?? null);
    setPresetName(activePreset?.name ?? '');
    const last = activePreset?.id ?? state.presets.lastSelectedPresetId;
    setPresetDropdownId(last && state.presets.presets.some((p) => p.id === last) ? last : '');
  }, []);

  useEffect(() => {
    if (!isOpen) {
      loadedConfigurationSignature.current = null;
      setSaveError(null);
      return;
    }
    const rawState = configuration?.sections['automation.autoBird']
      ?? buildAutoBirdClientState(defaultAutoBirdSettings(), emptyPresetsFile());
    const signature = JSON.stringify(rawState);
    if (loadedConfigurationSignature.current === signature) return;
    loadedConfigurationSignature.current = signature;
    applyFullClientState(parseAutoBirdClientState(rawState));
    setPresetError('');

  }, [configuration?.sections, isOpen, applyFullClientState]);

  const handleAddItem = async (castleId: string) => {
    const currentItems = settings[castleId] || [];
    const castleState = state?.castles[castleId];
    const fortressProtected = autoFortressReservesDirewolves(
      autoFortressEnabled,
      castleState,
      configuration?.sections['automation.autoFortress'],
    );
    const editableItems = visibleAutoBirdReserveItems(currentItems, fortressProtected);
    const preselectedQuantities: Record<number, number> = {};
    editableItems.forEach((item) => {
      if (item.id) preselectedQuantities[item.id] = item.amount;
    });

    const result = await showTroopPicker({
      mode: 'multi',
      title: `Keep in castle (not sent on bird) — ${castles.find((c) => c.id === parseInt(castleId, 10))?.name ?? castleId}`,
      allowQuantity: true,
      preselected: editableItems.map((i) => i.id),
      preselectedQuantities,
      excludedUnitIds: fortressProtected ? [AUTO_FORTRESS_DIREWOLF_ID] : undefined,
    });

    if (Array.isArray(result)) {
      const newItems = (result as UnitWithQuantity[]).map((u) => ({
        id: u.unitId,
        amount: u.quantity,
      }));
      setSettings((prev) => ({
        ...prev,
        [castleId]: mergeAutoBirdPickerItems(prev[castleId] || [], newItems, fortressProtected),
      }));
    }
  };

  const handleRemoveItem = (castleId: string, unitId: number) => {
    setSettings((prev) => ({
      ...prev,
      [castleId]: (prev[castleId] || []).filter((i) => i.id !== unitId),
    }));
  };

  const handleApplyPreset = () => {
    if (isSaving) return;
    setPresetError('');
    if (!presetDropdownId) {
      hydrateFromConfiguration();
      setAppliedPresetId(null);
      setPresetName('');
      return;
    }
    const preset = presetsState.presets.find((p) => p.id === presetDropdownId);
    if (!preset) return;
    const applied = applyPresetToStoredShape(preset);
    setSettings(applied.settings);
    setMinDelay(clampDelayHours(applied.minDelay));
    setMaxDelay(clampDelayHours(applied.maxDelay));
    setMinSend(applied.minSend);
    setMinRPTDays(clampMinRPTDays(applied.minRPTDays));
    setAppliedPresetId(preset.id);
    setPresetName(preset.name);
  };

  const handleSaveAsNewPreset = async () => {
    if (isSaving) return;
    setPresetError('');
    setSaveError(null);
    const name = presetName.trim();
    if (!name) {
      setPresetError('Enter a preset name first.');
      return;
    }
    const snap = snapshotFromForm(settings, minDelay, maxDelay, minSend, minRPTDays);
    const id = crypto.randomUUID();
    const next: AutoBirdPreset = { id, name, ...snap };
    const presetsFile = {
      ...presetsState,
      presets: [...presetsState.presets, next],
      lastSelectedPresetId: id,
    };
    setIsSaving(true);
    try {
      const snapshot = await persistAutoBirdClientState(buildAutoBirdClientState(currentIgnoreSettings(), presetsFile, id));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.autoBird']);
      setPresetsState(presetsFile);
      setPresetDropdownId(id);
      setAppliedPresetId(id);
      setActivePresetId(id);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save the Auto Bird preset.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeletePreset = async () => {
    if (isSaving) return;
    const id = presetDropdownId;
    if (!id) return;
    if (!window.confirm('Delete this preset? This cannot be undone.')) return;
    const presetsFile = {
      ...presetsState,
      presets: presetsState.presets.filter((p) => p.id !== id),
      lastSelectedPresetId:
        presetsState.lastSelectedPresetId === id ? null : presetsState.lastSelectedPresetId,
    };
    const nextActivePresetId = activePresetId === id ? null : activePresetId;
    setIsSaving(true);
    setSaveError(null);
    try {
      const snapshot = await persistAutoBirdClientState(buildAutoBirdClientState(
        currentIgnoreSettings(),
        presetsFile,
        nextActivePresetId,
      ));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.autoBird']);
      setPresetsState(presetsFile);
      setActivePresetId(nextActivePresetId);
      setPresetDropdownId('');
      if (appliedPresetId === id) {
        setAppliedPresetId(null);
        setPresetName('');
      }
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not delete the Auto Bird preset.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleSave = async () => {
    if (isSaving) return;
    setPresetError('');
    setSaveError(null);
    const payload = currentIgnoreSettings();

    const snap = snapshotFromForm(payload.settings, payload.minDelay, payload.maxDelay, payload.minSend, payload.minRPTDays);
    const nameTrim = presetName.trim();
    const updatedPresets = appliedPresetId
      ? presetsState.presets.map((p) =>
          p.id === appliedPresetId
            ? { ...p, ...snap, name: nameTrim || p.name, id: p.id }
            : p
        )
      : presetsState.presets;
    const presetsFile = {
      version: 1 as const,
      lastSelectedPresetId: appliedPresetId,
      presets: updatedPresets,
    };
    setIsSaving(true);
    try {
      const snapshot = await persistAutoBirdClientState(buildAutoBirdClientState(payload, presetsFile, appliedPresetId));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.autoBird']);
      setPresetsState(presetsFile);
      setActivePresetId(appliedPresetId);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Bird settings.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  const presetOptions = [
    { value: '', label: activePresetId ? '— Saved configuration —' : '— Saved configuration (runtime default) —' },
    ...presetsState.presets.map((p) => ({
      value: p.id,
      label: p.id === activePresetId ? `${p.name} (runtime default)` : p.name,
    })),
  ];
  const activePresetMissing = !!activePresetId &&
    !presetsState.presets.some((preset) => preset.id === activePresetId);

  return (
    <>
    <SettingsModal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="full"
      title={localizeStatic("ui.settings.components.autoBirdSettingsModal.title.auto.bird.settings.158a0a4f")}
      icon={<Bird className="h-5 w-5" />}
      description={(
            <>
              Configure runtime-selectable presets of troops to keep in each castle. These units will{' '}
              <span className="font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.not.254bb97b" /></span> be sent.
            </>
      )}
      titleTrailing={(
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={guideLocale}>{guidePack.ui.guideButton}</span></Button>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => onOpenFeatureSchedule('autoBird', 'Auto Bird')}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              <LocalizedText messageKey="common.calendar" />
            </Button>
        </div>
      )}
      onSave={handleSave}
      saveLabel="Save changes"
      isSaving={isSaving}
    >
      <div className="auto-bird-settings-workspace flex w-full flex-col gap-6 mx-auto">
        {saveError && (
          <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
            {saveError}
          </div>
        )}
        {activePresetMissing && (
          <div className="rounded-global border border-warning/30 bg-warning/10 px-4 py-3 text-sm font-semibold text-warning" role="alert">
            <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.the.selected.runtime.preset.no.longer.exists.f845b7fe" /></div>
        )}
        {/* Global settings bar */}
        <Card variant="solid" className="shrink-0 bg-bg-app border-border-base p-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:flex-wrap lg:items-end">
            <div className="flex flex-1 flex-wrap items-end gap-3">
              <span className="mb-1.5 w-full text-xs font-bold uppercase tracking-wider text-primary lg:mb-0 lg:mr-2 lg:w-auto">
                <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.random.delay.range.hours.91f88ef4" /></span>
              <div className="flex w-24 flex-col gap-1">
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.min.dea79332" /></span>
                <Input
                  type="number"
                  min={1}
                  max={12}
                  value={minDelay}
                  onChange={(e) => setMinDelay(clampDelayHours(parseInt(e.target.value, 10)))}
                  className="text-center font-mono"
                />
              </div>
              <div className="flex w-24 flex-col gap-1">
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.max.a1a5936d" /></span>
                <Input
                  type="number"
                  min={1}
                  max={12}
                  value={maxDelay}
                  onChange={(e) => setMaxDelay(clampDelayHours(parseInt(e.target.value, 10)))}
                  className="text-center font-mono"
                />
              </div>
            </div>
            <div className="flex min-w-0 flex-1 basis-full flex-col gap-1 md:basis-52 lg:min-w-[200px]">
              <span className="text-xs font-bold uppercase tracking-wider text-primary"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.minimum.to.send.634ab4a5" /></span>
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  min={0}
                  value={minSend}
                  onChange={(e) => setMinSend(parseInt(e.target.value, 10) || 0)}
                  className="font-mono"
                  rightIcon={<span className="text-xs font-medium uppercase text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.troops.5d47e163" /></span>}
                />
              </div>
            </div>
            <div className="flex min-w-0 flex-1 basis-full flex-col gap-1 md:basis-52 lg:min-w-[200px]">
              <span className="text-xs font-bold uppercase tracking-wider text-primary"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.minimum.rpt.4e22018c" /></span>
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  min={0}
                  max={30}
                  value={minRPTDays}
                  onChange={(e) => setMinRPTDays(clampMinRPTDays(parseInt(e.target.value, 10)))}
                  className="font-mono"
                  rightIcon={<span className="text-xs font-medium uppercase text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.days.e08c0aa8" /></span>}
                />
              </div>
            </div>
          </div>
          <p className="mt-3 text-xs text-text-muted">
            <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.birds.are.sent.with.a.random.delay.64f21ceb" /></p>
        </Card>

        {/* Presets */}
        <NamedPresetControls
          name={presetName}
          onNameChange={(value) => {
            setPresetName(value);
            setPresetError('');
          }}
          nameError={presetError}
          selectedID={presetDropdownId}
          onSelectedIDChange={setPresetDropdownId}
          options={presetOptions}
          onApply={handleApplyPreset}
          onSaveAsNew={handleSaveAsNewPreset}
          onDelete={handleDeletePreset}
          disabled={isSaving}
          help={(
            <>
            Choose a preset and click <span className="font-semibold text-text-main"><LocalizedText messageKey="common.apply" /></span> to load it into this draft.{' '}
            <span className="font-semibold text-text-main"><LocalizedText messageKey="common.saveChanges" /></span> persists that selection and updates the applied preset
            (including its name). Another feature can switch the runtime default by preset ID, while Calendar periods can override it.
            </>
          )}
        />

        {/* Castle grid */}
        <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto pr-1">
          {castles.length === 0 && (
            <p className="py-8 text-center text-sm text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.loading.castles.reopen.if.this.stays.empty.fea1a1d6" /></p>
          )}
          <div className="grid grid-cols-1 gap-4 pb-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {castles.map((castle) => {
              const cid = String(castle.id);
              const items = settings[cid] || [];
              const fortressProtected = autoFortressReservesDirewolves(
                autoFortressEnabled,
                state?.castles[cid],
                configuration?.sections['automation.autoFortress'],
              );
              const visibleItems = visibleAutoBirdReserveItems(items, fortressProtected);
              return (
                <Card key={castle.id} variant="solid" className="flex flex-col bg-bg-card-hover/40 p-4 shadow-inner">
                  <div className="mb-3 flex flex-wrap items-center gap-2 border-b border-border-base pb-2">
                    <h3 className="text-sm font-bold text-primary">{castle.name}</h3>
                  </div>
                  {visibleItems.length === 0 && !fortressProtected ? (
                    <div className="flex flex-1 flex-col items-center justify-center gap-3 py-6">
                      <p className="text-center text-xs font-medium uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.no.ignored.units.ab6d717f" /></p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleAddItem(cid)}
                        className="border-dashed"
                        leftIcon={<Plus className="w-4 h-4" />}
                      >
                        <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.add.unit.bc9e56f0" /></Button>
                    </div>
                  ) : (
                    <div className="flex flex-wrap justify-center gap-4">
                      {visibleItems.map((item) => (
                        <QuantityAssetTile
                          key={item.id}
                          visual={<UnitImage unitId={item.id} size={76} showLevel className="rounded-xl" />}
                          quantity={item.amount}
                          onRemove={() => handleRemoveItem(cid, item.id)}
                          removeLabel="Remove unit"
                        />
                      ))}
                      {fortressProtected && (
                        <div className="relative flex w-[84px] shrink-0 flex-col items-center" aria-label={localizeStatic("ui.settings.components.autoBirdSettingsModal.aria-label.all.direwolves.reserved.by.auto.fortress.c1d2bfc5")}>
                          <div className="relative h-[76px] w-[76px]">
                            <UnitImage unitId={AUTO_FORTRESS_DIREWOLF_ID} size={76} showLevel className="rounded-xl opacity-80" />
                            <span className="absolute left-1 top-1 z-10 flex h-6 w-6 items-center justify-center rounded-full bg-primary text-white shadow-md" title={localizeStatic("ui.settings.components.autoBirdSettingsModal.title.reserved.by.auto.fortress.fad9df89")}>
                              <LockKeyhole className="h-3.5 w-3.5" />
                            </span>
                            <span className="absolute bottom-0 right-0 z-10 translate-x-1/4 translate-y-1/4 rounded-full bg-white px-2.5 py-0.5 text-center text-[10px] font-bold text-slate-900 shadow-md ring-1 ring-black/10">
                              <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.all.a52ace42" /></span>
                          </div>
                          <span className="mt-2 text-center text-[10px] font-semibold leading-tight text-text-muted">
                            <LocalizedText messageKey="ui.settings.components.autoBirdSettingsModal.reserved.by.auto.fortress.fad9df89" /></span>
                        </div>
                      )}
                      <AddSlot
                        label={localizeStatic("ui.settings.components.autoBirdSettingsModal.label.add.unit.bc9e56f0")}
                        layout="icon"
                        onClick={() => handleAddItem(cid)}
                        className="h-[76px] w-[76px] shrink-0"
                        icon={<Plus className="h-8 w-8" strokeWidth={1.5} />}
                      />
                    </div>
                  )}
                </Card>
              );
            })}
          </div>
        </div>
      </div>
    </SettingsModal>
    <AutoBirdGuideModal isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
