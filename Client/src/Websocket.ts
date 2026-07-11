import { CitadelAPI } from './api/CitadelClient';
import type { APIConnectionStatus, APIEnvelope } from './api/Contracts';
import type { FeatureSchedules } from './settings/SchedulerTypes';

type MessageListener = (message: any) => void;
export type FrontendWebsocketStatus = APIConnectionStatus;
type StatusListener = (status: FrontendWebsocketStatus) => void;

/**
 * Temporary source-compatibility surface for feature views that have not yet moved to useCitadelAPI.
 * It does not own a websocket: all traffic is submitted through the versioned v2 client as intents.
 */
class LegacyIntentBridge {
	private listeners = new Set<MessageListener>();
	private statusListeners = new Set<StatusListener>();
	private status: FrontendWebsocketStatus = 'Disconnected';

	constructor() {
		CitadelAPI.subscribeStatus((status) => {
			this.status = status;
			for (const listener of this.statusListeners) listener(status);
		});
		CitadelAPI.subscribe((message) => this.forwardEvent(message));
	}

	connect() {
		CitadelAPI.connect();
	}

	addMessageListener(listener: MessageListener) {
		this.listeners.add(listener);
	}

	removeMessageListener(listener: MessageListener) {
		this.listeners.delete(listener);
	}

	addStatusListener(listener: StatusListener) {
		this.statusListeners.add(listener);
		listener(this.status);
	}

	removeStatusListener(listener: StatusListener) {
		this.statusListeners.delete(listener);
	}

	getStatus(): FrontendWebsocketStatus {
		return this.status;
	}

	sendMessage(message: Record<string, any>): boolean {
		const type = typeof message.type === 'string' ? message.type : '';
		if (!type) return false;
		const { type: _type, payload, ...fields } = message;
		const argumentsValue = isRecord(payload) ? payload : fields;
		return this.submit(legacyIntentName(type), argumentsValue);
	}

	showAlert(category: 'green' | 'yellow' | 'red', message: string) {
		this.emit({ type: 'alert', payload: { category, message } });
	}

	startGame() { return this.submit('session.start'); }
	stopGame() { return this.submit('session.stop'); }
	refreshEquipment() { return this.submit('equipment.refresh'); }
	sendFetchAllianceInfo() { return this.submit('alliance.refresh'); }
	sendGetAllianceTargets(allianceId = '') { return this.submit('alliance_targets.query', { allianceId }); }
	sendAllianceTargetSpy(targetX: number, targetY: number) { return this.submit('spy.launch', { targetX, targetY }); }

	refreshSingleCommander(equipmentMode: 'Commander' | 'Castellan', targetIndex: number) {
		return this.submit('equipment.refresh_leader', { equipmentMode, targetIndex });
	}

	sendReconfigureLoadout(payload: {
		equipmentMode: 'Commander' | 'Castellan';
		combatMode: 'PvP' | 'PvE';
		targetIndex: number;
		stats: Array<{ stat: string; tier: number; position: number }>;
	}) {
		return this.submit('equipment.plan_loadout', payload);
	}

	sendConfirmReconfigure(targetIndex: number, currentLoadout: any, newLoadout: any, equipmentMode: 'Commander' | 'Castellan') {
		return this.submit('equipment.apply_loadout', { targetIndex, currentLoadout, newLoadout, equipmentMode });
	}

	triggerUpdate(downloadUrl: string) { return this.submit('application.update', { downloadUrl }); }
	sendGetSchedulerSettings() { return this.submit('settings.query'); }

	sendSaveSchedulerSettings(payload: Partial<{
		minAttackDelay: number;
		maxAttackDelay: number;
		upgradeEreDelayMs: number;
		upgradeCoinThreshold: number;
		manualFocusIdleSec: number;
		tabPriorities: Record<string, string>;
		featureSchedules: FeatureSchedules;
	}>) {
		return this.submit('settings.update', payload);
	}

	sendGetCastleFocus() { return true; }
	sendGetRiftMapCoords(refresh = false) { return this.submit('rift.query_map', { refresh }); }
	sendGetRiftCRALaunch() { return this.submit('rift.launches.query'); }
	sendGetMovement(refresh = false) { return refresh ? this.submit('game.refresh_movements') : true; }

	sendReplayRiftCRALaunch(options: {
		launchId: string;
		commanderID?: number;
		sourceX?: number;
		sourceY?: number;
		arriveAtUnix?: number;
	}) {
		return this.submit('rift.launch.replay', options);
	}

	sendMaidenCommsWave(options: { sourceX?: number; sourceY?: number; unitWodID?: number } = {}) {
		return this.submit('rift.maiden_wave.launch', options);
	}

	sendRenameRiftCRALaunch(launchId: string, displayName: string) {
		return this.submit('rift.launch.rename', { launchId, displayName });
	}

	sendDeleteRiftCRALaunch(launchId: string) {
		return this.submit('rift.launch.delete', { launchId });
	}

	sendGetRiftMaidenCommsSettings() { return this.submit('rift.maiden_settings.query'); }
	sendSaveRiftMaidenCommsSettings(unitWodID: number) { return this.submit('rift.maiden_settings.update', { unitWodID }); }

	sendFocusPlayerCastle(payload: { castleId: number; kingdomId: number; mapX: number; mapY: number }) {
		return this.submit('game.focus_castle', { castleId: payload.castleId });
	}

	sendGetDecorationPresets(castleId?: number) {
		return this.submit('decoration_presets.query', castleId && castleId > 0 ? { castleId } : {});
	}

	sendSaveDecorationPreset(name: string, castleId?: number) {
		return this.submit('decoration_presets.save', { name, ...(castleId && castleId > 0 ? { castleId } : {}) });
	}

	sendDeleteDecorationPreset(castleId: number, presetId: string) {
		return this.submit('decoration_presets.delete', { castleId, presetId });
	}

	sendApplyDecorationPreset(castleId: number, presetId: string, kingdomId?: number) {
		return this.submit('decoration_presets.apply', { castleId, presetId, ...(kingdomId != null ? { kingdomId } : {}) });
	}

	sendCancelDecorationApply() { return this.submit('decoration_presets.cancel'); }

	private submit(name: string, argumentsValue: Record<string, unknown> = {}): boolean {
		void CitadelAPI.submitIntent(name, argumentsValue, { actor: 'legacy-ui-bridge' }).catch((error) => {
			const message = error instanceof Error ? error.message : `Intent ${name} failed`;
			if (isPassiveQuery(name)) {
				console.debug(`Compatibility query ${name} is not implemented yet: ${message}`);
			} else {
				this.showAlert('red', message);
			}
		});
		return true;
	}

	private forwardEvent(message: APIEnvelope) {
		this.emit({ type: message.type, payload: message.payload, revision: message.revision, id: message.id });
	}

	private emit(message: unknown) {
		for (const listener of this.listeners) listener(message);
	}
}

function legacyIntentName(type: string): string {
	const mappings: Record<string, string> = {
		startGame: 'session.start',
		stopGame: 'session.stop',
		fetchAllianceInfo: 'alliance.refresh',
		getMovement: 'game.refresh_movements',
		focusPlayerCastle: 'game.focus_castle',
	};
	return mappings[type] ?? `legacy.${type.replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase()}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isPassiveQuery(name: string): boolean {
	return name.endsWith('.query') || name.startsWith('legacy.get_');
}

export const FrontendWebsocket = new LegacyIntentBridge();
