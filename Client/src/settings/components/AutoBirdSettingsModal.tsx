import React, { useState, useEffect, useCallback } from 'react';
import { X, Save, Plus, Trash2 } from 'lucide-react';
import { FrontendWebsocket } from '../../websocket';
import { showTroopPicker } from '../../components/TroopPickerModal';
import type { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import {
  applyPresetToStoredShape,
  loadPresetsFile,
  savePresetsFile,
  snapshotFromForm,
  type AutoBirdPreset,
} from '../autobirdPresets';
import { Modal, Button, Input, Select, Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

const STORAGE_KEY = 'autobirdSettings';

export interface AutoBirdStoredSettings {
  settings: Record<string, { id: number; amount: number }[]>;
  minDelay: number;
  maxDelay: number;
  minSend: number;
}

const defaultStored = (): AutoBirdStoredSettings => ({
  settings: {},
  minDelay: 6,
  maxDelay: 12,
  minSend: 0,
});

function clampDelayHours(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.min(12, Math.max(1, value));
}

export function loadAutoBirdSettingsFromStorage(): AutoBirdStoredSettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return defaultStored();
    const parsed = JSON.parse(raw) as Partial<AutoBirdStoredSettings>;
    return {
      ...defaultStored(),
      ...parsed,
      settings: parsed.settings && typeof parsed.settings === 'object' ? parsed.settings : {},
    };
  } catch {
    return defaultStored();
  }
}

interface Castle {
  id: number;
  name: string;
  type: string;
}

interface AutoBirdSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

function AutoBirdTroopTile({
  unitId,
  amount,
  onRemove,
}: {
  unitId: number;
  amount: number;
  onRemove: () => void;
}) {
  return (
    <div className="group relative flex w-[84px] flex-col items-center">
      <button
        type="button"
        onClick={onRemove}
        className="absolute -right-1 -top-1 z-20 flex h-5 w-5 items-center justify-center rounded-full bg-error text-[10px] font-bold text-white opacity-0 shadow-md transition-opacity hover:brightness-110 group-hover:opacity-100"
        aria-label="Remove unit"
      >
        <X className="h-3 w-3" />
      </button>
      <div className="relative h-[76px] w-[76px] shrink-0">
        <UnitImage unitId={unitId} size={76} showLevel={true} className="rounded-xl" />
        <span className="absolute bottom-0 right-0 z-10 max-w-[calc(100%+8px)] translate-x-1/4 translate-y-1/4 truncate rounded-full bg-white px-2.5 py-0.5 text-center text-[10px] font-bold tabular-nums text-slate-900 shadow-md ring-1 ring-black/10">
          {amount.toLocaleString()}
        </span>
      </div>
    </div>
  );
}

