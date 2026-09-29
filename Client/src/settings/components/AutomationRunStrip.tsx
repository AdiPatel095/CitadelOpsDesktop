import React from 'react';
import { CalendarDays, Timer } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { StopControl } from '../../components/StopControl';
import { useAuth } from '../../context/AuthContext';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import { configurationSection } from '../Configuration';
import { normalizeFeatureSchedules, scheduleSummary } from '../SchedulerTypes';
import { AUTOMATION_ENABLED_KEYS, type SettingsFeatureId } from '../disclosure/placement';
import { useAutomationDescription } from '../readiness/useAutomationDescription';

export interface AutomationRunStripProps {
  featureId: SettingsFeatureId;
  /** Scheduler id when the feature supports a weekly schedule. */
  scheduleId?: string;
  onOpenSchedule?: () => void;
  onOpenDuration?: () => void;
  /** `explicit`: a Save button writes the settings. `immediate`: edits are written as they are made (Equipment Cleanup). */
  saveMode?: 'explicit' | 'immediate';
}

/**
 * Essentials run line of every settings modal (CIT-17, CIT-20): the phase the game reports (not just "on"),
 * the schedule, and a Stop that is reachable without expanding anything. Stop writes `automation.enabled`
 * only when clicked and never touches the settings draft; saving never starts or stops the automation. What
 * Stop does and does not do is spelled out beside it (`StopControl`).
 */
export const AutomationRunStrip: React.FC<AutomationRunStripProps> = ({ featureId, scheduleId, onOpenSchedule, onOpenDuration, saveMode = 'explicit' }) => {
  const { automationEnabledByKey, automationTimedUntilByKey } = useAuth();
  const { configuration } = useCitadelAPI();
  const { date } = useLocale();
  const enabledKey = AUTOMATION_ENABLED_KEYS[featureId];
  const running = automationEnabledByKey[enabledKey] === true;
  const timedUntil = automationTimedUntilByKey[enabledKey];
  const description = useAutomationDescription(featureId);
  const schedule = scheduleId
    ? normalizeFeatureSchedules(configurationSection(configuration, 'scheduler').featureSchedules)[scheduleId]
    : undefined;

  return (
    <div className="mb-4 rounded-global border border-border-base bg-bg-card/40 px-4 py-3" data-settings-run-strip={featureId}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge variant={running ? 'primary' : 'outline'} className="normal-case tracking-normal">
            <LocalizedText messageKey="runtimeState.phase" params={{ phase: description.phase.replaceAll('-', '_') }} />
          </Badge>
          {timedUntil ? (
            <span className="text-[11px] font-medium text-text-muted">
              <LocalizedText messageKey="settingsRun.until" params={{ time: date(timedUntil, { hour: 'numeric', minute: '2-digit' }) }} />
            </span>
          ) : null}
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
          {running ? <StopControl enabledKey={enabledKey} featureId={featureId} /> : null}
        </div>
      </div>
      <p className="mt-1.5 text-[11px] leading-relaxed text-text-muted">
        <LocalizedText messageKey={running ? 'settingsRun.savingNote' : saveMode === 'immediate' ? 'settingsRun.immediateNote' : 'settingsRun.stoppedNote'} />
      </p>
      {!running ? <StopControl enabledKey={enabledKey} featureId={featureId} variant="notice" className="mt-1.5" /> : null}
    </div>
  );
};
