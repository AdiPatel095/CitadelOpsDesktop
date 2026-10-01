export const TABS_SELECT_MIN_ITEMS = 4;
export const SEGMENTED_MAX_OPTIONS = 5;

export function tabsMode(count: number, isMedium: boolean): 'tabs' | 'select' {
  return !isMedium && count >= TABS_SELECT_MIN_ITEMS ? 'select' : 'tabs';
}

export function segmentedMode(count: number): 'segmented' | 'select' {
  return count > SEGMENTED_MAX_OPTIONS ? 'select' : 'segmented';
}

export function nextTabIndex(key: string, index: number, items: readonly { disabled?: boolean }[], rtl: boolean): number {
  if (key === 'Home') {
    const first = items.findIndex((item) => !item.disabled);
    return first < 0 ? index : first;
  }
  if (key === 'End') {
    for (let i = items.length - 1; i >= 0; i -= 1) {
      if (!items[i].disabled) return i;
    }
    return index;
  }
  if (key !== 'ArrowRight' && key !== 'ArrowLeft') return index;
  const direction = (key === 'ArrowRight' ? 1 : -1) * (rtl ? -1 : 1);
  for (let step = 1; step <= items.length; step += 1) {
    const next = ((index + direction * step) % items.length + items.length) % items.length;
    if (!items[next].disabled) return next;
  }
  return index;
}
