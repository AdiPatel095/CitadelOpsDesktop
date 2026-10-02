// CIT-72 exposes the pure prepared helper as part of this component's public API.
/* eslint react-refresh/only-export-components: ["error", { "allowExportNames": ["TIMER_TIP_STORAGE_KEY"] }] */
import { useRef, useState, type JSX } from 'react';
import { Banner } from '../ui/Banner';
import { useLocale } from '../../i18n/LocaleContext';
import { dismissTimerTip, timerTipDismissed } from './timerTipStorage';
export { TIMER_TIP_STORAGE_KEY } from './timerTipStorage';

let dismissedForSession = false;
export function TimerTip(): JSX.Element | null {
  const { t } = useLocale();
  const tip = useRef<HTMLDivElement>(null);
  const [dismissed, setDismissed] = useState(() => {
    if (dismissedForSession) return true;
    try { return timerTipDismissed(window.localStorage); } catch { return false; }
  });
  if (dismissed) return null;
  const dismiss = () => {
    try { dismissTimerTip(window.localStorage); } catch { /* Storage access itself can throw. */ }
    dismissedForSession = true;
    const element = tip.current;
    const view = element?.closest('[data-view]');
    const next = view && Array.from(view.querySelectorAll<HTMLElement>(
      'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]'
    )).find(candidate => candidate.tabIndex >= 0 && candidate.getClientRects().length > 0
      && !candidate.closest('[inert]') && !element?.contains(candidate)
      && element && (element.compareDocumentPosition(candidate) & Node.DOCUMENT_POSITION_FOLLOWING));
    setDismissed(true);
    next?.focus();
  };
  return <div ref={tip} data-timer-tip><Banner tone="info" onDismiss={dismiss} dismissLabel={t('common.close')}>
    {t('automation.timerTip')}
  </Banner></div>;
}
