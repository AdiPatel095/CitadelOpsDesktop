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
	AllianceTargetViewV2,
	AllianceTargetQueryV2,
	ApplicationUpdateV2,
	BuildingTargetCaptureRequest,
	BuildingTargetCaptureResponse,
  CatalogManifest,
  CatalogResponse,
  ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
  GameStateV2,
  IntentReceipt,
	RuntimeDiagnosticsV2,
  SubmitIntentOptions,
} from './Contracts';
import { Notifications } from '../components/Notifications';

const runtimeDiagnosticsEnabled = import.meta.env.DEV === true || import.meta.env.VITE_SHOW_HEADER_MEMORY === 'true';
const stateRefreshIntervalMs = 1_000;

interface APIContextValue {
  connectionStatus: APIConnectionStatus;
  state: GameStateV2 | null;
  catalogs: CatalogManifest | null;
  configuration: ConfigurationSnapshot | null;
	applicationUpdate: ApplicationUpdateV2 | null;
	diagnostics: RuntimeDiagnosticsV2 | null;
  operations: Record<string, IntentReceipt>;
  error: string | null;
  refreshState: () => Promise<void>;
  refreshCatalogs: () => Promise<void>;
  refreshConfiguration: () => Promise<void>;
	refreshApplicationUpdate: () => Promise<void>;
	refreshDiagnostics: () => Promise<void>;
  getCatalog: <T extends Record<string, unknown>>(name: string) => Promise<CatalogResponse<T>>;
  localize: (keys: string[]) => Promise<Record<string, string>>;
	getAllianceTargets: (input?: AllianceTargetQueryV2) => Promise<AllianceTargetViewV2>;
	optimizeEquipment: (input: EquipmentOptimizeRequest) => Promise<EquipmentOptimizeResponse>;
	captureBuildingTarget: (input: BuildingTargetCaptureRequest) => Promise<BuildingTargetCaptureResponse>;
  submitIntent: (
    name: string,
    argumentsValue?: Record<string, unknown>,
    options?: SubmitIntentOptions,
  ) => Promise<IntentReceipt>;
  cancelOperation: (id: string) => Promise<void>;
  updateConfiguration: (section: string, value: unknown) => Promise<IntentReceipt>;
}

const APIContext = createContext<APIContextValue | undefined>(undefined);

