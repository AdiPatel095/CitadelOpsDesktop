import React, { useState, useMemo, useCallback, useEffect, useRef } from 'react';
import { Search, Check, Heart, List, Flame } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import UnitImage from './UnitImage';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import {
  getFavorites,
  toggleFavorite,
  incrementUsage,
  getTopFrequent,
} from '../config/UnitPickerStorage';
import { Modal, Button, Input, PillSelector, Badge } from './ui';

// ============================================
// Types
// ============================================

export type SelectionMode = 'single' | 'multi';
export type QuickAccessTab = 'all' | 'favorites' | 'frequent';
export type TypeFilter = 'all' | 'melee' | 'range';
export type RoleFilter = 'all' | 'attack' | 'defense';
export type FoodFilter = 'all' | 'mead' | 'beef' | 'food';

export interface UnitWithQuantity {
  unitId: number;
  quantity: number;
}

export interface TroopPickerOptions {
  mode: SelectionMode;
  title?: string;
  preselected?: number[];
  /** When true, allows setting a quantity for each selected unit */
  allowQuantity?: boolean;
  /** Pre-filled quantities when allowQuantity is true */
  preselectedQuantities?: Record<number, number>;
  /** Restrict the list to these unit ids (e.g. main castle troopsI). */
  allowedUnitIds?: number[];
  /** Optional in-castle stock counts shown on each unit card. */
  stockQuantities?: Record<number, number>;
}

// Result type varies based on options
export type TroopPickerResultSimple = number | number[] | null;
export type TroopPickerResultWithQuantity = UnitWithQuantity | UnitWithQuantity[] | null;
export type TroopPickerResult = TroopPickerResultSimple | TroopPickerResultWithQuantity;

interface TroopPickerModalProps {
  isOpen: boolean;
  options: TroopPickerOptions;
  onClose: (result: TroopPickerResult) => void;
}

// ============================================
// Promise-based API
// ============================================

let resolvePickerPromise: ((value: TroopPickerResult) => void) | null = null;
let setPickerState: React.Dispatch<React.SetStateAction<{ isOpen: boolean; options: TroopPickerOptions | null }>> | null = null;

/**
 * Show the troop picker modal and return the selected troop(s).
 */
export function showTroopPicker(options: TroopPickerOptions): Promise<TroopPickerResult> {
  return new Promise((resolve) => {
    resolvePickerPromise = resolve;
    if (setPickerState) {
      setPickerState({ isOpen: true, options });
    }
  });
}

// ============================================
// Provider Component (mount once in App)
// ============================================

export const TroopPickerProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [state, setState] = useState<{ isOpen: boolean; options: TroopPickerOptions | null }>({
    isOpen: false,
    options: null,
  });

  // Register the setState function for the promise API
  useEffect(() => {
    setPickerState = setState;
    return () => { setPickerState = null; };
  }, []);

  const handleClose = useCallback((result: TroopPickerResult) => {
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
        <TroopPickerModal
          isOpen={state.isOpen}
          options={state.options}
          onClose={handleClose}
        />
      )}
    </>
  );
};

// ============================================
// Virtualized Unit Grid Component
// ============================================

interface VirtualizedUnitGridProps {
  filteredUnits: [string, string][];
  selectedIds: Set<number>;
  favorites: Set<number>;
  quantities: Record<number, number>;
  allowQuantity: boolean;
  stockQuantities?: Record<number, number>;
  quickAccessTab: QuickAccessTab;
  onUnitClick: (unitId: number) => void;
  onFavoriteClick: (e: React.MouseEvent, unitId: number) => void;
  onQuantityChange: (unitId: number, value: string) => void;
}

