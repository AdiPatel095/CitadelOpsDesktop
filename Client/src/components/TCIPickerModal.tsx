import React, { useState, useMemo, useEffect, useRef, useCallback } from 'react';
import { Search, Check, Minus, Plus } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Modal, Button, Input, ToggleGroup, Badge } from './ui';
import {
  fetchConstructionItemsCatalog,
  type ConstructionItemCatalogEntry,
  formatGroupTiersLine,
  levelRangeLabel,
} from './TCICatalogCache';

export const TCI_LEVEL_MIN = 1;
export const TCI_LEVEL_MAX = 4;

export function clampLevelCeiling(n: number): number {
  if (Number.isNaN(n)) {
    return TCI_LEVEL_MIN;
  }
  return Math.min(TCI_LEVEL_MAX, Math.max(TCI_LEVEL_MIN, Math.floor(n)));
}

export function clampLevelFloor(n: number): number {
  return clampLevelCeiling(n);
}

export function normalizeLevelRange(floor: number, ceiling: number): { floor: number; ceiling: number } {
  const f = clampLevelFloor(floor);
  const c = clampLevelCeiling(ceiling);
  return f <= c ? { floor: f, ceiling: c } : { floor: c, ceiling: c };
}

export type TCISelectionMode = 'single' | 'multi';

/** Picked construction item with allowed tier range (1–4). */
export interface TCIWithLevelCeiling {
  constructionItemId: number;
  levelCeiling: number;
  levelFloor: number;
}

/** @deprecated Use TCIWithLevelCeiling */
export type TCIWithQuantity = TCIWithLevelCeiling;

export interface TCIPickerOptions {
  mode: TCISelectionMode;
  title?: string;
  preselected?: number[];
  /** Per selected construction item id → level ceiling (1–4) */
  preselectedLevelCeilings?: Record<number, number>;
  /** Per selected construction item id → level floor (1–4, default 1) */
  preselectedLevelFloors?: Record<number, number>;
}

export type TCIPickerResult = TCIWithLevelCeiling | TCIWithLevelCeiling[] | null;

let resolvePickerPromise: ((value: TCIPickerResult) => void) | null = null;
let setPickerState: React.Dispatch<
  React.SetStateAction<{ isOpen: boolean; options: TCIPickerOptions | null }>
> | null = null;

export async function showTCIPicker(options: TCIPickerOptions): Promise<TCIPickerResult> {
  try {
    await fetchConstructionItemsCatalog();
  } catch {
    /* still open picker; grid may be empty */
  }
  return new Promise((resolve) => {
    resolvePickerPromise = resolve;
    if (setPickerState) {
      setPickerState({ isOpen: true, options });
    }
  });
}

interface TCIPickerModalProps {
  isOpen: boolean;
  options: TCIPickerOptions;
  catalog: ConstructionItemCatalogEntry[];
  onClose: (result: TCIPickerResult) => void;
}

export const TCIPickerProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [state, setState] = useState<{ isOpen: boolean; options: TCIPickerOptions | null }>({
    isOpen: false,
    options: null,
  });
  const [catalog, setCatalog] = useState<ConstructionItemCatalogEntry[]>([]);

  useEffect(() => {
    setPickerState = setState;
    return () => {
      setPickerState = null;
    };
  }, []);

  useEffect(() => {
    if (state.isOpen && state.options) {
      fetchConstructionItemsCatalog()
        .then(setCatalog)
        .catch(() => setCatalog([]));
    }
  }, [state.isOpen, state.options]);

  const handleClose = useCallback((result: TCIPickerResult) => {
    setState({ isOpen: false, options: null });
    if (resolvePickerPromise) {
      resolvePickerPromise(result);
      resolvePickerPromise = null;
    }
  }, []);

  return (
    <>
      {children}
      {state.isOpen && state.options && (
        <TCIPickerModal
          isOpen={state.isOpen}
          options={state.options}
          catalog={catalog}
          onClose={handleClose}
        />
      )}
    </>
  );
};

