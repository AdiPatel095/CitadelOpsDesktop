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
import { APIError, CitadelAPI } from './CitadelClient';
import type {
  APIConnectionStatus,
	AllianceTargetViewV2,
	AllianceTargetQueryV2,
	AllianceTargetAttackPreviewRequest,
	AllianceTargetAttackPreviewV2,
	ApplicationUpdateV2,
	BuildingTargetCaptureRequest,
	BuildingTargetCaptureResponse,
  CatalogManifest,
  CatalogResponse,
  ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
  GameStateV2,
	GameStatePatchV2,
	IntentReceipt,
	PlayerHistoryRetentionApplyV1,
	RuntimeDiagnosticsV2,
	StateChangeEventV2,
  SubmitIntentOptions,
} from './Contracts';
import { Notifications } from '../components/Notifications';
import { hasExternalConfiguration } from './RuntimeURL';

const runtimeDiagnosticsEnabled = import.meta.env.DEV === true || import.meta.env.VITE_SHOW_HEADER_MEMORY === 'true';
const stateRefreshIntervalMs = 1_000;

function isAvailabilityGateFailure(value: string): boolean {
	const lower = value.trim().toLowerCase();
	if (lower.includes('not enough troops') || lower.includes('insufficient troops')) return true;
	return lower.includes(' of item ')
		&& (lower.includes(' commander(s) require ') || lower.includes(' attack formation requires '));
}

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
	previewAllianceTargetAttack: (input: AllianceTargetAttackPreviewRequest) => Promise<AllianceTargetAttackPreviewV2>;
	optimizeEquipment: (input: EquipmentOptimizeRequest) => Promise<EquipmentOptimizeResponse>;
	captureBuildingTarget: (input: BuildingTargetCaptureRequest) => Promise<BuildingTargetCaptureResponse>;
  submitIntent: (
    name: string,
    argumentsValue?: Record<string, unknown>,
    options?: SubmitIntentOptions,
  ) => Promise<IntentReceipt>;
  cancelOperation: (id: string) => Promise<void>;
  updateConfiguration: (
    section: string,
    value: unknown,
    options?: { expectedValue?: unknown },
  ) => Promise<ConfigurationSnapshot>;
	applyPlayerHistoryRetention: (retention: string, expectedRevision: number) => Promise<PlayerHistoryRetentionApplyV1>;
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
	const initialSyncTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const stateRef = useRef<GameStateV2 | null>(null);
	const configurationRef = useRef<ConfigurationSnapshot | null>(null);
	const configurationUpdateQueue = useRef<Promise<void>>(Promise.resolve());
	const stateReady = useRef(false);
	const catalogsReady = useRef(false);
	const configurationReady = useRef(false);
	const operationsReady = useRef(false);
  const stateRefreshInFlight = useRef<Promise<void> | null>(null);
  const stateRefreshPending = useRef(false);

	const acceptStateSnapshot = useCallback((snapshot: GameStateV2) => {
		const current = stateRef.current;
		if (current != null && current.revision > snapshot.revision) return;
		stateRef.current = snapshot;
		stateReady.current = true;
		setState(snapshot);
	}, []);

	const acceptConfigurationSnapshot = useCallback((snapshot: ConfigurationSnapshot) => {
		const current = configurationRef.current;
		if (current != null && current.revision > snapshot.revision) return;
		configurationRef.current = snapshot;
		configurationReady.current = true;
		setConfiguration(snapshot);
	}, []);

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
		  acceptStateSnapshot(await CitadelAPI.getState());
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
  }, [acceptStateSnapshot]);

  const refreshCatalogs = useCallback(async () => {
    try {
	  setCatalogs(await CitadelAPI.getCatalogManifest());
	  catalogsReady.current = true;
      setError(null);
    } catch (requestError) {
      setError(errorMessage(requestError));
    }
  }, []);

  const refreshConfiguration = useCallback(async () => {
    try {
	  acceptConfigurationSnapshot(await CitadelAPI.getConfiguration());
      setError(null);
    } catch (requestError) {
      setError(errorMessage(requestError));
    }
  }, [acceptConfigurationSnapshot]);

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
			operationsReady.current = true;
		} catch (requestError) {
			console.warn('Could not resynchronize operation stream', requestError);
		}
	}, []);

  useEffect(() => {
    const unsubscribeStatus = CitadelAPI.subscribeStatus(setConnectionStatus);
	const unsubscribeConfiguration = CitadelAPI.subscribeConfiguration(acceptConfigurationSnapshot);
    const unsubscribeEvents = CitadelAPI.subscribe((message) => {
      if (message.type === 'state.snapshot' && isGameState(message.payload)) {
		acceptStateSnapshot(message.payload);
        return;
      }
	  if (message.type === 'state.changed' && isStateChangeEvent(message.payload)) {
		const current = stateRef.current;
		const patch = message.payload.patch;
		if (current != null && patch.revision <= current.revision) return;
		if (message.gap || current == null || patch.schemaVersion !== current.schemaVersion
			|| patch.revision !== current.revision + 1) {
			void refreshState();
			return;
		}
		const next = applyGameStatePatch(current, patch);
		stateRef.current = next;
		stateReady.current = true;
		setState(next);
        return;
      }
	  if (message.type === 'state.changed') {
		void refreshState();
		return;
	  }
      if (message.type === 'catalog.changed' && isCatalogManifest(message.payload)) {
        setCatalogs(message.payload);
		catalogsReady.current = true;
        return;
      }
      if (message.type === 'config.changed' && isConfigurationSnapshot(message.payload)) {
		if (hasExternalConfiguration()) return;
		acceptConfigurationSnapshot(message.payload);
		if (message.gap) void refreshConfiguration();
        return;
      }
	  if (message.type === 'operations.snapshot' && isIntentReceiptArray(message.payload)) {
		const receipts = message.payload;
		setOperations((current) => ({
			...current,
			...Object.fromEntries(receipts.map((receipt) => [receipt.id, receipt])),
		}));
		operationsReady.current = true;
		return;
	  }
      if ((message.type === 'operation.changed' || message.type === 'intent.receipt') && isIntentReceipt(message.payload)) {
		const receipt = message.payload;
		setOperations((current) => ({ ...current, [receipt.id]: receipt }));
		if (message.gap) void refreshOperations();
		if (receipt.status === 'failed' && receipt.error) {
			if (isAvailabilityGateFailure(receipt.error)) {
				Notifications.warning(receipt.error, `operation-${receipt.id}`);
			} else {
				Notifications.error(receipt.error, `operation-${receipt.id}`);
			}
		}
		return;
	  }
	  if (message.type === 'notification' && isNotification(message.payload)) {
		Notifications.publish(message.payload);
      }
    });
    CitadelAPI.connect();
	initialSyncTimer.current = setTimeout(() => {
		if (!stateReady.current) void refreshState();
		if (!catalogsReady.current) void refreshCatalogs();
		if (!configurationReady.current) void refreshConfiguration();
		if (!operationsReady.current) void refreshOperations();
	}, 2_500);
	void Promise.all([
		refreshApplicationUpdate(), runtimeDiagnosticsEnabled ? refreshDiagnostics() : Promise.resolve(),
	]);
    return () => {
      unsubscribeEvents();
      unsubscribeStatus();
	  unsubscribeConfiguration();
	  if (initialSyncTimer.current != null) clearTimeout(initialSyncTimer.current);
      CitadelAPI.disconnect();
    };
  }, [acceptConfigurationSnapshot, acceptStateSnapshot, refreshApplicationUpdate, refreshCatalogs, refreshConfiguration, refreshDiagnostics, refreshOperations, refreshState]);

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

	const previewAllianceTargetAttack = useCallback((input: AllianceTargetAttackPreviewRequest) => (
		CitadelAPI.previewAllianceTargetAttack(input)
	), []);

  const cancelOperation = useCallback(async (id: string) => {
	await CitadelAPI.cancelOperation(id);
  }, []);

	const updateConfiguration = useCallback((
		section: string,
		value: unknown,
		options?: { expectedValue?: unknown },
	) => {
	const hasExpectedValue = options != null && Object.prototype.hasOwnProperty.call(options, 'expectedValue');
	if (hasExpectedValue && options?.expectedValue === undefined) {
		return Promise.reject(new Error('A section-scoped configuration update requires a concrete expected value.'));
	}
	const configurationScope = CitadelAPI.configurationScope();
	const execute = async () => {
	  try {
		const snapshot = await CitadelAPI.updateConfiguration(section, value, hasExpectedValue
			? { expectedValue: options?.expectedValue }
			: { expectedRevision: configurationRef.current?.revision }, configurationScope);
		acceptConfigurationSnapshot(snapshot);
		return snapshot;
	  } catch (requestError) {
		if (requestError instanceof APIError && requestError.code === 'configuration_conflict') {
			try {
				acceptConfigurationSnapshot(await CitadelAPI.getConfiguration(configurationScope));
			} catch {
				// Preserve the original conflict; the regular snapshot stream can retry the refresh.
			}
		}
		Notifications.error(errorMessage(requestError));
		throw requestError;
	  }
	};
	const result = configurationUpdateQueue.current.then(execute, execute);
	configurationUpdateQueue.current = result.then(() => undefined, () => undefined);
	return result;
  }, [acceptConfigurationSnapshot]);

	const applyPlayerHistoryRetention = useCallback((retention: string, expectedRevision: number) => {
		const execute = async () => {
			try {
				return await CitadelAPI.applyPlayerHistoryRetention(retention, expectedRevision);
			} catch (requestError) {
				Notifications.error(errorMessage(requestError));
				throw requestError;
			}
		};
		const result = configurationUpdateQueue.current.then(execute, execute);
		configurationUpdateQueue.current = result.then(() => undefined, () => undefined);
		return result;
	}, []);

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
	previewAllianceTargetAttack,
	optimizeEquipment: (input) => CitadelAPI.optimizeEquipment(input),
	captureBuildingTarget: (input) => CitadelAPI.captureBuildingTarget(input),
    submitIntent,
    cancelOperation,
    updateConfiguration,
	applyPlayerHistoryRetention,
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
	previewAllianceTargetAttack,
	cancelOperation,
    updateConfiguration,
	applyPlayerHistoryRetention,
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

