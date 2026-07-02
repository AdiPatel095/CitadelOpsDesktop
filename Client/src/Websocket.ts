import type { FeatureSchedules } from './settings/SchedulerTypes';

type MessageListener = (message: any) => void;

class FrontendWebsocketService {
  private socket: WebSocket | null = null;
  private listeners: MessageListener[] = [];
  private mock: boolean = import.meta.env.VITE_MOCK_WEBSOCKET === 'true';
  private status: string = 'Disconnected';
  private url: string | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private intentionalClose = false;

  public connect(url: string) {
    this.url = url;
    this.intentionalClose = false;
    this.openSocket();
  }

  private openSocket() {
    if (this.mock) {
      this.status = 'Connecting';
      setTimeout(() => {
        this.status = 'Connected';
        this.sendMockData();
      }, 1000);
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

    this.status = 'Connecting';
    this.socket = new WebSocket(this.url);

    this.socket.onopen = () => {
      this.reconnectAttempt = 0;
      this.status = 'Connected';
      // Defer follow-up requests so server SendInitialData can drain first.
      window.setTimeout(() => this.sendOnOpenRequests(), 300);
    };

    this.socket.onclose = () => {
      this.socket = null;
      this.status = 'Disconnected';
      this.scheduleReconnect();
    };

    this.socket.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    this.socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.listeners.forEach((listener) => listener(message));
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

  private sendMockData() {
    // Mock resource update
    const mockResources = {
      type: 'globalResourceUpdate',
      payload: {
        rubies: 1234,
        coins: 567890,
        relic_shard: 100,
        sceat: 2500,
        ducat: 50,
        const_token: 5,
        upgr_token: 2,
        affl_tix: 10,
        plaster: 500,
        drg_scale: 20,
        drg_spl: 15,
        min1: 30,
        min5: 10,
        min10: 5,
        min30: 2,
        hr1: 1,
        hr5: 0,
        hr24: 0,
        might_pt: 15000,
        glory_pt: 7500,
        gallan_pt: 1000,
      },
    };
    this.listeners.forEach((listener) => listener(mockResources));

    // Mock alert
    setTimeout(() => {
      this.listeners.forEach((listener) => listener({
        type: 'alert',
        payload: {
          category: 'green',
          message: 'Successfully connected and synced with CitadelOps Network.'
        }
      }));
    }, 2000);
  }

  public addMessageListener(listener: MessageListener) {
    this.listeners.push(listener);
  }

  public removeMessageListener(listener: MessageListener) {
    this.listeners = this.listeners.filter((l) => l !== listener);
  }

  public sendMessage(message: object): boolean {
    if (this.mock) {
      return true;
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
    this.listeners.forEach((listener) => listener(alert));
  }

  private emitLocalAlert(category: 'green' | 'yellow' | 'red', message: string) {
    this.showAlert(category, message);
  }

  public getStatus(): string {
    if (this.mock) {
      return this.status;
    }
    if (!this.socket) {
      return 'Disconnected';
    }
    switch (this.socket.readyState) {
      case WebSocket.OPEN:
        return 'Connected';
      case WebSocket.CONNECTING:
        return 'Connecting';
      case WebSocket.CLOSING:
        return 'Closing';
      case WebSocket.CLOSED:
        return 'Disconnected';
      default:
        return 'Unknown';
    }
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