const TCIPickerModal: React.FC<TCIPickerModalProps> = ({ isOpen, options, catalog, onClose }) => {
  const { mode, title, preselected = [], preselectedLevelCeilings = {}, preselectedLevelFloors = {} } = options;

  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set(preselected));
  const [levelCeilings, setLevelCeilings] = useState<Record<number, number>>(() => {
    const m: Record<number, number> = {};
    preselected.forEach((id) => {
      const range = normalizeLevelRange(
        preselectedLevelFloors[id] ?? TCI_LEVEL_MIN,
        preselectedLevelCeilings[id] ?? TCI_LEVEL_MIN,
      );
      m[id] = range.ceiling;
    });
    return m;
  });
  const [levelFloors, setLevelFloors] = useState<Record<number, number>>(() => {
    const m: Record<number, number> = {};
    preselected.forEach((id) => {
      const range = normalizeLevelRange(
        preselectedLevelFloors[id] ?? TCI_LEVEL_MIN,
        preselectedLevelCeilings[id] ?? TCI_LEVEL_MIN,
      );
      m[id] = range.floor;
    });
    return m;
  });
  const [searchQuery, setSearchQuery] = useState('');
  const [categoryFilter, setCategoryFilter] = useState<string>('all');
  const [nameFilter, setNameFilter] = useState<string>('all');
  const [effectFilter, setEffectFilter] = useState<string>('all');

  const nameOptions = useMemo(() => {
    const s = new Set<string>();
    catalog.forEach((c) => {
      if (c.label) {
        s.add(c.label);
      }
    });
    return Array.from(s).sort((a, b) => a.localeCompare(b));
  }, [catalog]);

  const effectOptions = useMemo(() => {
    let rows = catalog;
    if (nameFilter !== 'all') {
      rows = rows.filter((c) => c.label === nameFilter);
    }
    const s = new Set<string>();
    rows.forEach((c) => {
      const fx = (c.effects || '').trim();
      s.add(fx || '(no listed effects)');
    });
    return Array.from(s).sort((a, b) => a.localeCompare(b));
  }, [catalog, nameFilter]);

  useEffect(() => {
    setEffectFilter('all');
  }, [nameFilter]);

  const categories = useMemo(() => {
    const s = new Set<string>();
    const isLimitedTime = (cat: string) => {
      const lower = cat.toLowerCase();
      const terms = [
        'sale', 'pack', 'test', 'offer', 'bundle', 'event', 'season',
        'january', 'february', 'march', 'april', 'may', 'june', 
        'july', 'august', 'september', 'october', 'november', 'december'
      ];
      return terms.some((term) => lower.includes(term));
    };

    catalog.forEach((c) => {
      if (c.category && !isLimitedTime(c.category)) {
        s.add(c.category);
      }
    });
    return Array.from(s).sort();
  }, [catalog]);

  const filtered = useMemo(() => {
    let rows = catalog;
    if (nameFilter !== 'all') {
      rows = rows.filter((c) => c.label === nameFilter);
    }
    if (effectFilter !== 'all') {
      rows = rows.filter((c) => {
        const fx = (c.effects || '').trim();
        const key = fx || '(no listed effects)';
        return key === effectFilter;
      });
    }
    if (categoryFilter !== 'all') {
      rows = rows.filter((c) => c.category === categoryFilter);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      rows = rows.filter((c) => {
        const tierLine = formatGroupTiersLine(c).toLowerCase();
        return (
          c.label.toLowerCase().includes(q) ||
          c.internal.toLowerCase().includes(q) ||
          c.effects.toLowerCase().includes(q) ||
          c.category.toLowerCase().includes(q) ||
          String(c.id).includes(q) ||
          c.level.toLowerCase().includes(q) ||
          tierLine.includes(q) ||
          c.groupTiers?.some(
            (t) => String(t.wireCid).includes(q) || String(t.level).includes(q)
          )
        );
      });
    }
    return rows;
  }, [catalog, nameFilter, effectFilter, categoryFilter, searchQuery]);

  const handleUnitClick = (id: number) => {
    if (mode === 'single') {
      setSelectedIds(new Set([id]));
      setLevelCeilings((prev) => {
        const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, prev[id] ?? TCI_LEVEL_MIN);
        setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
        return { ...prev, [id]: range.ceiling };
      });
      return;
    }
    const deselecting = selectedIds.has(id);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
    if (!deselecting) {
      setLevelCeilings((prev) => {
        const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, prev[id] ?? TCI_LEVEL_MIN);
        setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
        return { ...prev, [id]: range.ceiling };
      });
    }
  };

  const handleCeilingStep = (id: number, delta: number) => {
    setLevelCeilings((prev) => {
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, (prev[id] ?? TCI_LEVEL_MIN) + delta);
      setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
      return { ...prev, [id]: range.ceiling };
    });
  };

  const handleFloorStep = (id: number, delta: number) => {
    setLevelFloors((prev) => {
      const range = normalizeLevelRange((prev[id] ?? TCI_LEVEL_MIN) + delta, levelCeilings[id] ?? TCI_LEVEL_MIN);
      setLevelCeilings((ceilings) => ({ ...ceilings, [id]: range.ceiling }));
      return { ...prev, [id]: range.floor };
    });
  };

  const handleConfirm = () => {
    const selectedArray = Array.from(selectedIds);
    if (selectedArray.length === 0) {
      onClose(null);
      return;
    }
    if (mode === 'single') {
      const id = selectedArray[0];
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, levelCeilings[id] ?? TCI_LEVEL_MIN);
      onClose({
        constructionItemId: id,
        levelFloor: range.floor,
        levelCeiling: range.ceiling,
      });
      return;
    }
    const selected: TCIWithLevelCeiling[] = selectedArray.map((id) => {
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, levelCeilings[id] ?? TCI_LEVEL_MIN);
      return {
        constructionItemId: id,
        levelFloor: range.floor,
        levelCeiling: range.ceiling,
      };
    });
    onClose(selected);
  };

  const handleCancel = () => onClose(null);

  if (!isOpen) {
    return null;
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleCancel}
      maxWidth="5xl"
      title={
        <div className="flex items-center gap-3">
          <div className="w-2 h-6 rounded-full bg-amber-500 shadow-[0_0_10px_rgba(245,158,11,0.4)]" />
          <span className="text-xl">
            {title || (mode === 'single' ? 'Select construction item' : 'Select construction items')}
          </span>
          {mode === 'multi' && selectedIds.size > 0 && (
            <Badge variant="primary" className="ml-2">
              {selectedIds.size} selected
            </Badge>
          )}
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleCancel} className="px-8">
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleConfirm}
            disabled={selectedIds.size === 0}
            className="px-10"
            leftIcon={<Check className="w-4 h-4" />}
          >
            Confirm Selection
          </Button>
        </>
      }
    >
      <div className="mx-auto flex h-[calc(100vh-14rem)] w-full min-w-0 max-w-4xl flex-col overflow-hidden rounded-global border border-border-base bg-bg-app">
        <div className="shrink-0 space-y-3 border-b border-border-base bg-bg-card-hover/40 px-4 py-4 sm:px-5">
          <Input
            type="text"
            placeholder="Search by name, effect, category, or ID…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            leftIcon={<Search className="w-4 h-4" />}
          />
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="min-w-0 w-full">
              <div className="mb-1.5 text-[10px] font-bold uppercase tracking-wider text-text-muted">Name</div>
              <div className="relative -mx-1 px-1">
                <div className="flex overflow-x-auto overflow-y-hidden pb-2 pt-0.5 scroll-smooth custom-scrollbar">
                  <ToggleGroup
                    value={nameFilter}
                    onChange={(v) => setNameFilter(v)}
                    size="sm"
                    options={[
                      { value: 'all', label: 'All names', title: 'All construction item names' },
                      ...nameOptions.map((n) => ({ value: n, label: n, title: n })),
                    ]}
                    className="shrink-0 shadow-sm"
                  />
                </div>
              </div>
            </div>
            <div className="min-w-0 w-full">
              <div className="mb-1.5 text-[10px] font-bold uppercase tracking-wider text-text-muted">Effect</div>
              <div className="relative -mx-1 px-1">
                <div className="flex overflow-x-auto overflow-y-hidden pb-2 pt-0.5 scroll-smooth custom-scrollbar">
                  <ToggleGroup
                    value={effectFilter}
                    onChange={(v) => setEffectFilter(v)}
                    size="sm"
                    options={[
                      { value: 'all', label: 'All effects', title: 'All effect variants' },
                      ...effectOptions.map((fx) => ({
                        value: fx,
                        label: fx.length > 42 ? `${fx.slice(0, 39)}…` : fx,
                        title: fx,
                      })),
                    ]}
                    className="shrink-0 shadow-sm"
                  />
                </div>
              </div>
            </div>
          </div>
          <div className="min-w-0 w-full">
            <div className="mb-1.5 text-[10px] font-bold uppercase tracking-wider text-text-muted">Category</div>
            <div className="relative -mx-1 px-1">
              <div className="flex overflow-x-auto overflow-y-hidden pb-2 pt-0.5 scroll-smooth custom-scrollbar">
                <ToggleGroup
                  value={categoryFilter}
                  onChange={(v) => setCategoryFilter(v)}
                  size="sm"
                  options={[
                    { value: 'all', label: 'All', title: 'All categories' },
                    ...categories.map((c) => ({ value: c, label: c, title: c })),
                  ]}
                  className="shrink-0 shadow-sm"
                />
              </div>
            </div>
          </div>
          <p className="text-xs leading-relaxed text-text-muted">
            Same display name can have multiple effect variants (General&apos;s Camp style). Pick the name, then narrow by
            effect if needed. Set a <span className="font-semibold text-text-main">level floor and ceiling</span> (1–4) per
            variant when you only keep higher tiers in stash.
          </p>
        </div>
        <VirtualizedTCIList
          filtered={filtered}
          catalogCount={catalog.length}
          selectedIds={selectedIds}
          levelCeilings={levelCeilings}
          levelFloors={levelFloors}
          onRowClick={handleUnitClick}
          onCeilingStep={handleCeilingStep}
          onFloorStep={handleFloorStep}
        />
      </div>
    </Modal>
  );
};

