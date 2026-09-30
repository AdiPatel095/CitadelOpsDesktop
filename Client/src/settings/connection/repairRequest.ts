import { useSyncExternalStore } from 'react';

/**
 * Opens the connection repair over whatever is on screen (CIT-19): the checklist's "repair" action and every readiness
 * check that waits for the game connection ask for it here, and one host mounted in the application shell shows it.
 * The request is only "show the repair"; it changes no settings, drafts or connections. The host returns focus to the
 * control that opened it when it closes, and the editor underneath stays mounted with its draft untouched.
 */
let open = false;
const listeners = new Set<() => void>();

const notify = () => { for (const listener of listeners) listener(); };
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };

export function requestConnectionRepair(): void {
  if (open) return;
  open = true;
  notify();
}

export function closeConnectionRepair(): void {
  if (!open) return;
  open = false;
  notify();
}

export const isConnectionRepairOpen = (): boolean => open;

export function useConnectionRepairOpen(): boolean {
  return useSyncExternalStore(subscribe, isConnectionRepairOpen, isConnectionRepairOpen);
}
