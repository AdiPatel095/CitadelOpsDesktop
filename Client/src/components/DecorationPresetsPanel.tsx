import React, { useEffect, useRef, useState } from 'react';
import { Castle, Layers, Play, Save, Sparkles, Trash2 } from 'lucide-react';
import { useCastleFocus } from '../context/CastleFocusContext';
import { FrontendWebsocket } from '../Websocket';
import CastleFocusHoverPopover from './CastleFocusHoverPopover';
import { castleFocusDisplayName } from '../types/CastleFocusState.ts';
import { Input, Button, Select } from './ui';

interface NamedPreset {
  id: string;
  name: string;
  items: { wid: number; x: number; y: number; r: number; layer: string }[];
}

const DecorationPresetsPanel: React.FC = () => {
  const { castleFocus } = useCastleFocus();
  const [presets, setPresets] = useState<NamedPreset[]>([]);
  const [newName, setNewName] = useState('');
  const [selectedPresetId, setSelectedPresetId] = useState('');

  const castleId = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;
  const focusLabel = castleFocusDisplayName(castleFocus);

  /** Latest focused castle id for websocket handlers (avoid applying stale preset lists). */
  const castleIdRef = useRef(castleId);
  castleIdRef.current = castleId;

  useEffect(() => {
    setPresets([]);
    setSelectedPresetId('');
    if (castleId > 0) {
      FrontendWebsocket.sendGetDecorationPresets(castleId);
    }
  }, [castleId]);

  useEffect(() => {
    const onMsg = (msg: { type?: string; payload?: unknown; optionalData?: string }) => {
      if (msg.type === 'decorationPresets' && Array.isArray(msg.payload)) {
        const tag = msg.optionalData?.trim() ?? '';
        const responseCastleId = tag === '' ? NaN : parseInt(tag, 10);
        const current = castleIdRef.current;
        if (!Number.isFinite(responseCastleId)) {
          return;
        }
        if (responseCastleId !== current) {
          return;
        }
        const nextPresets = msg.payload as NamedPreset[];
        setPresets(nextPresets);
        setSelectedPresetId((prev) =>
          prev && nextPresets.some((preset) => preset.id === prev) ? prev : nextPresets[0]?.id ?? ''
        );
      }
    };
    FrontendWebsocket.addMessageListener(onMsg);
    return () => FrontendWebsocket.removeMessageListener(onMsg);
  }, []);

  const handleSave = () => {
    if (!newName.trim()) return;
    FrontendWebsocket.sendSaveDecorationPreset(newName.trim(), castleId > 0 ? castleId : undefined);
    setNewName('');
  };

  const handleApply = (presetId: string) => {
    if (castleId <= 0) return;
    FrontendWebsocket.sendApplyDecorationPreset(castleId, presetId, castleFocus?.kingdomID);
  };

  const handleDelete = (presetId: string) => {
    if (castleId <= 0) return;
    if (!window.confirm('Delete this decoration preset?')) return;
    FrontendWebsocket.sendDeleteDecorationPreset(castleId, presetId);
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
              castleFocus={castleFocus}
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
    </div>
  );
};

export default DecorationPresetsPanel;
