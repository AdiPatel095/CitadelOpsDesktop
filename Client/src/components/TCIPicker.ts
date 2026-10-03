import type { Dispatch, SetStateAction } from 'react';
import { fetchConstructionItemsCatalog } from './TCICatalogCache';
import { readViewerLocale } from '../i18n/viewerLocaleStore';

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

export const tciPickerBridge: {
  resolve: ((value: TCIPickerResult) => void) | null;
  setState: Dispatch<SetStateAction<{ isOpen: boolean; options: TCIPickerOptions | null }>> | null;
} = { resolve: null, setState: null };

export async function showTCIPicker(options: TCIPickerOptions): Promise<TCIPickerResult> {
  try {
    await fetchConstructionItemsCatalog(readViewerLocale());
  } catch {
    // Open the picker with its empty-state guidance when the catalog is unavailable.
  }
  return new Promise((resolve) => {
    tciPickerBridge.resolve = resolve;
    tciPickerBridge.setState?.({ isOpen: true, options });
  });
}
