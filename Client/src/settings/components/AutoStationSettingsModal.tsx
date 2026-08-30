import React, { useEffect, useState } from 'react';
import { Plus, Shield } from 'lucide-react';
import { showTroopPicker, type UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import {
  AddSlot,
  Button,
  Card,
  Input,
  QuantityAssetTile,
  SettingsModal,
  SettingsToggleRow,
} from '../../components/ui';
import {
  parseAutoStationClientState,
  persistAutoStationClientState,
  type AutoStationClientStateV1,
} from '../AutoStationClientState';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/Selectors';

interface AutoStationSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

function clampMinutes(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.min(60, Math.max(1, Math.round(value)));
}

function clampDays(value: number): number {
  if (!Number.isFinite(value)) return 3;
  return Math.min(30, Math.max(0, Math.round(value)));
}

export const AutoStationSettingsModal: React.FC<AutoStationSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state: gameState, configuration } = useCitadelAPI();
  const castles = castleOptionsFromState(gameState);
  const [state, setState] = useState<AutoStationClientStateV1>(() => parseAutoStationClientState(null));
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    setState(parseAutoStationClientState(
      configuration?.sections['automation.autoStation'],
    ));
  }, [configuration?.sections, isOpen]);

  const selectReserve = async (castle: CastleOptionV2) => {
    const castleID = String(castle.id);
    const current = state.settings[castleID] ?? [];
    const preselectedQuantities: Record<number, number> = {};
    current.forEach((troop) => {
      preselectedQuantities[troop.id] = troop.amount;
    });
    const result = await showTroopPicker({
      mode: 'multi',
      title: `Troops left to defend — ${castle.name}`,
      allowQuantity: true,
      preselected: current.map((troop) => troop.id),
      preselectedQuantities,
    });
    if (!Array.isArray(result)) return;
    const troops = (result as UnitWithQuantity[]).map((troop) => ({
      id: troop.unitId,
      amount: troop.quantity,
    }));
    setState((previous) => ({
      ...previous,
      settings: { ...previous.settings, [castleID]: troops },
    }));
  };

  const removeReserve = (castleID: string, unitID: number) => {
    setState((previous) => ({
      ...previous,
      settings: {
        ...previous.settings,
        [castleID]: (previous.settings[castleID] ?? []).filter((troop) => troop.id !== unitID),
      },
    }));
  };

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await persistAutoStationClientState(state);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Station settings.');
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
      title="Auto Station Settings"
      icon={<Shield className="h-5 w-5" />}
      description="Choose the exact troops that stay behind to defend. Every other troop currently in the threatened castle is temporarily stationed away."
      onSave={save}
      saveLabel="Save changes"
      isSaving={isSaving}
    >
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}
      <div className="flex w-full flex-col gap-6">
        <Card variant="solid" className="bg-bg-app p-4">
          <div className="grid gap-4 md:grid-cols-4">
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-bold uppercase tracking-wider text-primary">Evacuate at</span>
              <Input
                type="number"
                min={1}
                max={60}
                value={Math.round(state.leadTimeSec / 60)}
                onChange={(event) => setState((previous) => ({
                  ...previous,
                  leadTimeSec: clampMinutes(Number(event.target.value)) * 60,
                }))}
                className="font-mono"
                rightIcon={<span className="text-xs font-medium uppercase text-text-muted">Minutes left</span>}
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-bold uppercase tracking-wider text-primary">Minimum Bird Days on Target</span>
              <Input
                type="number"
                min={0}
                max={30}
                value={state.minRPTDays}
                onChange={(event) => setState((previous) => ({
                  ...previous,
                  minRPTDays: clampDays(Number(event.target.value)),
                }))}
                className="font-mono"
                rightIcon={<span className="text-xs font-medium uppercase text-text-muted">Days</span>}
              />
            </label>
            <SettingsToggleRow
              title="Recall when clear"
              checked={state.recallWhenClear}
              onChange={(checked) => setState((previous) => ({ ...previous, recallWhenClear: checked }))}
            />
            <SettingsToggleRow
              title="Open Gate Fallback"
              checked={state.openGateFallback}
              onChange={(checked) => setState((previous) => ({ ...previous, openGateFallback: checked }))}
            />
          </div>
          <p className="mt-4 text-xs leading-relaxed text-text-muted">
            Station targets are the nearest protected alliance castle in the same kingdom. Sends use a one-hour station timer as a fallback even when recall is enabled. If an attack is already inside the configured window when detected, evacuation starts immediately.
          </p>
        </Card>

        <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto pr-1">
          {castles.length === 0 && (
            <p className="py-8 text-center text-sm text-text-muted">Loading castles…</p>
          )}
          <div className="grid grid-cols-1 gap-4 pb-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {castles.map((castle) => {
              const castleID = String(castle.id);
              const reserves = state.settings[castleID] ?? [];
              return (
                <Card key={castle.id} variant="solid" className="flex flex-col bg-bg-card-hover/40 p-4 shadow-inner">
                  <div className="mb-3 border-b border-border-base pb-2">
                    <h3 className="text-sm font-bold text-primary">{castle.name || `${castle.type} castle`}</h3>
                    <p className="mt-1 text-[11px] text-text-muted">These amounts remain in the castle.</p>
                  </div>
                  {reserves.length === 0 ? (
                    <div className="flex flex-1 flex-col items-center justify-center gap-3 py-6">
                      <p className="text-center text-xs font-medium uppercase tracking-wider text-text-muted">No defense reserve</p>
                      <Button variant="outline" size="sm" onClick={() => selectReserve(castle)} leftIcon={<Plus className="h-4 w-4" />}>
                        Add troops
                      </Button>
                    </div>
                  ) : (
                    <div className="flex flex-wrap justify-center gap-4">
                      {reserves.map((troop) => (
                        <QuantityAssetTile
                          key={troop.id}
                          visual={<UnitImage unitId={troop.id} size={76} showLevel className="rounded-xl" />}
                          quantity={troop.amount}
                          onRemove={() => removeReserve(castleID, troop.id)}
                          removeLabel="Remove defense troop"
                        />
                      ))}
                      <AddSlot
                        label="Edit defense troops"
                        layout="icon"
                        onClick={() => selectReserve(castle)}
                        className="h-[76px] w-[76px]"
                        icon={<Plus className="h-8 w-8" strokeWidth={1.5} />}
                      />
                    </div>
                  )}
                </Card>
              );
            })}
          </div>
        </div>
      </div>
    </SettingsModal>
  );
};
