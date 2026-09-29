import React, { useState } from 'react';
import { CalendarDays, CircleStop, Timer } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { useAuth } from '../../context/AuthContext';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import { configurationSection } from '../Configuration';
import { normalizeFeatureSchedules, scheduleSummary } from '../SchedulerTypes';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../disclosure/placement';

export interface AutomationRunStripProps {
  featureId: SettingsFeatureId;
  /** Scheduler id when the feature supports a weekly schedule. */
  scheduleId?: string;
  onOpenSchedule?: () => void;
  onOpenDuration?: () => void;
}

/**
 * Essentials run line of every settings modal (CIT-17): whether the
 * automation is running, its schedule, and a Stop that is reachable without
 * expanding anything. Stop writes `automation.enabled` only when clicked and
 * never touches the settings draft; saving the modal never starts or stops
 * the automation.
 */
export const AutomationRunStrip: React.FC<AutomationRunStripProps> = ({ featureId, scheduleId, onOpenSchedule, onOpenDuration }) => {
  const { automationEnabledByKey, automationTimedUntilByKey, setAutomationEnabled } = useAuth();
  const { configuration } = useCitadelAPI();
  const { date, t } = useLocale();
  const [stopping, setStopping] = useState(false);
  const [stopError, setStopError] = useState('');
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const running = automationEnabledByKey[enabledKey] === true;
  const timedUntil = automationTimedUntilByKey[enabledKey];
  const schedule = scheduleId
    ? normalizeFeatureSchedules(configurationSection(configuration, 'scheduler').featureSchedules)[scheduleId]
    : undefined;

  const stop = async () => {
    if (stopping) return;
    setStopping(true);
    setStopError('');
    try {
      await setAutomationEnabled(enabledKey, false);
    } catch (error) {
      const fallback = t('ui.settings.components.automationRunStrip.could.not.stop.this.automation.it.is.683025c4');
      // Always say the automation is still running; add the reason when the write reports one.
      setStopError(error instanceof Error && error.message.trim() ? `${fallback} ${error.message}` : fallback);
    } finally {
      setStopping(false);
    }
  };

  const state = !running ? 'stopped' : timedUntil ? 'timed' : 'running';
  return (
    <div className="mb-4 rounded-global border border-border-base bg-bg-card/40 px-4 py-3" data-settings-run-strip={featureId}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge variant={running ? 'success' : 'outline'} className="normal-case tracking-normal">
            <LocalizedText
              messageKey="settingsRun.status"
              params={{ state, time: timedUntil ? date(timedUntil, { hour: 'numeric', minute: '2-digit' }) : '' }}
            />
          </Badge>
          {scheduleId ? (
            <span className="text-[11px] font-medium text-text-muted">
              {schedule?.enabled ? scheduleSummary(schedule) : <LocalizedText messageKey="ui.settings.components.automationRunStrip.runs.at.any.time.no.weekly.schedule.38fca396" />}
            </span>
          ) : null}
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          {scheduleId && onOpenSchedule ? (
            <Button variant="outline" size="sm" onClick={onOpenSchedule} leftIcon={<CalendarDays className="h-4 w-4" />}>
              <LocalizedText messageKey="common.calendar" />
            </Button>
          ) : null}
          {onOpenDuration ? (
            <Button variant="outline" size="sm" onClick={onOpenDuration} leftIcon={<Timer className="h-4 w-4" />}>
              <LocalizedText messageKey="ui.settings.components.automationRunStrip.run.for.a.time.b8753047" />
            </Button>
          ) : null}
          {running ? (
            <Button variant="outline" size="sm" onClick={() => { void stop(); }} isLoading={stopping} leftIcon={<CircleStop className="h-4 w-4" />}>
              <LocalizedText messageKey="ui.settings.components.automationRunStrip.stop.now.6ebe3863" />
            </Button>
          ) : null}
        </div>
      </div>
      <p className="mt-1.5 text-[11px] leading-relaxed text-text-muted">
        {running
          ? <LocalizedText messageKey="ui.settings.components.automationRunStrip.stop.turns.this.automation.off.right.away.64af434a" />
          : <LocalizedText messageKey="ui.settings.components.automationRunStrip.saving.keeps.these.settings.without.starting.the.343fbef8" />}
      </p>
      {stopError ? <p role="alert" className="mt-1.5 text-[11px] font-semibold text-error">{stopError}</p> : null}
    </div>
  );
};
