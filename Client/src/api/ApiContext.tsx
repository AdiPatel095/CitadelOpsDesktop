import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { CitadelAPI } from './CitadelClient';
import type {
  APIConnectionStatus,
  CatalogManifest,
  CatalogResponse,
  GameStateV2,
  IntentReceipt,
  SubmitIntentOptions,
} from './Contracts';

interface APIContextValue {
  connectionStatus: APIConnectionStatus;
  state: GameStateV2 | null;
  catalogs: CatalogManifest | null;
  operations: Record<string, IntentReceipt>;
  error: string | null;
  refreshState: () => Promise<void>;
  refreshCatalogs: () => Promise<void>;
  getCatalog: <T extends Record<string, unknown>>(name: string) => Promise<CatalogResponse<T>>;
  localize: (keys: string[]) => Promise<Record<string, string>>;
  submitIntent: (
    name: string,
    argumentsValue?: Record<string, unknown>,
    options?: SubmitIntentOptions,
  ) => Promise<IntentReceipt>;
}

const APIContext = createContext<APIContextValue | undefined>(undefined);

export function APIProvider({ children }: { children: ReactNode }) {
  const [connectionStatus, setConnectionStatus] = useState<APIConnectionStatus>('Disconnected');
  const [state, setState] = useState<GameStateV2 | null>(null);
  const [catalogs, setCatalogs] = useState<CatalogManifest | null>(null);
  const [operations, setOperations] = useState<Record<string, IntentReceipt>>({});
  const [error, setError] = useState<string | null>(null);
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const refreshState = useCallback(async () => {
    try {
      setState(await CitadelAPI.getState());
      setError(null);
    } catch (requestError) {
      setError(errorMessage(requestError));
    }
  }, []);

  const refreshCatalogs = useCallback(async () => {
    try {
      setCatalogs(await CitadelAPI.getCatalogManifest());
      setError(null);
    } catch (requestError) {
      setError(errorMessage(requestError));
    }
  }, []);

  useEffect(() => {
    const unsubscribeStatus = CitadelAPI.subscribeStatus(setConnectionStatus);
    const unsubscribeEvents = CitadelAPI.subscribe((message) => {
      if (message.type === 'state.snapshot' && isGameState(message.payload)) {
        setState(message.payload);
        return;
      }
      if (message.type === 'state.changed') {
        if (refreshTimer.current != null) clearTimeout(refreshTimer.current);
        refreshTimer.current = setTimeout(() => void refreshState(), 25);
        return;
      }
      if (message.type === 'catalog.changed' && isCatalogManifest(message.payload)) {
        setCatalogs(message.payload);
        return;
      }
      if ((message.type === 'operation.changed' || message.type === 'intent.receipt') && isIntentReceipt(message.payload)) {
        setOperations((current) => ({ ...current, [message.payload.id]: message.payload }));
      }
    });
    CitadelAPI.connect();
    void Promise.all([refreshState(), refreshCatalogs()]);
    return () => {
      unsubscribeEvents();
      unsubscribeStatus();
      if (refreshTimer.current != null) clearTimeout(refreshTimer.current);
      CitadelAPI.disconnect();
    };
  }, [refreshCatalogs, refreshState]);

  const submitIntent = useCallback(async (
    name: string,
    argumentsValue: Record<string, unknown> = {},
    options: SubmitIntentOptions = {},
  ) => {
    const receipt = await CitadelAPI.submitIntent(name, argumentsValue, options);
    setOperations((current) => ({ ...current, [receipt.id]: receipt }));
    return receipt;
  }, []);

  const value = useMemo<APIContextValue>(() => ({
    connectionStatus,
    state,
    catalogs,
    operations,
    error,
    refreshState,
    refreshCatalogs,
    getCatalog: (name) => CitadelAPI.getCatalog(name),
    localize: (keys) => CitadelAPI.localize(keys),
    submitIntent,
  }), [catalogs, connectionStatus, error, operations, refreshCatalogs, refreshState, state, submitIntent]);

  return <APIContext.Provider value={value}>{children}</APIContext.Provider>;
}

export function useCitadelAPI(): APIContextValue {
  const context = useContext(APIContext);
  if (!context) throw new Error('useCitadelAPI must be used within APIProvider');
  return context;
}

function isGameState(value: unknown): value is GameStateV2 {
  return isRecord(value) && typeof value.revision === 'number' && isRecord(value.session);
}

function isCatalogManifest(value: unknown): value is CatalogManifest {
  return isRecord(value) && isRecord(value.metadata) && Array.isArray(value.catalogs);
}

function isIntentReceipt(value: unknown): value is IntentReceipt {
  return isRecord(value) && typeof value.id === 'string' && typeof value.status === 'string';
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function errorMessage(value: unknown): string {
  return value instanceof Error ? value.message : 'Could not reach the CitadelOps API';
}
