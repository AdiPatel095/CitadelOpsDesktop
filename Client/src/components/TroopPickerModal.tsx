import React, { useState, useMemo, useCallback, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { X, Search, Check, Heart, List, Flame } from 'lucide-react';
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
    const [quickAccessTab, setQuickAccessTab] = useState<QuickAccessTab>('all');
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
                    onClose({ unitId, quantity: quantities[unitId] || 1 });
                } else {
                    onClose(null);
                }
            } else {
                const selected = selectedArray.map(unitId => ({
                    unitId,
                    quantity: quantities[unitId] || 1
                }));
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

    const modalContent = (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm animate-fade-in">
            <div
                className="w-full max-w-5xl max-h-[90vh] bg-bg-card border border-border-base shadow-2xl flex flex-col overflow-hidden"
                style={{ borderRadius: 'var(--radius-global)' }}
            >
                {/* Header */}
                <div className="h-16 border-b border-border-base flex items-center justify-between px-6 bg-bg-app/50 shrink-0">
                    <div className="flex items-center gap-3">
                        <div className="w-2 h-8 rounded-full bg-primary shadow-[0_0_10px] shadow-primary/50" />
                        <h2 className="heading-1">
                            {title || (mode === 'single' ? 'Select Troop' : 'Select Troops')}
                        </h2>
                        {mode === 'multi' && selectedIds.size > 0 && (
                            <span className="px-2 py-1 rounded-full bg-primary/20 text-primary text-sm font-bold">
                                {selectedIds.size} selected
                            </span>
                        )}
                    </div>
                    <button
                        onClick={handleCancel}
                        className="w-10 h-10 rounded-full flex items-center justify-center hover:bg-white/10 transition-colors"
                    >
                        <X className="w-6 h-6 text-text-muted hover:text-text-main" />
                    </button>
                </div>

                {/* Filter Bar */}
                <div className="px-6 py-4 border-b border-border-base bg-bg-app/30 shrink-0 space-y-3">
                    {/* Row 1: Type + Role + Food filters */}
                    <div className="flex items-center gap-3 flex-wrap">
                        {/* Type Filter (Melee/Range) */}
                        <div className="toggle-container">
                            <button
                                onClick={() => setTypeFilter('all')}
                                className={`toggle-btn ${typeFilter === 'all' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                All
                            </button>
                            <button
                                onClick={() => setTypeFilter('melee')}
                                className={`toggle-btn ${typeFilter === 'melee' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Melee
                            </button>
                            <button
                                onClick={() => setTypeFilter('range')}
                                className={`toggle-btn ${typeFilter === 'range' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Range
                            </button>
                        </div>

                        {/* Role Filter (Attack/Defense) */}
                        <div className="toggle-container">
                            <button
                                onClick={() => setRoleFilter('all')}
                                className={`toggle-btn ${roleFilter === 'all' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                All
                            </button>
                            <button
                                onClick={() => setRoleFilter('attack')}
                                className={`toggle-btn ${roleFilter === 'attack' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Attack
                            </button>
                            <button
                                onClick={() => setRoleFilter('defense')}
                                className={`toggle-btn ${roleFilter === 'defense' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Defense
                            </button>
                        </div>

                        {/* Food Filter (Mead/Beef/Food) */}
                        <div className="toggle-container">
                            <button
                                onClick={() => setFoodFilter('all')}
                                className={`toggle-btn ${foodFilter === 'all' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                All
                            </button>
                            <button
                                onClick={() => setFoodFilter('mead')}
                                className={`toggle-btn ${foodFilter === 'mead' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Mead
                            </button>
                            <button
                                onClick={() => setFoodFilter('beef')}
                                className={`toggle-btn ${foodFilter === 'beef' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Beef
                            </button>
                            <button
                                onClick={() => setFoodFilter('food')}
                                className={`toggle-btn ${foodFilter === 'food' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                Food
                            </button>
                        </div>
                    </div>

                    {/* Row 2: Search + Quick Access */}
                    <div className="flex items-center gap-4">
                        {/* Search */}
                        <div className="flex-1 relative">
                            <Search className="w-5 h-5 absolute left-4 top-1/2 -translate-y-1/2 text-text-muted" />
                            <input
                                type="text"
                                placeholder="Search by name or ID..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="input-field pl-12"
                            />
                        </div>

                        {/* Quick Access Tabs */}
                        <div className="toggle-container">
                            <button
                                onClick={() => setQuickAccessTab('all')}
                                className={`toggle-btn flex items-center gap-1.5 ${quickAccessTab === 'all' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                <List className="w-4 h-4" />
                                All
                            </button>
                            <button
                                onClick={() => setQuickAccessTab('favorites')}
                                className={`toggle-btn flex items-center gap-1.5 ${quickAccessTab === 'favorites' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                <Heart className="w-4 h-4" />
                                Favorites
                            </button>
                            <button
                                onClick={() => setQuickAccessTab('frequent')}
                                className={`toggle-btn flex items-center gap-1.5 ${quickAccessTab === 'frequent' ? 'toggle-btn-active' : 'toggle-btn-inactive'}`}
                            >
                                <Flame className="w-4 h-4" />
                                Frequent
                            </button>
                        </div>
                    </div>
                </div>

                {/* Unit Grid */}
                <div className="flex-1 overflow-y-auto p-6">
                    {filteredUnits.length === 0 ? (
                        <div className="text-center py-12 text-text-muted">
                            <p className="text-lg">No units found</p>
                            <p className="text-sm mt-1">
                                {quickAccessTab === 'favorites'
                                    ? 'Click the heart icon on units to add favorites'
                                    : quickAccessTab === 'frequent'
                                        ? 'Select units to build your frequently used list'
                                        : 'Try adjusting your filters'}
                            </p>
                        </div>
                    ) : (
                        <div className="grid grid-cols-4 sm:grid-cols-5 md:grid-cols-6 lg:grid-cols-8 gap-4">
                            {filteredUnits.map(([idStr, name]) => {
                                const unitId = parseInt(idStr);
                                const isSelected = selectedIds.has(unitId);
                                const isFav = favorites.has(unitId);

                                return (
                                    <div
                                        key={unitId}
                                        onClick={() => handleUnitClick(unitId)}
                                        role="button"
                                        tabIndex={0}
                                        className={`
                                            relative flex flex-col items-center rounded-xl transition-all duration-200
                                            border-2 hover:scale-105 overflow-hidden cursor-pointer
                                            ${isSelected
                                                ? 'border-primary bg-primary/10 shadow-lg shadow-primary/20'
                                                : 'border-border-base bg-bg-card hover:border-primary/50 hover:bg-bg-card-hover'
                                            }
                                        `}
                                    >
                                        {/* Top-right icons container */}
                                        <div className="absolute top-1 right-1 flex flex-col gap-1 z-10">
                                            {/* Favorite button */}
                                            <button
                                                onClick={(e) => handleFavoriteClick(e, unitId)}
                                                className={`w-6 h-6 rounded-full flex items-center justify-center transition-all ${isFav
                                                    ? 'bg-red-500 text-white'
                                                    : 'bg-black/40 text-white/60 hover:bg-red-500/50 hover:text-white'
                                                    }`}
                                            >
                                                <Heart className={`w-3.5 h-3.5 ${isFav ? 'fill-current' : ''}`} />
                                            </button>

                                            {/* Selection indicator */}
                                            {isSelected && (
                                                <div className="w-6 h-6 rounded-full bg-primary flex items-center justify-center">
                                                    <Check className="w-3.5 h-3.5 text-bg-app" />
                                                </div>
                                            )}
                                        </div>

                                        {/* Unit Image - edge to edge */}
                                        <div className="w-full aspect-square flex items-center justify-center pt-2">
                                            <UnitImage unitId={unitId} size={80} showLevel={true} />
                                        </div>

                                        {/* Bottom area: Name or Quantity Input */}
                                        {allowQuantity && isSelected ? (
                                            <div className="px-2 pb-2 pt-1 w-full">
                                                <input
                                                    type="text"
                                                    value={quantities[unitId] ? quantities[unitId].toLocaleString() : ''}
                                                    onChange={(e) => {
                                                        e.stopPropagation();
                                                        handleQuantityChange(unitId, e.target.value);
                                                    }}
                                                    onClick={(e) => e.stopPropagation()}
                                                    placeholder="Qty"
                                                    className="w-full px-2 py-1 text-xs text-center bg-bg-app border border-border-base rounded-global focus:border-primary focus:outline-none text-text-main"
                                                />
                                            </div>
                                        ) : (
                                            <span className="px-2 pb-2 pt-1 text-xs font-medium text-text-main text-center line-clamp-2 w-full">
                                                {name}
                                            </span>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>

                {/* Footer */}
                <div className="h-20 border-t border-border-base flex items-center justify-end px-6 bg-bg-app/50 gap-4 shrink-0">
                    <button
                        onClick={handleCancel}
                        className="px-6 py-2.5 rounded-global text-text-muted hover:text-text-main font-bold transition-colors"
                    >
                        CANCEL
                    </button>
                    <button
                        onClick={handleConfirm}
                        disabled={selectedIds.size === 0}
                        className={`
                            px-8 py-2.5 rounded-global font-bold transition-all flex items-center gap-2
                            ${selectedIds.size > 0
                                ? 'bg-primary text-bg-app hover:brightness-110 shadow-lg shadow-primary/20 hover:shadow-primary/40 active:scale-95'
                                : 'bg-border-base text-text-muted cursor-not-allowed'
                            }
                        `}
                    >
                        <Check className="w-5 h-5" />
                        CONFIRM
                    </button>
                </div>
            </div>
        </div>
    );

    return createPortal(modalContent, document.body);
}; export default TroopPickerModal;
