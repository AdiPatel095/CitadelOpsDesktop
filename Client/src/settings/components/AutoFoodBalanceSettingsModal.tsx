import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, FastForward, Truck, Wheat } from 'lucide-react';
import { Button, ChoiceChipGroup, Input, SettingsModal, SettingsToggleRow } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import { normalizeFeatureSchedules, scheduleSummary } from '../SchedulerTypes';
import {
  AUTO_FOOD_BALANCE_TIME_SKIPS,
  DEFAULT_AUTO_FOOD_BALANCE_SETTINGS,
  parseAutoFoodBalanceSettings,
  type AutoFoodBalanceSettings,
} from '../AutoFoodBalanceClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';

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
      maxWidth="lg"
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
          Before sending resources, CitadelOps refreshes each castle’s food state so troop consumption, brewery inputs, and equipped bonuses are current. It ranks donors by net surplus, requires enough protected storage to fill the destination, then uses market barrows or allowed kingdom transport.
        </p>

        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField label="Polling interval" value={settings.checkIntervalSec} min={30} max={3600} suffix="seconds" onChange={(value) => setNumber('checkIntervalSec', value)} />
          <NumberField label="Minimum kingdom shipment" value={settings.minimumShipmentSize} min={1} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumShipmentSize', value)} />
          <NumberField label="Donor reserve" value={settings.minimumSourceReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumSourceReserve', value)} />
          <NumberField label="Coin reserve" value={settings.minimumCoinReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumCoinReserve', value)} />
        </div>

        <div className="rounded-global border border-border-base bg-bg-card/40 p-4">
          <HorseTravelBoostSelect
            value={settings.horseTravelBoostId}
            onChange={(horseTravelBoostId) => setSettings((current) => ({ ...current, horseTravelBoostId }))}
            negativeOneLabel="No horse boost · HBW -1"
            description="Applied to every Auto Food market-barrow shipment. Coin and ruby horses are used only when explicitly selected."
          />
        </div>

        <SettingsToggleRow
          title="Allow kingdom transport"
          description="Allow the highest eligible donor to use kingdom transport when it is in another kingdom."
          icon={<Truck className="h-4 w-4" />}
          checked={settings.autoKingdomTransport}
          onChange={(checked) => setSettings((current) => ({ ...current, autoKingdomTransport: checked }))}
        />

        <SettingsToggleRow
          title="Use transport time skips"
          description="Apply a covering selected skip during launch; only shipments without one remain marked in transit."
          icon={<FastForward className="h-4 w-4" />}
          checked={settings.useKingdomTimeSkips}
          disabled={!settings.autoKingdomTransport}
          onChange={(checked) => setSettings((current) => ({ ...current, useKingdomTimeSkips: checked }))}
        />

        {settings.useKingdomTimeSkips && settings.autoKingdomTransport && (
          <div className="space-y-3 rounded-global border border-border-base bg-bg-input/35 p-4">
            <div>
              <div className="text-xs font-bold uppercase tracking-wider text-text-muted">Allowed transport skips</div>
              <ChoiceChipGroup
                className="mt-2"
                size="sm"
                ariaLabel="Allowed Auto Food transport time skips"
                options={AUTO_FOOD_BALANCE_TIME_SKIPS.map((skip) => ({ value: skip.id, label: skip.label }))}
                selected={settings.allowedTimeSkips}
                onToggle={(skipID) => setSettings((current) => parseAutoFoodBalanceSettings({
                  ...current,
                  allowedTimeSkips: current.allowedTimeSkips.includes(skipID)
                    ? current.allowedTimeSkips.filter((id) => id !== skipID)
                    : [...current.allowedTimeSkips, skipID],
                }))}
              />
            </div>

            {settings.allowedTimeSkips.length > 0 && (
              <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-7">
                {AUTO_FOOD_BALANCE_TIME_SKIPS
                  .filter((skip) => settings.allowedTimeSkips.includes(skip.id))
                  .map((skip) => (
                    <label key={skip.id} className="grid gap-1 text-[10px] font-bold text-text-muted">
                      Keep {skip.label}
                      <Input
                        type="number"
                        min={0}
                        value={settings.timeSkipReserve[skip.id] ?? 0}
                        onChange={(event) => setSettings((current) => parseAutoFoodBalanceSettings({
                          ...current,
                          timeSkipReserve: {
                            ...current.timeSkipReserve,
                            [skip.id]: Number(event.target.value),
                          },
                        }))}
                        className="px-2 text-center font-mono"
                      />
                    </label>
                  ))}
              </div>
            )}

            <p className="text-[11px] leading-relaxed text-text-muted">
              CitadelOps prefers the smallest selected skip that completes a shipment, then the largest selected partial skip. The amounts above are always kept in reserve.
            </p>
          </div>
        )}

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
