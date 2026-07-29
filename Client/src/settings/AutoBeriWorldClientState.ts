import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_BERI_COIN_ATTACK_TOOLS = [
	{ id: 614, name: 'Scaling ladders' },
	{ id: 611, name: 'Battering rams' },
	{ id: 620, name: 'Mantlets' },
] as const;

export type AutoBeriToolMinimums = Record<string, number>;

export interface AutoBeriWorldSettings {
	minTroopsToTransfer: number;
	beriCastleId: number;
	transferTroopId: number;
	sourceCastleId: number;
	wireCastleId: number;
	troopSpaceCheckIntervalSec: number;
	presetId: string;
	attackCheckIntervalSec: number;
	horseTravelBoostId: HorseTravelBoostID;
	toolMinimums: AutoBeriToolMinimums;
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
	horseTravelBoostId: -1,
	toolMinimums: defaultToolMinimums(),
};

export function parseAutoBeriWorldSettings(payload: unknown): AutoBeriWorldSettings {
	const value = isRecord(payload) ? payload : {};
	const rawToolMinimums = isRecord(value.toolMinimums) ? value.toolMinimums : {};
	return {
		minTroopsToTransfer: nonNegativeInteger(value.minTroopsToTransfer, 1),
		beriCastleId: nonNegativeInteger(value.beriCastleId ?? value.beriCastleCID, 0),
		transferTroopId: nonNegativeInteger(value.transferTroopId ?? value.transferTroopWID, 0),
		sourceCastleId: nonNegativeInteger(value.sourceCastleId ?? value.kutSourceCastleSCID, 0),
		wireCastleId: integer(value.wireCastleId ?? value.kutCastleCID, -1),
		troopSpaceCheckIntervalSec: clamp(integer(value.troopSpaceCheckIntervalSec, 30), 5, 3600),
		presetId: typeof value.presetId === 'string' ? value.presetId.trim() : '',
		attackCheckIntervalSec: clamp(integer(value.attackCheckIntervalSec, 30), 30, 3600),
		horseTravelBoostId: parseHorseTravelBoostID(value.horseTravelBoostId),
		toolMinimums: Object.fromEntries(AUTO_BERI_COIN_ATTACK_TOOLS.map((tool) => [
			String(tool.id),
			nonNegativeInteger(rawToolMinimums[String(tool.id)], 0),
		])),
	};
}

function defaultToolMinimums(): AutoBeriToolMinimums {
	return Object.fromEntries(AUTO_BERI_COIN_ATTACK_TOOLS.map((tool) => [String(tool.id), 0]));
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
