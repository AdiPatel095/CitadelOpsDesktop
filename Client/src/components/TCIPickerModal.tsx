import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown, Clock3, Layers3, Minus, Plus, Sparkles } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Button, CatalogPickerModal, EmptyState, PillSelector } from './ui';
import TCIImage from './TCIImage';
import {
  durationRangeLabel,
  fetchConstructionItemsCatalog,
  formatDuration,
  formatEffectUpgradeLine,
  formatGroupTiersLine,
  levelRangeLabel,
  type ConstructionItemCatalogEntry,
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
  const normalizedFloor = clampLevelFloor(floor, minLevel, maxLevel);
  const normalizedCeiling = clampLevelCeiling(ceiling, minLevel, maxLevel);
  return normalizedFloor <= normalizedCeiling
    ? { floor: normalizedFloor, ceiling: normalizedCeiling }
    : { floor: normalizedCeiling, ceiling: normalizedCeiling };
}

export type TCISelectionMode = 'single' | 'multi';

export interface TCIWithLevelCeiling {
  constructionItemId: number;
  levelCeiling: number;
  levelFloor: number;
}

export interface TCIPickerOptions {
  mode: TCISelectionMode;
  title?: string;
  preselected?: number[];
  preselectedLevelCeilings?: Record<number, number>;
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
    // Open the picker with its empty-state guidance when the catalog is unavailable.
  }
  return new Promise((resolve) => {
    resolvePickerPromise = resolve;
    setPickerState?.({ isOpen: true, options });
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
    if (!state.isOpen || !state.options) return;
    fetchConstructionItemsCatalog()
      .then(setCatalog)
      .catch(() => setCatalog([]));
  }, [state.isOpen, state.options]);

  const handleClose = useCallback((result: TCIPickerResult) => {
    setState({ isOpen: false, options: null });
    resolvePickerPromise?.(result);
    resolvePickerPromise = null;
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

type TCICatalogFilter = 'all' | 'selected' | 'short' | 'long';

const SEVEN_DAYS_SECONDS = 7 * 86_400;

const TCIPickerModal: React.FC<TCIPickerModalProps> = ({ isOpen, options, catalog, onClose }) => {
  const {
    mode,
    title,
    preselected = [],
    preselectedLevelCeilings = {},
    preselectedLevelFloors = {},
  } = options;
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set(preselected));
  const [levelCeilings, setLevelCeilings] = useState<Record<number, number>>(() => {
    const values: Record<number, number> = {};
    preselected.forEach((id) => {
      const range = normalizeLevelRange(
        preselectedLevelFloors[id] ?? TCI_LEVEL_MIN,
        preselectedLevelCeilings[id] ?? TCI_LEVEL_MIN,
        ...catalogLevelBounds(catalog, id),
      );
      values[id] = range.ceiling;
    });
    return values;
  });
  const [levelFloors, setLevelFloors] = useState<Record<number, number>>(() => {
    const values: Record<number, number> = {};
    preselected.forEach((id) => {
      const range = normalizeLevelRange(
        preselectedLevelFloors[id] ?? TCI_LEVEL_MIN,
        preselectedLevelCeilings[id] ?? TCI_LEVEL_MIN,
        ...catalogLevelBounds(catalog, id),
      );
      values[id] = range.floor;
    });
    return values;
  });
  const [searchQuery, setSearchQuery] = useState('');
  const [catalogFilter, setCatalogFilter] = useState<TCICatalogFilter>('all');
  const [activeId, setActiveId] = useState<number | null>(preselected[0] ?? null);

  const filtered = useMemo(() => {
    let rows = catalog;
    const query = searchQuery.toLowerCase().trim();
    if (query) {
      const compactQuery = query.replace(/,/g, '');
      const matches = (value: string | number | undefined) => {
        const text = String(value ?? '').toLowerCase();
        return text.includes(query) || text.replace(/,/g, '').includes(compactQuery);
      };
      rows = rows.filter((entry) => {
        const effectLine = formatEffectUpgradeLine(entry);
        const tierLine = formatGroupTiersLine(entry);
        return (
          matches(entry.label) ||
          matches(entry.internal) ||
          matches(entry.effects) ||
          matches(effectLine) ||
          matches(entry.category) ||
          matches(entry.id) ||
          matches(entry.level) ||
          matches(tierLine) ||
          matches(durationRangeLabel(entry)) ||
          entry.groupTiers.some((tier) =>
            matches(tier.wireCid) ||
            matches(tier.level) ||
            matches(tier.effects) ||
            matches(formatDuration(tier.durationSeconds))
          )
        );
      });
    }
    if (catalogFilter === 'selected') {
      rows = rows.filter((entry) => selectedIds.has(entry.id));
    } else if (catalogFilter === 'short') {
      rows = rows.filter((entry) => entry.durationSecondsMax <= SEVEN_DAYS_SECONDS);
    } else if (catalogFilter === 'long') {
      rows = rows.filter((entry) => entry.durationSecondsMax > SEVEN_DAYS_SECONDS);
    }
    return rows;
  }, [catalog, catalogFilter, searchQuery, selectedIds]);

  useEffect(() => {
    if (filtered.length === 0) {
      setActiveId(null);
      return;
    }
    if (activeId == null || !filtered.some((entry) => entry.id === activeId)) {
      setActiveId(filtered[0].id);
    }
  }, [activeId, filtered]);

  const handleItemClick = (id: number) => {
    setActiveId(id);
    if (mode === 'single') {
      setSelectedIds(new Set([id]));
      initializeRange(id);
      return;
    }

    const deselecting = selectedIds.has(id);
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    if (!deselecting) initializeRange(id);
  };

  const initializeRange = (id: number) => {
    const [minLevel, maxLevel] = catalogLevelBounds(catalog, id);
    const range = normalizeLevelRange(
      levelFloors[id] ?? minLevel,
      levelCeilings[id] ?? minLevel,
      minLevel,
      maxLevel,
    );
    setLevelFloors((current) => ({ ...current, [id]: range.floor }));
    setLevelCeilings((current) => ({ ...current, [id]: range.ceiling }));
  };

  const handleCeilingStep = (id: number, delta: number) => {
    setLevelCeilings((current) => {
      const range = normalizeLevelRange(
        levelFloors[id] ?? TCI_LEVEL_MIN,
        (current[id] ?? TCI_LEVEL_MIN) + delta,
        ...catalogLevelBounds(catalog, id),
      );
      setLevelFloors((floors) => ({ ...floors, [id]: range.floor }));
      return { ...current, [id]: range.ceiling };
    });
  };

  const handleFloorStep = (id: number, delta: number) => {
    setLevelFloors((current) => {
      const range = normalizeLevelRange(
        (current[id] ?? TCI_LEVEL_MIN) + delta,
        levelCeilings[id] ?? TCI_LEVEL_MIN,
        ...catalogLevelBounds(catalog, id),
      );
      setLevelCeilings((ceilings) => ({ ...ceilings, [id]: range.ceiling }));
      return { ...current, [id]: range.floor };
    });
  };

  const handleConfirm = () => {
    const selected = Array.from(selectedIds);
    if (selected.length === 0) {
      onClose(null);
      return;
    }
    const result = selected.map((id) => {
      const range = normalizeLevelRange(
        levelFloors[id] ?? TCI_LEVEL_MIN,
        levelCeilings[id] ?? TCI_LEVEL_MIN,
        ...catalogLevelBounds(catalog, id),
      );
      return {
        constructionItemId: id,
        levelFloor: range.floor,
        levelCeiling: range.ceiling,
      };
    });
    onClose(mode === 'single' ? result[0] : result);
  };

  const handleCancel = () => onClose(null);
  const activeItem = activeId == null ? undefined : filtered.find((entry) => entry.id === activeId);
  const visibleItemLabel = filtered.length === 1 ? 'design' : 'designs';

  if (!isOpen) return null;

  return (
    <CatalogPickerModal
      isOpen={isOpen}
      onClose={handleCancel}
      onConfirm={handleConfirm}
      title={title || (mode === 'single' ? 'Select construction item' : 'Select construction items')}
      modeLabel="Construction item browser"
      selectedCount={selectedIds.size}
      resultCount={filtered.length}
      resultLabel={visibleItemLabel}
      searchValue={searchQuery}
      onSearchChange={setSearchQuery}
      searchPlaceholder="Search names, effects, durations, or CIDs…"
      shellClassName="picker-shell-tci"
      toolbarClassName="tci-browser-toolbar"
      commandRowClassName="tci-browser-command-row"
      commandExtras={(
        <PillSelector
          ariaLabel="Catalog filters"
          value={catalogFilter}
          onChange={(value) => setCatalogFilter(value as TCICatalogFilter)}
          options={[
            { value: 'all', label: 'All' },
            { value: 'selected', label: 'Selected' },
            { value: 'short', label: '≤ 7 days' },
            { value: 'long', label: '8+ days' },
          ]}
          size="header"
          className="tci-browser-filters"
        />
      )}
    >
      <div className="tci-browser-layout">
          {filtered.length === 0 ? (
            <EmptyState
              surface="plain"
              icon={<Layers3 className="h-8 w-8" />}
              title={catalog.length === 0 ? 'Construction item catalog not loaded' : 'No matching construction items'}
              description={catalog.length === 0
                    ? 'Reconnect to the server and try again.'
                    : 'Try a different search or duration filter.'}
              className="picker-empty-state tci-browser-empty"
            />
          ) : (
            <VirtualizedTCIGallery
              items={filtered}
              selectedIds={selectedIds}
              activeId={activeId}
              onActivate={setActiveId}
              onItemClick={handleItemClick}
            />
          )}

          <TCIDetailPanel
            item={activeItem}
            isSelected={activeItem ? selectedIds.has(activeItem.id) : false}
            levelFloor={activeItem ? levelFloors[activeItem.id] ?? activeItem.minLevel : TCI_LEVEL_MIN}
            levelCeiling={activeItem ? levelCeilings[activeItem.id] ?? activeItem.minLevel : TCI_LEVEL_MIN}
            onToggle={() => activeItem && handleItemClick(activeItem.id)}
            onFloorStep={(delta) => activeItem && handleFloorStep(activeItem.id, delta)}
            onCeilingStep={(delta) => activeItem && handleCeilingStep(activeItem.id, delta)}
          />
      </div>
    </CatalogPickerModal>
  );
};

interface VirtualizedTCIGalleryProps {
  items: ConstructionItemCatalogEntry[];
  selectedIds: Set<number>;
  activeId: number | null;
  onActivate: (id: number) => void;
  onItemClick: (id: number) => void;
}

const TCI_GRID_ROW_ESTIMATE = 244;

const VirtualizedTCIGallery: React.FC<VirtualizedTCIGalleryProps> = ({
  items,
  selectedIds,
  activeId,
  onActivate,
  onItemClick,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(2);

  useEffect(() => {
    const updateColumns = () => {
      if (!parentRef.current) return;
      setColumns(parentRef.current.clientWidth >= 650 ? 2 : 1);
    };
    updateColumns();
    const observer = new ResizeObserver(updateColumns);
    if (parentRef.current) observer.observe(parentRef.current);
    return () => observer.disconnect();
  }, []);

  const rows = useMemo(() => {
    const result: ConstructionItemCatalogEntry[][] = [];
    for (let index = 0; index < items.length; index += columns) {
      result.push(items.slice(index, index + columns));
    }
    return result;
  }, [columns, items]);

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => TCI_GRID_ROW_ESTIMATE,
    overscan: 3,
    measureElement: (element) => element?.getBoundingClientRect().height ?? TCI_GRID_ROW_ESTIMATE,
  });

  return (
    <div ref={parentRef} className="tci-browser-results custom-scrollbar">
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => (
          <div
            key={virtualRow.index}
            ref={rowVirtualizer.measureElement}
            data-index={virtualRow.index}
            className="tci-browser-grid-row"
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              transform: `translateY(${virtualRow.start}px)`,
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
            }}
          >
            {rows[virtualRow.index].map((item) => (
              <TCIBrowserCard
                key={item.id}
                item={item}
                isSelected={selectedIds.has(item.id)}
                isActive={activeId === item.id}
                onActivate={() => onActivate(item.id)}
                onClick={() => onItemClick(item.id)}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
};

interface TCIBrowserCardProps {
  item: ConstructionItemCatalogEntry;
  isSelected: boolean;
  isActive: boolean;
  onActivate: () => void;
  onClick: () => void;
}

const TCIBrowserCard: React.FC<TCIBrowserCardProps> = ({ item, isSelected, isActive, onActivate, onClick }) => {
  const effectLine = formatEffectUpgradeLine(item);
  return (
    <button
      type="button"
      className={`tci-browser-card ${isSelected ? 'tci-browser-card-selected' : ''} ${isActive ? 'tci-browser-card-active' : ''}`}
      aria-pressed={isSelected}
      onMouseEnter={onActivate}
      onFocus={onActivate}
      onClick={onClick}
    >
      <div className="tci-browser-card-topline">
        <span className="tci-browser-category">{item.category || 'Timed item'}</span>
        <span className="tci-browser-duration">
          <Clock3 aria-hidden="true" />
          {durationRangeLabel(item)}
        </span>
      </div>
      <div className="tci-browser-card-main">
        <div className="tci-browser-card-art">
          <TCIImage src={item.imageUrl} alt={item.buildingName || item.label} size={84} />
          {isSelected && (
            <span className="tci-browser-selected-mark" aria-label="Selected">
              <Check aria-hidden="true" />
            </span>
          )}
        </div>
        <div className="tci-browser-card-copy">
          <h3>{item.label}</h3>
          <p className="tci-browser-effect line-clamp-2">
            {effectLine || 'No effect description available'}
          </p>
          <span className="tci-browser-level-summary">{levelRangeLabel(item)}</span>
        </div>
      </div>
      <div className="tci-browser-mini-chain" aria-label={`Upgrade chain for ${item.label}`}>
        {item.groupTiers.map((tier, index) => (
          <React.Fragment key={tier.wireCid}>
            <span className="tci-browser-mini-tier" data-rarity={tier.rarity}>
              <strong>L{tier.level}</strong>
              <small>{formatDuration(tier.durationSeconds)}</small>
            </span>
            {index < item.groupTiers.length - 1 && <span className="tci-browser-mini-arrow">→</span>}
          </React.Fragment>
        ))}
      </div>
    </button>
  );
};

interface TCIDetailPanelProps {
  item?: ConstructionItemCatalogEntry;
  isSelected: boolean;
  levelFloor: number;
  levelCeiling: number;
  onToggle: () => void;
  onFloorStep: (delta: number) => void;
  onCeilingStep: (delta: number) => void;
}

const TCIDetailPanel: React.FC<TCIDetailPanelProps> = ({
  item,
  isSelected,
  levelFloor,
  levelCeiling,
  onToggle,
  onFloorStep,
  onCeilingStep,
}) => {
  if (!item) {
    return (
      <aside className="tci-detail-panel tci-detail-panel-empty">
        <Layers3 aria-hidden="true" />
        <p>Choose a design to inspect its full upgrade chain.</p>
      </aside>
    );
  }

  const range = normalizeLevelRange(levelFloor, levelCeiling, item.minLevel, item.maxLevel);
  const effectLine = formatEffectUpgradeLine(item);

  return (
    <aside className="tci-detail-panel custom-scrollbar">
      <div className="tci-detail-hero">
        <div className="tci-detail-art-stage">
          <TCIImage src={item.imageUrl} alt={item.buildingName || item.label} size={116} />
        </div>
        <div className="tci-detail-heading">
          <span className="tci-detail-kicker">{item.category || 'Timed construction item'}</span>
          <h3>{item.label}</h3>
          <div className="tci-detail-badges">
            <span><Clock3 aria-hidden="true" />{durationRangeLabel(item)}</span>
            <span><Layers3 aria-hidden="true" />{item.groupTiers.length} tiers</span>
            {item.premium && <span><Sparkles aria-hidden="true" />Premium</span>}
          </div>
        </div>
      </div>

      <p className={`tci-detail-effect ${effectLine ? '' : 'tci-detail-effect-empty'}`}>
        {effectLine || 'The official catalog does not expose a translated effect description for this item.'}
      </p>

      <Button
        variant={isSelected ? 'ghost' : 'primary'}
        className="w-full"
        onClick={onToggle}
        leftIcon={<Check className="h-4 w-4" />}
      >
        {isSelected ? 'Remove from selection' : 'Select this design'}
      </Button>

      {isSelected && (
        <div className="tci-range-editor">
          <div className="tci-range-editor-heading">
            <span>Allowed tier range</span>
            <strong>L{range.floor}–L{range.ceiling}</strong>
          </div>
          <div className="tci-range-editor-controls">
            <TCILevelStepper
              label="Min"
              value={range.floor}
              decrementDisabled={range.floor <= item.minLevel}
              incrementDisabled={range.floor >= range.ceiling}
              onDecrement={() => onFloorStep(-1)}
              onIncrement={() => onFloorStep(1)}
            />
            <TCILevelStepper
              label="Max"
              value={range.ceiling}
              decrementDisabled={range.ceiling <= range.floor}
              incrementDisabled={range.ceiling >= item.maxLevel}
              onDecrement={() => onCeilingStep(-1)}
              onIncrement={() => onCeilingStep(1)}
            />
          </div>
        </div>
      )}

      <div className="tci-detail-chain-heading">
        <span>Upgrade chain</span>
        <small>Official CID and active duration by tier</small>
      </div>
      <div className="tci-detail-chain">
        {item.groupTiers.map((tier, index) => {
          const included = isSelected && tier.level >= range.floor && tier.level <= range.ceiling;
          return (
            <React.Fragment key={tier.wireCid}>
              <article
                className={`tci-detail-tier ${included ? 'tci-detail-tier-included' : ''}`}
                data-rarity={tier.rarity}
              >
                <div className="tci-detail-tier-level">
                  <span>L{tier.level}</span>
                  <small>#{tier.wireCid}</small>
                </div>
                <div className="tci-detail-tier-copy">
                  <div className="tci-detail-tier-meta">
                    <strong><Clock3 aria-hidden="true" />{formatDuration(tier.durationSeconds)}</strong>
                    {tier.premium && <span>Premium</span>}
                    {tier.removalCost > 0 && <span>Removal {tier.removalCost.toLocaleString()}</span>}
                  </div>
                  <p>{tier.effects || 'Same visual design; no translated effect line available.'}</p>
                </div>
                {included && <Check className="tci-detail-tier-check" aria-label="Included in selected range" />}
              </article>
              {index < item.groupTiers.length - 1 && (
                <ChevronDown className="tci-detail-chain-arrow" aria-hidden="true" />
              )}
            </React.Fragment>
          );
        })}
      </div>
    </aside>
  );
};

interface TCILevelStepperProps {
  label: string;
  value: number;
  decrementDisabled: boolean;
  incrementDisabled: boolean;
  onDecrement: () => void;
  onIncrement: () => void;
}

const TCILevelStepper: React.FC<TCILevelStepperProps> = ({
  label,
  value,
  decrementDisabled,
  incrementDisabled,
  onDecrement,
  onIncrement,
}) => (
  <div className="tci-level-row">
    <span className="tci-level-label">{label}</span>
    <button
      type="button"
      className="tci-level-button"
      disabled={decrementDisabled}
      onClick={onDecrement}
      aria-label={`Decrease ${label.toLowerCase()}imum tier`}
    >
      <Minus className="h-3.5 w-3.5" />
    </button>
    <span className="tci-level-value">{value}</span>
    <button
      type="button"
      className="tci-level-button"
      disabled={incrementDisabled}
      onClick={onIncrement}
      aria-label={`Increase ${label.toLowerCase()}imum tier`}
    >
      <Plus className="h-3.5 w-3.5" />
    </button>
  </div>
);

export default TCIPickerModal;
