import { parseMessageDescriptor } from '../i18n/messageDescriptor';
import { useLocalizedMessages } from '../i18n/useLocalizedMessages';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocale as useStaticLocale } from '../i18n/useLocale';
import { useEffect, useSyncExternalStore } from 'react';
import { Icons } from './Icons';
import { NOTIFICATION_EXIT_MS, Notifications, type AppNotification, type VisibleNotification } from './Notifications';

export const Alerts = () => {
  const alerts = useSyncExternalStore(Notifications.subscribeVisible, Notifications.visible, Notifications.visible);

  useEffect(() => {
    Notifications.attach();
    return () => Notifications.detach();
  }, []);

  return (
    <div className="fixed top-24 right-6 z-50 flex w-96 max-w-[calc(100vw-3rem)] flex-col gap-3 pointer-events-none">
      {alerts.map((alert) => (
        <AlertItem key={alert.id} alert={alert} />
      ))}
    </div>
  );
};

const AlertItem = ({ alert }: { alert: VisibleNotification }) => {
  const { t: localizeStatic } = useStaticLocale();
  const { id, exiting } = alert;

  // The countdown lives in `Notifications`; the item only removes itself once the exit animation is over.
  useEffect(() => {
    if (!exiting) return;
    const timer = window.setTimeout(() => Notifications.remove(id), NOTIFICATION_EXIT_MS);
    return () => window.clearTimeout(timer);
  }, [id, exiting]);

  const handleDismiss = () => Notifications.dismiss(id);

  const localized = useLocalizedMessages([{descriptor:parseMessageDescriptor(alert.messageDescriptor),legacyText:alert.message},...(alert.lines ?? []).map((line,index)=>({descriptor:parseMessageDescriptor(alert.lineDescriptors?.[index]),legacyText:line})),{descriptor:parseMessageDescriptor(alert.action?.labelDescriptor),legacyText:alert.action?.label ?? ''}]);
  const style = alertStyles(alert.category);
  const hasLines = Boolean(alert.lines?.length);

  return (
    <div
      className={`pointer-events-auto relative flex items-start gap-3 overflow-hidden rounded-xl border p-4 ${style.bg} ${style.border} ${style.shadow} transition-all duration-300 ease-out ${exiting ? 'animate-fade-out-right' : 'animate-fade-in-right opacity-0'}`}
      role="alert"
      onMouseEnter={() => Notifications.pause(id, 'hover')}
      onMouseLeave={() => Notifications.resume(id, 'hover')}
      onFocus={() => Notifications.pause(id, 'focus')}
      onBlur={(event) => {
        if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
        Notifications.resume(id, 'focus');
      }}
    >
      <div className="mt-0.5 shrink-0">{style.icon}</div>
      <div className={`flex min-w-0 flex-1 flex-col gap-2 text-sm ${style.text} ${hasLines ? 'max-h-[min(70vh,28rem)] overflow-y-auto pr-1' : ''}`}>
        <div className="leading-snug" {...messageLanguageAttributes(localized[0])}>{localized[0].text}</div>
        {hasLines && (
          <ul className={`mt-0.5 list-inside list-disc space-y-1.5 pl-0.5 text-[13px] font-normal ${style.list}`}>
            {alert.lines?.map((line, index) => <li key={`${line}-${index}`} {...messageLanguageAttributes(localized[index+1])}>{localized[index+1].text}</li>)}
          </ul>
        )}
        {alert.action && (
          <button
            type="button"
            onClick={alert.action.onClick}
            className={`self-start rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors ${style.border} hover:bg-white/10`}
          >
            <span {...messageLanguageAttributes(localized[localized.length-1])}>{localized[localized.length-1].text}</span>
          </button>
        )}
      </div>
      <button
        type="button"
        onClick={handleDismiss}
        className={`shrink-0 rounded-lg p-1 opacity-70 transition-colors hover:bg-white/10 hover:opacity-100 ${style.text}`}
        aria-label={localizeStatic("ui.components.alerts.aria-label.dismiss.48845bff")}
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
