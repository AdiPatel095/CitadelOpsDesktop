import { useCallback, useEffect, useState } from 'react';
import { goalById } from './goals';

/**
 * The chosen goal (CIT-19): a per-browser, per-account pointer to which automation the player is setting up. It holds
 * no configuration, credentials or progress; progress is always recomputed from observed state. A storage failure
 * (private window, blocked site data) means "no goal", never an error.
 */
export const GOAL_STORAGE_PREFIX = 'citadelops.goal.v1.';

export interface GoalRecord {
  version: 1;
  goalId: string;
  startedAt: string;
  /** Whether the checklist panel is collapsed for this goal. */
  collapsed?: boolean;
}

type GoalStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function browserStorage(): GoalStorage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

/** Used before the game has reported an account and world (disconnected first use); adopted by the first account that connects. */
export const PENDING_ACCOUNT_KEY = 'pending';

export const goalStorageKey = (accountKey: string): string => `${GOAL_STORAGE_PREFIX}${accountKey || PENDING_ACCOUNT_KEY}`;

// Held in memory when storage refuses, so the panel still works for this visit.
const memoryGoals = new Map<string, GoalRecord>();

export function readGoal(accountKey: string, storage: GoalStorage | null = browserStorage()): GoalRecord | null {
  const key = accountKey || PENDING_ACCOUNT_KEY;
  if (!storage) return memoryGoals.get(key) ?? null;
  try {
    const raw = storage.getItem(goalStorageKey(accountKey));
    if (!raw) return memoryGoals.get(key) ?? null;
    const parsed = JSON.parse(raw) as Partial<GoalRecord> | null;
    if (!parsed || parsed.version !== 1 || typeof parsed.goalId !== 'string' || !goalById(parsed.goalId)) return memoryGoals.get(key) ?? null;
    return {
      version: 1,
      goalId: parsed.goalId,
      startedAt: typeof parsed.startedAt === 'string' ? parsed.startedAt : '',
      ...(parsed.collapsed === true ? { collapsed: true } : {}),
    };
  } catch {
    return memoryGoals.get(key) ?? null;
  }
}

/** True when the record reached storage; otherwise it is kept in memory for this visit. */
export function writeGoal(accountKey: string, record: GoalRecord, storage: GoalStorage | null = browserStorage()): boolean {
  const key = accountKey || PENDING_ACCOUNT_KEY;
  if (storage) {
    try {
      storage.setItem(goalStorageKey(accountKey), JSON.stringify(record));
      memoryGoals.delete(key);
      return true;
    } catch {
      // Falls through to memory.
    }
  }
  memoryGoals.set(key, record);
  return false;
}

export function clearGoal(accountKey: string, storage: GoalStorage | null = browserStorage()): void {
  memoryGoals.delete(accountKey || PENDING_ACCOUNT_KEY);
  if (!storage) return;
  try {
    storage.removeItem(goalStorageKey(accountKey));
  } catch {
    // Nothing to clear when storage is unavailable.
  }
}

/**
 * A goal chosen before any account was known belongs to the first account that connects, unless that account already
 * has its own goal. Never moves a goal between two known accounts.
 */
export function adoptPendingGoal(accountKey: string, storage: GoalStorage | null = browserStorage()): void {
  if (!accountKey || readGoal(accountKey, storage)) return;
  const pending = readGoal('', storage);
  if (!pending) return;
  writeGoal(accountKey, pending, storage);
  clearGoal('', storage);
}

export function resetGoalMemoryForTests(): void {
  memoryGoals.clear();
}

// In-page subscribers, so the header button, the panel and the "Guide me through this" buttons agree at once.
const listeners = new Set<() => void>();
const notify = () => { for (const listener of listeners) listener(); };

export interface GoalApi {
  goal: GoalRecord | null;
  choose: (goalId: string) => void;
  clear: () => void;
  setCollapsed: (collapsed: boolean) => void;
}

/** The chosen goal for one account. `accountKey` is empty until the game reports the account and world. */
export function useGoal(accountKey: string): GoalApi {
  const [goal, setGoal] = useState<GoalRecord | null>(() => readGoal(accountKey));
  useEffect(() => {
    adoptPendingGoal(accountKey);
    setGoal(readGoal(accountKey));
    const listener = () => setGoal(readGoal(accountKey));
    listeners.add(listener);
    return () => { listeners.delete(listener); };
  }, [accountKey]);
  const choose = useCallback((goalId: string) => {
    if (!goalById(goalId)) return;
    writeGoal(accountKey, { version: 1, goalId, startedAt: new Date().toISOString() });
    notify();
  }, [accountKey]);
  const clear = useCallback(() => {
    clearGoal(accountKey);
    notify();
  }, [accountKey]);
  const setCollapsed = useCallback((collapsed: boolean) => {
    const current = readGoal(accountKey);
    if (!current) return;
    const next: GoalRecord = { ...current };
    if (collapsed) next.collapsed = true;
    else delete next.collapsed;
    writeGoal(accountKey, next);
    notify();
  }, [accountKey]);
  return { goal, choose, clear, setCollapsed };
}
