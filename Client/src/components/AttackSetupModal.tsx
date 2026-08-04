import React, { useEffect, useId, useMemo, useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  Copy,
  Eraser,
  Minus,
  Plus,
  Shield,
  Swords,
} from 'lucide-react';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { useCitadelAPI } from '../api/ApiContext';
import type { CastleStateV2 } from '../api/Contracts';
import ToolImage from './ToolImage';
import { showToolPicker } from './ToolPickerModal';
import UnitImage from './UnitImage';
import { showTroopPicker } from './TroopPickerModal';
import { AddSlot, Badge, Button, Card, CardContent, CardHeader, Input, MetricTile, Modal, ModalTitle, PillSelector, QuantityAssetTile } from './ui';

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

export interface AttackSetupCourtyardSupport {
  troops: AttackSetupSlot[];
  tools: AttackSetupSlot[];
}

export interface AttackSetupDraft {
  name: string;
  waves: AttackSetupWave[];
  courtyardSupport: AttackSetupCourtyardSupport;
}

/** Optional inventory scope for callers that need something other than the default all-castles aggregate. */
export interface AttackSetupInventory {
  label?: string;
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
}

export interface AttackSetupToolLimits {
  L: number;
  M: number;
  R: number;
}

export interface AttackSetupModalProps {
  isOpen: boolean;
  initialDraft?: AttackSetupDraft;
  inventory?: AttackSetupInventory;
  inventoryPolicy?: 'enforced' | 'advisory';
  targetType?: 'pve' | 'pvp';
  toolLimits?: AttackSetupToolLimits;
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

interface ToolLimitIssue {
  waveIndex: number;
  laneKey: LaneKey;
  requested: number;
  limit: number;
}

const laneKeys: LaneKey[] = ['L', 'M', 'R'];
const MAX_WAVES = 30;
const COURTYARD_TROOP_SLOTS = 8;
const COURTYARD_TOOL_SLOTS = 3;

const AttackSetupModal: React.FC<AttackSetupModalProps> = ({
  isOpen,
  initialDraft,
  inventory: inventoryOverride,
  inventoryPolicy = 'enforced',
  targetType,
  toolLimits,
  onClose,
  onSave,
}) => {
  const { state } = useCitadelAPI();
  const { troops, tools, isLoading: isMetadataLoading } = useMetadata();
  const [draft, setDraft] = useState<AttackSetupDraft>(() => normalizeDraft(initialDraft));
  const [activeSupportKind, setActiveSupportKind] = useState<InventoryKind>('troop');

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
  const allToolItems = useMemo(
    () => inventoryItems(inventory.tools, tools, inventoryPolicy === 'advisory'),
    [inventory.tools, inventoryPolicy, tools]
  );
  const toolItems = useMemo(
    () => allToolItems.filter((item) => !isSceatAttackSupportTool(item.metadata)),
    [allToolItems]
  );
  const supportToolItems = useMemo(
    () => allToolItems.filter((item) => isSceatAttackSupportTool(item.metadata)),
    [allToolItems]
  );
  const troopIDs = useMemo(() => troopItems.map((item) => item.id), [troopItems]);
  const toolIDs = useMemo(() => toolItems.map((item) => item.id), [toolItems]);
  const supportToolIDs = useMemo(() => supportToolItems.map((item) => item.id), [supportToolItems]);

  useEffect(() => {
    if (!isOpen) return;
    setDraft(normalizeDraft(initialDraft));
    setActiveSupportKind('troop');
  }, [initialDraft, isOpen]);

  const totals = useMemo(() => summarizeDraft(draft), [draft]);
  const allocations = useMemo(() => allocatedInventory(draft), [draft]);
  const inventoryIssues = useMemo(
    () => findInventoryIssues(allocations, inventory),
    [allocations, inventory]
  );
  const toolLimitIssues = useMemo(
    () => findToolLimitIssues(draft, toolLimits),
    [draft, toolLimits]
  );
  const canSave = draft.name.trim().length > 0
    && totals.formationTroops > 0
    && toolLimitIssues.length === 0
    && (inventoryPolicy === 'advisory' || inventoryIssues.length === 0);

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
  };

  const updateWaveAt = (waveIndex: number, updater: (wave: AttackSetupWave) => AttackSetupWave) => {
    setDraft((current) => ({
      ...current,
      waves: current.waves.map((wave, index) => (index === waveIndex ? updater(wave) : wave)),
    }));
  };

