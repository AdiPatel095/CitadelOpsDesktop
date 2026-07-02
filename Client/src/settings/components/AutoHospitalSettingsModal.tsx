import React, { useEffect, useState } from 'react';
import { CalendarDays, Clock3, Save, Settings } from 'lucide-react';
import { FrontendWebsocket } from '../../Websocket';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Input, Modal } from '../../components/ui';
import {
  autoHospitalCheckIntervalMinutesToSec,
  autoHospitalCheckIntervalSecToMinutes,
  DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
  loadAutoHospitalSettingsFromStorage,
  MIN_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
  normalizeAutoHospitalSettings,
  notifyAutoHospitalSettingsChanged,
  persistAutoHospitalSettings,
  type AutoHospitalClientSettingsV1,
} from '../AutoHospitalClientState';
import {
  formatMinuteOfDay,
  normalizeFeatureSchedules,
  scheduleSummary,
  WEEK_DAYS,
  type FeatureSchedules,
  type WeeklySchedule,
} from '../SchedulerTypes';

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
  const [settings, setSettings] = useState<AutoHospitalClientSettingsV1>(() => loadAutoHospitalSettingsFromStorage());
  const [featureSchedules, setFeatureSchedules] = useState<FeatureSchedules>({});
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    setSettings(loadAutoHospitalSettingsFromStorage());
    FrontendWebsocket.sendMessage({ type: 'getAutoHospitalSettings' });
    FrontendWebsocket.sendGetSchedulerSettings();
  }, [isOpen]);

  useEffect(() => {
    const handleMessage = (msg: any) => {
      if (msg.type === 'autoHospitalSettings') {
        setSettings(normalizeAutoHospitalSettings(msg.payload));
        setIsSaving(false);
      }
      if (msg.type === 'schedulerSettings' && msg.payload) {
        setFeatureSchedules(normalizeFeatureSchedules(msg.payload.featureSchedules));
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

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
    notifyAutoHospitalSettingsChanged(loadAutoHospitalSettingsFromStorage());
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
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="4xl"
      title={
        <div className="scheduler-modal-title">
          <span className="scheduler-modal-title-mark" aria-hidden="true">
            <Settings className="h-5 w-5" />
          </span>
          <span className="flex min-w-0 flex-col">
            <span className="scheduler-modal-title-text">Auto Hospital Settings</span>
            <span className="mt-1 text-xs font-semibold text-text-muted">
              Queue scans and calendar windows
            </span>
          </span>
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} className="px-6">Cancel</Button>
          <Button variant="primary" onClick={handleSave} disabled={isSaving} className="px-8" leftIcon={<Save className="w-4 h-4" />}>
            {isSaving ? 'Saving...' : 'Save Settings'}
          </Button>
        </>
      }
    >
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-5 overflow-visible pb-2">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-[minmax(14rem,0.8fr)_minmax(18rem,1.2fr)]">
          <Card variant="solid" className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent">
              <div>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Clock3 className="h-4 w-4 text-primary" />
                  Queue Check
                </CardTitle>
                <p className="mt-1 text-xs text-text-muted">Minutes between hospital scans.</p>
              </div>
            </CardHeader>
            <CardContent className="liquid-prominent-header-content p-5">
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
            </CardContent>
          </Card>

          <Card variant="solid" className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-3">
              <div className="min-w-0">
                <CardTitle className="flex items-center gap-2 text-base">
                  <CalendarDays className="h-4 w-4 text-primary" />
                  Shared Schedule
                </CardTitle>
                <p className="mt-1 text-xs text-text-muted">
                  {autoHospitalSchedule ? scheduleSummary(autoHospitalSchedule) : 'Schedule off'}
                </p>
              </div>
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
            </CardHeader>
            <CardContent className="liquid-prominent-header-content p-5">
              {autoHospitalScheduleEnabled && autoHospitalSchedule ? (
                renderScheduleSlots(autoHospitalSchedule)
              ) : (
                <div className="rounded-global border border-dashed border-border-base bg-bg-card/45 px-4 py-5 text-sm font-semibold text-text-muted">
                  Auto Hospital can scan at any time.
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </Modal>
  );
};
