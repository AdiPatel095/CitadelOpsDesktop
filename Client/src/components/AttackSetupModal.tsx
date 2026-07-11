import React, { useEffect, useId, useMemo, useState } from 'react';
import {
  Boxes,
  ChevronDown,
  ChevronUp,
  Copy,
  Eraser,
  Minus,
  MousePointerClick,
  Plus,
  Shield,
  Swords,
} from 'lucide-react';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { useCitadelAPI } from '../api/ApiContext';
import type { CastleStateV2 } from '../api/Contracts';
import { showToolPicker } from './ToolPickerModal';
import { showTroopPicker } from './TroopPickerModal';
import { Badge, Button, Card, CardContent, CardHeader, Input, Modal, PillSelector } from './ui';

export interface AttackSetupSlot {
  itemId: number | null;
  quantity: number;
}

export interface AttackSetupLane {
  troops: AttackSetupSlot[];
  tools: AttackSetupSlot[];
}

export interface AttackSetupWave {
  L: AttackSetupLane;
  M: AttackSetupLane;
  R: AttackSetupLane;
}

export interface AttackSetupDraft {
  name: string;
  waves: AttackSetupWave[];
}

/** Optional inventory scope for callers that need something other than the default all-castles aggregate. */
export interface AttackSetupInventory {
  label?: string;
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
}

export interface AttackSetupModalProps {
  isOpen: boolean;
  initialDraft?: AttackSetupDraft;
  inventory?: AttackSetupInventory;
  onClose: () => void;
  onSave: (draft: AttackSetupDraft) => void;
}

type LaneKey = 'L' | 'M' | 'R';
type InventoryKind = 'troop' | 'tool';

interface InventoryItem {
  id: number;
  name: string;
  stock: number;
  metadata: MetadataItem;
}

interface InventoryIssue {
  kind: InventoryKind;
  itemId: number;
  requested: number;
  stock: number;
}

const laneKeys: LaneKey[] = ['L', 'M', 'R'];
const MAX_WAVES = 10;

