export type NotificationCategory = 'green' | 'yellow' | 'red';

export interface AppNotification {
  id: string;
  category: NotificationCategory;
  message: string;
  lines?: string[];
  persistent?: boolean;
  action?: {
    label: string;
    onClick: () => void;
  };
}

type NotificationListener = (notification: AppNotification) => void;

class NotificationCenter {
  private listeners = new Set<NotificationListener>();

  subscribe(listener: NotificationListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  publish(input: Omit<AppNotification, 'id'> & { id?: string }): string {
    const notification: AppNotification = {
      ...input,
      id: input.id ?? this.nextID(),
    };
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

  private nextID(): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
}

export const Notifications = new NotificationCenter();
