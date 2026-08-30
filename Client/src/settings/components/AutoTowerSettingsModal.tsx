import React, { useCallback, useEffect, useState } from 'react';
import { CalendarDays, Crosshair } from 'lucide-react';
import UnitImage from '../../components/UnitImage';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { Button, Card, Input, SettingsModal, Switch } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import {
	clampMapRefreshInterval,
	clampRadius,
  defaultAutoTowerCastleSettings,
  defaultAutoTowerClientState,
  parseAutoTowerClientState,
  persistAutoTowerClientState,
  type AutoTowerCastleSettings,
} from '../AutoTowerClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';
import type { HorseTravelBoostID } from '../HorseTravelBoost';

interface AutoTowerSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

export const AutoTowerSettingsModal: React.FC<AutoTowerSettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule }) => {
  const { state, configuration } = useCitadelAPI();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<Record<string, AutoTowerCastleSettings>>({});
  const [mapRefreshIntervalSec, setMapRefreshIntervalSec] = useState(1800);
  const [dailyAttackLimit, setDailyAttackLimit] = useState(0);
  const [horseTravelBoostId, setHorseTravelBoostId] = useState<HorseTravelBoostID>(-1);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    const current = parseAutoTowerClientState(
      configuration?.sections['automation.autoTowers'] ?? defaultAutoTowerClientState(),
    );
    setSettings(current.castles);
    setMapRefreshIntervalSec(current.mapRefreshIntervalSec);
    setDailyAttackLimit(current.dailyAttackLimit);
    setHorseTravelBoostId(current.horseTravelBoostId);
  }, [configuration?.sections, isOpen]);

  const settingsFor = useCallback((castleID: number): AutoTowerCastleSettings => (
    settings[String(castleID)] ?? defaultAutoTowerCastleSettings()
  ), [settings]);

  const updateCastle = (castleID: number, update: Partial<AutoTowerCastleSettings>) => {
    const key = String(castleID);
    setSettings((current) => ({ ...current, [key]: { ...(current[key] ?? defaultAutoTowerCastleSettings()), ...update } }));
  };

  const chooseTroop = async (castleID: number) => {
    const castle = state?.castles[String(castleID)];
    const selected = settingsFor(castleID).unitId;
    const result = await showTroopPicker({
      mode: 'single',
      title: `Tower troop — ${castle?.name?.trim() || `Castle ${castleID}`}`,
      preselected: selected > 0 ? [selected] : [],
      stockQuantities: castle?.units.stationed,
    });
    if (typeof result === 'number' && result > 0) updateCastle(castleID, { unitId: result });
  };

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    const current = parseAutoTowerClientState(configuration?.sections['automation.autoTowers']);
    try {
      await persistAutoTowerClientState({ ...current, version: 2, mapRefreshIntervalSec, dailyAttackLimit, horseTravelBoostId, castles: settings });
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Towers settings.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="full"
      title="Auto Towers"
      icon={<Crosshair className="h-5 w-5" />}
      description="Each scan saves every tower observed in range per castle, including cooldown state. Attacks select the nearest eligible targets independently, so castle focus is only changed when an attack needs it."
      titleTrailing={(
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => onOpenFeatureSchedule('autoTowers', 'Auto Towers')}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Calendar
            </Button>
      )}
      onSave={save}
      saveLabel="Save changes"
      isSaving={isSaving}
    >
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-4 rounded-global border border-primary/20 bg-primary/5 p-4">
        <div className="min-w-0">
          <div className="text-sm font-bold text-text-main">Authoritative map scan</div>
          <p className="mt-1 text-xs text-text-muted">Fast focus-switch through every enabled castle to rebuild its target list.</p>
        </div>
        <label className="flex items-center gap-2">
          <span className="text-xs font-semibold text-text-muted">Every</span>
          <div className="w-24">
            <Input
              type="number"
              min={1800}
              max={3600}
              value={mapRefreshIntervalSec}
              onChange={(event) => setMapRefreshIntervalSec(clampMapRefreshInterval(event.target.value))}
              className="text-center font-mono"
            />
          </div>
          <span className="text-xs font-semibold text-text-muted">sec</span>
        </label>
      </div>

      <div className="mb-4">
        <DailyAttackLimitField value={dailyAttackLimit} onChange={setDailyAttackLimit} serverState={state?.dailyAttacks} />
      </div>

      <div className="mb-4 rounded-global border border-border-base bg-bg-card/40 p-4">
        <HorseTravelBoostSelect value={horseTravelBoostId} onChange={setHorseTravelBoostId} />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {castles.map((castle) => {
          const plan = settingsFor(castle.id);
          const stock = state?.castles[String(castle.id)]?.units.stationed[String(plan.unitId)] ?? 0;
          return (
            <Card key={castle.id} variant="solid" className="flex flex-col gap-4 bg-bg-card-hover/40 p-4 shadow-inner">
              <div className="flex items-start justify-between gap-3 border-b border-border-base pb-3">
                <div className="min-w-0">
                  <h3 className="truncate text-sm font-bold text-primary">{castle.name}</h3>
                  <p className="mt-0.5 text-xs text-text-muted">{castle.kingdomId}:{castle.x}:{castle.y}</p>
                </div>
                <Switch
                  checked={plan.enabled}
                  onChange={() => updateCastle(castle.id, { enabled: !plan.enabled })}
                  ariaLabel={`Toggle Auto Towers for ${castle.name}`}
                />
              </div>

              <div className="grid gap-3">
                <label className="flex flex-col gap-1">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Radius</span>
                  <Input
                    type="number"
                    min={1}
                    max={50}
                    value={plan.radius}
                    onChange={(event) => updateCastle(castle.id, { radius: clampRadius(event.target.value) })}
                    className="text-center font-mono"
                    rightIcon={<span className="text-[10px] text-text-muted">tiles</span>}
                  />
                </label>
              </div>

              <p className="rounded-xl border border-border-base bg-bg-app/50 px-3 py-2.5 text-[11px] text-text-muted">
                No batch cap: launch every eligible target supported by a currently available selected commander and enough stationed troops. Any return from another attack can wake the next launch immediately.
              </p>

              <button
                type="button"
                onClick={() => chooseTroop(castle.id)}
                className="flex min-h-16 items-center gap-3 rounded-xl border border-dashed border-border-base bg-bg-app/60 p-3 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
              >
                {plan.unitId > 0 ? <UnitImage unitId={plan.unitId} size={48} showLevel /> : <Crosshair className="h-7 w-7 text-text-muted" />}
                <span className="min-w-0">
                  <span className="block text-xs font-bold text-text-main">{plan.unitId > 0 ? `Unit ${plan.unitId}` : 'Choose troop'}</span>
                  <span className="mt-0.5 block text-[11px] text-text-muted">
                    {plan.unitId > 0 ? `${stock.toLocaleString()} stationed` : 'Two full flanks use this unit'}
                  </span>
                </span>
              </button>

              <div className="flex items-center justify-between gap-3 rounded-xl border border-border-base bg-bg-app/50 px-3 py-2.5">
                <div className="min-w-0">
                  <div className="text-xs font-bold text-text-main">Maiden-supported only</div>
                  <p className="mt-0.5 text-[11px] text-text-muted">Only use an available commander with the supported maiden relic.</p>
                </div>
                <Switch
                  checked={plan.maidenOnly}
                  onChange={() => updateCastle(castle.id, { maidenOnly: !plan.maidenOnly })}
                  ariaLabel={`Require maiden-supported commander for ${castle.name}`}
                />
              </div>
            </Card>
          );
        })}
      </div>
    </SettingsModal>
  );
};
