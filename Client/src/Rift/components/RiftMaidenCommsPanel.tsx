import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Shield, Users } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { showTroopPicker } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { Button, SectionCard } from '../../components/ui';
import {
  mainCastleAvailableUnitIds,
  mainCastleStockQuantities,
  resolveMainCastleTroops,
} from '../types/MainCastleTroops';
import {
  DEFAULT_MAIDEN_PROBE_UNIT_ID,
  parseRiftMaidenCommsSettings,
} from '../types/RiftMaidenCommsSettings';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import { useRiftMap } from '../context/RiftMapContext';
import {
  COMMANDER_FEATURE_SECTION,
  parseCommanderFeatureAssignments,
} from '../../Movement/types/CommanderFeatureAssignments';
import HorseTravelBoostSelect from '../../settings/components/HorseTravelBoostSelect';
import type { HorseTravelBoostID } from '../../settings/HorseTravelBoost';
import {
  RIFT_ATTACK_PREFERENCES_SECTION,
  parseRiftAttackPreferences,
} from '../types/RiftAttackPreferences';

const PROBE_UNITS_PER_COMMANDER = 33;

const RiftMaidenCommsPanel: React.FC = () => {
  const { state, configuration, connectionStatus, submitIntent, updateConfiguration } = useCitadelAPI();
  const { getTroop } = useMetadata();
  const { gameLoggedIn } = useAuth();
  const { castle } = useCastleFocus();
  const { riftMapCoords } = useRiftMap();
  const [unitWodID, setUnitWodID] = useState(DEFAULT_MAIDEN_PROBE_UNIT_ID);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [savingUnit, setSavingUnit] = useState(false);
  const [sending, setSending] = useState(false);
  const [horseTravelBoostId, setHorseTravelBoostId] = useState<HorseTravelBoostID>(-1);
  const [sendStatus, setSendStatus] = useState<{ message: string; error: boolean } | null>(null);
  const dashboardConnected = connectionStatus === 'Connected';
  const assignedMaidenCommanders = useMemo(
    () => parseCommanderFeatureAssignments(configuration?.sections[COMMANDER_FEATURE_SECTION]).assignments.riftMaiden,
    [configuration?.sections],
  );

  const mainCastle = useMemo(
    () => resolveMainCastleTroops(state),
    [state]
  );

  const availableUnitIds = useMemo(
    () => (mainCastle ? mainCastleAvailableUnitIds(mainCastle.troopsI) : []),
    [mainCastle]
  );

  const stockQuantities = useMemo(
    () => (mainCastle ? mainCastleStockQuantities(mainCastle.troopsI) : {}),
    [mainCastle]
  );

  const probeReadyUnitIds = useMemo(
    () => availableUnitIds.filter((id) => (stockQuantities[id] ?? 0) >= PROBE_UNITS_PER_COMMANDER),
    [availableUnitIds, stockQuantities]
  );

  useEffect(() => {
    if (!configuration || settingsLoaded) return;
    const parsed = parseRiftMaidenCommsSettings(configuration.sections['rift.maidenComms']);
    setUnitWodID(parsed.unitWodID);
    setSettingsLoaded(true);
  }, [configuration, settingsLoaded]);

  useEffect(() => {
    const preferences = parseRiftAttackPreferences(configuration?.sections[RIFT_ATTACK_PREFERENCES_SECTION]);
    setHorseTravelBoostId(preferences.maidenHorseTravelBoostId);
  }, [configuration?.sections]);

  const updateMaidenHorseTravelBoost = useCallback((next: HorseTravelBoostID) => {
    setHorseTravelBoostId(next);
    const current = parseRiftAttackPreferences(configuration?.sections[RIFT_ATTACK_PREFERENCES_SECTION]);
    void updateConfiguration(RIFT_ATTACK_PREFERENCES_SECTION, { ...current, maidenHorseTravelBoostId: next })
      .catch((error) => setSendStatus({
        message: error instanceof Error ? error.message : 'Could not save the maiden-wave travel boost.',
        error: true,
      }));
  }, [configuration?.sections, updateConfiguration]);

  const selectedQuantity = stockQuantities[unitWodID] ?? 0;
  const selectedInStock = unitWodID > 0 && selectedQuantity >= PROBE_UNITS_PER_COMMANDER;
  const stockCommanderCapacity = Math.floor(selectedQuantity / PROBE_UNITS_PER_COMMANDER);
  const unitLabel = getTroop(unitWodID)?.name ?? `Unit ${unitWodID}`;
  const sendBlockedReason = !settingsLoaded
    ? 'Loading the saved probe unit.'
    : !dashboardConnected
      ? 'Dashboard disconnected — wait for it to reconnect.'
      : !gameLoggedIn
        ? 'Game disconnected — start the bot before launching.'
        : !riftMapCoords?.found
          ? 'Rift location unknown — discover it on the world map first.'
          : !mainCastle || availableUnitIds.length === 0
            ? 'Main castle troop data is not ready yet.'
            : !selectedInStock
              ? `Choose a unit with at least ${PROBE_UNITS_PER_COMMANDER} in main castle stock.`
              : null;

  const handlePickUnit = useCallback(async () => {
    if (!settingsLoaded || savingUnit || probeReadyUnitIds.length === 0) return;
    const result = await showTroopPicker({
      mode: 'single',
      title: `Probe unit (${PROBE_UNITS_PER_COMMANDER}+ in main castle)`,
      preselected: unitWodID > 0 ? [unitWodID] : [],
      allowedUnitIds: probeReadyUnitIds,
      stockQuantities,
    });
    if (typeof result === 'number' && result > 0) {
      const previousUnitWodID = unitWodID;
      const selectedLabel = getTroop(result)?.name ?? `Unit ${result}`;
      setUnitWodID(result);
      setSavingUnit(true);
      setSendStatus({ message: `Saving ${selectedLabel} as the probe unit…`, error: false });
      try {
        await updateConfiguration('rift.maidenComms', { unitWodID: result });
        setSendStatus({ message: `${selectedLabel} saved as the probe unit.`, error: false });
      } catch (error) {
        setUnitWodID(previousUnitWodID);
        setSendStatus({
          message: error instanceof Error ? error.message : 'Could not save the probe unit.',
          error: true,
        });
      } finally {
        setSavingUnit(false);
      }
    }
  }, [getTroop, probeReadyUnitIds, savingUnit, settingsLoaded, stockQuantities, unitWodID, updateConfiguration]);

  const handleSend = useCallback(() => {
    if (sending) {
      return;
    }
    if (sendBlockedReason) {
      setSendStatus({ message: sendBlockedReason, error: true });
      return;
    }
    const useFocusCoords = castle != null && (castle.x !== 0 || castle.y !== 0);
    setSending(true);
    setSendStatus({ message: 'Submitting maiden comms wave…', error: false });
    void submitIntent('rift.maiden_wave.launch', {
      unitWodID,
      horseTravelBoostId,
      ...(assignedMaidenCommanders == null ? {} : { commanderIds: assignedMaidenCommanders }),
      ...(useFocusCoords ? { sourceX: castle!.x, sourceY: castle!.y } : {}),
    })
      .then(() => setSendStatus({ message: 'Maiden comms wave submitted.', error: false }))
      .catch((error) => setSendStatus({
        message: error instanceof Error ? error.message : 'Could not send maiden comms.',
        error: true,
      }))
      .finally(() => setSending(false));
  }, [
    castle,
    assignedMaidenCommanders,
    horseTravelBoostId,
    sendBlockedReason,
    sending,
    submitIntent,
    unitWodID,
  ]);

  return (
    <SectionCard variant="glass" title="Maiden comms wave" titleClassName="text-lg text-primary"
      description="Sends a dummy 1-wave Rift attack (11 per flank) for each commander that is not busy and wears a relic with shield-maiden support (300–1050). Probe unit comes from main castle stock (last troop read)."
      descriptionClassName="" contentClassName="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-col gap-2 min-w-0">
          <p className="text-sm text-text-muted">
            Launch point: {castle ? `focused castle (${castle.name?.trim() || castle.id})` : 'main castle'}.
            {' '}Attacks are staggered (4–5s apart, or Settings attack delay range).
          </p>
          {mainCastle ? (
            <div className="flex flex-col gap-1">
              <p className="text-xs text-text-muted font-mono">
                Main castle AID {mainCastle.aid} · {probeReadyUnitIds.length} probe-ready unit type
                {probeReadyUnitIds.length === 1 ? '' : 's'}
              </p>
              <p className={`text-xs ${sendBlockedReason ? 'text-warning' : 'text-success'}`}>
                {sendBlockedReason ?? `Ready · selected stock supports up to ${stockCommanderCapacity} commander probe${stockCommanderCapacity === 1 ? '' : 's'}.`}
              </p>
            </div>
          ) : (
            <p className="text-xs text-amber-400/90">No main castle troop data yet — connect and wait for castle sync.</p>
          )}
          {sendStatus ? (
            <p
              role="status"
              aria-live="polite"
              className={`text-xs ${sendStatus.error ? 'text-error' : 'text-success'}`}
            >
              {sendStatus.message}
            </p>
          ) : null}
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 shrink-0">
          <div className="min-w-[15rem]">
            <HorseTravelBoostSelect value={horseTravelBoostId} onChange={updateMaidenHorseTravelBoost} />
          </div>
          <Button
            variant="secondary"
            size="sm"
            disabled={!settingsLoaded || savingUnit || probeReadyUnitIds.length === 0}
            isLoading={savingUnit}
            onClick={handlePickUnit}
            title={
              !settingsLoaded
                ? 'Loading the saved probe unit'
                : probeReadyUnitIds.length === 0
                  ? `No main castle unit stack has the required ${PROBE_UNITS_PER_COMMANDER} troops`
                  : `Pick a probe unit with at least ${PROBE_UNITS_PER_COMMANDER} in main castle stock`
            }
            leftIcon={<Users className="w-3.5 h-3.5" />}
          >
            Pick unit
          </Button>

          <div
            className={`flex items-center gap-2 rounded-lg border px-2.5 py-1.5 min-w-[140px] ${
              selectedInStock ? 'border-border-base bg-bg-card/50' : 'border-amber-500/40 bg-amber-500/5'
            }`}
            title={selectedInStock ? unitLabel : 'Selected unit is not in main castle stock'}
          >
            {unitWodID > 0 ? (
              <UnitImage unitId={unitWodID} size={28} showLevel={false} className="rounded-md shrink-0" />
            ) : null}
            <div className="min-w-0">
              <p className="text-xs font-semibold text-text-main truncate">{unitLabel}</p>
              <p className="text-[10px] font-mono text-text-muted">
                {selectedInStock
                  ? `${selectedQuantity.toLocaleString()} · up to ${stockCommanderCapacity} probes`
                  : `${selectedQuantity.toLocaleString()} · ${PROBE_UNITS_PER_COMMANDER} required`}
              </p>
            </div>
          </div>

          <Button
            variant="primary"
            size="sm"
            disabled={sending || sendBlockedReason != null}
            isLoading={sending}
            onClick={handleSend}
            title={sendBlockedReason ?? 'Send maiden comms wave to the Rift'}
            leftIcon={<Shield className="w-3.5 h-3.5" />}
          >
            {sending ? 'Sending…' : 'Send maiden comms'}
          </Button>
        </div>
    </SectionCard>
  );
};

export default RiftMaidenCommsPanel;
