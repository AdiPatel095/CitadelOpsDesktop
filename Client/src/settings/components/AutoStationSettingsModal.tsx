import React, { useEffect, useState } from 'react';
import { Plus, Save, Shield, X } from 'lucide-react';
import { showTroopPicker, type UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { Button, Card, Input, Modal, Switch } from '../../components/ui';
import {
  loadAutoStationClientState,
  parseAutoStationClientState,
  persistAutoStationClientState,
  type AutoStationClientStateV1,
} from '../AutoStationClientState';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/StateAdapters';

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

function ReserveTile({
  unitID,
  amount,
  onRemove,
}: {
  unitID: number;
  amount: number;
  onRemove: () => void;
}) {
  return (
    <div className="group relative flex w-[84px] flex-col items-center">
      <button
        type="button"
        onClick={onRemove}
        className="absolute -right-1 -top-1 z-20 flex h-5 w-5 items-center justify-center rounded-full bg-error text-white opacity-0 shadow-md transition-opacity group-hover:opacity-100"
        aria-label="Remove defense troop"
      >
        <X className="h-3 w-3" />
      </button>
      <div className="relative h-[76px] w-[76px]">
        <UnitImage unitId={unitID} size={76} showLevel={true} className="rounded-xl" />
        <span className="absolute bottom-0 right-0 z-10 translate-x-1/4 translate-y-1/4 rounded-full bg-white px-2.5 py-0.5 text-[10px] font-bold tabular-nums text-slate-900 shadow-md ring-1 ring-black/10">
          {amount.toLocaleString()}
        </span>
      </div>
    </div>
  );
}

export const AutoStationSettingsModal: React.FC<AutoStationSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state: gameState, configuration } = useCitadelAPI();
  const castles = castleOptionsFromState(gameState);
  const [state, setState] = useState<AutoStationClientStateV1>(() => loadAutoStationClientState());

  useEffect(() => {
    if (!isOpen) return;
    setState(parseAutoStationClientState(
      configuration?.sections['automation.autoStation'] ?? loadAutoStationClientState(),
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

  const save = () => {
    persistAutoStationClientState(state);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="full"
      title={
        <div className="min-w-0">
          <span className="flex items-center gap-2 text-primary">
            <Shield className="h-5 w-5" />
            Auto Station Settings
          </span>
          <p className="mt-1 text-sm font-normal text-text-muted">
            Choose the exact troops that stay behind to defend. Every other troop currently in the threatened castle is temporarily stationed away.
          </p>
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose} className="px-6">Cancel</Button>
          <Button variant="primary" onClick={save} className="px-8" leftIcon={<Save className="h-4 w-4" />}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="flex w-full flex-col gap-6">
        <Card variant="solid" className="bg-bg-app p-4">
          <div className="grid gap-4 md:grid-cols-3">
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
              <span className="text-xs font-bold uppercase tracking-wider text-primary">Minimum target RPT</span>
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
            <div className="flex items-center justify-between gap-4 rounded-global border border-border-base bg-bg-card/50 px-4 py-3">
              <div>
                <div className="text-xs font-bold uppercase tracking-wider text-primary">Recall when clear</div>
                <p className="mt-1 text-xs text-text-muted">Use MCM after the last attack lands and three fresh checks show no incoming threat.</p>
              </div>
              <Switch
                checked={state.recallWhenClear}
                onChange={(checked) => setState((previous) => ({ ...previous, recallWhenClear: checked }))}
              />
            </div>
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
                        <ReserveTile
                          key={troop.id}
                          unitID={troop.id}
                          amount={troop.amount}
                          onRemove={() => removeReserve(castleID, troop.id)}
                        />
                      ))}
                      <button
                        type="button"
                        onClick={() => selectReserve(castle)}
                        className="flex h-[76px] w-[76px] items-center justify-center rounded-global border-2 border-dashed border-border-base text-text-muted transition-colors hover:border-primary/50 hover:bg-primary/5 hover:text-primary"
                        aria-label="Edit defense troops"
                      >
                        <Plus className="h-8 w-8" strokeWidth={1.5} />
                      </button>
                    </div>
                  )}
                </Card>
              );
            })}
          </div>
        </div>
      </div>
    </Modal>
  );
};