export function APIProvider({ children }: { children: ReactNode }) {
  const [connectionStatus, setConnectionStatus] = useState<APIConnectionStatus>('Disconnected');
  const [state, setState] = useState<GameStateV2 | null>(null);
  const [catalogs, setCatalogs] = useState<CatalogManifest | null>(null);
  const [configuration, setConfiguration] = useState<ConfigurationSnapshot | null>(null);
	const [applicationUpdate, setApplicationUpdate] = useState<ApplicationUpdateV2 | null>(null);
	const [diagnostics, setDiagnostics] = useState<RuntimeDiagnosticsV2 | null>(null);
  const [operations, setOperations] = useState<Record<string, IntentReceipt>>({});
  const [error, setError] = useState<string | null>(null);
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stateRefreshInFlight = useRef<Promise<void> | null>(null);
  const stateRefreshPending = useRef(false);

  const refreshState = useCallback(async function refreshStateRequest() {
    if (stateRefreshInFlight.current != null) {
      stateRefreshPending.current = true;
      await stateRefreshInFlight.current;
      return;
    }
    const request = (async () => {
      do {
        stateRefreshPending.current = false;
        const startedAt = Date.now();
        try {
          setState(await CitadelAPI.getState());
          setError(null);
        } catch (requestError) {
          setError(errorMessage(requestError));
        }
        if (stateRefreshPending.current) {
          const remaining = stateRefreshIntervalMs - (Date.now() - startedAt);
          if (remaining > 0) await new Promise((resolve) => setTimeout(resolve, remaining));
        }
      } while (stateRefreshPending.current);
    })();
    stateRefreshInFlight.current = request;
    try {
      await request;
    } finally {
      if (stateRefreshInFlight.current === request) stateRefreshInFlight.current = null;
    }
    if (stateRefreshPending.current) {
      await new Promise((resolve) => setTimeout(resolve, stateRefreshIntervalMs));
      await refreshStateRequest();
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

	const refreshApplicationUpdate = useCallback(async () => {
		try {
			setApplicationUpdate(await CitadelAPI.getApplicationUpdate());
		} catch (requestError) {
			console.warn('Could not refresh application update state', requestError);
		}
	}, []);

	const refreshDiagnostics = useCallback(async () => {
		try {
			setDiagnostics(await CitadelAPI.getDiagnostics());
		} catch (requestError) {
			console.warn('Could not refresh runtime diagnostics', requestError);
		}
	}, []);

	const refreshOperations = useCallback(async () => {
		try {
			const receipts = await CitadelAPI.getOperations();
			setOperations((current) => ({
				...current,
				...Object.fromEntries(receipts.map((receipt) => [receipt.id, receipt])),
			}));
		} catch (requestError) {
			console.warn('Could not resynchronize operation stream', requestError);
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
        if (message.gap) {
          if (refreshTimer.current != null) clearTimeout(refreshTimer.current);
          refreshTimer.current = null;
          void refreshState();
        } else if (refreshTimer.current == null) {
          refreshTimer.current = setTimeout(() => {
            refreshTimer.current = null;
            void refreshState();
          }, stateRefreshIntervalMs);
        }
        return;
      }
      if (message.type === 'catalog.changed' && isCatalogManifest(message.payload)) {
        setCatalogs(message.payload);
        return;
      }
      if (message.type === 'config.changed' && isConfigurationSnapshot(message.payload)) {
        setConfiguration(message.payload);
		if (message.gap) void refreshConfiguration();
        return;
      }
	  if (message.type === 'operations.snapshot' && isIntentReceiptArray(message.payload)) {
		const receipts = message.payload;
		setOperations((current) => ({
			...current,
			...Object.fromEntries(receipts.map((receipt) => [receipt.id, receipt])),
		}));
		return;
	  }
      if ((message.type === 'operation.changed' || message.type === 'intent.receipt') && isIntentReceipt(message.payload)) {
		const receipt = message.payload;
		setOperations((current) => ({ ...current, [receipt.id]: receipt }));
		if (message.gap) void refreshOperations();
		if (receipt.status === 'failed' && receipt.error) {
			Notifications.error(receipt.error, `operation-${receipt.id}`);
		}
		return;
	  }
	  if (message.type === 'notification' && isNotification(message.payload)) {
		Notifications.publish(message.payload);
      }
    });
    CitadelAPI.connect();
    void Promise.all([
		refreshState(), refreshCatalogs(), refreshConfiguration(), refreshOperations(), refreshApplicationUpdate(),
		runtimeDiagnosticsEnabled ? refreshDiagnostics() : Promise.resolve(),
	]);
    return () => {
      unsubscribeEvents();
      unsubscribeStatus();
      if (refreshTimer.current != null) clearTimeout(refreshTimer.current);
      CitadelAPI.disconnect();
    };
  }, [refreshApplicationUpdate, refreshCatalogs, refreshConfiguration, refreshDiagnostics, refreshOperations, refreshState]);

	useEffect(() => {
		const interval = window.setInterval(() => void refreshApplicationUpdate(), 5_000);
		return () => window.clearInterval(interval);
	}, [refreshApplicationUpdate]);

	useEffect(() => {
		if (!runtimeDiagnosticsEnabled) return;
		const interval = window.setInterval(() => void refreshDiagnostics(), 5_000);
		return () => window.clearInterval(interval);
	}, [refreshDiagnostics]);

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

  const getAllianceTargets = useCallback((input: AllianceTargetQueryV2 = {}) => (
	CitadelAPI.getAllianceTargets(input)
  ), []);

  const cancelOperation = useCallback(async (id: string) => {
	await CitadelAPI.cancelOperation(id);
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
	applicationUpdate,
	diagnostics,
    operations,
    error,
    refreshState,
    refreshCatalogs,
    refreshConfiguration,
	refreshApplicationUpdate,
	refreshDiagnostics,
    getCatalog: (name) => CitadelAPI.getCatalog(name),
    localize: (keys) => CitadelAPI.localize(keys),
	getAllianceTargets,
	optimizeEquipment: (input) => CitadelAPI.optimizeEquipment(input),
	captureBuildingTarget: (input) => CitadelAPI.captureBuildingTarget(input),
    submitIntent,
    cancelOperation,
    updateConfiguration,
  }), [
    catalogs,
	applicationUpdate,
	diagnostics,
    configuration,
    connectionStatus,
    error,
    operations,
    refreshCatalogs,
	refreshApplicationUpdate,
	refreshDiagnostics,
    refreshConfiguration,
    refreshState,
    state,
    submitIntent,
	getAllianceTargets,
	cancelOperation,
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

function isIntentReceiptArray(value: unknown): value is IntentReceipt[] {
  return Array.isArray(value) && value.every(isIntentReceipt);
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
