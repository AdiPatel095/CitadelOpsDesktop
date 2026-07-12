import { API_CONFIG } from '../config/Api';
import type {
  APIConnectionStatus,
  APIEnvelope,
	AllianceTargetViewV2,
  BrowserInventory,
  CatalogManifest,
  CatalogResponse,
  ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
  GameStateV2,
  IntentReceipt,
	IntentDefinition,
  SubmitIntentOptions,
} from './Contracts';

type EnvelopeListener = (message: APIEnvelope) => void;
type StatusListener = (status: APIConnectionStatus) => void;

export class APIError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.code = code;
  }
}

class CitadelClient {
  private socket: WebSocket | null = null;
  private status: APIConnectionStatus = 'Disconnected';
  private listeners = new Set<EnvelopeListener>();
  private statusListeners = new Set<StatusListener>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private intentionalClose = false;

  connect() {
    this.intentionalClose = false;
    if (this.socket?.readyState === WebSocket.OPEN || this.socket?.readyState === WebSocket.CONNECTING) {
      return;
    }
    this.setStatus('Connecting');
    const socket = new WebSocket(this.eventsURL());
    this.socket = socket;
    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.reconnectAttempt = 0;
      this.setStatus('Connected');
    };
    socket.onmessage = (event) => {
      if (this.socket !== socket) return;
      try {
        const message = JSON.parse(String(event.data)) as APIEnvelope;
        if (message.v !== 2 || typeof message.type !== 'string') return;
        for (const listener of this.listeners) listener(message);
      } catch (error) {
        console.error('Could not decode CitadelOps API event', error);
      }
    };
    socket.onerror = () => {
      if (this.socket === socket) this.setStatus('Disconnected');
    };
    socket.onclose = () => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.setStatus('Disconnected');
      this.scheduleReconnect();
    };
  }

  disconnect() {
    this.intentionalClose = true;
    if (this.reconnectTimer != null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.socket?.close();
    this.socket = null;
    this.setStatus('Disconnected');
  }

  subscribe(listener: EnvelopeListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  subscribeStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => this.statusListeners.delete(listener);
  }

  getState(): Promise<GameStateV2> {
    return this.request<GameStateV2>('/api/v2/state');
  }

  getBrowsers(): Promise<BrowserInventory> {
    return this.request<BrowserInventory>('/api/v2/browsers');
  }

  getConfiguration(): Promise<ConfigurationSnapshot> {
    return this.request<ConfigurationSnapshot>('/api/v2/config');
  }

  selectBrowser(browser: string, options: SubmitIntentOptions = {}): Promise<IntentReceipt> {
    return this.submitIntent('session.select_browser', { browser }, options);
  }

  getCatalogManifest(): Promise<CatalogManifest> {
    return this.request<CatalogManifest>('/api/v2/game-data');
  }

  getCatalog<T extends Record<string, unknown>>(name: string): Promise<CatalogResponse<T>> {
    return this.request<CatalogResponse<T>>(`/api/v2/game-data/${encodeURIComponent(name)}`);
  }

  getProjection<T>(name: string): Promise<T> {
    return this.request<T>(`/api/v2/projections/${encodeURIComponent(name)}`);
  }

	getAllianceTargets(allianceId = '', server = '', refresh = false): Promise<AllianceTargetViewV2> {
		const query = new URLSearchParams();
		if (allianceId) query.set('allianceId', allianceId);
		if (server) query.set('server', server);
		if (refresh) query.set('refresh', '1');
		const suffix = query.size > 0 ? `?${query.toString()}` : '';
		return this.request<AllianceTargetViewV2>(`/api/v2/alliance-targets${suffix}`);
	}

	optimizeEquipment(input: EquipmentOptimizeRequest): Promise<EquipmentOptimizeResponse> {
		return this.request<EquipmentOptimizeResponse>('/api/v2/equipment/optimize', {
			method: 'POST',
			body: JSON.stringify(input),
		});
	}

  async localize(keys: string[]): Promise<Record<string, string>> {
    const response = await this.request<{ values: Record<string, string> }>('/api/v2/game-data/localize', {
      method: 'POST',
      body: JSON.stringify({ keys }),
    });
    return response.values;
  }

  getIntentDefinitions(): Promise<IntentDefinition[]> {
	return this.request<IntentDefinition[]>('/api/v2/intents');
  }

  submitIntent(
    name: string,
    argumentsValue: Record<string, unknown> = {},
    options: SubmitIntentOptions = {},
  ): Promise<IntentReceipt> {
    return this.request<IntentReceipt>(`/api/v2/intents/${encodeURIComponent(name)}`, {
      method: 'POST',
      body: JSON.stringify({
        actor: options.actor ?? 'ui',
        arguments: argumentsValue,
        expectedRevision: options.expectedRevision,
        dryRun: options.dryRun,
      }),
    });
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(this.httpURL(path), {
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
        ...init.headers,
      },
    });
    const payload = await response.json().catch(() => null) as unknown;
    if (!response.ok) {
      const structuredError = isRecord(payload) && isRecord(payload.error) ? payload.error : null;
      const receiptError = isRecord(payload) && typeof payload.error === 'string' ? payload.error : null;
      const message = structuredError && typeof structuredError.message === 'string'
        ? structuredError.message
        : receiptError ?? `CitadelOps API returned HTTP ${response.status}`;
      throw new APIError(
        message,
        response.status,
        structuredError && typeof structuredError.code === 'string' ? structuredError.code : undefined,
      );
    }
    return payload as T;
  }

  private httpURL(path: string): string {
    const base = API_CONFIG.BASE_URL.replace(/\/$/, '');
    return `${base}${path}`;
  }

  private eventsURL(): string {
    const base = API_CONFIG.BASE_URL || window.location.origin;
    const url = new URL('/api/v2/events', base);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.toString();
  }

  private scheduleReconnect() {
    if (this.intentionalClose || this.reconnectTimer != null) return;
    const delay = Math.min(30_000, 1000 * 2 ** this.reconnectAttempt);
    this.reconnectAttempt += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private setStatus(status: APIConnectionStatus) {
    if (this.status === status) return;
    this.status = status;
    for (const listener of this.statusListeners) listener(status);
  }
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export const CitadelAPI = new CitadelClient();