const AttackSetupModal: React.FC<AttackSetupModalProps> = ({ isOpen, initialDraft, inventory: inventoryOverride, onClose, onSave }) => {
  const { state } = useCitadelAPI();
  const { troops, tools, isLoading: isMetadataLoading } = useMetadata();
  const [draft, setDraft] = useState<AttackSetupDraft>(() => normalizeDraft(initialDraft));
  const [activeWaveIndex, setActiveWaveIndex] = useState(0);
  const [collapsedWaveIndexes, setCollapsedWaveIndexes] = useState<Set<number>>(() => new Set());

  const allCastlesInventory = useMemo(
    () => isMetadataLoading
      ? { troops: {}, tools: {}, castleCount: Object.keys(state?.castles ?? {}).length }
      : aggregateCastleInventory(Object.values(state?.castles ?? {}), troops, tools),
    [isMetadataLoading, state?.castles, tools, troops]
  );
  const inventory = useMemo(
    () => inventoryOverride
      ? { troops: inventoryOverride.troopStock, tools: inventoryOverride.toolStock }
      : { troops: allCastlesInventory.troops, tools: allCastlesInventory.tools },
    [allCastlesInventory, inventoryOverride]
  );
  const inventoryLabel = inventoryOverride?.label?.trim()
    || `Account inventory · ${allCastlesInventory.castleCount.toLocaleString()} castle${allCastlesInventory.castleCount === 1 ? '' : 's'}`;
  const hasInventory = !isMetadataLoading && (Object.keys(inventory.troops).length > 0 || Object.keys(inventory.tools).length > 0);
  const troopItems = useMemo(() => inventoryItems(inventory.troops, troops), [inventory.troops, troops]);
  const toolItems = useMemo(() => inventoryItems(inventory.tools, tools), [inventory.tools, tools]);
  const troopIDs = useMemo(() => troopItems.map((item) => item.id), [troopItems]);
  const toolIDs = useMemo(() => toolItems.map((item) => item.id), [toolItems]);

  useEffect(() => {
    if (!isOpen) return;
    setDraft(normalizeDraft(initialDraft));
    setActiveWaveIndex(0);
    setCollapsedWaveIndexes(new Set());
  }, [initialDraft, isOpen]);

  const totals = useMemo(() => summarizeDraft(draft), [draft]);
  const allocations = useMemo(() => allocatedInventory(draft), [draft]);
  const inventoryIssues = useMemo(
    () => findInventoryIssues(allocations, inventory),
    [allocations, inventory]
  );
  const canSave = draft.name.trim().length > 0 && totals.troops > 0 && inventoryIssues.length === 0;
  const activeWave = draft.waves[activeWaveIndex] ?? draft.waves[0];

  const setWaveCount = (count: number) => {
    const nextCount = Math.max(1, Math.min(MAX_WAVES, Math.trunc(count) || 1));
    setDraft((current) => {
      if (current.waves.length === nextCount) return current;
      if (current.waves.length > nextCount) {
        return { ...current, waves: current.waves.slice(0, nextCount) };
      }
      return {
        ...current,
        waves: [
          ...current.waves,
          ...Array.from({ length: nextCount - current.waves.length }, () => emptyWave()),
        ],
      };
    });
    setActiveWaveIndex((current) => Math.min(current, nextCount - 1));
    setCollapsedWaveIndexes((current) => new Set(Array.from(current).filter((index) => index < nextCount)));
  };

  const updateActiveWave = (updater: (wave: AttackSetupWave) => AttackSetupWave) => {
    updateWaveAt(activeWaveIndex, updater);
  };

  const updateWaveAt = (waveIndex: number, updater: (wave: AttackSetupWave) => AttackSetupWave) => {
    setDraft((current) => ({
      ...current,
      waves: current.waves.map((wave, index) => (index === waveIndex ? updater(wave) : wave)),
    }));
  };

  const selectWave = (waveIndex: number, scrollIntoView = false) => {
    setActiveWaveIndex(waveIndex);
    setCollapsedWaveIndexes((current) => {
      if (!current.has(waveIndex)) return current;
      const next = new Set(current);
      next.delete(waveIndex);
      return next;
    });
    if (!scrollIntoView) return;
    window.requestAnimationFrame(() => {
      document.getElementById(`attack-wave-${waveIndex}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  };

  const duplicateWave = () => {
    if (draft.waves.length >= MAX_WAVES || !activeWave) return;
    const insertAt = activeWaveIndex + 1;
    setDraft((current) => ({
      ...current,
      waves: [
        ...current.waves.slice(0, insertAt),
        cloneWave(current.waves[activeWaveIndex]),
        ...current.waves.slice(insertAt),
      ],
    }));
    setCollapsedWaveIndexes((current) => new Set(Array.from(current, (index) => index >= insertAt ? index + 1 : index)));
    setActiveWaveIndex(insertAt);
  };

  const toggleWaveCollapsed = (waveIndex: number) => {
    setActiveWaveIndex(waveIndex);
    setCollapsedWaveIndexes((current) => {
      const next = new Set(current);
      if (next.has(waveIndex)) {
        next.delete(waveIndex);
      } else {
        next.add(waveIndex);
      }
      return next;
    });
  };

  const fillAllWaves = () => {
    if (!activeWave) return;
    setDraft((current) => ({
      ...current,
      waves: current.waves.map(() => cloneWave(activeWave)),
    }));
  };

  const handleSave = () => {
    if (!canSave) return;
    onSave({ ...draft, name: draft.name.trim() });
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="full"
      title={
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-global border border-primary/30 bg-primary/10 text-primary shadow-glow">
            <Swords className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <div className="truncate text-lg font-black">Attack preset</div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs font-medium text-text-muted">
              <span>{inventoryLabel}</span>
              <Badge variant={isMetadataLoading ? 'secondary' : hasInventory ? 'success' : 'warning'} className="normal-case tracking-normal">
                {isMetadataLoading ? 'Loading inventory' : hasInventory ? 'All-castles inventory' : 'Inventory unavailable'}
              </Badge>
            </div>
          </div>
        </div>
      }
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
          <div className="text-xs text-text-muted">
            {inventoryIssues.length > 0 ? (
              <span className="font-semibold text-error">
                {inventoryIssues.length} stock conflict{inventoryIssues.length === 1 ? '' : 's'} must be resolved
              </span>
            ) : totals.troops === 0 ? (
              'Add at least one troop to save this preset.'
            ) : (
              `${totals.troops.toLocaleString()} troops and ${totals.tools.toLocaleString()} tools allocated`
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" onClick={onClose}>Cancel</Button>
            <Button variant="primary" onClick={handleSave} disabled={!canSave}>Save preset</Button>
          </div>
        </div>
      }
    >
      <div className="mx-auto flex w-full max-w-[1760px] flex-col gap-4">
        <section className="grid gap-3 rounded-global border border-border-base bg-bg-card/65 p-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl lg:grid-cols-[minmax(15rem,1.4fr)_auto_auto] lg:items-end">
          <label className="block min-w-0">
            <span className="mb-1.5 block text-[11px] font-black uppercase tracking-wider text-text-muted">Preset name</span>
            <Input
              value={draft.name}
              onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
              placeholder="e.g. RBC — 5 wave ranged"
              maxLength={80}
              className="font-semibold"
            />
          </label>

          <div>
            <span className="mb-1.5 block text-[11px] font-black uppercase tracking-wider text-text-muted">Waves</span>
            <div className="flex items-center gap-2">
              <Button
                variant="secondary"
                size="icon"
                onClick={() => setWaveCount(draft.waves.length - 1)}
                disabled={draft.waves.length <= 1}
                title="Remove last wave"
              >
                <Minus className="h-4 w-4" />
              </Button>
              <Input
                type="number"
                min={1}
                max={MAX_WAVES}
                value={draft.waves.length}
                onChange={(event) => setWaveCount(Number(event.target.value))}
                className="w-16 text-center font-mono font-bold"
                aria-label="Wave count"
              />
              <Button
                variant="secondary"
                size="icon"
                onClick={() => setWaveCount(draft.waves.length + 1)}
                disabled={draft.waves.length >= MAX_WAVES}
                title="Add wave"
              >
                <Plus className="h-4 w-4" />
              </Button>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2">
            <Metric label="Waves" value={draft.waves.length.toLocaleString()} />
            <Metric label="Troops" value={totals.troops.toLocaleString()} />
            <Metric label="Tools" value={totals.tools.toLocaleString()} />
          </div>
        </section>

        <section className="flex flex-wrap items-center justify-between gap-3 rounded-global border border-border-base bg-bg-card/55 p-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl">
          <div className="min-w-0 flex-1 overflow-x-auto custom-scrollbar">
            <PillSelector
              value={String(activeWaveIndex)}
              onChange={(value) => selectWave(Number(value), true)}
              options={draft.waves.map((wave, index) => {
                const waveTotals = summarizeWave(wave);
                return {
                  value: String(index),
                  label: `Wave ${index + 1}`,
                  title: `${waveTotals.troops.toLocaleString()} troops · ${waveTotals.tools.toLocaleString()} tools`,
                };
              })}
              size="sm"
            />
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button variant="ghost" size="sm" onClick={duplicateWave} disabled={draft.waves.length >= MAX_WAVES} leftIcon={<Copy className="h-3.5 w-3.5" />}>
              Duplicate selected
            </Button>
            <Button variant="ghost" size="sm" onClick={fillAllWaves} disabled={draft.waves.length <= 1} leftIcon={<Boxes className="h-3.5 w-3.5" />}>
              Fill all
            </Button>
            <Button variant="ghost" size="sm" onClick={() => updateActiveWave(() => emptyWave())} leftIcon={<Eraser className="h-3.5 w-3.5" />}>
              Clear selected
            </Button>
          </div>
        </section>

        <div className="space-y-5">
          {draft.waves.map((wave, waveIndex) => (
            <WaveEditorCard
              key={waveIndex}
              id={`attack-wave-${waveIndex}`}
              waveIndex={waveIndex}
              wave={wave}
              isActive={waveIndex === activeWaveIndex}
              isCollapsed={collapsedWaveIndexes.has(waveIndex)}
              troopItems={troopItems}
              toolItems={toolItems}
              troopStock={inventory.troops}
              toolStock={inventory.tools}
              troopAllocations={allocations.troops}
              toolAllocations={allocations.tools}
              onActivate={() => selectWave(waveIndex)}
              onToggleCollapsed={() => toggleWaveCollapsed(waveIndex)}
              onChangeLane={(laneKey, lane) => {
                updateWaveAt(waveIndex, (currentWave) => ({ ...currentWave, [laneKey]: lane }));
              }}
              onPickTroop={async (laneKey, slot, slotIndex) => {
                const result = await showTroopPicker({
                  mode: 'single',
                  title: `Choose a troop for Wave ${waveIndex + 1} · ${laneLabel(laneKey)}`,
                  preselected: slot.itemId == null ? [] : [slot.itemId],
                  allowedUnitIds: troopIDs,
                  stockQuantities: inventory.troops,
                });
                if (typeof result !== 'number') return;
                updateWaveAt(waveIndex, (currentWave) => ({
                  ...currentWave,
                  [laneKey]: {
                    ...currentWave[laneKey],
                    troops: updateSlot(currentWave[laneKey].troops, slotIndex, {
                      itemId: result,
                      quantity: slot.quantity > 0 ? slot.quantity : 1,
                    }),
                  },
                }));
              }}
              onPickTool={async (laneKey, slot, slotIndex) => {
                const result = await showToolPicker({
                  mode: 'single',
                  title: `Choose a tool for Wave ${waveIndex + 1} · ${laneLabel(laneKey)}`,
                  preselected: slot.itemId == null ? [] : [slot.itemId],
                  allowedToolIds: toolIDs,
                  stockQuantities: inventory.tools,
                });
                if (typeof result !== 'number') return;
                updateWaveAt(waveIndex, (currentWave) => ({
                  ...currentWave,
                  [laneKey]: {
                    ...currentWave[laneKey],
                    tools: updateSlot(currentWave[laneKey].tools, slotIndex, {
                      itemId: result,
                      quantity: slot.quantity > 0 ? slot.quantity : 1,
                    }),
                  },
                }));
              }}
            />
          ))}
        </div>

        {inventoryIssues.length > 0 ? (
          <section className="rounded-global border border-error/30 bg-error/8 p-3 text-sm text-error">
            <div className="mb-2 font-black">Preset exceeds available inventory</div>
            <div className="flex flex-wrap gap-2">
              {inventoryIssues.map((issue) => {
                const meta = issue.kind === 'troop' ? troops[issue.itemId] : tools[issue.itemId];
                return (
                  <span key={`${issue.kind}-${issue.itemId}`} className="rounded-full border border-error/25 bg-bg-card/45 px-3 py-1.5 text-xs font-semibold">
                    {meta?.name || `#${issue.itemId}`}: {issue.requested.toLocaleString()} / {issue.stock.toLocaleString()}
                  </span>
                );
              })}
            </div>
          </section>
        ) : null}
      </div>
    </Modal>
  );
};

