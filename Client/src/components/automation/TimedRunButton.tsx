// CIT-72 exposes the pure prepared helper as part of this component's public API.
/* eslint react-refresh/only-export-components: ["error", { "allowExportNames": ["timedRunState"] }] */
import type { JSX } from 'react';
import { Timer } from 'lucide-react';
import { useLocale } from '../../i18n/LocaleContext';
import { Button } from '../ui/Button';
import { timedRunState } from './timedRun';
export { timedRunState, type TimedRunState } from './timedRun';

export function TimedRunButton({ featureName, expiresAt, now, disabled, onOpen }: {
  featureName: string; expiresAt?: number; now: number; disabled?: boolean; onOpen(): void;
}): JSX.Element {
  const { locale, t } = useLocale();
  const state = timedRunState(expiresAt, now, locale);
  const label = state.kind === 'active'
    ? t('automation.timer.active', { duration: state.duration, feature: featureName })
    : t('automation.timer.run', { feature: featureName });
  return (
    <Button variant={state.kind === 'active' ? 'secondary' : 'ghost'}
      size={state.kind === 'active' ? 'sm' : 'md'} iconOnly={state.kind === 'idle'}
      className="automation-timer" data-timed-run={state.kind} disabled={disabled}
      aria-label={label} title={label} onClick={onOpen} leftIcon={<Timer aria-hidden="true" />}>
      {state.kind === 'active' ? t('automation.timeLeft', { duration: state.duration }) : null}
    </Button>
  );
}