const GRID_CARD_GAP_REM = 0.75;
const GRID_CARD_ASPECT_WIDTH = 3;
const GRID_CARD_ASPECT_HEIGHT = 4;
const GRID_ICON_SCALE = 0.85;
const DEFAULT_GRID_ROW_SIZE = 192;
const DEFAULT_GRID_CARD_WIDTH = DEFAULT_GRID_ROW_SIZE * GRID_CARD_ASPECT_WIDTH / GRID_CARD_ASPECT_HEIGHT;
const DEFAULT_GRID_ICON_SIZE = Math.round(DEFAULT_GRID_CARD_WIDTH * GRID_ICON_SCALE);
const DEFAULT_GRID_ROW_GAP = 12;

const estimateGridMetrics = (container?: HTMLDivElement | null, columnCount = 7) => {
  if (!container || typeof window === 'undefined') {
    return {
      iconSize: DEFAULT_GRID_ICON_SIZE,
      rowSize: DEFAULT_GRID_ROW_SIZE + DEFAULT_GRID_ROW_GAP,
    };
  }

  const styles = window.getComputedStyle(container);
  const paddingX = (parseFloat(styles.paddingLeft) || 0) + (parseFloat(styles.paddingRight) || 0);
  const rootFontSize = parseFloat(window.getComputedStyle(document.documentElement).fontSize) || 16;
  const gap = rootFontSize * GRID_CARD_GAP_REM;
  const contentWidth = Math.max(1, container.clientWidth - paddingX);
  const cardWidth = (contentWidth - gap * Math.max(0, columnCount - 1)) / columnCount;
  return {
    iconSize: Math.round(cardWidth * GRID_ICON_SCALE),
    rowSize: Math.round(cardWidth * GRID_CARD_ASPECT_HEIGHT / GRID_CARD_ASPECT_WIDTH + gap),
  };
};

