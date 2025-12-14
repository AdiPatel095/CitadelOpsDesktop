type MessageListener = (message: any) => void;

class FrontendWebsocketService {
  private socket: WebSocket | null = null;
  private listeners: MessageListener[] = [];
  private mock: boolean = import.meta.env.VITE_MOCK_WEBSOCKET === 'true';
  private status: string = 'Disconnected';

  public connect(url: string) {
    if (this.mock) {
      this.status = 'Connecting';
      setTimeout(() => {
        this.status = 'Connected';
        this.sendMockData();
      }, 1000);
      return;
    }

    if (this.socket) {
      return;
    }

    this.socket = new WebSocket(url);

    this.socket.onopen = () => {
      // Connection established
    };

    this.socket.onclose = () => {
      this.socket = null;
      this.status = 'Disconnected';
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

  private sendMockData() {
    // Mock registration status (registered for dev)
    const mockRegistration = {
      type: 'registrationStatus',
      payload: {
        registered: true,
        hardwareID: 'mock-dev-hardware-id',
        credits: 50000
      },
    };
    this.listeners.forEach((listener) => listener(mockRegistration));

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

  public sendMessage(message: object) {
    if (this.mock) {
      // Mock mode - no actual message sent
      return;
    }

    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(message));
    } else {
      console.error('WebSocket is not connected. Cannot send message:', message);
    }
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

  public refreshSingleCommander(equipmentMode: 'Commander' | 'Castellan', targetIndex: number) {
    this.sendMessage({
      type: 'refreshSingleCommander',
      payload: { equipmentMode, targetIndex }
    });
  }

  public sendReconfigureLoadout(payload: {
    hardwareID: string;
    equipmentMode: 'Commander' | 'Castellan';
    combatMode: 'PvP' | 'PvE';
    interTierMultiplier: number;
    intraTierMultiplier: number;
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

  public sendConfirmReconfigure(targetIndex: number, currentLoadout: any, newLoadout: any) {
    this.sendMessage({
      type: 'confirmReconfigure',
      payload: {
        targetIndex,
        currentLoadout,
        newLoadout
      }
    });
  }
}

export const FrontendWebsocket = new FrontendWebsocketService();
