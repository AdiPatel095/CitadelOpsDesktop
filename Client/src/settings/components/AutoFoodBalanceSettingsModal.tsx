import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, Truck, Wheat } from 'lucide-react';
import { Button, Input, SettingsModal, SettingsToggleRow } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import { normalizeFeatureSchedules, scheduleSummary } from '../SchedulerTypes';
import {
  DEFAULT_AUTO_FOOD_BALANCE_SETTINGS,
  parseAutoFoodBalanceSettings,
  type AutoFoodBalanceSettings,
} from '../AutoFoodBalanceClientState';

interface AutoFoodBalanceSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

export const AutoFoodBalanceSettingsModal: React.FC<AutoFoodBalanceSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
}) => {
  const { configuration, updateConfiguration } = useCitadelAPI();
  const saved = useMemo(
    () => parseAutoFoodBalanceSettings(configurationSection(configuration, 'automation.autoFoodBalance')),
    [configuration?.sections['automation.autoFoodBalance']],
  );
  const schedules = normalizeFeatureSchedules(configurationSection(configuration, 'scheduler').featureSchedules);
  const [settings, setSettings] = useState<AutoFoodBalanceSettings>(DEFAULT_AUTO_FOOD_BALANCE_SETTINGS);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    if (isOpen) setSettings(saved);
  }, [isOpen, saved]);

  const setNumber = (field: keyof AutoFoodBalanceSettings, value: string) => {
    setSettings((current) => parseAutoFoodBalanceSettings({ ...current, [field]: Number(value) }));
  };

  const save = () => {
    const normalized = parseAutoFoodBalanceSettings(settings);
    setSaveError('');
    void updateConfiguration('automation.autoFoodBalance', normalized)
      .then(onClose)
      .catch((error) => setSaveError(error instanceof Error ? error.message : 'Could not save food-balance settings.'));
  };

  const schedule = schedules.autoFoodBalance;
  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="md"
      title="Auto Food Balance"
      icon={<Wheat className="h-5 w-5" />}
      description="Maintains Food, Honey, Mead, and Beef reserves across every owned castle."
      titleTrailing={(
            <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            onClick={() => onOpenFeatureSchedule('autoFoodBalance', 'Auto Food Balance')}
            leftIcon={<CalendarDays className="h-4 w-4" />}
          >
            {schedule?.enabled ? scheduleSummary(schedule) : 'Schedule'}
          </Button>
      )}
      onSave={save}
      saveLabel="Save"
    >
      <div className="space-y-5">
        <p className="text-sm text-text-muted">
          Before sending resources, CitadelOps refreshes each castle’s food state so troop consumption, brewery inputs, and equipped bonuses are current. It prefers market barrows inside a kingdom and uses kingdom transport only when needed.
        </p>

        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField label="Polling interval" value={settings.checkIntervalSec} min={30} max={3600} suffix="seconds" onChange={(value) => setNumber('checkIntervalSec', value)} />
          <NumberField label="Donor reserve" value={settings.minimumSourceReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumSourceReserve', value)} />
          <NumberField label="Coin reserve" value={settings.minimumCoinReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumCoinReserve', value)} />
        </div>

        <SettingsToggleRow
          title="Allow kingdom transport"
          description="Use kingdom transport when no same-kingdom market donor can safely cover the reserve."
          icon={<Truck className="h-4 w-4" />}
          checked={settings.autoKingdomTransport}
          onChange={(checked) => setSettings((current) => ({ ...current, autoKingdomTransport: checked }))}
        />

        {saveError && <p className="text-xs text-error">{saveError}</p>}
      </div>
    </SettingsModal>
  );
};

function NumberField({
  label,
  value,
  min,
  max,
  suffix,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  suffix?: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-bold uppercase tracking-wider text-text-muted">{label}</label>
      <Input type="number" min={min} max={max} value={value} onChange={(event) => onChange(event.target.value)} rightIcon={suffix ? <span className="text-xs">{suffix}</span> : undefined} />
    </div>
  );
}

export default AutoFoodBalanceSettingsModal;
