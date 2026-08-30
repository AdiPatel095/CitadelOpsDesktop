import type { BuildingTargetCaptureResponse } from '../api/Contracts';
import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_BERI_WORLD_BLUEPRINTS_SECTION = 'automation.autoBeriWorldBlueprints';
export const AUTO_BERI_MINIMUM_STABLE_LEVEL = 1;
export const AUTO_BERI_MAXIMUM_STABLE_LEVEL = 5;
export const AUTO_BERI_DEFAULT_STABLE_LEVEL = AUTO_BERI_MAXIMUM_STABLE_LEVEL;

export const AUTO_BERI_COIN_ATTACK_TOOLS = [
	{ id: 614, name: 'Scaling ladders' },
	{ id: 611, name: 'Battering rams' },
	{ id: 620, name: 'Mantlets' },
] as const;

export const AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS = [
	{ id: 'MS1', label: '1 minute' },
	{ id: 'MS2', label: '5 minutes' },
	{ id: 'MS3', label: '10 minutes' },
	{ id: 'MS4', label: '30 minutes' },
	{ id: 'MS5', label: '1 hour' },
	{ id: 'MS6', label: '5 hours' },
	{ id: 'MS7', label: '24 hours' },
] as const;

export type AutoBeriTroopTransportTimeSkipID = typeof AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS[number]['id'];
export type AutoBeriToolMinimums = Record<string, number>;

export interface AutoBeriBuildSettings {
	enabled: boolean;
	stableLevel: number;
	allowPremium: boolean;
	allowDemolition: boolean;
	allowTimeSkips: boolean;
	resourceReserves: Record<string, number>;
	timeSkipReserve: Record<string, number>;
}

export interface AutoBeriWorldSettings {
	minTroopsToTransfer: number;
	beriCastleId: number;
	transferTroopId: number;
	sourceCastleId: number;
	wireCastleId: number;
	troopSpaceCheckIntervalSec: number;
	presetId: string;
	attackCheckIntervalSec: number;
	dailyAttackLimit: number;
	horseTravelBoostId: HorseTravelBoostID;
	toolMinimums: AutoBeriToolMinimums;
	build: AutoBeriBuildSettings;
	requireActiveGallantryBooster: boolean;
	useTroopTransportTimeSkips: boolean;
	troopTransportTimeSkipId: AutoBeriTroopTransportTimeSkipID;
}

export const DEFAULT_AUTO_BERI_WORLD_SETTINGS: AutoBeriWorldSettings = {
	minTroopsToTransfer: 1,
	beriCastleId: 0,
	transferTroopId: 0,
	sourceCastleId: 0,
	wireCastleId: -1,
	troopSpaceCheckIntervalSec: 30,
	presetId: '',
	attackCheckIntervalSec: 30,
	dailyAttackLimit: 0,
	horseTravelBoostId: -1,
	toolMinimums: defaultToolMinimums(),
	build: defaultBuildSettings(),
	requireActiveGallantryBooster: false,
	useTroopTransportTimeSkips: false,
	troopTransportTimeSkipId: 'MS5',
};

export function parseAutoBeriWorldSettings(payload: unknown): AutoBeriWorldSettings {
	const value = isRecord(payload) ? payload : {};
	const rawToolMinimums = isRecord(value.toolMinimums) ? value.toolMinimums : {};
	const rawBuild = isRecord(value.build) ? value.build : {};
	return {
		minTroopsToTransfer: nonNegativeInteger(value.minTroopsToTransfer, 1),
		beriCastleId: nonNegativeInteger(value.beriCastleId ?? value.beriCastleCID, 0),
		transferTroopId: nonNegativeInteger(value.transferTroopId ?? value.transferTroopWID, 0),
		sourceCastleId: nonNegativeInteger(value.sourceCastleId ?? value.kutSourceCastleSCID, 0),
		wireCastleId: integer(value.wireCastleId ?? value.kutCastleCID, -1),
		troopSpaceCheckIntervalSec: clamp(integer(value.troopSpaceCheckIntervalSec, 30), 5, 3600),
		presetId: typeof value.presetId === 'string' ? value.presetId.trim() : '',
		attackCheckIntervalSec: clamp(integer(value.attackCheckIntervalSec, 30), 30, 3600),
		dailyAttackLimit: clamp(integer(value.dailyAttackLimit, 0), 0, Number.MAX_SAFE_INTEGER),
		horseTravelBoostId: parseHorseTravelBoostID(value.horseTravelBoostId),
		toolMinimums: Object.fromEntries(AUTO_BERI_COIN_ATTACK_TOOLS.map((tool) => [
			String(tool.id),
			nonNegativeInteger(rawToolMinimums[String(tool.id)], 0),
		])),
		build: {
			enabled: rawBuild.enabled === true,
			stableLevel: clamp(
				integer(rawBuild.stableLevel, AUTO_BERI_DEFAULT_STABLE_LEVEL),
				AUTO_BERI_MINIMUM_STABLE_LEVEL,
				AUTO_BERI_MAXIMUM_STABLE_LEVEL,
			),
			allowPremium: rawBuild.allowPremium === true,
			allowDemolition: rawBuild.allowDemolition === true,
			allowTimeSkips: rawBuild.allowTimeSkips === true,
			resourceReserves: parseNumberMap(rawBuild.resourceReserves),
			timeSkipReserve: parseNumberMap(rawBuild.timeSkipReserve),
		},
		requireActiveGallantryBooster: value.requireActiveGallantryBooster === true,
		useTroopTransportTimeSkips: value.useTroopTransportTimeSkips === true,
		troopTransportTimeSkipId: parseTroopTransportTimeSkipID(value.troopTransportTimeSkipId),
	};
}

