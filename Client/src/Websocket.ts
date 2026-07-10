import type { FeatureSchedules } from './settings/SchedulerTypes';
import { CommandJsonMockRuntime } from './dev/CommandJsonMock';

type MessageListener = (message: any) => void;
export type FrontendWebsocketStatus = 'Disconnected' | 'Connecting' | 'Connected';
type StatusListener = (status: FrontendWebsocketStatus) => void;

class FrontendWebsocketService {
  private socket: WebSocket | null = null;
  private listeners: MessageListener[] = [];
  private statusListeners: StatusListener[] = [];
  private mock: boolean = import.meta.env.VITE_MOCK_WEBSOCKET === 'true';
  private status: FrontendWebsocketStatus = 'Disconnected';
  private url: string | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private intentionalClose = false;
  private mockRuntime: CommandJsonMockRuntime | null = null;

  public connect(url: string) {
    this.url = url;
    this.intentionalClose = false;
    this.openSocket();
  }

  private openSocket() {
    if (this.mock) {
      this.setStatus('Connecting');
      setTimeout(() => {
        this.setStatus('Connected');
        if (!this.mockRuntime) {
          this.mockRuntime = new CommandJsonMockRuntime((message) => this.emitMessage(message));
        }
        this.mockRuntime.start().catch((error) => {
          console.error('Failed to start command JSON mock:', error);
          this.emitLocalAlert('red', 'Could not load dev mock command JSON. Check Logs/RecvCommandsJSON/gbd.json.');
        });
      }, 250);
      return;
    }

    if (!this.url) {
      return;
    }

    if (this.socket) {
      const state = this.socket.readyState;
      if (state === WebSocket.OPEN || state === WebSocket.CONNECTING) {
        return;
      }
      this.socket = null;
    }

    this.setStatus('Connecting');
    let socket: WebSocket;
    try {
      socket = new WebSocket(this.url);
    } catch (error) {
      console.error('Failed to create WebSocket:', error);
      this.setStatus('Disconnected');
      this.scheduleReconnect();
      return;
    }
    this.socket = socket;

    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.reconnectAttempt = 0;
      this.setStatus('Connected');
      // Defer follow-up requests so server SendInitialData can drain first.
      window.setTimeout(() => this.sendOnOpenRequests(), 300);
    };

