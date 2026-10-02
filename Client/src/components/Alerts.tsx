import { Button } from './ui/Button';
import { parseMessageDescriptor } from '../i18n/messageDescriptor';
import { useLocalizedMessages } from '../i18n/useLocalizedMessages';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocale as useStaticLocale } from "../i18n/LocaleContext";
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
      className={`motion-reduce:opacity-100 pointer-events-auto relative flex items-start gap-3 overflow-hidden rounded-xl border p-4 ${style.bg} ${style.border} ${style.shadow} transition-all duration-300 ease-out ${exiting ? 'animate-fade-out-right' : 'animate-fade-in-right opacity-0'}`}
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
      <div className={`flex min-w-0 flex-1 flex-col gap-2 text-body ${style.text} ${hasLines ? 'max-h-[min(70vh,28rem)] overflow-y-auto pr-1' : ''}`}>
        <div className="leading-snug" {...messageLanguageAttributes(localized[0])}>{localized[0].text}</div>
        {hasLines && (
          <ul className={`mt-0.5 list-inside list-disc space-y-1.5 pl-0.5 text-body-sm font-normal ${style.list}`}>
            {alert.lines?.map((line, index) => <li key={`${line}-${index}`} {...messageLanguageAttributes(localized[index+1])}>{localized[index+1].text}</li>)}
          </ul>
        )}
        {alert.action && (
          <Button variant="secondary"
            type="button"
            onClick={alert.action.onClick}
            className="self-start"
          >
            <span {...messageLanguageAttributes(localized[localized.length-1])}>{localized[localized.length-1].text}</span>
          </Button>
        )}
      </div>
      <Button iconOnly variant="ghost"
        type="button"
        onClick={handleDismiss}
        className="shrink-0"
        aria-label={localizeStatic("ui.components.alerts.aria-label.dismiss.48845bff")}
      >
        <Icons.X className="h-4 w-4" />
      </Button>
    </div>
  );
};

function alertStyles(category: AppNotification['category']) {
  switch (category) {
    case 'green':
      return {
        bg: 'bg-[var(--status-success-bg)]',
        border: 'border-[var(--status-success-border)]',
        text: 'text-[var(--status-success)] font-semibold',
        list: 'text-[var(--status-success)]',
        icon: <Icons.Check className="h-5 w-5 text-[var(--status-success)]" />,
        shadow: 'shadow-md',
      };
    case 'red':
      return {
        bg: 'bg-[var(--status-danger-bg)]',
        border: 'border-[var(--status-danger-border)]',
        text: 'text-[var(--status-danger)] font-semibold',
        list: 'text-[var(--status-danger)]',
        icon: <Icons.AlertCircle className="h-5 w-5 text-[var(--status-danger)]" />,
        shadow: 'shadow-md',
      };
    default:
      return {
        bg: 'bg-[var(--status-warning-bg)]',
        border: 'border-[var(--status-warning-border)]',
        text: 'text-[var(--status-warning)] font-semibold',
        list: 'text-[var(--status-warning)]',
        icon: <Icons.AlertTriangle className="h-5 w-5 text-[var(--status-warning)]" />,
        shadow: 'shadow-md',
      };
  }
}