const Metric: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="min-w-[4.75rem] rounded-global border border-border-base bg-bg-input/45 px-3 py-2">
    <div className="text-[9px] font-black uppercase tracking-wider text-text-muted">{label}</div>
    <div className="mt-1 font-mono text-sm font-black text-text-main">{value}</div>
  </div>
);

interface WaveEditorCardProps {
  id: string;
  waveIndex: number;
  wave: AttackSetupWave;
  isActive: boolean;
  isCollapsed: boolean;
  troopItems: InventoryItem[];
  toolItems: InventoryItem[];
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
  troopAllocations: Record<number, number>;
  toolAllocations: Record<number, number>;
  onActivate: () => void;
  onToggleCollapsed: () => void;
  onChangeLane: (laneKey: LaneKey, lane: AttackSetupLane) => void;
  onPickTroop: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
  onPickTool: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
}

const WaveEditorCard: React.FC<WaveEditorCardProps> = ({
  id,
  waveIndex,
  wave,
  isActive,
  isCollapsed,
  troopItems,
  toolItems,
  troopStock,
  toolStock,
  troopAllocations,
  toolAllocations,
  onActivate,
  onToggleCollapsed,
  onChangeLane,
  onPickTroop,
  onPickTool,
}) => {
  const waveTotals = summarizeWave(wave);
  return (
    <Card
      id={id}
      variant="solid"
      className={`liquid-prominent-header-card scroll-mt-3 ${isActive ? 'ring-1 ring-primary/30' : ''}`}
      onMouseDown={onActivate}
    >
      <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-global border ${isActive ? 'border-primary/40 bg-primary/12 text-primary shadow-glow' : 'border-border-base bg-bg-input/70 text-text-muted'}`}>
            <Swords className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="m-0 text-base font-black text-text-main">Wave {waveIndex + 1}</h3>
              {isActive ? <Badge variant="primary" className="normal-case tracking-normal">Selected</Badge> : null}
            </div>
            <p className="mt-1 text-xs text-text-muted">Three fronts · 10 unit slots · 7 tool slots</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Metric label="Troops" value={waveTotals.troops.toLocaleString()} />
          <Metric label="Tools" value={waveTotals.tools.toLocaleString()} />
          <Button
            variant="ghost"
            size="icon"
            onClick={onToggleCollapsed}
            onMouseDown={(event) => event.stopPropagation()}
            aria-expanded={!isCollapsed}
            aria-controls={`${id}-content`}
            title={isCollapsed ? `Expand Wave ${waveIndex + 1}` : `Collapse Wave ${waveIndex + 1}`}
            className="ml-1"
          >
            {isCollapsed ? <ChevronDown className="h-4 w-4" /> : <ChevronUp className="h-4 w-4" />}
          </Button>
        </div>
      </CardHeader>

      {!isCollapsed ? (
        <CardContent id={`${id}-content`} className="liquid-prominent-header-content space-y-4 p-4">
          <FormationRow
            kind="troop"
            label="Units"
            wave={wave}
            items={troopItems}
            stock={troopStock}
            allocations={troopAllocations}
            onChangeLane={onChangeLane}
            onPick={onPickTroop}
          />
          <FormationRow
            kind="tool"
            label="Tools"
            wave={wave}
            items={toolItems}
            stock={toolStock}
            allocations={toolAllocations}
            onChangeLane={onChangeLane}
            onPick={onPickTool}
          />
        </CardContent>
      ) : null}
    </Card>
  );
};

interface FormationRowProps {
  kind: InventoryKind;
  label: string;
  wave: AttackSetupWave;
  items: InventoryItem[];
  stock: Record<number, number>;
  allocations: Record<number, number>;
  onChangeLane: (laneKey: LaneKey, lane: AttackSetupLane) => void;
  onPick: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
}

const FormationRow: React.FC<FormationRowProps> = ({ kind, label, wave, items, stock, allocations, onChangeLane, onPick }) => (
  <section className="overflow-hidden rounded-global border border-border-base bg-bg-app/42">
    <div className="flex items-center justify-between gap-3 border-b border-border-base bg-bg-card/50 px-3 py-2.5">
      <div className="flex items-center gap-2">
        <span className={`flex h-7 w-7 items-center justify-center rounded-full ${kind === 'troop' ? 'bg-primary/12 text-primary' : 'bg-info/12 text-info'}`}>
          {kind === 'troop' ? <Swords className="h-3.5 w-3.5" /> : <Shield className="h-3.5 w-3.5" />}
        </span>
        <div>
          <div className="text-xs font-black uppercase tracking-wider text-text-main">{label}</div>
          <div className="text-[9px] text-text-muted">Left · center · right formation</div>
        </div>
      </div>
      <Badge variant="outline" className="font-mono normal-case tracking-normal">
        {laneKeys.reduce((total, laneKey) => total + wave[laneKey][kind === 'troop' ? 'troops' : 'tools'].length, 0)} slots
      </Badge>
    </div>

    <div className="overflow-x-auto p-3 custom-scrollbar">
      <div className="mx-auto flex min-w-max items-start justify-center gap-3">
        {laneKeys.map((laneKey) => {
          const lane = wave[laneKey];
          const slots = kind === 'troop' ? lane.troops : lane.tools;
          return (
            <div key={laneKey} className={`rounded-global border px-2.5 pb-2.5 pt-2 ${laneKey === 'M' ? 'border-primary/25 bg-primary/5' : 'border-border-base bg-bg-card/35'}`}>
              <div className="mb-2 text-center text-[9px] font-black uppercase tracking-wider text-text-muted">
                {laneLabel(laneKey)}
              </div>
              <div className="flex items-start justify-center gap-1.5">
                {slots.map((slot, slotIndex) => (
                  <InventorySlotCard
                    key={slotIndex}
                    kind={kind}
                    index={slotIndex}
                    slot={slot}
                    items={items}
                    stock={stock}
                    allocated={slot.itemId == null ? 0 : allocations[slot.itemId] ?? 0}
                    onChange={(patch) => {
                      const updatedSlots = updateSlot(slots, slotIndex, patch);
                      onChangeLane(laneKey, {
                        ...lane,
                        [kind === 'troop' ? 'troops' : 'tools']: updatedSlots,
                      });
                    }}
                    onPick={() => onPick(laneKey, slot, slotIndex)}
                  />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  </section>
);

interface InventorySlotCardProps {
  kind: InventoryKind;
  index: number;
  slot: AttackSetupSlot;
  items: InventoryItem[];
  stock: Record<number, number>;
  allocated: number;
  onChange: (patch: Partial<AttackSetupSlot>) => void;
  onPick: () => void;
}

const InventorySlotCard: React.FC<InventorySlotCardProps> = ({
  kind,
  index,
  slot,
  items,
  stock,
  allocated,
  onChange,
  onPick,
}) => {
  const listID = useId();
  const selected = slot.itemId == null ? undefined : items.find((item) => item.id === slot.itemId);
  const [query, setQuery] = useState(() => selected ? itemInputLabel(selected) : '');
  const [inputError, setInputError] = useState('');

  useEffect(() => {
    setQuery(selected ? itemInputLabel(selected) : '');
    setInputError('');
  }, [selected]);

  const commitTypedItem = () => {
    const value = query.trim();
    if (!value) {
      onChange({ itemId: null, quantity: 0 });
      setInputError('');
      return;
    }
    const match = matchInventoryItem(value, items);
    if (!match) {
      setInputError('Not available in this inventory');
      return;
    }
    onChange({ itemId: match.id, quantity: slot.quantity > 0 ? slot.quantity : 1 });
    setQuery(itemInputLabel(match));
    setInputError('');
  };

  const available = slot.itemId == null ? 0 : stock[slot.itemId] ?? 0;
  const remainingAfterPreset = available - allocated;
  const overAllocated = slot.itemId != null && allocated > available;

  return (
    <div
      className={`group relative flex aspect-[3/4] w-[clamp(4rem,5.2vw,4.8rem)] shrink-0 flex-col overflow-hidden rounded-[0.95rem] border p-1.5 transition-all hover:-translate-y-0.5 hover:shadow-[var(--glass-shadow-compact)] ${
        overAllocated || inputError
          ? 'border-error/45 bg-error/7'
          : selected
            ? 'border-primary/45 bg-primary/8 shadow-[0_0_16px_color-mix(in_srgb,var(--primary)_14%,transparent)]'
            : 'border-border-base bg-bg-card/60 hover:border-primary/30'
      }`}
    >
      <div className="flex h-3 items-center justify-between gap-1 text-[8px] font-black uppercase text-text-muted">
        <span className={selected ? 'text-primary' : ''}>{kind === 'troop' ? 'U' : 'T'}{index + 1}</span>
        <span className="max-w-[2.2rem] truncate font-mono">{selected ? `#${selected.id}` : '—'}</span>
      </div>

      <input
        list={listID}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setInputError('');
        }}
        onBlur={commitTypedItem}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault();
            commitTypedItem();
            event.currentTarget.blur();
          }
        }}
        placeholder={kind === 'troop' ? 'Unit' : 'Tool'}
        title={inputError || selected?.name || `Type an available ${kind} name or ID`}
        className={`mt-1 min-h-0 w-full flex-1 rounded-lg border bg-bg-input/55 px-1 text-center text-[9px] font-bold leading-tight text-text-main outline-none transition focus:border-primary focus:ring-1 focus:ring-primary ${inputError ? 'border-error text-error' : 'border-border-base'}`}
        aria-label={`${kind} slot ${index + 1}`}
      />
      <datalist id={listID}>
        {items.map((item) => <option key={item.id} value={itemInputLabel(item)} />)}
      </datalist>

      <div className="mt-1 grid grid-cols-[minmax(0,1fr)_1.3rem] gap-1">
        <input
          type="number"
          min={0}
          max={available || undefined}
          value={slot.quantity || ''}
          onChange={(event) => onChange({ quantity: positiveInteger(event.target.value) })}
          placeholder="Qty"
          disabled={slot.itemId == null}
          className="h-5 min-w-0 rounded-md border border-border-base bg-bg-input/65 px-0.5 text-center font-mono text-[9px] font-black text-text-main outline-none transition focus:border-primary focus:ring-1 focus:ring-primary disabled:cursor-not-allowed disabled:opacity-45"
          aria-label={`${kind} slot ${index + 1} quantity`}
        />
        <button
          type="button"
          onClick={onPick}
          disabled={items.length === 0}
          className="flex h-5 w-5 items-center justify-center rounded-md border border-primary/30 bg-primary/8 text-primary transition hover:bg-primary/18 disabled:cursor-not-allowed disabled:opacity-35"
          title={items.length === 0 ? `No available ${kind}s in this inventory` : `Open available ${kind} picker`}
        >
          <MousePointerClick className="h-2.5 w-2.5" />
        </button>
      </div>

      <div className={`mt-1 truncate text-center font-mono text-[8px] leading-none ${inputError || overAllocated ? 'text-error' : 'text-text-muted'}`}>
        {inputError
          ? 'Unavailable'
          : selected
            ? overAllocated
              ? `${allocated.toLocaleString()}/${available.toLocaleString()} used`
              : `${Math.max(0, remainingAfterPreset).toLocaleString()} left`
            : 'Empty'}
      </div>
    </div>
  );
};