const VirtualizedUnitGrid: React.FC<VirtualizedUnitGridProps> = ({
  filteredUnits,
  selectedIds,
  favorites,
  quantities,
  allowQuantity,
  stockQuantities,
  quickAccessTab,
  onUnitClick,
  onFavoriteClick,
  onQuantityChange,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(8);
  const [gridRowEstimate, setGridRowEstimate] = useState(() => estimateGridMetrics().rowSize);
  const [gridIconSize, setGridIconSize] = useState(() => estimateGridMetrics().iconSize);
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');

  // Determine column count and view mode based on container width
  useEffect(() => {
    const updateColumns = () => {
      if (!parentRef.current) return;
      const width = parentRef.current.offsetWidth;
      let nextColumns = 1;
      if (width < 500) {
        setViewMode('list');
      } else {
        setViewMode('grid');
        if (width >= 1080) nextColumns = 8;
        else if (width >= 920) nextColumns = 7;
        else if (width >= 760) nextColumns = 6;
        else nextColumns = 4;
      }
      setColumns(nextColumns);
      const metrics = estimateGridMetrics(parentRef.current, nextColumns);
      setGridRowEstimate(metrics.rowSize);
      setGridIconSize(metrics.iconSize);
    };

    updateColumns();
    const observer = new ResizeObserver(updateColumns);
    if (parentRef.current) {
      observer.observe(parentRef.current);
    }
    window.addEventListener('resize', updateColumns);
    return () => {
      observer.disconnect();
      window.removeEventListener('resize', updateColumns);
    };
  }, []);

  // Group units into rows
  const rows = useMemo(() => {
    const result: [string, string][][] = [];
    if (viewMode === 'list') {
      for (const unit of filteredUnits) {
        result.push([unit]);
      }
    } else {
      for (let i = 0; i < filteredUnits.length; i += columns) {
        result.push(filteredUnits.slice(i, i + columns));
      }
    }
    return result;
  }, [filteredUnits, columns, viewMode]);

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => viewMode === 'list' ? 76 : gridRowEstimate,
    overscan: 3,
    measureElement: (el) => el?.getBoundingClientRect().height ?? (viewMode === 'list' ? 76 : gridRowEstimate),
  });

  if (filteredUnits.length === 0) {
    return (
      <div className="flex-1 overflow-y-auto p-6">
        <div className="picker-empty-state">
          <p className="text-lg font-medium">No units found</p>
          <p className="text-sm mt-2">
            {quickAccessTab === 'favorites'
              ? 'Click the heart icon on units to add favorites'
              : quickAccessTab === 'frequent'
                ? 'Select units to build your frequently used list'
                : 'Try adjusting your filters'}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div ref={parentRef} className="picker-results-scroll custom-scrollbar">
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => {
          const row = rows[virtualRow.index];
          return (
            <div
              key={virtualRow.index}
              ref={rowVirtualizer.measureElement}
              data-index={virtualRow.index}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              {viewMode === 'list' ? (
                // LIST MODE: single full-width horizontal strip per unit
                row.map(([idStr, name]) => {
                  const unitId = parseInt(idStr);
                  const isSelected = selectedIds.has(unitId);
                  const isFav = favorites.has(unitId);

                  return (
                    <div
                      key={unitId}
                      onClick={() => onUnitClick(unitId)}
                      role="button"
                      tabIndex={0}
                      className={`picker-list-row ${isSelected ? 'picker-list-row-selected' : ''}`}
                    >
                      <div className="picker-list-image">
                        <UnitImage unitId={unitId} size={42} showLevel={false} className="!bg-transparent drop-shadow-md" />
                      </div>

                      <div className="picker-list-body">
                        <div className="picker-list-name-row">
                          <span className="picker-list-name">{name}</span>
                          <span className="picker-card-id">#{unitId}</span>
                        </div>
                      </div>

                      {stockQuantities?.[unitId] != null ? (
                        <span className="picker-list-stock">
                          {stockQuantities[unitId].toLocaleString()}
                        </span>
                      ) : null}

                      {allowQuantity && isSelected && (
                        <div className="shrink-0" onClick={(e) => e.stopPropagation()}>
                          <Input
                            type="text"
                            value={quantities[unitId] ? quantities[unitId].toLocaleString() : ''}
                            onChange={(e) => onQuantityChange(unitId, e.target.value)}
                            placeholder="Qty"
                            className="w-24 h-8 text-center font-mono"
                          />
                        </div>
                      )}

                      <button
                        onClick={(e) => onFavoriteClick(e, unitId)}
                        className={`picker-favorite-button ${isFav ? 'picker-favorite-button-active' : ''}`}
                      >
                        <Heart className={`w-4 h-4 ${isFav ? 'fill-current' : ''}`} />
                      </button>

                      {isSelected && (
                        <div className="picker-selection-indicator">
                          <Check className="w-3.5 h-3.5 text-primary stroke-[3]" />
                        </div>
                      )}
                    </div>
                  );
                })
              ) : (
                // GRID MODE: card grid sized from the available picker width
                <div className="picker-grid-row grid gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
                  {row.map(([idStr, name]) => {
                    const unitId = parseInt(idStr);
                    const isSelected = selectedIds.has(unitId);
                    const isFav = favorites.has(unitId);

                    return (
                      <div
                        key={unitId}
                        onClick={() => onUnitClick(unitId)}
                        role="button"
                        tabIndex={0}
                        className={`picker-grid-card ${isSelected ? 'picker-grid-card-selected' : ''}`}
                      >
                        <div className="picker-card-topline">
                          <span className="picker-card-id">#{unitId}</span>
                          {stockQuantities?.[unitId] != null ? (
                            <span className="picker-stock-pill">
                              {stockQuantities[unitId].toLocaleString()}
                            </span>
                          ) : null}
                        </div>

                        <div className="picker-grid-actions">
                          <button
                            onClick={(e) => onFavoriteClick(e, unitId)}
                            className={`picker-favorite-button ${isFav ? 'picker-favorite-button-active' : ''}`}
                          >
                            <Heart className={`w-4 h-4 ${isFav ? 'fill-current' : ''}`} />
                          </button>

                          {isSelected && (
                            <div className="picker-selection-indicator">
                              <Check className="w-4 h-4 text-primary stroke-[3]" />
                            </div>
                          )}
                        </div>

                        <div className="picker-image-stage">
                          <UnitImage unitId={unitId} size={gridIconSize} showLevel={true} className="!bg-transparent drop-shadow-md" />
                        </div>

                        <div className="picker-card-body">
                          <span className={`picker-unit-name line-clamp-2 ${isSelected ? 'picker-unit-name-selected' : ''}`}>
                            {name}
                          </span>
                        </div>

                        {allowQuantity && isSelected ? (
                          <div className="picker-card-quantity" onClick={(e) => e.stopPropagation()}>
                            <Input
                              type="text"
                              value={quantities[unitId] ? quantities[unitId].toLocaleString() : ''}
                              onChange={(e) => onQuantityChange(unitId, e.target.value)}
                              placeholder="Qty"
                              className="text-center font-mono h-8"
                            />
                          </div>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};

// ============================================
// Modal Component
// ============================================

const TroopPickerModal: React.FC<TroopPickerModalProps> = ({ isOpen, options, onClose }) => {
  const {
    mode,
    title,
    preselected = [],
    allowQuantity = false,
    preselectedQuantities = {},
    allowedUnitIds,
    stockQuantities,
  } = options;
  const { troops } = useMetadata();

  // Selection state
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set(preselected));
  const [quantities, setQuantities] = useState<Record<number, number>>(preselectedQuantities);

  // Filter state
  const [searchQuery, setSearchQuery] = useState('');
  const restrictToAllowed = allowedUnitIds !== undefined;
  const initialTab: QuickAccessTab = restrictToAllowed ? 'all' : getTopFrequent(50).length > 0 ? 'frequent' : 'all';
  const [quickAccessTab, setQuickAccessTab] = useState<QuickAccessTab>(initialTab);
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('all');
  const [roleFilter, setRoleFilter] = useState<RoleFilter>('all');
  const [foodFilter, setFoodFilter] = useState<FoodFilter>('all');

  // Favorites state (local)
  const [favorites, setFavoritesState] = useState<Set<number>>(getFavorites());
  const [frequentIds, setFrequentIds] = useState<number[]>(getTopFrequent(50));

  // Refresh favorites on mount
  useEffect(() => {
    setFavoritesState(getFavorites());
    setFrequentIds(getTopFrequent(50));
    if (restrictToAllowed) {
      setQuickAccessTab('all');
    }
  }, [isOpen, restrictToAllowed]);

  // Get definitions and metadata for troops
  const definitions = useMemo<Record<number, string>>(() => {
    if (!restrictToAllowed) {
	  return Object.fromEntries(Object.entries(troops).map(([id, item]) => [Number(id), item.name]));
    }
    const out: Record<number, string> = {};
    for (const id of allowedUnitIds ?? []) {
      if (!Number.isFinite(id) || id <= 0) {
        continue;
      }
      const unitID = Math.floor(id);
	  out[unitID] = troops[unitID]?.name || `Unit #${unitID}`;
    }
    return out;
  }, [allowedUnitIds, restrictToAllowed, troops]);
  const metadata = useMemo(() => Object.fromEntries(
	Object.entries(troops).map(([id, item]) => [Number(id), pickerUnitMetadata(item)]),
  ), [troops]);

  // Filter units by all criteria
  const filteredUnits = useMemo(() => {
    let entries = Object.entries(definitions);

    if (restrictToAllowed) {
      const allowed = new Set(allowedUnitIds ?? []);
      entries = entries.filter(([id]) => allowed.has(parseInt(id)));
      if (stockQuantities) {
        entries.sort(
          (a, b) => (stockQuantities[parseInt(b[0])] ?? 0) - (stockQuantities[parseInt(a[0])] ?? 0)
        );
      }
    }

    // Filter by search query
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      entries = entries.filter(([id, name]) =>
        name.toLowerCase().includes(query) || id.includes(query)
      );
    }

    // Filter by type (melee/range)
    if (typeFilter !== 'all') {
      entries = entries.filter(([id]) => {
        const meta = metadata[parseInt(id)];
        return meta?.type === typeFilter;
      });
    }

    // Filter by role (attack/defense)
    if (roleFilter !== 'all') {
      entries = entries.filter(([id]) => {
        const meta = metadata[parseInt(id)];
        return meta?.role === roleFilter;
      });
    }

    // Filter by food type (mead/beef/food)
    if (foodFilter !== 'all') {
      entries = entries.filter(([id]) => {
        const meta = metadata[parseInt(id)];
        // Default to 'food' if not specified
        const unitFood = meta?.food || 'food';
        return unitFood === foodFilter;
      });
    }

    // Filter by quick access tab
    if (quickAccessTab === 'favorites') {
      entries = entries.filter(([id]) => favorites.has(parseInt(id)));
    } else if (quickAccessTab === 'frequent') {
      const frequentSet = new Set(frequentIds);
      entries = entries.filter(([id]) => frequentSet.has(parseInt(id)));
      // Sort by frequency
      entries.sort((a, b) => {
        const aIdx = frequentIds.indexOf(parseInt(a[0]));
        const bIdx = frequentIds.indexOf(parseInt(b[0]));
        return aIdx - bIdx;
      });
    }

    return entries;
  }, [definitions, metadata, allowedUnitIds, restrictToAllowed, stockQuantities, searchQuery, typeFilter, roleFilter, foodFilter, quickAccessTab, favorites, frequentIds]);
  const visibleUnitLabel = filteredUnits.length === 1 ? 'unit' : 'units';

  // Handle unit selection
  const handleUnitClick = (unitId: number) => {
    if (mode === 'single') {
      setSelectedIds(new Set([unitId]));
      if (allowQuantity && quantities[unitId] === undefined) {
        setQuantities(prev => ({ ...prev, [unitId]: 1 }));
      }
    } else {
      setSelectedIds(prev => {
        const next = new Set(prev);
        if (next.has(unitId)) {
          next.delete(unitId);
        } else {
          next.add(unitId);
          if (allowQuantity && quantities[unitId] === undefined) {
            setQuantities(prev => ({ ...prev, [unitId]: 1 }));
          }
        }
        return next;
      });
    }
  };

  // Handle favorite toggle
  const handleFavoriteClick = (e: React.MouseEvent, unitId: number) => {
    e.stopPropagation();
    toggleFavorite(unitId);
    setFavoritesState(getFavorites());
  };

  // Handle quantity change
  const handleQuantityChange = (unitId: number, value: string) => {
    // Remove non-digits (commas, etc)
    const cleanValue = value.replace(/[^0-9]/g, '');
    const numValue = parseInt(cleanValue) || 0;
    setQuantities(prev => ({ ...prev, [unitId]: Math.max(0, numValue) }));
  };

  // Handle confirm
  const handleConfirm = () => {
    const selectedArray = Array.from(selectedIds);

    // Track usage for frequently used
    if (selectedArray.length > 0) {
      incrementUsage(selectedArray);
    }

    if (allowQuantity) {
      if (mode === 'single') {
        const unitId = selectedArray[0];
        if (unitId !== undefined) {
          const qty = quantities[unitId];
          onClose({ unitId, quantity: qty !== undefined ? qty : 1 });
        } else {
          onClose(null);
        }
      } else {
        const selected = selectedArray.map(unitId => {
          const qty = quantities[unitId];
          return {
            unitId,
            quantity: qty !== undefined ? qty : 1
          };
        });
        onClose(selected.length > 0 ? selected : null);
      }
    } else {
      if (mode === 'single') {
        onClose(selectedArray[0] ?? null);
      } else {
        onClose(selectedArray.length > 0 ? selectedArray : null);
      }
    }
  };

  // Handle cancel
  const handleCancel = () => {
    onClose(null);
  };

  if (!isOpen) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleCancel}
      maxWidth="6xl"
      title={
        <div className="picker-modal-title">
          <span className="picker-modal-title-mark" aria-hidden="true" />
          <span className="picker-modal-title-text">
            {title || (mode === 'single' ? 'Select Troop' : 'Select Troops')}
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
      <div className="picker-shell">
        <div className="picker-toolbar">
          <div className="picker-toolbar-overview">
            <div className="picker-toolbar-copy">
              <span className="picker-toolbar-kicker">
                {allowQuantity ? 'Quantity picker' : mode === 'single' ? 'Single pick' : 'Multi pick'}
              </span>
              <span className="picker-toolbar-count">
                {filteredUnits.length.toLocaleString()} {visibleUnitLabel}
              </span>
            </div>
            <div className="picker-selection-summary">
              <Check className="w-3.5 h-3.5" />
              <span>{selectedIds.size.toLocaleString()}</span>
              selected
            </div>
          </div>

          <div className="picker-command-row">
            <div className="picker-search-slot">
              <Input
                type="text"
                placeholder="Search by name or ID..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                leftIcon={<Search className="w-4 h-4" />}
              />
            </div>

            <PillSelector
              value={quickAccessTab}
              onChange={(v) => setQuickAccessTab(v as QuickAccessTab)}
              options={[
                { value: 'all', label: 'All', icon: <List className="w-3.5 h-3.5" /> },
                { value: 'favorites', label: 'Favorites', icon: <Heart className="w-3.5 h-3.5" /> },
                { value: 'frequent', label: 'Frequent', icon: <Flame className="w-3.5 h-3.5" /> }
              ]}
              className="picker-quick-pills"
            />
          </div>

          <div className="picker-filter-dock">
            <span className="picker-filter-dock-label">Filters</span>
            <div className="picker-filter-row">
              <PillSelector
                value={typeFilter}
                onChange={(v) => setTypeFilter(v as TypeFilter)}
                options={[
                  { value: 'all', label: 'All Types' },
                  { value: 'melee', label: 'Melee' },
                  { value: 'range', label: 'Range' }
                ]}
                size="sm"
              />
              <PillSelector
                value={roleFilter}
                onChange={(v) => setRoleFilter(v as RoleFilter)}
                options={[
                  { value: 'all', label: 'All Roles' },
                  { value: 'attack', label: 'Attack' },
                  { value: 'defense', label: 'Defense' }
                ]}
                size="sm"
              />
              <PillSelector
                value={foodFilter}
                onChange={(v) => setFoodFilter(v as FoodFilter)}
                options={[
                  { value: 'all', label: 'All Food' },
                  { value: 'mead', label: 'Mead' },
                  { value: 'beef', label: 'Beef' },
                  { value: 'food', label: 'Food' }
                ]}
                size="sm"
              />
            </div>
          </div>
        </div>

        {/* Unit Grid - Virtualized */}
        <VirtualizedUnitGrid
          filteredUnits={filteredUnits}
          selectedIds={selectedIds}
          favorites={favorites}
          quantities={quantities}
          allowQuantity={allowQuantity}
          stockQuantities={stockQuantities}
          quickAccessTab={quickAccessTab}
          onUnitClick={handleUnitClick}
          onFavoriteClick={handleFavoriteClick}
          onQuantityChange={handleQuantityChange}
        />
      </div>
    </Modal>
  );
};

function pickerUnitMetadata(item: MetadataItem) {
  const officialRole = String(item.role ?? '').toLowerCase();
  const type: 'melee' | 'range' = officialRole.includes('range') ? 'range' : 'melee';
  const attack = Math.max(metadataNumber(item.meleeAttack), metadataNumber(item.rangeAttack));
  const defense = Math.max(metadataNumber(item.meleeDefence), metadataNumber(item.rangeDefence));
  const role: 'attack' | 'defense' = attack >= defense ? 'attack' : 'defense';
  const mead = metadataNumber(item.meadSupply);
  const beef = metadataNumber(item.beefSupply);
  const food = metadataNumber(item.foodSupply);
  return {
    type,
    role,
    food: mead > 0 ? 'mead' as const : beef > 0 ? 'beef' as const : 'food' as const,
    consumption: mead || beef || food,
  };
}

function metadataNumber(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export default TroopPickerModal;
