import React, { useEffect, useState } from 'react';
import { CalendarDays, Clock3, Settings } from 'lucide-react';
import { Badge, Button, Input, SectionCard, SettingsModal } from '../../components/ui';
import {
  autoHospitalCheckIntervalMinutesToSec,
  autoHospitalCheckIntervalSecToMinutes,
  DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
  defaultAutoHospitalSettings,
  MIN_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
  normalizeAutoHospitalSettings,
  persistAutoHospitalSettings,
  type AutoHospitalClientSettingsV1,
} from '../AutoHospitalClientState';
import {
  formatMinuteOfDay,
  normalizeFeatureSchedules,
  scheduleSummary,
  WEEK_DAYS,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';

interface AutoHospitalSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

export const AutoHospitalSettingsModal: React.FC<AutoHospitalSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
}) => {
  const { configuration } = useCitadelAPI();
  const [settings, setSettings] = useState<AutoHospitalClientSettingsV1>(() => defaultAutoHospitalSettings());
  const featureSchedules = normalizeFeatureSchedules(
    configurationSection(configuration, 'scheduler').featureSchedules,
  );
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    setSettings(normalizeAutoHospitalSettings(
      configuration?.sections['automation.autoHospital'] ?? defaultAutoHospitalSettings(),
    ));
  }, [configuration?.sections, isOpen]);

  const updateCheckIntervalMinutes = (value: string) => {
    const raw = value.replace(/,/g, '');
    if (!/^\d*$/.test(raw)) return;
    setSettings(prev => ({
      ...prev,
      checkIntervalSec: autoHospitalCheckIntervalMinutesToSec(raw === '' ? DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_MIN : Number(raw)),
    }));
  };

  const handleSave = () => {
    setIsSaving(true);
    const sent = persistAutoHospitalSettings(settings);
    setIsSaving(false);
    if (sent) onClose();
  };

  const handleClose = () => {
    onClose();
  };

  const autoHospitalSchedule = featureSchedules.autoHospital;
  const autoHospitalScheduleEnabled = !!autoHospitalSchedule?.enabled;

  const renderScheduleSlots = (schedule: WeeklySchedule) => {
    const visibleSlots = schedule.slots.slice(0, 5);
    const hiddenCount = Math.max(0, schedule.slots.length - visibleSlots.length);

    return (
      <div className="grid gap-2">
        {visibleSlots.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-base bg-bg-card/45 px-4 py-3 text-xs font-semibold text-text-muted">
            No scheduled windows
          </div>
        ) : (
          visibleSlots.map((slot) => {
            const day = WEEK_DAYS[slot.day]?.short ?? 'Day';
            return (
              <div
                key={slot.id}
                className="flex min-w-0 items-center gap-3 rounded-global border border-border-base bg-bg-card/65 px-3 py-2"
                title={`${day} ${formatMinuteOfDay(slot.startMinute)}-${formatMinuteOfDay(slot.endMinute)}`}
              >
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-xs font-black text-primary">
                  {day}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-xs font-bold text-text-main">
                    {formatMinuteOfDay(slot.startMinute)}-{formatMinuteOfDay(slot.endMinute)}
                  </div>
                  <div className="mt-0.5 text-[11px] font-semibold text-text-muted">
                    Hospital scan window
                  </div>
                </div>
              </div>
            );
          })
        )}
        {hiddenCount > 0 && (
          <div className="rounded-global border border-border-base bg-bg-card/45 px-3 py-2 text-center text-[11px] font-bold uppercase tracking-wide text-text-muted">
            +{hiddenCount} more slot{hiddenCount === 1 ? '' : 's'}
          </div>
        )}
      </div>
    );
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="4xl"
      title="Auto Hospital Settings"
      icon={<Settings className="h-5 w-5" />}
      description="Queue scans and calendar windows"
      onSave={handleSave}
      isSaving={isSaving}
    >
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-5 overflow-visible pb-2">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-[minmax(14rem,0.8fr)_minmax(18rem,1.2fr)]">
          <SectionCard
            title="Queue Check"
            description="Minutes between hospital scans."
            icon={<Clock3 className="h-4 w-4" />}
            titleClassName="text-base"
          >
              <Input
                type="text"
                value={autoHospitalCheckIntervalSecToMinutes(settings.checkIntervalSec).toLocaleString()}
                onChange={(e) => updateCheckIntervalMinutes(e.target.value)}
                className="font-mono text-lg font-black tabular-nums"
                rightIcon={<span className="text-xs font-bold uppercase text-text-muted">min</span>}
              />
              <p className="mt-2 text-[11px] font-medium text-text-muted">
                Minimum {MIN_AUTO_HOSPITAL_CHECK_INTERVAL_MIN.toLocaleString()} minute. Default is {DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_MIN.toLocaleString()} minutes.
              </p>
          </SectionCard>

          <SectionCard
            title="Shared Schedule"
            description={autoHospitalSchedule ? scheduleSummary(autoHospitalSchedule) : 'Schedule off'}
            icon={<CalendarDays className="h-4 w-4" />}
            titleClassName="text-base"
            actions={(
              <div className="flex shrink-0 items-center gap-2">
                <Badge variant={autoHospitalScheduleEnabled ? 'primary' : 'secondary'}>
                  {autoHospitalScheduleEnabled ? 'On' : 'Off'}
                </Badge>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onOpenFeatureSchedule('autoHospital', 'Auto Hospital')}
                  leftIcon={<CalendarDays className="h-4 w-4" />}
                >
                  Calendar
                </Button>
              </div>
            )}
          >
              {autoHospitalScheduleEnabled && autoHospitalSchedule ? (
                renderScheduleSlots(autoHospitalSchedule)
              ) : (
                <div className="rounded-global border border-dashed border-border-base bg-bg-card/45 px-4 py-5 text-sm font-semibold text-text-muted">
                  Auto Hospital can scan at any time.
                </div>
              )}
          </SectionCard>
        </div>
      </div>
    </SettingsModal>
  );
};