function aggregateCastleInventory(
  castles: CastleStateV2[],
  troopMetadata: Record<number, MetadataItem>,
  toolMetadata: Record<number, MetadataItem>
): { troops: Record<number, number>; tools: Record<number, number>; castleCount: number } {
  const troopStock: Record<number, number> = {};
  const toolStock: Record<number, number> = {};
  let castleCount = 0;
  for (const castle of castles) {
    castleCount += 1;
    for (const [rawID, rawCount] of Object.entries(castle.units.stationed)) {
      const id = Number(rawID);
      const count = positiveInteger(rawCount);
      if (!Number.isFinite(id) || id <= 0 || count <= 0) continue;
      if (toolMetadata[id]) {
        toolStock[id] = (toolStock[id] ?? 0) + count;
      } else if (troopMetadata[id]) {
        troopStock[id] = (troopStock[id] ?? 0) + count;
      }
    }
  }
  return { troops: troopStock, tools: toolStock, castleCount };
}

function inventoryItems(stock: Record<number, number>, metadata: Record<number, MetadataItem>): InventoryItem[] {
  return Object.entries(stock)
    .map(([rawID, count]) => {
      const id = Number(rawID);
      const itemMetadata = metadata[id];
      if (!itemMetadata) return null;
      return { id, name: itemMetadata.name || `Item ${id}`, stock: count, metadata: itemMetadata };
    })
    .filter((item): item is InventoryItem => item != null)
    .sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id);
}