function isStateChangeEvent(value: unknown): value is StateChangeEventV2 {
	if (!isRecord(value) || !Array.isArray(value.components) || !isRecord(value.patch)) return false;
	return typeof value.revision === 'number'
		&& typeof value.patch.schemaVersion === 'number'
		&& typeof value.patch.revision === 'number'
		&& typeof value.patch.updatedAt === 'string'
		&& value.revision === value.patch.revision;
}

function applyGameStatePatch(current: GameStateV2, patch: GameStatePatchV2): GameStateV2 {
	const { mapChanges, castleChanges, movementChanges, inventoryChanges, eventScoreChanges, ...statePatch } = patch;
	const next: GameStateV2 = { ...current, ...statePatch };
	if (castleChanges != null && castleChanges.length > 0) {
		const castles = { ...next.castles };
		for (const change of castleChanges) {
			const id = String(change.id);
			if (change.deleted) delete castles[id];
			else if (change.castle != null) castles[id] = change.castle;
			else if (change.patch != null && castles[id] != null) castles[id] = { ...castles[id], ...change.patch };
		}
		next.castles = castles;
	}
	if (movementChanges != null && movementChanges.length > 0) {
		const movements = { ...next.movements };
		for (const change of movementChanges) {
			const id = String(change.id);
			if (change.deleted || change.movement == null) delete movements[id];
			else movements[id] = change.movement;
		}
		next.movements = movements;
	}
	if (inventoryChanges != null) {
		const { equipmentChanges, gemChanges, itemChanges, ...inventoryPatch } = inventoryChanges;
		const inventory = { ...next.inventory, ...inventoryPatch };
		if (equipmentChanges != null && equipmentChanges.length > 0) {
			const equipment = { ...inventory.equipment };
			for (const change of equipmentChanges) {
				const id = String(change.id);
				if (change.deleted || change.equipment == null) delete equipment[id];
				else equipment[id] = change.equipment;
			}
			inventory.equipment = equipment;
		}
		if (gemChanges != null && gemChanges.length > 0) {
			const gems = { ...inventory.gems };
			for (const change of gemChanges) {
				const id = String(change.id);
				if (change.deleted || change.gem == null) delete gems[id];
				else gems[id] = change.gem;
			}
			inventory.gems = gems;
		}
		if (itemChanges != null && itemChanges.length > 0) {
			const items = { ...inventory.items };
			for (const change of itemChanges) {
				if (change.deleted || change.items == null) delete items[change.collection];
				else items[change.collection] = change.items;
			}
			inventory.items = items;
		}
		next.inventory = inventory;
	}
	if (mapChanges != null && mapChanges.length > 0) {
		const map = { ...next.map };
		const changedKingdoms = new Map<string, Record<string, typeof mapChanges[number]['observation']>>();
		for (const change of mapChanges) {
			const kingdomId = String(change.kingdomId);
			let kingdom = changedKingdoms.get(kingdomId);
			if (kingdom == null) {
				kingdom = { ...(map[kingdomId] ?? {}) };
				changedKingdoms.set(kingdomId, kingdom);
				map[kingdomId] = kingdom as Record<string, NonNullable<typeof change.observation>>;
			}
			if (change.deleted || change.observation == null) {
				delete kingdom[change.key];
			} else {
				kingdom[change.key] = change.observation;
			}
		}
		next.map = map;
	}
	if (eventScoreChanges != null) {
		const eventScores = { ...next.eventScores };
		if (eventScoreChanges.activeEventId != null) eventScores.activeEventId = eventScoreChanges.activeEventId;
		if (eventScoreChanges.inventory != null) eventScores.inventory = eventScoreChanges.inventory;
		if (eventScoreChanges.changes != null && eventScoreChanges.changes.length > 0) {
			const byEvent = { ...eventScores.byEvent };
			const activityByEvent = { ...eventScores.activityByEvent };
			const rankingByEvent = { ...eventScores.rankingByEvent };
			for (const change of eventScoreChanges.changes) {
				const id = String(change.eventId);
				if (change.scoreDeleted || change.score == null) delete byEvent[id];
				else byEvent[id] = change.score;
				if (change.activityDeleted || change.activity == null) delete activityByEvent[id];
				else activityByEvent[id] = change.activity;
				if (change.rankingDeleted || change.ranking == null) delete rankingByEvent[id];
				else rankingByEvent[id] = change.ranking;
			}
			eventScores.byEvent = byEvent;
			eventScores.activityByEvent = activityByEvent;
			eventScores.rankingByEvent = rankingByEvent;
		}
		next.eventScores = eventScores;
	}
	return next;
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