    socket.onclose = () => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.setStatus('Disconnected');
      this.scheduleReconnect();
    };

    socket.onerror = (error) => {
      if (this.socket !== socket) return;
      console.error('WebSocket error:', error);
    };

    socket.onmessage = (event) => {
      if (this.socket !== socket) return;
      try {
        const message = JSON.parse(event.data);
        this.emitMessage(message);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };
  }

  private sendOnOpenRequests() {
    this.sendMessage({ type: 'getSchedulerSettings' });
    this.sendMessage({ type: 'getRecruitTroopsSettings' });
    this.sendMessage({ type: 'getAutoToolSettings' });
    this.sendMessage({ type: 'getAutoBirdClientState' });
    this.sendMessage({ type: 'getAutoTCIClientState' });
  }

  private scheduleReconnect() {
    if (this.mock || !this.url || this.intentionalClose) {
      return;
    }
    if (this.reconnectTimer != null) {
      return;
    }
    const delay = Math.min(30_000, 1000 * 2 ** this.reconnectAttempt);
    this.reconnectAttempt += 1;
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.openSocket();
    }, delay);
  }

  private emitMessage(message: any) {
    this.listeners.forEach((listener) => listener(message));
  }

  private setStatus(status: FrontendWebsocketStatus) {
    if (this.status === status) return;
    this.status = status;
    this.statusListeners.forEach((listener) => listener(status));
  }

  public addMessageListener(listener: MessageListener) {
    this.listeners.push(listener);
  }

  public removeMessageListener(listener: MessageListener) {
    this.listeners = this.listeners.filter((l) => l !== listener);
  }

  public addStatusListener(listener: StatusListener) {
    this.statusListeners.push(listener);
    listener(this.status);
  }

  public removeStatusListener(listener: StatusListener) {
    this.statusListeners = this.statusListeners.filter((l) => l !== listener);
  }

  public sendMessage(message: object): boolean {
    if (this.mock) {
      return this.mockRuntime?.handleClientMessage(message) ?? true;
    }

    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(message));
      return true;
    }
    console.error('WebSocket is not connected. Cannot send message:', message);
    this.emitLocalAlert('red', 'Dashboard disconnected — reconnecting. Try again in a moment.');
    return false;
  }

  /** Show a toast in the global Alerts stack (works even when the server is not reached). */
  public showAlert(category: 'green' | 'yellow' | 'red', message: string) {
    const alert = { type: 'alert', payload: { category, message } };
    this.emitMessage(alert);
  }

  private emitLocalAlert(category: 'green' | 'yellow' | 'red', message: string) {
    this.showAlert(category, message);
  }

  public getStatus(): FrontendWebsocketStatus {
    return this.status;
  }

  public startGame() {
    this.sendMessage({ type: 'startGame' });
  }

  public stopGame() {
    this.sendMessage({ type: 'stopGame' });
  }

  public refreshEquipment() {
    this.sendMessage({ type: 'refreshEquipment' });
  }

  public sendFetchAllianceInfo(): boolean {
    return this.sendMessage({ type: 'fetchAllianceInfo' });
  }

  public refreshSingleCommander(equipmentMode: 'Commander' | 'Castellan', targetIndex: number) {
    this.sendMessage({
      type: 'refreshSingleCommander',
      payload: { equipmentMode, targetIndex }
    });
  }

  public sendReconfigureLoadout(payload: {
    equipmentMode: 'Commander' | 'Castellan';
    combatMode: 'PvP' | 'PvE';
    targetIndex: number;
    stats: Array<{
      stat: string;
      tier: number;
      position: number;
    }>;
  }) {
    this.sendMessage({
      type: 'reconfigureLoadout',
      payload: payload
    });
  }

  public sendConfirmReconfigure(targetIndex: number, currentLoadout: any, newLoadout: any, equipmentMode: 'Commander' | 'Castellan') {
    this.sendMessage({
      type: 'confirmReconfigure',
      payload: {
        targetIndex,
        currentLoadout,
        newLoadout,
        equipmentMode
      }
    });
  }

  public triggerUpdate(downloadUrl: string) {
    this.sendMessage({
      type: 'triggerUpdate',
      payload: { downloadUrl }
    });
  }
  public sendGetSchedulerSettings() {
    this.sendMessage({
      type: 'getSchedulerSettings'
    });
  }

  public sendSaveSchedulerSettings(payload: Partial<{
    minAttackDelay: number;
    maxAttackDelay: number;
    upgradeEreDelayMs: number;
    upgradeCoinThreshold: number;
    manualFocusIdleSec: number;
    tabPriorities: Record<string, string>;
    featureSchedules: FeatureSchedules;
  }>): boolean {
    return this.sendMessage({
      type: 'saveSchedulerSettings',
      payload: payload
    });
  }

  public sendGetCastleFocus() {
    this.sendMessage({ type: 'getCastleFocus' });
  }

  /** Rift view: sole world Rift tile (GAA type 43). Set refresh to re-request GAA. */
  public sendGetRiftMapCoords(refresh = false) {
    this.sendMessage({ type: 'getRiftMapCoords', payload: { refresh } });
  }

  /** Rift view: last saved outbound cra launch targeting the Rift. */
  public sendGetRiftCRALaunch() {
    this.sendMessage({ type: 'getRiftCRALaunch' });
  }

  /** Movement view: active GAM movements. Set refresh to re-request **gam**. */
  public sendGetMovement(refresh = false) {
    this.sendMessage({ type: 'getMovement', payload: { refresh } });
  }

  /** Re-queue one saved Rift cra template (optional commander / source overrides). */
  public sendReplayRiftCRALaunch(options: {
    launchId: string;
    commanderID?: number;
    sourceX?: number;
    sourceY?: number;
    arriveAtUnix?: number;
  }) {
    this.sendMessage({ type: 'replayRiftCRALaunch', payload: options });
  }

  /** Queue dummy 1-wave Rift attacks for eligible shield-maiden commanders. */
  public sendMaidenCommsWave(options?: { sourceX?: number; sourceY?: number; unitWodID?: number }): boolean {
    return this.sendMessage({ type: 'sendMaidenCommsWave', payload: options ?? {} });
  }

  public sendRenameRiftCRALaunch(launchId: string, displayName: string) {
    this.sendMessage({ type: 'renameRiftCRALaunch', payload: { launchId, displayName } });
  }

  public sendDeleteRiftCRALaunch(launchId: string) {
    this.sendMessage({ type: 'deleteRiftCRALaunch', payload: { launchId } });
  }

  public sendGetRiftMaidenCommsSettings() {
    this.sendMessage({ type: 'getRiftMaidenCommsSettings' });
  }

  public sendSaveRiftMaidenCommsSettings(unitWodID: number) {
    this.sendMessage({ type: 'saveRiftMaidenCommsSettings', payload: { unitWodID } });
  }

  /**
   * Ask server to send JCA/JAA for the castle (GameCommands.SendCastleFocus).
   * Pass kingdom + map coords from castleResourceUpdate / initial details (troops.kingdomID, troops.x, troops.y).
   */
  public sendFocusPlayerCastle(payload: {
    castleId: number;
    kingdomId: number;
    mapX: number;
    mapY: number;
  }) {
    this.sendMessage({
      type: 'focusPlayerCastle',
      payload: {
        castleId: payload.castleId,
        kingdomId: payload.kingdomId,
        mapX: payload.mapX,
        mapY: payload.mapY,
      },
    });
  }

  public sendGetDecorationPresets(castleId?: number) {
    this.sendMessage({
      type: 'getDecorationPresets',
      payload: castleId != null && castleId > 0 ? { castleId } : {}
    });
  }

  public sendSaveDecorationPreset(name: string, castleId?: number) {
    this.sendMessage({
      type: 'saveDecorationPreset',
      payload: { name, ...(castleId != null && castleId > 0 ? { castleId } : {}) }
    });
  }

  public sendDeleteDecorationPreset(castleId: number, presetId: string) {
    this.sendMessage({
      type: 'deleteDecorationPreset',
      payload: { castleId, presetId }
    });
  }

  public sendApplyDecorationPreset(castleId: number, presetId: string, kingdomId?: number) {
    this.sendMessage({
      type: 'applyDecorationPreset',
      payload: {
        castleId,
        presetId,
        ...(kingdomId != null ? { kingdomId } : {})
      }
    });
  }

  public sendCancelDecorationApply() {
    this.sendMessage({ type: 'cancelDecorationApply' });
  }
}


export const FrontendWebsocket = new FrontendWebsocketService();
