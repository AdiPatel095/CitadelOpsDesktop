import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { CalendarDays, Hammer, Trash2, Plus, Minus } from 'lucide-react';
import {
  showTCIPicker,
  type TCIWithLevelCeiling,
  normalizeLevelRange,
  TCI_LEVEL_MIN,
} from '../../components/TCIPickerModal';
import {
  fetchConstructionItemsCatalog,
  type ConstructionItemCatalogEntry,
  formatEffectUpgradeLine,
  levelRangeLabel,
  formatGroupTiersLine,
} from '../../components/TCICatalogCache';
import {
  applyPresetToStoredShape,
  emptyPresetsFile,
  snapshotFromForm,
  type AutoTCIPreset,
} from '../AutoTCIPresets';
import {
  buildAutoTCIClientState,
  parseAutoTCIClientState,
  persistAutoTCIClientState,
} from '../AutoTCIClientState';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  NamedPresetControls,
  SettingsModal,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';

interface AutoTCISettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

/** `amount` is the level ceiling; optional `minLevel` is the floor (default 1). */
interface AutoTCIItem {
  id: number;
  amount: number;
  minLevel?: number;
}

export const AutoTCISettingsModal: React.FC<AutoTCISettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule }) => {
  const { state, configuration } = useCitadelAPI();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<Record<string, AutoTCIItem[]>>({});
  const [catalog, setCatalog] = useState<ConstructionItemCatalogEntry[]>([]);
  const [presetsState, setPresetsState] = useState(() => emptyPresetsFile());
  const [presetDropdownId, setPresetDropdownId] = useState('');
  const [appliedPresetId, setAppliedPresetId] = useState<string | null>(null);
  const [presetName, setPresetName] = useState('');
  const [presetError, setPresetError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const loadedConfigurationSignature = useRef<string | null>(null);

  const catalogByWireId = useMemo(() => {
    const m = new Map<number, ConstructionItemCatalogEntry>();
    for (const c of catalog) {
      m.set(c.id, c);
      for (const gid of c.groupIds) {
        m.set(gid, c);
      }
    }
    return m;
  }, [catalog]);

  const hydrateFromConfiguration = useCallback(() => {
    setSettings(parseAutoTCIClientState(configuration?.sections['automation.constructionItems']).targets);
  }, [configuration?.sections]);

  const applyFullClientState = useCallback((state: ReturnType<typeof parseAutoTCIClientState>) => {
    setSettings(state.targets);
    setPresetsState(state.presets);
    const last = state.presets.lastSelectedPresetId;
    setPresetDropdownId(last && state.presets.presets.some((p) => p.id === last) ? last : '');
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    fetchConstructionItemsCatalog().then(setCatalog).catch(() => setCatalog([]));
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      loadedConfigurationSignature.current = null;
      setSaveError(null);
      return;
    }
    const rawState = configuration?.sections['automation.constructionItems']
      ?? buildAutoTCIClientState({}, emptyPresetsFile());
    const signature = JSON.stringify(rawState);
    if (loadedConfigurationSignature.current === signature) return;
    loadedConfigurationSignature.current = signature;
    applyFullClientState(parseAutoTCIClientState(rawState));

    setAppliedPresetId(null);
    setPresetName('');
    setPresetError('');

  }, [configuration?.sections, isOpen, applyFullClientState]);

  const handleAddItem = async (castleId: string) => {
    const currentItems = settings[castleId] || [];
    const preselectedLevelCeilings: Record<number, number> = {};
    const preselectedLevelFloors: Record<number, number> = {};
    currentItems.forEach((item) => {
      if (item.id) {
        const meta = catalogByWireId.get(item.id);
        const range = normalizeLevelRange(
          item.minLevel ?? meta?.minLevel ?? TCI_LEVEL_MIN,
          item.amount,
          meta?.minLevel ?? TCI_LEVEL_MIN,
          meta?.maxLevel ?? Number.MAX_SAFE_INTEGER,
        );
        preselectedLevelCeilings[item.id] = range.ceiling;
        preselectedLevelFloors[item.id] = range.floor;
      }
    });

    const result = await showTCIPicker({
      mode: 'multi',
      title: `Select construction items — ${castles.find((c) => c.id === parseInt(castleId, 10))?.name ?? 'Castle'}`,
      preselected: currentItems.map((i) => i.id),
      preselectedLevelCeilings,
      preselectedLevelFloors,
    });

    if (Array.isArray(result)) {
      const newItems: AutoTCIItem[] = (result as TCIWithLevelCeiling[]).map((u) => {
        const range = normalizeLevelRange(u.levelFloor, u.levelCeiling);
        const row: AutoTCIItem = {
          id: u.constructionItemId,
          amount: range.ceiling,
        };
        if (range.floor > 1) {
          row.minLevel = range.floor;
        }
        return row;
      });
      setSettings((prev) => ({
        ...prev,
        [castleId]: newItems,
      }));
    }
  };

  const bumpCeiling = (castleId: string, itemId: number, delta: number) => {
    setSettings((prev) => {
      const list = prev[castleId] || [];
      const next = list.map((it) => {
        if (it.id !== itemId) return it;
        const meta = catalogByWireId.get(itemId);
        const range = normalizeLevelRange(
          it.minLevel ?? meta?.minLevel ?? TCI_LEVEL_MIN,
          it.amount + delta,
          meta?.minLevel ?? TCI_LEVEL_MIN,
          meta?.maxLevel ?? Number.MAX_SAFE_INTEGER,
        );
        const row: AutoTCIItem = { ...it, amount: range.ceiling };
        if (range.floor > 1) row.minLevel = range.floor;
        else delete row.minLevel;
        return row;
      });
      return { ...prev, [castleId]: next };
    });
  };

  const bumpFloor = (castleId: string, itemId: number, delta: number) => {
    setSettings((prev) => {
      const list = prev[castleId] || [];
      const next = list.map((it) => {
        if (it.id !== itemId) return it;
        const meta = catalogByWireId.get(itemId);
        const range = normalizeLevelRange(
          (it.minLevel ?? meta?.minLevel ?? TCI_LEVEL_MIN) + delta,
          it.amount,
          meta?.minLevel ?? TCI_LEVEL_MIN,
          meta?.maxLevel ?? Number.MAX_SAFE_INTEGER,
        );
        const row: AutoTCIItem = { ...it, amount: range.ceiling };
        if (range.floor > 1) row.minLevel = range.floor;
        else delete row.minLevel;
        return row;
      });
      return { ...prev, [castleId]: next };
    });
  };

  const removeItem = (castleId: string, itemId: number) => {
    setSettings((prev) => ({
      ...prev,
      [castleId]: (prev[castleId] || []).filter((it) => it.id !== itemId),
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
    const snap = snapshotFromForm(settings);
    const id = crypto.randomUUID();
    const next: AutoTCIPreset = { id, name, ...snap };
    const presetsFile = {
      ...presetsState,
      presets: [...presetsState.presets, next],
      lastSelectedPresetId: id,
    };
    setIsSaving(true);
    try {
      const snapshot = await persistAutoTCIClientState(buildAutoTCIClientState(settings, presetsFile));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.constructionItems']);
      setPresetsState(presetsFile);
      setPresetDropdownId(id);
      setAppliedPresetId(id);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save the Auto TCI preset.');
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
    setIsSaving(true);
    setSaveError(null);
    try {
      const snapshot = await persistAutoTCIClientState(buildAutoTCIClientState(settings, presetsFile));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.constructionItems']);
      setPresetsState(presetsFile);
      setPresetDropdownId('');
      if (appliedPresetId === id) {
        setAppliedPresetId(null);
        setPresetName('');
      }
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not delete the Auto TCI preset.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleSave = async () => {
    if (isSaving) return;
    setPresetError('');
    setSaveError(null);
    const snap = snapshotFromForm(settings);
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
      const snapshot = await persistAutoTCIClientState(buildAutoTCIClientState(settings, presetsFile));
      loadedConfigurationSignature.current = JSON.stringify(snapshot.sections['automation.constructionItems']);
      setPresetsState(presetsFile);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto TCI settings.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  const presetOptions = [
    { value: '', label: '— Saved configuration —' },
    ...presetsState.presets.map((p) => ({ value: p.id, label: p.name })),
  ];

  const labelFor = (constructionItemId: number) => {
    const row = catalogByWireId.get(constructionItemId);
    if (row) {
      const effectLine = formatEffectUpgradeLine(row);
      const eff = effectLine ? ` — ${effectLine}` : '';
      return `${row.label}${eff} (#${constructionItemId})`;
    }
    return `TCI #${constructionItemId}`;
  };

  const levelStepper = (
    label: string,
    value: number,
    onDec: () => void,
    onInc: () => void,
    decDisabled: boolean,
    incDisabled: boolean,
    decLabel: string,
    incLabel: string,
  ) => (
    <div className="flex flex-col items-center gap-1">
      <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{label}</span>
      <div className="flex items-center gap-1">
        <button
          type="button"
          className="flex h-8 w-8 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
          disabled={decDisabled}
          onClick={onDec}
          aria-label={decLabel}
        >
          <Minus className="h-4 w-4" />
        </button>
        <span className="min-w-[32px] text-center font-mono text-sm font-bold tabular-nums">{value}</span>
        <button
          type="button"
          className="flex h-8 w-8 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
          disabled={incDisabled}
          onClick={onInc}
          aria-label={incLabel}
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
    </div>
  );

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="full"
      title={<span className="text-amber-500">Auto TCI Settings</span>}
      icon={<Hammer className="h-5 w-5 text-amber-500" />}
      description={(
            <>
              Per castle, pick construction item variants and set a <span className="font-medium text-text-main">level floor and ceiling</span>{' '}
              using the level range supplied by the current official construction-item catalog.
            </>
      )}
      titleTrailing={(
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => onOpenFeatureSchedule('autoTCI', 'Auto TCI')}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Calendar
            </Button>
      )}
      onSave={handleSave}
      saveLabel="Save changes"
      isSaving={isSaving}
    >
      <div className="auto-tci-settings-workspace custom-scrollbar mx-auto flex w-full flex-col gap-5 overflow-y-auto pb-4">
        {saveError && (
          <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
            {saveError}
          </div>
        )}
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
            Choose a preset and click <span className="font-semibold text-text-main">Apply</span> to load it into the grid.{' '}
            <span className="font-semibold text-text-main">Save changes</span> writes Auto TCI settings and updates the applied preset
            (including name). Data is stored next to Auto Bird settings (see AutoTCI.json).
            </>
          )}
        />

        <div className="grid w-full auto-rows-max grid-cols-1 gap-5 md:grid-cols-2 2xl:grid-cols-3">
        {castles.map((castle) => {
          const castleId = castle.id.toString();
          const castleItems = settings[castleId] || [];
          const hasItems = castleItems.length > 0;

          return (
            <Card key={castle.id} variant="solid" className="flex min-h-0 shrink-0 flex-col border-border-base bg-bg-app">
              <CardHeader className="flex flex-row items-center justify-between rounded-t-[calc(var(--radius-global)-1px)] border-b border-border-base bg-bg-card-hover px-5 py-3.5">
                <div className="flex items-center gap-3">
                  <CardTitle className="text-lg text-text-main">{castle.name}</CardTitle>
                  {hasItems && (
                    <Badge variant="primary">
                      {castleItems.length} item{castleItems.length !== 1 ? 's' : ''}
                    </Badge>
                  )}
                </div>
              </CardHeader>

              <CardContent className="bg-bg-app/30 p-5">
                {hasItems ? (
                  <div className="w-full">
                    <div className="flex flex-col gap-3">
                      {castleItems.map((item, index) => {
                        const meta = catalogByWireId.get(item.id);
                        const range = normalizeLevelRange(
                          item.minLevel ?? meta?.minLevel ?? TCI_LEVEL_MIN,
                          item.amount,
                          meta?.minLevel ?? TCI_LEVEL_MIN,
                          meta?.maxLevel ?? Number.MAX_SAFE_INTEGER,
                        );
                        const floor = range.floor;
                        const ceil = range.ceiling;
                        const effectLine = meta ? formatEffectUpgradeLine(meta) : '';
                        return (
                          <div
                            key={`${item.id}-${index}`}
                            className="flex flex-col gap-3 rounded-xl border border-border-base bg-bg-card p-4 shadow-sm sm:flex-row sm:items-center"
                            title={labelFor(item.id)}
                          >
                            <div className="min-w-0 flex-1">
                              <div className="text-sm font-bold leading-snug text-text-main" title={meta?.label}>
                                {meta?.label ?? `TCI #${item.id}`}
                              </div>
                              {effectLine && (
                                <div className="mt-1 text-xs leading-relaxed text-text-main/85" title={effectLine}>
                                  {effectLine}
                                </div>
                              )}
                              <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] font-medium uppercase tracking-wide text-text-muted">
                                <span>{meta ? levelRangeLabel(meta) : '—'}</span>
                                {meta?.category ? <span>· {meta.category}</span> : null}
                              </div>
                              <div
                                className="mt-1 font-mono text-[10px] leading-relaxed text-text-muted/90"
                                title={meta ? formatGroupTiersLine(meta) : undefined}
                              >
                                {meta ? formatGroupTiersLine(meta) : `#${item.id}`}
                              </div>
                            </div>
                            <div className="flex shrink-0 flex-wrap items-center justify-end gap-4 sm:gap-5">
                              {levelStepper(
                                'Min',
                                floor,
                                () => bumpFloor(castleId, item.id, -1),
                                () => bumpFloor(castleId, item.id, 1),
                                floor <= (meta?.minLevel ?? TCI_LEVEL_MIN),
                                floor >= ceil,
                                'Decrease level floor',
                                'Increase level floor',
                              )}
                              {levelStepper(
                                'Max',
                                ceil,
                                () => bumpCeiling(castleId, item.id, -1),
                                () => bumpCeiling(castleId, item.id, 1),
                                ceil <= floor,
                                ceil >= (meta?.maxLevel ?? ceil),
                                'Decrease level ceiling',
                                'Increase level ceiling',
                              )}
                              <button
                                type="button"
                                onClick={() => removeItem(castleId, item.id)}
                                className="flex h-8 items-center gap-1.5 rounded-lg px-2 text-xs font-semibold text-error/90 hover:bg-error/10"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                                Remove
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                    <button
                      type="button"
                      onClick={() => handleAddItem(castleId)}
                      className="mt-4 flex w-full items-center justify-center gap-2 rounded-global border-2 border-dashed border-border-base py-3.5 text-sm font-medium text-text-muted transition-colors hover:border-primary hover:bg-primary/5 hover:text-primary"
                    >
                      <Plus className="h-4 w-4" />
                      Add construction item
                    </button>
                  </div>
                ) : (
                  <div className="flex min-h-[10rem] flex-col items-center justify-center py-8">
                    <div className="mb-3 text-center text-xs font-bold uppercase tracking-wider text-text-muted/60">
                      No construction items selected
                    </div>
                    <Button variant="outline" size="sm" onClick={() => handleAddItem(castleId)} leftIcon={<Plus className="h-4 w-4" />}>
                      Add construction item
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          );
        })}
        </div>
      </div>
    </SettingsModal>
  );
};
