import React, { useEffect, useRef, useState } from 'react';
import { Layers, Play, Save, Trash2, XCircle } from 'lucide-react';
import { useCastleFocus } from '../context/CastleFocusContext';
import { FrontendWebsocket } from '../websocket';
import CastleFocusHoverPopover from './CastleFocusHoverPopover';
import { castleFocusDisplayName } from '../types/castleFocusState.ts';
import { Icons } from './Icons';

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

  const castleId = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;
  const focusLabel = castleFocusDisplayName(castleFocus);

  /** Latest focused castle id for websocket handlers (avoid applying stale preset lists). */
  const castleIdRef = useRef(castleId);
  castleIdRef.current = castleId;

  useEffect(() => {
    setPresets([]);
    setProgress(null);
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
        setPresets(msg.payload as NamedPreset[]);
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

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 lg:items-stretch">
      {/* Save from current scan */}
      <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-global border border-border-base bg-bg-card">
        <div className="flex items-center gap-3 border-b border-border-base bg-bg-card-hover/50 px-6 py-4">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/10">
            <Icons.Sparkles className="h-4 w-4 text-violet-400" />
          </div>
          <div>
            <h2 className="text-lg font-bold text-text-main">Save layout as preset</h2>
            <p className="text-xs text-text-muted">
              Stores pickup-eligible decoration rows from your current in-game castle focus (JAA).
            </p>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 flex-col space-y-5 p-6">
          <div className="rounded-global border border-border-light/80 bg-bg-app/40 px-4 py-3">
            <div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Focused castle</div>
            <div className="mt-1.5 flex flex-wrap items-center gap-2">
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
              {castleId <= 0 && (
                <span className="text-xs text-amber-500/90">Focus a castle in the game to enable saving.</span>
              )}
            </div>
          </div>

          <div className="flex flex-col gap-4 sm:flex-row sm:items-end">
            <div className="min-w-0 flex-1">
              <label className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
                Preset name
              </label>
              <input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleSave();
                }}
                className="w-full rounded-global border border-border-light bg-bg-input px-3 py-2.5 text-sm text-text-main transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary/50"
                placeholder="e.g. Event layout"
              />
            </div>
            <button
              type="button"
              disabled={castleId <= 0 || !newName.trim()}
              onClick={handleSave}
              className="inline-flex shrink-0 items-center justify-center gap-2 rounded-global bg-primary px-5 py-2.5 text-sm font-bold text-bg-app shadow-lg shadow-primary/20 transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Save className="h-4 w-4" strokeWidth={2.25} />
              Save preset
            </button>
          </div>

          {progress && (
            <div className="space-y-3 rounded-global border border-amber-500/35 bg-amber-500/10 px-4 py-3">
              <p className="font-mono text-xs text-amber-200/95">{progress}</p>
              <button
                type="button"
                onClick={() => FrontendWebsocket.sendCancelDecorationApply()}
                className="inline-flex items-center gap-2 rounded-global border border-border-light bg-bg-card-hover px-4 py-2 text-xs font-bold text-text-main transition-colors hover:border-amber-500/40 hover:bg-bg-input"
              >
                <XCircle className="h-3.5 w-3.5 text-amber-400" strokeWidth={2.25} />
                Cancel apply
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Saved presets */}
      <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-global border border-border-base bg-bg-card">
        <div className="flex items-center gap-3 border-b border-border-base bg-bg-card-hover/50 px-6 py-4">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-teal-500/10">
            <Layers className="h-4 w-4 text-teal-400" strokeWidth={2.25} />
          </div>
          <div>
            <h2 className="text-lg font-bold text-text-main">Saved presets</h2>
            <p className="text-xs text-text-muted">Per castle. Apply runs the smart replacer (SOB / EBU) until the layout matches.</p>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 flex-col p-6">
          {castleId <= 0 ? (
            <p className="text-sm text-text-muted">Focus a castle in-game to load and manage presets for it.</p>
          ) : presets.length === 0 ? (
            <div className="rounded-global border border-dashed border-border-light bg-bg-app/30 px-4 py-8 text-center">
              <p className="text-sm text-text-muted">No presets yet for this castle.</p>
              <p className="mt-1 text-xs text-text-muted/80">Name a layout above and save from your current focus.</p>
            </div>
          ) : (
            <ul className="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1 [scrollbar-gutter:stable]">
              {presets.map((p) => (
                <li
                  key={p.id}
                  className="flex flex-col gap-3 rounded-global border border-border-base bg-bg-card-hover/35 p-4 transition-colors hover:bg-bg-card-hover/55 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0">
                    <div className="font-semibold text-text-main">{p.name}</div>
                    <div className="mt-0.5 text-xs text-text-muted">{p.items?.length ?? 0} decoration placements</div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <button
                      type="button"
                      disabled={castleId <= 0}
                      onClick={() => handleApply(p.id)}
                      title="Run smart replacer until layout matches this preset"
                      className="inline-flex items-center justify-center gap-2 rounded-global bg-emerald-500 px-4 py-2 text-xs font-bold text-white shadow-md shadow-emerald-500/25 transition-colors hover:bg-emerald-600 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      <Play className="h-3.5 w-3.5" strokeWidth={2.5} />
                      Apply
                    </button>
                    <button
                      type="button"
                      onClick={() => handleDelete(p.id)}
                      className="inline-flex items-center justify-center gap-2 rounded-global border border-border-light bg-bg-card-hover px-4 py-2 text-xs font-bold text-text-muted transition-colors hover:border-red-500/40 hover:bg-red-500/10 hover:text-red-300"
                    >
                      <Trash2 className="h-3.5 w-3.5" strokeWidth={2.25} />
                      Delete
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
};

export default DecorationPresetsPanel;
