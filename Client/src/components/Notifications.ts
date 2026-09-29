import type { LocalizedMessage } from '../i18n/formatMessage';
export type NotificationCategory = 'green' | 'yellow' | 'red';

/**
 * Notification timing (CIT-58). This file and `Alerts.tsx` are kept byte-identical
 * in the desktop client and the hosted portal.
 *
 * - `NOTIFICATION_DURATION_MS`: default time a non-persistent notification stays
 *   visible. It counts only time the notification is actually shown.
 * - `NOTIFICATION_MINIMUM_VISIBLE_MS`: hard floor. A notification never removes
 *   itself sooner than this after it first appeared, even when a shorter
 *   duration is configured, and an update to the same id never shortens it.
 * - `NOTIFICATION_EXIT_MS`: length of the exit animation after a dismissal.
 *
 * The countdown is owned by `NotificationCenter`, not by the component, so a
 * same-id republish or a remount of the view cannot cut it short. It pauses
 * while the notification is hovered or focused, and while no `Alerts` view is
 * attached. Manual dismissal is always immediate.
 */
export const NOTIFICATION_DURATION_MS = 30_000;
export const NOTIFICATION_MINIMUM_VISIBLE_MS = 10_000;
export const NOTIFICATION_EXIT_MS = 300;

export function notificationDurationMs(category: NotificationCategory): number {
  void category;
  return NOTIFICATION_DURATION_MS;
}

export interface AppNotification {
  id: string;
  revision: number;
  category: NotificationCategory;
  message: string;
  messageDescriptor?: LocalizedMessage;
  lineDescriptors?: Array<LocalizedMessage | undefined>;
  lines?: string[];
  persistent?: boolean;
  action?: {
    label: string;
    labelDescriptor?: LocalizedMessage;
    onClick: () => void;
  };
}

/** A notification as held by the center. `shownAt` is null until a view has displayed it. */
export type VisibleNotification = AppNotification & { shownAt: number | null; exiting: boolean };

export type NotificationPauseReason = 'hover' | 'focus' | 'hidden';

type NotificationListener = (notification: AppNotification) => void;
type VisibleListener = () => void;

export interface NotificationCenterOptions {
  now?: () => number;
  setTimeout?: (callback: () => void, delayMs: number) => unknown;
  clearTimeout?: (handle: unknown) => void;
  durationMs?: (category: NotificationCategory) => number;
}

interface NotificationEntry {
  item: VisibleNotification;
  /** Wall-clock removal time while the countdown is armed; null otherwise. */
  deadline: number | null;
  /** Visible time left while the countdown is not armed; null when not applicable. */
  remaining: number | null;
  timer: unknown;
  pauses: Set<NotificationPauseReason>;
}

const NO_NOTIFICATIONS: readonly VisibleNotification[] = Object.freeze([]);

export class NotificationCenter {
  private listeners = new Set<NotificationListener>();
  private visibleListeners = new Set<VisibleListener>();
  private entries = new Map<string, NotificationEntry>();
  private snapshot: readonly VisibleNotification[] = NO_NOTIFICATIONS;
  private revision = 0;
  private attached = 0;
  private scopeKey: string | null = null;
  private readonly clock: () => number;
  private readonly schedule: (callback: () => void, delayMs: number) => unknown;
  private readonly cancel: (handle: unknown) => void;
  private readonly durationFor: (category: NotificationCategory) => number;

  constructor(options: NotificationCenterOptions = {}) {
    this.clock = options.now ?? (() => Date.now());
    this.schedule = options.setTimeout ?? ((callback, delayMs) => globalThis.setTimeout(callback, delayMs));
    this.cancel = options.clearTimeout ?? ((handle) => globalThis.clearTimeout(handle as ReturnType<typeof setTimeout>));
    this.durationFor = options.durationMs ?? notificationDurationMs;
  }