export interface AutoBeriBlueprint {
	id: string;
	name: string;
	createdAt: string;
	updatedAt: string;
	target: BuildingTargetCaptureResponse;
}

export interface AutoBeriBlueprintDocument {
	version: 1;
	activeId: string;
	blueprints: Record<string, AutoBeriBlueprint>;
}

export function parseAutoBeriBlueprintDocument(value: unknown): AutoBeriBlueprintDocument {
	const fallback: AutoBeriBlueprintDocument = { version: 1, activeId: '', blueprints: {} };
	if (!isRecord(value) || !isRecord(value.blueprints)) return fallback;
	const blueprints: Record<string, AutoBeriBlueprint> = {};
	for (const [key, raw] of Object.entries(value.blueprints)) {
		if (!isRecord(raw)) continue;
		const target = parseTarget(raw.target);
		const id = stringValue(raw.id) || key.trim();
		if (!target || !id) continue;
		blueprints[id] = {
			id,
			name: stringValue(raw.name) || blueprintModeLabel(target.mode),
			createdAt: stringValue(raw.createdAt),
			updatedAt: stringValue(raw.updatedAt),
			target,
		};
	}
	const activeId = stringValue(value.activeId);
	return { version: 1, activeId: blueprints[activeId] ? activeId : '', blueprints };
}

export function saveAutoBeriBlueprint(
	value: unknown,
	target: BuildingTargetCaptureResponse,
	now = new Date().toISOString(),
): AutoBeriBlueprintDocument {
	const document = parseAutoBeriBlueprintDocument(value);
	const id = target.mode === 'functional' ? 'beri-functional'
		: target.mode === 'layout' || target.mode === 'buildings' ? 'beri-layout'
			: 'beri-exact';
	const existing = document.blueprints[id];
	return {
		version: 1,
		activeId: id,
		blueprints: {
			...document.blueprints,
			[id]: {
				id,
				name: blueprintModeLabel(target.mode),
				createdAt: existing?.createdAt || now,
				updatedAt: now,
				target,
			},
		},
	};
}

export function activateAutoBeriBlueprint(value: unknown, id: string): AutoBeriBlueprintDocument {
	const document = parseAutoBeriBlueprintDocument(value);
	const activeId = id.trim();
	if (activeId && !document.blueprints[activeId]) throw new Error(`Berimond blueprint ${activeId} does not exist`);
	return { ...document, activeId };
}

function defaultToolMinimums(): AutoBeriToolMinimums {
	return Object.fromEntries(AUTO_BERI_COIN_ATTACK_TOOLS.map((tool) => [String(tool.id), 0]));
}

function defaultBuildSettings(): AutoBeriBuildSettings {
	return {
		enabled: false,
		stableLevel: AUTO_BERI_DEFAULT_STABLE_LEVEL,
		allowPremium: false,
		allowDemolition: false,
		allowTimeSkips: false,
		resourceReserves: {},
		timeSkipReserve: {},
	};
}

function parseTarget(value: unknown): BuildingTargetCaptureResponse | undefined {
	if (!isRecord(value)
		|| value.version !== 1
		|| nonNegativeInteger(value.castleId, 0) <= 0
		|| !Number.isFinite(Number(value.kingdomId))
		|| !['functional', 'layout', 'exact', 'buildings', 'full'].includes(String(value.mode))
		|| !Array.isArray(value.ground)
		|| !Array.isArray(value.buildings)
		|| !isRecord(value.summary)) {
		return undefined;
	}
	return value as unknown as BuildingTargetCaptureResponse;
}

function blueprintModeLabel(mode: BuildingTargetCaptureResponse['mode']): string {
	if (mode === 'functional') return 'Functional target';
	if (mode === 'layout' || mode === 'buildings') return 'Layout target';
	return 'Exact clone';
}

function parseNumberMap(value: unknown): Record<string, number> {
	if (!isRecord(value)) return {};
	const result: Record<string, number> = {};
	for (const [key, raw] of Object.entries(value)) {
		const amount = nonNegativeInteger(raw, 0);
		if (amount > 0) result[key.trim()] = amount;
	}
	return result;
}

function stringValue(value: unknown): string {
	return typeof value === 'string' ? value.trim() : '';
}

function parseTroopTransportTimeSkipID(value: unknown): AutoBeriTroopTransportTimeSkipID {
	const id = typeof value === 'string' ? value.trim().toUpperCase() : '';
	const match = AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS.find((skip) => skip.id === id);
	return match?.id ?? DEFAULT_AUTO_BERI_WORLD_SETTINGS.troopTransportTimeSkipId;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function integer(value: unknown, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) ? Math.trunc(parsed) : fallback;
}

function nonNegativeInteger(value: unknown, fallback: number): number {
	return Math.max(0, integer(value, fallback));
}

function clamp(value: number, minimum: number, maximum: number): number {
	return Math.min(maximum, Math.max(minimum, value));
}