function matchInventoryItem(value: string, items: InventoryItem[]): InventoryItem | undefined {
  const normalized = value.trim().toLowerCase();
  const idMatch = normalized.match(/(?:#|\()?(\d+)\)?$/);
  if (idMatch) {
    const item = items.find((candidate) => candidate.id === Number(idMatch[1]));
    if (item) return item;
  }
  const exact = items.find((item) => item.name.toLowerCase() === normalized);
  if (exact) return exact;
  const partial = items.filter((item) => item.name.toLowerCase().includes(normalized));
  return partial.length === 1 ? partial[0] : undefined;
}

function itemInputLabel(item: Pick<InventoryItem, 'id' | 'name'>): string {
  return `${item.name} (#${item.id})`;
}

function allocatedInventory(draft: AttackSetupDraft): { troops: Record<number, number>; tools: Record<number, number> } {
  const allocations = { troops: {} as Record<number, number>, tools: {} as Record<number, number> };
  for (const wave of draft.waves) {
    for (const laneKey of laneKeys) {
      for (const slot of wave[laneKey].troops) {
        if (slot.itemId != null && slot.quantity > 0) {
          allocations.troops[slot.itemId] = (allocations.troops[slot.itemId] ?? 0) + slot.quantity;
        }
      }
      for (const slot of wave[laneKey].tools) {
        if (slot.itemId != null && slot.quantity > 0) {
          allocations.tools[slot.itemId] = (allocations.tools[slot.itemId] ?? 0) + slot.quantity;
        }
      }
    }
  }
  return allocations;
}

function findInventoryIssues(
  allocations: { troops: Record<number, number>; tools: Record<number, number> },
  inventory: { troops: Record<number, number>; tools: Record<number, number> }
): InventoryIssue[] {
  const issues: InventoryIssue[] = [];
  for (const kind of ['troop', 'tool'] as const) {
    const plural = kind === 'troop' ? 'troops' : 'tools';
    for (const [rawID, requested] of Object.entries(allocations[plural])) {
      const itemId = Number(rawID);
      const stock = inventory[plural][itemId] ?? 0;
      if (requested > stock) issues.push({ kind, itemId, requested, stock });
    }
  }
  return issues;
}

function updateSlot(slots: AttackSetupSlot[], index: number, patch: Partial<AttackSetupSlot>): AttackSetupSlot[] {
  return slots.map((slot, slotIndex) => (slotIndex === index ? { ...slot, ...patch } : slot));
}

function positiveInteger(value: string | number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.trunc(parsed)) : 0;
}

function summarizeDraft(draft: AttackSetupDraft): { troops: number; tools: number } {
  return draft.waves.reduce((total, wave) => {
    const waveTotal = summarizeWave(wave);
    return { troops: total.troops + waveTotal.troops, tools: total.tools + waveTotal.tools };
  }, { troops: 0, tools: 0 });
}

function summarizeWave(wave: AttackSetupWave): { troops: number; tools: number } {
  return laneKeys.reduce((total, laneKey) => {
    const laneTotal = summarizeLane(wave[laneKey]);
    return { troops: total.troops + laneTotal.troops, tools: total.tools + laneTotal.tools };
  }, { troops: 0, tools: 0 });
}

function summarizeLane(lane: AttackSetupLane): { troops: number; tools: number } {
  return {
    troops: lane.troops.reduce((total, slot) => total + slot.quantity, 0),
    tools: lane.tools.reduce((total, slot) => total + slot.quantity, 0),
  };
}

function normalizeDraft(draft?: AttackSetupDraft): AttackSetupDraft {
  if (!draft) return defaultDraft();
  const waves = (draft.waves.length > 0 ? draft.waves : [emptyWave()])
    .slice(0, MAX_WAVES)
    .map(normalizeWave);
  return { name: draft.name || 'New attack preset', waves };
}

function normalizeWave(wave: AttackSetupWave): AttackSetupWave {
  return {
    L: normalizeLane(wave.L, 'L'),
    M: normalizeLane(wave.M, 'M'),
    R: normalizeLane(wave.R, 'R'),
  };
}

function normalizeLane(lane: AttackSetupLane | undefined, laneKey: LaneKey): AttackSetupLane {
  const troopSlots = laneKey === 'M' ? 6 : 2;
  const toolSlots = laneKey === 'M' ? 3 : 2;
  return {
    troops: normalizeSlots(lane?.troops, troopSlots),
    tools: normalizeSlots(lane?.tools, toolSlots),
  };
}

function normalizeSlots(slots: AttackSetupSlot[] | undefined, count: number): AttackSetupSlot[] {
  return Array.from({ length: count }, (_, index) => {
    const slot = slots?.[index];
    return slot ? { itemId: slot.itemId, quantity: positiveInteger(slot.quantity) } : emptySlot();
  });
}

function defaultDraft(): AttackSetupDraft {
  return { name: 'New attack preset', waves: [emptyWave()] };
}

function emptyWave(): AttackSetupWave {
  return { L: emptyLane('L'), M: emptyLane('M'), R: emptyLane('R') };
}

function emptyLane(laneKey: LaneKey): AttackSetupLane {
  return normalizeLane(undefined, laneKey);
}

function emptySlot(): AttackSetupSlot {
  return { itemId: null, quantity: 0 };
}

function cloneWave(wave: AttackSetupWave): AttackSetupWave {
  return {
    L: { troops: wave.L.troops.map((slot) => ({ ...slot })), tools: wave.L.tools.map((slot) => ({ ...slot })) },
    M: { troops: wave.M.troops.map((slot) => ({ ...slot })), tools: wave.M.tools.map((slot) => ({ ...slot })) },
    R: { troops: wave.R.troops.map((slot) => ({ ...slot })), tools: wave.R.tools.map((slot) => ({ ...slot })) },
  };
}

function laneLabel(laneKey: LaneKey): string {
  if (laneKey === 'L') return 'Left flank';
  if (laneKey === 'M') return 'Center front';
  return 'Right flank';
}

export default AttackSetupModal;
