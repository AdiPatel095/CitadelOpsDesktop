import { useEffect, useState } from 'react';
import { Icons } from './Icons';
import { Notifications, type AppNotification } from './Notifications';

const ALERT_DURATION_MS = 5000;

export const Alerts = () => {
  const [alerts, setAlerts] = useState<AppNotification[]>([]);

  useEffect(() => Notifications.subscribe((notification) => {
    setAlerts((current) => [
      ...current.filter((item) => item.id !== notification.id),
      notification,
    ]);
  }), []);

  return (
    <div className="fixed top-24 right-6 z-50 flex w-96 max-w-[calc(100vw-3rem)] flex-col gap-3 pointer-events-none">
      {alerts.map((alert) => (
        <AlertItem
          key={alert.id}
          alert={alert}
          onDismiss={() => setAlerts((current) => current.filter((item) => item.id !== alert.id))}
        />
      ))}
    </div>
  );
};

const AlertItem = ({ alert, onDismiss }: { alert: AppNotification; onDismiss: () => void }) => {
  const [isExiting, setIsExiting] = useState(false);

  const handleDismiss = () => {
    setIsExiting(true);
    window.setTimeout(onDismiss, 300);
  };

  useEffect(() => {
    if (alert.persistent) return;
    const timer = window.setTimeout(handleDismiss, ALERT_DURATION_MS);
    return () => window.clearTimeout(timer);
    // The timer intentionally starts only when this notification is mounted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [alert.id, alert.persistent]);

  const style = alertStyles(alert.category);
  const hasLines = Boolean(alert.lines?.length);

  return (
    <div
      className={`pointer-events-auto relative flex items-start gap-3 overflow-hidden rounded-xl border p-4 backdrop-blur-md ${style.bg} ${style.border} ${style.shadow} transition-all duration-300 ease-out ${isExiting ? 'animate-fade-out-right' : 'animate-fade-in-right opacity-0'}`}
      role="alert"
    >
      <div className="mt-0.5 shrink-0">{style.icon}</div>
      <div className={`flex min-w-0 flex-1 flex-col gap-2 text-sm ${style.text} ${hasLines ? 'max-h-[min(70vh,28rem)] overflow-y-auto pr-1' : ''}`}>
        <div className="leading-snug">{alert.message}</div>
        {hasLines && (
          <ul className={`mt-0.5 list-inside list-disc space-y-1.5 pl-0.5 text-[13px] font-normal ${style.list}`}>
            {alert.lines?.map((line, index) => <li key={`${line}-${index}`}>{line}</li>)}
          </ul>
        )}
        {alert.action && (
          <button
            type="button"
            onClick={alert.action.onClick}
            className={`self-start rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors ${style.border} hover:bg-white/10`}
          >
            {alert.action.label}
          </button>
        )}
      </div>
      <button
        type="button"
        onClick={handleDismiss}
        className={`shrink-0 rounded-lg p-1 opacity-70 transition-colors hover:bg-white/10 hover:opacity-100 ${style.text}`}
        aria-label="Dismiss"
      >
        <Icons.X className="h-4 w-4" />
      </button>
    </div>
  );
};

function alertStyles(category: AppNotification['category']) {
  switch (category) {
    case 'green':
      return {
        bg: 'bg-emerald-500/10 dark:bg-emerald-500/20',
        border: 'border-emerald-500/20 dark:border-emerald-500/50',
        text: 'text-emerald-950 dark:text-white font-semibold',
        list: 'text-emerald-950/90 dark:text-white/95',
        icon: <Icons.Check className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />,
        shadow: 'shadow-[0_0_15px_rgba(16,185,129,0.1)] dark:shadow-[0_0_15px_rgba(16,185,129,0.2)]',
      };
    case 'red':
      return {
        bg: 'bg-red-500/10 dark:bg-red-500/20',
        border: 'border-red-500/20 dark:border-red-500/50',
        text: 'text-red-950 dark:text-white font-semibold',
        list: 'text-red-950/90 dark:text-red-100',
        icon: <Icons.AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400" />,
        shadow: 'shadow-[0_0_15px_rgba(239,68,68,0.1)] dark:shadow-[0_0_15px_rgba(239,68,68,0.2)]',
      };
    default:
      return {
        bg: 'bg-amber-500/10 dark:bg-amber-500/20',
        border: 'border-amber-500/20 dark:border-amber-500/50',
        text: 'text-amber-950 dark:text-white font-semibold',
        list: 'text-amber-950/90 dark:text-white/95',
        icon: <Icons.AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400" />,
        shadow: 'shadow-[0_0_15px_rgba(245,158,11,0.1)] dark:shadow-[0_0_15px_rgba(245,158,11,0.2)]',
      };
  }
}