export const AutoBirdSettingsModal: React.FC<AutoBirdSettingsModalProps> = ({ isOpen, onClose }) => {
  const [castles, setCastles] = useState<Castle[]>([]);
  const [settings, setSettings] = useState<Record<string, { id: number; amount: number }[]>>({});
  const [minDelay, setMinDelay] = useState(6);
  const [maxDelay, setMaxDelay] = useState(12);
  const [minSend, setMinSend] = useState(0);
  const [presetsState, setPresetsState] = useState(() => loadPresetsFile());
  const [presetDropdownId, setPresetDropdownId] = useState('');
  const [appliedPresetId, setAppliedPresetId] = useState<string | null>(null);
  const [presetName, setPresetName] = useState('');
  const [presetError, setPresetError] = useState('');

  const hydrateFromStorage = useCallback(() => {
    const s = loadAutoBirdSettingsFromStorage();
    setSettings(s.settings);
    setMinDelay(clampDelayHours(s.minDelay));
    setMaxDelay(clampDelayHours(s.maxDelay));
    setMinSend(s.minSend);
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    FrontendWebsocket.sendMessage({ type: 'getCastleList' });
    hydrateFromStorage();
    const file = loadPresetsFile();
    setPresetsState(file);
    const last = file.lastSelectedPresetId;
    setPresetDropdownId(last && file.presets.some((p) => p.id === last) ? last : '');
    setAppliedPresetId(null);
    setPresetName('');
    setPresetError('');
  }, [isOpen, hydrateFromStorage]);

  useEffect(() => {
    const handleMessage = (msg: any) => {
      if (msg.type === 'castleList') {
        const list = msg.payload as Castle[];
        if (list && list.length > 0) {
          setCastles(list);
        }
      }
    };
    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

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
      hydrateFromStorage();
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
    const snap = snapshotFromForm(settings, minDelay, maxDelay, minSend);
    const id = crypto.randomUUID();
    const next: AutoBirdPreset = { id, name, ...snap };
    const file = {
      ...presetsState,
      presets: [...presetsState.presets, next],
      lastSelectedPresetId: id,
    };
    savePresetsFile(file);
    setPresetsState(file);
    setPresetDropdownId(id);
    setAppliedPresetId(id);
  };

  const handleDeletePreset = () => {
    const id = presetDropdownId;
    if (!id) return;
    if (!window.confirm('Delete this preset? This cannot be undone.')) return;
    const file = {
      ...presetsState,
      presets: presetsState.presets.filter((p) => p.id !== id),
      lastSelectedPresetId:
        presetsState.lastSelectedPresetId === id ? null : presetsState.lastSelectedPresetId,
    };
    savePresetsFile(file);
    setPresetsState(file);
    setPresetDropdownId('');
    if (appliedPresetId === id) {
      setAppliedPresetId(null);
      hydrateFromStorage();
      setPresetName('');
    }
  };

  const handleSave = () => {
    setPresetError('');
    const payload: AutoBirdStoredSettings = {
      settings,
      minDelay: clampDelayHours(minDelay),
      maxDelay: clampDelayHours(maxDelay),
      minSend: Math.max(0, minSend),
    };
    if (payload.maxDelay < payload.minDelay) {
      payload.maxDelay = payload.minDelay;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));

    const snap = snapshotFromForm(payload.settings, payload.minDelay, payload.maxDelay, payload.minSend);
    const nameTrim = presetName.trim();
    const updatedPresets = appliedPresetId
      ? presetsState.presets.map((p) =>
          p.id === appliedPresetId
            ? { ...p, ...snap, name: nameTrim || p.name, id: p.id }
            : p
        )
      : presetsState.presets;
    const file = {
      version: 1 as const,
      lastSelectedPresetId: appliedPresetId,
      presets: updatedPresets,
    };
    savePresetsFile(file);
    setPresetsState(file);

    onClose();
  };

  const presetOptions = [
    { value: '', label: '— Saved file on disk (default) —' },
    ...presetsState.presets.map(p => ({ value: p.id, label: p.name }))
  ];

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="full"
      title={
        <div className="flex flex-col">
          <span className="text-primary">Auto Bird Settings</span>
          <p className="mt-1 text-sm text-text-muted font-normal">
            Configure troops to keep (ignore) for each castle when auto-birding. These units will{' '}
            <span className="font-bold text-text-main">not</span> be sent.
          </p>
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose} className="px-6">Cancel</Button>
          <Button variant="primary" onClick={handleSave} className="px-8" leftIcon={<Save className="w-4 h-4" />}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-6 max-w-6xl mx-auto w-full h-[calc(100vh-14rem)]">
        {/* Global settings bar */}
        <Card variant="solid" className="shrink-0 bg-bg-app border-border-base p-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:flex-wrap lg:items-end">
            <div className="flex flex-1 flex-wrap items-end gap-3">
              <span className="w-full text-xs font-bold uppercase tracking-wider text-primary lg:w-auto lg:mr-2 mb-1.5 lg:mb-0">
                Random delay range (hours)
              </span>
              <div className="flex flex-col gap-1 w-24">
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Min</span>
                <Input
                  type="number"
                  min={1}
                  max={12}
                  value={minDelay}
                  onChange={(e) => setMinDelay(clampDelayHours(parseInt(e.target.value, 10)))}
                  className="font-mono text-center"
                />
              </div>
              <div className="flex flex-col gap-1 w-24">
                <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Max</span>
                <Input
                  type="number"
                  min={1}
                  max={12}
                  value={maxDelay}
                  onChange={(e) => setMaxDelay(clampDelayHours(parseInt(e.target.value, 10)))}
                  className="font-mono text-center"
                />
              </div>
            </div>
            <div className="flex flex-col gap-1 lg:min-w-[200px]">
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
          </div>
          <p className="mt-3 text-xs text-text-muted">
            Birds are sent with a random delay between min and max hours after travel completes.
          </p>
        </Card>

        {/* Presets */}
        <Card variant="solid" className="shrink-0 bg-bg-app border-border-base p-4">
          <div className="mb-3 text-xs font-bold uppercase tracking-wider text-primary">Presets</div>
          <div className="flex flex-col gap-4 lg:flex-row lg:flex-wrap lg:items-end">
            <div className="flex min-w-[200px] flex-1 flex-col gap-1.5">
              <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Preset name</span>
              <Input
                type="text"
                placeholder="Name for new preset or rename on save"
                value={presetName}
                onChange={(e) => {
                  setPresetName(e.target.value);
                  setPresetError('');
                }}
                error={presetError}
              />
            </div>
            <div className="flex min-w-[280px] flex-1 flex-col gap-1.5">
              <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Load preset</span>
              <div className="flex gap-2">
                <div className="flex-1">
                  <Select
                    value={presetDropdownId}
                    onChange={(v) => setPresetDropdownId(v)}
                    options={presetOptions}
                  />
                </div>
                <Button variant="outline" onClick={handleApplyPreset} className="shrink-0 bg-bg-card">
                  Apply
                </Button>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                onClick={handleSaveAsNewPreset}
                className="text-info border-info/40 hover:bg-info/10"
                leftIcon={<Plus className="w-4 h-4" />}
              >
                Save as new
              </Button>
              <Button
                variant="danger"
                disabled={!presetDropdownId}
                onClick={handleDeletePreset}
                leftIcon={<Trash2 className="w-4 h-4" />}
              >
                Delete
              </Button>
            </div>
          </div>
          <p className="mt-3 text-xs text-text-muted">
            Choose a preset and click <span className="font-semibold text-text-main">Apply</span> to load it into the grid.{' '}
            <span className="font-semibold text-text-main">Save changes</span> writes Auto Bird settings and updates the applied preset
            (including name).
          </p>
        </Card>

        {/* Castle grid */}
        <div className="min-h-0 flex-1 overflow-y-auto pr-1 custom-scrollbar">
          {castles.length === 0 && (
            <p className="text-sm text-text-muted text-center py-8">Loading castles… reopen if this stays empty.</p>
          )}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 pb-4">
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
                      <p className="text-center text-xs text-text-muted font-medium uppercase tracking-wider">No ignored units</p>
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
                        <AutoBirdTroopTile
                          key={item.id}
                          unitId={item.id}
                          amount={item.amount}
                          onRemove={() => handleRemoveItem(cid, item.id)}
                        />
                      ))}
                      <button
                        type="button"
                        onClick={() => handleAddItem(cid)}
                        className="flex h-[76px] w-[76px] shrink-0 items-center justify-center rounded-global border-2 border-dashed border-border-base text-text-muted transition-colors hover:border-primary/50 hover:text-primary hover:bg-primary/5"
                        aria-label="Add unit"
                      >
                        <Plus className="h-8 w-8" strokeWidth={1.5} />
                      </button>
                    </div>
                  )}
                </Card>
              );
            })}
          </div>
        </div>
      </div>
    </Modal>
  );
};
