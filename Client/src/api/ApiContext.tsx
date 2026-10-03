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
import { APIError, CitadelAPI, OperationError } from './CitadelClient';
import { shouldToastConfigurationError } from './configurationErrorToast';
import { OperationFailureNotificationCoordinator, RubyUpgradeNotificationCoordinator } from './OperationNotifications';
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
	PlayerHistoryRetentionV1,
	RuntimeDiagnosticsV2,
	StateChangeEventV2,
  SubmitIntentOptions,
} from './Contracts';
import { Notifications } from '../components/Notifications';
import { hasExternalConfiguration } from './RuntimeURL';
import { StateResync, type StateResyncOutcome } from './StateResync';

const runtimeDiagnosticsEnabled = import.meta.env.DEV === true || import.meta.env.VITE_SHOW_HEADER_MEMORY === 'true';

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
  /** Loads and accepts the latest configuration snapshot; rejects on failure (draft sessions need the snapshot). */
  loadLatestConfiguration: () => Promise<ConfigurationSnapshot>;
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
    options?: ConfigurationUpdateOptions,
  ) => Promise<ConfigurationSnapshot>;
	getPlayerHistoryRetention: () => Promise<PlayerHistoryRetentionV1>;
	applyPlayerHistoryRetention: (
		retention: string,
		recordingIntervalSeconds: number,
		expectedRevision: number,
		expectedConfigured: string,
		expectedRecordingIntervalSeconds: number,
	) => Promise<PlayerHistoryRetentionApplyV1>;
}

