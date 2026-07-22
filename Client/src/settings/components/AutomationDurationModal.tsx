import React, { useEffect, useMemo, useState } from 'react';
import { TimerReset } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Button, Input, Modal, ModalTitle, Select } from '../../components/ui';

interface AutomationDurationModalProps {
  isOpen: boolean;
  featureKey: string;
  featureLabel: string;
  onClose: () => void;
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
}) => {
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
  const currentUntil = automationTimedUntilByKey[featureKey];

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
      await enableAutomationFor(featureKey, durationMinutes);
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
      title={<ModalTitle icon={<TimerReset className="h-5 w-5" />}>Run {featureLabel} for a duration</ModalTitle>}
      footer={(
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button onClick={() => void save()} disabled={!valid} isLoading={saving}>Turn on for this duration</Button>
        </div>
      )}
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-global border border-primary/20 bg-primary/5 p-4">
          <div className="text-sm font-bold text-text-main">Quick durations</div>
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
          <div className="text-sm font-bold text-text-main">Custom duration</div>
          <div className="mt-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3">
            <Input
              type="number"
              min={1}
              step={1}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              className="font-mono"
              aria-label="Automation duration amount"
            />
            <Select
              value={unit}
              onChange={(value) => setUnit(value as DurationUnit)}
              options={[
                { value: 'minutes', label: 'Minutes' },
                { value: 'hours', label: 'Hours' },
                { value: 'days', label: 'Days' },
              ]}
              ariaLabel="Automation duration unit"
            />
          </div>
          {!valid ? <p className="mt-2 text-xs text-error">Choose a duration from 1 minute through 7 days.</p> : null}
        </div>

        <div className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs leading-relaxed text-text-muted">
          {turnsOffAt ? (
            <p>
              {featureLabel} turns on immediately and the server turns it off at{' '}
              <span className="font-semibold text-text-main">{turnsOffAt.toLocaleString()}</span>.
            </p>
          ) : null}
          <p className="mt-1">Weekly schedules and the global automation lock still apply during this window.</p>
          {currentUntil ? (
            <p className="mt-2 text-primary">Current timed run ends {new Date(currentUntil).toLocaleString()}.</p>
          ) : null}
        </div>

        {error ? <p className="text-xs text-error">{error}</p> : null}
      </div>
    </Modal>
  );
};
