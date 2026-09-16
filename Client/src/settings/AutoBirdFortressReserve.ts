import {
  AUTO_FORTRESS_DIREWOLF_ID,
  parseAutoFortressClientState,
} from './AutoFortressClientState';

export interface AutoBirdReserveItem {
  id: number;
  amount: number;
}

export interface AutoBirdReserveCastle {
  kingdomId: number;
  slotType?: number;
}

export function autoFortressReservesDirewolves(
  globalEnabled: boolean,
  castle: AutoBirdReserveCastle | null | undefined,
  rawAutoFortress: unknown,
): boolean {
  if (!globalEnabled || castle?.slotType !== 12 || castle.kingdomId < 1 || castle.kingdomId > 3) {
    return false;
  }
  return parseAutoFortressClientState(rawAutoFortress).kingdoms[String(castle.kingdomId)]?.enabled === true;
}

export function visibleAutoBirdReserveItems(
  items: AutoBirdReserveItem[],
  fortressProtected: boolean,
): AutoBirdReserveItem[] {
  return fortressProtected
    ? items.filter((item) => item.id !== AUTO_FORTRESS_DIREWOLF_ID)
    : items;
}

export function mergeAutoBirdPickerItems(
  currentItems: AutoBirdReserveItem[],
  pickedItems: AutoBirdReserveItem[],
  fortressProtected: boolean,
): AutoBirdReserveItem[] {
  if (!fortressProtected) return pickedItems;
  const manualDirewolf = currentItems.find((item) => item.id === AUTO_FORTRESS_DIREWOLF_ID);
  const editable = pickedItems.filter((item) => item.id !== AUTO_FORTRESS_DIREWOLF_ID);
  return manualDirewolf ? [...editable, manualDirewolf] : editable;
}
