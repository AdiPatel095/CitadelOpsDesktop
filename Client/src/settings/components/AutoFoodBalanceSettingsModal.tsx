import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, FastForward, Truck, Wheat } from 'lucide-react';
import { Button, ChoiceChipGroup, Input, SettingsModal, SettingsToggleRow } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { asRecord } from '../Configuration';
import { useConfigurationDraftSession } from '../ConfigurationDraftSession';
import { useMetadata } from '../../context/MetadataContext';
import { evaluateFoodBalanceReadiness } from '../requirements/setupReadiness';
import { useSetupContext } from '../requirements/useSetupContext';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import { ReadinessPanel } from './ReadinessPanel';
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
  const { t: localizeStatic } = useStaticLocale();
  const { state } = useCitadelAPI();
  const setup = useSetupContext('automation.autoFoodBalance');
  const draftSession = useConfigurationDraftSession({ isOpen, section: 'automation.autoFoodBalance', sessionKey: setup.sessionKey });
  const { resources, isLoading: metadataLoading } = useMetadata();
  const schedules = normalizeFeatureSchedules(asRecord(draftSession.sections?.scheduler).featureSchedules);
  const [settings, setSettings] = useState<AutoFoodBalanceSettings>(DEFAULT_AUTO_FOOD_BALANCE_SETTINGS);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    if (isOpen && draftSession.initialSnapshot) {
      setSettings(parseAutoFoodBalanceSettings(asRecord(
        draftSession.initialSections?.['automation.autoFoodBalance'],
      )));
    }
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const readiness = useMemo(() => evaluateFoodBalanceReadiness({
    state,
    resources,
    metadataReady: !metadataLoading,
    minimumSourceReserve: settings.minimumSourceReserve,
    minimumCoinReserve: settings.minimumCoinReserve,
    autoKingdomTransport: settings.autoKingdomTransport,
  }), [metadataLoading, resources, settings.autoKingdomTransport, settings.minimumCoinReserve, settings.minimumSourceReserve, state]);

  const setNumber = (field: keyof AutoFoodBalanceSettings, value: string) => {
    setSettings((current) => parseAutoFoodBalanceSettings({ ...current, [field]: Number(value) }));
  };

  const save = () => {
    const normalized = parseAutoFoodBalanceSettings(settings);
    setSaveError('');
    void draftSession.save(normalized)
      .then(onClose)
      .catch((error) => setSaveError(error instanceof Error ? error.message : 'Could not save food-balance settings.'));
  };

  const schedule = schedules.autoFoodBalance;
  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="lg"
      title={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.title.auto.food.balance.c200b4fa")}
      icon={<Wheat className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.description.maintains.food.honey.mead.and.beef.reserves.3c47f1fd")}
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
      saveDisabled={!draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={draftSession.conflictNotice}
    >
      <div className="space-y-5">
        <p className="text-sm text-text-muted">
          <LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.before.sending.resources.citadelops.refreshes.each.castle.1110c8f5" /></p>

        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField label={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.label.polling.interval.f0059325")} value={settings.checkIntervalSec} min={30} max={3600} suffix="seconds" onChange={(value) => setNumber('checkIntervalSec', value)} />
          <NumberField label={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.label.minimum.kingdom.shipment.60e2c3d8")} value={settings.minimumShipmentSize} min={1} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumShipmentSize', value)} />
          <NumberField label={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.label.minimum.storm.delivery.8340a87c")} value={settings.minimumStormShipmentSize} min={10_000} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumStormShipmentSize', value)} />
          <NumberField label={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.label.donor.reserve.6e8205ec")} value={settings.minimumSourceReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumSourceReserve', value)} />
          <NumberField label={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.label.coin.reserve.06dee9e3")} value={settings.minimumCoinReserve} min={0} max={Number.MAX_SAFE_INTEGER} onChange={(value) => setNumber('minimumCoinReserve', value)} />
        </div>

        <p className="text-xs text-text-muted">
          <LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.storm.food.and.mead.wait.until.the.1090dfc3" /></p>

        <div className="rounded-global border border-border-base bg-bg-card/40 p-4">
          <HorseTravelBoostSelect
            value={settings.horseTravelBoostId}
            onChange={(horseTravelBoostId) => setSettings((current) => ({ ...current, horseTravelBoostId }))}
            negativeOneLabel="No horse boost · HBW -1"
            description={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.description.applied.to.every.auto.food.market.barrow.76bf87ca")}
          />
        </div>

        <SettingsToggleRow
          title={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.title.allow.kingdom.transport.dfd413bf")}
          description={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.description.allow.the.highest.eligible.donor.to.use.59e691cf")}
          icon={<Truck className="h-4 w-4" />}
          checked={settings.autoKingdomTransport}
          onChange={(checked) => setSettings((current) => ({ ...current, autoKingdomTransport: checked }))}
        />

        <SettingsToggleRow
          title={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.title.use.transport.time.skips.9d4e14fe")}
          description={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.description.apply.selected.skips.one.command.at.a.12767303")}
          icon={<FastForward className="h-4 w-4" />}
          checked={settings.useKingdomTimeSkips}
          disabled={!settings.autoKingdomTransport}
          onChange={(checked) => setSettings((current) => ({ ...current, useKingdomTimeSkips: checked }))}
        />

        {settings.useKingdomTimeSkips && settings.autoKingdomTransport && (
          <div className="space-y-3 rounded-global border border-border-base bg-bg-input/35 p-4">
            <div>
              <div className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.allowed.transport.skips.73eba8a9" /></div>
              <ChoiceChipGroup
                className="mt-2"
                size="sm"
                ariaLabel={localizeStatic("ui.settings.components.autoFoodBalanceSettingsModal.ariaLabel.allowed.auto.food.transport.time.skips.f5271cb4")}
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
              <LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.citadelops.prefers.the.smallest.selected.skip.that.a106f6cf" /></p>
          </div>
        )}

        <div id="auto-food-castles" tabIndex={-1} className="space-y-2 rounded-global border border-border-base bg-bg-card/40 p-4 outline-none">
          <div className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.castles.and.food.stock.a2f41062" /></div>
          {readiness.rows.length === 0 ? (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.no.castles.are.observed.yet.ba035a87" /></p>
          ) : (
            <table className="w-full text-left text-xs">
              <thead className="text-[10px] uppercase tracking-wider text-text-muted">
                <tr>
                  <th scope="col" className="py-1 pr-2 font-bold"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.castle.419fb3b8" /></th>
                  <th scope="col" className="py-1 pr-2 text-right font-bold"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.food.e4eb1806" /></th>
                  <th scope="col" className="py-1 font-bold"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.role.14736a2e" /></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-base">
                {readiness.rows.map((row) => (
                  <tr key={row.castleId}>
                    <td className="max-w-0 truncate py-1 pr-2 text-text-main">{row.name}</td>
                    <td className="py-1 pr-2 text-right font-mono tabular-nums">{row.food == null ? '—' : Math.floor(row.food).toLocaleString()}</td>
                    <td className="py-1 text-text-muted"><LocalizedText messageKey="setupReadiness.foodRole" params={{ role: row.role }} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFoodBalanceSettingsModal.donors.hold.food.above.the.donor.reserve.e605cba8" /></p>
        </div>

        <ReadinessPanel report={readiness.report} onFix={() => focusReadinessTarget('auto-food-castles')} />

        {saveError && <p role="alert" className="text-xs text-error">{saveError}</p>}
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
