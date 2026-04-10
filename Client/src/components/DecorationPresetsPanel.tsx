import React, { useEffect, useRef, useState } from 'react';
import { Layers, Play, Save, Trash2, XCircle } from 'lucide-react';
import { useCastleFocus } from '../context/CastleFocusContext';
import { FrontendWebsocket } from '../websocket';
import CastleFocusHoverPopover from './CastleFocusHoverPopover';
import { castleFocusDisplayName } from '../types/castleFocusState.ts';
import { Icons } from './Icons';
import { Card, CardHeader, CardTitle, CardContent, Input, Button, Select } from './ui';

interface NamedPreset {
  id: string;
  name: string;
  items: { wid: number; x: number; y: number; r: number; layer: string }[];
}

const DecorationPresetsPanel: React.FC = () => {
  const { castleFocus } = useCastleFocus();
  const [presets, setPresets] = useState<NamedPreset[]>([]);
  const [newName, setNewName] = useState('');
  const [progress, setProgress] = useState<string | null>(null);
  const [selectedPresetId, setSelectedPresetId] = useState('');

  const castleId = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;
  const focusLabel = castleFocusDisplayName(castleFocus);

  /** Latest focused castle id for websocket handlers (avoid applying stale preset lists). */
  const castleIdRef = useRef(castleId);
  castleIdRef.current = castleId;

  useEffect(() => {
    setPresets([]);
    setProgress(null);
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
      if (msg.type === 'decorationPlacerProgress' && msg.payload && typeof msg.payload === 'object') {
        const m = (msg.payload as { message?: string }).message;
        setProgress(m ?? null);
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
    setProgress(null);
    FrontendWebsocket.sendApplyDecorationPreset(castleId, presetId, castleFocus?.kingdomID);
  };

  const handleDelete = (presetId: string) => {
    if (castleId <= 0) return;
    if (!window.confirm('Delete this decoration preset?')) return;
    FrontendWebsocket.sendDeleteDecorationPreset(castleId, presetId);
  };

  const selectedPreset = presets.find((preset) => preset.id === selectedPresetId) ?? null;

  return (
    <div className="grid grid-cols-1 gap-6 h-full">
      {/* Save from current scan */}
      <Card variant="solid" className="flex flex-col min-h-0 border-border-base bg-bg-app/20">
        <CardHeader className="bg-bg-card-hover/50 pb-4 border-b border-border-base rounded-t-[calc(var(--radius-global)-1px)]">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/10">
              <Icons.Sparkles className="h-4 w-4 text-violet-400" />
            </div>
            <div>
              <CardTitle className="text-base">Save layout as preset</CardTitle>
              <p className="text-xs text-text-muted mt-0.5">
                Stores pickup-eligible decoration rows from your current in-game castle focus (JAA).
              </p>
            </div>
          </div>
        </CardHeader>

        <CardContent className="flex flex-col flex-1 min-h-0 space-y-5 p-5">
          <div className="px-2 py-1">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-end">
              <div className="min-w-0 lg:w-48">
                <div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Focused castle</div>
                <div className="mt-1.5 flex min-h-[42px] items-center">
                  <CastleFocusHoverPopover
                    castleFocus={castleFocus}
                    align="start"
                    expandToViewport
                    className="min-w-0 max-w-full"
                  >
                    <span className="cursor-help truncate border-b border-dotted border-text-muted/50 font-medium text-text-main">
                      {focusLabel}
                    </span>
                  </CastleFocusHoverPopover>
                </div>
              </div>
              <div className="min-w-0 flex-1">
                <label className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
                  Preset name
                </label>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <Input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleSave();
                  }}
                  placeholder="e.g. Event layout"
                    className="flex-1"
                />
                  <Button
                    disabled={castleId <= 0 || !newName.trim()}
                    onClick={handleSave}
                    leftIcon={<Save className="h-4 w-4" strokeWidth={2.25} />}
                    className="shrink-0 shadow-none hover:shadow-none"
                  >
                    Save preset
                  </Button>
                </div>
              </div>
            </div>
            {castleId <= 0 && (
              <div className="mt-3 text-xs text-warning">Focus a castle in the game to enable saving.</div>
            )}
          </div>

          {progress && (
            <div className="space-y-3 rounded-global border border-warning/35 bg-warning/10 px-4 py-3">
              <p className="font-mono text-xs text-warning/90">{progress}</p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => FrontendWebsocket.sendCancelDecorationApply()}
                className="border-warning/40 text-warning hover:bg-warning/10 w-full justify-center shadow-none hover:shadow-none"
                leftIcon={<XCircle className="h-3.5 w-3.5" strokeWidth={2.25} />}
              >
                Cancel apply
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Saved presets */}
      <Card variant="solid" className="flex flex-col min-h-0 border-border-base bg-bg-app/20">
        <CardHeader className="bg-bg-card-hover/50 pb-4 border-b border-border-base rounded-t-[calc(var(--radius-global)-1px)]">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-teal-500/10">
              <Layers className="h-4 w-4 text-teal-400" strokeWidth={2.25} />
            </div>
            <div>
              <CardTitle className="text-base">Saved presets</CardTitle>
              <p className="text-xs text-text-muted mt-0.5">
                Per castle. Apply runs the smart replacer (SOB / EBU) until the layout matches.
              </p>
            </div>
          </div>
        </CardHeader>

        <CardContent className="flex flex-col flex-1 min-h-0 p-5">
          {castleId <= 0 ? (
            <p className="text-sm text-text-muted">Focus a castle in-game to load and manage presets for it.</p>
          ) : presets.length === 0 ? (
            <div className="rounded-global border border-dashed border-border-base bg-bg-card px-4 py-8 text-center h-full flex flex-col justify-center">
              <p className="text-sm text-text-muted font-medium">No presets yet for this castle.</p>
              <p className="mt-1 text-xs text-text-muted/80">Name a layout above and save from your current focus.</p>
            </div>
          ) : (
            <div className="px-2 py-1">
              <label className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
                Saved preset
              </label>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <Select
                  value={selectedPresetId}
                  options={presets.map((preset) => ({
                    value: preset.id,
                    label: preset.name,
                  }))}
                  onChange={setSelectedPresetId}
                  placeholder="Select a preset"
                  className="min-w-0 flex-1"
                />
                <div className="flex shrink-0 gap-2">
                  <Button
                    size="sm"
                    disabled={castleId <= 0 || !selectedPreset}
                    onClick={() => selectedPreset && handleApply(selectedPreset.id)}
                    title="Run smart replacer until layout matches this preset"
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
              {selectedPreset && (
                <div className="mt-3 text-xs text-text-muted">
                  {selectedPreset.items?.length ?? 0} decoration placements
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default DecorationPresetsPanel;
