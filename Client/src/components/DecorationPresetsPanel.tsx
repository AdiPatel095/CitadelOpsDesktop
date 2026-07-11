import React, { useEffect, useMemo, useState } from 'react';
import { Castle, Layers, Play, Save, Sparkles, Trash2 } from 'lucide-react';
import { useCastleFocus } from '../context/CastleFocusContext';
import CastleFocusHoverPopover from './CastleFocusHoverPopover';
import { castleDisplayName } from '../api/Selectors';
import { Input, Button, Select } from './ui';
import { useCitadelAPI } from '../api/ApiContext';
import { useMetadata } from '../context/MetadataContext';

interface NamedPreset {
  id: string;
  name: string;
  items: { wid: number; x: number; y: number; r: number; layer: string }[];
}

interface DecorationPresetDocument {
  version: 1;
  castles: Record<string, NamedPreset[]>;
}

const EMPTY_PRESETS: NamedPreset[] = [];

const DecorationPresetsPanel: React.FC = () => {
  const { castle } = useCastleFocus();
  const { configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const { decorations } = useMetadata();
  const [newName, setNewName] = useState('');
  const [selectedPresetId, setSelectedPresetId] = useState('');
  const [operationError, setOperationError] = useState('');

  const castleId = castle?.id && castle.id > 0 ? castle.id : 0;
  const focusLabel = castleDisplayName(castle);
  const presetDocument = useMemo(
    () => parsePresetDocument(configuration?.sections['decorations.presets']),
    [configuration?.sections],
  );
  const presets = presetDocument.castles[String(castleId)] ?? EMPTY_PRESETS;

  useEffect(() => {
    setSelectedPresetId((previous) => (
      previous && presets.some((preset) => preset.id === previous) ? previous : presets[0]?.id ?? ''
    ));
    setOperationError('');
  }, [castleId, presets]);

  const handleSave = () => {
    if (!newName.trim() || castleId <= 0) return;
    const items = Object.values(castle?.buildings ?? {})
      .filter((building) => decorations[building.definitionId] != null)
      .map((building) => ({
        wid: building.definitionId,
        x: building.gridX ?? 0,
        y: building.gridY ?? 0,
        r: building.rotation ?? 0,
        layer: 'BG',
      }));
    const preset: NamedPreset = {
      id: crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`,
      name: newName.trim(),
      items,
    };
    setOperationError('');
    void updateConfiguration('decorations.presets', {
      ...presetDocument,
      castles: { ...presetDocument.castles, [String(castleId)]: [...presets, preset] },
    }).catch((error) => setOperationError(error instanceof Error ? error.message : 'Could not save preset'));
    setNewName('');
  };

  const handleApply = (presetId: string) => {
    if (castleId <= 0) return;
    const preset = presets.find((candidate) => candidate.id === presetId);
    if (!preset) return;
    setOperationError('');
    void submitIntent('decoration.apply_preset', {
      castleId,
      kingdomId: castle?.kingdomId,
      presetId,
      items: preset.items,
    }).catch((error) => setOperationError(error instanceof Error ? error.message : 'Could not apply preset'));
  };

  const handleDelete = (presetId: string) => {
    if (castleId <= 0) return;
    if (!window.confirm('Delete this decoration preset?')) return;
    setOperationError('');
    void updateConfiguration('decorations.presets', {
      ...presetDocument,
      castles: {
        ...presetDocument.castles,
        [String(castleId)]: presets.filter((preset) => preset.id !== presetId),
      },
    }).catch((error) => setOperationError(error instanceof Error ? error.message : 'Could not delete preset'));
  };

  const selectedPreset = presets.find((preset) => preset.id === selectedPresetId) ?? null;
  const presetCountLabel = `${presets.length.toLocaleString()} ${presets.length === 1 ? 'preset' : 'presets'}`;
  const selectedPlacementCount = selectedPreset?.items?.length ?? 0;
  const selectedPlacementLabel = selectedPreset
    ? `${selectedPlacementCount.toLocaleString()} ${
        selectedPlacementCount === 1 ? 'decoration placement' : 'decoration placements'
      }`
    : 'No preset selected';
  const canUseCastle = castleId > 0;
  const canSave = canUseCastle && Boolean(newName.trim());
  const hasPresets = presets.length > 0;

  return (
    <div className="flex h-full min-h-0 flex-col gap-5">
      <div className="grid grid-cols-1 gap-x-5 gap-y-3 xl:grid-cols-[minmax(0,1fr)_auto]">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-[10px] font-bold uppercase text-text-muted">
            <Castle className="h-3.5 w-3.5" strokeWidth={2.25} />
            Focused castle
          </div>
          <div className="mt-1.5 flex min-h-[1.75rem] items-center">
            <CastleFocusHoverPopover
              castle={castle}
              align="start"
              expandToViewport
              className="min-w-0 max-w-full"
            >
              <span className="cursor-help truncate border-b border-dotted border-text-muted/50 font-semibold text-text-main">
                {focusLabel}
              </span>
            </CastleFocusHoverPopover>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-x-5 gap-y-3 sm:grid-cols-[minmax(8rem,10rem)_minmax(10rem,14rem)] xl:min-w-[22rem]">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-bold uppercase text-text-muted">
              <Layers className="h-3.5 w-3.5" strokeWidth={2.25} />
              Saved
            </div>
            <div className="mt-1.5 truncate text-sm font-semibold text-text-main">{presetCountLabel}</div>
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-bold uppercase text-text-muted">
              <Sparkles className="h-3.5 w-3.5" strokeWidth={2.25} />
              Selected
            </div>
            <div className="mt-1.5 truncate text-sm font-semibold text-text-main">{selectedPlacementLabel}</div>
          </div>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-x-5 gap-y-5 2xl:grid-cols-[minmax(16rem,0.92fr)_minmax(0,1.08fr)]">
        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2 text-[10px] font-bold uppercase text-text-muted">
            <Save className="h-3.5 w-3.5" strokeWidth={2.25} />
            Capture current layout
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center 2xl:flex-col 2xl:items-stretch">
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSave();
              }}
              placeholder="Preset name"
              className="flex-1"
            />
            <Button
              disabled={!canSave}
              onClick={handleSave}
              leftIcon={<Save className="h-4 w-4" strokeWidth={2.25} />}
              className="shrink-0 shadow-none hover:shadow-none"
            >
              Save preset
            </Button>
          </div>
          {!canUseCastle && (
            <div className="mt-3 text-xs font-medium text-warning">
              Castle focus required.
            </div>
          )}
        </div>

        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2 text-[10px] font-bold uppercase text-text-muted">
            <Layers className="h-3.5 w-3.5" strokeWidth={2.25} />
            Saved layout
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <Select
              value={selectedPresetId}
              options={presets.map((preset) => ({
                value: preset.id,
                label: preset.name,
              }))}
              onChange={setSelectedPresetId}
              placeholder={hasPresets ? 'Select a preset' : 'No presets saved'}
              disabled={!canUseCastle || !hasPresets}
              className="min-w-0 flex-1"
              menuGrowToViewport
            />
            <div className="grid grid-cols-2 gap-2 sm:flex sm:shrink-0">
              <Button
                size="sm"
                disabled={!canUseCastle || !selectedPreset}
                onClick={() => selectedPreset && handleApply(selectedPreset.id)}
                title="Apply selected preset"
                leftIcon={<Play className="h-3.5 w-3.5" strokeWidth={2.5} />}
                className="shadow-none hover:shadow-none"
              >
                Apply
              </Button>
              <Button
                variant="danger"
                size="sm"
                disabled={!selectedPreset}
                onClick={() => selectedPreset && handleDelete(selectedPreset.id)}
                leftIcon={<Trash2 className="h-3.5 w-3.5" strokeWidth={2.25} />}
                className="shadow-none hover:shadow-none"
              >
                Delete
              </Button>
            </div>
          </div>
          {canUseCastle && !hasPresets && (
            <div className="mt-3 text-xs font-medium text-text-muted">
              No saved presets for this castle.
            </div>
          )}
        </div>
      </div>
      {operationError && <p className="text-xs text-error">{operationError}</p>}
    </div>
  );
};

function parsePresetDocument(value: unknown): DecorationPresetDocument {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { version: 1, castles: {} };
  }
  const raw = value as { castles?: unknown };
  if (!raw.castles || typeof raw.castles !== 'object' || Array.isArray(raw.castles)) {
    return { version: 1, castles: {} };
  }
  const castles: Record<string, NamedPreset[]> = {};
  for (const [castleId, presets] of Object.entries(raw.castles as Record<string, unknown>)) {
    if (!Array.isArray(presets)) continue;
    castles[castleId] = presets.filter(isNamedPreset);
  }
  return { version: 1, castles };
}

function isNamedPreset(value: unknown): value is NamedPreset {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const preset = value as Partial<NamedPreset>;
  return typeof preset.id === 'string' && typeof preset.name === 'string' && Array.isArray(preset.items);
}

export default DecorationPresetsPanel;