  const duplicateWave = (waveIndex: number) => {
    setDraft((current) => {
      const wave = current.waves[waveIndex];
      if (!wave || current.waves.length >= MAX_WAVES) return current;
      const insertAt = waveIndex + 1;
      return {
        ...current,
        waves: [
          ...current.waves.slice(0, insertAt),
          cloneWave(wave),
          ...current.waves.slice(insertAt),
        ],
      };
    });
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
              {targetType ? (
                <Badge variant={targetType === 'pvp' ? 'primary' : 'success'} className="normal-case tracking-normal">
                  {targetType === 'pvp' ? 'PvP preset' : 'PvE preset'}
                </Badge>
              ) : null}
            </span>
          )}
        >
          Attack preset
        </ModalTitle>
      }
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
          <div className="text-xs text-text-muted">
            {toolLimitIssues.length > 0 ? (
              <span className="font-semibold text-error">
                {toolLimitIssues.length} tool section limit{toolLimitIssues.length === 1 ? '' : 's'} must be resolved
              </span>
            ) : inventoryIssues.length > 0 ? (
              <span className={`font-semibold ${inventoryPolicy === 'advisory' ? 'text-warning' : 'text-error'}`}>
                {inventoryIssues.length} stock conflict{inventoryIssues.length === 1 ? '' : 's'}
                {inventoryPolicy === 'advisory' ? ' will be checked at launch' : ' must be resolved'}
              </span>
            ) : totals.formationTroops === 0 ? (
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
        {toolLimits ? (
          <section className="flex flex-wrap items-center justify-between gap-3 rounded-global border border-primary/25 bg-primary/8 px-4 py-3">
            <div>
              <div className="text-sm font-black text-text-main">
                {targetType === 'pvp' ? 'PvP tool limits' : 'PvE tool limits'}
              </div>
              <p className="mt-0.5 text-xs text-text-muted">
                Each wave is checked independently. The server checks the actual target again before CRA is sent.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline" className="normal-case tracking-normal">Left {toolLimits.L}</Badge>
              <Badge variant="outline" className="normal-case tracking-normal">Center {toolLimits.M}</Badge>
              <Badge variant="outline" className="normal-case tracking-normal">Right {toolLimits.R}</Badge>
            </div>
          </section>
        ) : null}

        <section className="grid gap-3 rounded-global border border-border-base bg-bg-card/65 p-3 shadow-[var(--shadow-raised)] lg:grid-cols-[minmax(15rem,1.4fr)_auto_auto] lg:items-end">
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

        <section className="flex flex-col gap-4" aria-label="Attack waves">
          {draft.waves.map((wave, waveIndex) => (
            <WaveEditorCard
              key={waveIndex}
              waveIndex={waveIndex}
              waveCount={draft.waves.length}
              wave={wave}
              troopItems={troopItems}
              toolItems={toolItems}
              troopStock={inventory.troops}
              toolStock={inventory.tools}
              troopAllocations={allocations.troops}
              toolAllocations={allocations.tools}
              toolLimits={toolLimits}
              onDuplicate={() => duplicateWave(waveIndex)}
              onClear={() => updateWaveAt(waveIndex, () => emptyWave())}
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
                updateWaveAt(waveIndex, (currentWave) => {
                  const laneTools = currentWave[laneKey].tools;
                  const otherTools = laneTools.reduce(
                    (total, currentSlot, currentIndex) => currentIndex === slotIndex
                      ? total
                      : total + currentSlot.quantity,
                    0,
                  );
                  const maximum = toolLimits == null
                    ? Number.MAX_SAFE_INTEGER
                    : Math.max(0, toolLimits[laneKey] - otherTools);
                  return {
                    ...currentWave,
                    [laneKey]: {
                      ...currentWave[laneKey],
                      tools: updateSlot(laneTools, slotIndex, {
                        itemId: result,
                        quantity: Math.min(slot.quantity > 0 ? slot.quantity : 1, maximum),
                      }),
                    },
                  };
                });
              }}
            />
          ))}
        </section>

        <CourtyardSupportCard
          support={draft.courtyardSupport}
          activeKind={activeSupportKind}
          troopItems={troopItems}
          toolItems={supportToolItems}
          troopStock={inventory.troops}
          toolStock={inventory.tools}
          troopAllocations={allocations.troops}
          toolAllocations={allocations.tools}
          onChangeKind={setActiveSupportKind}
          onChange={(courtyardSupport) => setDraft((current) => ({ ...current, courtyardSupport }))}
          onPickTroop={async (slot, slotIndex) => {
            const result = await showTroopPicker({
              mode: 'single',
              title: `Choose courtyard support troop ${slotIndex + 1}`,
              preselected: slot.itemId == null ? [] : [slot.itemId],
              allowedUnitIds: troopIDs,
              stockQuantities: inventory.troops,
            });
            if (typeof result !== 'number') return;
            setDraft((current) => ({
              ...current,
              courtyardSupport: {
                ...current.courtyardSupport,
                troops: updateSlot(current.courtyardSupport.troops, slotIndex, {
                  itemId: result,
                  quantity: slot.quantity > 0 ? slot.quantity : 1,
                }),
              },
            }));
          }}
          onPickTool={async (slot, slotIndex) => {
            const result = await showToolPicker({
              mode: 'single',
              title: `Choose Sceat support tool ${slotIndex + 1}`,
              preselected: slot.itemId == null ? [] : [slot.itemId],
              allowedToolIds: supportToolIDs,
              stockQuantities: inventory.tools,
            });
            if (typeof result !== 'number') return;
            setDraft((current) => ({
              ...current,
              courtyardSupport: {
                ...current.courtyardSupport,
                tools: updateSlot(current.courtyardSupport.tools, slotIndex, {
                  itemId: result,
                  quantity: 1,
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

        {toolLimitIssues.length > 0 ? (
          <section className="rounded-global border border-error/30 bg-error/8 p-3 text-sm text-error">
            <div className="mb-2 font-black">Preset exceeds the selected target type’s tool limits</div>
            <p className="mb-2 text-xs font-medium text-text-muted">
              Reduce tools in each listed section before saving. Limits apply separately to every wave.
            </p>
            <div className="flex flex-wrap gap-2">
              {toolLimitIssues.slice(0, 12).map((issue) => (
                <span
                  key={`${issue.waveIndex}-${issue.laneKey}`}
                  className="rounded-full border border-current/25 bg-bg-card/45 px-3 py-1.5 text-xs font-semibold"
                >
                  Wave {issue.waveIndex + 1} · {laneLabel(issue.laneKey)}: {issue.requested} / {issue.limit}
                </span>
              ))}
              {toolLimitIssues.length > 12 ? (
                <span className="rounded-full border border-current/25 bg-bg-card/45 px-3 py-1.5 text-xs font-semibold">
                  +{toolLimitIssues.length - 12} more sections
                </span>
              ) : null}
            </div>
          </section>
        ) : null}
      </div>
    </Modal>
  );
};

interface WaveEditorCardProps {
  waveIndex: number;
  waveCount: number;
  wave: AttackSetupWave;
  troopItems: InventoryItem[];
  toolItems: InventoryItem[];
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
  troopAllocations: Record<number, number>;
  toolAllocations: Record<number, number>;
  toolLimits?: AttackSetupToolLimits;
  onDuplicate: () => void;
  onClear: () => void;
  onChangeLane: (laneKey: LaneKey, lane: AttackSetupLane) => void;
  onPickTroop: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
  onPickTool: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
}

const WaveEditorCard: React.FC<WaveEditorCardProps> = ({
  waveIndex,
  waveCount,
  wave,
  troopItems,
  toolItems,
  troopStock,
  toolStock,
  troopAllocations,
  toolAllocations,
  toolLimits,
  onDuplicate,
  onClear,
  onChangeLane,
  onPickTroop,
  onPickTool,
}) => {
  const waveTotals = summarizeWave(wave);
  const [isOpen, setIsOpen] = useState(true);
  const contentId = useId();

  return (
    <Card
      variant="solid"
      className="liquid-prominent-header-card"
    >
      <CardHeader className="liquid-card-header-prominent !m-0 !min-h-0 !rounded-full !p-0">
        <div className="flex h-11 w-full items-center gap-1 overflow-hidden rounded-full px-1.5">
          <button
            type="button"
            className="flex h-full min-w-0 flex-1 items-center justify-between gap-3 rounded-full px-2 text-left transition-colors hover:text-primary"
            aria-expanded={isOpen}
            aria-controls={contentId}
            onClick={() => setIsOpen((current) => !current)}
          >
            <div className="flex min-w-0 items-center gap-2">
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-primary/40 bg-primary/12 text-primary shadow-glow">
                <Swords className="h-4 w-4" />
              </span>
              <h3 className="m-0 whitespace-nowrap text-sm font-black text-text-main">Wave {waveIndex + 1}</h3>
              <Badge variant="primary" className="shrink-0 normal-case tracking-normal">{waveIndex + 1} of {waveCount}</Badge>
            </div>

            <div className="flex shrink-0 items-center justify-end gap-1.5">
              <span className="rounded-full border border-border-base bg-bg-input/45 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-text-muted">
                Troops <strong className="ml-1 font-mono text-xs text-text-main">{waveTotals.troops.toLocaleString()}</strong>
              </span>
              <span className="rounded-full border border-border-base bg-bg-input/45 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-text-muted">
                Tools <strong className="ml-1 font-mono text-xs text-text-main">{waveTotals.tools.toLocaleString()}</strong>
              </span>
              {isOpen ? (
                <ChevronDown className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
              ) : (
                <ChevronRight className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
              )}
            </div>
          </button>
          <span className="h-5 w-px shrink-0 bg-border-base/80" aria-hidden="true" />
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 rounded-full !p-0"
            onClick={onDuplicate}
            disabled={waveCount >= MAX_WAVES}
            title="Duplicate wave"
            aria-label={`Duplicate Wave ${waveIndex + 1}`}
          >
            <Copy className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 rounded-full !p-0 hover:!text-error"
            onClick={onClear}
            title="Clear wave"
            aria-label={`Clear Wave ${waveIndex + 1}`}
          >
            <Eraser className="h-3.5 w-3.5" />
          </Button>
        </div>
      </CardHeader>

      {isOpen ? (
        <CardContent id={contentId} className="liquid-prominent-header-content !px-1 !pb-2 !pt-3">
          <div className="grid gap-1">
            <FormationRow
              kind="tool"
              label="Tools"
              wave={wave}
              items={toolItems}
              stock={toolStock}
              allocations={toolAllocations}
              laneLimits={toolLimits}
              onChangeLane={onChangeLane}
              onPick={onPickTool}
            />
            <FormationRow
              kind="troop"
              label="Troops"
              divided
              wave={wave}
              items={troopItems}
              stock={troopStock}
              allocations={troopAllocations}
              onChangeLane={onChangeLane}
              onPick={onPickTroop}
            />
          </div>
        </CardContent>
      ) : null}
    </Card>
  );
};

interface CourtyardSupportCardProps {
  support: AttackSetupCourtyardSupport;
  activeKind: InventoryKind;
  troopItems: InventoryItem[];
  toolItems: InventoryItem[];
  troopStock: Record<number, number>;
  toolStock: Record<number, number>;
  troopAllocations: Record<number, number>;
  toolAllocations: Record<number, number>;
  onChangeKind: (kind: InventoryKind) => void;
  onChange: (support: AttackSetupCourtyardSupport) => void;
  onPickTroop: (slot: AttackSetupSlot, slotIndex: number) => void;
  onPickTool: (slot: AttackSetupSlot, slotIndex: number) => void;
}

const CourtyardSupportCard: React.FC<CourtyardSupportCardProps> = ({
  support,
  activeKind,
  troopItems,
  toolItems,
  troopStock,
  toolStock,
  troopAllocations,
  toolAllocations,
  onChangeKind,
  onChange,
  onPickTroop,
  onPickTool,
}) => {
  const kindIsTroop = activeKind === 'troop';
  const slots = kindIsTroop ? support.troops : support.tools;
  const items = kindIsTroop ? troopItems : toolItems;
  const stock = kindIsTroop ? troopStock : toolStock;
  const allocations = kindIsTroop ? troopAllocations : toolAllocations;
  const troopTotal = support.troops.reduce((total, slot) => total + slot.quantity, 0);
  const toolTotal = support.tools.filter((slot) => slot.itemId != null).length;

  return (
    <Card variant="solid" className="liquid-prominent-header-card ring-1 ring-warning/25">
      <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-global border border-warning/40 bg-warning/12 text-warning">
            <Shield className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="m-0 text-base font-black text-text-main">Courtyard support wave</h3>
              <Badge variant="warning" className="normal-case tracking-normal">Optional</Badge>
            </div>
            <p className="mt-1 text-xs text-text-muted">
              Add up to {COURTYARD_TROOP_SLOTS} extra troops and {COURTYARD_TOOL_SLOTS} one-use Sceat support tools.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2">
          <PillSelector
            ariaLabel="Courtyard support item type"
            value={activeKind}
            onChange={(value) => onChangeKind(value as InventoryKind)}
            options={[
              { value: 'troop', label: `Units · ${COURTYARD_TROOP_SLOTS}` },
              { value: 'tool', label: `Sceat tools · ${COURTYARD_TOOL_SLOTS}` },
            ]}
            size="header"
          />
          <MetricTile size="sm" className="min-w-[4.75rem]" label="Troops" value={troopTotal.toLocaleString()} />
          <MetricTile size="sm" className="min-w-[4.75rem]" label="Tools" value={toolTotal.toLocaleString()} />
        </div>
      </CardHeader>

      <CardContent className="liquid-prominent-header-content p-3">
        <section className="overflow-hidden rounded-global border border-border-base bg-bg-app/42" aria-label="Courtyard support formation">
          <div className="overflow-x-auto p-3 custom-scrollbar">
            <div className={`mx-auto flex w-max items-start justify-center gap-2 ${kindIsTroop ? 'min-w-[46rem]' : 'min-w-[18rem]'}`}>
              {slots.map((slot, slotIndex) => (
                <InventorySlotCard
                  key={slotIndex}
                  kind={activeKind}
                  index={slotIndex}
                  slot={slot}
                  items={items}
                  stock={stock}
                  allocated={slot.itemId == null ? 0 : allocations[slot.itemId] ?? 0}
                  slotCodePrefix={kindIsTroop ? 'RW' : 'AST'}
                  slotContext={kindIsTroop ? 'courtyard troop' : 'Sceat support tool'}
                  fixedQuantity={kindIsTroop ? undefined : 1}
                  onChange={(patch) => {
                    const slotKey = kindIsTroop ? 'troops' : 'tools';
                    onChange({
                      ...support,
                      [slotKey]: updateSlot(slots, slotIndex, patch),
                    });
                  }}
                  onPick={() => kindIsTroop ? onPickTroop(slot, slotIndex) : onPickTool(slot, slotIndex)}
                />
              ))}
            </div>
          </div>
        </section>
        {!kindIsTroop && toolItems.length === 0 ? (
          <p className="mt-3 text-xs font-medium text-text-muted">
            No Sceat attack support tools are available in this inventory.
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
};

interface FormationRowProps {
  kind: InventoryKind;
  label: string;
  divided?: boolean;
  wave: AttackSetupWave;
  items: InventoryItem[];
  stock: Record<number, number>;
  allocations: Record<number, number>;
  laneLimits?: AttackSetupToolLimits;
  onChangeLane: (laneKey: LaneKey, lane: AttackSetupLane) => void;
  onPick: (laneKey: LaneKey, slot: AttackSetupSlot, slotIndex: number) => void;
}

const FormationRow: React.FC<FormationRowProps> = ({
  kind,
  label,
  divided = false,
  wave,
  items,
  stock,
  allocations,
  laneLimits,
  onChangeLane,
  onPick,
}) => {
  const slotKey = kind === 'troop' ? 'troops' : 'tools';
  const laneTemplate = laneKeys.map((laneKey) => `${wave[laneKey][slotKey].length}fr`).join(' ');

  return (
    <div
      className={`overflow-x-auto custom-scrollbar ${divided ? 'relative pt-2 before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-border-base/70' : ''}`}
      role="group"
      aria-label={`${label} formation`}
    >
      <div>
        <div
          className={`mx-auto grid items-stretch gap-3 ${kind === 'troop' ? 'min-w-[64rem]' : 'min-w-[48rem]'}`}
          style={{ gridTemplateColumns: laneTemplate }}
        >
          {laneKeys.map((laneKey) => {
            const lane = wave[laneKey];
            const slots = lane[slotKey];
            const filledSlots = slots.filter((slot) => slot.itemId != null).length;
            const laneTotal = slots.reduce((total, slot) => total + slot.quantity, 0);
            const laneLimit = kind === 'tool' ? laneLimits?.[laneKey] : undefined;
            const laneOverLimit = laneLimit != null && laneTotal > laneLimit;
            return (
              <div key={laneKey} className="min-w-0 px-2 py-1">
                <div className="mb-1 flex items-center justify-between gap-2">
                  <span className={`text-[10px] font-black uppercase tracking-wider ${laneKey === 'M' ? 'text-primary' : 'text-text-muted'}`}>
                    {laneLabel(laneKey)}
                  </span>
                  <span className={`font-mono text-[9px] font-bold ${laneOverLimit ? 'text-error' : 'text-text-muted'}`}>
                    {filledSlots}/{slots.length} filled
                    {laneLimit == null ? '' : ` · ${laneTotal}/${laneLimit}`}
                  </span>
                </div>
                <div className="flex items-start justify-center gap-2">
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
                      maxQuantity={laneLimit == null
                        ? undefined
                        : Math.max(0, laneLimit - (laneTotal - slot.quantity))}
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
    </div>
  );
};

interface InventorySlotCardProps {
  kind: InventoryKind;
  laneKey?: LaneKey;
  index: number;
  slot: AttackSetupSlot;
  items: InventoryItem[];
  stock: Record<number, number>;
  allocated: number;
  slotCodePrefix?: string;
  slotContext?: string;
  fixedQuantity?: number;
  maxQuantity?: number;
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
  slotCodePrefix,
  slotContext,
  fixedQuantity,
  maxQuantity,
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
  const slotLabel = `${slotContext ?? `${laneKey ? laneLabel(laneKey) : 'formation'} ${itemKindLabel}`} slot ${index + 1}`;
  const slotCode = `${slotCodePrefix ?? (kind === 'troop' ? 'U' : 'T')}${index + 1}`;
  const sectionLimitReached = !hasItem && maxQuantity === 0;
  const pickerDisabled = items.length === 0 || sectionLimitReached;

  return (
    <div className="flex w-[5.25rem] shrink-0 flex-col items-center">
      <div className="mb-1 flex w-full items-center justify-between gap-1 px-0.5 text-[9px] font-black uppercase text-text-muted">
        <span className={hasItem ? 'text-primary' : ''}>{slotCode}</span>
        <span className="max-w-[3.25rem] truncate font-mono">{hasItem ? `#${slot.itemId}` : 'Empty'}</span>
      </div>

      {hasItem ? (
        <>
          <QuantityAssetTile
            size={76}
            visual={(
              <button
                type="button"
                onClick={onPick}
                disabled={pickerDisabled}
                className={`flex h-full w-full items-center justify-center rounded-xl border bg-bg-input/55 transition focus:outline-none focus:ring-2 focus:ring-primary/45 disabled:cursor-not-allowed disabled:opacity-40 ${overAllocated ? 'border-error/55' : 'border-primary/35 hover:border-primary/70 hover:bg-primary/8'}`}
                title={pickerDisabled ? `No available ${itemKindLabel}s in this inventory` : `Change ${itemKindLabel}`}
                aria-label={`Change ${slotLabel}`}
              >
                {kind === 'troop' ? (
                  <UnitImage unitId={slot.itemId} size={68} showLevel className="rounded-xl" />
                ) : (
                  <ToolImage toolId={slot.itemId} size={68} className="rounded-xl" />
                )}
              </button>
            )}
            quantity={fixedQuantity == null ? (
              <input
                type="number"
                min={0}
                max={maxQuantity ?? (available || undefined)}
                value={slot.quantity || ''}
                onChange={(event) => onChange({
                  quantity: Math.min(positiveInteger(event.target.value), maxQuantity ?? Number.MAX_SAFE_INTEGER),
                })}
                onClick={(event) => event.stopPropagation()}
                placeholder="0"
                className="w-12 bg-transparent p-0 text-center font-mono text-[10px] font-black tabular-nums text-slate-900 outline-none"
                aria-label={`${slotLabel} amount`}
                title={`${slotLabel} amount`}
              />
            ) : (
              <span
                className="font-mono text-[10px] font-black tabular-nums text-slate-900"
                aria-label={`${slotLabel} amount ${fixedQuantity}`}
                title={`${slotLabel} uses one tool`}
              >
                ×{fixedQuantity}
              </span>
            )}
            onRemove={() => onChange({ itemId: null, quantity: 0 })}
            removeLabel={`Clear ${slotLabel}`}
          />
          <button
            type="button"
            onClick={onPick}
            disabled={pickerDisabled}
            className="mt-1.5 line-clamp-2 h-7 w-full text-center text-[10px] font-bold leading-[1.05] text-text-main transition hover:text-primary disabled:cursor-not-allowed"
            title={itemName}
          >
            {itemName}
          </button>
          <div className={`mt-1 truncate text-center font-mono text-[9px] leading-none ${overAllocated ? 'text-error' : 'text-text-muted'}`}>
            {overAllocated
              ? `${allocated.toLocaleString()}/${available.toLocaleString()} used`
              : `${Math.max(0, remainingAfterPreset).toLocaleString()} left`}
          </div>
        </>
      ) : (
        <>
          <AddSlot
            label={`Choose ${itemKindLabel}`}
            layout="stacked"
            onClick={onPick}
            disabled={pickerDisabled}
            className="h-[76px] w-[76px] shrink-0 px-1 text-[9px] disabled:cursor-not-allowed disabled:opacity-40"
            title={sectionLimitReached
              ? 'This section has reached its tool limit'
              : pickerDisabled ? `No available ${itemKindLabel}s in this inventory` : `Choose ${itemKindLabel}`}
            aria-label={`Choose ${slotLabel}`}
          />
          <span className="mt-1.5 flex h-7 items-center text-center text-[10px] font-bold leading-[1.05] text-text-muted">
            Empty {itemKindLabel} slot
          </span>
          <span className="mt-1 font-mono text-[9px] leading-none text-text-muted">Available</span>
        </>
      )}
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
  for (const slot of draft.courtyardSupport.troops) {
    if (slot.itemId != null && slot.quantity > 0) {
      allocations.troops[slot.itemId] = (allocations.troops[slot.itemId] ?? 0) + slot.quantity;
    }
  }
  for (const slot of draft.courtyardSupport.tools) {
    if (slot.itemId != null) {
      allocations.tools[slot.itemId] = (allocations.tools[slot.itemId] ?? 0) + 1;
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

function findToolLimitIssues(
  draft: AttackSetupDraft,
  limits?: AttackSetupToolLimits,
): ToolLimitIssue[] {
  if (!limits) return [];
  const issues: ToolLimitIssue[] = [];
  draft.waves.forEach((wave, waveIndex) => {
    laneKeys.forEach((laneKey) => {
      const requested = wave[laneKey].tools.reduce((total, slot) => total + slot.quantity, 0);
      const limit = limits[laneKey];
      if (requested > limit) issues.push({ waveIndex, laneKey, requested, limit });
    });
  });
  return issues;
}

function updateSlot(slots: AttackSetupSlot[], index: number, patch: Partial<AttackSetupSlot>): AttackSetupSlot[] {
  return slots.map((slot, slotIndex) => (slotIndex === index ? { ...slot, ...patch } : slot));
}

function positiveInteger(value: string | number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.trunc(parsed)) : 0;
}

function summarizeDraft(draft: AttackSetupDraft): {
  troops: number;
  tools: number;
  formationTroops: number;
  courtyardTroops: number;
  courtyardTools: number;
} {
  const formation = draft.waves.reduce((total, wave) => {
    const waveTotal = summarizeWave(wave);
    return { troops: total.troops + waveTotal.troops, tools: total.tools + waveTotal.tools };
  }, { troops: 0, tools: 0 });
  const courtyardTroops = draft.courtyardSupport.troops.reduce((total, slot) => total + slot.quantity, 0);
  const courtyardTools = draft.courtyardSupport.tools.filter((slot) => slot.itemId != null).length;
  return {
    troops: formation.troops + courtyardTroops,
    tools: formation.tools + courtyardTools,
    formationTroops: formation.troops,
    courtyardTroops,
    courtyardTools,
  };
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
  return {
    name: draft.name || 'New attack preset',
    waves,
    courtyardSupport: normalizeCourtyardSupport(draft.courtyardSupport),
  };
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

function normalizeCourtyardSupport(support?: AttackSetupCourtyardSupport): AttackSetupCourtyardSupport {
  return {
    troops: normalizeSlots(support?.troops, COURTYARD_TROOP_SLOTS),
    tools: Array.from({ length: COURTYARD_TOOL_SLOTS }, (_, index) => {
      const slot = support?.tools[index];
      return slot?.itemId == null ? emptySlot() : { itemId: slot.itemId, quantity: 1 };
    }),
  };
}

function defaultDraft(): AttackSetupDraft {
  return {
    name: 'New attack preset',
    waves: [emptyWave()],
    courtyardSupport: normalizeCourtyardSupport(),
  };
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

function isSceatAttackSupportTool(metadata: MetadataItem): boolean {
  return typeof metadata.type === 'string' && metadata.type.startsWith('SceatSuppAtt');
}

export default AttackSetupModal;
