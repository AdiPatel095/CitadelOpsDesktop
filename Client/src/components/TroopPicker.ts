import type { Dispatch, SetStateAction } from 'react';
import type { CastleStateV2 } from '../api/Contracts';
import type { ObservationContext } from '../settings/requirements/observationFreshness';

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
  /** Hide units that the calling workflow owns or reserves. */
  excludedUnitIds?: number[];
  /** Optional in-castle stock counts shown on each unit card. */
  stockQuantities?: Record<number, number>;
  /**
   * Where `stockQuantities` come from and how current they are (CIT-20): counts are captioned "last known"
   * with the reason when the game connection is not current, and "as of <time>" when the game reports a
   * castle time. Selection never changes.
   */
  stockObservation?: { castle: Pick<CastleStateV2, 'unitsObservedAt'> | null; observation: ObservationContext };
}

// Result type varies based on options
export type TroopPickerResultSimple = number | number[] | null;
export type TroopPickerResultWithQuantity = UnitWithQuantity | UnitWithQuantity[] | null;
export type TroopPickerResult = TroopPickerResultSimple | TroopPickerResultWithQuantity;

export const troopPickerBridge: {
  resolve: ((value: TroopPickerResult) => void) | null;
  setState: Dispatch<SetStateAction<{ isOpen: boolean; options: TroopPickerOptions | null }>> | null;
} = { resolve: null, setState: null };

/**
 * Show the troop picker modal and return the selected troop(s).
 */
export function showTroopPicker(options: TroopPickerOptions): Promise<TroopPickerResult> {
  return new Promise((resolve) => {
    troopPickerBridge.resolve = resolve;
    if (troopPickerBridge.setState) {
      troopPickerBridge.setState({ isOpen: true, options });
    }
  });
}