export type ConfigurationUpdateOptions =
	| { expectedValue: unknown; expectedRevision?: never; conflictShownByEditor?: boolean }
	| { expectedRevision: number; expectedValue?: never; conflictShownByEditor?: boolean };

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
	// Bind delayed runtime work to the account where this provider mounted.
	// Account navigation may change the pathname before a queued save begins.
	const runtimeScope = useRef(CitadelAPI.runtimeScope()).current;
	const stateReady = useRef(false);
	const catalogsReady = useRef(false);
	const configurationReady = useRef(false);
	const operationsReady = useRef(false);
	const stateInstance = useRef<string | null>(null);
	const operationSequence = useRef(0);
	const operationNotificationIDs = useRef(new Map<string, string>());
	const rubyUpgradeNotifications = useRef(new RubyUpgradeNotificationCoordinator());
	useEffect(() => {
		for (const notice of rubyUpgradeNotifications.current.next(state?.automations ?? {})) Notifications.publish(notice);
	}, [state?.automations]);
	const operationFailureNotifications = useRef(new OperationFailureNotificationCoordinator());
  const stateRefreshInFlight = useRef<Promise<void> | null>(null);
	// Revision bookkeeping for the state stream: applies in-order and merged (gap) events,
	// buffers events across holes and resyncs, and asks for at most one snapshot at a time.
	const stateResync = useRef(new StateResync<GameStateV2, GameStatePatchV2>(applyGameStatePatch)).current;
	const stateResyncTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const stateResyncSequence = useRef(0);

	const handleStateOutcome = useCallback(function handle(outcome: StateResyncOutcome<GameStateV2>) {
		if (outcome.changed && outcome.state != null) {
			stateRef.current = outcome.state;
			stateReady.current = true;
			setState(outcome.state);
		}
		if (stateResyncTimer.current != null) clearTimeout(stateResyncTimer.current);
		stateResyncTimer.current = null;
		if (outcome.wakeAt != null) {
			stateResyncTimer.current = setTimeout(() => {
				stateResyncTimer.current = null;
				handle(stateResync.tick(Date.now()));
			}, Math.max(0, outcome.wakeAt - Date.now()));
		}
		if (outcome.resync == null) return;
		// Prefer the event socket: the snapshot then arrives in order with later events. REST only
		// when the socket is down, or when the previous socket request went unanswered (the state
		// machine says so through `transport`), however many resyncs happened within the backoff window.
		if (outcome.resync.transport === 'socket' && CitadelAPI.requestState(`state-resync-${++stateResyncSequence.current}`)) return;
		void CitadelAPI.getState().then(
			(snapshot) => handle(stateResync.acceptSnapshot(snapshot, Date.now())),
			(requestError) => {
				setError(errorMessage(requestError));
				handle(stateResync.resyncFailed(Date.now()));
			},
		);
	}, [stateResync]);

	const resetStateStream = useCallback(() => {
		stateResync.connectionReset();
		if (stateResyncTimer.current != null) clearTimeout(stateResyncTimer.current);
		stateResyncTimer.current = null;
	}, [stateResync]);

	const acceptStateSnapshot = useCallback((snapshot: GameStateV2) => {
		handleStateOutcome(stateResync.acceptSnapshot(snapshot, Date.now()));
	}, [handleStateOutcome, stateResync]);

	const acceptConfigurationSnapshot = useCallback((snapshot: ConfigurationSnapshot) => {
		const current = configurationRef.current;
		if (current != null && current.revision > snapshot.revision) return;
		configurationRef.current = snapshot;
		configurationReady.current = true;
		setConfiguration(snapshot);
	}, []);

	const publishOperationFailure = useCallback((receipt: IntentReceipt) => {
		const notification = operationFailureNotifications.current.next(
			receipt,
			operationNotificationIDs.current.get(receipt.id),
		);
		if (notification) Notifications.publish(notification);
	}, []);

  const refreshState = useCallback(async function refreshStateRequest() {
    // Explicit REST refresh (initial fallback, callers that just mutated state). One request at a
    // time; never re-armed by stream events.
    while (stateRefreshInFlight.current != null) await stateRefreshInFlight.current;
    const request = (async () => {
      try {
        acceptStateSnapshot(await CitadelAPI.getState());
        setError(null);
      } catch (requestError) {
        setError(errorMessage(requestError));
      }
    })();
    stateRefreshInFlight.current = request;
    try {
      await request;
    } finally {
      if (stateRefreshInFlight.current === request) stateRefreshInFlight.current = null;
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

	const loadLatestConfiguration = useCallback(async () => {
		try {
			const snapshot = await CitadelAPI.getConfiguration();
			acceptConfigurationSnapshot(snapshot);
			setError(null);
			return configurationRef.current != null && configurationRef.current.revision > snapshot.revision
				? configurationRef.current
				: snapshot;
		} catch (requestError) {
			setError(errorMessage(requestError));
			throw requestError;
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
	const clearResumeCursor = CitadelAPI.setResumeCursorProvider((scope) => {
		const current = stateResync.current();
		if (scope !== runtimeScope || current == null || stateInstance.current == null) return null;
		return {
			instance: stateInstance.current, since: current.revision,
			// If a greeting was interrupted before these arrived, force a full
			// greeting rather than claiming that an absent part is already held.
			ops: operationsReady.current ? operationSequence.current : -1,
			config: configurationRef.current?.revision ?? -1,
			catalog: CitadelAPI.getCatalogDigest(),
		};
	});
    const unsubscribeStatus = CitadelAPI.subscribeStatus((status) => {
		setConnectionStatus(status);
		// A reconnect supplies a snapshot or a merged patch; old socket requests and buffers are void.
		if (status === 'Disconnected') resetStateStream();
	});
	const unsubscribeConfiguration = CitadelAPI.subscribeConfiguration(acceptConfigurationSnapshot);
    const unsubscribeEvents = CitadelAPI.subscribe((message) => {
      if (message.type === 'state.snapshot' && isGameState(message.payload)) {
		const instance = typeof message.instance === 'string' && message.instance ? message.instance : null;
		if (instance !== stateInstance.current) {
			stateResync.forgetState();
			operationSequence.current = 0;
			operationsReady.current = false;
		}
		stateInstance.current = instance;
		acceptStateSnapshot(message.payload);
        return;
      }
	  if (message.type === 'state.resumed') {
		// State revisions advance only when the merged patch is applied. The
		// other held parts survive the skipped greeting snapshots.
		return;
	  }
	  if (message.type === 'state.changed' && isStateChangeEvent(message.payload)) {
		handleStateOutcome(stateResync.receiveEvent({
			patch: message.payload.patch,
			gap: message.gap,
			baseRevision: message.baseRevision,
		}, Date.now()));
        return;
      }
	  if (message.type === 'state.changed') {
		handleStateOutcome(stateResync.unusableEvent(Date.now()));
		return;
	  }
      if (message.type === 'catalog.changed' && isCatalogManifest(message.payload)) {
        CitadelAPI.noteCatalogManifest(message.payload);
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
		operationSequence.current = Math.max(operationSequence.current, message.sequence ?? 0);
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
		if (message.type === 'operation.changed') operationSequence.current = Math.max(operationSequence.current, message.sequence ?? 0);
		setOperations((current) => ({ ...current, [receipt.id]: receipt }));
		if (message.gap) void refreshOperations();
		publishOperationFailure(receipt);
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
	  clearResumeCursor();
      unsubscribeEvents();
      unsubscribeStatus();
	  unsubscribeConfiguration();
	  if (initialSyncTimer.current != null) clearTimeout(initialSyncTimer.current);
	  resetStateStream();
      CitadelAPI.disconnect();
    };
  }, [acceptConfigurationSnapshot, acceptStateSnapshot, handleStateOutcome, publishOperationFailure, refreshApplicationUpdate, refreshCatalogs, refreshConfiguration, refreshDiagnostics, refreshOperations, refreshState, resetStateStream, runtimeScope, stateResync]);

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
	const preferredNotificationID = options.notificationId?.trim();
	const operationID = options.id?.trim() || (preferredNotificationID ? newClientOperationID() : '');
	const submitOptions = operationID ? { ...options, id: operationID } : options;
	if (operationID && preferredNotificationID) {
		operationNotificationIDs.current.set(operationID, preferredNotificationID);
	}
	try {
	  const receipt = await CitadelAPI.submitIntent(name, argumentsValue, submitOptions);
	  setOperations((current) => ({ ...current, [receipt.id]: receipt }));
	  return receipt;
	} catch (requestError) {
	  if (requestError instanceof OperationError) {
		publishOperationFailure(requestError.receipt);
	  } else {
		Notifications.error(errorMessage(requestError), preferredNotificationID);
	  }
	  throw requestError;
	} finally {
	  if (operationID && preferredNotificationID) {
		operationNotificationIDs.current.delete(operationID);
	  }
	}
  }, [publishOperationFailure]);

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
		options?: ConfigurationUpdateOptions,
	) => {
	const hasExpectedValue = options != null && Object.prototype.hasOwnProperty.call(options, 'expectedValue');
	// ConfigurationUpdateOptions admits exactly one condition; an explicit revision wins over the live one.
	const hasExpectedRevision = !hasExpectedValue && options != null && Object.prototype.hasOwnProperty.call(options, 'expectedRevision');
	if (hasExpectedValue && options?.expectedValue === undefined) {
		return Promise.reject(new Error('A section-scoped configuration update requires a concrete expected value.'));
	}
	const configurationScope = CitadelAPI.configurationScope();
	const execute = async () => {
	  try {
		const snapshot = await CitadelAPI.updateConfiguration(section, value, hasExpectedValue
			? { expectedValue: options?.expectedValue }
			: { expectedRevision: hasExpectedRevision ? options?.expectedRevision : configurationRef.current?.revision }, configurationScope);
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
		if (shouldToastConfigurationError(requestError, options?.conflictShownByEditor)) {
			Notifications.error(errorMessage(requestError));
		}
		throw requestError;
	  }
	};
	const result = configurationUpdateQueue.current.then(execute, execute);
	configurationUpdateQueue.current = result.then(() => undefined, () => undefined);
	return result;
  }, [acceptConfigurationSnapshot]);

	const applyPlayerHistoryRetention = useCallback((
		retention: string,
		recordingIntervalSeconds: number,
		expectedRevision: number,
		expectedConfigured: string,
		expectedRecordingIntervalSeconds: number,
	) => {
		const execute = async () => {
			try {
				let policy = await CitadelAPI.getPlayerHistoryRetention(runtimeScope);
				for (let attempt = 0; attempt < 2; attempt += 1) {
					const policyIntervalSeconds = Number(policy.recordingIntervalSeconds) || 60 * 60;
					if (policy.revision < expectedRevision ||
						(policy.configured !== expectedConfigured && policy.configured !== retention) ||
						(policyIntervalSeconds !== expectedRecordingIntervalSeconds &&
							policyIntervalSeconds !== recordingIntervalSeconds)) {
						throw new APIError(
							'My Stats storage policy changed before this update could be applied. Please review the current settings and try again.',
							409,
							'history_retention_conflict',
						);
					}
					try {
						return await CitadelAPI.applyPlayerHistoryRetention(
							retention,
							policy.revision,
							recordingIntervalSeconds,
							runtimeScope,
						);
					} catch (requestError) {
						if (!(requestError instanceof APIError)
							|| requestError.code !== 'history_retention_conflict'
							|| attempt > 0) throw requestError;
						policy = await CitadelAPI.getPlayerHistoryRetention(runtimeScope);
					}
				}
				throw new Error('My Stats storage policy could not be applied.');
			} catch (requestError) {
				Notifications.error(errorMessage(requestError));
				throw requestError;
			}
		};
		const result = configurationUpdateQueue.current.then(execute, execute);
		configurationUpdateQueue.current = result.then(() => undefined, () => undefined);
		return result;
	}, [runtimeScope]);

	const getPlayerHistoryRetention = useCallback(
		() => CitadelAPI.getPlayerHistoryRetention(runtimeScope),
		[runtimeScope],
	);

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
    loadLatestConfiguration,
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
	getPlayerHistoryRetention,
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
    loadLatestConfiguration,
    refreshState,
    state,
    submitIntent,
	getAllianceTargets,
	previewAllianceTargetAttack,
	cancelOperation,
    updateConfiguration,
	getPlayerHistoryRetention,
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

function newClientOperationID(): string {
	return globalThis.crypto?.randomUUID?.()
		?? `ui-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}
