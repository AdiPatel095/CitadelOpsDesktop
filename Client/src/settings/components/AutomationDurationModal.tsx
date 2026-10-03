import { formatDurationEnd } from "../../i18n/automationDuration";
import { useLocale } from "../../i18n/LocaleContext";
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
  { key: 'automationDurationDialog.presetMinutes', count: 30, minutes: 30 },
  { key: 'automationDurationDialog.presetHours', count: 1, minutes: 60 },
  { key: 'automationDurationDialog.presetHours', count: 2, minutes: 120 },
  { key: 'automationDurationDialog.presetHours', count: 4, minutes: 240 },
  { key: 'automationDurationDialog.presetHours', count: 8, minutes: 480 },
  { key: 'automationDurationDialog.presetHours', count: 24, minutes: 1440 },
] as const;

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
  const { t, locale } = useLocale();
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
      setError(value instanceof Error ? value.message : t('automationDurationDialog.saveFailed'));
      setSaving(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="md"
      title={<ModalTitle icon={<TimerReset className="h-5 w-5" />}>{t(onPauseFor ? 'automationDurationDialog.pauseTitle' : 'automationDurationDialog.runTitle', { feature: featureLabel })}</ModalTitle>}
      footer={(
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={saving}><LocalizedText messageKey="game.cancel" /></Button>
          <Button onClick={() => void save()} disabled={!valid} isLoading={saving}>{t(onPauseFor ? 'automationDurationDialog.pauseButton' : 'automationDurationDialog.runButton')}</Button>
        </div>
      )}
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-global border border-primary/20 bg-primary/5 p-4">
          <div className="text-sm font-bold text-text-main">{t('automationDurationDialog.quickDurations')}</div>
          <div className="mt-3 grid grid-cols-3 gap-2">
            {durationPresets.map((preset) => (
              <Button
                key={preset.minutes}
                variant={durationMinutes === preset.minutes ? 'primary' : 'outline'}
                size="sm"
                onClick={() => selectPreset(preset.minutes)}
              >
                {t(preset.key, { count: preset.count })}
              </Button>
            ))}
          </div>
        </div>

        <div className="rounded-global border border-border-base bg-bg-card/45 p-4">
          <div className="text-sm font-bold text-text-main">{t('automationDurationDialog.customDuration')}</div>
          <div className="mt-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3">
            <Input
              type="number"
              min={1}
              step={1}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              className="font-mono"
              aria-label={t('automationDurationDialog.amountLabel')}
            />
            <Select
              value={unit}
              onChange={(value) => setUnit(value as DurationUnit)}
              options={[
                { value: 'minutes', label: t('automationDurationDialog.minutes') },
                { value: 'hours', label: t('automationDurationDialog.hours') },
                { value: 'days', label: t('automationDurationDialog.days') },
              ]}
              ariaLabel={t('automationDurationDialog.unitLabel')}
            />
          </div>
          {!valid ? <p className="mt-2 text-xs text-error">{t('automationDurationDialog.invalidDuration')}</p> : null}
        </div>

        <div className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs leading-relaxed text-text-muted">
          {turnsOffAt ? (
            <p>
              {t(onPauseFor ? 'automationDurationDialog.pauseStarts' : 'automationDurationDialog.runStarts', { feature: featureLabel, endsAt: formatDurationEnd(turnsOffAt, locale) })}
            </p>
          ) : null}
          <p className="mt-1">{t('automationDurationDialog.scheduleNotice')}</p>
          {currentUntil ? (
            <p className="mt-2 text-primary">{t(onPauseFor ? 'automationDurationDialog.currentPauseEnds' : 'automationDurationDialog.currentRunEnds', { date: formatDurationEnd(currentUntil, locale) })}</p>
          ) : null}
        </div>

        {error ? <p className="text-xs text-error">{error}</p> : null}
      </div>
    </Modal>
  );
};
