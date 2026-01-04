/**
 * Unit Picker Storage
 * 
 * Manages local storage for favorites and frequently used units.
 */

const STORAGE_KEYS = {
    FAVORITES: 'unitPicker_favorites',
    FREQUENTLY_USED: 'unitPicker_frequentlyUsed',
};

// ============================================
// Favorites
// ============================================

/**
 * Get all favorited unit IDs
 */
export function getFavorites(): Set<number> {
    try {
        const stored = localStorage.getItem(STORAGE_KEYS.FAVORITES);
        if (stored) {
            const parsed = JSON.parse(stored) as number[];
            return new Set(parsed);
        }
    } catch (e) {
        console.error('Error reading favorites from storage:', e);
    }
    return new Set();
}

/**
 * Save favorites to local storage
 */
export function setFavorites(ids: Set<number>): void {
    try {
        localStorage.setItem(STORAGE_KEYS.FAVORITES, JSON.stringify(Array.from(ids)));
    } catch (e) {
        console.error('Error saving favorites to storage:', e);
    }
}

/**
 * Toggle favorite status for a unit
 * @returns true if now favorited, false if unfavorited
 */
export function toggleFavorite(id: number): boolean {
    const favorites = getFavorites();
    if (favorites.has(id)) {
        favorites.delete(id);
        setFavorites(favorites);
        return false;
    } else {
        favorites.add(id);
        setFavorites(favorites);
        return true;
    }
}

/**
 * Check if a unit is favorited
 */
export function isFavorite(id: number): boolean {
    return getFavorites().has(id);
}

// ============================================
// Frequently Used
// ============================================

/**
 * Get frequently used units as a map of unitId -> usage count
 */
export function getFrequentlyUsed(): Map<number, number> {
    try {
        const stored = localStorage.getItem(STORAGE_KEYS.FREQUENTLY_USED);
        if (stored) {
            const parsed = JSON.parse(stored) as Record<string, number>;
            const map = new Map<number, number>();
            Object.entries(parsed).forEach(([id, count]) => {
                map.set(parseInt(id), count);
            });
            return map;
        }
    } catch (e) {
        console.error('Error reading frequently used from storage:', e);
    }
    return new Map();
}

/**
 * Save frequently used to local storage
 */
function saveFrequentlyUsed(usage: Map<number, number>): void {
    try {
        const obj: Record<string, number> = {};
        usage.forEach((count, id) => {
            obj[id.toString()] = count;
        });
        localStorage.setItem(STORAGE_KEYS.FREQUENTLY_USED, JSON.stringify(obj));
    } catch (e) {
        console.error('Error saving frequently used to storage:', e);
    }
}

/**
 * Increment usage count for selected units
 * Call this when user confirms a selection
 */
export function incrementUsage(ids: number[]): void {
    const usage = getFrequentlyUsed();
    ids.forEach(id => {
        usage.set(id, (usage.get(id) || 0) + 1);
    });
    saveFrequentlyUsed(usage);
}

/**
 * Get top frequently used unit IDs, sorted by usage count
 * @param limit Maximum number to return (default: 20)
 */
export function getTopFrequent(limit: number = 20): number[] {
    const usage = getFrequentlyUsed();
    const sorted = Array.from(usage.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, limit)
        .map(([id]) => id);
    return sorted;
}

/**
 * Get usage count for a specific unit
 */
export function getUsageCount(id: number): number {
    return getFrequentlyUsed().get(id) || 0;
}
