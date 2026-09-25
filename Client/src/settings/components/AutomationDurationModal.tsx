import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { TimerReset } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Button, Input, Modal, ModalTitle, Select } from '../../components/ui';

interface AutomationDurationModalProps {
  isOpen: boolean;
  featureKey: string;
  featureLabel: string;
  onClose: () => void;
  onPauseFor?: (minutes: number) => Promise<void>;
  pausedUntil?: number;
}

type DurationUnit = 'minutes' | 'hours' | 'days';

const durationPresets = [
  { label: '30 min', minutes: 30 },
  { label: '1 hr', minutes: 60 },
  { label: '2 hr', minutes: 120 },
  { label: '4 hr', minutes: 240 },
  { label: '8 hr', minutes: 480 },
  { label: '24 hr', minutes: 1440 },
];

const durationMultipliers: Record<DurationUnit, number> = {
  minutes: 1,
  hours: 60,
  days: 1440,
};

export const AutomationDurationModal: React.FC<AutomationDurationModalProps> = ({
  isOpen,
  featureKey,
  featureLabel,
  onClose,
  onPauseFor,
  pausedUntil,
}) => {
  const { t: localizeStatic } = useStaticLocale();
  const { enableAutomationFor, automationTimedUntilByKey } = useAuth();
  const [amount, setAmount] = useState('1');
  const [unit, setUnit] = useState<DurationUnit>('hours');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    setAmount('1');
    setUnit('hours');
    setSaving(false);
    setError('');
  }, [featureKey, isOpen]);

  const durationMinutes = useMemo(() => {
    const numeric = Number(amount);
    if (!Number.isFinite(numeric) || numeric <= 0) return 0;
    return Math.round(numeric * durationMultipliers[unit]);
  }, [amount, unit]);
  const valid = durationMinutes >= 1 && durationMinutes <= 10_080;
  const turnsOffAt = valid ? new Date(Date.now() + durationMinutes * 60_000) : null;
  const currentUntil = onPauseFor ? pausedUntil : automationTimedUntilByKey[featureKey];

  const selectPreset = (minutes: number) => {
    if (minutes < 60) {
      setAmount(String(minutes));
      setUnit('minutes');
    } else if (minutes % 1440 === 0) {
      setAmount(String(minutes / 1440));
      setUnit('days');
    } else {
      setAmount(String(minutes / 60));
      setUnit('hours');
    }
    setError('');
  };

  const save = async () => {
    if (!valid || saving) return;
    setSaving(true);
    setError('');
    try {
      if (onPauseFor) await onPauseFor(durationMinutes);
      else await enableAutomationFor(featureKey, durationMinutes);
      onClose();
    } catch (value) {
      setError(value instanceof Error ? value.message : 'Could not save the timed automation duration');
      setSaving(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="md"
      title={<ModalTitle icon={<TimerReset className="h-5 w-5" />}>{onPauseFor ? 'Pause' : 'Run'} {featureLabel} for a duration</ModalTitle>}
      footer={(
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={saving}><LocalizedText messageKey="game.cancel" /></Button>
          <Button onClick={() => void save()} disabled={!valid} isLoading={saving}>{onPauseFor ? 'Pause for this duration' : 'Turn on for this duration'}</Button>
        </div>
      )}
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-global border border-primary/20 bg-primary/5 p-4">
          <div className="text-sm font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.automationDurationModal.quick.durations.e1a95cd2" /></div>
          <div className="mt-3 grid grid-cols-3 gap-2">
            {durationPresets.map((preset) => (
              <Button
                key={preset.minutes}
                variant={durationMinutes === preset.minutes ? 'primary' : 'outline'}
                size="sm"
                onClick={() => selectPreset(preset.minutes)}
              >
                {preset.label}
              </Button>
            ))}
          </div>
        </div>

        <div className="rounded-global border border-border-base bg-bg-card/45 p-4">
          <div className="text-sm font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.automationDurationModal.custom.duration.37421efc" /></div>
          <div className="mt-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3">
            <Input
              type="number"
              min={1}
              step={1}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              className="font-mono"
              aria-label={localizeStatic("ui.settings.components.automationDurationModal.aria-label.automation.duration.amount.11388035")}
            />
            <Select
              value={unit}
              onChange={(value) => setUnit(value as DurationUnit)}
              options={[
                { value: 'minutes', label: 'Minutes' },
                { value: 'hours', label: 'Hours' },
                { value: 'days', label: 'Days' },
              ]}
              ariaLabel={localizeStatic("ui.settings.components.automationDurationModal.ariaLabel.automation.duration.unit.c82ad9ba")}
            />
          </div>
          {!valid ? <p className="mt-2 text-xs text-error"><LocalizedText messageKey="ui.settings.components.automationDurationModal.choose.a.duration.from.1.minute.through.270a3657" /></p> : null}
        </div>

        <div className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs leading-relaxed text-text-muted">
          {turnsOffAt ? (
            <p>
              {featureLabel} {onPauseFor ? 'pauses immediately and resumes at' : 'turns on immediately and the server turns it off at'}{' '}
              <span className="font-semibold text-text-main">{turnsOffAt.toLocaleString()}</span>.
            </p>
          ) : null}
          <p className="mt-1"><LocalizedText messageKey="ui.settings.components.automationDurationModal.weekly.schedules.and.the.global.automation.lock.a4fd4bf2" /></p>
          {currentUntil ? (
            <p className="mt-2 text-primary">{onPauseFor ? 'Current pause ends' : 'Current timed run ends'} {new Date(currentUntil).toLocaleString()}.</p>
          ) : null}
        </div>

        {error ? <p className="text-xs text-error">{error}</p> : null}
      </div>
    </Modal>
  );
};
