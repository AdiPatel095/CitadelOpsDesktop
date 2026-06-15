import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Shield, Users } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { useCastleResources } from '../../dashboard/context/CastleResourceContext';
import { showTroopPicker } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { TROOP_DEFINITIONS } from '../../config/Constants';
import { Card, CardContent, CardHeader, CardTitle, Button } from '../../components/ui';
import { FrontendWebsocket } from '../../Websocket';
import {
  mainCastleAvailableUnitIds,
  mainCastleStockQuantities,
  resolveMainCastleTroops,
} from '../types/MainCastleTroops';
import {
  DEFAULT_MAIDEN_PROBE_UNIT_ID,
  parseRiftMaidenCommsSettings,
} from '../types/RiftMaidenCommsSettings';

const RiftMaidenCommsPanel: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();
  const { castleResources } = useCastleResources();
  const { snapshot } = useLastKnownSnapshot();
  const [unitWodID, setUnitWodID] = useState(DEFAULT_MAIDEN_PROBE_UNIT_ID);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [dashboardConnected, setDashboardConnected] = useState(
    () => FrontendWebsocket.getStatus() === 'Connected'
  );
  const [sendStatus, setSendStatus] = useState<string | null>(null);

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
    FrontendWebsocket.sendSaveRiftMaidenCommsSettings(preferred);
  }, [availableUnitIds, settingsLoaded, unitWodID]);

  useEffect(() => {
    const sync = () => setDashboardConnected(FrontendWebsocket.getStatus() === 'Connected');
    sync();
    const id = window.setInterval(sync, 500);
    const onMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type === 'maidenCommsWaveResult' || message.type === 'alert') {
        setSendStatus(null);
      }
      if (message.type === 'riftMaidenCommsSettings' && message.payload != null) {
        const parsed = parseRiftMaidenCommsSettings(message.payload);
        setUnitWodID(parsed.unitWodID);
        setSettingsLoaded(true);
      }
    };
    FrontendWebsocket.addMessageListener(onMessage);
    FrontendWebsocket.sendGetRiftMaidenCommsSettings();
    return () => {
      window.clearInterval(id);
      FrontendWebsocket.removeMessageListener(onMessage);
    };
  }, []);

  const selectedInStock = unitWodID > 0 && (stockQuantities[unitWodID] ?? 0) > 0;
  const unitLabel = TROOP_DEFINITIONS[unitWodID] ?? `Unit ${unitWodID}`;

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
      FrontendWebsocket.sendSaveRiftMaidenCommsSettings(result);
    }
  }, [availableUnitIds, stockQuantities, unitWodID]);

  const handleSend = useCallback(() => {
    if (sendStatus != null) {
      return;
    }
    if (!dashboardConnected) {
      FrontendWebsocket.showAlert('red', 'Dashboard disconnected — wait for reconnect, then try again.');
      return;
    }
    if (!gameLoggedIn) {
      FrontendWebsocket.showAlert('red', 'Game not connected — log in before sending maiden comms.');
      return;
    }
    if (!mainCastle || availableUnitIds.length === 0) {
      FrontendWebsocket.showAlert(
        'yellow',
        'No main castle troop data yet — connect, wait for castle sync, then try again.'
      );
      return;
    }
    if (!selectedInStock) {
      FrontendWebsocket.showAlert('yellow', 'Pick a probe unit that is in main castle stock.');
      return;
    }
    const useFocusCoords =
      castleFocus?.mapPX != null &&
      castleFocus?.mapPY != null &&
      (castleFocus.mapPX !== 0 || castleFocus.mapPY !== 0);
    setSendStatus('Sending maiden comms wave…');
    FrontendWebsocket.showAlert('yellow', 'Maiden comms wave requested…');
    const sent = FrontendWebsocket.sendMaidenCommsWave({
      unitWodID,
      ...(useFocusCoords ? { sourceX: castleFocus!.mapPX, sourceY: castleFocus!.mapPY } : {}),
    });
    if (!sent) {
      setSendStatus(null);
      FrontendWebsocket.showAlert('red', 'Could not send — dashboard websocket is not open.');
    }
  }, [
    availableUnitIds.length,
    castleFocus,
    dashboardConnected,
    gameLoggedIn,
    mainCastle,
    selectedInStock,
    sendStatus,
    unitWodID,
  ]);

  return (
    <Card className="border-border-base bg-bg-app/20">
      <CardHeader className="pb-3 border-b border-border-base bg-bg-card-hover/50 rounded-t-[calc(var(--radius-global)-1px)]">
        <div>
          <CardTitle className="text-lg text-primary">Maiden comms wave</CardTitle>
          <p className="text-xs text-text-muted mt-1">
            Sends a dummy 1-wave Rift attack (11 per flank) for each commander that is not busy and wears a relic with
            shield-maiden support (300–1050). Probe unit comes from main castle stock (last troop read).
          </p>
        </div>
      </CardHeader>
      <CardContent className="pt-4 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
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
            disabled={sendStatus != null}
            onClick={handleSend}
            title="Send maiden comms wave to the Rift"
            leftIcon={<Shield className="w-3.5 h-3.5" />}
          >
            {sendStatus ? 'Sending…' : 'Send maiden comms'}
          </Button>
        </div>
        {sendStatus ? (
          <p className="text-xs text-amber-400/90 text-right">{sendStatus}</p>
        ) : !dashboardConnected ? (
          <p className="text-xs text-red-400/90 text-right">Dashboard websocket disconnected.</p>
        ) : null}
      </CardContent>
    </Card>
  );
};

export default RiftMaidenCommsPanel;
