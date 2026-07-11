import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Shield, Users } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { useCastleResources } from '../../dashboard/context/CastleResourceContext';
import { showTroopPicker } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { Card, CardContent, CardHeader, CardTitle, Button } from '../../components/ui';
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

const RiftMaidenCommsPanel: React.FC = () => {
  const { configuration, connectionStatus, submitIntent, updateConfiguration } = useCitadelAPI();
  const { getTroop } = useMetadata();
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();
  const { castleResources } = useCastleResources();
  const { snapshot } = useLastKnownSnapshot();
  const [unitWodID, setUnitWodID] = useState(DEFAULT_MAIDEN_PROBE_UNIT_ID);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [sending, setSending] = useState(false);
  const [sendStatus, setSendStatus] = useState<string | null>(null);
  const dashboardConnected = connectionStatus === 'Connected';

  const mainCastle = useMemo(
    () => resolveMainCastleTroops(castleResources, snapshot?.gameState),
    [castleResources, snapshot?.gameState]
  );

  const availableUnitIds = useMemo(
    () => (mainCastle ? mainCastleAvailableUnitIds(mainCastle.troopsI) : []),
    [mainCastle]
  );

  const stockQuantities = useMemo(
    () => (mainCastle ? mainCastleStockQuantities(mainCastle.troopsI) : {}),
    [mainCastle]
  );

  useEffect(() => {
    if (!settingsLoaded || availableUnitIds.length === 0) return;
    if (availableUnitIds.includes(unitWodID)) return;
    const preferred =
      availableUnitIds.find((id) => id === DEFAULT_MAIDEN_PROBE_UNIT_ID) ?? availableUnitIds[0];
    setUnitWodID(preferred);
    void updateConfiguration('rift.maidenComms', { unitWodID: preferred });
  }, [availableUnitIds, settingsLoaded, unitWodID, updateConfiguration]);

  useEffect(() => {
    if (!configuration) return;
    const parsed = parseRiftMaidenCommsSettings(configuration.sections['rift.maidenComms']);
    setUnitWodID(parsed.unitWodID);
    setSettingsLoaded(true);
  }, [configuration]);

  const selectedInStock = unitWodID > 0 && (stockQuantities[unitWodID] ?? 0) > 0;
  const unitLabel = getTroop(unitWodID)?.name ?? `Unit ${unitWodID}`;

  const handlePickUnit = useCallback(async () => {
    if (availableUnitIds.length === 0) return;
    const result = await showTroopPicker({
      mode: 'single',
      title: 'Probe unit (main castle stock)',
      preselected: unitWodID > 0 ? [unitWodID] : [],
      allowedUnitIds: availableUnitIds,
      stockQuantities,
    });
    if (typeof result === 'number' && result > 0) {
      setUnitWodID(result);
      void updateConfiguration('rift.maidenComms', { unitWodID: result });
    }
  }, [availableUnitIds, stockQuantities, unitWodID, updateConfiguration]);

  const handleSend = useCallback(() => {
    if (sending) {
      return;
    }
    if (!dashboardConnected) {
      setSendStatus('Dashboard disconnected — wait for reconnect, then try again.');
      return;
    }
    if (!gameLoggedIn) {
      setSendStatus('Game not connected — log in before sending maiden comms.');
      return;
    }
    if (!mainCastle || availableUnitIds.length === 0) {
      setSendStatus('No main castle troop data yet — connect and wait for castle sync.');
      return;
    }
    if (!selectedInStock) {
      setSendStatus('Pick a probe unit that is in main castle stock.');
      return;
    }
    const useFocusCoords =
      castleFocus?.mapPX != null &&
      castleFocus?.mapPY != null &&
      (castleFocus.mapPX !== 0 || castleFocus.mapPY !== 0);
    setSending(true);
    setSendStatus('Sending maiden comms wave…');
    void submitIntent('rift.maiden_wave.launch', {
      unitWodID,
      ...(useFocusCoords ? { sourceX: castleFocus!.mapPX, sourceY: castleFocus!.mapPY } : {}),
    })
      .then(() => setSendStatus('Maiden comms wave submitted.'))
      .catch((error) => setSendStatus(error instanceof Error ? error.message : 'Could not send maiden comms.'))
      .finally(() => setSending(false));
  }, [
    availableUnitIds.length,
    castleFocus,
    dashboardConnected,
    gameLoggedIn,
    mainCastle,
    selectedInStock,
    sending,
    submitIntent,
    unitWodID,
  ]);

  return (
    <Card className="liquid-prominent-header-card">
      <CardHeader className="liquid-card-header-prominent">
        <div>
          <CardTitle className="text-lg text-primary">Maiden comms wave</CardTitle>
          <p className="text-xs text-text-muted mt-1">
            Sends a dummy 1-wave Rift attack (11 per flank) for each commander that is not busy and wears a relic with
            shield-maiden support (300–1050). Probe unit comes from main castle stock (last troop read).
          </p>
        </div>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-col gap-2 min-w-0">
          <p className="text-sm text-text-muted">
            Launch point: focused castle. Attacks are staggered (4–5s apart, or Settings attack delay range).
          </p>
          {mainCastle ? (
            <p className="text-xs text-text-muted font-mono">
              Main castle AID {mainCastle.aid} · {availableUnitIds.length} unit type
              {availableUnitIds.length === 1 ? '' : 's'} in stock
            </p>
          ) : (
            <p className="text-xs text-amber-400/90">No main castle troop data yet — connect and wait for castle sync.</p>
          )}
          {sendStatus && <p className="text-xs text-warning">{sendStatus}</p>}
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 shrink-0">
          <Button
            variant="secondary"
            size="sm"
            disabled={!gameLoggedIn || availableUnitIds.length === 0}
            onClick={handlePickUnit}
            title={
              availableUnitIds.length === 0
                ? 'No units in main castle stock'
                : 'Pick probe unit from main castle'
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
                  ? `${(stockQuantities[unitWodID] ?? 0).toLocaleString()} in castle`
                  : 'not in stock'}
              </p>
            </div>
          </div>

          <Button
            variant="primary"
            size="sm"
            disabled={sending}
            onClick={handleSend}
            title="Send maiden comms wave to the Rift"
            leftIcon={<Shield className="w-3.5 h-3.5" />}
          >
            {sending ? 'Sending…' : 'Send maiden comms'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
};

export default RiftMaidenCommsPanel;
