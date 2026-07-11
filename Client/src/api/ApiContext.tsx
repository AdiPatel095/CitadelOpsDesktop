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
  ConfigurationSnapshot,
  GameStateV2,
  IntentReceipt,
  SubmitIntentOptions,
} from './Contracts';
import { Notifications } from '../components/Notifications';

interface APIContextValue {
  connectionStatus: APIConnectionStatus;
  state: GameStateV2 | null;
  catalogs: CatalogManifest | null;
  configuration: ConfigurationSnapshot | null;
  operations: Record<string, IntentReceipt>;
  error: string | null;
  refreshState: () => Promise<void>;
  refreshCatalogs: () => Promise<void>;
  refreshConfiguration: () => Promise<void>;
  getCatalog: <T extends Record<string, unknown>>(name: string) => Promise<CatalogResponse<T>>;
  localize: (keys: string[]) => Promise<Record<string, string>>;
  submitIntent: (
    name: string,
    argumentsValue?: Record<string, unknown>,
    options?: SubmitIntentOptions,
  ) => Promise<IntentReceipt>;
  updateConfiguration: (section: string, value: unknown) => Promise<IntentReceipt>;
}

const APIContext = createContext<APIContextValue | undefined>(undefined);

export function APIProvider({ children }: { children: ReactNode }) {
  const [connectionStatus, setConnectionStatus] = useState<APIConnectionStatus>('Disconnected');
  const [state, setState] = useState<GameStateV2 | null>(null);
  const [catalogs, setCatalogs] = useState<CatalogManifest | null>(null);
  const [configuration, setConfiguration] = useState<ConfigurationSnapshot | null>(null);
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

  const refreshConfiguration = useCallback(async () => {
    try {
      setConfiguration(await CitadelAPI.getConfiguration());
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
      if (message.type === 'config.changed' && isConfigurationSnapshot(message.payload)) {
        setConfiguration(message.payload);
        return;
      }
      if ((message.type === 'operation.changed' || message.type === 'intent.receipt') && isIntentReceipt(message.payload)) {
        setOperations((current) => ({ ...current, [message.payload.id]: message.payload }));
		if (message.payload.status === 'failed' && message.payload.error) {
			Notifications.error(message.payload.error, `operation-${message.payload.id}`);
		}
		return;
	  }
	  if (message.type === 'notification' && isNotification(message.payload)) {
		Notifications.publish(message.payload);
      }
    });
    CitadelAPI.connect();
    void Promise.all([refreshState(), refreshCatalogs(), refreshConfiguration()]);
    return () => {
      unsubscribeEvents();
      unsubscribeStatus();
      if (refreshTimer.current != null) clearTimeout(refreshTimer.current);
      CitadelAPI.disconnect();
    };
  }, [refreshCatalogs, refreshConfiguration, refreshState]);

  const submitIntent = useCallback(async (
    name: string,
    argumentsValue: Record<string, unknown> = {},
    options: SubmitIntentOptions = {},
  ) => {
	try {
	  const receipt = await CitadelAPI.submitIntent(name, argumentsValue, options);
	  setOperations((current) => ({ ...current, [receipt.id]: receipt }));
	  return receipt;
	} catch (requestError) {
	  Notifications.error(errorMessage(requestError));
	  throw requestError;
	}
  }, []);

  const updateConfiguration = useCallback((section: string, value: unknown) => (
    submitIntent('config.update', {
      section,
      value,
      expectedRevision: configuration?.revision,
    })
  ), [configuration?.revision, submitIntent]);

  const value = useMemo<APIContextValue>(() => ({
    connectionStatus,
    state,
    catalogs,
    configuration,
    operations,
    error,
    refreshState,
    refreshCatalogs,
    refreshConfiguration,
    getCatalog: (name) => CitadelAPI.getCatalog(name),
    localize: (keys) => CitadelAPI.localize(keys),
    submitIntent,
    updateConfiguration,
  }), [
    catalogs,
    configuration,
    connectionStatus,
    error,
    operations,
    refreshCatalogs,
    refreshConfiguration,
    refreshState,
    state,
    submitIntent,
    updateConfiguration,
  ]);

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

function isConfigurationSnapshot(value: unknown): value is ConfigurationSnapshot {
  return isRecord(value) && typeof value.revision === 'number' && isRecord(value.sections);
}

function isIntentReceipt(value: unknown): value is IntentReceipt {
  return isRecord(value) && typeof value.id === 'string' && typeof value.status === 'string';
}

function isNotification(value: unknown): value is {
	category: 'green' | 'yellow' | 'red';
	message: string;
	id?: string;
	lines?: string[];
	persistent?: boolean;
} {
	return isRecord(value)
		&& (value.category === 'green' || value.category === 'yellow' || value.category === 'red')
		&& typeof value.message === 'string';
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function errorMessage(value: unknown): string {
  return value instanceof Error ? value.message : 'Could not reach the CitadelOps API';
}
