type MessageListener = (message: any) => void;

class FrontendWebsocketService {
  private socket: WebSocket | null = null;
  private listeners: MessageListener[] = [];
  private mock: boolean = import.meta.env.VITE_MOCK_WEBSOCKET === 'true';
  private status: string = 'Disconnected';

  public connect(url: string) {
    if (this.mock) {
      console.log('Running in mock websocket mode. No real connection will be established.');
      this.status = 'Connecting';
      setTimeout(() => {
        this.status = 'Connected';
        console.log('Mock WebSocket connected');
        this.sendMockData();
      }, 1000);
      return;
    }

    if (this.socket) {
      return;
    }

    this.socket = new WebSocket(url);

    this.socket.onopen = () => {
      console.log('WebSocket connected');
    };

    this.socket.onclose = () => {
      console.log('WebSocket disconnected');
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
    // Mock login success
    const loginSuccess = {
        type: 'LOGIN_STATUS',
        payload: { status: 'success' },
    };
    this.listeners.forEach((listener) => listener(loginSuccess));

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
  }

  public addMessageListener(listener: MessageListener) {
    this.listeners.push(listener);
  }

  public removeMessageListener(listener: MessageListener) {
    this.listeners = this.listeners.filter((l) => l !== listener);
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
}

export const FrontendWebsocket = new FrontendWebsocketService();
