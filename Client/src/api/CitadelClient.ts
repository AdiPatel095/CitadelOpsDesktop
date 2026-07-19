import { API_CONFIG } from '../config/Api';
import type {
  APIConnectionStatus,
  APIEnvelope,
	AllianceTargetViewV2,
	ApplicationUpdateV2,
	BuildingCatalogQuery,
	BuildingCatalogResponse,
	BuildingPreviewRequest,
	BuildingPreviewResponse,
	BuildingTargetCaptureRequest,
	BuildingTargetCaptureResponse,
	ExpansionPreviewRequest,
	ExpansionPreviewResponse,
  BrowserInventory,
  CatalogManifest,
  CatalogResponse,
  ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
  GameStateV2,
  IntentReceipt,
	IntentDefinition,
	RuntimeDiagnosticsV2,
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
  private pendingIntents = new Map<string, Promise<IntentReceipt>>();

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

  getApplicationUpdate(): Promise<ApplicationUpdateV2> {
	return this.request<ApplicationUpdateV2>('/api/v2/update');
  }

	getDiagnostics(): Promise<RuntimeDiagnosticsV2> {
		return this.request<RuntimeDiagnosticsV2>('/api/v2/diagnostics');
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

	getBuildingCatalog(input: BuildingCatalogQuery = {}): Promise<BuildingCatalogResponse> {
		const query = new URLSearchParams();
		for (const id of input.ids ?? []) query.append('id', String(id));
		if (input.kingdomId != null) query.set('kingdomId', String(input.kingdomId));
		if (input.eventId != null) query.set('eventId', String(input.eventId));
		if (input.areaTypeId != null) query.set('areaTypeId', String(input.areaTypeId));
		if (input.mapId != null) query.set('mapId', String(input.mapId));
		if (input.maxLevel != null) query.set('maxLevel', String(input.maxLevel));
		if (input.rootOnly != null) query.set('rootOnly', input.rootOnly ? '1' : '0');
		if (input.query) query.set('q', input.query);
		if (input.offset != null) query.set('offset', String(input.offset));
		if (input.limit != null) query.set('limit', String(input.limit));
		const suffix = query.size > 0 ? `?${query.toString()}` : '';
		return this.request<BuildingCatalogResponse>(`/api/v2/buildings/catalog${suffix}`);
	}

	previewBuildings(input: BuildingPreviewRequest): Promise<BuildingPreviewResponse> {
		return this.request<BuildingPreviewResponse>('/api/v2/buildings/preview', {
			method: 'POST',
			body: JSON.stringify(input),
		});
	}

	previewExpansion(input: ExpansionPreviewRequest): Promise<ExpansionPreviewResponse> {
		return this.request<ExpansionPreviewResponse>('/api/v2/buildings/expansion/preview', {
			method: 'POST',
			body: JSON.stringify(input),
		});
	}

	captureBuildingTarget(input: BuildingTargetCaptureRequest): Promise<BuildingTargetCaptureResponse> {
		return this.request<BuildingTargetCaptureResponse>('/api/v2/buildings/target/capture', {
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
	const operationId = options.id?.trim() || this.newOperationId();
	const requestKey = `${name}\u0000${JSON.stringify(argumentsValue)}\u0000${options.expectedRevision ?? ''}\u0000${options.dryRun ?? false}`;
	const duplicateKey = options.id?.trim() ? `id:${operationId}\u0000${requestKey}` : `intent:${requestKey}`;
	const pending = this.pendingIntents.get(duplicateKey);
	if (pending) return pending;
	const request = this.request<IntentReceipt>(`/api/v2/intents/${encodeURIComponent(name)}`, {
      method: 'POST',
      body: JSON.stringify({
        id: operationId,
        actor: options.actor ?? 'ui',
        priority: options.priority,
        arguments: argumentsValue,
        expectedRevision: options.expectedRevision,
        dryRun: options.dryRun,
      }),
    });
	this.pendingIntents.set(duplicateKey, request);
	void request.finally(() => {
	  if (this.pendingIntents.get(duplicateKey) === request) this.pendingIntents.delete(duplicateKey);
	}).catch(() => undefined);
	return request;
  }

  cancelOperation(id: string): Promise<{ id: string; cancelled: boolean }> {
    return this.request<{ id: string; cancelled: boolean }>(`/api/v2/operations/${encodeURIComponent(id)}/cancel`, {
      method: 'POST',
    });
  }

  getOperations(limit = 100): Promise<IntentReceipt[]> {
    const bounded = Math.max(1, Math.min(1000, Math.trunc(limit)));
    return this.request<IntentReceipt[]>(`/api/v2/operations?limit=${bounded}`);
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

  private newOperationId(): string {
	return globalThis.crypto?.randomUUID?.()
	  ?? `ui-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
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
