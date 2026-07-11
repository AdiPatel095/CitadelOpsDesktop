import React, { useState, useMemo, useEffect, useRef, useCallback } from 'react';
import { Search, Check, Minus, Plus } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Modal, Button, Input, Badge } from './ui';
import {
  fetchConstructionItemsCatalog,
  type ConstructionItemCatalogEntry,
  formatEffectUpgradeLine,
  formatGroupTiersLine,
  levelRangeLabel,
} from './TCICatalogCache';

export const TCI_LEVEL_MIN = 1;

export function clampLevelCeiling(
  n: number,
  minLevel = TCI_LEVEL_MIN,
  maxLevel = Number.MAX_SAFE_INTEGER,
): number {
  if (Number.isNaN(n)) {
    return minLevel;
  }
  return Math.min(maxLevel, Math.max(minLevel, Math.floor(n)));
}

export function clampLevelFloor(n: number, minLevel = TCI_LEVEL_MIN, maxLevel = Number.MAX_SAFE_INTEGER): number {
  return clampLevelCeiling(n, minLevel, maxLevel);
}

export function normalizeLevelRange(
  floor: number,
  ceiling: number,
  minLevel = TCI_LEVEL_MIN,
  maxLevel = Number.MAX_SAFE_INTEGER,
): { floor: number; ceiling: number } {
  const f = clampLevelFloor(floor, minLevel, maxLevel);
  const c = clampLevelCeiling(ceiling, minLevel, maxLevel);
  return f <= c ? { floor: f, ceiling: c } : { floor: c, ceiling: c };
}

export type TCISelectionMode = 'single' | 'multi';

/** Picked construction item with allowed tier range (1–4). */
export interface TCIWithLevelCeiling {
  constructionItemId: number;
  levelCeiling: number;
  levelFloor: number;
}

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