interface VirtualizedTCIListProps {
  filtered: ConstructionItemCatalogEntry[];
  catalogCount: number;
  selectedIds: Set<number>;
  levelCeilings: Record<number, number>;
  levelFloors: Record<number, number>;
  onRowClick: (id: number) => void;
  onCeilingStep: (id: number, delta: number) => void;
  onFloorStep: (id: number, delta: number) => void;
}

const VirtualizedTCIList: React.FC<VirtualizedTCIListProps> = ({
  filtered,
  catalogCount,
  selectedIds,
  levelCeilings,
  levelFloors,
  onRowClick,
  onCeilingStep,
  onFloorStep,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 120,
    overscan: 8,
    measureElement: (el) => el?.getBoundingClientRect().height ?? 120,
  });

  if (filtered.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-12 text-text-muted">
        <p>
          {catalogCount === 0
            ? 'Construction item catalog not loaded. Reconnect to the server and try again.'
            : 'No construction items match your filters.'}
        </p>
      </div>
    );
  }

  return (
    <div ref={parentRef} className="custom-scrollbar mx-auto min-w-0 flex-1 w-full max-w-4xl overflow-y-auto px-4 py-4 sm:px-6">
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => {
          const item = filtered[virtualRow.index];
          const isSelected = selectedIds.has(item.id);
          const range = normalizeLevelRange(levelFloors[item.id] ?? TCI_LEVEL_MIN, levelCeilings[item.id] ?? TCI_LEVEL_MIN);
          const floor = range.floor;
          const ceil = range.ceiling;
          return (
            <div
              key={item.id}
              ref={rowVirtualizer.measureElement}
              data-index={virtualRow.index}
              className="pb-2"
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <div
                role="button"
                tabIndex={0}
                onClick={() => onRowClick(item.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onRowClick(item.id);
                  }
                }}
                className={`
                  flex min-h-0 cursor-pointer items-center gap-3 rounded-xl border-2 px-3 py-2.5 transition-colors
                  ${
                    isSelected
                      ? 'border-primary bg-primary/10 shadow-[0_0_12px_var(--color-primary-glow)]'
                      : 'border-border-base bg-bg-card hover:border-primary/50 hover:bg-bg-card-hover'
                  }
                `}
              >
                <div
                  className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 ${
                    isSelected ? 'border-primary bg-primary text-bg-app' : 'border-border-base bg-bg-app text-transparent'
                  }`}
                >
                  {isSelected && <Check className="h-4 w-4 stroke-[3]" />}
                </div>
                <div className="min-w-0 flex-1 text-left">
                  <div className={`text-sm font-semibold leading-tight ${isSelected ? 'text-primary' : 'text-text-main'}`}>
                    {item.label}
                  </div>
                  {item.effects ? (
                    <div className="mt-1 text-xs font-medium leading-snug text-text-main/90">{item.effects}</div>
                  ) : (
                    <div className="mt-1 text-xs italic text-text-muted">No effect line parsed</div>
                  )}
                  <div className="mt-1 text-[10px] font-medium uppercase tracking-wide text-text-muted">
                    {levelRangeLabel(item)}
                    {item.category ? ` · ${item.category}` : ''}
                  </div>
                  <div
                    className="mt-0.5 font-mono text-[10px] leading-snug text-text-muted/70 line-clamp-2"
                    title={formatGroupTiersLine(item)}
                  >
                    {formatGroupTiersLine(item)}
                  </div>
                </div>
                {isSelected && (
                  <div className="flex shrink-0 flex-col items-end gap-1" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center gap-1">
                      <span className="w-8 text-right text-[9px] font-bold uppercase tracking-wide text-text-muted">Max</span>
                      <button
                        type="button"
                        className="flex h-7 w-7 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
                        disabled={ceil <= floor}
                        onClick={() => onCeilingStep(item.id, -1)}
                        aria-label="Decrease level ceiling"
                      >
                        <Minus className="h-3.5 w-3.5" />
                      </button>
                      <span className="min-w-[24px] text-center font-mono text-sm font-bold tabular-nums">{ceil}</span>
                      <button
                        type="button"
                        className="flex h-7 w-7 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
                        disabled={ceil >= TCI_LEVEL_MAX}
                        onClick={() => onCeilingStep(item.id, 1)}
                        aria-label="Increase level ceiling"
                      >
                        <Plus className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <div className="flex items-center gap-1">
                      <span className="w-8 text-right text-[9px] font-bold uppercase tracking-wide text-text-muted">Min</span>
                      <button
                        type="button"
                        className="flex h-7 w-7 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
                        disabled={floor <= TCI_LEVEL_MIN}
                        onClick={() => onFloorStep(item.id, -1)}
                        aria-label="Decrease level floor"
                      >
                        <Minus className="h-3.5 w-3.5" />
                      </button>
                      <span className="min-w-[24px] text-center font-mono text-sm font-bold tabular-nums">{floor}</span>
                      <button
                        type="button"
                        className="flex h-7 w-7 items-center justify-center rounded-lg border border-border-base bg-bg-app text-text-main hover:bg-bg-card-hover disabled:opacity-40"
                        disabled={floor >= ceil}
                        onClick={() => onFloorStep(item.id, 1)}
                        aria-label="Increase level floor"
                      >
                        <Plus className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default TCIPickerModal;
