export const TIMER_TIP_STORAGE_KEY = 'citadelops.automation.timerTipDismissed';

export function timerTipDismissed(storage?: Pick<Storage, 'getItem'>): boolean {
  try {
    return storage?.getItem(TIMER_TIP_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

export function dismissTimerTip(storage?: Pick<Storage, 'setItem'>): boolean {
  if (!storage) return false;
  try {
    storage.setItem(TIMER_TIP_STORAGE_KEY, '1');
    return true;
  } catch {
    return false;
  }
}
