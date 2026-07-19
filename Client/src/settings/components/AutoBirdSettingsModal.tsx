import React, { useState, useEffect, useCallback } from 'react';
import { Bird, CalendarDays, Plus } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import type { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import {
  applyPresetToStoredShape,
  emptyPresetsFile,
  parsePresetsPayload,
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
  const { state, configuration } = useCitadelAPI();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<Record<string, { id: number; amount: number }[]>>({});
  const [minDelay, setMinDelay] = useState(6);
  const [maxDelay, setMaxDelay] = useState(12);
  const [minSend, setMinSend] = useState(0);
  const [minRPTDays, setMinRPTDays] = useState(3);
  const [presetsState, setPresetsState] = useState(() => emptyPresetsFile());
  const [presetDropdownId, setPresetDropdownId] = useState('');
  const [appliedPresetId, setAppliedPresetId] = useState<string | null>(null);
  const [presetName, setPresetName] = useState('');
  const [presetError, setPresetError] = useState('');

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
    const ig = state.ignoreSettings;
    setSettings(ig.settings);
    setMinDelay(clampDelayHours(ig.minDelay));
    setMaxDelay(clampDelayHours(ig.maxDelay));
    setMinSend(ig.minSend);
    setMinRPTDays(clampMinRPTDays(ig.minRPTDays));
    setPresetsState(state.presets);
    const last = state.presets.lastSelectedPresetId;
    setPresetDropdownId(last && state.presets.presets.some((p) => p.id === last) ? last : '');
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    applyFullClientState(parseAutoBirdClientState(
      configuration?.sections['automation.autoBird']
        ?? buildAutoBirdClientState(defaultAutoBirdSettings(), emptyPresetsFile()),
    ));

    setAppliedPresetId(null);
    setPresetName('');
    setPresetError('');

  }, [configuration?.sections, isOpen, applyFullClientState]);

  const handleAddItem = async (castleId: string) => {
    const currentItems = settings[castleId] || [];
    const preselectedQuantities: Record<number, number> = {};
    currentItems.forEach((item) => {
      if (item.id) preselectedQuantities[item.id] = item.amount;
    });

    const result = await showTroopPicker({
      mode: 'multi',
      title: `Keep in castle (not sent on bird) — ${castles.find((c) => c.id === parseInt(castleId, 10))?.name ?? castleId}`,
      allowQuantity: true,
      preselected: currentItems.map((i) => i.id),
      preselectedQuantities,
    });

    if (Array.isArray(result)) {
      const newItems = (result as UnitWithQuantity[]).map((u) => ({
        id: u.unitId,
        amount: u.quantity,
      }));
      setSettings((prev) => ({ ...prev, [castleId]: newItems }));
    }
  };

  const handleRemoveItem = (castleId: string, unitId: number) => {
    setSettings((prev) => ({
      ...prev,
      [castleId]: (prev[castleId] || []).filter((i) => i.id !== unitId),
    }));
  };

  const handleApplyPreset = () => {
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

  const handleSaveAsNewPreset = () => {
    setPresetError('');
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
    persistAutoBirdClientState(buildAutoBirdClientState(currentIgnoreSettings(), presetsFile));
    setPresetsState(presetsFile);
    setPresetDropdownId(id);
    setAppliedPresetId(id);
  };

  const handleDeletePreset = () => {
    const id = presetDropdownId;
    if (!id) return;
    if (!window.confirm('Delete this preset? This cannot be undone.')) return;
    const presetsFile = {
      ...presetsState,
      presets: presetsState.presets.filter((p) => p.id !== id),
      lastSelectedPresetId:
        presetsState.lastSelectedPresetId === id ? null : presetsState.lastSelectedPresetId,
    };
    persistAutoBirdClientState(buildAutoBirdClientState(currentIgnoreSettings(), presetsFile));
    setPresetsState(presetsFile);
    setPresetDropdownId('');
    if (appliedPresetId === id) {
      setAppliedPresetId(null);
      hydrateFromConfiguration();
      setPresetName('');
    }
  };

  const handleSave = () => {
    setPresetError('');
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
    persistAutoBirdClientState(buildAutoBirdClientState(payload, presetsFile));
    setPresetsState(presetsFile);

    onClose();
  };

  const presetOptions = [
    { value: '', label: '— Saved configuration —' },
    ...presetsState.presets.map((p) => ({ value: p.id, label: p.name })),
  ];

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="full"
      title="Auto Bird Settings"
      icon={<Bird className="h-5 w-5" />}
      description={(
            <>
              Configure troops to keep (ignore) for each castle when auto-birding. These units will{' '}
              <span className="font-bold text-text-main">not</span> be sent.
            </>
      )}
      titleTrailing={(
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => onOpenFeatureSchedule('autoBird', 'Auto Bird')}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Calendar
            </Button>
      )}
      onSave={handleSave}
      saveLabel="Save changes"
    >
      <div className="auto-bird-settings-workspace flex w-full flex-col gap-6 mx-auto">
        {/* Global settings bar */}
        <Card variant="solid" className="shrink-0 bg-bg-app border-border-base p-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:flex-wrap lg:items-end">
            <div className="flex flex-1 flex-wrap items-end gap-3">
              <span className="mb-1.5 w-full text-xs font-bold uppercase tracking-wider text-primary lg:mb-0 lg:mr-2 lg:w-auto">
                Random delay range (hours)
              </span>
              <div className="flex w-24 flex-col gap-1">
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Min</span>
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
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Max</span>
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
            <div className="flex min-w-[200px] flex-col gap-1 lg:min-w-[200px]">
              <span className="text-xs font-bold uppercase tracking-wider text-primary">Minimum to send</span>
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  min={0}
                  value={minSend}
                  onChange={(e) => setMinSend(parseInt(e.target.value, 10) || 0)}
                  className="font-mono"
                  rightIcon={<span className="text-xs font-medium uppercase text-text-muted">Troops</span>}
                />
              </div>
            </div>
            <div className="flex min-w-[200px] flex-col gap-1 lg:min-w-[200px]">
              <span className="text-xs font-bold uppercase tracking-wider text-primary">Minimum RPT</span>
              <div className="flex items-center gap-2">
                <Input
                  type="number"
                  min={0}
                  max={30}
                  value={minRPTDays}
                  onChange={(e) => setMinRPTDays(clampMinRPTDays(parseInt(e.target.value, 10)))}
                  className="font-mono"
                  rightIcon={<span className="text-xs font-medium uppercase text-text-muted">Days</span>}
                />
              </div>
            </div>
          </div>
          <p className="mt-3 text-xs text-text-muted">
            Birds are sent with a random delay between min and max hours after travel completes. Alliance castles
            are only used as bird targets when the member&apos;s RPT is greater than the minimum days setting (0 =
            any active post).
          </p>
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
          help={(
            <>
            Choose a preset and click <span className="font-semibold text-text-main">Apply</span> to load it into the grid.{' '}
            <span className="font-semibold text-text-main">Save changes</span> writes Auto Bird settings and updates the applied preset
            (including name). Data is stored next to decoration presets (see AutoBird.json).
            </>
          )}
        />

        {/* Castle grid */}
        <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto pr-1">
          {castles.length === 0 && (
            <p className="py-8 text-center text-sm text-text-muted">Loading castles… reopen if this stays empty.</p>
          )}
          <div className="grid grid-cols-1 gap-4 pb-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {castles.map((castle) => {
              const cid = String(castle.id);
              const items = settings[cid] || [];
              return (
                <Card key={castle.id} variant="solid" className="flex flex-col bg-bg-card-hover/40 p-4 shadow-inner">
                  <div className="mb-3 flex flex-wrap items-center gap-2 border-b border-border-base pb-2">
                    <h3 className="text-sm font-bold text-primary">{castle.name}</h3>
                  </div>
                  {items.length === 0 ? (
                    <div className="flex flex-1 flex-col items-center justify-center gap-3 py-6">
                      <p className="text-center text-xs font-medium uppercase tracking-wider text-text-muted">No ignored units</p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleAddItem(cid)}
                        className="border-dashed"
                        leftIcon={<Plus className="w-4 h-4" />}
                      >
                        Add unit
                      </Button>
                    </div>
                  ) : (
                    <div className="flex flex-wrap justify-center gap-4">
                      {items.map((item) => (
                        <QuantityAssetTile
                          key={item.id}
                          visual={<UnitImage unitId={item.id} size={76} showLevel className="rounded-xl" />}
                          quantity={item.amount}
                          onRemove={() => handleRemoveItem(cid, item.id)}
                          removeLabel="Remove unit"
                        />
                      ))}
                      <AddSlot
                        label="Add unit"
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
  );
};
