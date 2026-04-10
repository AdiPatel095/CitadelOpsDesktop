import React, { useState, useMemo, useCallback, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X, Search, Check, Heart, List, Flame } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import UnitImage from './UnitImage';
import {
  TROOP_DEFINITIONS,
  TROOP_METADATA,
} from '../config/constants';
import {
  getFavorites,
  toggleFavorite,
  incrementUsage,
  getTopFrequent,
} from '../config/unitPickerStorage';
import { Modal, Button, Input, ToggleGroup, Badge } from './ui';

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
  quickAccessTab: QuickAccessTab;
  onUnitClick: (unitId: number) => void;
  onFavoriteClick: (e: React.MouseEvent, unitId: number) => void;
  onQuantityChange: (unitId: number, value: string) => void;
}

const VirtualizedUnitGrid: React.FC<VirtualizedUnitGridProps> = ({
  filteredUnits,
  selectedIds,
  favorites,
  quantities,
  allowQuantity,
  quickAccessTab,
  onUnitClick,
  onFavoriteClick,
  onQuantityChange,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(8);
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');

  // Determine column count and view mode based on container width
  useEffect(() => {
    const updateColumns = () => {
      if (!parentRef.current) return;
      const width = parentRef.current.offsetWidth;
      if (width < 500) {
        setViewMode('list');
        setColumns(1);
      } else {
        setViewMode('grid');
        if (width >= 1120) setColumns(8);
        else if (width >= 920) setColumns(7);
        else if (width >= 760) setColumns(6);
        else if (width >= 500) setColumns(4);
        else setColumns(4);
      }
    };

    updateColumns();
    const observer = new ResizeObserver(updateColumns);
    if (parentRef.current) {
      observer.observe(parentRef.current);
    }
    return () => observer.disconnect();
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
    estimateSize: () => viewMode === 'list' ? 48 : 150,
    overscan: 3,
    measureElement: (el) => el?.getBoundingClientRect().height ?? (viewMode === 'list' ? 48 : 140),
  });

  if (filteredUnits.length === 0) {
    return (
      <div className="flex-1 overflow-y-auto p-6">
        <div className="text-center py-12 text-text-muted">
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
    <div ref={parentRef} className="mx-auto flex-1 w-full max-w-[1120px] overflow-y-auto px-4 py-6 custom-scrollbar sm:px-6">
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
                      className={`
                        flex items-center gap-3 px-4 h-12 rounded-global border transition-all duration-200 cursor-pointer mb-2
                        ${isSelected
                          ? 'border-primary/50 bg-primary/10 shadow-[0_0_15px_var(--color-primary-glow)] text-primary'
                          : 'border-border-base bg-bg-card hover:bg-bg-card-hover hover:border-primary/30 text-text-main'
                        }
                      `}
                    >
                      {/* Unit image */}
                      <div className="shrink-0">
                        <UnitImage unitId={unitId} size={32} showLevel={false} className="rounded-md" />
                      </div>

                      {/* Unit name */}
                      <span className="flex-1 text-sm font-semibold truncate">
                        {name}
                      </span>

                      {/* Inline quantity input when selected */}
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

                      {/* Favorite button */}
                      <button
                        onClick={(e) => onFavoriteClick(e, unitId)}
                        className={`shrink-0 w-8 h-8 rounded-full flex items-center justify-center transition-all ${isFav
                          ? 'bg-error/20 text-error hover:bg-error hover:text-white'
                          : 'bg-transparent text-text-muted hover:bg-error/10 hover:text-error'
                          }`}
                      >
                        <Heart className={`w-4 h-4 ${isFav ? 'fill-current' : ''}`} />
                      </button>

                      {/* Selection indicator */}
                      {isSelected && (
                        <div className="shrink-0 w-6 h-6 rounded-full bg-primary flex items-center justify-center shadow-lg shadow-primary/30">
                          <Check className="w-3.5 h-3.5 text-bg-app stroke-[3]" />
                        </div>
                      )}
                    </div>
                  );
                })
              ) : (
                // GRID MODE: existing card grid, unchanged
                <div className="grid gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
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
                        className={`
                          relative flex flex-col items-center rounded-xl transition-all duration-200
                          border-2 hover:-translate-y-1 overflow-hidden cursor-pointer w-full py-2
                          ${isSelected
                            ? 'border-primary bg-primary/10 shadow-[0_0_20px_var(--color-primary-glow)]'
                            : 'border-border-base bg-bg-card hover:border-primary/50 hover:bg-bg-card-hover shadow-sm'
                          }
                        `}
                      >
                        {/* Top-right icons container */}
                        <div className="absolute top-1.5 right-1.5 flex flex-col gap-1.5 z-10">
                          {/* Favorite button */}
                          <button
                            onClick={(e) => onFavoriteClick(e, unitId)}
                            className={`w-7 h-7 rounded-full flex items-center justify-center transition-all shadow-md ${isFav
                              ? 'bg-error text-white'
                              : 'bg-black/50 text-white/70 hover:bg-error/80 hover:text-white backdrop-blur-sm'
                              }`}
                          >
                            <Heart className={`w-4 h-4 ${isFav ? 'fill-current' : ''}`} />
                          </button>

                          {/* Selection indicator */}
                          {isSelected && (
                            <div className="w-7 h-7 rounded-full bg-primary flex items-center justify-center shadow-lg shadow-primary/40">
                              <Check className="w-4 h-4 text-bg-app stroke-[3]" />
                            </div>
                          )}
                        </div>

                        {/* Unit Image - edge to edge */}
                        <div className="w-full h-[90px] flex items-center justify-center pt-1">
                          <UnitImage unitId={unitId} size={84} showLevel={true} className="drop-shadow-md" />
                        </div>

                        {/* Bottom area: Name or Quantity Input */}
                        {allowQuantity && isSelected ? (
                          <div className="px-2 pt-2 pb-1 w-full" onClick={(e) => e.stopPropagation()}>
                            <Input
                              type="text"
                              value={quantities[unitId] ? quantities[unitId].toLocaleString() : ''}
                              onChange={(e) => onQuantityChange(unitId, e.target.value)}
                              placeholder="Qty"
                              className="text-center font-mono h-8"
                            />
                          </div>
                        ) : (
                          <span className={`px-2 pt-2 pb-1 text-[11px] font-semibold text-center line-clamp-2 w-full ${isSelected ? 'text-primary' : 'text-text-main'}`}>
                            {name}
                          </span>
                        )}
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
    preselectedQuantities = {}
  } = options;

  // Selection state
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set(preselected));
  const [quantities, setQuantities] = useState<Record<number, number>>(preselectedQuantities);

  // Filter state
  const [searchQuery, setSearchQuery] = useState('');
  const initialTab = getTopFrequent(50).length > 0 ? 'frequent' : 'all';
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
  }, [isOpen]);

  // Get definitions and metadata for troops
  const definitions = TROOP_DEFINITIONS;
  const metadata = TROOP_METADATA;

  // Filter units by all criteria
  const filteredUnits = useMemo(() => {
    let entries = Object.entries(definitions);

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
  }, [definitions, metadata, searchQuery, typeFilter, roleFilter, foodFilter, quickAccessTab, favorites, frequentIds]);

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
        <div className="flex items-center gap-3">
          <div className="w-2 h-6 rounded-full bg-primary shadow-[0_0_10px_var(--color-primary-glow)]" />
          <span className="text-xl">
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
      <div className="mx-auto flex h-[calc(100vh-14rem)] w-full max-w-[1120px] flex-col overflow-hidden rounded-global border border-border-base bg-bg-app">
        {/* Filter Bar */}
        <div className="px-5 py-4 border-b border-border-base bg-bg-card-hover/40 shrink-0 space-y-4">
          {/* Row 1: Search + Quick Access */}
          <div className="flex flex-col md:flex-row items-center gap-4">
            <div className="w-full md:flex-1">
              <Input
                type="text"
                placeholder="Search by name or ID..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                leftIcon={<Search className="w-4 h-4" />}
              />
            </div>

            <ToggleGroup
              value={quickAccessTab}
              onChange={(v) => setQuickAccessTab(v as QuickAccessTab)}
              options={[
                { value: 'all', label: 'All', icon: <List className="w-3.5 h-3.5" /> },
                { value: 'favorites', label: 'Favorites', icon: <Heart className="w-3.5 h-3.5" /> },
                { value: 'frequent', label: 'Frequent', icon: <Flame className="w-3.5 h-3.5" /> }
              ]}
              className="w-full md:w-auto"
            />
          </div>

          {/* Row 2: Type + Role + Food filters */}
          <div className="flex items-center gap-3 flex-wrap">
            <ToggleGroup
              value={typeFilter}
              onChange={(v) => setTypeFilter(v as TypeFilter)}
              options={[
                { value: 'all', label: 'All Types' },
                { value: 'melee', label: 'Melee' },
                { value: 'range', label: 'Range' }
              ]}
              size="sm"
            />
            <ToggleGroup
              value={roleFilter}
              onChange={(v) => setRoleFilter(v as RoleFilter)}
              options={[
                { value: 'all', label: 'All Roles' },
                { value: 'attack', label: 'Attack' },
                { value: 'defense', label: 'Defense' }
              ]}
              size="sm"
            />
            <ToggleGroup
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

        {/* Unit Grid - Virtualized */}
        <VirtualizedUnitGrid
          filteredUnits={filteredUnits}
          selectedIds={selectedIds}
          favorites={favorites}
          quantities={quantities}
          allowQuantity={allowQuantity}
          quickAccessTab={quickAccessTab}
          onUnitClick={handleUnitClick}
          onFavoriteClick={handleFavoriteClick}
          onQuantityChange={handleQuantityChange}
        />
      </div>
    </Modal>
  );
};

export default TroopPickerModal;