  subscribe(listener: NotificationListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /** Change subscription for `useSyncExternalStore`; pair it with `visible`. */
  subscribeVisible = (listener: VisibleListener): (() => void) => {
    this.visibleListeners.add(listener);
    return () => { this.visibleListeners.delete(listener); };
  };

  /** Current notifications in insertion order; the array identity changes only when something changed. */
  visible = (): readonly VisibleNotification[] => this.snapshot;

  publish(input: Omit<AppNotification, 'id' | 'revision'> & { id?: string }): string {
    const notification: AppNotification = {
      ...input,
      id: input.id ?? this.nextID(),
      revision: ++this.revision,
    };
    const now = this.clock();
    const existing = this.entries.get(notification.id);
    if (existing) {
      // Same id: update in place. Never remount, never shorten the countdown.
      existing.item = { ...notification, shownAt: existing.item.shownAt, exiting: false };
      this.extend(existing, now);
    } else {
      const shown = this.attached > 0;
      const entry: NotificationEntry = {
        item: { ...notification, shownAt: shown ? now : null, exiting: false },
        deadline: null,
        remaining: null,
        timer: null,
        pauses: new Set<NotificationPauseReason>(shown ? [] : ['hidden']),
      };
      this.entries.set(notification.id, entry);
      if (shown) this.begin(entry);
    }
    this.changed();
    for (const listener of this.listeners) listener(notification);
    return notification.id;
  }

  success(message: string, id?: string): string {
    return this.publish({ id, category: 'green', message });
  }

  warning(message: string, id?: string): string {
    return this.publish({ id, category: 'yellow', message });
  }

  error(message: string, id?: string): string {
    return this.publish({ id, category: 'red', message });
  }

  /** Start the exit animation immediately and stop the countdown. The view calls `remove` afterwards. */
  dismiss(id: string): void {
    const entry = this.entries.get(id);
    if (!entry || entry.item.exiting) return;
    this.disarm(entry);
    entry.remaining = null;
    entry.item = { ...entry.item, exiting: true };
    this.changed();
  }

  remove(id: string): void {
    const entry = this.entries.get(id);
    if (!entry) return;
    this.disarm(entry);
    this.entries.delete(id);
    this.changed();
  }

  pause(id: string, reason: NotificationPauseReason): void {
    const entry = this.entries.get(id);
    if (!entry || entry.item.exiting) return;
    this.holdCountdown(entry, reason);
  }

  resume(id: string, reason: NotificationPauseReason): void {
    const entry = this.entries.get(id);
    if (!entry) return;
    entry.pauses.delete(reason);
    this.settle(entry);
  }

  /** A view started showing notifications. The first view starts the countdown of anything not yet shown. */
  attach(): void {
    this.attached += 1;
    if (this.attached !== 1) return;
    const now = this.clock();
    let changed = false;
    for (const entry of this.entries.values()) {
      entry.pauses.delete('hidden');
      if (entry.item.shownAt === null) {
        entry.item = { ...entry.item, shownAt: now };
        this.begin(entry);
        changed = true;
      } else {
        this.settle(entry);
      }
    }
    if (changed) this.changed();
  }

  /** A view stopped showing notifications. While none is attached, every countdown is paused. */
  detach(): void {
    if (this.attached === 0) return;
    this.attached -= 1;
    if (this.attached !== 0) return;
    for (const entry of this.entries.values()) {
      // The elements are gone, so no mouse-leave or blur will arrive for them.
      entry.pauses.delete('hover');
      entry.pauses.delete('focus');
      this.holdCountdown(entry, 'hidden');
    }
  }

  /**
   * Identify the scope (the hosted account) the list belongs to. Only a change
   * of scope clears the list; the first call just adopts the scope.
   */
  setScope(key: string): void {
    if (key === this.scopeKey) return;
    const previous = this.scopeKey;
    this.scopeKey = key;
    if (previous === null) return;
    for (const entry of this.entries.values()) this.disarm(entry);
    if (this.entries.size === 0) return;
    this.entries.clear();
    this.changed();
  }

  private nextID(): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }

  private changed(): void {
    this.snapshot = Object.freeze(Array.from(this.entries.values(), (entry) => entry.item));
    for (const listener of Array.from(this.visibleListeners)) listener();
  }

  /** First display: a full duration, never less than the floor. */
  private begin(entry: NotificationEntry): void {
    entry.remaining = entry.item.persistent
      ? null
      : Math.max(this.durationFor(entry.item.category), NOTIFICATION_MINIMUM_VISIBLE_MS);
    this.settle(entry);
  }

  /** Same-id update: the countdown may be extended, never shortened. */
  private extend(entry: NotificationEntry, now: number): void {
    const item = entry.item;
    if (item.persistent) {
      this.disarm(entry);
      entry.remaining = null;
      return;
    }
    if (item.shownAt === null) return; // not shown yet; `attach` starts a full countdown
    const duration = this.durationFor(item.category);
    const floorAt = item.shownAt + NOTIFICATION_MINIMUM_VISIBLE_MS;
    if (entry.deadline !== null) {
      this.arm(entry, Math.max(entry.deadline, now + duration, floorAt) - now);
      return;
    }
    entry.remaining = Math.max(entry.remaining ?? 0, duration, floorAt - now);
    this.settle(entry);
  }

  /** Arm or hold the countdown to match the entry's state and pause reasons. */
  private settle(entry: NotificationEntry): void {
    const item = entry.item;
    if (item.persistent || item.exiting || item.shownAt === null) {
      this.disarm(entry);
      return;
    }
    if (entry.pauses.size > 0) {
      this.holdCountdown(entry);
      return;
    }
    if (entry.deadline === null) {
      const remaining = entry.remaining
        ?? Math.max(this.durationFor(item.category), NOTIFICATION_MINIMUM_VISIBLE_MS);
      this.arm(entry, remaining);
    }
  }

  private holdCountdown(entry: NotificationEntry, reason?: NotificationPauseReason): void {
    if (reason) entry.pauses.add(reason);
    if (entry.deadline === null) return;
    entry.remaining = Math.max(0, entry.deadline - this.clock());
    this.disarm(entry);
  }

  private arm(entry: NotificationEntry, delayMs: number): void {
    this.disarm(entry);
    const delay = Math.max(0, delayMs);
    const id = entry.item.id;
    entry.deadline = this.clock() + delay;
    entry.remaining = null;
    entry.timer = this.schedule(() => {
      if (this.entries.get(id) !== entry) return;
      entry.timer = null;
      entry.deadline = null;
      this.dismiss(id);
    }, delay);
  }

  private disarm(entry: NotificationEntry): void {
    if (entry.timer !== null) this.cancel(entry.timer);
    entry.timer = null;
    entry.deadline = null;
  }
}

export const Notifications = new NotificationCenter();