function catalogLevelBounds(catalog: ConstructionItemCatalogEntry[], id: number): [number, number] {
  const entry = catalog.find((candidate) => candidate.id === id || candidate.groupIds.includes(id));
  return entry ? [entry.minLevel, entry.maxLevel] : [TCI_LEVEL_MIN, Number.MAX_SAFE_INTEGER];
}

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
        ...catalogLevelBounds(catalog, id),
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
        ...catalogLevelBounds(catalog, id),
      );
      m[id] = range.floor;
    });
    return m;
  });
  const [searchQuery, setSearchQuery] = useState('');

  const filtered = useMemo(() => {
    let rows = catalog;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim();
      const qCompact = q.replace(/,/g, '');
      const matches = (value: string | number | undefined) => {
        const text = String(value ?? '').toLowerCase();
        return text.includes(q) || text.replace(/,/g, '').includes(qCompact);
      };
      rows = rows.filter((c) => {
        const effectLine = formatEffectUpgradeLine(c);
        const tierLine = formatGroupTiersLine(c);
        return (
          matches(c.label) ||
          matches(c.internal) ||
          matches(c.effects) ||
          matches(effectLine) ||
          matches(c.category) ||
          matches(c.id) ||
          matches(c.level) ||
          matches(tierLine) ||
          c.groupTiers?.some(
            (t) =>
              matches(t.wireCid) ||
              matches(t.level) ||
              matches(t.effects)
          )
        );
      });
    }
    return rows;
  }, [catalog, searchQuery]);

  const handleUnitClick = (id: number) => {
    if (mode === 'single') {
      setSelectedIds(new Set([id]));
      setLevelCeilings((prev) => {
        const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, prev[id] ?? TCI_LEVEL_MIN, ...catalogLevelBounds(catalog, id));
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
        const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, prev[id] ?? TCI_LEVEL_MIN, ...catalogLevelBounds(catalog, id));
        setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
        return { ...prev, [id]: range.ceiling };
      });
    }
  };

  const handleCeilingStep = (id: number, delta: number) => {
    setLevelCeilings((prev) => {
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, (prev[id] ?? TCI_LEVEL_MIN) + delta, ...catalogLevelBounds(catalog, id));
      setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
      return { ...prev, [id]: range.ceiling };
    });
  };

  const handleFloorStep = (id: number, delta: number) => {
    setLevelFloors((prev) => {
      const range = normalizeLevelRange((prev[id] ?? TCI_LEVEL_MIN) + delta, levelCeilings[id] ?? TCI_LEVEL_MIN, ...catalogLevelBounds(catalog, id));
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
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, levelCeilings[id] ?? TCI_LEVEL_MIN, ...catalogLevelBounds(catalog, id));
      onClose({
        constructionItemId: id,
        levelFloor: range.floor,
        levelCeiling: range.ceiling,
      });
      return;
    }
    const selected: TCIWithLevelCeiling[] = selectedArray.map((id) => {
      const range = normalizeLevelRange(levelFloors[id] ?? TCI_LEVEL_MIN, levelCeilings[id] ?? TCI_LEVEL_MIN, ...catalogLevelBounds(catalog, id));
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
        <div className="picker-modal-title">
          <span className="picker-modal-title-mark" aria-hidden="true" />
          <span className="picker-modal-title-text">
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
      <div className="picker-shell picker-shell-narrow">
        <div className="picker-toolbar">
          <Input
            type="text"
            placeholder="Search effects, names, categories, or IDs…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            leftIcon={<Search className="w-4 h-4" />}
          />
          <p className="picker-helper-copy">
            Search by the effect you want, then set a <span className="font-semibold text-text-main">level floor and ceiling</span>{' '}
            (1–4) per variant when you only keep higher tiers in stash.
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
      <div className="picker-empty-state">
        <p>
          {catalogCount === 0
            ? 'Construction item catalog not loaded. Reconnect to the server and try again.'
            : 'No construction items match your filters.'}
        </p>
      </div>
    );
  }

  return (
    <div ref={parentRef} className="picker-results-scroll picker-results-scroll-narrow custom-scrollbar">
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
          const effectUpgradeLine = formatEffectUpgradeLine(item);
          const range = normalizeLevelRange(levelFloors[item.id] ?? item.minLevel, levelCeilings[item.id] ?? item.minLevel, item.minLevel, item.maxLevel);
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
                className={`tci-picker-row ${isSelected ? 'tci-picker-row-selected' : ''}`}
              >
                <div
                  className={`tci-picker-check ${isSelected ? 'tci-picker-check-selected' : ''}`}
                >
                  {isSelected && <Check className="h-4 w-4 stroke-[3]" />}
                </div>
                <div className="min-w-0 flex-1 text-left">
                  <div className={`tci-picker-title ${isSelected ? 'tci-picker-title-selected' : ''}`}>
                    {item.label}
                  </div>
                  {effectUpgradeLine ? (
                    <div className="tci-picker-effect" title={effectUpgradeLine}>
                      {effectUpgradeLine}
                    </div>
                  ) : (
                    <div className="tci-picker-effect tci-picker-effect-empty">No effect line parsed</div>
                  )}
                  <div className="tci-picker-meta">
                    {levelRangeLabel(item)}
                    {item.category ? ` · ${item.category}` : ''}
                  </div>
                  <div
                    className="tci-picker-tiers line-clamp-2"
                    title={formatGroupTiersLine(item)}
                  >
                    {formatGroupTiersLine(item)}
                  </div>
                </div>
                {isSelected && (
                  <div className="tci-level-controls" onClick={(e) => e.stopPropagation()}>
                    <div className="tci-level-row">
                      <span className="tci-level-label">Max</span>
                      <button
                        type="button"
                        className="tci-level-button"
                        disabled={ceil <= floor}
                        onClick={() => onCeilingStep(item.id, -1)}
                        aria-label="Decrease level ceiling"
                      >
                        <Minus className="h-3.5 w-3.5" />
                      </button>
                      <span className="tci-level-value">{ceil}</span>
                      <button
                        type="button"
                        className="tci-level-button"
                        disabled={ceil >= item.maxLevel}
                        onClick={() => onCeilingStep(item.id, 1)}
                        aria-label="Increase level ceiling"
                      >
                        <Plus className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <div className="tci-level-row">
                      <span className="tci-level-label">Min</span>
                      <button
                        type="button"
                        className="tci-level-button"
                        disabled={floor <= item.minLevel}
                        onClick={() => onFloorStep(item.id, -1)}
                        aria-label="Decrease level floor"
                      >
                        <Minus className="h-3.5 w-3.5" />
                      </button>
                      <span className="tci-level-value">{floor}</span>
                      <button
                        type="button"
                        className="tci-level-button"
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
