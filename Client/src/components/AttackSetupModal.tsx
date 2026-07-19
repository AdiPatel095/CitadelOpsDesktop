import React, { useEffect, useMemo, useState } from 'react';
import {
  Boxes,
  Copy,
  Eraser,
  Minus,
  MousePointerClick,
  Plus,
  Swords,
  X,
} from 'lucide-react';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { useCitadelAPI } from '../api/ApiContext';
import type { CastleStateV2 } from '../api/Contracts';
import ToolImage from './ToolImage';
import { showToolPicker } from './ToolPickerModal';
import UnitImage from './UnitImage';
import { showTroopPicker } from './TroopPickerModal';
import { Badge, Button, Card, CardContent, CardHeader, Input, MetricTile, Modal, ModalTitle, PillSelector } from './ui';

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
  inventoryPolicy?: 'enforced' | 'advisory';
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

const AttackSetupModal: React.FC<AttackSetupModalProps> = ({
  isOpen,
  initialDraft,
  inventory: inventoryOverride,
  inventoryPolicy = 'enforced',
  onClose,
  onSave,
}) => {
  const { state } = useCitadelAPI();
  const { troops, tools, isLoading: isMetadataLoading } = useMetadata();
  const [draft, setDraft] = useState<AttackSetupDraft>(() => normalizeDraft(initialDraft));
  const [activeWaveIndex, setActiveWaveIndex] = useState(0);
  const [activeFormationKind, setActiveFormationKind] = useState<InventoryKind>('troop');

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
  const troopItems = useMemo(
    () => inventoryItems(inventory.troops, troops, inventoryPolicy === 'advisory'),
    [inventory.troops, inventoryPolicy, troops]
  );
  const toolItems = useMemo(
    () => inventoryItems(inventory.tools, tools, inventoryPolicy === 'advisory'),
    [inventory.tools, inventoryPolicy, tools]
  );
  const troopIDs = useMemo(() => troopItems.map((item) => item.id), [troopItems]);
  const toolIDs = useMemo(() => toolItems.map((item) => item.id), [toolItems]);

  useEffect(() => {
    if (!isOpen) return;
    setDraft(normalizeDraft(initialDraft));
    setActiveWaveIndex(0);
    setActiveFormationKind('troop');
  }, [initialDraft, isOpen]);

  const totals = useMemo(() => summarizeDraft(draft), [draft]);
  const allocations = useMemo(() => allocatedInventory(draft), [draft]);
  const inventoryIssues = useMemo(
    () => findInventoryIssues(allocations, inventory),
    [allocations, inventory]
  );
  const canSave = draft.name.trim().length > 0
    && totals.troops > 0
    && (inventoryPolicy === 'advisory' || inventoryIssues.length === 0);
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

  const selectWave = (waveIndex: number) => {
    setActiveWaveIndex(waveIndex);
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
    setActiveWaveIndex(insertAt);
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
        <ModalTitle
          icon={<Swords className="h-5 w-5" />}
          description={(
            <span className="flex flex-wrap items-center gap-2">
              <span>{inventoryLabel}</span>
              <Badge variant={isMetadataLoading ? 'secondary' : inventoryPolicy === 'advisory' ? 'outline' : hasInventory ? 'success' : 'warning'} className="normal-case tracking-normal">
                {isMetadataLoading
                  ? 'Loading inventory'
                  : inventoryPolicy === 'advisory'
                    ? 'Full catalog · stock advisory'
                    : hasInventory ? 'All-castles inventory' : 'Inventory unavailable'}
              </Badge>
            </span>
          )}
        >
          Attack preset
        </ModalTitle>
      }
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
          <div className="text-xs text-text-muted">
            {inventoryIssues.length > 0 ? (
              <span className={`font-semibold ${inventoryPolicy === 'advisory' ? 'text-warning' : 'text-error'}`}>
                {inventoryIssues.length} stock conflict{inventoryIssues.length === 1 ? '' : 's'}
                {inventoryPolicy === 'advisory' ? ' will be checked at launch' : ' must be resolved'}
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
            <MetricTile size="sm" className="min-w-[4.75rem]" label="Waves" value={draft.waves.length.toLocaleString()} />
            <MetricTile size="sm" className="min-w-[4.75rem]" label="Troops" value={totals.troops.toLocaleString()} />
            <MetricTile size="sm" className="min-w-[4.75rem]" label="Tools" value={totals.tools.toLocaleString()} />
          </div>
        </section>

        <section className="flex flex-wrap items-center justify-between gap-3 rounded-global border border-border-base bg-bg-card/55 p-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl">
          <div className="min-w-0 flex-1 overflow-x-auto custom-scrollbar">
            <PillSelector
              ariaLabel="Attack wave"
              value={String(activeWaveIndex)}
              onChange={(value) => selectWave(Number(value))}
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

        <WaveEditorCard
          key={activeWaveIndex}
          waveIndex={activeWaveIndex}
          wave={activeWave}
          activeFormationKind={activeFormationKind}
          troopItems={troopItems}
          toolItems={toolItems}
          troopStock={inventory.troops}
          toolStock={inventory.tools}
          troopAllocations={allocations.troops}
          toolAllocations={allocations.tools}
          onChangeFormationKind={setActiveFormationKind}
          onChangeLane={(laneKey, lane) => {
            updateWaveAt(activeWaveIndex, (currentWave) => ({ ...currentWave, [laneKey]: lane }));
          }}
          onPickTroop={async (laneKey, slot, slotIndex) => {
            const result = await showTroopPicker({
              mode: 'single',
              title: `Choose a troop for Wave ${activeWaveIndex + 1} · ${laneLabel(laneKey)}`,
              preselected: slot.itemId == null ? [] : [slot.itemId],
              allowedUnitIds: troopIDs,
              stockQuantities: inventory.troops,
            });
            if (typeof result !== 'number') return;
            updateWaveAt(activeWaveIndex, (currentWave) => ({
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
              title: `Choose a tool for Wave ${activeWaveIndex + 1} · ${laneLabel(laneKey)}`,
              preselected: slot.itemId == null ? [] : [slot.itemId],
              allowedToolIds: toolIDs,
              stockQuantities: inventory.tools,
            });
            if (typeof result !== 'number') return;
            updateWaveAt(activeWaveIndex, (currentWave) => ({
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

        {inventoryIssues.length > 0 ? (
          <section className={`rounded-global border p-3 text-sm ${inventoryPolicy === 'advisory' ? 'border-warning/30 bg-warning/8 text-warning' : 'border-error/30 bg-error/8 text-error'}`}>
            <div className="mb-2 font-black">
              {inventoryPolicy === 'advisory' ? 'Current account inventory is lower than this preset' : 'Preset exceeds available inventory'}
            </div>
            {inventoryPolicy === 'advisory' ? (
              <p className="mb-2 text-xs font-medium text-text-muted">The preset can still be saved. Live inventory will be validated before an attack launches.</p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              {inventoryIssues.map((issue) => {
                const meta = issue.kind === 'troop' ? troops[issue.itemId] : tools[issue.itemId];
                return (
                  <span key={`${issue.kind}-${issue.itemId}`} className="rounded-full border border-current/25 bg-bg-card/45 px-3 py-1.5 text-xs font-semibold">
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

interface WaveEditorCardProps {
  waveIndex: number;
  wave: AttackSetupWave;
  activeFormationKind: InventoryKind;
  troopItems: InventoryItem[];
  toolItems: InventoryItem[];
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
  troopAllocations: Record<number, number>;
  toolAllocations: Record<number, number>;
  onChangeFormationKind: (kind: InventoryKind) => void;
  onChangeLane: (laneKey: LaneKey, lane: AttackSetupLane) => void;
  onPickTroop: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
  onPickTool: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
}

const WaveEditorCard: React.FC<WaveEditorCardProps> = ({
  waveIndex,
  wave,
  activeFormationKind,
  troopItems,
  toolItems,
  troopStock,
  toolStock,
  troopAllocations,
  toolAllocations,
  onChangeFormationKind,
  onChangeLane,
  onPickTroop,
  onPickTool,
}) => {
  const waveTotals = summarizeWave(wave);
  return (
    <Card
      variant="solid"
      className="liquid-prominent-header-card ring-1 ring-primary/30"
    >
      <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-global border border-primary/40 bg-primary/12 text-primary shadow-glow">
            <Swords className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="m-0 text-base font-black text-text-main">Wave {waveIndex + 1}</h3>
              <Badge variant="primary" className="normal-case tracking-normal">Editing</Badge>
            </div>
            <p className="mt-1 text-xs text-text-muted">Three fronts · 10 unit slots · 7 tool slots</p>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2">
          <PillSelector
            ariaLabel="Formation item type"
            value={activeFormationKind}
            onChange={(value) => onChangeFormationKind(value as InventoryKind)}
            options={[
              { value: 'troop', label: 'Units · 10' },
              { value: 'tool', label: 'Tools · 7' },
            ]}
            size="sm"
          />
          <MetricTile size="sm" className="min-w-[4.75rem]" label="Troops" value={waveTotals.troops.toLocaleString()} />
          <MetricTile size="sm" className="min-w-[4.75rem]" label="Tools" value={waveTotals.tools.toLocaleString()} />
        </div>
      </CardHeader>

      <CardContent className="liquid-prominent-header-content p-3">
        {activeFormationKind === 'troop' ? (
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
        ) : (
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
        )}
      </CardContent>
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

const FormationRow: React.FC<FormationRowProps> = ({ kind, label, wave, items, stock, allocations, onChangeLane, onPick }) => {
  const slotKey = kind === 'troop' ? 'troops' : 'tools';
  const laneTemplate = laneKeys.map((laneKey) => `${wave[laneKey][slotKey].length}fr`).join(' ');

  return (
    <section className="overflow-hidden rounded-global border border-border-base bg-bg-app/42" aria-label={`${label} formation`}>
      <div className="overflow-x-auto p-3 custom-scrollbar">
        <div
          className={`mx-auto grid items-stretch gap-3 ${kind === 'troop' ? 'min-w-[64rem]' : 'min-w-[48rem]'}`}
          style={{ gridTemplateColumns: laneTemplate }}
        >
          {laneKeys.map((laneKey) => {
            const lane = wave[laneKey];
            const slots = lane[slotKey];
            return (
              <div key={laneKey} className={`min-w-0 rounded-global border px-3 pb-3 pt-2.5 ${laneKey === 'M' ? 'border-primary/25 bg-primary/5' : 'border-border-base bg-bg-card/35'}`}>
                <div className="mb-2.5 flex items-center justify-between gap-2">
                  <span className={`text-[10px] font-black uppercase tracking-wider ${laneKey === 'M' ? 'text-primary' : 'text-text-muted'}`}>
                    {laneLabel(laneKey)}
                  </span>
                  <span className="font-mono text-[9px] font-bold text-text-muted">
                    {slots.length} slot{slots.length === 1 ? '' : 's'}
                  </span>
                </div>
                <div className="grid grid-flow-col auto-cols-fr items-stretch gap-2">
                  {slots.map((slot, slotIndex) => (
                    <InventorySlotCard
                      key={slotIndex}
                      kind={kind}
                      laneKey={laneKey}
                      index={slotIndex}
                      slot={slot}
                      items={items}
                      stock={stock}
                      allocated={slot.itemId == null ? 0 : allocations[slot.itemId] ?? 0}
                      onChange={(patch) => {
                        const updatedSlots = updateSlot(slots, slotIndex, patch);
                        onChangeLane(laneKey, {
                          ...lane,
                          [slotKey]: updatedSlots,
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
};

interface InventorySlotCardProps {
  kind: InventoryKind;
  laneKey: LaneKey;
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
  laneKey,
  index,
  slot,
  items,
  stock,
  allocated,
  onChange,
  onPick,
}) => {
  const selected = slot.itemId == null ? undefined : items.find((item) => item.id === slot.itemId);
  const hasItem = slot.itemId != null;
  const itemKindLabel = kind === 'troop' ? 'unit' : 'tool';
  const itemName = selected?.name || (hasItem ? `${itemKindLabel} #${slot.itemId}` : '');
  const available = slot.itemId == null ? 0 : stock[slot.itemId] ?? 0;
  const remainingAfterPreset = available - allocated;
  const overAllocated = hasItem && allocated > available;
  const slotLabel = `${laneLabel(laneKey)} ${itemKindLabel} slot ${index + 1}`;
  const pickerDisabled = items.length === 0;

  return (
    <div
      className={`group relative flex min-h-[10.75rem] w-full min-w-0 flex-col overflow-hidden rounded-[0.95rem] border p-2 transition-all hover:-translate-y-0.5 hover:shadow-[var(--glass-shadow-compact)] ${
        overAllocated
          ? 'border-error/45 bg-error/7'
          : hasItem
            ? 'border-primary/45 bg-primary/8 shadow-[0_0_16px_color-mix(in_srgb,var(--primary)_14%,transparent)]'
            : 'border-border-base bg-bg-card/60 hover:border-primary/30'
      }`}
    >
      <div className="flex h-3.5 items-center justify-between gap-1 text-[9px] font-black uppercase text-text-muted">
        <span className={hasItem ? 'text-primary' : ''}>{kind === 'troop' ? 'U' : 'T'}{index + 1}</span>
        {hasItem ? (
          <span className="flex min-w-0 items-center gap-0.5">
            <span className="max-w-[2.75rem] truncate font-mono">#{slot.itemId}</span>
            <button
              type="button"
              onClick={() => onChange({ itemId: null, quantity: 0 })}
              className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-text-muted transition hover:bg-error/10 hover:text-error"
              title={`Clear ${slotLabel}`}
              aria-label={`Clear ${slotLabel}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ) : (
          <span>Empty</span>
        )}
      </div>

      <button
        type="button"
        onClick={onPick}
        disabled={pickerDisabled}
        className={`mt-1.5 flex h-[4.25rem] w-full min-w-0 flex-col items-center justify-center gap-1 rounded-lg border px-1.5 text-center transition focus:outline-none focus:ring-2 focus:ring-primary/45 disabled:cursor-not-allowed disabled:opacity-40 ${hasItem ? 'border-primary/25 bg-bg-app/65 hover:border-primary/55 hover:bg-primary/8' : 'border-dashed border-border-base bg-bg-input/45 text-text-muted hover:border-primary/40 hover:text-primary'}`}
        title={pickerDisabled ? `No available ${itemKindLabel}s in this inventory` : `${hasItem ? 'Change' : 'Choose'} ${itemKindLabel}`}
        aria-label={`${hasItem ? 'Change' : 'Choose'} ${slotLabel}`}
      >
        {hasItem ? (
          <>
            {kind === 'troop' ? (
              <UnitImage unitId={slot.itemId} size={36} showLevel />
            ) : (
              <ToolImage toolId={slot.itemId} size={36} />
            )}
            <span className="line-clamp-2 w-full text-[10px] font-bold leading-[1.05] text-text-main" title={itemName}>
              {itemName}
            </span>
          </>
        ) : (
          <>
            <MousePointerClick className="h-4 w-4" />
            <span className="text-[10px] font-bold">Choose {itemKindLabel}</span>
          </>
        )}
      </button>

      <label className="mt-2 block">
        <span className="mb-1 block text-[9px] font-black uppercase tracking-wider text-text-muted">Amount</span>
        <input
          type="number"
          min={0}
          max={available || undefined}
          value={slot.quantity || ''}
          onChange={(event) => onChange({ quantity: positiveInteger(event.target.value) })}
          placeholder="0"
          disabled={!hasItem}
          className="h-8 w-full min-w-0 rounded-md border border-border-base bg-bg-input/65 px-1.5 text-center font-mono text-xs font-black text-text-main outline-none transition focus:border-primary focus:ring-1 focus:ring-primary disabled:cursor-not-allowed disabled:opacity-45"
          aria-label={`${slotLabel} amount`}
        />
      </label>

      <div className={`mt-1.5 truncate text-center font-mono text-[9px] leading-none ${overAllocated ? 'text-error' : 'text-text-muted'}`}>
        {hasItem
          ? overAllocated
            ? `${allocated.toLocaleString()}/${available.toLocaleString()} used`
            : `${Math.max(0, remainingAfterPreset).toLocaleString()} left`
          : 'No item selected'}
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

function inventoryItems(stock: Record<number, number>, metadata: Record<number, MetadataItem>, includeUnowned: boolean): InventoryItem[] {
  const entries = includeUnowned
    ? Object.keys(metadata).map((rawID) => [rawID, stock[Number(rawID)] ?? 0] as const)
    : Object.entries(stock);
  return entries
    .map(([rawID, count]) => {
      const id = Number(rawID);
      const itemMetadata = metadata[id];
      if (!itemMetadata) return null;
      return { id, name: itemMetadata.name || `Item ${id}`, stock: count, metadata: itemMetadata };
    })
    .filter((item): item is InventoryItem => item != null)
    .sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id);
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
