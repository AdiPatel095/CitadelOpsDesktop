import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Shield, Users } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
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
  commanderIDsEligibleForFeature,
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
  const { riftMapCoords } = useRiftMap();
  const [unitWodID, setUnitWodID] = useState(DEFAULT_MAIDEN_PROBE_UNIT_ID);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [savingUnit, setSavingUnit] = useState(false);
  const [sending, setSending] = useState(false);
	const [cancelling, setCancelling] = useState(false);
	const [probeGoal, setProbeGoal] = useState(1);
  const [horseTravelBoostId, setHorseTravelBoostId] = useState<HorseTravelBoostID>(-1);
  const [sendStatus, setSendStatus] = useState<{ message: string; error: boolean } | null>(null);
  const dashboardConnected = connectionStatus === 'Connected';
  const assignedMaidenCommanders = useMemo(
    () => commanderIDsEligibleForFeature(
      parseCommanderFeatureAssignments(configuration?.sections[COMMANDER_FEATURE_SECTION]),
      'riftMaiden',
      Object.values(state?.commanders ?? {}).map((commander) => commander.id),
      state,
    ),
    [configuration?.sections, state],
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
	const maidenRun = state?.rift?.maidenRun;
	const runActive = maidenRun?.status === 'running';
	const runRemaining = maidenRun ? Math.max(0, maidenRun.requestedAttacks - maidenRun.attacksLaunched) : 0;
  const selectedInStock = unitWodID > 0 && selectedQuantity >= PROBE_UNITS_PER_COMMANDER;
  const stockCommanderCapacity = Math.floor(selectedQuantity / PROBE_UNITS_PER_COMMANDER);
  const unitLabel = getTroop(unitWodID)?.name ?? `Unit ${unitWodID}`;
  const sendBlockedReason = !settingsLoaded
    ? 'Loading the saved probe unit.'
    : runActive
		? `A Rift Maiden run is already active (${maidenRun.attacksLaunched}/${maidenRun.requestedAttacks}).`
		: !dashboardConnected
      ? 'Dashboard disconnected — wait for it to reconnect.'
      : !gameLoggedIn
        ? 'Game disconnected — start the bot before launching.'
        : assignedMaidenCommanders.length === 0
          ? 'Enable Rift Maiden Waves for at least one commander in Commanders / Functions.'
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
    setSending(true);
		setSendStatus({ message: `Starting a ${probeGoal}-probe Rift Maiden run…`, error: false });
		void submitIntent('rift.maiden_run.start', {
			attackCount: probeGoal,
      unitWodID,
      horseTravelBoostId,
      commanderIds: assignedMaidenCommanders,
    })
			.then(() => setSendStatus({ message: `${probeGoal}-probe Rift Maiden run started.`, error: false }))
      .catch((error) => setSendStatus({
        message: error instanceof Error ? error.message : 'Could not send maiden comms.',
        error: true,
      }))
      .finally(() => setSending(false));
  }, [
    assignedMaidenCommanders,
    horseTravelBoostId,
		probeGoal,
    sendBlockedReason,
    sending,
    submitIntent,
    unitWodID,
  ]);

	const handleCancel = useCallback(() => {
		if (!maidenRun || maidenRun.status !== 'running' || cancelling) return;
		setCancelling(true);
		setSendStatus({ message: 'Cancelling the Rift Maiden run…', error: false });
		void submitIntent('rift.maiden_run.cancel', { runId: maidenRun.id })
			.then(() => setSendStatus({ message: 'Rift Maiden run cancelled.', error: false }))
			.catch((error) => setSendStatus({
				message: error instanceof Error ? error.message : 'Could not cancel the Rift Maiden run.',
				error: true,
			}))
			.finally(() => setCancelling(false));
	}, [cancelling, maidenRun, submitIntent]);

  return (
    <SectionCard variant="solid" title="Maiden comms wave" titleClassName="text-lg text-primary"
      description="Starts an exact-count run of dummy 1-wave Rift attacks (11 per flank). Eligible commanders launch in rounds and automatically continue after they return until the requested number is confirmed."
      descriptionClassName="" contentClassName="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-col gap-2 min-w-0">
          <p className="text-sm text-text-muted">
			Launch point: {mainCastle?.name || 'main castle'}.
            {' '}Attacks are staggered (4–5s apart, or Settings attack delay range).
          </p>
					{maidenRun ? (
						<p className={`text-xs font-semibold ${runActive ? 'text-primary' : maidenRun.status === 'completed' ? 'text-success' : 'text-text-muted'}`}>
							{runActive
								? `Run active · ${maidenRun.attacksLaunched}/${maidenRun.requestedAttacks} confirmed · ${runRemaining} remaining`
								: `Last run ${maidenRun.status} · ${maidenRun.attacksLaunched}/${maidenRun.requestedAttacks} confirmed`}
						</p>
					) : null}
          {mainCastle ? (
            <div className="flex flex-col gap-1">
              <p className="text-xs text-text-muted font-mono">
				{mainCastle.name || 'Main castle'} · {probeReadyUnitIds.length} probe-ready unit type
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

					<label className="flex items-center gap-2 rounded-lg border border-border-base bg-bg-card/50 px-2.5 py-1.5">
						<span className="text-xs font-semibold text-text-muted">Probe goal</span>
						<input
							type="number"
							min={1}
							max={9999}
							step={1}
							value={probeGoal}
							disabled={runActive}
							onChange={(event) => {
								const value = Math.trunc(Number(event.target.value));
								setProbeGoal(Number.isFinite(value) ? Math.min(9999, Math.max(1, value)) : 1);
							}}
							className="w-20 bg-transparent text-right text-sm font-mono text-text-main outline-none disabled:opacity-60"
							aria-label="Total Rift Maiden probes to launch"
						/>
					</label>

					{runActive ? (
						<Button
							variant="secondary"
							size="sm"
							disabled={cancelling}
							isLoading={cancelling}
							onClick={handleCancel}
							title="Stop after already-dispatched probes"
						>
							{cancelling ? 'Cancelling…' : 'Cancel run'}
						</Button>
					) : null}

          <Button
            variant="primary"
            size="sm"
            disabled={sending || sendBlockedReason != null}
            isLoading={sending}
            onClick={handleSend}
				title={sendBlockedReason ?? `Start a run of exactly ${probeGoal} Rift probes`}
            leftIcon={<Shield className="w-3.5 h-3.5" />}
          >
				{sending ? 'Starting…' : `Send ${probeGoal} probe${probeGoal === 1 ? '' : 's'}`}
          </Button>
        </div>
    </SectionCard>
  );
};

export default RiftMaidenCommsPanel;
